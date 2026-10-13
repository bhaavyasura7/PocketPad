// Package session implements the desktop receiver: it authenticates phones,
// validates packets, keeps the latest controller state, runs the input
// watchdog and drives the gamepad backend.
package session

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/gamepad"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// Config configures a Server.
type Config struct {
	// Addr is the UDP listen address. Default ":47770".
	Addr string
	// PairKey authenticates handshakes; see protocol.PairKey.
	PairKey []byte
	// Insecure also accepts bare, unauthenticated 20-byte STATE packets.
	// For local development only.
	Insecure bool
	// Name is advertised to phones during discovery.
	Name string
	// Watchdog releases every input when no STATE arrives for this long
	// while something is held. Default 300ms.
	Watchdog time.Duration
	// SessionTimeout ends a session after this long without any valid
	// packet. Default 3s.
	SessionTimeout time.Duration
	// Heartbeat is the heartbeat interval requested from the phone.
	// Default 500ms.
	Heartbeat time.Duration
	// MaxRateHz is the highest STATE rate requested from the phone.
	// Default 250.
	MaxRateHz int
	Logger    *slog.Logger
}

func (c *Config) setDefaults() {
	if c.Addr == "" {
		c.Addr = fmt.Sprintf(":%d", protocol.DefaultPort)
	}
	if c.Watchdog <= 0 {
		c.Watchdog = 300 * time.Millisecond
	}
	if c.SessionTimeout <= 0 {
		c.SessionTimeout = 3 * time.Second
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = 500 * time.Millisecond
	}
	if c.MaxRateHz <= 0 {
		c.MaxRateHz = 250
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

const (
	pendingTimeout    = 5 * time.Second
	maxPending        = 4
	rumbleMinInterval = 20 * time.Millisecond
	rumbleRefresh     = 500 * time.Millisecond
	rumbleDurationMs  = 750
)

type session struct {
	id       uint32
	deviceID uint64
	name     string
	addr     netip.AddrPort
	caps     protocol.Caps
	opener   *protocol.Opener
	sealer   *protocol.Sealer
	created  time.Time
	lastSeen time.Time

	peerTimeUs uint32
	rttUs      uint32
	battery    uint8
	charging   bool
}

// Counters are cumulative packet statistics.
type Counters struct {
	Received         uint64 // datagrams read
	Accepted         uint64 // authenticated session packets
	States           uint64 // accepted STATE packets
	Stale            uint64 // late, duplicate or replayed
	AuthFailed       uint64 // bad tag or no matching session
	Malformed        uint64 // failed validation
	Lost             uint64 // sequence numbers never seen
	Blocked          uint64 // dropped from rate-limited sources
	Rejected         uint64 // HELLOs refused
	WatchdogReleases uint64
	Sessions         uint64
}

// Snapshot is a consistent view of the receiver for diagnostics.
type Snapshot struct {
	Connected bool
	Device    string
	Addr      netip.AddrPort
	SessionID uint32
	Since     time.Time
	State     protocol.ControllerState
	Released  bool // inputs were force-released (watchdog, disconnect or timeout)
	Battery   uint8
	Charging  bool
	PhoneRTT  time.Duration // reported by the phone; 0 if unknown
	Rumble    [2]uint16
	RateHz    float64 // accepted STATE packets per second
	Jitter    time.Duration
	ProcP50   time.Duration // datagram read → backend updated
	ProcP95   time.Duration
	ProcP99   time.Duration
	Counters
}

// Server is the desktop receiver.
type Server struct {
	cfg   Config
	pad   gamepad.Gamepad
	caps  protocol.Caps
	log   *slog.Logger
	conn  *net.UDPConn
	start time.Time

	mu      sync.Mutex
	active  *session
	pending []*session
	nonces  nonceCache
	limiter limiter
	stats   Counters
	out     [protocol.MaxDatagram]byte

	state         protocol.ControllerState // what the backend currently has
	released      bool
	lastStateAt   time.Time
	watchdog      *time.Timer
	watchdogArmed bool
	expiry        *time.Timer
	padErrLogged  time.Time

	rumbleIn               atomic.Uint32 // left<<16 | right, set by the backend
	rumbleWant, rumbleSent [2]uint16
	rumbleLastSent         time.Time
	rumbleTimer            *time.Timer

	insecureAddr netip.AddrPort
	insecureSeq  uint16
	insecureLast time.Time

	rateCount   int
	rateStart   time.Time
	rateHz      float64
	lastArrival time.Time
	lastDelta   time.Duration
	jitter      float64 // nanoseconds, RFC 3550 estimator
	proc        [1024]time.Duration
	procN       int
}

// New creates a Server that drives pad. The caller owns pad and must have
// connected it.
func New(cfg Config, pad gamepad.Gamepad) (*Server, error) {
	cfg.setDefaults()
	if len(cfg.PairKey) == 0 {
		return nil, errors.New("session: a pairing key is required")
	}
	pc := pad.Capabilities()
	caps := protocol.CapLatencyEcho
	if pc.Rumble {
		caps |= protocol.CapRumble
	}
	if pc.Guide {
		caps |= protocol.CapGuide
	}
	s := &Server{cfg: cfg, pad: pad, caps: caps, log: cfg.Logger, start: time.Now()}
	s.watchdog = time.AfterFunc(time.Hour, s.onWatchdog)
	s.watchdog.Stop()
	s.expiry = time.AfterFunc(time.Hour, s.onExpiry)
	s.expiry.Stop()
	s.rumbleTimer = time.AfterFunc(time.Hour, s.onRumbleTimer)
	s.rumbleTimer.Stop()
	pad.SetRumbleHandler(s.onRumble)
	return s, nil
}

// Listen opens the UDP socket. It is separate from Serve so callers can
// learn the bound address first.
func (s *Server) Listen() error {
	addr, err := net.ResolveUDPAddr("udp", s.cfg.Addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	// Absorb bursts (and floods) without dropping the phone's packets. The
	// OS may cap this; failure is harmless.
	conn.SetReadBuffer(1 << 20)
	s.conn = conn
	return nil
}

// LocalAddr returns the bound address after Listen.
func (s *Server) LocalAddr() netip.AddrPort {
	return s.conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

// Serve processes datagrams until ctx is cancelled, then notifies the phone,
// releases every input and closes the socket.
func (s *Server) Serve(ctx context.Context) error {
	if s.conn == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			s.conn.Close()
		case <-done:
		}
	}()
	defer s.shutdown()

	buf := make([]byte, protocol.MaxDatagram+1)
	for {
		n, from, err := s.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			// ICMP port-unreachable surfaces as a read error on some
			// platforms; it says nothing about our socket.
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			s.log.Debug("read error", "err", err)
			continue
		}
		s.handle(buf[:n], netip.AddrPortFrom(from.Addr().Unmap(), from.Port()), time.Now())
	}
}

func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pad.SetRumbleHandler(nil)
	if s.active != nil {
		s.endSession(protocol.ReasonShutdown, true, "receiver shutting down")
	}
	s.release()
	s.watchdog.Stop()
	s.expiry.Stop()
	s.rumbleTimer.Stop()
	s.conn.Close()
}

