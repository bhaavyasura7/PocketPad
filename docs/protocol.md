# PocketPad Wi-Fi protocol, version 1

This document is normative. The test vectors in
[`protocol/testvectors/v1.json`](../protocol/testvectors/v1.json) are generated
from the Go reference implementation and must pass on every implementation.

## Transport

- UDP, default port **47770**. One datagram is one packet. No packet is larger
  than 128 bytes.
- Every multibyte field is **little-endian**.
- The phone is the *client*; the desktop receiver is the *server*.

## Common header (6 bytes)

| Offset | Size | Field            | Value                       |
|-------:|-----:|------------------|-----------------------------|
| 0      | 2    | Magic            | `'P' 'P'` (`50 50`)         |
| 2      | 1    | Protocol version | `1`                         |
| 3      | 1    | Packet type      | see below                   |
| 4      | 2    | Sequence number  | per-direction, wraps at 2¹⁶ |

| Type   | Name           | Direction     | Authenticated by       | Size on the wire |
|--------|----------------|---------------|------------------------|-----------------:|
| `0x01` | STATE          | phone → desk  | session key (8-byte tag) | 20 + 8         |
| `0x02` | HEARTBEAT      | both          | session key            | 20 + 8           |
| `0x03` | DISCONNECT     | both          | session key            | 8 + 8            |
| `0x04` | RUMBLE         | desk → phone  | session key            | 12 + 8           |
| `0x10` | DISCOVER       | phone → desk  | none                   | 6                |
| `0x11` | DISCOVER_REPLY | desk → phone  | none                   | 13 + name        |
| `0x12` | HELLO          | phone → desk  | pairing key (16-byte tag) | 58 + 16       |
| `0x13` | WELCOME        | desk → phone  | pairing key            | 36 + 16          |
| `0x14` | REJECT         | desk → phone  | none                   | 10               |

A receiver drops any datagram whose length does not exactly match its type,
whose magic is wrong, whose type is unknown or whose reserved bits are set.

## Controller state (STATE, 20-byte body)

| Offset | Size | Field        | Encoding                                   |
|-------:|-----:|--------------|--------------------------------------------|
| 0      | 6    | Header       | type `0x01`                                |
| 6      | 2    | Button mask  | bits below; bits 11–15 reserved (zero)     |
| 8      | 1    | D-pad        | 0 neutral, 1 N, 2 NE, 3 E, 4 SE, 5 S, 6 SW, 7 W, 8 NW |
| 9      | 1    | Flags        | bit 0 = snapshot (unchanged resend); others reserved |
| 10     | 2    | Left X       | int16                                      |
| 12     | 2    | Left Y       | int16                                      |
| 14     | 2    | Right X      | int16                                      |
| 16     | 2    | Right Y      | int16                                      |
| 18     | 1    | Left trigger | uint8                                      |
| 19     | 1    | Right trigger| uint8                                      |

Buttons: bit 0 A, 1 B, 2 X, 3 Y, 4 LB, 5 RB, 6 Back, 7 Start, 8 left-stick
click, 9 right-stick click, 10 Guide.

**Axes** are normalized to [-1, 1] internally, with +X right and **+Y up**
(stick pushed away from the player). Encoding: `round_half_away_from_zero(v ×
32767)`, clamped to ±32767; NaN encodes as 0. Decoding: `v / 32767`, with
-32768 treated as -1.

**Triggers** are normalized to [0, 1]. Encoding: `round_half_away_from_zero(v ×
255)`, clamped; decoding: `v / 255`.

Golden packet (seq `0x0102`, A + Start, D-pad E, LX = 1, LY = -1, RY = 16384,
LT = 255), before its tag:

```
50 50 01 01 02 01 81 00 03 00 ff 7f 01 80 00 00 00 40 ff 00
```

## Pairing

The receiver holds a 120-bit random **pairing secret**, shown to the user as
24 Crockford base32 characters in groups of four
(`pocketpad pair`, e.g. `000G-40R4-0M30-E209-185G-R38E`). Parsers accept any case,
ignore `-` and whitespace, and read `O` as `0` and `I`/`L` as `1`.

```
pairKey = HMAC-SHA256(key = "PocketPad v1 pairing key", msg = secret)
```

Anyone holding the code can drive the receiver, so the receiver never logs it,
and a new code (`pocketpad pair -reset`) revokes every phone.

## Session establishment

```
phone                                      receiver
  | -- HELLO (fresh client nonce) ---------> |  verify tag, check version & busy
  | <------------- WELCOME (server nonce) -- |  session is *pending*
  | -- first session packet --------------> |  tag proves key: session *active*
```

**HELLO** body (58 bytes): header (seq 0) · device ID u64 · client nonce
16 bytes · min version u8 · max version u8 · capabilities u16 · device name 24
bytes (UTF-8, zero padded, truncated at a character boundary). Tag:
`HMAC-SHA256(pairKey, body)[:16]`.

**WELCOME** body (36 bytes): header (seq 0) · session ID u32 · server nonce
16 bytes · negotiated version u8 · reserved u8 (0) · capabilities u16 ·
heartbeat interval ms u16 · input watchdog ms u16 · maximum STATE rate Hz u16.
Tag: `HMAC-SHA256(pairKey, body ‖ client nonce)[:16]`, binding it to its HELLO.

