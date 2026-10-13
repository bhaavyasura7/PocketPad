package protocol

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"testing"
)

// The golden STATE packet from docs/protocol.md, computed by hand.
var goldenState = mustHex("5050010102018100" + "0300ff7f01800000" + "0040ff00")

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func goldenWire() WireState {
	return WireState{
		Buttons:     ButtonA | ButtonStart,
		DPad:        DPadRight,
		LeftX:       math.MaxInt16,
		LeftY:       -math.MaxInt16,
		RightY:      16384,
		LeftTrigger: 255,
	}
}

func TestGoldenState(t *testing.T) {
	var b [StateSize]byte
	w := goldenWire()
	PutState(b[:], 0x0102, &w)
	if !bytes.Equal(b[:], goldenState) {
		t.Fatalf("encoded\n %x\nwant\n %x", b, goldenState)
	}
	seq, got, err := DecodeState(goldenState)
	if err != nil || seq != 0x0102 || got != w {
		t.Fatalf("decode = %d %+v %v", seq, got, err)
	}
	h, err := CheckHeader(goldenState[:StateSize])
	if !errors.Is(err, ErrLength) {
		t.Fatalf("an untagged STATE must fail CheckHeader with ErrLength, got %v %v", h, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	s := ControllerState{
		LeftX: -0.25, LeftY: 1, RightX: 0.75, RightY: -1,
		LeftTrigger: 0.5, RightTrigger: 1,
		Buttons: ButtonsDefined, DPad: DPadUpLeft,
	}
	w := s.ToWire(FlagSnapshot)
	var b [StateSize]byte
	PutState(b[:], 65535, &w)
	seq, w2, err := DecodeState(b[:])
	if err != nil || seq != 65535 || w2 != w {
		t.Fatalf("round trip: %d %+v %v", seq, w2, err)
	}
	n := w2.Normalized()
	const eps = 1.0 / 250
	for _, d := range []float32{n.LeftX - s.LeftX, n.LeftY - s.LeftY, n.RightX - s.RightX, n.RightY - s.RightY, n.LeftTrigger - s.LeftTrigger, n.RightTrigger - s.RightTrigger} {
		if d > eps || d < -eps {
			t.Fatalf("normalized drift %v: %+v vs %+v", d, n, s)
		}
	}
	if n.Buttons != s.Buttons || n.DPad != s.DPad {
		t.Fatalf("buttons/dpad: %+v", n)
	}
}

func TestAxisEncoding(t *testing.T) {
	cases := []struct {
		in   float32
		want int16
	}{
		{0, 0}, {1, 32767}, {-1, -32767}, {2, 32767}, {-2, -32767},
		{0.5, 16384}, {-0.5, -16384}, {float32(math.NaN()), 0},
	}
	for _, c := range cases {
		if got := EncodeAxis(c.in); got != c.want {
			t.Errorf("EncodeAxis(%v) = %d, want %d", c.in, got, c.want)
		}
	}
	if DecodeAxis(-32768) != -1 || DecodeAxis(-32767) != -1 || DecodeAxis(32767) != 1 || DecodeAxis(0) != 0 {
		t.Error("DecodeAxis extremes")
	}
	if EncodeTrigger(0) != 0 || EncodeTrigger(1) != 255 || EncodeTrigger(-1) != 0 || EncodeTrigger(0.5) != 128 {
		t.Error("EncodeTrigger")
	}
}

func TestDecodeStateRejects(t *testing.T) {
	mod := func(i int, v byte) []byte {
		b := bytes.Clone(goldenState)
		b[i] = v
		return b
	}
	cases := []struct {
		name string
		pkt  []byte
		want error
	}{
		{"short", goldenState[:19], ErrShort},
		{"magic", mod(0, 'X'), ErrMagic},
		{"version", mod(2, 2), ErrVersion},
		{"type", mod(3, 2), ErrType},
		{"reserved button", mod(7, 0x08), ErrReserved},
		{"reserved flag", mod(9, 0x02), ErrReserved},
		{"dpad", mod(8, 9), ErrRange},
	}
	for _, c := range cases {
		if _, _, err := DecodeState(c.pkt); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestCheckHeader(t *testing.T) {
	var b [StateSize + TagSize]byte
	w := goldenWire()
	PutState(b[:], 1, &w)
	if _, err := CheckHeader(b[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckHeader(b[:StateSize+TagSize-1]); !errors.Is(err, ErrLength) {
		t.Fatal(err)
	}
	b[3] = 0x7f
	if _, err := CheckHeader(b[:]); !errors.Is(err, ErrType) {
		t.Fatal(err)
	}
	b[2] = 9
	h, err := ParseHeader(b[:])
	if err != nil || h.Version != 9 {
		t.Fatal("ParseHeader must report foreign versions without failing")
	}
	if _, err := CheckHeader(b[:]); !errors.Is(err, ErrVersion) {
		t.Fatal(err)
	}
	if _, err := CheckHeader(make([]byte, MaxDatagram+1)); !errors.Is(err, ErrLong) {
		t.Fatal(err)
	}
}

func TestDPadFrom(t *testing.T) {
	for d := DPadNeutral; d <= dpadMax; d++ {
		if got := DPadFrom(d.Up(), d.Down(), d.Left(), d.Right()); got != d {
			t.Errorf("DPadFrom(parts of %v) = %v", d, got)
		}
	}
	if DPadFrom(true, true, false, true) != DPadRight {
		t.Error("opposites must cancel")
	}
}

func TestControlPacketsRoundTrip(t *testing.T) {
	var b [MaxDatagram]byte

	hb := Heartbeat{TimeUs: 0xDEADBEEF, EchoUs: 7, RttUs: 1500, Battery: 87, Flags: HeartbeatCharging}
	PutHeartbeat(b[:], 3, &hb)
	if got, err := DecodeHeartbeat(b[:HeartbeatSize]); err != nil || got != hb {
		t.Fatalf("heartbeat %+v %v", got, err)
	}

	PutDisconnect(b[:], 4, ReasonShutdown)
	if r, err := DecodeDisconnect(b[:DisconnectSize]); err != nil || r != ReasonShutdown {
		t.Fatalf("disconnect %v %v", r, err)
	}

	rb := Rumble{Left: 0xFFFF, Right: 0x8000, DurationMs: 500}
	PutRumble(b[:], 5, &rb)
	if got, err := DecodeRumble(b[:RumbleSize]); err != nil || got != rb {
		t.Fatalf("rumble %+v %v", got, err)
	}

	var hello Hello
	hello.DeviceID = 0x0102030405060708
	hello.ClientNonce[0] = 9
	hello.MinVersion, hello.MaxVersion = 1, 3
	hello.Caps = CapRumble | CapBattery
	hello.SetName("Pixel 8 Pro — Bhaavya's phone")
	PutHello(b[:], &hello)
	got, err := DecodeHello(b[:HelloSize])
	if err != nil || got != hello {
		t.Fatalf("hello %+v %v", got, err)
	}
	if n := got.Name(); len(n) > DeviceNameSize || n != "Pixel 8 Pro — Bhaavya'" {
		t.Fatalf("name truncation: %q", n)
	}
	for in, want := range map[string]string{
		"Pixel 8 Pro Controller —x": "Pixel 8 Pro Controller ", // split "—" is dropped
		"Pixel 8 Pro Controlle—x":   "Pixel 8 Pro Controlle—",  // "—" ends at byte 24 and is kept
	} {
		hello.SetName(in)
		if got := hello.Name(); got != want {
			t.Errorf("SetName(%q) = %q, want %q", in, got, want)
		}
	}

	wel := Welcome{SessionID: 42, Version: 1, Caps: CapRumble, HeartbeatMs: 500, TimeoutMs: 300, MaxRateHz: 250}
	wel.ServerNonce[15] = 1
	PutWelcome(b[:], &wel)
	if got, err := DecodeWelcome(b[:WelcomeSize]); err != nil || got != wel {
		t.Fatalf("welcome %+v %v", got, err)
	}

	rej := Reject{Reason: RejectVersion, MinVersion: 1, MaxVersion: 1}
	PutReject(b[:], &rej)
	if got, err := DecodeReject(b[:RejectSize]); err != nil || got != rej {
		t.Fatalf("reject %+v %v", got, err)
	}

	dr := DiscoverReply{Port: 47770, Caps: CapRumble, MinVersion: 1, MaxVersion: 1, Name: "Studio PC"}
	pkt := AppendDiscoverReply(nil, &dr)
	if _, err := CheckHeader(pkt); err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeDiscoverReply(pkt); err != nil || got != dr {
		t.Fatalf("discover reply %+v %v", got, err)
	}
}

func TestSeqNewer(t *testing.T) {
	cases := []struct {
		a, b  uint16
		newer bool
	}{
		{1, 0, true}, {0, 1, false}, {5, 5, false},
		{0, 65535, true}, {65535, 0, false},
		{100, 65000, true}, {32767, 0, true}, {32768, 0, false},
	}
	for _, c := range cases {
		if got := SeqNewer(c.a, c.b); got != c.newer {
			t.Errorf("SeqNewer(%d, %d) = %v", c.a, c.b, got)
		}
	}
}

func TestReplayWindowWraps(t *testing.T) {
	var w ReplayWindow
	var c SeqCounter
	for i := 0; i < 70000; i++ {
		e := c.Next()
		ext, fresh := w.Estimate(e.Wire())
		if !fresh || ext != e {
			t.Fatalf("i=%d: estimate %d fresh=%v, want %d", i, ext, fresh, e)
		}
		w.Accept(ext)
		if _, fresh := w.Estimate(e.Wire()); fresh {
			t.Fatalf("i=%d: duplicate accepted", i)
		}
	}
	// A burst of loss shorter than half the sequence space is recovered.
	ext, fresh := w.Estimate(uint16((70000 + 30000) & 0xFFFF))
	if !fresh || ext != 70000+30000 {
		t.Fatalf("after loss: %d %v", ext, fresh)
	}
	if gap := w.Accept(ext); gap != 30000 {
		t.Fatalf("gap %d", gap)
	}
}

func TestSessionSealOpen(t *testing.T) {
	keys := DeriveSessionKeys(PairKey([]byte("0123456789abcde")), 1, [16]byte{1}, [16]byte{2}, 7)
	sealer := NewSealer(keys.ClientToServer)
	opener := NewOpener(keys.ClientToServer)
	wrong := NewOpener(keys.ServerToClient)

	var pkt [StateSize + TagSize]byte
	w := goldenWire()
	var captured [][]byte
	for i := 0; i < 65536+10; i++ {
		ext := sealer.Next()
		PutState(pkt[:], ext.Wire(), &w)
		d := sealer.Seal(pkt[:], StateSize, ext)
		if i == 20 {
			captured = append(captured, bytes.Clone(d))
		}
		if i == 2 {
			if _, err := wrong.Open(d); !errors.Is(err, ErrAuth) {
				t.Fatalf("reflected packet: %v", err)
			}
		}
		if _, err := opener.Open(d); err != nil {
			t.Fatalf("i=%d: %v", i, err)
		}
	}
	// Packet 20 has the same wire sequence number as packet 65556, which is
	// still in the future, but its tag binds rollover counter 0.
	if _, err := opener.Open(captured[0]); !errors.Is(err, ErrAuth) {
		t.Fatalf("replay across wraparound: %v", err)
	}
	// Tampering is detected.
	ext := sealer.Next()
	PutState(pkt[:], ext.Wire(), &w)
	d := sealer.Seal(pkt[:], StateSize, ext)
	d[6] ^= 1
	if _, err := opener.Open(d); !errors.Is(err, ErrAuth) {
		t.Fatalf("tamper: %v", err)
	}
	d[6] ^= 1
	if _, err := opener.Open(d); err != nil {
		t.Fatalf("a failed forgery must not advance the window: %v", err)
	}
	if _, err := opener.Open(d); !errors.Is(err, ErrStale) {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestHandshakeTag(t *testing.T) {
	key := PairKey([]byte("0123456789abcde"))
	var b [HelloSize + HandshakeTagSize]byte
	h := Hello{DeviceID: 5, MinVersion: 1, MaxVersion: 1}
	PutHello(b[:], &h)
	copy(b[HelloSize:], HandshakeTag(key, b[:HelloSize], nil))
	if !VerifyHandshake(key, b[:], nil) {
		t.Fatal("verify")
	}
	if VerifyHandshake(PairKey([]byte("another secret!")), b[:], nil) {
		t.Fatal("wrong key accepted")
	}
	if VerifyHandshake(key, b[:], []byte{1}) {
		t.Fatal("binding ignored")
	}
}

func TestPairingCode(t *testing.T) {
	secret, err := NewPairingSecret()
	if err != nil {
		t.Fatal(err)
	}
	code := FormatPairingCode(secret)
	if len(code) != 29 {
		t.Fatalf("code %q", code)
	}
	got, err := ParsePairingCode(code)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("parse %q: %x %v", code, got, err)
	}
	fixed := mustHex("000102030405060708090a0b0c0d0e")
	code = FormatPairingCode(fixed)
	if code != "000G-40R4-0M30-E209-185G-R38E" {
		t.Fatalf("format %q", code)
	}
	for _, in := range []string{"000G40R40M30 E209185GR38E", "ooog-4or4-om3o-e2o9-l85g-r38e", " 000G-40R4-0M30-E209-185G-R38E\t"} {
		if got, err := ParsePairingCode(in); err != nil || !bytes.Equal(got, fixed) {
			t.Errorf("parse %q: %x %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0004-0810", "UUUU-40R4-0M30-E209-185G-R38E"} {
		if _, err := ParsePairingCode(in); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestHotPathDoesNotAllocate(t *testing.T) {
	keys := DeriveSessionKeys(PairKey([]byte("0123456789abcde")), 1, [16]byte{}, [16]byte{}, 1)
	sealer := NewSealer(keys.ClientToServer)
	opener := NewOpener(keys.ClientToServer)
	var pkt [StateSize + TagSize]byte
	s := ControllerState{LeftX: 0.3, Buttons: ButtonA}
	allocs := testing.AllocsPerRun(1000, func() {
		w := s.ToWire(0)
		ext := sealer.Next()
		PutState(pkt[:], ext.Wire(), &w)
		d := sealer.Seal(pkt[:], StateSize, ext)
		if _, err := CheckHeader(d); err != nil {
			t.Fatal(err)
		}
		if _, err := opener.Open(d); err != nil {
			t.Fatal(err)
		}
		if _, _, err := DecodeState(d); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("%v allocations per packet", allocs)
	}
}

func FuzzDecoders(f *testing.F) {
	f.Add(goldenState)
	f.Add(append(bytes.Clone(goldenState), make([]byte, TagSize)...))
	f.Fuzz(func(t *testing.T, b []byte) {
		h, err := CheckHeader(b)
		if err != nil {
			return
		}
		switch h.Type {
		case TypeState:
			DecodeState(b)
		case TypeHeartbeat:
			DecodeHeartbeat(b)
		case TypeDisconnect:
			DecodeDisconnect(b)
		case TypeRumble:
			DecodeRumble(b)
		case TypeHello:
			DecodeHello(b)
		case TypeWelcome:
			DecodeWelcome(b)
		case TypeReject:
			DecodeReject(b)
		case TypeDiscoverReply:
			DecodeDiscoverReply(b)
		}
	})
}

func BenchmarkSealOpenState(b *testing.B) {
	keys := DeriveSessionKeys(PairKey([]byte("0123456789abcde")), 1, [16]byte{}, [16]byte{}, 1)
	sealer := NewSealer(keys.ClientToServer)
	opener := NewOpener(keys.ClientToServer)
	var pkt [StateSize + TagSize]byte
	w := goldenWire()
	for b.Loop() {
		ext := sealer.Next()
		PutState(pkt[:], ext.Wire(), &w)
		opener.Open(sealer.Seal(pkt[:], StateSize, ext))
	}
}
