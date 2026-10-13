package gamepad

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// The "mackeys" backend turns the controller into a keyboard and mouse for
// macOS. It exists because macOS 26 refuses to create virtual HID gamepads
// without an Apple-granted entitlement, while synthetic keyboard/mouse
// events only need the user's Accessibility approval.
//
// Three built-in profiles (general, racing, gta) cover common game families.
// Beyond those, `-profile custom` (or a path to a .json file) loads a fully
// custom mapping where any control can act as any key, mouse click or wheel
// turn; the file is watched and reloaded live while playing.
// See docs/GETTING_STARTED.md for the tables and the mapping format.

// macOS virtual key codes (ANSI layout). These are stable ABI values defined
// by Carbon's HIToolbox Events.h.
const (
	macKeyA         = 0x00
	macKeyS         = 0x01
	macKeyD         = 0x02
	macKeyF         = 0x03
	macKeyH         = 0x04
	macKeyG         = 0x05
	macKeyZ         = 0x06
	macKeyX         = 0x07
	macKeyC         = 0x08
	macKeyV         = 0x09
	macKeyB         = 0x0B
	macKeyQ         = 0x0C
	macKeyW         = 0x0D
	macKeyE         = 0x0E
	macKeyR         = 0x0F
	macKeyY         = 0x10
	macKeyT         = 0x11
	macKey1         = 0x12
	macKey2         = 0x13
	macKey3         = 0x14
	macKey4         = 0x15
	macKey6         = 0x16
	macKey5         = 0x17
	macKeyEqual     = 0x18
	macKey9         = 0x19
	macKey7         = 0x1A
	macKeyMinus     = 0x1B
	macKey8         = 0x1C
	macKey0         = 0x1D
	macKeyRBracket  = 0x1E
	macKeyO         = 0x1F
	macKeyU         = 0x20
	macKeyLBracket  = 0x21
	macKeyI         = 0x22
	macKeyP         = 0x23
	macKeyReturn    = 0x24
	macKeyL         = 0x25
	macKeyJ         = 0x26
	macKeyQuote     = 0x27
	macKeyK         = 0x28
	macKeySemicolon = 0x29
	macKeyBackslash = 0x2A
	macKeyComma     = 0x2B
	macKeySlash     = 0x2C
	macKeyN         = 0x2D
	macKeyM         = 0x2E
	macKeyPeriod    = 0x2F
	macKeyTab       = 0x30
	macKeySpace     = 0x31
	macKeyGrave     = 0x32
	macKeyDelete    = 0x33
	macKeyEscape    = 0x35
	macKeyCommand   = 0x37
	macKeyShift     = 0x38
	macKeyCapsLock  = 0x39
	macKeyOption    = 0x3A
	macKeyControl   = 0x3B
	macKeyRShift    = 0x3C
	macKeyROption   = 0x3D
	macKeyRControl  = 0x3E
	macKeyF17       = 0x40
	macKeyKPDecimal = 0x41
	macKeyKPMult    = 0x43
	macKeyKPPlus    = 0x45
	macKeyKPClear   = 0x47
	macKeyKPDivide  = 0x4B
	macKeyKPEnter   = 0x4C
	macKeyKPMinus   = 0x4E
	macKeyKPEquals  = 0x51
	macKeyKP0       = 0x52
	macKeyKP1       = 0x53
	macKeyKP2       = 0x54
	macKeyKP3       = 0x55
	macKeyKP4       = 0x56
	macKeyKP5       = 0x57
	macKeyKP6       = 0x58
	macKeyKP7       = 0x59
	macKeyF18       = 0x4F
	macKeyF19       = 0x50
	macKeyF20       = 0x5A
	macKeyF5        = 0x60
	macKeyF6        = 0x61
	macKeyF7        = 0x62
	macKeyF3        = 0x63
	macKeyF8        = 0x64
	macKeyF9        = 0x65
	macKeyF11       = 0x67
	macKeyF13       = 0x69
	macKeyF16       = 0x6A
	macKeyF14       = 0x6B
	macKeyF10       = 0x6D
	macKeyF12       = 0x6F
	macKeyF15       = 0x71
	macKeyHelp      = 0x72
	macKeyHome      = 0x73
	macKeyPageUp    = 0x74
	macKeyFwdDelete = 0x75
	macKeyF4        = 0x76
	macKeyEnd       = 0x77
	macKeyF2        = 0x78
	macKeyPageDown  = 0x79
	macKeyF1        = 0x7A
	macKeyLeft      = 0x7B
	macKeyRight     = 0x7C
	macKeyDown      = 0x7D
	macKeyUp        = 0x7E
	macKeyKP8       = 0x5B
	macKeyKP9       = 0x5C
	macKeyKPStar    = 0x43 // alias of multiply for readability
)

