package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Wire constants for protocol version 1.
const (
	Magic0, Magic1 = 'P', 'P'

	Version    uint8 = 1
	MinVersion uint8 = 1
	MaxVersion uint8 = 1

	DefaultPort = 47770

	HeaderSize = 6
	// TagSize is the truncated HMAC appended to every session packet.
	TagSize = 8
	// HandshakeTagSize is the truncated HMAC appended to HELLO and WELCOME.
	HandshakeTagSize = 16
	NonceSize        = 16

	StateSize         = 20
	HeartbeatSize     = 20
	DisconnectSize    = 8
	RumbleSize        = 12
	DiscoverSize      = 6
	DiscoverReplyMin  = 13
	HelloSize         = 58
	WelcomeSize       = 36
	RejectSize        = 10
	DeviceNameSize    = 24
	MaxServerNameSize = 32

	// MaxDatagram bounds every PocketPad datagram; anything larger is malformed.
	MaxDatagram = 128
)

// PacketType identifies the layout following the common header.
type PacketType uint8

const (
	// Session packets: authenticated with the per-direction session key.
	TypeState      PacketType = 0x01
	TypeHeartbeat  PacketType = 0x02
	TypeDisconnect PacketType = 0x03
	TypeRumble     PacketType = 0x04

	// Discovery: unauthenticated and informational only.
	TypeDiscover      PacketType = 0x10
	TypeDiscoverReply PacketType = 0x11

	// Handshake: authenticated with the pairing key.
	TypeHello   PacketType = 0x12
	TypeWelcome PacketType = 0x13

	// Reject is unauthenticated and informational only.
	TypeReject PacketType = 0x14
)

func (t PacketType) String() string {
	switch t {
	case TypeState:
		return "STATE"
	case TypeHeartbeat:
		return "HEARTBEAT"
	case TypeDisconnect:
		return "DISCONNECT"
	case TypeRumble:
		return "RUMBLE"
	case TypeDiscover:
		return "DISCOVER"
	case TypeDiscoverReply:
		return "DISCOVER_REPLY"
	case TypeHello:
		return "HELLO"
	case TypeWelcome:
		return "WELCOME"
	case TypeReject:
		return "REJECT"
	}
	return fmt.Sprintf("0x%02x", uint8(t))
}

// SessionPacket reports whether t is authenticated with a session key.
func (t PacketType) SessionPacket() bool {
	return t >= TypeState && t <= TypeRumble
}

// BodySize returns the fixed size of a packet of type t excluding any tag,
// or 0 for variable-length or unknown types.
func (t PacketType) BodySize() int {
	switch t {
	case TypeState:
		return StateSize
	case TypeHeartbeat:
		return HeartbeatSize
	case TypeDisconnect:
		return DisconnectSize
	case TypeRumble:
		return RumbleSize
	case TypeDiscover:
		return DiscoverSize
	case TypeHello:
		return HelloSize
	case TypeWelcome:
		return WelcomeSize
	case TypeReject:
		return RejectSize
	}
	return 0
}

// Capability bits, exchanged in HELLO, WELCOME and DISCOVER_REPLY.
type Caps uint16

const (
	CapRumble Caps = 1 << iota
	CapBattery
	CapGuide
	CapLatencyEcho
)

// StateFlags travel in the controller-state packet.
type StateFlags uint8

const (
	// FlagSnapshot marks a periodic resend of unchanged state.
	FlagSnapshot StateFlags = 1 << iota

	stateFlagsDefined = FlagSnapshot
)

// DisconnectReason travels in DISCONNECT.
type DisconnectReason uint8

const (
	ReasonUser DisconnectReason = iota
	ReasonShutdown
	ReasonTimeout
	ReasonReplaced
	reasonMax = ReasonReplaced
)

// RejectReason travels in REJECT.
type RejectReason uint8

const (
	RejectVersion RejectReason = 1 + iota
	RejectBusy
	RejectRateLimited
	rejectMax = RejectRateLimited
)

func (r RejectReason) String() string {
	switch r {
	case RejectVersion:
		return "unsupported protocol version"
	case RejectBusy:
		return "another controller is connected"
	case RejectRateLimited:
		return "rate limited"
	}
	return fmt.Sprintf("reason %d", uint8(r))
}