// nowUs never returns 0, which means "no timestamp" on the wire.
func (s *Server) nowUs() uint32 { return uint32(time.Since(s.start).Microseconds()) | 1 }

// handle processes one datagram. now is when it was read.
func (s *Server) handle(pkt []byte, from netip.AddrPort, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Received++

	fromActive := s.active != nil && s.active.addr == from
	if !fromActive && s.limiter.blocked(from.Addr(), now) {
		s.stats.Blocked++
		return
	}
	if s.cfg.Insecure && len(pkt) == protocol.StateSize {
		s.handleInsecure(pkt, from, now)
		return
	}
	h, err := protocol.CheckHeader(pkt)
	if err != nil {
		if errors.Is(err, protocol.ErrVersion) && (h.Type == protocol.TypeHello || h.Type == protocol.TypeDiscover) {
			s.sendReject(from, protocol.RejectVersion, now)
		}
		s.bad(from, now, &s.stats.Malformed)
		return
	}
	switch {
	case h.Type.SessionPacket():
		s.handleSession(pkt, h, from, now)
	case h.Type == protocol.TypeHello:
		s.handleHello(pkt, from, now)
	case h.Type == protocol.TypeDiscover:
		if s.limiter.allowReply(from.Addr(), now) {
			reply := protocol.AppendDiscoverReply(s.out[:0], &protocol.DiscoverReply{
				Port: s.LocalAddr().Port(), Caps: s.caps,
				MinVersion: protocol.MinVersion, MaxVersion: protocol.MaxVersion, Name: s.cfg.Name,
			})
			s.write(reply, from)
		}
	default:
		// WELCOME, REJECT and DISCOVER_REPLY only flow to the phone.
		s.bad(from, now, &s.stats.Malformed)
	}
}

