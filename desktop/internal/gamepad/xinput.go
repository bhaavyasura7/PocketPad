package gamepad

import "github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"

// XUSBReport is the Xbox 360 (XInput) report used by the ViGEm backend.
// Its layout matches ViGEm's XUSB_REPORT (12 bytes, little-endian).
type XUSBReport struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

// XInput button bits.
const (
	xusbDPadUp    = 0x0001
	xusbDPadDown  = 0x0002
	xusbDPadLeft  = 0x0004
	xusbDPadRight = 0x0008
	xusbStart     = 0x0010
	xusbBack      = 0x0020
	xusbLeftThumb = 0x0040
	xusbRightThmb = 0x0080
	xusbLeftShldr = 0x0100
	xusbRightShdr = 0x0200
	xusbGuide     = 0x0400
	xusbA         = 0x1000
	xusbB         = 0x2000
	xusbX         = 0x4000
	xusbY         = 0x8000
)

var xusbButtonMap = [...]struct {
	from protocol.Buttons
	to   uint16
}{
	{protocol.ButtonA, xusbA}, {protocol.ButtonB, xusbB}, {protocol.ButtonX, xusbX}, {protocol.ButtonY, xusbY},
	{protocol.ButtonLB, xusbLeftShldr}, {protocol.ButtonRB, xusbRightShdr},
	{protocol.ButtonBack, xusbBack}, {protocol.ButtonStart, xusbStart},
	{protocol.ButtonLS, xusbLeftThumb}, {protocol.ButtonRS, xusbRightThmb},
	{protocol.ButtonGuide, xusbGuide},
}

// ToXUSB converts a normalized state to an XInput report. XInput uses +Y up,
// like the protocol, so no axis is flipped.
func ToXUSB(s protocol.ControllerState) XUSBReport {
	var r XUSBReport
	for _, m := range xusbButtonMap {
		if s.Buttons&m.from != 0 {
			r.Buttons |= m.to
		}
	}
	if s.DPad.Up() {
		r.Buttons |= xusbDPadUp
	}
	if s.DPad.Down() {
		r.Buttons |= xusbDPadDown
	}
	if s.DPad.Left() {
		r.Buttons |= xusbDPadLeft
	}
	if s.DPad.Right() {
		r.Buttons |= xusbDPadRight
	}
	r.LeftTrigger = protocol.EncodeTrigger(s.LeftTrigger)
	r.RightTrigger = protocol.EncodeTrigger(s.RightTrigger)
	r.ThumbLX = protocol.EncodeAxis(s.LeftX)
	r.ThumbLY = protocol.EncodeAxis(s.LeftY)
	r.ThumbRX = protocol.EncodeAxis(s.RightX)
	r.ThumbRY = protocol.EncodeAxis(s.RightY)
	return r
}

// Bytes returns the report in its 12-byte wire layout.
func (r XUSBReport) Bytes() [12]byte {
	return [12]byte{
		byte(r.Buttons), byte(r.Buttons >> 8), r.LeftTrigger, r.RightTrigger,
		byte(r.ThumbLX), byte(uint16(r.ThumbLX) >> 8), byte(r.ThumbLY), byte(uint16(r.ThumbLY) >> 8),
		byte(r.ThumbRX), byte(uint16(r.ThumbRX) >> 8), byte(r.ThumbRY), byte(uint16(r.ThumbRY) >> 8),
	}
}

// motor8To16 scales an XInput motor byte to the protocol's 16-bit range.
func motor8To16(v uint8) uint16 { return uint16(v) * 257 }
