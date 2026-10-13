package gamepad

import (
	"testing"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func TestToHIDReportNeutral(t *testing.T) {
	r := ToHIDReport(protocol.Neutral)
	want := [HIDReportSize]byte{1, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if r != want {
		t.Fatalf("neutral report % x, want % x", r, want)
	}
}

func TestToHIDReportKnownState(t *testing.T) {
	s := protocol.ControllerState{
		LeftX: 1, LeftY: 1, RightX: -1, RightY: -1,
		LeftTrigger: 1, RightTrigger: 0,
		Buttons: protocol.ButtonA | protocol.ButtonStart,
		DPad:    protocol.DPadUp,
	}
	r := ToHIDReport(s)
	want := [HIDReportSize]byte{
		1,          // report ID
		0x81, 0x00, // A (bit 0) + Start (bit 7)
		0x00,       // hat: up (1) -> 0
		0xFF, 0x7F, // LX = +32767
		0x01, 0x80, // LY = -32767 (HID Y down)
		0x01, 0x80, // RX = -32767
		0xFF, 0x7F, // RY = +32767
		0xFF, // LT full
		0x00, // RT off
	}
	if r != want {
		t.Fatalf("report % x,\nwant % x", r, want)
	}
}

func TestToHIDReportMasksReservedBits(t *testing.T) {
	r := ToHIDReport(protocol.ControllerState{Buttons: 0xFFFF})
	if r[1] != 0xFF || r[2] != 0x07 {
		t.Fatalf("buttons % x%x, want ff07", r[2], r[1])
	}
}

func TestHIDDescriptorIsStable(t *testing.T) {
	if len(HIDDescriptor) != 95 {
		t.Fatalf("descriptor length %d, want 95", len(HIDDescriptor))
	}
	if HIDDescriptor[0] != 0x05 || HIDDescriptor[len(HIDDescriptor)-1] != 0xC0 {
		t.Fatal("descriptor anchors changed")
	}
}