func (s *Server) bad(from netip.AddrPort, now time.Time, counter *uint64) {
	*counter++
	if s.active != nil && s.active.addr == from {
		return // never lock out the connected phone over a corrupt packet
	}
	if s.limiter.bad(from.Addr(), now) {
		s.log.Warn("blocking source after repeated invalid packets", "ip", from.Addr(), "for", blockFor)
	}
}

func (s *Server) write(b []byte, to netip.AddrPort) {
	if _, err := s.conn.WriteToUDPAddrPort(b, to); err != nil {
		s.log.Debug("send failed", "to", to, "err", err)
	}
}

// ---------------------------------------------------------------------------
// Handshake

func (s *Server) handleHello(pkt []byte, from netip.AddrPort, now time.Time) {
	if !s.limiter.allowHello(from.Addr(), now) {
		s.stats.Rejected++
		return
	}
	if !protocol.VerifyHandshake(s.cfg.PairKey, pkt, nil) {
		s.bad(from, now, &s.stats.AuthFailed)
		s.log.Info("rejected handshake with wrong pairing code", "from", from)
		return
	}
	hello, err := protocol.DecodeHello(pkt)
	if err != nil {
		s.bad(from, now, &s.stats.Malformed)
		return
	}
	if s.nonces.seenOrAdd(hello.ClientNonce) {
		s.stats.Stale++
		return
	}
	version := min(hello.MaxVersion, protocol.MaxVersion)
	if version < max(hello.MinVersion, protocol.MinVersion) {
		s.stats.Rejected++
		s.sendReject(from, protocol.RejectVersion, now)
		return
	}
	if a := s.active; a != nil && a.deviceID != hello.DeviceID && now.Sub(a.lastSeen) < s.cfg.SessionTimeout {
		s.stats.Rejected++
		s.sendReject(from, protocol.RejectBusy, now)
		s.log.Info("refused second controller", "device", hello.Name(), "from", from, "connected", a.name)
		return
	}

	var rnd [protocol.NonceSize + 4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		s.log.Error("random source failed", "err", err)
		return
	}
	w := protocol.Welcome{
		SessionID:   binary.LittleEndian.Uint32(rnd[protocol.NonceSize:]) | 1,
		Version:     version,
		Caps:        hello.Caps & s.caps,
		HeartbeatMs: uint16(s.cfg.Heartbeat.Milliseconds()),
		TimeoutMs:   uint16(s.cfg.Watchdog.Milliseconds()),
		MaxRateHz:   uint16(s.cfg.MaxRateHz),
	}
	copy(w.ServerNonce[:], rnd[:protocol.NonceSize])
	keys := protocol.DeriveSessionKeys(s.cfg.PairKey, hello.DeviceID, hello.ClientNonce, w.ServerNonce, w.SessionID)

	sess := &session{
		id: w.SessionID, deviceID: hello.DeviceID, name: hello.Name(), addr: from, caps: w.Caps,
		opener: protocol.NewOpener(keys.ClientToServer), sealer: protocol.NewSealer(keys.ServerToClient),
		created: now, lastSeen: now, battery: protocol.BatteryUnknown,
	}
	s.addPending(sess, now)

	b := s.out[:protocol.WelcomeSize+protocol.HandshakeTagSize]
	protocol.PutWelcome(b, &w)
	copy(b[protocol.WelcomeSize:], protocol.HandshakeTag(s.cfg.PairKey, b[:protocol.WelcomeSize], hello.ClientNonce[:]))
	s.write(b, from)
	s.log.Debug("handshake", "device", sess.name, "from", from, "session", sess.id)
}