// keyNone marks an action that a profile leaves unmapped.
const keyNone = -1

// ActionKind is what a control does: press a key, click a mouse button or
// turn the wheel.
type ActionKind int

const (
	ActionNone ActionKind = iota
	ActionKey
	ActionMouseLeft
	ActionMouseRight
	ActionWheelUp
	ActionWheelDown
)

// Action is one mapping target. The zero value does nothing.
type Action struct {
	Kind ActionKind
	Key  int // valid when Kind == ActionKey
}

func keyAction(code int) Action       { return Action{Kind: ActionKey, Key: code} }
func mouseAction(k ActionKind) Action { return Action{Kind: k} }
func (a Action) isKey() bool          { return a.Kind == ActionKey }
func (a Action) isMouseClick() bool   { return a.Kind == ActionMouseLeft || a.Kind == ActionMouseRight }
func (a Action) isWheel() bool        { return a.Kind == ActionWheelUp || a.Kind == ActionWheelDown }
func (a Action) unset() bool          { return a.Kind == ActionNone }

// MacKeyProfile describes one keyboard/mouse mapping.
type MacKeyProfile struct {
	Name string

	// Buttons maps controller buttons; unmapped buttons do nothing.
	Buttons map[protocol.Buttons]Action

	DPadUp, DPadDown, DPadLeft, DPadRight Action

	// The left stick acts as four keys around a threshold.
	StickUp, StickDown, StickLeft, StickRight Action
	StickThreshold                            float32

	// Right stick: pointer movement, or four actions when RightStickMouse is
	// false.
	RightStickMouse               bool
	RSUp, RSDown, RSLeft, RSRight Action

	// Triggers.
	LT, RT Action

	// MouseSpeedPxPerSecond overrides the pointer speed at full deflection
	// when positive.
	MouseSpeedPxPerSecond float64
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
	macKeyProfiles["general"] = MacKeyProfile{
		Name: "general",
		Buttons: map[protocol.Buttons]Action{
			protocol.ButtonA: keyAction(macKeySpace), protocol.ButtonB: keyAction(macKeyShift),
			protocol.ButtonX: keyAction(macKeyE), protocol.ButtonY: keyAction(macKeyQ),
			protocol.ButtonLB: keyAction(macKeyTab), protocol.ButtonRB: keyAction(macKeyEscape),
			protocol.ButtonBack: keyAction(macKeyM), protocol.ButtonStart: keyAction(macKeyReturn),
			protocol.ButtonGuide: keyAction(macKeyF1), protocol.ButtonLS: keyAction(macKeyC),
			protocol.ButtonRS: keyAction(macKeyR),
		},
		DPadUp: keyAction(macKeyUp), DPadDown: keyAction(macKeyDown),
		DPadLeft: keyAction(macKeyLeft), DPadRight: keyAction(macKeyRight),
		StickUp: keyAction(macKeyW), StickDown: keyAction(macKeyS),
		StickLeft: keyAction(macKeyA), StickRight: keyAction(macKeyD),
		StickThreshold:  stickKeyThresholdDefault,
		RightStickMouse: true,
		LT:              mouseAction(ActionMouseLeft),
		RT:              mouseAction(ActionMouseRight),
	}

	macKeyProfiles["racing"] = MacKeyProfile{
		Name: "racing",
		Buttons: map[protocol.Buttons]Action{
			protocol.ButtonA: keyAction(macKeySpace), protocol.ButtonB: keyAction(macKeyShift),
			protocol.ButtonX: keyAction(macKeyC), protocol.ButtonY: keyAction(macKeyR),
			protocol.ButtonLB: keyAction(macKeyQ), protocol.ButtonRB: keyAction(macKeyE),
			protocol.ButtonBack: keyAction(macKeyM), protocol.ButtonStart: keyAction(macKeyReturn),
			protocol.ButtonGuide: keyAction(macKeyEscape), protocol.ButtonLS: keyAction(macKeyH),
			protocol.ButtonRS: keyAction(macKeyV),
		},
		DPadUp: keyAction(macKeyUp), DPadDown: keyAction(macKeyDown),
		DPadLeft: keyAction(macKeyLeft), DPadRight: keyAction(macKeyRight),
		StickUp: keyAction(macKeyW), StickDown: keyAction(macKeyS),
		StickLeft: keyAction(macKeyA), StickRight: keyAction(macKeyD),
		StickThreshold: stickKeyThresholdDefault,
		// Analog triggers as throttle and brake, the racing default.
		LT: keyAction(macKeyS), RT: keyAction(macKeyW),
	}

	macKeyProfiles["gta"] = MacKeyProfile{
		Name: "gta",
		Buttons: map[protocol.Buttons]Action{
			protocol.ButtonA: keyAction(macKeySpace), protocol.ButtonB: keyAction(macKeyShift),
			protocol.ButtonX: keyAction(macKeyF), protocol.ButtonY: keyAction(macKeyQ),
			protocol.ButtonLB: keyAction(macKeyE), protocol.ButtonRB: keyAction(macKeyV),
			protocol.ButtonBack: keyAction(macKeyM), protocol.ButtonStart: keyAction(macKeyEscape),
			protocol.ButtonGuide: keyAction(macKeyTab), protocol.ButtonLS: keyAction(macKeyC),
			protocol.ButtonRS: keyAction(macKeyR),
		},
		// D-pad up/down zoom the map and switch weapons via the wheel.
		DPadUp: mouseAction(ActionWheelUp), DPadDown: mouseAction(ActionWheelDown),
		DPadLeft: keyAction(macKeyLeft), DPadRight: keyAction(macKeyRight),
		StickUp: keyAction(macKeyW), StickDown: keyAction(macKeyS),
		StickLeft: keyAction(macKeyA), StickRight: keyAction(macKeyD),
		StickThreshold:  stickKeyThresholdDefault,
		RightStickMouse: true, // mouse look
		// Like a controller: LT aims (right click), RT fires (left click).
		LT: mouseAction(ActionMouseRight), RT: mouseAction(ActionMouseLeft),
	}
}

