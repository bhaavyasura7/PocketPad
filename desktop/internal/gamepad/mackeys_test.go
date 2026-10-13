package gamepad

import (
	"testing"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func profile(t *testing.T, name string) MacKeyProfile {
	t.Helper()
	p, ok := macKeyProfiles[name]
	if !ok {
		t.Fatalf("profile %q missing", name)
	}
	return p
}

func TestDesiredKeysButtons(t *testing.T) {
	g := profile(t, "general")
	s := protocol.ControllerState{
		Buttons: protocol.ButtonA | protocol.ButtonB | protocol.ButtonLB | protocol.ButtonGuide,
	}
	keys := desiredKeys(s, g)
	for _, want := range []int{macKeySpace, macKeyLeftShift, macKeyTab, macKeyF1} {
		if !keys[want] {
			t.Fatalf("key %d missing from %v", want, keys)
		}
	}
	if len(keys) != 4 {
		t.Fatalf("unexpected keys: %v", keys)
	}
}

func TestEveryProfileMapsEveryButton(t *testing.T) {
	for _, name := range MacKeysProfileNames() {
		p := profile(t, name)
		for i := 0; i < 11; i++ {
			btn := protocol.Buttons(1 << i)
			if len(desiredKeys(protocol.ControllerState{Buttons: btn}, p)) == 0 {
				t.Fatalf("profile %s: button bit %d has no mapping", name, i)
			}
		}
	}
}

func TestDesiredKeysStickAndDPad(t *testing.T) {
	g := profile(t, "general")
	s := protocol.ControllerState{LeftX: 0.8, LeftY: -0.9}
	keys := desiredKeys(s, g)
	if !keys[macKeyD] || !keys[macKeyS] || keys[macKeyA] || keys[macKeyW] {
		t.Fatalf("stick keys wrong: %v", keys)
	}

	s = protocol.ControllerState{LeftX: 0.2, LeftY: 0.2} // below the threshold
	if len(desiredKeys(s, g)) != 0 {
		t.Fatalf("resting stick should hold no keys")
	}

	s = protocol.ControllerState{DPad: protocol.DPadUpLeft}
	keys = desiredKeys(s, g)
	if !keys[macKeyUp] || !keys[macKeyLeft] {
		t.Fatalf("d-pad keys wrong: %v", keys)
	}
}

func TestRacingProfileTriggersAreThrottleAndBrake(t *testing.T) {
	r := profile(t, "racing")
	if r.RightStickMouse {
		t.Fatal("racing profile should not move the pointer")
	}
	keys := desiredKeys(protocol.ControllerState{RightTrigger: 1}, r)
	if !keys[macKeyW] || len(keys) != 1 {
		t.Fatalf("right trigger should hold W only: %v", keys)
	}
	keys = desiredKeys(protocol.ControllerState{LeftTrigger: 1}, r)
	if !keys[macKeyS] || len(keys) != 1 {
		t.Fatalf("left trigger should hold S only: %v", keys)
	}
	// The right stick is unmapped in racing.
	if len(desiredKeys(protocol.ControllerState{RightX: 1, RightY: 1}, r)) != 0 {
		t.Fatal("right stick should be silent in racing")
	}
}

func TestGTAProfileVehicleKeys(t *testing.T) {
	g := profile(t, "gta")
	if !g.RightStickMouse {
		t.Fatal("gta profile uses mouse look")
	}
	keys := desiredKeys(protocol.ControllerState{Buttons: protocol.ButtonX | protocol.ButtonY}, g)
	if !keys[macKeyF] || !keys[macKeyQ] {
		t.Fatalf("gta keys wrong: %v", keys)
	}
	if keys[macKeyE] { // LB only
		t.Fatal("unexpected E")
	}
}

func TestSetMacKeysProfileValidation(t *testing.T) {
	if err := SetMacKeysProfile("racing"); err != nil {
		t.Fatal(err)
	}
	if currentProfile().Name != "racing" {
		t.Fatal("profile not selected")
	}
	if err := SetMacKeysProfile("nope"); err == nil {
		t.Fatal("unknown profile accepted")
	}
	// Restore so other tests start from the default.
	if err := SetMacKeysProfile("general"); err != nil {
		t.Fatal(err)
	}
	names := MacKeysProfileNames()
	if len(names) != 3 || names[0] != "general" {
		t.Fatalf("profile list %v", names)
	}
}

func TestMouseDelta(t *testing.T) {
	close := func(a, b float64) bool {
		d := a - b
		return d < 0.01 && d > -0.01
	}
	dx, dy := mouseDelta(1, 0, 0.02, 900)
	if !close(dx, 18) || dy != 0 {
		t.Fatalf("delta %v,%v", dx, dy)
	}
	// Protocol +Y is up, screen +Y is down.
	_, dy = mouseDelta(0, 1, 0.02, 900)
	if !close(dy, -18) {
		t.Fatalf("dy %v", dy)
	}
}

func TestTriggerClicks(t *testing.T) {
	for name, want := range map[string][2]bool{
		"general": {false, true}, // LT left click, RT right click
		"gta":     {true, false}, // LT aims (right click), RT fires (left click)
	} {
		lt, rt := macKeyProfiles[name].triggerClicks()
		if lt != want[0] || rt != want[1] {
			t.Errorf("%s: LT right=%v RT right=%v, want %v", name, lt, rt, want)
		}
	}
	if p := macKeyProfiles["racing"]; p.LTKey == keyNone || p.RTKey == keyNone {
		t.Error("racing triggers must be keys (brake/throttle), not clicks")
	}
}