func (s *Server) addPending(sess *session, now time.Time) {
	s.pending = slices.DeleteFunc(s.pending, func(p *session) bool {
		return p.addr == sess.addr || now.Sub(p.created) > pendingTimeout
	})
	if len(s.pending) >= maxPending {
		s.pending = s.pending[1:]
	}
	s.pending = append(s.pending, sess)
}

func (s *Server) sendReject(to netip.AddrPort, reason protocol.RejectReason, now time.Time) {
	if !s.limiter.allowReply(to.Addr(), now) {
		return
	}
	b := s.out[:protocol.RejectSize]
	protocol.PutReject(b, &protocol.Reject{Reason: reason, MinVersion: protocol.MinVersion, MaxVersion: protocol.MaxVersion})
	s.write(b, to)
}

// promote makes a pending session, proven by its first authenticated
// packet, the active one.
func (s *Server) promote(sess *session, now time.Time) {
	s.pending = slices.DeleteFunc(s.pending, func(p *session) bool { return p == sess })
	if old := s.active; old != nil {
		reason := "replaced by a new connection from the same phone"
		if old.deviceID != sess.deviceID {
			reason = "timed out; replaced"
		}
		s.endSession(protocol.ReasonReplaced, old.addr != sess.addr, reason)
	}
	s.active = sess
	s.stats.Sessions++
	s.release() // a new session always starts neutral
	s.rumbleSent, s.rumbleLastSent = [2]uint16{}, time.Time{}
	s.expiry.Reset(s.cfg.SessionTimeout)
	s.rateStart, s.rateCount, s.lastArrival = now, 0, time.Time{}
	s.log.Info("controller connected", "device", sess.name, "from", sess.addr, "session", sess.id)
	s.flushRumble(now)
}

// endSession releases every input and forgets the active session.
func (s *Server) endSession(reason protocol.DisconnectReason, notify bool, why string) {
	a := s.active
	if a == nil {
		return
	}
	if notify {
		ext := a.sealer.Next()
		protocol.PutDisconnect(s.out[:], ext.Wire(), reason)
		s.write(a.sealer.Seal(s.out[:], protocol.DisconnectSize, ext), a.addr)
	}
	s.active = nil
	s.release()
	s.expiry.Stop()
	s.log.Info("controller disconnected", "device", a.name, "reason", why)
}

// ---------------------------------------------------------------------------
// Session packets