// MacKeysProfileNames lists the built-in profiles in display order.
func MacKeysProfileNames() []string {
	return append([]string(nil), macProfileOrder...)
}

// SetMacKeysProfile selects a built-in profile. Call it before opening the
// backend.
func SetMacKeysProfile(name string) error {
	profileMu.Lock()
	defer profileMu.Unlock()
	if _, ok := macKeyProfiles[name]; !ok {
		return fmt.Errorf("unknown keyboard/mouse profile %q (built-in: %s; or use \"custom\" for a mapping.json)",
			name, strings.Join(macProfileOrder, ", "))
	}
	selectedProfile = name
	return nil
}

// SetCustomProfile registers a custom mapping and selects it immediately.
// Used at startup and by the live-reload watcher.
func SetCustomProfile(p MacKeyProfile) {
	profileMu.Lock()
	defer profileMu.Unlock()
	p.Name = "custom"
	macKeyProfiles["custom"] = p
	selectedProfile = "custom"
}

func currentProfile() MacKeyProfile {
	profileMu.RLock()
	defer profileMu.RUnlock()
	return macKeyProfiles[selectedProfile]
}

// SelectedProfileName is the active profile ("general", "racing", "gta" or
// "custom").
func SelectedProfileName() string {
	profileMu.RLock()
	defer profileMu.RUnlock()
	return selectedProfile
}

