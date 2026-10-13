# PocketPad

Turn an Android phone into a low-latency game controller for macOS and Windows.

- **Wi-Fi mode:** the phone sends compact, authenticated UDP packets to a small
  desktop receiver, which drives an operating-system-visible virtual gamepad.
- **Bluetooth mode** (planned): the phone acts as a Bluetooth HID gamepad
  directly, on phones whose Android build supports it.

No accounts, no cloud, and nothing leaves your local network.

## Status

| Milestone | State |
|---|---|
| 1. Shared protocol (codec, test vectors, authentication) | ✅ done, tested |
| 2. Desktop receiver (validation, sessions, watchdog, diagnostics) | ✅ done, tested on Linux; CI runs macOS and Windows |
| 3. Android Wi-Fi app | ⏳ protocol module written (`android/protocol`); app to do |
| 4. Windows virtual gamepad backend | 🧪 written (ViGEmBus, Xbox 360 pad + rumble); needs testing on a real PC |
| 5. macOS virtual gamepad backend | ⏳ planned (needs testing on a real Mac) |
| 6. Bluetooth HID | ⏳ planned (needs testing on a real phone) |
| 7. Rumble on the phone, battery, measurements | ⏳ protocol and receiver side done |

**Remaining work is specified step by step in [docs/HANDOFF.md](docs/HANDOFF.md).**

## Try the receiver

Requires Go 1.24+.

```sh
cd desktop
go run ./cmd/pocketpad pair            # shows the pairing code
go run ./cmd/pocketpad serve -backend mock
```

`-backend mock` drives an in-memory gamepad and shows the live controller
state in the terminal. On Windows, the default backend (`vigem`) creates a
virtual Xbox 360 controller; it needs [ViGEmBus](https://github.com/nefarius/ViGEmBus/releases)
installed and `ViGEmClient.dll` next to `pocketpad.exe` (see docs/HANDOFF.md §4.1). In another terminal,
simulate a phone:

```sh
go run ./cmd/ppsim -addr 127.0.0.1:47770            # reads the local pairing code
go run ./cmd/ppsim -addr 192.168.1.20:47770 -code XXXX-XXXX-XXXX-XXXX-XXXX-XXXX -pattern buttons
```

Patterns: `mixed` (default), `circle`, `buttons` (walks every button and D-pad
direction), `hold`, `idle`.

Useful `serve` flags: `-watchdog 300ms`, `-session-timeout 3s`, `-rate 250`,
`-diag live|log|off`, `-v`. Run `pocketpad serve -h` for all of them.

Allow UDP port 47770 through the desktop firewall for the phone to reach the
receiver. On first run, Windows and macOS usually ask about this.

## Layout

```
docs/protocol.md            normative wire protocol
protocol/testvectors/       vectors shared by the Go and Kotlin implementations
desktop/                    Go receiver
  cmd/pocketpad             the receiver CLI
  cmd/ppsim                 phone simulator
  internal/protocol         state model, codec, sequence numbers, authentication
  internal/session          receiver: handshake, validation, watchdog, rumble
  internal/gamepad          backend interface, mock, Windows ViGEm backend
  internal/client           Go implementation of the phone side
  internal/config           pairing secret storage
  internal/diag             live diagnostic display
android/protocol/           Kotlin protocol library for the app (tested against the shared vectors)
```

## Design notes

- **Packets:** controller state is a 20-byte little-endian packet plus an
  8-byte truncated HMAC-SHA256 tag. The receiver never trusts a packet because
  it comes from the local network. See [docs/protocol.md](docs/protocol.md).
- **Pairing:** the receiver generates a 120-bit secret, shown as a 24-character
  code. Each connection derives fresh per-direction session keys, so packets
  from an old session or the opposite direction never validate.
- **Safety:** an input watchdog releases held controls when input stops, a
  session timeout drops dead connections, and every new session starts with
  everything released. Lost release events cannot leave buttons stuck.
- **Latency and resources:** the receiver keeps only the latest state, with no
  queues, and sealing or verifying a packet does not allocate (about 0.4 µs).
  With nothing held, the phone sends no input packets, only a heartbeat
  every 500 ms.
- **Gamepad interface:** `internal/gamepad.Gamepad` follows the specification,
  with one change. Because rumble flows from the game to the phone, the
  backend reports it through `SetRumbleHandler` rather than accepting
  `SetRumble`. `Capabilities` lets the session negotiate features.

## Development

```sh
cd desktop
go vet ./... && go test -race ./...
go test ./internal/protocol -run TestVectors -update   # after an intentional wire change
go test -run XXX -fuzz FuzzDecoders ./internal/protocol
```

CI runs the tests on Linux, macOS and Windows and uploads receiver binaries for
each platform.
