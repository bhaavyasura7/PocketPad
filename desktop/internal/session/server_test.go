package session

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/client"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/gamepad"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

var testKey = protocol.PairKey([]byte("test pairing!!!"))

type harness struct {
	srv  *Server
	pad  *gamepad.Mock
	addr string
}

func start(t *testing.T, cfg Config) *harness {
	t.Helper()
	cfg.Addr = "127.0.0.1:0"
	if cfg.PairKey == nil {
		cfg.PairKey = testKey
	}
	if cfg.Watchdog == 0 {
		cfg.Watchdog = 100 * time.Millisecond
	}
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = 400 * time.Millisecond
	}
	cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg.Name = "test receiver"
	pad := gamepad.NewMock()
	pad.Connect()
	srv, err := New(cfg, pad)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { srv.Serve(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return &harness{srv: srv, pad: pad, addr: srv.LocalAddr().String()}
}

func (h *harness) dial(t *testing.T, opts client.Options) *client.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if opts.DeviceName == "" {
		opts.DeviceName = "test phone"
	}
	if opts.Caps == 0 {
		opts.Caps = protocol.CapRumble | protocol.CapLatencyEcho | protocol.CapGuide
	}
	c, err := client.Dial(ctx, h.addr, testKey, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// eventually polls cond for up to a second.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (h *harness) padState() protocol.ControllerState {
	s, _ := h.pad.State()
	return s
}

func TestStateReachesBackend(t *testing.T) {
	h := start(t, Config{})
	c := h.dial(t, client.Options{})
	if c.Welcome.MaxRateHz != 250 || c.Welcome.TimeoutMs != 100 || c.Welcome.Caps&protocol.CapRumble == 0 {
		t.Fatalf("welcome %+v", c.Welcome)
	}
	want := protocol.ControllerState{
		LeftX: 1, LeftY: -1, RightX: -0.5, RightTrigger: 1,
		Buttons: protocol.ButtonA | protocol.ButtonB | protocol.ButtonLB | protocol.ButtonStart | protocol.ButtonGuide,
		DPad:    protocol.DPadUpRight,
	}
	if err := c.SendState(want, 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, "state", func() bool { return h.padState().Buttons == want.Buttons })
	got := h.padState()
	if got.LeftX != 1 || got.LeftY != -1 || got.RightTrigger != 1 || got.DPad != protocol.DPadUpRight || got.RightX > -0.49 || got.RightX < -0.51 {
		t.Fatalf("got %+v", got)
	}
	snap := h.srv.Snapshot()
	if !snap.Connected || snap.Device != "test phone" || snap.States != 1 {
		t.Fatalf("snapshot %+v", snap)
	}
}

func TestGuideStrippedWithoutCapability(t *testing.T) {
	h := start(t, Config{})
	c := h.dial(t, client.Options{Caps: protocol.CapLatencyEcho})
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonGuide | protocol.ButtonX}, 0)
	eventually(t, "state", func() bool { return h.padState().Buttons != 0 })
	if h.padState().Buttons != protocol.ButtonX {
		t.Fatalf("buttons %v", h.padState().Buttons.Names())
	}
}

func TestWrongPairingCodeGetsNoAnswer(t *testing.T) {
	h := start(t, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	_, err := client.Dial(ctx, h.addr, protocol.PairKey([]byte("wrong secret!!!")), client.Options{})
	if err == nil {
		t.Fatal("handshake with the wrong key succeeded")
	}
	if snap := h.srv.Snapshot(); snap.AuthFailed == 0 || snap.Connected {
		t.Fatalf("snapshot %+v", snap)
	}
}

func TestWatchdogReleasesHeldInputs(t *testing.T) {
	h := start(t, Config{Watchdog: 80 * time.Millisecond, SessionTimeout: 5 * time.Second})
	c := h.dial(t, client.Options{})
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonA, LeftX: 1}, 0)
	eventually(t, "press", func() bool { return h.padState().Buttons == protocol.ButtonA })

	// Snapshots keep the press alive past the watchdog interval.
	for i := 0; i < 6; i++ {
		time.Sleep(30 * time.Millisecond)
		c.SendState(protocol.ControllerState{Buttons: protocol.ButtonA, LeftX: 1}, protocol.FlagSnapshot)
	}
	if h.padState().Buttons != protocol.ButtonA {
		t.Fatal("released while snapshots were arriving")
	}
	// Silence releases everything, though the session stays up.
	eventually(t, "watchdog release", func() bool { return h.padState().IsNeutral() })
	snap := h.srv.Snapshot()
	if snap.WatchdogReleases != 1 || !snap.Released || !snap.Connected {
		t.Fatalf("snapshot %+v", snap)
	}
	// The next packet restores input.
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonB}, 0)
	eventually(t, "resume", func() bool { return h.padState().Buttons == protocol.ButtonB })
}

