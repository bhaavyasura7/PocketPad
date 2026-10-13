// Package protocol implements the PocketPad wire protocol: the canonical
// controller state model, the binary packet codec, sequence-number handling
// and the session authentication primitives.
//
// See docs/protocol.md for the normative description. Every multibyte field
// is little-endian.
package protocol

import "math"

// Buttons is the 16-bit button mask carried in the controller-state packet.
type Buttons uint16

const (
	ButtonA Buttons = 1 << iota
	ButtonB
	ButtonX
	ButtonY
	ButtonLB
	ButtonRB
	ButtonBack
	ButtonStart
	ButtonLS // left stick click
	ButtonRS // right stick click
	ButtonGuide

	// ButtonsDefined has every bit defined by protocol version 1 set.
	// The remaining bits are reserved and must be zero.
	ButtonsDefined Buttons = 1<<11 - 1
)

var buttonNames = [...]string{"A", "B", "X", "Y", "LB", "RB", "Back", "Start", "LS", "RS", "Guide"}

// Names returns the names of the pressed buttons in bit order.
func (b Buttons) Names() []string {
	var out []string
	for i, n := range buttonNames {
		if b&(1<<i) != 0 {
			out = append(out, n)
		}
	}
	return out
}

// DPad is the D-pad direction. Zero is neutral; 1..8 run clockwise from up.
type DPad uint8

const (
	DPadNeutral DPad = iota
	DPadUp
	DPadUpRight
	DPadRight
	DPadDownRight
	DPadDown
	DPadDownLeft
	DPadLeft
	DPadUpLeft

	dpadMax = DPadUpLeft
)

var dpadNames = [...]string{"·", "N", "NE", "E", "SE", "S", "SW", "W", "NW"}

func (d DPad) String() string {
	if d > dpadMax {
		return "?"
	}
	return dpadNames[d]
}

// Valid reports whether d is a defined D-pad value.
func (d DPad) Valid() bool { return d <= dpadMax }

// Up, Down, Left and Right decompose the direction into its cardinal parts.
func (d DPad) Up() bool    { return d == DPadUp || d == DPadUpRight || d == DPadUpLeft }
func (d DPad) Down() bool  { return d == DPadDown || d == DPadDownRight || d == DPadDownLeft }
func (d DPad) Left() bool  { return d == DPadLeft || d == DPadUpLeft || d == DPadDownLeft }
func (d DPad) Right() bool { return d == DPadRight || d == DPadUpRight || d == DPadDownRight }

// DPadFrom composes a direction from cardinal parts. Opposing directions
// cancel each other out.
func DPadFrom(up, down, left, right bool) DPad {
	if up && down {
		up, down = false, false
	}
	if left && right {
		left, right = false, false
	}
	switch {
	case up && right:
		return DPadUpRight
	case up && left:
		return DPadUpLeft
	case down && right:
		return DPadDownRight
	case down && left:
		return DPadDownLeft
	case up:
		return DPadUp
	case down:
		return DPadDown
	case left:
		return DPadLeft
	case right:
		return DPadRight
	}
	return DPadNeutral
}

// ControllerState is the canonical, normalized controller state.
//
// Axes are in [-1, 1] with positive X to the right and positive Y up (stick
// pushed away from the player). Triggers are in [0, 1].
type ControllerState struct {
	LeftX, LeftY   float32
	RightX, RightY float32
	LeftTrigger    float32
	RightTrigger   float32
	Buttons        Buttons
	DPad           DPad
}

// Neutral is the state with every input released.
var Neutral ControllerState

// IsNeutral reports whether every input is released.
func (s ControllerState) IsNeutral() bool { return s == Neutral }

// EncodeAxis converts a normalized axis value to its wire form. Values
// outside [-1, 1] are clamped; NaN encodes as center.
func EncodeAxis(v float32) int16 {
	if v != v { // NaN
		return 0
	}
	if v >= 1 {
		return math.MaxInt16
	}
	if v <= -1 {
		return -math.MaxInt16
	}
	return int16(math.Round(float64(v) * math.MaxInt16))
}

// DecodeAxis converts a wire axis value to [-1, 1]. Both -32768 and -32767
// decode to -1 so the range is symmetric.
func DecodeAxis(v int16) float32 {
	if v <= -math.MaxInt16 {
		return -1
	}
	return float32(v) / math.MaxInt16
}

// EncodeTrigger converts a normalized trigger value to its wire form.
func EncodeTrigger(v float32) uint8 {
	if v != v || v <= 0 {
		return 0
	}
	if v >= 1 {
		return math.MaxUint8
	}
	return uint8(math.Round(float64(v) * math.MaxUint8))
}

// DecodeTrigger converts a wire trigger value to [0, 1].
func DecodeTrigger(v uint8) float32 { return float32(v) / math.MaxUint8 }
