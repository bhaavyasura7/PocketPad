# PocketPad

<img src="PocketPad.png" alt="PocketPad official logo" width="150">

Turn an Android phone into a low-latency game controller for macOS and Windows.

**New here? → [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md)** walks you
from zero to playing in one pass: one command on the computer, one APK on
the phone, the pairing code is printed right in the receiver banner.

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
| 3. Android Wi-Fi app | ✅ done (`android/app`); unit tests, lint and CI, verified end-to-end on a real phone (motorola edge 70 fusion): all 21 controls, 0 lost packets |
| 4. Windows virtual gamepad backend | 🧪 written (ViGEmBus, Xbox 360 pad + rumble); needs testing on a real PC |
| 5. macOS backends | ✅ `machid` (real HID gamepad; macOS 15 and earlier with `sudo`, macOS 26 needs Apple's entitlement) and `mackeys` (keyboard/mouse control with game profiles: `general`, `racing`, `gta`). `serve` picks automatically |
| 6. Bluetooth HID | 🧪 written (report descriptor + transport, unit-tested mapping). Many phones ship without the Bluetooth HID **Device** profile (e.g. motorola edge 70 fusion: HID_HOST only); the app now detects that and says so after a 5 s timeout. Needs a phone that has the profile to validate |
| 7. Rumble on the phone, battery, indicators | ✅ phone side done: amplitude rumble, battery in every heartbeat, RTT/rate/battery overlay |

**Remaining work is specified step by step in [docs/HANDOFF.md](docs/HANDOFF.md).**

## Try the receiver

Requires Go 1.24+.

```sh
cd desktop
go run ./cmd/pocketpad serve      # picks the best backend for your OS
```

The banner shows the LAN address and the **pairing code** for the phone — no
second command needed. On Windows install ViGEmBus for a real rumble-capable
controller; on macOS 26 grant Accessibility once and the receiver controls
the Mac (profiles: `-profile general|racing|gta`); on this Mac also
`sudo ./pocketpad serve` for a real HID gamepad. Diagnostics-only anywhere:
`-backend mock`. In another terminal, simulate a phone:

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

### macOS

```sh
cd desktop
go build -o pocketpad ./cmd/pocketpad
sudo ./pocketpad serve          # "machid": a real virtual controller (macOS 15 and earlier)
./pocketpad serve -backend mock # diagnostics only, no password
```

macOS has no unprivileged way to create a virtual controller. On macOS 15
and earlier the `machid` backend works with `sudo`; **on macOS 26 Apple
enforces the restricted `com.apple.developer.hid.virtual.device` entitlement
for root too**, so the software backend there needs an entitlement Apple
grants case by case. Two working options on a Mac without it:

- **Control the Mac with keyboard and mouse (`mackeys`):** the phone drives
  the Mac directly — sticks, d-pad and buttons become keys and pointer
  movement, which works in keyboard/mouse games and for the desktop itself.
  It needs the one-time Accessibility approval: run it, and when macOS asks
  (or in System Settings → Privacy & Security → Accessibility) enable the
  app that runs it (Terminal, or `pocketpad` itself).

  ```sh
  ./pocketpad serve -backend mackeys
  ```

  | Phone | Mac | Phone | Mac |
  |---|---|---|---|
  | Left stick | W A S D | D-pad | arrow keys |
  | Right stick | mouse look | LT / RT | left / right click |
  | A | Space | B | Left Shift |
  | X | E | Y | Q |
  | LB | Tab | RB | Escape |
  | Back | M | Start | Return |
  | Guide | F1 | LS / RS | C / R |

- **Diagnostics:** `./pocketpad serve -backend mock` shows every input live
  in the terminal (also what the phone pairs to for Wi-Fi testing).

Rumble is not available in macOS modes.

Rumble is not available in macOS Wi-Fi mode (a generic HID pad has no rumble
channel); it works through the Windows ViGEm backend.

### Android app

Requires JDK 17 and the Android SDK (platform 35).

```sh
cd android
./gradlew :app:assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

The app auto-discovers receivers on the local network; enter the pairing code
from `pocketpad pair` (saved per computer afterwards) and tap Connect. The
controller screen opens in landscape; the top-centre chip shows the live
connection, RTT, rate and phone battery. Settings (gear icon) cover dead
zone, analog triggers, update rate, haptics, vibration, layout size and
left-handed mode.

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