func TestDisconnectReleasesImmediately(t *testing.T) {
	h := start(t, Config{Watchdog: 10 * time.Second, SessionTimeout: 10 * time.Second})
	c := h.dial(t, client.Options{})
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonY, RightTrigger: 1}, 0)
	eventually(t, "press", func() bool { return h.padState().Buttons == protocol.ButtonY })
	c.Close()
	eventually(t, "release", func() bool { return h.padState().IsNeutral() && !h.srv.Snapshot().Connected })
}

func TestSessionTimeoutAndCleanReconnect(t *testing.T) {
	h := start(t, Config{Watchdog: 10 * time.Second, SessionTimeout: 150 * time.Millisecond})
	var reason protocol.DisconnectReason = 255
	var mu sync.Mutex
	c := h.dial(t, client.Options{DeviceID: 7, OnDisconnect: func(r protocol.DisconnectReason) { mu.Lock(); reason = r; mu.Unlock() }})
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonRB}, 0)
	eventually(t, "press", func() bool { return h.padState().Buttons == protocol.ButtonRB })

	eventually(t, "timeout", func() bool { return !h.srv.Snapshot().Connected })
	if !h.padState().IsNeutral() {
		t.Fatal("expired session left input held")
	}
	<-c.Done()
	mu.Lock()
	if reason != protocol.ReasonTimeout {
		t.Fatalf("phone told %v", reason)
	}
	mu.Unlock()

	// The same phone reconnects; nothing from the old session comes back.
	c2 := h.dial(t, client.Options{DeviceID: 7})
	c2.SendHeartbeat(50)
	eventually(t, "reconnect", func() bool { return h.srv.Snapshot().Connected })
	if !h.padState().IsNeutral() {
		t.Fatal("reconnect restored stale input")
	}
	if c.SendState(protocol.ControllerState{Buttons: protocol.ButtonA}, 0) == nil {
		time.Sleep(20 * time.Millisecond)
		if h.padState().Buttons != 0 {
			t.Fatal("old session's packets were accepted")
		}
	}
}

func TestSamePhoneCanReplaceItsSession(t *testing.T) {
	h := start(t, Config{SessionTimeout: 10 * time.Second, Watchdog: 10 * time.Second})
	c1 := h.dial(t, client.Options{DeviceID: 9})
	c1.SendState(protocol.ControllerState{Buttons: protocol.ButtonX}, 0)
	eventually(t, "press", func() bool { return h.padState().Buttons == protocol.ButtonX })

	// E.g. the phone's Wi-Fi address changed: a new handshake from a new port.
	c2 := h.dial(t, client.Options{DeviceID: 9})
	c2.SendHeartbeat(protocol.BatteryUnknown)
	eventually(t, "replace", func() bool { return h.srv.Snapshot().Sessions == 2 })
	if !h.padState().IsNeutral() {
		t.Fatal("replacing the session kept old input")
	}
	<-c1.Done() // told to go away
	c2.SendState(protocol.ControllerState{Buttons: protocol.ButtonY}, 0)
	eventually(t, "new input", func() bool { return h.padState().Buttons == protocol.ButtonY })
}

