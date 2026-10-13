package session

import (
	"net/netip"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// bucket is a token bucket refilled at rate tokens/s up to burst.
type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) take(now time.Time, rate, burst float64) bool {
	if b.last.IsZero() {
		b.tokens = burst
	} else {
		b.tokens = min(burst, b.tokens+now.Sub(b.last).Seconds()*rate)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type peer struct {
	hello, bad, reply bucket
	blockedUntil      time.Time
	seen              time.Time
}

const (
	helloRate, helloBurst = 2, 5   // handshakes per second per IP
	badRate, badBurst     = 20, 50 // malformed or forged packets per second per IP
	replyRate, replyBurst = 1, 3   // unauthenticated replies per second per IP
	globalReplyRate       = 20     // unauthenticated replies per second overall
	blockFor              = 10 * time.Second
	maxPeers              = 4096
)

// limiter rate-limits handshakes, unauthenticated replies and bad packets
// per source IP. It is only used with the server mutex held.
type limiter struct {
	peers       map[netip.Addr]*peer
	globalReply bucket
}

func (l *limiter) get(ip netip.Addr, now time.Time) *peer {
	if l.peers == nil {
		l.peers = make(map[netip.Addr]*peer)
	}
	p := l.peers[ip]
	if p == nil {
		if len(l.peers) >= maxPeers {
			l.prune(now)
		}
		p = &peer{}
		l.peers[ip] = p
	}
	p.seen = now
	return p
}

func (l *limiter) prune(now time.Time) {
	for ip, p := range l.peers {
		if now.Sub(p.seen) > time.Minute && now.After(p.blockedUntil) {
			delete(l.peers, ip)
		}
	}
	if len(l.peers) >= maxPeers {
		// Under a spoofed-source flood, forget everything rather than grow.
		clear(l.peers)
	}
}

func (l *limiter) blocked(ip netip.Addr, now time.Time) bool {
	p := l.peers[ip]
	return p != nil && now.Before(p.blockedUntil)
}

// bad records a malformed or unauthenticated packet and reports whether
// the source just became blocked.
func (l *limiter) bad(ip netip.Addr, now time.Time) bool {
	p := l.get(ip, now)
	if p.bad.take(now, badRate, badBurst) {
		return false
	}
	p.blockedUntil = now.Add(blockFor)
	return true
}

func (l *limiter) allowHello(ip netip.Addr, now time.Time) bool {
	return l.get(ip, now).hello.take(now, helloRate, helloBurst)
}

// allowReply limits unauthenticated replies, so a spoofed source cannot
// use the receiver to reflect traffic.
func (l *limiter) allowReply(ip netip.Addr, now time.Time) bool {
	return l.get(ip, now).reply.take(now, replyRate, replyBurst) &&
		l.globalReply.take(now, globalReplyRate, globalReplyRate)
}

// nonceCache remembers recent HELLO nonces so a captured HELLO cannot be
// replayed to open a pending session.
type nonceCache struct {
	ring [256][protocol.NonceSize]byte
	n    int
}

func (c *nonceCache) seenOrAdd(nonce [protocol.NonceSize]byte) bool {
	for i := 0; i < min(c.n, len(c.ring)); i++ {
		if c.ring[i] == nonce {
			return true
		}
	}
	c.ring[c.n%len(c.ring)] = nonce
	c.n++
	return false
}
