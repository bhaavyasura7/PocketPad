package protocol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"hash"
	"strings"
)

// ---------------------------------------------------------------------------
// Sequence numbers

// SeqNewer reports whether a is newer than b, accounting for 16-bit
// wraparound (RFC 1982 serial arithmetic).
func SeqNewer(a, b uint16) bool { return int16(a-b) > 0 }

// ExtSeq is a 64-bit extended sequence number: the 16-bit wire sequence in
// the low bits and an implicit rollover counter above it. The rollover
// counter is never transmitted but is covered by the packet tag, so a
// packet captured before a wraparound cannot be replayed after it.
type ExtSeq uint64

// Wire returns the 16-bit sequence number carried in the header.
func (e ExtSeq) Wire() uint16 { return uint16(e) }

// ROC returns the rollover counter.
func (e ExtSeq) ROC() uint32 { return uint32(e >> 16) }

// SeqCounter allocates outgoing sequence numbers. The first is 0.
type SeqCounter struct{ next ExtSeq }

// Next returns the next sequence number and advances the counter.
func (c *SeqCounter) Next() ExtSeq {
	e := c.next
	c.next++
	return e
}

// ReplayWindow tracks the highest accepted sequence number of one peer.
// Only strictly newer packets are accepted: the receiver keeps the latest
// state, so a late packet is as useless as a replayed one.
type ReplayWindow struct {
	highest ExtSeq
	has     bool
}

// Estimate maps a wire sequence number to the extended sequence number
// closest to the highest one accepted so far. fresh is false when the packet
// is not newer than everything already accepted.
func (w *ReplayWindow) Estimate(seq uint16) (ext ExtSeq, fresh bool) {
	if !w.has {
		return ExtSeq(seq), true
	}
	delta := int16(seq - w.highest.Wire())
	if delta <= 0 {
		return 0, false
	}
	return w.highest + ExtSeq(delta), true
}

// Accept records ext as accepted and returns how many sequence numbers were
// skipped since the previous accepted packet (lost or reordered).
func (w *ReplayWindow) Accept(ext ExtSeq) (gap uint64) {
	if w.has && ext > w.highest {
		gap = uint64(ext - w.highest - 1)
	}
	if !w.has || ext > w.highest {
		w.highest, w.has = ext, true
	}
	return gap
}

// ---------------------------------------------------------------------------
// Pairing codes

// PairingCodeBytes is the entropy in a pairing code: 120 bits.
const PairingCodeBytes = 15

// Crockford base32: no I, L, O or U, so codes survive being read aloud or
// typed on a phone keyboard.
var codeEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

var ErrPairingCode = errors.New("protocol: invalid pairing code")