func TestSecondControllerIsRefused(t *testing.T) {
	h := start(t, Config{SessionTimeout: 10 * time.Second})
	c := h.dial(t, client.Options{DeviceID: 1})
	c.SendHeartbeat(protocol.BatteryUnknown)
	eventually(t, "connect", func() bool { return h.srv.Snapshot().Connected })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.Dial(ctx, h.addr, testKey, client.Options{DeviceID: 2})
	var rej *client.RejectError
	if !errors.As(err, &rej) || rej.Reason != protocol.RejectBusy {
		t.Fatalf("second controller: %v", err)
	}
}

func TestPendingSessionDoesNotDisturbActive(t *testing.T) {
	// A HELLO alone (e.g. replayed by someone without the pairing key, who
	// cannot follow up) must not end the current session.
	h := start(t, Config{SessionTimeout: 10 * time.Second, Watchdog: 10 * time.Second})
	c := h.dial(t, client.Options{DeviceID: 3})
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonA}, 0)
	eventually(t, "press", func() bool { return h.padState().Buttons == protocol.ButtonA })

	other := h.dial(t, client.Options{DeviceID: 3}) // handshake only, never confirmed
	_ = other
	time.Sleep(30 * time.Millisecond)
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonB}, 0)
	eventually(t, "still active", func() bool { return h.padState().Buttons == protocol.ButtonB })
}

func TestHeartbeatEchoAndBattery(t *testing.T) {
	h := start(t, Config{SessionTimeout: 10 * time.Second})
	c := h.dial(t, client.Options{})
	c.SendHeartbeat(77)
	eventually(t, "rtt", func() bool { return c.RTT() > 0 })
	c.SendHeartbeat(76) // carries the measured RTT
	eventually(t, "battery", func() bool {
		s := h.srv.Snapshot()
		return s.Battery == 76 && s.PhoneRTT > 0
	})
}

func TestRumbleForwarding(t *testing.T) {
	h := start(t, Config{SessionTimeout: 10 * time.Second})
	var mu sync.Mutex
	var got []protocol.Rumble
	c := h.dial(t, client.Options{OnRumble: func(r protocol.Rumble) { mu.Lock(); got = append(got, r); mu.Unlock() }})
	c.SendHeartbeat(protocol.BatteryUnknown)
	eventually(t, "connect", func() bool { return h.srv.Snapshot().Connected })

	last := func() (protocol.Rumble, int) {
		mu.Lock()
		defer mu.Unlock()
		if len(got) == 0 {
			return protocol.Rumble{}, 0
		}
		return got[len(got)-1], len(got)
	}
	// A burst of changes is coalesced, but the final value always arrives.
	for i := 1; i <= 50; i++ {
		h.pad.SimulateRumble(uint16(i*1000), 0x8000)
	}
	eventually(t, "rumble", func() bool { r, _ := last(); return r.Left == 50000 })
	r, n := last()
	if r.Right != 0x8000 || r.DurationMs == 0 || n > 10 {
		t.Fatalf("rumble %+v after %d packets", r, n)
	}
	h.pad.SimulateRumble(0, 0)
	eventually(t, "stop", func() bool { r, _ := last(); return r.Left == 0 && r.Right == 0 && r.DurationMs == 0 })
}

func rawConn(t *testing.T, addr string) *net.UDPConn {
	t.Helper()
	raddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetReadDeadline(time.Now().Add(time.Second))
	return conn
}

func TestVersionMismatchIsAnswered(t *testing.T) {
	h := start(t, Config{})
	conn := rawConn(t, h.addr)
	pkt := make([]byte, protocol.HelloSize+protocol.HandshakeTagSize)
	protocol.PutHello(pkt, &protocol.Hello{MinVersion: 2, MaxVersion: 2})
	pkt[2] = 2 // a future phone
	conn.Write(pkt)
	in := make([]byte, 64)
	n, err := conn.Read(in)
	if err != nil {
		t.Fatal(err)
	}
	r, err := protocol.DecodeReject(in[:n])
	if err != nil || r.Reason != protocol.RejectVersion || r.MaxVersion != protocol.MaxVersion {
		t.Fatalf("reply %x: %+v %v", in[:n], r, err)
	}
}

