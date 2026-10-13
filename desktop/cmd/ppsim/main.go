// Command ppsim simulates the PocketPad phone app, for testing a receiver
// without an Android device.
//
//	ppsim -code XXXX-XXXX-XXXX-XXXX-XXXX-XXXX            # find a receiver and connect
//	ppsim -addr 192.168.1.20:47770 -code ... -pattern buttons
//	ppsim -addr 127.0.0.1:47770 -insecure                 # bare 20-byte packets
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/client"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/config"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func main() {
	addr := flag.String("addr", "", "receiver address (default: discover on the LAN)")
	code := flag.String("code", "", "pairing code (default: read the local receiver's config)")
	rate := flag.Int("rate", 0, "STATE packets per second (default: the receiver's maximum)")
	pattern := flag.String("pattern", "mixed", "input pattern: mixed, circle, buttons, hold, idle")
	duration := flag.Duration("duration", 0, "stop after this long (default: until Ctrl+C)")
	insecure := flag.Bool("insecure", false, "send bare unauthenticated packets (receiver must use -insecure)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	if err := run(ctx, *addr, *code, *rate, *pattern, *insecure); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "ppsim:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, code string, rate int, pattern string, insecure bool) error {
	gen, ok := patterns[pattern]
	if !ok {
		return fmt.Errorf("unknown pattern %q", pattern)
	}
	if addr == "" {
		fmt.Println("discovering receivers…")
		found, err := client.Discover(fmt.Sprintf("255.255.255.255:%d", protocol.DefaultPort), time.Second)
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return errors.New("no receiver answered; pass -addr")
		}
		for _, f := range found {
			fmt.Printf("  found %q at %s\n", f.Name, f.Addr)
		}
		addr = found[0].Addr.String()
	}
	if insecure {
		return runInsecure(ctx, addr, rate, gen)
	}

	var secret []byte
	var err error
	if code != "" {
		secret, err = protocol.ParsePairingCode(code)
	} else {
		var path string
		if path, err = config.DefaultPath(); err == nil {
			var f *config.File
			if f, err = config.Load(path); err == nil {
				secret, err = f.Secret()
			}
		}
	}
	if err != nil {
		return fmt.Errorf("pairing code: %w", err)
	}

	host, _ := os.Hostname()
	c, err := client.Dial(ctx, addr, protocol.PairKey(secret), client.Options{
		DeviceName: "ppsim@" + host,
		Caps:       protocol.CapRumble | protocol.CapBattery | protocol.CapLatencyEcho | protocol.CapGuide,
		OnRumble: func(r protocol.Rumble) {
			fmt.Printf("\nrumble: left %3d%% right %3d%% for %dms\n", int(r.Left)*100/0xFFFF, int(r.Right)*100/0xFFFF, r.DurationMs)
		},
		OnDisconnect: func(r protocol.DisconnectReason) { fmt.Printf("\nreceiver ended the session (reason %d)\n", r) },
	})
	if err != nil {
		return err
	}
	defer c.Close()
	if rate <= 0 {
		rate = int(c.Welcome.MaxRateHz)
	}
	fmt.Printf("connected to %s: session %08x, %d Hz, heartbeat %dms, watchdog %dms\n",
		addr, c.Welcome.SessionID, rate, c.Welcome.HeartbeatMs, c.Welcome.TimeoutMs)

	tick := time.NewTicker(time.Second / time.Duration(rate))
	defer tick.Stop()
	hb := time.NewTicker(time.Duration(c.Welcome.HeartbeatMs) * time.Millisecond)
	defer hb.Stop()
	report := time.NewTicker(time.Second)
	defer report.Stop()

	start := time.Now()
	var last protocol.ControllerState
	var lastSent time.Time
	sent := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\ndisconnecting")
			return ctx.Err()
		case <-c.Done():
			return errors.New("session ended")
		case <-hb.C:
			c.SendHeartbeat(uint8(80))
			if time.Since(c.LastReceived()) > 3*time.Second {
				return errors.New("receiver stopped answering")
			}
		case <-report.C:
			fmt.Printf("\rsent %d packets/s  rtt %v      ", sent, c.RTT().Round(10*time.Microsecond))
			sent = 0
		case now := <-tick.C:
			// Like the app: send changes at the input rate, and while
			// something is held resend it every 50ms so the receiver's
			// watchdog stays satisfied. Nothing is sent while idle.
			s := gen(now.Sub(start))
			switch {
			case s != last:
				c.SendState(s, 0)
			case !s.IsNeutral() && now.Sub(lastSent) >= 50*time.Millisecond:
				c.SendState(s, protocol.FlagSnapshot)
			default:
				continue
			}
			last, lastSent = s, now
			sent++
		}
	}
}

func runInsecure(ctx context.Context, addr string, rate int, gen func(time.Duration) protocol.ControllerState) error {
	if rate <= 0 {
		rate = 250
	}
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	fmt.Printf("sending unauthenticated packets to %s at %d Hz\n", addr, rate)
	tick := time.NewTicker(time.Second / time.Duration(rate))
	defer tick.Stop()
	start := time.Now()
	var buf [protocol.StateSize]byte
	var seq uint16
	for {
		select {
		case <-ctx.Done():
			w := protocol.WireState{}
			protocol.PutState(buf[:], seq, &w)
			conn.Write(buf[:]) // release everything
			return ctx.Err()
		case now := <-tick.C:
			s := gen(now.Sub(start))
			w := s.ToWire(0)
			protocol.PutState(buf[:], seq, &w)
			seq++
			conn.Write(buf[:])
		}
	}
}

var patterns = map[string]func(time.Duration) protocol.ControllerState{
	"idle": func(time.Duration) protocol.ControllerState { return protocol.Neutral },
	"hold": func(time.Duration) protocol.ControllerState {
		return protocol.ControllerState{Buttons: protocol.ButtonA}
	},
	"circle": func(t time.Duration) protocol.ControllerState {
		a := t.Seconds() * math.Pi
		return protocol.ControllerState{
			LeftX: float32(math.Cos(a)), LeftY: float32(math.Sin(a)),
			RightX: float32(math.Cos(-a / 2)), RightY: float32(math.Sin(-a / 2)),
		}
	},
	"buttons": func(t time.Duration) protocol.ControllerState {
		// Walk through every button, then every D-pad direction, half a
		// second each.
		i := int(t / (500 * time.Millisecond))
		n := 11 + 8
		i %= n
		if i < 11 {
			return protocol.ControllerState{Buttons: 1 << i}
		}
		return protocol.ControllerState{DPad: protocol.DPad(i - 11 + 1)}
	},
	"mixed": func(t time.Duration) protocol.ControllerState {
		a := t.Seconds() * math.Pi
		trig := float32(0.5 + 0.5*math.Sin(a/2))
		s := protocol.ControllerState{
			LeftX: float32(math.Cos(a)), LeftY: float32(math.Sin(a)),
			LeftTrigger: trig, RightTrigger: 1 - trig,
		}
		// Hold A and B together for a second out of every three, to show
		// simultaneous presses.
		if int(t.Seconds())%3 == 0 {
			s.Buttons = protocol.ButtonA | protocol.ButtonB
		}
		return s
	},
}
