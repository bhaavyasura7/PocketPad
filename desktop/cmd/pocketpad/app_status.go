package main

import (
	"fmt"
	"strings"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/session"
)

// Status states shown by the macOS window (must match cocoa_darwin.m).
const (
	appWaiting    = 0
	appConnected  = 1
	appPermission = 2
)

// appStatus is the window's status area in plain language.
type appStatus struct {
	state  int
	title  string // headline next to the coloured dot
	detail string // one line of explanation
	stats  string // small connection statistics, empty when idle
}

// appStatusFor describes a receiver snapshot for people rather than
// developers. needsPermission is true while keyboard and mouse control is
// waiting for the Accessibility permission.
func appStatusFor(s session.Snapshot, needsPermission bool) appStatus {
	var st appStatus
	switch {
	case needsPermission:
		st.state = appPermission
		st.title = "Permission needed"
		// The window shows an explanation with an Open Settings button.
	case s.Connected:
		st.state = appConnected
		st.title = "Connected"
		if s.Device != "" {
			st.title = "Connected to " + s.Device
		}
		var parts []string
		if s.PhoneRTT > 0 {
			parts = append(parts, fmt.Sprintf("%.1f ms latency", float64(s.PhoneRTT.Microseconds())/1000))
		}
		if s.Battery <= 100 {
			b := fmt.Sprintf("Phone battery %d%%", s.Battery)
			if s.Charging {
				b += ", charging"
			}
			parts = append(parts, b)
		}
		st.detail = strings.Join(parts, " · ")
		if st.detail == "" {
			st.detail = "Ready to play."
		}
	default:
		st.state = appWaiting
		st.title = "Waiting for your phone"
		st.detail = "Open PocketPad on your phone, on the same Wi-Fi network."
	}
	if s.Connected {
		st.stats = fmt.Sprintf("%.0f updates/s · %d lost · %d late", s.RateHz, s.Lost, s.Stale)
	}
	return st
}

// backendTitle names a gamepad backend for the window's subtitle.
func backendTitle(name string) string {
	switch name {
	case "mackeys":
		return "Keyboard and mouse"
	case "machid":
		return "Game controller"
	case "vigem":
		return "Xbox 360 controller"
	case "mock":
		return "Diagnostics only"
	}
	return name
}