func TestDiscovery(t *testing.T) {
	h := start(t, Config{})
	found, err := client.Discover(h.addr, 300*time.Millisecond)
	if err != nil || len(found) != 1 || found[0].Name != "test receiver" || found[0].Addr.String() != h.addr {
		t.Fatalf("found %+v %v", found, err)
	}
}

func TestForgedAndMalformedPacketsAreDropped(t *testing.T) {
	h := start(t, Config{SessionTimeout: 10 * time.Second})
	c := h.dial(t, client.Options{})
	c.SendState(protocol.ControllerState{Buttons: protocol.ButtonA}, 0)
	eventually(t, "press", func() bool { return h.padState().Buttons == protocol.ButtonA })

	attacker := rawConn(t, h.addr)
	pkt := make([]byte, protocol.StateSize+protocol.TagSize)
	w := protocol.WireState{Buttons: protocol.ButtonStart}
	for i := 0; i < 200; i++ {
		protocol.PutState(pkt, uint16(1000+i), &w)
		attacker.Write(pkt)
		attacker.Write([]byte("hello there"))
	}
	eventually(t, "blocking", func() bool { return h.srv.Snapshot().Blocked > 0 })
	snap := h.srv.Snapshot()
	if h.padState().Buttons != protocol.ButtonA || snap.AuthFailed == 0 || snap.Malformed == 0 {
		t.Fatalf("pad %v, snapshot %+v", h.padState().Buttons.Names(), snap)
	}
	// The real phone is unaffected. It resends, as the app does, because the
	// flood may have overflowed the socket buffer and lost a packet.
	eventually(t, "phone still works", func() bool {
		c.SendState(protocol.ControllerState{Buttons: protocol.ButtonB}, protocol.FlagSnapshot)
		time.Sleep(5 * time.Millisecond)
		return h.padState().Buttons == protocol.ButtonB
	})
}

func TestInsecureMode(t *testing.T) {
	h := start(t, Config{Insecure: true})
	conn := rawConn(t, h.addr)
	var pkt [protocol.StateSize]byte
	w := protocol.WireState{Buttons: protocol.ButtonA | protocol.ButtonX, LeftX: 32767}
	protocol.PutState(pkt[:], 10, &w)
	conn.Write(pkt[:])
	eventually(t, "insecure state", func() bool { return h.padState().Buttons == w.Buttons })
	// An older sequence number is stale.
	w.Buttons = protocol.ButtonY
	protocol.PutState(pkt[:], 9, &w)
	conn.Write(pkt[:])
	time.Sleep(20 * time.Millisecond)
	if h.padState().Buttons != protocol.ButtonA|protocol.ButtonX {
		t.Fatal("stale packet applied")
	}
	// Wraparound is newer.
	h2 := start(t, Config{Insecure: true})
	conn2 := rawConn(t, h2.addr)
	protocol.PutState(pkt[:], 65535, &w)
	conn2.Write(pkt[:])
	eventually(t, "first", func() bool { return h2.padState().Buttons == protocol.ButtonY })
	w.Buttons = protocol.ButtonB
	protocol.PutState(pkt[:], 0, &w)
	conn2.Write(pkt[:])
	eventually(t, "wrapped", func() bool { return h2.padState().Buttons == protocol.ButtonB })
}

func TestSecureModeIgnoresBarePackets(t *testing.T) {
	h := start(t, Config{})
	conn := rawConn(t, h.addr)
	pkt := make([]byte, protocol.StateSize)
	protocol.PutState(pkt, 1, &protocol.WireState{Buttons: protocol.ButtonA})
	conn.Write(pkt)
	eventually(t, "counted", func() bool { return h.srv.Snapshot().Malformed == 1 })
	if !h.padState().IsNeutral() {
		t.Fatal("unauthenticated packet applied")
	}
}
