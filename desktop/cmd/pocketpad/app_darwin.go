//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -fblocks
#cgo LDFLAGS: -framework Cocoa
#include <stdint.h>
#include <stdlib.h>

void pp_app_launch(const char *title, const char *code, const char *address,
                   const uint8_t *png, size_t pngLen,
                   const uint8_t *logo, size_t logoLen);
void pp_app_set_status(const char *text);
*/
import "C"

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unsafe"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/diag"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/gamepad"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/session"
)

//go:embed pocketpad_logo.png
var pocketpadLogo []byte

// runApp is `pocketpad app`: the native macOS window with the pairing QR
// code, the pairing code itself, the LAN address and live status, with the
// receiver running in the same process. Closing the window quits.
func runApp(args []string) error {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	listen := fs.String("listen", fmt.Sprintf(":%d", protocol.DefaultPort), "UDP listen address")
	backend := fs.String("backend", "auto", "gamepad backend: auto, "+strings.Join(gamepad.Names(), ", "))
	watchdog := fs.Duration("watchdog", 300*time.Millisecond, "release held inputs after this long without input")
	timeout := fs.Duration("session-timeout", 3*time.Second, "end the session after this long without any packet")
	rate := fs.Int("rate", 250, "highest input rate (Hz) to request from the phone")
	name := fs.String("name", "", "name shown on the phone (default: host name)")
	profile := fs.String("profile", "general", "keyboard/mouse profile for the mackeys backend: "+strings.Join(gamepad.MacKeysProfileNames(), ", "))
	cfgPath := fs.String("config", "", "config file (default: user config dir)")
	fs.Parse(args)

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

	if err := gamepad.SetMacKeysProfile(*profile); err != nil {
		return err
	}
	pad, backendName, err := gamepad.ConnectBest(*backend)
	if err != nil {
		return err
	}
	defer pad.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv, err := session.New(session.Config{
		Addr: *listen, PairKey: protocol.PairKey(secret), Name: *name,
		Watchdog: *watchdog, SessionTimeout: *timeout, MaxRateHz: *rate, Logger: log,
	}, pad)
	if err != nil {
		return err
	}
	if err := srv.Listen(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	// The QR code carries everything the phone needs.
	port := srv.LocalAddr().Port()
	host := "127.0.0.1"
	if ips := lanAddrs(); len(ips) > 0 {
		host = ips[0]
	}
	code := protocol.FormatPairingCode(secret)
	png, err := qrcode.Encode(pairingURI(host, int(port), code), qrcode.Medium, 512)
	if err != nil {
		return err
	}

	// Live status line, refreshed twice a second.
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			lines := diag.Lines(srv.Snapshot())
			text := lines[0] + "\n" + lines[2]
			if backendName == "mackeys" && !gamepad.MacKeysTrusted() {
				text = "⚠︎ To control the Mac, allow Accessibility for PocketPad in " +
					"System Settings → Privacy & Security → Accessibility.\n" + text
			}
			cs := C.CString(text)
			C.pp_app_set_status(cs)
			C.free(unsafe.Pointer(cs))
		}
	}()

	title := C.CString("PocketPad — " + backendName)
	codeC := C.CString(code)
	addrC := C.CString(fmt.Sprintf("%s:%d", host, port))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(codeC))
	defer C.free(unsafe.Pointer(addrC))

	// Blocks until the window closes.
	C.pp_app_launch(title, codeC, addrC,
		(*C.uint8_t)(unsafe.Pointer(&png[0])), C.size_t(len(png)),
		(*C.uint8_t)(unsafe.Pointer(&pocketpadLogo[0])), C.size_t(len(pocketpadLogo)))
	return nil
}