// Errors returned by the decoders. They are sentinel values so the receiver
// can count them without allocating.
var (
	ErrShort    = errors.New("protocol: packet too short")
	ErrLong     = errors.New("protocol: packet too long")
	ErrMagic    = errors.New("protocol: bad magic")
	ErrVersion  = errors.New("protocol: unsupported version")
	ErrType     = errors.New("protocol: unknown packet type")
	ErrLength   = errors.New("protocol: wrong length for packet type")
	ErrReserved = errors.New("protocol: reserved bits set")
	ErrRange    = errors.New("protocol: field out of range")
	ErrAuth     = errors.New("protocol: authentication failed")
	ErrStale    = errors.New("protocol: stale or replayed packet")
)

// Header is the 6-byte prefix shared by every packet.
type Header struct {
	Version uint8
	Type    PacketType
	Seq     uint16
}

// PutHeader writes the header for t and seq into b[0:6].
func PutHeader(b []byte, t PacketType, seq uint16) {
	_ = b[HeaderSize-1]
	b[0], b[1] = Magic0, Magic1
	b[2] = Version
	b[3] = uint8(t)
	binary.LittleEndian.PutUint16(b[4:], seq)
}

// ParseHeader checks the magic and returns the header. It does not reject
// other versions, so the caller can answer them with REJECT; use
// CheckHeader to validate the version and length as well.
func ParseHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, ErrShort
	}
	if len(b) > MaxDatagram {
		return Header{}, ErrLong
	}
	if b[0] != Magic0 || b[1] != Magic1 {
		return Header{}, ErrMagic
	}
	return Header{Version: b[2], Type: PacketType(b[3]), Seq: binary.LittleEndian.Uint16(b[4:])}, nil
}

// CheckHeader parses the header and verifies the version, the type and that
// the datagram length matches the type (including its tag).
func CheckHeader(b []byte) (Header, error) {
	h, err := ParseHeader(b)
	if err != nil {
		return h, err
	}
	if h.Version < MinVersion || h.Version > MaxVersion {
		return h, ErrVersion
	}
	want := WireSize(h.Type)
	if h.Type == TypeDiscoverReply {
		if len(b) < DiscoverReplyMin || len(b) > DiscoverReplyMin+MaxServerNameSize {
			return h, ErrLength
		}
		return h, nil
	}
	if want == 0 {
		return h, ErrType
	}
	if len(b) != want {
		return h, ErrLength
	}
	return h, nil
}

// WireSize is the full datagram size for a fixed-size packet type,
// including its authentication tag, or 0 if unknown or variable.
func WireSize(t PacketType) int {
	n := t.BodySize()
	if n == 0 {
		return 0
	}
	switch {
	case t.SessionPacket():
		return n + TagSize
	case t == TypeHello || t == TypeWelcome:
		return n + HandshakeTagSize
	}
	return n
}

// ---------------------------------------------------------------------------
// Controller state (0x01)

// WireState is the fixed-width form of ControllerState in a STATE packet.
type WireState struct {
	Buttons        Buttons
	DPad           DPad
	Flags          StateFlags
	LeftX, LeftY   int16
	RightX, RightY int16
	LeftTrigger    uint8
	RightTrigger   uint8
}

// ToWire converts a normalized state to its wire form.
func (s *ControllerState) ToWire(flags StateFlags) WireState {
	return WireState{
		Buttons:      s.Buttons & ButtonsDefined,
		DPad:         s.DPad,
		Flags:        flags,
		LeftX:        EncodeAxis(s.LeftX),
		LeftY:        EncodeAxis(s.LeftY),
		RightX:       EncodeAxis(s.RightX),
		RightY:       EncodeAxis(s.RightY),
		LeftTrigger:  EncodeTrigger(s.LeftTrigger),
		RightTrigger: EncodeTrigger(s.RightTrigger),
	}
}

// Normalized converts a wire state to the canonical normalized state.
func (w *WireState) Normalized() ControllerState {
	return ControllerState{
		LeftX:        DecodeAxis(w.LeftX),
		LeftY:        DecodeAxis(w.LeftY),
		RightX:       DecodeAxis(w.RightX),
		RightY:       DecodeAxis(w.RightY),
		LeftTrigger:  DecodeTrigger(w.LeftTrigger),
		RightTrigger: DecodeTrigger(w.RightTrigger),
		Buttons:      w.Buttons,
		DPad:         w.DPad,
	}
}