func stickThreshold(p MacKeyProfile) float32 {
	if p.StickThreshold <= 0 {
		return stickKeyThresholdDefault
	}
	return p.StickThreshold
}

// desiredKeys returns the virtual key codes that should be held for s under
// the profile. It is pure (no events) so the mapping is unit tested.
func desiredKeys(s protocol.ControllerState, p MacKeyProfile) map[int]bool {
	keys := make(map[int]bool, 10)
	add := func(a Action) {
		if a.isKey() {
			keys[a.Key] = true
		}
	}
	for btn, a := range p.Buttons {
		if s.Buttons&btn != 0 {
			add(a)
		}
	}
	if s.DPad.Up() {
		add(p.DPadUp)
	}
	if s.DPad.Down() {
		add(p.DPadDown)
	}
	if s.DPad.Left() {
		add(p.DPadLeft)
	}
	if s.DPad.Right() {
		add(p.DPadRight)
	}
	thr := stickThreshold(p)
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
	if s.LeftTrigger > 0.5 {
		add(p.LT)
	}
	if s.RightTrigger > 0.5 {
		add(p.RT)
	}
	return keys
}

// desiredMouse reports whether a left and/or right click should be held for s.
func desiredMouse(s protocol.ControllerState, p MacKeyProfile) (left, right bool) {
	consider := func(a Action, pressed bool) {
		if !pressed {
			return
		}
		switch a.Kind {
		case ActionMouseLeft:
			left = true
		case ActionMouseRight:
			right = true
		}
	}
	for btn, a := range p.Buttons {
		consider(a, s.Buttons&btn != 0)
	}
	consider(p.DPadUp, s.DPad.Up())
	consider(p.DPadDown, s.DPad.Down())
	consider(p.DPadLeft, s.DPad.Left())
	consider(p.DPadRight, s.DPad.Right())
	thr := stickThreshold(p)
	consider(p.StickUp, s.LeftY >= thr)
	consider(p.StickDown, s.LeftY <= -thr)
	consider(p.StickLeft, s.LeftX <= -thr)
	consider(p.StickRight, s.LeftX >= thr)
	if !p.RightStickMouse {
		consider(p.RSUp, s.RightY >= thr)
		consider(p.RSDown, s.RightY <= -thr)
		consider(p.RSLeft, s.RightX <= -thr)
		consider(p.RSRight, s.RightX >= thr)
	}
	consider(p.LT, s.LeftTrigger > 0.5)
	consider(p.RT, s.RightTrigger > 0.5)
	return left, right
}

// Fixed wheel-source ids, used to fire one wheel notch per press edge.
const (
	wheelSrcButtonBase = 100
	wheelSrcDPadBase   = 200
	wheelSrcStickBase  = 300
	wheelSrcRSBase     = 400
	wheelSrcLT         = 500
	wheelSrcRT         = 501
)

var buttonOrder = [...]protocol.Buttons{
	protocol.ButtonA, protocol.ButtonB, protocol.ButtonX, protocol.ButtonY,
	protocol.ButtonLB, protocol.ButtonRB, protocol.ButtonBack, protocol.ButtonStart,
	protocol.ButtonLS, protocol.ButtonRS, protocol.ButtonGuide,
}