func (s *Server) handleSession(pkt []byte, h protocol.Header, from netip.AddrPort, now time.Time) {
	var sess *session
	var err error = protocol.ErrAuth
	for _, p := range s.pending {
		if p.addr == from && now.Sub(p.created) <= pendingTimeout {
			if _, err = p.opener.Open(pkt); err == nil {
				sess = p
				s.promote(p, now)
			}
			break
		}
	}
	var gap uint64
	if sess == nil && s.active != nil && s.active.addr == from {
		sess = s.active
		gap, err = sess.opener.Open(pkt)
	}
	switch {
	case errors.Is(err, protocol.ErrStale):
		s.stats.Stale++
		return
	case err != nil:
		s.bad(from, now, &s.stats.AuthFailed)
		return
	}
	s.stats.Accepted++
	s.stats.Lost += gap
	sess.lastSeen = now

	switch h.Type {
	case protocol.TypeState:
		_, w, err := protocol.DecodeState(pkt)
		if err != nil {
			s.stats.Malformed++
			return
		}
		s.stats.States++
		s.measureArrival(now)
		st := w.Normalized()
		if sess.caps&protocol.CapGuide == 0 {
			st.Buttons &^= protocol.ButtonGuide
		}
		s.apply(st, now)

	case protocol.TypeHeartbeat:
		hb, err := protocol.DecodeHeartbeat(pkt)
		if err != nil {
			s.stats.Malformed++
			return
		}
		sess.peerTimeUs, sess.rttUs, sess.battery = hb.TimeUs, hb.RttUs, hb.Battery
		sess.charging = hb.Flags&protocol.HeartbeatCharging != 0
		// Echo immediately so the phone's round-trip measurement excludes
		// any waiting on our side.
		ext := sess.sealer.Next()
		protocol.PutHeartbeat(s.out[:], ext.Wire(), &protocol.Heartbeat{TimeUs: s.nowUs(), EchoUs: hb.TimeUs, Battery: protocol.BatteryUnknown})
		s.write(sess.sealer.Seal(s.out[:], protocol.HeartbeatSize, ext), sess.addr)

	case protocol.TypeDisconnect:
		if sess == s.active {
			s.endSession(0, false, "phone disconnected")
		}

	default:
		s.stats.Malformed++ // RUMBLE only flows to the phone
	}
}

func (s *Server) handleInsecure(pkt []byte, from netip.AddrPort, now time.Time) {
	seq, w, err := protocol.DecodeState(pkt)
	if err != nil {
		s.bad(from, now, &s.stats.Malformed)
		return
	}
	if from != s.insecureAddr {
		if s.insecureAddr.IsValid() && now.Sub(s.insecureLast) < s.cfg.SessionTimeout {
			s.stats.Rejected++
			return
		}
		s.insecureAddr = from
		s.log.Warn("accepting unauthenticated packets (insecure mode)", "from", from)
	} else if !protocol.SeqNewer(seq, s.insecureSeq) && now.Sub(s.insecureLast) < s.cfg.SessionTimeout {
		s.stats.Stale++
		return
	}
	s.insecureSeq, s.insecureLast = seq, now
	s.stats.Accepted++
	s.stats.States++
	s.measureArrival(now)
	s.apply(w.Normalized(), now)
}

// apply hands a state to the backend if it changed and arms the watchdog
// while anything is held.
func (s *Server) apply(st protocol.ControllerState, now time.Time) {
	s.lastStateAt = now
	s.released = false
	if st != s.state {
		s.state = st
		if err := s.pad.Update(st); err != nil && now.Sub(s.padErrLogged) > 5*time.Second {
			s.padErrLogged = now
			s.log.Error("gamepad update failed", "err", err)
		}
		s.recordProc(time.Since(now))
	}
	if !st.IsNeutral() && !s.watchdogArmed {
		s.watchdogArmed = true
		s.watchdog.Reset(s.cfg.Watchdog)
	}
}

// release sets the backend to neutral.
func (s *Server) release() {
	if s.state.IsNeutral() {
		return
	}
	s.state = protocol.Neutral
	s.released = true
	if err := s.pad.Update(protocol.Neutral); err != nil {
		s.log.Error("gamepad release failed", "err", err)
	}
}

// onWatchdog fires at most once per Watchdog interval while input is held;
// packets only record a timestamp, so the hot path never touches the timer.
func (s *Server) onWatchdog() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchdogArmed = false
	if s.state.IsNeutral() {
		return
	}
	if idle := time.Since(s.lastStateAt); idle < s.cfg.Watchdog {
		s.watchdogArmed = true
		s.watchdog.Reset(s.cfg.Watchdog - idle)
		return
	}
	s.stats.WatchdogReleases++
	s.release()
	s.log.Debug("watchdog released held inputs")
}

func (s *Server) onExpiry() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return
	}
	if idle := time.Since(s.active.lastSeen); idle < s.cfg.SessionTimeout {
		s.expiry.Reset(s.cfg.SessionTimeout - idle)
		return
	}
	s.endSession(protocol.ReasonTimeout, true, "timed out")
}

