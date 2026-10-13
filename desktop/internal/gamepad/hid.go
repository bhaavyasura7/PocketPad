package gamepad

import (
	"encoding/binary"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// The HID gamepad shape shared by Bluetooth mode (Android) and the macOS
// backend (docs/HANDOFF.md §4.4): report ID 1 and 13 data bytes — 16 buttons,
// a 4-bit hat switch plus padding, four int16 axes and two uint8 triggers.
const (
	HIDReportID   = 1
	HIDDataSize   = 13
	HIDReportSize = HIDDataSize + 1 // what reaches the OS includes the report ID
)

// HIDDescriptor is the HID report descriptor for that layout. It is frozen:
// the Android Bluetooth transport and the macOS IOHIDUserDevice backend must
// send byte-identical reports, so changes here are a protocol-visible change.
var HIDDescriptor = []byte{
	0x05, 0x01, // Usage Page (Generic Desktop)
	0x09, 0x05, // Usage (Game Pad)
	0xA1, 0x01, // Collection (Application)
	0x85, 0x01, //   Report ID (1)
	0x05, 0x09, //   Usage Page (Button)
	0x19, 0x01, 0x29, 0x10, //   Usage Min 1, Max 16
	0x15, 0x00, 0x25, 0x01, //   Logical 0..1
	0x75, 0x01, 0x95, 0x10, //   16 x 1 bit
	0x81, 0x02, //   Input (Data,Var,Abs)       -> bytes 0-1: buttons
	0x05, 0x01, //   Usage Page (Generic Desktop)
	0x09, 0x39, //   Usage (Hat switch)
	0x15, 0x00, 0x25, 0x07, //   Logical 0..7
	0x35, 0x00, 0x46, 0x3B, 0x01, // Physical 0..315
	0x65, 0x14, //   Unit (Degrees)
	0x75, 0x04, 0x95, 0x01,
	0x81, 0x42, //   Input (Data,Var,Abs,Null)  -> byte 2 low nibble
	0x65, 0x00, //   Unit (None)
	0x35, 0x00, 0x45, 0x00, // Physical reset
	0x75, 0x04, 0x95, 0x01,
	0x81, 0x03, //   Input (Const)              -> byte 2 high nibble
	0x09, 0x30, 0x09, 0x31, 0x09, 0x32, 0x09, 0x35, // X, Y, Z, Rz
	0x16, 0x01, 0x80, 0x26, 0xFF, 0x7F, // Logical -32767..32767
	0x75, 0x10, 0x95, 0x04,
	0x81, 0x02, //   Input                      -> bytes 3-10: LX, LY, RX, RY
	0x05, 0x02, //   Usage Page (Simulation Controls)
	0x09, 0xC5, 0x09, 0xC4, // Brake (LT), Accelerator (RT)
	0x15, 0x00, 0x26, 0xFF, 0x00, // Logical 0..255
	0x75, 0x08, 0x95, 0x02,
	0x81, 0x02, //   Input                      -> bytes 11-12: LT, RT
	0xC0, // End Collection
}

// ToHIDReport converts a normalized state into the wire report, including
// the leading report ID. Button bits map 1:1 (HID button 1 = A … 11 =
// Guide); the hat switch is 0..7 with 8 meaning null; HID Y grows downwards,
// so LY and RY are negated.
func ToHIDReport(s protocol.ControllerState) [HIDReportSize]byte {
	var r [HIDReportSize]byte
	r[0] = HIDReportID
	buttons := uint16(s.Buttons) & 0x07FF
	r[1] = byte(buttons)
	r[2] = byte(buttons >> 8)
	r[3] = 8 // hat: null/neutral
	if s.DPad != protocol.DPadNeutral {
		r[3] = byte(s.DPad) - 1
	}
	binary.LittleEndian.PutUint16(r[4:], uint16(protocol.EncodeAxis(s.LeftX)))
	binary.LittleEndian.PutUint16(r[6:], uint16(-protocol.EncodeAxis(s.LeftY)))
	binary.LittleEndian.PutUint16(r[8:], uint16(protocol.EncodeAxis(s.RightX)))
	binary.LittleEndian.PutUint16(r[10:], uint16(-protocol.EncodeAxis(s.RightY)))
	r[12] = protocol.EncodeTrigger(s.LeftTrigger)
	r[13] = protocol.EncodeTrigger(s.RightTrigger)
	return r
}