// activeWheels returns sourceID -> direction (+1 up, -1 down) for every
// control whose action is a wheel turn and that is currently pressed. The
// caller fires a notch when a source appears here for the first time.
func activeWheels(s protocol.ControllerState, p MacKeyProfile) map[int]int {
	out := make(map[int]int, 2)
	add := func(src int, a Action, pressed bool) {
		if !pressed {
			return
		}
		switch a.Kind {
		case ActionWheelUp:
			out[src] = 1
		case ActionWheelDown:
			out[src] = -1
		}
	}
	for i, btn := range buttonOrder {
		add(wheelSrcButtonBase+i, p.Buttons[btn], s.Buttons&btn != 0)
	}
	add(wheelSrcDPadBase+0, p.DPadUp, s.DPad.Up())
	add(wheelSrcDPadBase+1, p.DPadDown, s.DPad.Down())
	add(wheelSrcDPadBase+2, p.DPadLeft, s.DPad.Left())
	add(wheelSrcDPadBase+3, p.DPadRight, s.DPad.Right())
	thr := stickThreshold(p)
	add(wheelSrcStickBase+0, p.StickUp, s.LeftY >= thr)
	add(wheelSrcStickBase+1, p.StickDown, s.LeftY <= -thr)
	add(wheelSrcStickBase+2, p.StickLeft, s.LeftX <= -thr)
	add(wheelSrcStickBase+3, p.StickRight, s.LeftX >= thr)
	if !p.RightStickMouse {
		add(wheelSrcRSBase+0, p.RSUp, s.RightY >= thr)
		add(wheelSrcRSBase+1, p.RSDown, s.RightY <= -thr)
		add(wheelSrcRSBase+2, p.RSLeft, s.RightX <= -thr)
		add(wheelSrcRSBase+3, p.RSRight, s.RightX >= thr)
	}
	add(wheelSrcLT, p.LT, s.LeftTrigger > 0.5)
	add(wheelSrcRT, p.RT, s.RightTrigger > 0.5)
	return out
}

// mouseIdleGuard suppresses tiny stick values so a resting thumb can never
// creep the pointer; the mouse curve then keeps light touches precise.
const mouseIdleGuard = 0.12

// mouseDelta converts the right stick into pointer movement in pixels for one
// update tick. A curved response keeps small deflections slow for precise
// aiming and map pointing while a full push still moves fast (the curve is
// 0.35 linear + 0.65 quadratic in the stick magnitude). Protocol +Y is up;
// screen +Y is down, so the Y axis flips.
func mouseDelta(rx, ry, dtSeconds, speedPxPerSecond float32) (float64, float64) {
	mag := math.Sqrt(float64(rx)*float64(rx) + float64(ry)*float64(ry))
	if mag < mouseIdleGuard {
		return 0, 0
	}
	if mag > 1 {
		mag = 1
	}
	shaped := mag * (0.35 + 0.65*mag)
	scale := float64(speedPxPerSecond) * float64(dtSeconds) * shaped / mag
	return float64(rx) * scale, -float64(ry) * scale
}

// ---------------------------------------------------------------------------
// Custom mapping files (mapping.json)
// ---------------------------------------------------------------------------