// ---------------------------------------------------------------------------
// Rumble

// onRumble is the backend's rumble handler. It never takes the server lock:
// backends may call it while Update is running under that lock.
func (s *Server) onRumble(left, right uint16) {
	s.rumbleIn.Store(uint32(left)<<16 | uint32(right))
	s.rumbleTimer.Reset(0)
}

func (s *Server) onRumbleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.rumbleIn.Load()
	s.rumbleWant = [2]uint16{uint16(v >> 16), uint16(v)}
	s.flushRumble(time.Now())
}

// flushRumble sends the wanted motor state when it changed (at most every
// rumbleMinInterval) and refreshes a running motor before the phone's
// duration limit stops it.
func (s *Server) flushRumble(now time.Time) {
	a := s.active
	if a == nil || a.caps&protocol.CapRumble == 0 {
		s.rumbleTimer.Stop()
		return
	}
	on := s.rumbleWant != [2]uint16{}
	since := now.Sub(s.rumbleLastSent)
	changed := s.rumbleWant != s.rumbleSent
	if (changed || on && since >= rumbleRefresh) && since >= rumbleMinInterval {
		r := protocol.Rumble{Left: s.rumbleWant[0], Right: s.rumbleWant[1]}
		if on {
			r.DurationMs = rumbleDurationMs
		}
		ext := a.sealer.Next()
		protocol.PutRumble(s.out[:], ext.Wire(), &r)
		s.write(a.sealer.Seal(s.out[:], protocol.RumbleSize, ext), a.addr)
		s.rumbleSent, s.rumbleLastSent, since, changed = s.rumbleWant, now, 0, false
	}
	switch {
	case changed:
		s.rumbleTimer.Reset(rumbleMinInterval - since)
	case on:
		s.rumbleTimer.Reset(rumbleRefresh - since)
	default:
		s.rumbleTimer.Stop()
	}
}

// ---------------------------------------------------------------------------
// Diagnostics

func (s *Server) measureArrival(now time.Time) {
	if !s.lastArrival.IsZero() {
		d := now.Sub(s.lastArrival)
		if s.lastDelta != 0 {
			diff := float64(d - s.lastDelta)
			if diff < 0 {
				diff = -diff
			}
			s.jitter += (diff - s.jitter) / 16
		}
		s.lastDelta = d
	}
	s.lastArrival = now
	s.rateCount++
	if el := now.Sub(s.rateStart); el >= time.Second {
		s.rateHz = float64(s.rateCount) / el.Seconds()
		s.rateCount, s.rateStart = 0, now
	}
}

func (s *Server) recordProc(d time.Duration) {
	s.proc[s.procN%len(s.proc)] = d
	s.procN++
}

// Snapshot returns the current diagnostics.
func (s *Server) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{
		State: s.state, Released: s.released, Counters: s.stats,
		Rumble: s.rumbleWant, Jitter: time.Duration(s.jitter), Battery: protocol.BatteryUnknown,
	}
	if time.Since(s.lastArrival) < 2*time.Second {
		snap.RateHz = s.rateHz
	}
	if a := s.active; a != nil {
		snap.Connected, snap.Device, snap.Addr, snap.SessionID, snap.Since = true, a.name, a.addr, a.id, a.created
		snap.Battery, snap.Charging, snap.PhoneRTT = a.battery, a.charging, time.Duration(a.rttUs)*time.Microsecond
	} else if s.cfg.Insecure && s.insecureAddr.IsValid() && time.Since(s.insecureLast) < s.cfg.SessionTimeout {
		snap.Connected, snap.Device, snap.Addr = true, "(unauthenticated)", s.insecureAddr
	}
	if n := min(s.procN, len(s.proc)); n > 0 {
		p := slices.Clone(s.proc[:n])
		slices.Sort(p)
		at := func(q float64) time.Duration { return p[min(n-1, int(q*float64(n)))] }
		snap.ProcP50, snap.ProcP95, snap.ProcP99 = at(0.50), at(0.95), at(0.99)
	}
	return snap
}