**REJECT** (10 bytes): header · reason u8 (1 unsupported version, 2 another
controller is connected, 3 rate limited) · receiver min version u8 · max
version u8 · reserved u8. Its layout is frozen across versions, so an old
receiver can always explain a mismatch to a newer phone. The receiver answers
a HELLO or DISCOVER of a foreign version with REJECT(1).

Capabilities: bit 0 rumble, 1 battery, 2 Guide button, 3 latency echo. The
WELCOME carries the intersection.

Rules:
- The phone uses a fresh random nonce for every HELLO, including retries;
  the receiver drops repeated nonces.
- A HELLO with a bad tag is silently dropped (and rate-limited), so a phone
  that hears nothing back should suggest checking the pairing code.
- The receiver drives one controller in version 1. A HELLO from a different
  device while a session is live gets REJECT(2). A HELLO from the *same*
  device creates a pending session that replaces the active one only once its
  first authenticated packet arrives, so a forged or replayed HELLO cannot
  interrupt play.

Session keys:

```
master = HMAC-SHA256(pairKey, "PocketPad v1 session" ‖ clientNonce ‖ serverNonce ‖ deviceID u64 ‖ sessionID u32)
c2s    = HMAC-SHA256(master, "c2s")    phone → receiver
s2c    = HMAC-SHA256(master, "s2c")    receiver → phone
```

## Session packets, sequence numbers and replay protection

Each direction numbers its packets from 0 with one counter shared by all
session packet types. The 16-bit header field is the low half of a 64-bit
**extended sequence number**; the rest is an implicit rollover counter (ROC)
that is never sent but is authenticated:

```
tag = HMAC-SHA256(directionKey, ROC u32 ‖ body)[:8]
```

The receiver estimates the extended number closest to the highest one it has
accepted (`delta = int16(seq − highest.low16)`; `delta ≤ 0` is stale), verifies
the tag with that ROC and only then advances. Consequences:

- Late, duplicated and replayed packets are dropped; for controller state
  the newest packet always wins, so there is no reorder buffer.
- A packet captured before a wraparound fails authentication after it.
- Packets from an earlier session fail authentication because the keys differ.
- Up to 32 767 consecutive losses are recovered without resynchronisation.

**HEARTBEAT** body (20 bytes): header · sender time µs u32 (the sender's own
monotonic clock, wrapping) · echo µs u32 (the peer's last time value, or 0) ·
RTT µs u32 (the sender's latest measurement, or 0) · battery u8 (0–100, 255
unknown) · flags u8 (bit 0 charging). The phone sends one every heartbeat
interval; the receiver echoes each one immediately. The phone computes
`RTT = now − echo` on its own clock. Times from different devices are never
compared with each other.

**DISCONNECT** body (8 bytes): header · reason u8 (0 user, 1 shutdown,
2 timeout, 3 replaced) · reserved u8.

**RUMBLE** body (12 bytes): header · left motor u16 · right motor u16
(0 off – 65535 full) · duration ms u16 (≤ 2000). The phone stops after
the duration unless refreshed, so a lost stop command cannot leave it
vibrating. The receiver coalesces changes to at most one every 20 ms,
refreshes a running motor every 500 ms with a 750 ms duration, and sends
`0, 0, 0` to stop.

## Sending rules (phone)

- On any input change, send STATE at once, at most at the WELCOME's maximum
  rate (coalesce faster changes; only the latest state matters).
- While anything is held, resend the state with the snapshot flag at least
  every 50 ms, well inside the receiver's watchdog.
- When everything is released and unchanged, send no STATE at all; only
  heartbeats continue.
- On leaving the controller screen, send a neutral STATE and then DISCONNECT.
- If no authenticated packet arrives for 3 s, consider the session lost
  and reconnect with a new HELLO.

## Receiver safety rules

- **Input watchdog:** while any input is held, if no STATE arrives within the
  watchdog interval (default 300 ms) every input is released. The session
  stays up; the next STATE restores input.
- **Session timeout:** with no authenticated packet for 3 s, the session ends,
  every input is released and the phone is sent DISCONNECT(2).
- A new or replaced session always starts with every input released.
- Per source IP: at most 2 handshakes/s (burst 5); after 50 invalid packets
  at more than 20/s the source is ignored for 10 s; unauthenticated replies
  (DISCOVER_REPLY, REJECT) are limited to 1/s (burst 3) per IP and 20/s
  overall, so the receiver cannot be used to reflect traffic. The connected
  phone's address is never blocked.

## Discovery

The phone broadcasts DISCOVER (6 bytes) to `255.255.255.255:47770`. Receivers
answer with DISCOVER_REPLY: header · port u16 · capabilities u16 · min
version u8 · max version u8 · name length u8 (≤ 32) · name (UTF-8). Discovery
is unauthenticated and only fills in an address; the handshake proves the
receiver knows the pairing secret.

## Development mode

`pocketpad serve -insecure` additionally accepts bare 20-byte STATE packets
without a tag, from a single source at a time, ordered by `SeqNewer`. It
exists for bring-up and must never be the default.
