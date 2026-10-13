package gamepad

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// The "mackeys" backend turns the controller into a keyboard and mouse for
// macOS. It exists because macOS 26 refuses to create virtual HID gamepads
// without an Apple-granted entitlement, while synthetic keyboard/mouse
// events only need the user's Accessibility approval.
//
// Profiles change the mapping per game family; `serve -profile <name>`
// selects one. See docs/GETTING_STARTED.md for the tables.

// macOS virtual key codes (ANSI layout). These are stable ABI values defined
// by Carbon's HIToolbox Events.h.
const (
	macKeyA         = 0x00
	macKeyS         = 0x01
	macKeyD         = 0x02
	macKeyF         = 0x03
	macKeyH         = 0x04
	macKeyC         = 0x08
	macKeyV         = 0x09
	macKeyQ         = 0x0C
	macKeyW         = 0x0D
	macKeyE         = 0x0E
	macKeyR         = 0x0F
	macKeyM         = 0x2E
	macKeyTab       = 0x30
	macKeySpace     = 0x31
	macKeyReturn    = 0x24
	macKeyEscape    = 0x35
	macKeyLeftShift = 0x38
	macKeyLeft      = 0x7B
	macKeyRight     = 0x7C
	macKeyDown      = 0x7D
	macKeyUp        = 0x7E
	macKeyF1        = 0x7A
)

// keyNone marks an action that a profile leaves unmapped.
const keyNone = -1

// MacKeyProfile describes one keyboard/mouse mapping.
type MacKeyProfile struct {
	Name string

	ButtonKeys map[protocol.Buttons]int

	DPadUp, DPadDown, DPadLeft, DPadRight int

	// The left stick acts as four keys around a threshold.
	StickUp, StickDown, StickLeft, StickRight int
	StickThreshold                            float32

	// Right stick: pointer movement, or four keys when RightStickMouse is false.
	RightStickMouse               bool
	RSUp, RSDown, RSLeft, RSRight int

	// When set, d-pad up/down turn the mouse wheel (map zoom, weapon
	// switching) instead of pressing DPadUp/Down.
	DPadScroll bool

	// Triggers: a key when >= 0, otherwise a mouse button: LT left click and
	// RT right click, or the reverse when SwapTriggerClicks is set.
	LTKey, RTKey      int
	SwapTriggerClicks bool
}

// triggerClicks returns which mouse button each trigger presses when it is
// not mapped to a key: false = left click, true = right click.
func (p MacKeyProfile) triggerClicks() (ltRight, rtRight bool) {
	if p.SwapTriggerClicks {
		return true, false
	}
	return false, true
}

// stickKeyThresholdDefault keeps resting thumbs from walking the character.
const stickKeyThresholdDefault = 0.35

var (
	profileMu       sync.RWMutex
	selectedProfile = "general"
	macKeyProfiles  = map[string]MacKeyProfile{}
	macProfileOrder = []string{"general", "racing", "gta"}
)

func init() {
	general := MacKeyProfile{
		Name: "general",
		ButtonKeys: map[protocol.Buttons]int{
			protocol.ButtonA: macKeySpace, protocol.ButtonB: macKeyLeftShift,
			protocol.ButtonX: macKeyE, protocol.ButtonY: macKeyQ,
			protocol.ButtonLB: macKeyTab, protocol.ButtonRB: macKeyEscape,
			protocol.ButtonBack: macKeyM, protocol.ButtonStart: macKeyReturn,
			protocol.ButtonGuide: macKeyF1, protocol.ButtonLS: macKeyC, protocol.ButtonRS: macKeyR,
		},
		DPadUp: macKeyUp, DPadDown: macKeyDown, DPadLeft: macKeyLeft, DPadRight: macKeyRight,
		StickUp: macKeyW, StickDown: macKeyS, StickLeft: macKeyA, StickRight: macKeyD,
		StickThreshold:  stickKeyThresholdDefault,
		RightStickMouse: true,
		RSUp:            keyNone, RSDown: keyNone, RSLeft: keyNone, RSRight: keyNone,
		LTKey: keyNone, RTKey: keyNone, // mouse buttons
	}

	racing := MacKeyProfile{
		Name: "racing",
		ButtonKeys: map[protocol.Buttons]int{
			protocol.ButtonA: macKeySpace, protocol.ButtonB: macKeyLeftShift,
			protocol.ButtonX: macKeyC, protocol.ButtonY: macKeyR,
			protocol.ButtonLB: macKeyQ, protocol.ButtonRB: macKeyE,
			protocol.ButtonBack: macKeyM, protocol.ButtonStart: macKeyReturn,
			protocol.ButtonGuide: macKeyEscape, protocol.ButtonLS: macKeyH, protocol.ButtonRS: macKeyV,
		},
		DPadUp: macKeyUp, DPadDown: macKeyDown, DPadLeft: macKeyLeft, DPadRight: macKeyRight,
		StickUp: macKeyW, StickDown: macKeyS, StickLeft: macKeyA, StickRight: macKeyD,
		StickThreshold:  stickKeyThresholdDefault,
		RightStickMouse: false, // unused in racing; no pointer surprises
		RSUp:            keyNone, RSDown: keyNone, RSLeft: keyNone, RSRight: keyNone,
		// Analog triggers as throttle/brake, the racing default.
		LTKey: macKeyS, RTKey: macKeyW,
	}

	gta := MacKeyProfile{
		Name: "gta",
		ButtonKeys: map[protocol.Buttons]int{
			protocol.ButtonA: macKeySpace, protocol.ButtonB: macKeyLeftShift,
			protocol.ButtonX: macKeyF, protocol.ButtonY: macKeyQ,
			protocol.ButtonLB: macKeyE, protocol.ButtonRB: macKeyV,
			protocol.ButtonBack: macKeyM, protocol.ButtonStart: macKeyEscape,
			protocol.ButtonGuide: macKeyTab, protocol.ButtonLS: macKeyC, protocol.ButtonRS: macKeyR,
		},
		DPadUp: macKeyUp, DPadDown: macKeyDown, DPadLeft: macKeyLeft, DPadRight: macKeyRight,
		StickUp: macKeyW, StickDown: macKeyS, StickLeft: macKeyA, StickRight: macKeyD,
		StickThreshold:  stickKeyThresholdDefault,
		RightStickMouse: true, // mouse look; LT/RT aim and fire
		RSUp:            keyNone, RSDown: keyNone, RSLeft: keyNone, RSRight: keyNone,
		LTKey: keyNone, RTKey: keyNone,
		// Like a controller: LT aims (right click), RT fires (left click).
		SwapTriggerClicks: true,
		// D-pad up/down zoom the map and switch weapons.
		DPadScroll: true,
	}

	for _, p := range []MacKeyProfile{general, racing, gta} {
		macKeyProfiles[p.Name] = p
	}
}