// keyNames maps the names accepted in mapping.json to macOS key codes.
// "pocketpad keys" prints this list.
var keyNames = map[string]int{
	"a": macKeyA, "b": macKeyB, "c": macKeyC, "d": macKeyD, "e": macKeyE,
	"f": macKeyF, "g": macKeyG, "h": macKeyH, "i": macKeyI, "j": macKeyJ,
	"k": macKeyK, "l": macKeyL, "m": macKeyM, "n": macKeyN, "o": macKeyO,
	"p": macKeyP, "q": macKeyQ, "r": macKeyR, "s": macKeyS, "t": macKeyT,
	"u": macKeyU, "v": macKeyV, "w": macKeyW, "x": macKeyX, "y": macKeyY,
	"z": macKeyZ,
	"0": macKey0, "1": macKey1, "2": macKey2, "3": macKey3, "4": macKey4,
	"5": macKey5, "6": macKey6, "7": macKey7, "8": macKey8, "9": macKey9,
	"space": macKeySpace, "enter": macKeyReturn, "return": macKeyReturn,
	"esc": macKeyEscape, "escape": macKeyEscape, "tab": macKeyTab,
	"shift": macKeyShift, "lshift": macKeyShift, "leftshift": macKeyShift,
	"rshift": macKeyRShift, "rightshift": macKeyRShift,
	"ctrl": macKeyControl, "control": macKeyControl, "lctrl": macKeyControl,
	"rctrl": macKeyRControl, "rightcontrol": macKeyRControl,
	"alt": macKeyOption, "option": macKeyOption, "lalt": macKeyOption, "loption": macKeyOption,
	"ralt": macKeyROption, "roption": macKeyROption,
	"cmd": macKeyCommand, "command": macKeyCommand,
	"capslock": macKeyCapsLock, "caps": macKeyCapsLock,
	"delete": macKeyDelete, "backspace": macKeyDelete,
	"forwarddelete": macKeyFwdDelete, "del": macKeyFwdDelete,
	"up": macKeyUp, "down": macKeyDown, "left": macKeyLeft, "right": macKeyRight,
	"home": macKeyHome, "end": macKeyEnd, "pageup": macKeyPageUp, "pagedown": macKeyPageDown,
	"help":  macKeyHelp,
	"minus": macKeyMinus, "equal": macKeyEqual, "comma": macKeyComma,
	"period": macKeyPeriod, "slash": macKeySlash, "backslash": macKeyBackslash,
	"semicolon": macKeySemicolon, "quote": macKeyQuote, "grave": macKeyGrave,
	"lbracket": macKeyLBracket, "rbracket": macKeyRBracket,
	"f1": macKeyF1, "f2": macKeyF2, "f3": macKeyF3, "f4": macKeyF4,
	"f5": macKeyF5, "f6": macKeyF6, "f7": macKeyF7, "f8": macKeyF8,
	"f9": macKeyF9, "f10": macKeyF10, "f11": macKeyF11, "f12": macKeyF12,
	"f13": macKeyF13, "f14": macKeyF14, "f15": macKeyF15, "f16": macKeyF16,
	"f17": macKeyF17, "f18": macKeyF18, "f19": macKeyF19, "f20": macKeyF20,
	"kp0": macKeyKP0, "kp1": macKeyKP1, "kp2": macKeyKP2, "kp3": macKeyKP3,
	"kp4": macKeyKP4, "kp5": macKeyKP5, "kp6": macKeyKP6, "kp7": macKeyKP7,
	"kp8": macKeyKP8, "kp9": macKeyKP9,
	"kpdecimal": macKeyKPDecimal, "kpenter": macKeyKPEnter, "kpplus": macKeyKPPlus,
	"kpminus": macKeyKPMinus, "kpstar": macKeyKPMult, "kpmultiply": macKeyKPMult,
	"kpdivide": macKeyKPDivide, "kpequals": macKeyKPEquals, "kpclear": macKeyKPClear,
}

// canonicalKeyOrder is the preferred spelling for each code (used when
// generating the template).
var canonicalKeyOrder = []string{
	"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
	"space", "enter", "esc", "tab", "shift", "ctrl", "alt", "cmd",
	"up", "down", "left", "right", "home", "end", "pageup", "pagedown",
	"delete", "forwarddelete", "help", "capslock",
	"minus", "equal", "comma", "period", "slash", "backslash", "semicolon", "quote", "grave", "lbracket", "rbracket",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12", "f13", "f14", "f15", "f16", "f17", "f18", "f19", "f20",
	"kp0", "kp1", "kp2", "kp3", "kp4", "kp5", "kp6", "kp7", "kp8", "kp9",
	"kpdecimal", "kpenter", "kpplus", "kpminus", "kpstar", "kpdivide", "kpequals", "kpclear",
}

// KeyNames returns every accepted key name, sorted.
func KeyNames() []string {
	names := make([]string, 0, len(keyNames))
	for n := range keyNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func keyName(code int) string {
	for _, n := range canonicalKeyOrder {
		if keyNames[n] == code {
			return n
		}
	}
	return fmt.Sprintf("key%d", code)
}

func actionFromString(s string) (Action, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return Action{}, nil
	case "mouseleft":
		return mouseAction(ActionMouseLeft), nil
	case "mouseright":
		return mouseAction(ActionMouseRight), nil
	case "wheelup":
		return mouseAction(ActionWheelUp), nil
	case "wheeldown":
		return mouseAction(ActionWheelDown), nil
	}
	if code, ok := keyNames[strings.ToLower(strings.TrimSpace(s))]; ok {
		return keyAction(code), nil
	}
	return Action{}, fmt.Errorf("unknown key or action %q (run \"pocketpad keys\" for the list)", s)
}

