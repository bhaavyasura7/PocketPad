package main

import (
	"strings"
	"testing"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
	"github.com/bhaavyasura7/pocketpad/desktop/internal/session"
)

func TestAppStatus(t *testing.T) {
	idle := appStatusFor(session.Snapshot{Battery: protocol.BatteryUnknown}, false)
	if idle.state != appWaiting || idle.title != "Waiting for your phone" || idle.stats != "" {
		t.Fatalf("idle: %+v", idle)
	}

	connected := session.Snapshot{
		Connected: true, Device: "Pixel 8", PhoneRTT: 4100 * time.Microsecond,
		Battery: 80, Charging: true, RateHz: 249.6,
	}
	connected.Lost = 2
	st := appStatusFor(connected, false)
	if st.state != appConnected || st.title != "Connected to Pixel 8" {
		t.Fatalf("connected: %+v", st)
	}
	if st.detail != "4.1 ms latency · Phone battery 80%, charging" {
		t.Fatalf("detail %q", st.detail)
	}
	if st.stats != "250 updates/s · 2 lost · 0 late" {
		t.Fatalf("stats %q", st.stats)
	}

	unknown := appStatusFor(session.Snapshot{Connected: true, Battery: protocol.BatteryUnknown}, false)
	if unknown.title != "Connected" || unknown.detail != "Ready to play." {
		t.Fatalf("unknown device: %+v", unknown)
	}

	perm := appStatusFor(connected, true)
	if perm.state != appPermission || !strings.Contains(perm.detail, "Accessibility") || perm.stats == "" {
		t.Fatalf("permission: %+v", perm)
	}

	for name, want := range map[string]string{"mackeys": "Keyboard and mouse", "machid": "Game controller", "other": "other"} {
		if got := backendTitle(name); got != want {
			t.Errorf("backendTitle(%q) = %q", name, got)
		}
	}
}