// NewPairingSecret returns fresh random pairing-secret bytes.
func NewPairingSecret() ([]byte, error) {
	b := make([]byte, PairingCodeBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// FormatPairingCode renders a secret as 6 groups of 4 characters.
func FormatPairingCode(secret []byte) string {
	s := codeEncoding.EncodeToString(secret)
	var sb strings.Builder
	for i := 0; i < len(s); i += 4 {
		if i > 0 {
			sb.WriteByte('-')
		}
		end := min(i+4, len(s))
		sb.WriteString(s[i:end])
	}
	return sb.String()
}

// ParsePairingCode accepts a code in any case, with or without separators,
// and with the usual Crockford substitutions (O→0, I and L→1).
func ParsePairingCode(code string) ([]byte, error) {
	var sb strings.Builder
	for _, r := range strings.ToUpper(code) {
		switch r {
		case '-', ' ', '\t':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		sb.WriteRune(r)
	}
	b, err := codeEncoding.DecodeString(sb.String())
	if err != nil || len(b) != PairingCodeBytes {
		return nil, ErrPairingCode
	}
	return b, nil
}

// ---------------------------------------------------------------------------
// Keys and tags

const KeySize = 32

func hmacSum(key []byte, parts ...[]byte) []byte {
	m := hmac.New(sha256.New, key)
	for _, p := range parts {
		m.Write(p)
	}
	return m.Sum(nil)
}

// PairKey derives the long-term handshake key from pairing-secret bytes.
func PairKey(secret []byte) []byte {
	return hmacSum([]byte("PocketPad v1 pairing key"), secret)
}

// HandshakeTag computes the 16-byte tag for a HELLO (bound = nil) or a
// WELCOME (bound = the client nonce of the HELLO it answers).
func HandshakeTag(pairKey, body, bound []byte) []byte {
	return hmacSum(pairKey, body, bound)[:HandshakeTagSize]
}

// VerifyHandshake checks the tag at the end of a HELLO or WELCOME datagram.
func VerifyHandshake(pairKey, pkt, bound []byte) bool {
	n := len(pkt) - HandshakeTagSize
	if n < HeaderSize {
		return false
	}
	return hmac.Equal(HandshakeTag(pairKey, pkt[:n], bound), pkt[n:])
}

// SessionKeys are the per-direction keys of one session. Using separate
// keys means a packet cannot be reflected back to its sender.
type SessionKeys struct {
	ClientToServer []byte
	ServerToClient []byte
}

// DeriveSessionKeys derives the session keys from the handshake values.
func DeriveSessionKeys(pairKey []byte, deviceID uint64, clientNonce, serverNonce [NonceSize]byte, sessionID uint32) SessionKeys {
	var ids [12]byte
	binary.LittleEndian.PutUint64(ids[0:], deviceID)
	binary.LittleEndian.PutUint32(ids[8:], sessionID)
	master := hmacSum(pairKey, []byte("PocketPad v1 session"), clientNonce[:], serverNonce[:], ids[:])
	return SessionKeys{
		ClientToServer: hmacSum(master, []byte("c2s")),
		ServerToClient: hmacSum(master, []byte("s2c")),
	}
}

// SessionMAC computes session-packet tags for one direction. It reuses its
// buffers, so tagging does not allocate. It is not safe for concurrent use.
type SessionMAC struct {
	h   hash.Hash
	roc [4]byte
	sum [sha256.Size]byte
}

// NewSessionMAC returns a SessionMAC for key.
func NewSessionMAC(key []byte) *SessionMAC {
	return &SessionMAC{h: hmac.New(sha256.New, key)}
}

func (m *SessionMAC) tag(body []byte, ext ExtSeq) []byte {
	m.h.Reset()
	binary.LittleEndian.PutUint32(m.roc[:], ext.ROC())
	m.h.Write(m.roc[:])
	m.h.Write(body)
	return m.h.Sum(m.sum[:0])[:TagSize]
}

// Seal writes the tag for pkt[:bodyLen] into pkt[bodyLen:bodyLen+TagSize].
// The header of the body must already carry ext.Wire().
func (m *SessionMAC) Seal(pkt []byte, bodyLen int, ext ExtSeq) {
	copy(pkt[bodyLen:bodyLen+TagSize], m.tag(pkt[:bodyLen], ext))
}

// Verify checks the tag at the end of pkt for the extended sequence ext.
func (m *SessionMAC) Verify(pkt []byte, ext ExtSeq) bool {
	n := len(pkt) - TagSize
	if n < HeaderSize {
		return false
	}
	return hmac.Equal(m.tag(pkt[:n], ext), pkt[n:])
}

// Sealer authenticates outgoing session packets for one direction.
type Sealer struct {
	mac *SessionMAC
	seq SeqCounter
}

// NewSealer returns a Sealer for key.
func NewSealer(key []byte) *Sealer { return &Sealer{mac: NewSessionMAC(key)} }

// Next allocates the sequence number for the next packet.
func (s *Sealer) Next() ExtSeq { return s.seq.Next() }

// Seal writes the tag after the bodyLen-byte body in pkt and returns the
// full datagram.
func (s *Sealer) Seal(pkt []byte, bodyLen int, ext ExtSeq) []byte {
	s.mac.Seal(pkt, bodyLen, ext)
	return pkt[:bodyLen+TagSize]
}

// Opener verifies incoming session packets for one direction and rejects
// stale and replayed ones.
type Opener struct {
	mac *SessionMAC
	win ReplayWindow
}

// NewOpener returns an Opener for key.
func NewOpener(key []byte) *Opener { return &Opener{mac: NewSessionMAC(key)} }

// Open authenticates a session datagram (header already checked). On
// success it returns the number of sequence numbers skipped since the last
// accepted packet.
func (o *Opener) Open(pkt []byte) (gap uint64, err error) {
	if len(pkt) < HeaderSize+TagSize {
		return 0, ErrShort
	}
	ext, fresh := o.win.Estimate(binary.LittleEndian.Uint16(pkt[4:]))
	if !fresh {
		return 0, ErrStale
	}
	if !o.mac.Verify(pkt, ext) {
		return 0, ErrAuth
	}
	return o.win.Accept(ext), nil
}
