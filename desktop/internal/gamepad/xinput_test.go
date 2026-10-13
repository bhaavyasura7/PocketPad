package gamepad

import (
	"testing"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func TestToXUSB(t *testing.T) {
	s := protocol.ControllerState{
		LeftX: 1, LeftY: -1, RightX: 0.5, RightTrigger: 1,
		Buttons: protocol.ButtonA | protocol.ButtonY | protocol.ButtonLB | protocol.ButtonStart | protocol.ButtonRS | protocol.ButtonGuide,
		DPad:    protocol.DPadDownLeft,
	}
	r := ToXUSB(s)
	want := uint16(xusbA | xusbY | xusbLeftShldr | xusbStart | xusbRightThmb | xusbGuide | xusbDPadDown | xusbDPadLeft)
	if r.Buttons != want {
		t.Fatalf("buttons %#04x, want %#04x", r.Buttons, want)
	}
	if r.ThumbLX != 32767 || r.ThumbLY != -32767 || r.ThumbRX != 16384 || r.RightTrigger != 255 || r.LeftTrigger != 0 {
		t.Fatalf("report %+v", r)
	}
	b := r.Bytes()
	if b[0] != byte(want) || b[1] != byte(want>>8) || b[3] != 255 || b[4] != 0xff || b[5] != 0x7f || b[6] != 0x01 || b[7] != 0x80 {
		t.Fatalf("bytes % x", b)
	}
	if ToXUSB(protocol.Neutral) != (XUSBReport{}) {
		t.Fatal("neutral is not all zero")
	}
	if motor8To16(255) != 0xFFFF || motor8To16(0) != 0 {
		t.Fatal("motor scaling")
	}
}

func TestOpenAuto(t *testing.T) {
	if _, err := Open("mock"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open("nope"); err == nil {
		t.Fatal("unknown backend opened")
	}
}
