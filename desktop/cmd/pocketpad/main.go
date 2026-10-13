// Command pocketpad is the PocketPad desktop receiver.
//
//	pocketpad serve   receive controller input and drive a virtual gamepad
//	pocketpad pair    show the pairing code to type into the phone
//	pocketpad backends
//	pocketpad version
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/config"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/diag"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/gamepad"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/session"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "pair":
		err = pair(args)
	case "backends":
		fmt.Println(strings.Join(gamepad.Names(), "\n"))
	case "version":
		fmt.Printf("pocketpad %s (protocol v%d)\n", version, protocol.Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pocketpad:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `PocketPad receiver: use an Android phone as a game controller.

Usage:
  pocketpad serve [flags]   receive input and drive a virtual gamepad (default)
  pocketpad pair [-reset]   show the pairing code to enter on the phone
  pocketpad backends        list gamepad backends
  pocketpad version

Run "pocketpad serve -h" for serve flags.
`)
}

func loadConfig(path string) (*config.File, error) {
	if path == "" {
		var err error
		if path, err = config.DefaultPath(); err != nil {
			return nil, err
		}
	}
	return config.Load(path)
}

func pair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	reset := fs.Bool("reset", false, "generate a new code; every phone must pair again")
	cfgPath := fs.String("config", "", "config file (default: user config dir)")
	fs.Parse(args)

	f, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	if *reset {
		if err := f.ResetSecret(); err != nil {
			return err
		}
		fmt.Println("Generated a new pairing code. Previously paired phones must pair again.")
	}
	secret, err := f.Secret()
	if err != nil {
		return err
	}
	fmt.Printf("\n  Pairing code:  %s\n\n", protocol.FormatPairingCode(secret))
	fmt.Println("Enter it in the PocketPad app. Anyone with this code can control this")
	fmt.Println("computer's virtual gamepad on your network: keep it private.")
	return nil
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", fmt.Sprintf(":%d", protocol.DefaultPort), "UDP listen address")
	backend := fs.String("backend", "auto", "gamepad backend: auto, "+strings.Join(gamepad.Names(), ", "))
	diagMode := fs.String("diag", "auto", "diagnostic display: auto (on for terminals), live, log, off")
	watchdog := fs.Duration("watchdog", 300*time.Millisecond, "release held inputs after this long without input")
	timeout := fs.Duration("session-timeout", 3*time.Second, "end the session after this long without any packet")
	rate := fs.Int("rate", 250, "highest input rate (Hz) to request from the phone, 120–250 recommended")
	name := fs.String("name", "", "name shown on the phone (default: host name)")
	insecure := fs.Bool("insecure", false, "ALSO accept unauthenticated 20-byte packets from anyone on the network (development only)")
	verbose := fs.Bool("v", false, "verbose logging")
	cfgPath := fs.String("config", "", "config file (default: user config dir)")
	fs.Parse(args)

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	f, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	secret, err := f.Secret()
	if err != nil {
		return err
	}
	if *name == "" {
		*name = f.Name
	}
	if *name == "" {
		*name, _ = os.Hostname()
	}

	pad, err := gamepad.Open(*backend)
	if err != nil {
		return err
	}
	if err := pad.Connect(); err != nil {
		return fmt.Errorf("gamepad backend %s: %w", *backend, err)
	}
	defer pad.Close()
	if !pad.Capabilities().OSVisible {
		log.Warn("the mock backend is diagnostics only: games will not see a controller")
	}

	srv, err := session.New(session.Config{
		Addr: *listen, PairKey: protocol.PairKey(secret), Insecure: *insecure, Name: *name,
		Watchdog: *watchdog, SessionTimeout: *timeout, MaxRateHz: *rate, Logger: log,
	}, pad)
	if err != nil {
		return err
	}
	if err := srv.Listen(); err != nil {
		return err
	}
	if *insecure {
		log.Warn("insecure mode: any device on the network can send input")
	}

	port := srv.LocalAddr().Port()
	fmt.Fprintf(os.Stderr, "PocketPad receiver %s listening on UDP port %d as %q\n", version, port, *name)
	for _, ip := range lanAddrs() {
		fmt.Fprintf(os.Stderr, "  address for the phone: %s:%d\n", ip, port)
	}
	fmt.Fprintln(os.Stderr, `Run "pocketpad pair" to see the pairing code. Press Ctrl+C to stop.`)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := *diagMode
	if mode == "auto" {
		mode = "off"
		if isTerminal(os.Stdout) {
			mode = "live"
		}
	}
	switch mode {
	case "live":
		go func() {
			d := diag.NewDisplay(os.Stdout)
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					d.Draw(srv.Snapshot())
				}
			}
		}()
	case "log":
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					diag.Plain(os.Stdout, srv.Snapshot())
				}
			}
		}()
	case "off":
	default:
		return fmt.Errorf("unknown -diag mode %q", mode)
	}

	err = srv.Serve(ctx)
	fmt.Fprintln(os.Stderr, "\nstopped; all inputs released")
	return err
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// lanAddrs lists this machine's non-loopback IPv4 addresses.
func lanAddrs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n.IP.String())
		}
	}
	return out
}