func actionToString(a Action) string {
	switch a.Kind {
	case ActionKey:
		return keyName(a.Key)
	case ActionMouseLeft:
		return "mouseLeft"
	case ActionMouseRight:
		return "mouseRight"
	case ActionWheelUp:
		return "wheelUp"
	case ActionWheelDown:
		return "wheelDown"
	}
	return "none"
}

var buttonNames = map[string]protocol.Buttons{
	"a": protocol.ButtonA, "b": protocol.ButtonB, "x": protocol.ButtonX, "y": protocol.ButtonY,
	"lb": protocol.ButtonLB, "rb": protocol.ButtonRB, "back": protocol.ButtonBack,
	"start": protocol.ButtonStart, "guide": protocol.ButtonGuide,
	"ls": protocol.ButtonLS, "rs": protocol.ButtonRS,
}

// fileProfile is the JSON shape of a mapping file.
type fileProfile struct {
	Name                  string            `json:"name,omitempty"`
	Buttons               map[string]string `json:"buttons,omitempty"`
	DPad                  map[string]string `json:"dpad,omitempty"`
	LeftStick             map[string]string `json:"leftStick,omitempty"`
	RightStick            json.RawMessage   `json:"rightStick,omitempty"`
	LeftTrigger           string            `json:"leftTrigger,omitempty"`
	RightTrigger          string            `json:"rightTrigger,omitempty"`
	StickThreshold        float32           `json:"stickThreshold,omitempty"`
	MouseSpeedPxPerSecond float64           `json:"mouseSpeedPxPerSecond,omitempty"`
}

func directions(from map[string]string, where string) ([4]Action, error) {
	var out [4]Action // up, down, left, right
	for name, value := range from {
		a, err := actionFromString(value)
		if err != nil {
			return out, fmt.Errorf("%s.%s: %w", where, name, err)
		}
		switch strings.ToLower(name) {
		case "up":
			out[0] = a
		case "down":
			out[1] = a
		case "left":
			out[2] = a
		case "right":
			out[3] = a
		default:
			return out, fmt.Errorf("%s: unknown direction %q (use up, down, left, right)", where, name)
		}
	}
	return out, nil
}

