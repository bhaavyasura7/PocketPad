// Package client implements the phone side of the Wi-Fi protocol in Go. It
// backs the ppsim simulator and the receiver's end-to-end tests, and serves
// as a reference for the Android implementation.
package client

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// Options configure Dial.
type Options struct {
	DeviceID   uint64 // random if zero
	DeviceName string
	Caps       protocol.Caps
	// OnRumble and OnDisconnect are called from the read goroutine.
	OnRumble     func(protocol.Rumble)
	OnDisconnect func(protocol.DisconnectReason)
}

// RejectError reports a REJECT from the receiver.
type RejectError struct{ protocol.Reject }

func (e *RejectError) Error() string {
	return fmt.Sprintf("receiver refused the connection: %v (receiver supports v%d–v%d)", e.Reason, e.MinVersion, e.MaxVersion)
}

// ErrNoAnswer means the receiver did not answer the handshake; usually a
// wrong pairing code (the receiver stays silent) or a wrong address.
var ErrNoAnswer = errors.New("client: no answer from receiver (check the address and pairing code)")

// Client is an authenticated session with a receiver.
type Client struct {
	conn    *net.UDPConn
	opts    Options
	Welcome protocol.Welcome
	start   time.Time

	mu     sync.Mutex // guards sealer and buf
	sealer *protocol.Sealer
	buf    [protocol.MaxDatagram]byte

	opener   *protocol.Opener // read goroutine only
	rttUs    atomic.Uint32
	echoUs   atomic.Uint32
	lastRecv atomic.Int64 // UnixNano of the last authenticated packet
	done     chan struct{}
	closed   atomic.Bool
}

// Dial performs the handshake with the receiver at addr.
func Dial(ctx context.Context, addr string, pairKey []byte, opts Options) (*Client, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	if opts.DeviceID == 0 {
		var b [8]byte
		rand.Read(b[:])
		opts.DeviceID = binary.LittleEndian.Uint64(b[:])
	}
	c := &Client{conn: conn, opts: opts, start: time.Now(), done: make(chan struct{})}
	if err := c.handshake(ctx, pairKey); err != nil {
		conn.Close()
		return nil, err
	}
	c.lastRecv.Store(time.Now().UnixNano())
	go c.readLoop()
	return c, nil
}

func (c *Client) handshake(ctx context.Context, pairKey []byte) error {
	hello := protocol.Hello{DeviceID: c.opts.DeviceID, MinVersion: protocol.MinVersion, MaxVersion: protocol.MaxVersion, Caps: c.opts.Caps}
	hello.SetName(c.opts.DeviceName)
	pkt := make([]byte, protocol.HelloSize+protocol.HandshakeTagSize)
	in := make([]byte, protocol.MaxDatagram+1)

	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A fresh nonce for every attempt: the receiver drops repeats.
		rand.Read(hello.ClientNonce[:])
		protocol.PutHello(pkt, &hello)
		copy(pkt[protocol.HelloSize:], protocol.HandshakeTag(pairKey, pkt[:protocol.HelloSize], nil))
		if _, err := c.conn.Write(pkt); err != nil {
			return err
		}
		deadline := time.Now().Add(time.Duration(250<<attempt) * time.Millisecond)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		c.conn.SetReadDeadline(deadline)
		for {
			n, err := c.conn.Read(in)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					break // retry
				}
				continue // e.g. ICMP unreachable while the receiver starts
			}
			b := in[:n]
			h, err := protocol.ParseHeader(b)
			if err != nil {
				continue
			}
			if h.Type == protocol.TypeReject && len(b) == protocol.RejectSize {
				if r, err := protocol.DecodeReject(b); err == nil {
					return &RejectError{r}
				}
				continue
			}
			if _, err := protocol.CheckHeader(b); err != nil || h.Type != protocol.TypeWelcome {
				continue
			}
			if !protocol.VerifyHandshake(pairKey, b, hello.ClientNonce[:]) {
				continue
			}
			w, err := protocol.DecodeWelcome(b)
			if err != nil {
				return err
			}
			c.Welcome = w
			keys := protocol.DeriveSessionKeys(pairKey, hello.DeviceID, hello.ClientNonce, w.ServerNonce, w.SessionID)
			c.sealer = protocol.NewSealer(keys.ClientToServer)
			c.opener = protocol.NewOpener(keys.ServerToClient)
			c.conn.SetReadDeadline(time.Time{})
			return nil
		}
	}
	return ErrNoAnswer
}