// MacKeysProfileNames lists the built-in profiles in display order.
func MacKeysProfileNames() []string {
	return append([]string(nil), macProfileOrder...)
}

// SetMacKeysProfile selects the profile for the mackeys backend. Call it
// before opening the backend.
func SetMacKeysProfile(name string) error {
	profileMu.Lock()
	defer profileMu.Unlock()
	if _, ok := macKeyProfiles[name]; !ok {
		return fmt.Errorf("unknown keyboard/mouse profile %q (available: %s)",
			name, strings.Join(macProfileOrder, ", "))
	}
	selectedProfile = name
	return nil
}

func currentProfile() MacKeyProfile {
	profileMu.RLock()
	defer profileMu.RUnlock()
	return macKeyProfiles[selectedProfile]
}

// desiredKeys returns the virtual key codes that should be held for s under
// the profile. It is pure (no events) so the mapping is unit tested.
func desiredKeys(s protocol.ControllerState, p MacKeyProfile) map[int]bool {
	keys := make(map[int]bool, 10)
	add := func(key int) {
		if key != keyNone {
			keys[key] = true
		}
	}
	for _, m := range macButtonMap(p) {
		if s.Buttons&m.from != 0 {
			add(m.key)
		}
	}
	if s.DPad.Up() && !p.DPadScroll {
		add(p.DPadUp)
	}
	if s.DPad.Down() && !p.DPadScroll {
		add(p.DPadDown)
	}
	if s.DPad.Left() {
		add(p.DPadLeft)
	}
	if s.DPad.Right() {
		add(p.DPadRight)
	}
	thr := p.StickThreshold
	if thr <= 0 {
		thr = stickKeyThresholdDefault
	}
	if s.LeftY >= thr {
		add(p.StickUp)
	}
	if s.LeftY <= -thr {
		add(p.StickDown)
	}
	if s.LeftX <= -thr {
		add(p.StickLeft)
	}
	if s.LeftX >= thr {
		add(p.StickRight)
	}
	if !p.RightStickMouse {
		if s.RightY >= thr {
			add(p.RSUp)
		}
		if s.RightY <= -thr {
			add(p.RSDown)
		}
		if s.RightX <= -thr {
			add(p.RSLeft)
		}
		if s.RightX >= thr {
			add(p.RSRight)
		}
	}
	if p.LTKey != keyNone && s.LeftTrigger > 0.5 {
		add(p.LTKey)
	}
	if p.RTKey != keyNone && s.RightTrigger > 0.5 {
		add(p.RTKey)
	}
	return keys
}

type buttonKey struct {
	from protocol.Buttons
	key  int
}

func macButtonMap(p MacKeyProfile) []buttonKey {
	out := make([]buttonKey, 0, len(p.ButtonKeys))
	for btn, key := range p.ButtonKeys {
		out = append(out, buttonKey{btn, key})
	}
	return out
}

// mouseDelta converts the right stick into pointer movement in pixels for one
// update tick. A curved response keeps small deflections slow for precise
// aiming and map pointing while a full push still moves fast (the curve is
// 0.35 linear + 0.65 quadratic in the stick magnitude). Protocol +Y is up;
// screen +Y is down, so the Y axis flips.
func mouseDelta(rx, ry, dtSeconds, speedPxPerSecond float32) (float64, float64) {
	mag := math.Sqrt(float64(rx)*float64(rx) + float64(ry)*float64(ry))
	if mag == 0 {
		return 0, 0
	}
	if mag > 1 {
		mag = 1
	}
	shaped := mag * (0.35 + 0.65*mag)
	scale := float64(speedPxPerSecond) * float64(dtSeconds) * shaped / mag
	return float64(rx) * scale, -float64(ry) * scale
}
