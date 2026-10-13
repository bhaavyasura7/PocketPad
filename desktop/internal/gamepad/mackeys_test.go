package gamepad

import (
	"strings"
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
	for _, want := range []int{macKeySpace, macKeyShift, macKeyTab, macKeyF1} {
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
			if p.Buttons[btn].unset() {
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

func TestGTAProfileVehicleKeysAndClicks(t *testing.T) {
	g := profile(t, "gta")
	if !g.RightStickMouse {
		t.Fatal("gta profile uses mouse look")
	}
	keys := desiredKeys(protocol.ControllerState{Buttons: protocol.ButtonX | protocol.ButtonY}, g)
	if !keys[macKeyF] || !keys[macKeyQ] {
		t.Fatalf("gta keys wrong: %v", keys)
	}
	if keys[macKeyE] {
		t.Fatal("unexpected E")
	}

	// LT aims (right click), RT fires (left click).
	left, right := desiredMouse(protocol.ControllerState{RightTrigger: 1}, g)
	if !left || right {
		t.Fatalf("RT should left click: left=%v right=%v", left, right)
	}
	left, right = desiredMouse(protocol.ControllerState{LeftTrigger: 1}, g)
	if left || !right {
		t.Fatalf("LT should right click: left=%v right=%v", left, right)
	}
}

func TestGTADPadScrollMapping(t *testing.T) {
	g := profile(t, "gta")
	active := activeWheels(protocol.ControllerState{DPad: protocol.DPadUp}, g)
	if active[wheelSrcDPadBase+0] != 1 {
		t.Fatalf("d-pad up should turn the wheel up: %v", active)
	}
	if keys := desiredKeys(protocol.ControllerState{DPad: protocol.DPadUp}, g); len(keys) != 0 {
		t.Fatalf("d-pad up must not press an arrow key in the gta profile: %v", keys)
	}
	// Left and right still send arrow keys.
	keys := desiredKeys(protocol.ControllerState{DPad: protocol.DPadLeft}, g)
	if !keys[macKeyLeft] {
		t.Fatalf("d-pad left should still press the arrow key: %v", keys)
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
	// Full deflection keeps the maximum speed.
	dx, dy := mouseDelta(1, 0, 0.02, 900)
	if !close(dx, 18) || dy != 0 {
		t.Fatalf("full deflection delta %v,%v", dx, dy)
	}
	// Protocol +Y is up, screen +Y is down.
	_, dy = mouseDelta(0, 1, 0.02, 900)
	if !close(dy, -18) {
		t.Fatalf("dy %v", dy)
	}
	// Half deflection is slower than half speed (curve): ~34% of maximum.
	dx, _ = mouseDelta(0.5, 0, 0.02, 900)
	if dx > 7 || dx < 5.5 {
		t.Fatalf("half deflection delta %v, want ~6.1", dx)
	}
	// A resting thumb can never creep the pointer.
	dx, dy = mouseDelta(0.1, 0, 0.02, 900)
	if dx != 0 || dy != 0 {
		t.Fatalf("idle guard delta %v,%v, want 0,0", dx, dy)
	}
	// Just above the guard a light touch is precise: ~1.2 px per 20 ms.
	dx, _ = mouseDelta(0.15, 0, 0.02, 900)
	if dx < 0.5 || dx > 2 {
		t.Fatalf("light deflection delta %v, want ~1.2", dx)
	}
}

func TestParseProfileTemplate(t *testing.T) {
	p, err := ParseProfile(TemplateMappingJSON())
	if err != nil {
		t.Fatalf("the generated template must parse: %v", err)
	}
	if !p.RightStickMouse {
		t.Fatal("template should use mouse look")
	}
	if keys := desiredKeys(protocol.ControllerState{Buttons: protocol.ButtonA}, p); !keys[macKeySpace] {
		t.Fatalf("template A should be space: %v", keys)
	}
	if keys := desiredKeys(protocol.ControllerState{Buttons: protocol.ButtonX}, p); !keys[macKeyE] {
		t.Fatalf("template X should be E: %v", keys)
	}
	if keys := desiredKeys(protocol.ControllerState{DPad: protocol.DPadUp}, p); !keys[macKeyUp] {
		t.Fatalf("template d-pad up should be the arrow key: %v", keys)
	}
	left, _ := desiredMouse(protocol.ControllerState{LeftTrigger: 1}, p)
	if !left {
		t.Fatal("template LT should left click")
	}
}

func TestParseProfileCustom(t *testing.T) {
	p, err := ParseProfile([]byte(`{
		"buttons": {"A": "x", "B": "mouseLeft"},
		"dpad": {"up": "wheelUp"},
		"leftStick": {"up": "i", "down": "k", "left": "j", "right": "l"},
		"rightStick": {"left": "q", "right": "e"},
		"leftTrigger": "wheelDown",
		"rightTrigger": "space",
		"stickThreshold": 0.5,
		"mouseSpeedPxPerSecond": 500
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if keys := desiredKeys(protocol.ControllerState{Buttons: protocol.ButtonA}, p); !keys[macKeyX] {
		t.Fatalf("A should be X: %v", keys)
	}
	left, _ := desiredMouse(protocol.ControllerState{Buttons: protocol.ButtonB}, p)
	if !left {
		t.Fatal("B should left click")
	}
	if p.RightStickMouse {
		t.Fatal("rightStick object should switch to key mode")
	}
	if keys := desiredKeys(protocol.ControllerState{RightX: 1}, p); !keys[macKeyE] {
		t.Fatalf("right stick right should be E: %v", keys)
	}
	if keys := desiredKeys(protocol.ControllerState{RightTrigger: 1}, p); !keys[macKeySpace] {
		t.Fatalf("RT should be space: %v", keys)
	}
	// Custom threshold: 0.4 is below it, 0.6 above.
	if keys := desiredKeys(protocol.ControllerState{LeftX: 0.4}, p); len(keys) != 0 {
		t.Fatalf("below the custom threshold nothing should press: %v", keys)
	}
	if keys := desiredKeys(protocol.ControllerState{LeftX: -0.6}, p); !keys[macKeyJ] {
		t.Fatalf("above the custom threshold the stick should press: %v", keys)
	}
	active := activeWheels(protocol.ControllerState{LeftTrigger: 1}, p)
	if active[wheelSrcLT] != -1 {
		t.Fatalf("LT should turn the wheel down: %v", active)
	}
	if p.MouseSpeedPxPerSecond != 500 {
		t.Fatalf("mouse speed override %v", p.MouseSpeedPxPerSecond)
	}
}

func TestParseProfileErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":      `{"buttons": {"A": "banana"}}`,
		"unknown button":   `{"buttons": {"Z": "a"}}`,
		"unknown dir":      `{"dpad": {"diagonal": "a"}}`,
		"bad rightStick":   `{"rightStick": "banana"}`,
		"invalid json":     `{`,
		"unknown trig key": `{"leftTrigger": "banana"}`,
	}
	for name, doc := range cases {
		if _, err := ParseProfile([]byte(doc)); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	if _, err := ParseProfile([]byte(`{"buttons": {"A": "banana"}}`)); err == nil ||
		!strings.Contains(err.Error(), "banana") {
		t.Fatalf("the error should name the offending value: %v", err)
	}
}