func (c *Client) nowUs() uint32 { return uint32(time.Since(c.start).Microseconds()) }

func (c *Client) send(bodyLen int, put func(b []byte, seq uint16)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ext := c.sealer.Next()
	put(c.buf[:], ext.Wire())
	_, err := c.conn.Write(c.sealer.Seal(c.buf[:], bodyLen, ext))
	return err
}

// SendState sends a STATE packet.
func (c *Client) SendState(s protocol.ControllerState, flags protocol.StateFlags) error {
	w := s.ToWire(flags)
	return c.send(protocol.StateSize, func(b []byte, seq uint16) { protocol.PutState(b, seq, &w) })
}

// SendHeartbeat sends a HEARTBEAT carrying the latest RTT measurement.
func (c *Client) SendHeartbeat(battery uint8) error {
	hb := protocol.Heartbeat{TimeUs: c.nowUs(), EchoUs: c.echoUs.Load(), RttUs: c.rttUs.Load(), Battery: battery}
	return c.send(protocol.HeartbeatSize, func(b []byte, seq uint16) { protocol.PutHeartbeat(b, seq, &hb) })
}

// RTT returns the last measured round-trip time, or 0.
func (c *Client) RTT() time.Duration { return time.Duration(c.rttUs.Load()) * time.Microsecond }

// LastReceived is when the receiver last sent an authenticated packet.
func (c *Client) LastReceived() time.Time { return time.Unix(0, c.lastRecv.Load()) }

// Done is closed when the read loop ends (Close, or the receiver hung up).
func (c *Client) Done() <-chan struct{} { return c.done }

// Close sends DISCONNECT and closes the socket.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	c.send(protocol.DisconnectSize, func(b []byte, seq uint16) { protocol.PutDisconnect(b, seq, protocol.ReasonUser) })
	return c.conn.Close()
}

func (c *Client) readLoop() {
	defer close(c.done)
	in := make([]byte, protocol.MaxDatagram+1)
	for {
		n, err := c.conn.Read(in)
		if err != nil {
			if c.closed.Load() {
				return
			}
			continue
		}
		b := in[:n]
		h, err := protocol.CheckHeader(b)
		if err != nil || !h.Type.SessionPacket() {
			continue
		}
		if _, err := c.opener.Open(b); err != nil {
			continue
		}
		c.lastRecv.Store(time.Now().UnixNano())
		switch h.Type {
		case protocol.TypeHeartbeat:
			hb, err := protocol.DecodeHeartbeat(b)
			if err != nil {
				continue
			}
			c.echoUs.Store(hb.TimeUs)
			if hb.EchoUs != 0 {
				c.rttUs.Store(c.nowUs() - hb.EchoUs)
			}
		case protocol.TypeRumble:
			if r, err := protocol.DecodeRumble(b); err == nil && c.opts.OnRumble != nil {
				c.opts.OnRumble(r)
			}
		case protocol.TypeDisconnect:
			r, _ := protocol.DecodeDisconnect(b)
			if c.opts.OnDisconnect != nil {
				c.opts.OnDisconnect(r)
			}
			c.closed.Store(true)
			c.conn.Close()
			return
		}
	}
}

// Found is a receiver that answered discovery.
type Found struct {
	Addr netip.AddrPort
	protocol.DiscoverReply
}

// Discover broadcasts DISCOVER to target (normally 255.255.255.255:47770)
// and collects replies until timeout.
func Discover(target string, timeout time.Duration) ([]Found, error) {
	raddr, err := net.ResolveUDPAddr("udp4", target)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var probe [protocol.DiscoverSize]byte
	protocol.PutDiscover(probe[:])
	if _, err := conn.WriteToUDP(probe[:], raddr); err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(timeout))
	var found []Found
	seen := map[netip.AddrPort]bool{}
	in := make([]byte, protocol.MaxDatagram+1)
	for {
		n, from, err := conn.ReadFromUDPAddrPort(in)
		if err != nil {
			return found, nil
		}
		b := in[:n]
		if h, err := protocol.CheckHeader(b); err != nil || h.Type != protocol.TypeDiscoverReply {
			continue
		}
		r, err := protocol.DecodeDiscoverReply(b)
		if err != nil {
			continue
		}
		addr := netip.AddrPortFrom(from.Addr().Unmap(), r.Port)
		if !seen[addr] {
			seen[addr] = true
			found = append(found, Found{addr, r})
		}
	}
}