// LoadProfileFile reads and validates a custom mapping file.
func LoadProfileFile(path string) (MacKeyProfile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return MacKeyProfile{}, err
	}
	p, err := ParseProfile(b)
	if err != nil {
		return MacKeyProfile{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// ParseProfile turns mapping JSON into a profile. Unknown keys, buttons or
// directions are errors that name the offending field.
func ParseProfile(b []byte) (MacKeyProfile, error) {
	var fp fileProfile
	if err := json.Unmarshal(b, &fp); err != nil {
		return MacKeyProfile{}, fmt.Errorf("invalid mapping JSON: %w", err)
	}
	p := MacKeyProfile{
		Name:           "custom",
		Buttons:        map[protocol.Buttons]Action{},
		StickThreshold: fp.StickThreshold,
		// Sensible defaults when the field is omitted.
		RightStickMouse:       true,
		MouseSpeedPxPerSecond: fp.MouseSpeedPxPerSecond,
		LT:                    mouseAction(ActionMouseLeft),
		RT:                    mouseAction(ActionMouseRight),
	}

	for name, value := range fp.Buttons {
		btn, ok := buttonNames[strings.ToLower(name)]
		if !ok {
			return MacKeyProfile{}, fmt.Errorf("buttons: unknown button %q (use A, B, X, Y, LB, RB, Back, Start, Guide, LS, RS)", name)
		}
		a, err := actionFromString(value)
		if err != nil {
			return MacKeyProfile{}, fmt.Errorf("buttons.%s: %w", name, err)
		}
		p.Buttons[btn] = a
	}

	if fp.DPad != nil {
		d, err := directions(fp.DPad, "dpad")
		if err != nil {
			return MacKeyProfile{}, err
		}
		p.DPadUp, p.DPadDown, p.DPadLeft, p.DPadRight = d[0], d[1], d[2], d[3]
	}
	if fp.LeftStick != nil {
		d, err := directions(fp.LeftStick, "leftStick")
		if err != nil {
			return MacKeyProfile{}, err
		}
		p.StickUp, p.StickDown, p.StickLeft, p.StickRight = d[0], d[1], d[2], d[3]
	}

	if len(fp.RightStick) > 0 {
		var asString string
		if err := json.Unmarshal(fp.RightStick, &asString); err == nil {
			switch strings.ToLower(asString) {
			case "mouse":
				p.RightStickMouse = true
			case "none", "":
				p.RightStickMouse = false
			default:
				return MacKeyProfile{}, fmt.Errorf("rightStick: expected \"mouse\" or an object of up/down/left/right keys, got %q", asString)
			}
		} else {
			var asMap map[string]string
			if err := json.Unmarshal(fp.RightStick, &asMap); err != nil {
				return MacKeyProfile{}, fmt.Errorf("rightStick: expected \"mouse\" or an object of up/down/left/right keys")
			}
			d, err := directions(asMap, "rightStick")
			if err != nil {
				return MacKeyProfile{}, err
			}
			p.RightStickMouse = false
			p.RSUp, p.RSDown, p.RSLeft, p.RSRight = d[0], d[1], d[2], d[3]
		}
	}

	if fp.LeftTrigger != "" {
		a, err := actionFromString(fp.LeftTrigger)
		if err != nil {
			return MacKeyProfile{}, fmt.Errorf("leftTrigger: %w", err)
		}
		p.LT = a
	}
	if fp.RightTrigger != "" {
		a, err := actionFromString(fp.RightTrigger)
		if err != nil {
			return MacKeyProfile{}, fmt.Errorf("rightTrigger: %w", err)
		}
		p.RT = a
	}
	return p, nil
}

var buttonDisplayName = map[string]string{
	"a": "A", "b": "B", "x": "X", "y": "Y", "lb": "LB", "rb": "RB",
	"back": "Back", "start": "Start", "guide": "Guide", "ls": "LS", "rs": "RS",
}

// toFileProfile converts a profile back into the JSON shape (template).
func toFileProfile(p MacKeyProfile) fileProfile {
	fp := fileProfile{
		Name:                  "custom",
		Buttons:               map[string]string{},
		DPad:                  map[string]string{},
		LeftStick:             map[string]string{},
		StickThreshold:        p.StickThreshold,
		MouseSpeedPxPerSecond: p.MouseSpeedPxPerSecond,
	}
	for name, btn := range buttonNames {
		if a, ok := p.Buttons[btn]; ok && !a.unset() {
			fp.Buttons[buttonDisplayName[name]] = actionToString(a)
		}
	}
	set := func(m map[string]string, dir string, a Action) {
		if !a.unset() {
			m[dir] = actionToString(a)
		}
	}
	set(fp.DPad, "up", p.DPadUp)
	set(fp.DPad, "down", p.DPadDown)
	set(fp.DPad, "left", p.DPadLeft)
	set(fp.DPad, "right", p.DPadRight)
	set(fp.LeftStick, "up", p.StickUp)
	set(fp.LeftStick, "down", p.StickDown)
	set(fp.LeftStick, "left", p.StickLeft)
	set(fp.LeftStick, "right", p.StickRight)

	if p.RightStickMouse {
		fp.RightStick = json.RawMessage(`"mouse"`)
	} else {
		rs := map[string]string{}
		set(rs, "up", p.RSUp)
		set(rs, "down", p.RSDown)
		set(rs, "left", p.RSLeft)
		set(rs, "right", p.RSRight)
		b, _ := json.Marshal(rs)
		fp.RightStick = b
	}
	fp.LeftTrigger = actionToString(p.LT)
	fp.RightTrigger = actionToString(p.RT)
	return fp
}

// TemplateMappingJSON is what `-profile custom` writes on first use: the
// general mapping, fully spelled out, ready to edit.
func TemplateMappingJSON() []byte {
	fp := toFileProfile(macKeyProfiles["general"])
	b, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(b, '\n')
}

// CurrentMappingJSON exposes the currently selected mapping in fileProfile
// shape, for the Mac app's mapping editor.
func CurrentMappingJSON() []byte {
	fp := toFileProfile(currentProfile())
	b, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(b, '\n')
}