// PutState encodes a STATE body into b[0:20].
func PutState(b []byte, seq uint16, w *WireState) {
	_ = b[StateSize-1]
	PutHeader(b, TypeState, seq)
	le := binary.LittleEndian
	le.PutUint16(b[6:], uint16(w.Buttons))
	b[8] = uint8(w.DPad)
	b[9] = uint8(w.Flags)
	le.PutUint16(b[10:], uint16(w.LeftX))
	le.PutUint16(b[12:], uint16(w.LeftY))
	le.PutUint16(b[14:], uint16(w.RightX))
	le.PutUint16(b[16:], uint16(w.RightY))
	b[18] = w.LeftTrigger
	b[19] = w.RightTrigger
}

// DecodeState decodes and validates the 20-byte STATE body at the start of
// b. Any tag after the body is ignored; the header must already have been
// checked by the caller or b must be exactly the body.
func DecodeState(b []byte) (seq uint16, w WireState, err error) {
	if len(b) < StateSize {
		return 0, w, ErrShort
	}
	if b[0] != Magic0 || b[1] != Magic1 {
		return 0, w, ErrMagic
	}
	if b[2] != Version {
		return 0, w, ErrVersion
	}
	if PacketType(b[3]) != TypeState {
		return 0, w, ErrType
	}
	le := binary.LittleEndian
	seq = le.Uint16(b[4:])
	w = WireState{
		Buttons:      Buttons(le.Uint16(b[6:])),
		DPad:         DPad(b[8]),
		Flags:        StateFlags(b[9]),
		LeftX:        int16(le.Uint16(b[10:])),
		LeftY:        int16(le.Uint16(b[12:])),
		RightX:       int16(le.Uint16(b[14:])),
		RightY:       int16(le.Uint16(b[16:])),
		LeftTrigger:  b[18],
		RightTrigger: b[19],
	}
	if w.Buttons&^ButtonsDefined != 0 || w.Flags&^stateFlagsDefined != 0 {
		return seq, w, ErrReserved
	}
	if !w.DPad.Valid() {
		return seq, w, ErrRange
	}
	return seq, w, nil
}

// ---------------------------------------------------------------------------
// Heartbeat (0x02)

// Heartbeat carries liveness, latency-echo and battery information. Times
// are the sender's own monotonic clock in microseconds (wrapping); they are
// only ever compared with timestamps from the same clock.
type Heartbeat struct {
	TimeUs  uint32 // sender's clock when sent
	EchoUs  uint32 // the peer's TimeUs from the last heartbeat received, or 0
	RttUs   uint32 // sender's latest measured round-trip time, or 0
	Battery uint8  // 0..100, or BatteryUnknown
	Flags   uint8  // HeartbeatCharging
}

const (
	BatteryUnknown    uint8 = 0xFF
	HeartbeatCharging uint8 = 1 << 0
)

// PutHeartbeat encodes a HEARTBEAT body into b[0:20].
func PutHeartbeat(b []byte, seq uint16, h *Heartbeat) {
	_ = b[HeartbeatSize-1]
	PutHeader(b, TypeHeartbeat, seq)
	le := binary.LittleEndian
	le.PutUint32(b[6:], h.TimeUs)
	le.PutUint32(b[10:], h.EchoUs)
	le.PutUint32(b[14:], h.RttUs)
	b[18] = h.Battery
	b[19] = h.Flags
}

// DecodeHeartbeat decodes a HEARTBEAT body whose header was already checked.
func DecodeHeartbeat(b []byte) (Heartbeat, error) {
	if len(b) < HeartbeatSize {
		return Heartbeat{}, ErrShort
	}
	le := binary.LittleEndian
	h := Heartbeat{
		TimeUs:  le.Uint32(b[6:]),
		EchoUs:  le.Uint32(b[10:]),
		RttUs:   le.Uint32(b[14:]),
		Battery: b[18],
		Flags:   b[19],
	}
	if h.Battery > 100 && h.Battery != BatteryUnknown {
		return h, ErrRange
	}
	if h.Flags&^HeartbeatCharging != 0 {
		return h, ErrReserved
	}
	return h, nil
}

// ---------------------------------------------------------------------------
// Disconnect (0x03)

// PutDisconnect encodes a DISCONNECT body into b[0:8].
func PutDisconnect(b []byte, seq uint16, r DisconnectReason) {
	_ = b[DisconnectSize-1]
	PutHeader(b, TypeDisconnect, seq)
	b[6] = uint8(r)
	b[7] = 0
}

