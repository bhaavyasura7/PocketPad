// Command pocketpad is the PocketPad desktop receiver.
//
//	pocketpad serve   receive controller input and drive a virtual gamepad
//	pocketpad pair    show the pairing code to type into the phone
//	pocketpad backends
//	pocketpad version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
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
	} else if strings.Contains(os.Args[0], ".app/Contents/MacOS/") {
		// The bundle always opens the window, even with flags such as
		// "open PocketPad.app --args -profile racing".
		cmd = "app"
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "app":
		err = runApp(args)
	case "pair":
		err = pair(args)
	case "keys":
		err = printKeys()
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
  pocketpad app [flags]     macOS window with the pairing QR code (receiver included)
  pocketpad pair [-reset]   show the pairing code to enter on the phone
  pocketpad keys            list every key name for custom mapping.json files
  pocketpad backends        list gamepad backends
  pocketpad version

Run "pocketpad serve -h" or "pocketpad app -h" for flags.
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

// resolveProfile picks the keyboard/mouse mapping: the flag wins, then the
// selection remembered in the config, then "general". Built-in names select
// a preset; "custom" (or a path to a .json file) loads a fully custom
// mapping and creates a template when none exists yet. It returns the
// display name and, for custom mappings, the file to watch for live reload.
func resolveProfile(f *config.File, flagValue string) (name string, mappingPath string, err error) {
	name = flagValue
	if name == "" {
		name = f.Profile
	}
	if name == "" {
		name = "general"
	}

	if name == "custom" || strings.HasSuffix(strings.ToLower(name), ".json") {
		path := name
		if name == "custom" {
			path = filepath.Join(filepath.Dir(f.Path()), "mapping.json")
		}
		p, loadErr := gamepad.LoadProfileFile(path)
		if loadErr != nil {
			if errors.Is(loadErr, os.ErrNotExist) {
				if writeErr := os.WriteFile(path, gamepad.TemplateMappingJSON(), 0o600); writeErr == nil {
					return "", "", fmt.Errorf(
						"created a mapping template at %s — edit it and start again "+
							"(\"pocketpad keys\" lists every key name)", path)
				}
			}
			return "", "", loadErr
		}
		gamepad.SetCustomProfile(p)
		if f.Profile != name {
			if saveErr := f.SetProfile(name); saveErr != nil {
				return "", "", saveErr
			}
		}
		return "custom", path, nil
	}

	if err := gamepad.SetMacKeysProfile(name); err != nil {
		return "", "", err
	}
	if f.Profile != name {
		if err := f.SetProfile(name); err != nil {
			return "", "", err
		}
	}
	return name, "", nil
}

// watchMappingFile reloads a custom mapping while the receiver runs, so the
// file can be tuned in-game: save it and the change applies within seconds.
func watchMappingFile(path string, log *slog.Logger) {
	go func() {
		var last time.Time
		if fi, err := os.Stat(path); err == nil {
			last = fi.ModTime()
		}
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			fi, err := os.Stat(path)
			if err != nil || !fi.ModTime().After(last) {
				continue
			}
			last = fi.ModTime()
			p, err := gamepad.LoadProfileFile(path)
			if err != nil {
				log.Warn("mapping reload failed; keeping the previous mapping", "file", path, "error", err)
				continue
			}
			gamepad.SetCustomProfile(p)
			log.Info("mapping reloaded", "file", path)
		}
	}()
}

// printKeys lists every key name accepted in mapping.json plus a full
// example, so custom mappings never require guessing.
func printKeys() error {
	fmt.Println("Key names for mapping.json (case does not matter):")
	names := gamepad.KeyNames()
	for i := 0; i < len(names); i += 8 {
		end := i + 8
		if end > len(names) {
			end = len(names)
		}
		fmt.Println("  " + strings.Join(names[i:end], "  "))
	}
	fmt.Println()
	fmt.Println("Mouse actions:  mouseLeft  mouseRight  wheelUp  wheelDown  none")
	fmt.Println()
	fmt.Println("Example mapping.json (also created for you on the first \"-profile custom\"):")
	fmt.Println()
	_, err := os.Stdout.Write(gamepad.TemplateMappingJSON())
	return err
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
	testRumble := fs.Bool("test-rumble", false, "with -backend mock: cycle rumble strong, weak, off (1s steps) to test phone vibration")
	profile := fs.String("profile", "", "keyboard/mouse profile for the mackeys backend: "+strings.Join(gamepad.MacKeysProfileNames(), ", ")+" (remembered for next time)")
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

	profileName, mappingFile, err := resolveProfile(f, *profile)
	if err != nil {
		return err
	}
	if mappingFile != "" {
		watchMappingFile(mappingFile, log)
	}
	pad, backendName, err := gamepad.ConnectBest(*backend)
	if err != nil {
		return err
	}
	defer pad.Close()
	fmt.Fprintf(os.Stderr, "Gamepad backend: %s\n", backendName)
	if backendName == "mackeys" {
		if mappingFile != "" {
			fmt.Fprintf(os.Stderr, "Keyboard/mouse mapping: custom, live reload from %s\n", mappingFile)
		} else {
			fmt.Fprintf(os.Stderr, "Keyboard/mouse profile: %s\n", profileName)
		}
	}
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
	fmt.Fprintf(os.Stderr, "\n  Pairing code:  %s\n", protocol.FormatPairingCode(secret))
	fmt.Fprintln(os.Stderr, "  Enter it in the PocketPad app (same Wi-Fi network). Press Ctrl+C to stop.")

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

	if *testRumble {
		mock, ok := pad.(*gamepad.Mock)
		if !ok {
			return fmt.Errorf("-test-rumble needs -backend mock")
		}
		go pulseRumble(ctx, mock)
	}

	err = srv.Serve(ctx)
	fmt.Fprintln(os.Stderr, "\nstopped; all inputs released")
	return err
}

// pulseRumble cycles strong, weak and no rumble, as a game would.
func pulseRumble(ctx context.Context, m *gamepad.Mock) {
	steps := [][2]uint16{{0xFFFF, 0xFFFF}, {0x4000, 0}, {0, 0}}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s := steps[i%len(steps)]
			m.SimulateRumble(s[0], s[1])
		}
	}
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