// DecodeDisconnect decodes a DISCONNECT body whose header was already checked.
func DecodeDisconnect(b []byte) (DisconnectReason, error) {
	if len(b) < DisconnectSize {
		return 0, ErrShort
	}
	if b[7] != 0 {
		return 0, ErrReserved
	}
	r := DisconnectReason(b[6])
	if r > reasonMax {
		return r, ErrRange
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// Rumble (0x04)

// Rumble sets motor strengths on the phone. Zero for both motors stops
// vibration. DurationMs bounds how long the phone keeps vibrating without a
// refresh, so a lost stop command cannot leave the phone buzzing.
type Rumble struct {
	Left, Right uint16 // 0 = off, 0xFFFF = full strength
	DurationMs  uint16
}

// MaxRumbleMs caps DurationMs.
const MaxRumbleMs = 2000

// PutRumble encodes a RUMBLE body into b[0:12].
func PutRumble(b []byte, seq uint16, r *Rumble) {
	_ = b[RumbleSize-1]
	PutHeader(b, TypeRumble, seq)
	le := binary.LittleEndian
	le.PutUint16(b[6:], r.Left)
	le.PutUint16(b[8:], r.Right)
	le.PutUint16(b[10:], r.DurationMs)
}

// DecodeRumble decodes a RUMBLE body whose header was already checked.
func DecodeRumble(b []byte) (Rumble, error) {
	if len(b) < RumbleSize {
		return Rumble{}, ErrShort
	}
	le := binary.LittleEndian
	r := Rumble{Left: le.Uint16(b[6:]), Right: le.Uint16(b[8:]), DurationMs: le.Uint16(b[10:])}
	if r.DurationMs > MaxRumbleMs {
		return r, ErrRange
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// Discovery (0x10, 0x11)

// PutDiscover encodes a DISCOVER probe into b[0:6].
func PutDiscover(b []byte) { PutHeader(b, TypeDiscover, 0) }

// DiscoverReply advertises a receiver. It is unauthenticated: the phone
// uses it only to fill in an address, never as proof of identity.
type DiscoverReply struct {
	Port       uint16
	Caps       Caps
	MinVersion uint8
	MaxVersion uint8
	Name       string
}

// AppendDiscoverReply appends a DISCOVER_REPLY to b.
func AppendDiscoverReply(b []byte, r *DiscoverReply) []byte {
	name := r.Name
	if len(name) > MaxServerNameSize {
		name = name[:MaxServerNameSize]
	}
	var h [HeaderSize]byte
	PutHeader(h[:], TypeDiscoverReply, 0)
	b = append(b, h[:]...)
	b = binary.LittleEndian.AppendUint16(b, r.Port)
	b = binary.LittleEndian.AppendUint16(b, uint16(r.Caps))
	b = append(b, r.MinVersion, r.MaxVersion, uint8(len(name)))
	return append(b, name...)
}

// DecodeDiscoverReply decodes a DISCOVER_REPLY whose header was checked.
func DecodeDiscoverReply(b []byte) (DiscoverReply, error) {
	if len(b) < DiscoverReplyMin {
		return DiscoverReply{}, ErrShort
	}
	le := binary.LittleEndian
	r := DiscoverReply{
		Port:       le.Uint16(b[6:]),
		Caps:       Caps(le.Uint16(b[8:])),
		MinVersion: b[10],
		MaxVersion: b[11],
	}
	n := int(b[12])
	if n > MaxServerNameSize || len(b) != DiscoverReplyMin+n {
		return r, ErrLength
	}
	r.Name = string(b[13:])
	return r, nil
}

// ---------------------------------------------------------------------------
// Handshake (0x12, 0x13)

// Hello opens a session. It is sent by the phone and authenticated with the
// pairing key.
type Hello struct {
	DeviceID    uint64
	ClientNonce [NonceSize]byte
	MinVersion  uint8
	MaxVersion  uint8
	Caps        Caps
	DeviceName  [DeviceNameSize]byte // UTF-8, zero padded
}

// Name returns DeviceName without its zero padding.
func (h *Hello) Name() string {
	n := 0
	for n < len(h.DeviceName) && h.DeviceName[n] != 0 {
		n++
	}
	return string(h.DeviceName[:n])
}

// SetName stores s in DeviceName, truncating it at a UTF-8 boundary.
func (h *Hello) SetName(s string) {
	h.DeviceName = [DeviceNameSize]byte{}
	if len(s) > DeviceNameSize {
		s = s[:DeviceNameSize]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	copy(h.DeviceName[:], s)
}

// PutHello encodes a HELLO body into b[0:58].
func PutHello(b []byte, h *Hello) {
	_ = b[HelloSize-1]
	PutHeader(b, TypeHello, 0)
	le := binary.LittleEndian
	le.PutUint64(b[6:], h.DeviceID)
	copy(b[14:30], h.ClientNonce[:])
	b[30] = h.MinVersion
	b[31] = h.MaxVersion
	le.PutUint16(b[32:], uint16(h.Caps))
	copy(b[34:58], h.DeviceName[:])
}

// DecodeHello decodes a HELLO body whose header was already checked.
func DecodeHello(b []byte) (Hello, error) {
	if len(b) < HelloSize {
		return Hello{}, ErrShort
	}
	le := binary.LittleEndian
	var h Hello
	h.DeviceID = le.Uint64(b[6:])
	copy(h.ClientNonce[:], b[14:30])
	h.MinVersion = b[30]
	h.MaxVersion = b[31]
	h.Caps = Caps(le.Uint16(b[32:]))
	copy(h.DeviceName[:], b[34:58])
	if h.MinVersion > h.MaxVersion || h.MinVersion == 0 {
		return h, ErrRange
	}
	return h, nil
}

// Welcome accepts a HELLO and fixes the session parameters.
type Welcome struct {
	SessionID   uint32
	ServerNonce [NonceSize]byte
	Version     uint8 // negotiated protocol version
	Caps        Caps  // capabilities enabled for this session
	HeartbeatMs uint16
	TimeoutMs   uint16 // input watchdog: neutral state after this much silence
	MaxRateHz   uint16 // highest STATE rate the receiver wants
}

// PutWelcome encodes a WELCOME body into b[0:36].
func PutWelcome(b []byte, w *Welcome) {
	_ = b[WelcomeSize-1]
	PutHeader(b, TypeWelcome, 0)
	le := binary.LittleEndian
	le.PutUint32(b[6:], w.SessionID)
	copy(b[10:26], w.ServerNonce[:])
	b[26] = w.Version
	b[27] = 0
	le.PutUint16(b[28:], uint16(w.Caps))
	le.PutUint16(b[30:], w.HeartbeatMs)
	le.PutUint16(b[32:], w.TimeoutMs)
	le.PutUint16(b[34:], w.MaxRateHz)
}

// DecodeWelcome decodes a WELCOME body whose header was already checked.
func DecodeWelcome(b []byte) (Welcome, error) {
	if len(b) < WelcomeSize {
		return Welcome{}, ErrShort
	}
	le := binary.LittleEndian
	var w Welcome
	w.SessionID = le.Uint32(b[6:])
	copy(w.ServerNonce[:], b[10:26])
	w.Version = b[26]
	w.Caps = Caps(le.Uint16(b[28:]))
	w.HeartbeatMs = le.Uint16(b[30:])
	w.TimeoutMs = le.Uint16(b[32:])
	w.MaxRateHz = le.Uint16(b[34:])
	if b[27] != 0 {
		return w, ErrReserved
	}
	if w.Version < MinVersion || w.Version > MaxVersion || w.HeartbeatMs == 0 || w.TimeoutMs == 0 || w.MaxRateHz == 0 {
		return w, ErrRange
	}
	return w, nil
}

// ---------------------------------------------------------------------------
// Reject (0x14)

// Reject refuses a HELLO or answers a packet with an unsupported version.
type Reject struct {
	Reason     RejectReason
	MinVersion uint8
	MaxVersion uint8
}

// PutReject encodes a REJECT into b[0:10].
func PutReject(b []byte, r *Reject) {
	_ = b[RejectSize-1]
	PutHeader(b, TypeReject, 0)
	b[6] = uint8(r.Reason)
	b[7] = r.MinVersion
	b[8] = r.MaxVersion
	b[9] = 0
}

// DecodeReject decodes a REJECT whose header was already checked.
func DecodeReject(b []byte) (Reject, error) {
	if len(b) < RejectSize {
		return Reject{}, ErrShort
	}
	r := Reject{Reason: RejectReason(b[6]), MinVersion: b[7], MaxVersion: b[8]}
	if r.Reason == 0 || r.Reason > rejectMax {
		return r, ErrRange
	}
	return r, nil
}
