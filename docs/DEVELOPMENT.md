# Development guide

This document describes how PocketPad is built, how the parts fit together,
and the rules every change must keep. The wire format itself is specified in
[protocol.md](protocol.md).

## Contents

1. [Architecture](#architecture)
2. [Building and testing](#building-and-testing)
3. [Receiver](#receiver)
4. [Gamepad backends](#gamepad-backends)
5. [Android app](#android-app)
6. [Bluetooth HID report](#bluetooth-hid-report)
7. [Design rules](#design-rules)
8. [Changing the protocol](#changing-the-protocol)
9. [Releasing](#releasing)

## Architecture

```
 Android phone                                  Computer
+--------------------------+   Wi-Fi (UDP)    +-------------------------------+
| ControllerView (touch)   |  20-byte state   | pocketpad receiver            |
|   -> InputState          |  + 8-byte HMAC   |   session manager             |
|   -> WifiTransport  -----+----------------->|   -> gamepad backend          |
|      (sender, receiver)  |<-----------------+      vigem | machid | mackeys  |
|   <- Rumbler             |  heartbeat echo, |                               |
|                          |  rumble          +-------------------------------+
|   BluetoothHidTransport -+---- Bluetooth HID ----> operating system directly
+--------------------------+
```

- **Protocol.** One versioned binary protocol, implemented twice: in Go
  (`desktop/internal/protocol`) and in Kotlin (`android/protocol`). Both are
  tested against the same vectors in `protocol/testvectors/v1.json`.
- **Receiver.** A Go program with no runtime dependencies besides the QR code
  library used by the macOS window. It authenticates the phone, keeps only
  the latest controller state and drives a gamepad backend.
- **Backends.** Each backend turns controller state into something the
  operating system sees: a virtual Xbox 360 pad (Windows), a virtual HID
  gamepad (macOS 15 and earlier), or keyboard and mouse events (macOS 26).
- **Phone app.** Kotlin with Jetpack Compose for screens, and a custom
  `View` for the controller surface. Wi-Fi and Bluetooth are two
  implementations of one `Transport` interface.

## Building and testing

### Receiver (Go 1.24 or later)

```sh
cd desktop
go vet ./...
go test -race ./...
go build -o pocketpad ./cmd/pocketpad
```

Useful during development:

```sh
go run ./cmd/pocketpad serve -backend mock                # live input display, no OS integration
go run ./cmd/pocketpad serve -backend mock -test-rumble   # pulses rumble to test phone vibration
go run ./cmd/ppsim -addr 127.0.0.1:47770 -pattern buttons # simulated phone
go test -run XXX -fuzz FuzzDecoders -fuzztime 30s ./internal/protocol
```

`ppsim` patterns: `mixed` (default), `circle`, `buttons`, `hold`, `idle`.
Without `-code`, it reads the local receiver's pairing code; with
`-insecure` it sends bare unauthenticated packets to a receiver started with
`-insecure`.

The macOS backends and the `pocketpad app` window use cgo and build only on
macOS. `./scripts/build-mac-app.sh` produces `dist/PocketPad.app`, signed
with the local "PocketPad Dev" identity when `./scripts/make-signing-cert.sh`
has created it. A stable signature keeps the Accessibility permission across
rebuilds.

### Android (JDK 17, Android SDK platform 36)

```sh
cd android
./gradlew :app:assembleDebug :app:testDebugUnitTest :app:lintDebug
```

The protocol library is a separate Gradle build with no Android
dependencies, included by the app as a composite build:

```sh
cd android/protocol
./gradlew test
```

### Continuous integration

`.github/workflows/ci.yml` runs on every push:

| Job | What it does |
|---|---|
| Desktop (Linux, macOS, Windows) | `gofmt` check (Linux), `go vet`, `go test -race`, 30 s of fuzzing (Linux), and receiver plus `ppsim` binaries uploaded as artifacts |
| Kotlin protocol | Runs the Kotlin protocol tests against the shared vectors |
| Android app | Builds the debug APK, runs unit tests and lint, uploads the APK |

`.github/workflows/release.yml` builds and publishes releases; see
[Releasing](#releasing).

## Receiver

The receiver core is `desktop/internal/session/server.go`. One goroutine
reads datagrams; timers handle the watchdog, session expiry and rumble.

- **Handshake.** HELLO is verified with the pairing key and creates a
  pending session. The session becomes active only when its first
  authenticated packet arrives, so a forged or replayed HELLO cannot
  interrupt the current player.
- **Validation.** Every datagram is checked for length, magic, version, type
  and reserved bits before authentication. Session packets must carry a
  valid tag and a sequence number newer than any accepted so far.
- **One controller.** A HELLO from a second device while a session is live
  gets REJECT (busy). The same phone may replace its own session, for
  example after a network change.
- **Watchdog.** While any input is held and no state arrives for the
  watchdog interval (300 ms), every input is released. The next state
  restores input.
- **Session timeout.** After 3 s without any authenticated packet, the
  session ends, inputs are released and the phone is sent DISCONNECT.
- **Rate limiting.** Per source IP: at most 2 handshakes per second, and a
  10-second block after repeated invalid packets. Unauthenticated replies
  are limited per IP and globally, so the receiver cannot be used to reflect
  traffic. The connected phone's address is never blocked.
- **Rumble.** Backends report motor output through a lock-free handler. The
  receiver coalesces changes to at most one packet every 20 ms and refreshes
  a running motor every 500 ms.
- **Diagnostics.** `Server.Snapshot()` exposes state, packet counters, rate,
  jitter, the phone's reported round-trip time and battery, and processing
  time percentiles. `internal/diag` renders them.

`internal/client` is a Go implementation of the phone side. `ppsim` and the
end-to-end tests use it, and it is the reference for the Kotlin transport.

## Gamepad backends

All backends implement `internal/gamepad.Gamepad`:

```go
type Gamepad interface {
    Connect() error
    Update(state protocol.ControllerState) error
    SetRumbleHandler(fn RumbleHandler)
    Capabilities() Capabilities
    Close() error
}
```

`SetRumbleHandler` replaces the `SetRumble` method from the original
specification, because rumble flows from the game to the phone.
`Capabilities` tells the session which features to offer the phone.

With `-backend auto`, the receiver tries the platform's candidates in order
and uses the first one that connects (`gamepad.ConnectBest`).

| Backend | Platform | Mechanism | Notes |
|---|---|---|---|
| `vigem` | Windows | ViGEmBus through `ViGEmClient.dll` | Xbox 360 pad with rumble. Mapping in `xinput.go` |
| `machid` | macOS | `IOHIDUserDevice` | Real HID gamepad. Needs root on macOS 15 and earlier; on macOS 26, Apple's `com.apple.developer.hid.virtual.device` entitlement |
| `mackeys` | macOS | `CGEvent` keyboard and mouse events | Needs Accessibility permission. Profiles in `mackeys.go` |
| `mock` | All | In memory | Diagnostics and tests |

On macOS, `auto` tries `machid` first and falls back to `mackeys`.

Backend rules:

- `Update(Neutral)` and `Close()` must release everything. For `mackeys`
  that means a key-up for every held key and a button-up for any held mouse
  button; a stuck key affects the whole computer.
- Backends may call the rumble handler from any thread, including from
  inside `Update`.
- Keep platform-specific code behind build tags (`_windows.go`,
  `_darwin.go`) and the mapping logic in portable files with unit tests
  (`xinput.go`, `hid.go`, `mackeys.go`).

To add a backend, implement the interface in a new file, call `Register` in
its `init` function, and add it to the platform's `autoCandidates` if `auto`
should try it.

## Android app

Source is in `android/app/src/main/kotlin/io/pocketpad/app`:

| Path | Contents |
|---|---|
| `core/ControllerView.kt` | Custom `View`: draws the controller and handles multi-touch |
| `core/Layout.kt` | Control positions in normalized coordinates, scaling and left-handed mirroring |
| `core/Mapping.kt` | Dead zone, trigger and D-pad mapping (pure, unit tested) |
| `core/InputState.kt` | The latest controller state, shared between the UI and sender threads |
| `core/SendSchedule.kt` | Sending rules (pure, unit tested with an injectable clock) |
| `core/WifiTransport.kt` | Handshake, sender and receiver threads, reconnect |
| `core/BluetoothHidTransport.kt` | Bluetooth HID Device mode |
| `core/HidReport.kt` | Bluetooth report encoding |
| `core/Rumbler.kt` | Vibration from rumble packets |
| `core/Discovery.kt` | LAN discovery of receivers |
| `core/QrPairing.kt` | Parsing of the pairing QR code |
| `core/Prefs.kt` | Saved receivers, pairing secrets, device ID and settings |
| `ui/` | Compose screens: connect, controller, controls reference, settings |
| `ui/Ads.kt` | Consent (Google UMP) and the Settings-screen banner (AdMob) |

### Touch handling

- Each pointer is tracked by ID. A pointer that goes down on a stick or a
  trigger keeps it until lifted. Buttons and the D-pad follow the pointer's
  current position, so a thumb can slide from one button to the next.
- Sticks use a radial dead zone with rescaling. Screen Y is inverted,
  because the protocol uses positive Y for up.
- `ACTION_CANCEL`, losing window focus and leaving the screen clear every
  input.

### Sending rules (Wi-Fi)

- A change is sent at once, at most at the negotiated rate (250 Hz by
  default).
- While anything is held, the state is resent every 50 ms with the snapshot
  flag, well inside the receiver's watchdog.
- While nothing is held and nothing changes, no state is sent; only a
  heartbeat every 500 ms.
- Packets are encoded into a preallocated buffer, so nothing is allocated
  per packet.
- With no authenticated packet for 3 s, the session is treated as lost and
  the transport reconnects with backoff, starting at 500 ms.
- Stopping sends a neutral state, then DISCONNECT.

### Rumble

Strength is the stronger of the two motors. Below 2% the vibrator is
cancelled. On phones without amplitude control, the phone vibrates only
above 30% strength. Updates closer than 15 ms apart are dropped unless they
stop the motor. Each vibration is bounded by the duration in the packet, so
a lost stop command cannot leave the phone vibrating.

### Ads

The only ad is an adaptive banner at the bottom of the Settings screen.
`Ads.gatherConsent` runs at startup: it shows Google's consent form where the
law requires it and initialises the ads SDK only once ads may be requested.
Settings offers "Privacy options" where users must be able to change their
choice. Debug builds always use Google's test IDs; release builds use the
`POCKETPAD_ADMOB_APP_ID` and `POCKETPAD_ADMOB_BANNER_ID` repository variables
(see [PLAY_STORE.md](PLAY_STORE.md)).

Never place an ad on the controller screen or load one while a session is
running: ads there cause accidental taps, which Google Play's policies
prohibit, and the SDK's work would compete with the input path.

### Pairing QR code

The macOS window encodes:

```
pocketpad://pair?h=<host>&p=<port>&c=<pairing code>
```

`QrPairing.parse` also accepts any text that contains an address and a
pairing code.

## Bluetooth HID report

Bluetooth mode and the `machid` backend share one HID report: report ID 1
followed by 13 bytes. The descriptor and mapping are defined in
`desktop/internal/gamepad/hid.go` and
`android/app/src/main/kotlin/io/pocketpad/app/core/HidReport.kt`, and unit
tests on both sides check them.

| Bytes | Contents |
|---|---|
| 0-1 | 16 buttons; bits 0-10 in protocol order (A, B, X, Y, LB, RB, Back, Start, LS, RS, Guide) |
| 2 | Hat switch in the low 4 bits: 0-7 clockwise from up, 8 = neutral |
| 3-10 | Left X, left Y, right X, right Y as signed 16-bit little-endian, range -32767 to 32767, positive Y down |
| 11-12 | Left and right trigger, 0-255 |

Bluetooth mode sends at most 125 reports per second and resends every
100 ms while input is held. Generic HID has no rumble channel, so rumble is
not available over Bluetooth.

## Design rules

1. **Never trust the network.** Every session packet is authenticated. The
   `-insecure` flag exists for bring-up only and must never become the
   default.
2. **Inputs must always be released.** The receiver releases on watchdog,
   timeout, disconnect and new sessions; the phone releases when the
   controller screen stops. Any new backend or transport must keep this
   property.
3. **The newest state wins.** No queues of controller states, no reorder
   buffers.
4. **The input path is lean.** No JSON, disk I/O, blocking calls on the UI
   thread or per-packet allocation.
5. **Secrets stay secret.** Never log the pairing code, pairing key or
   session keys. Store them with owner-only or app-private permissions.
6. **No ads during play.** Ads stay off the controller screen.
7. **The user installs drivers.** PocketPad never installs a driver or
   changes system security settings by itself.
8. **Go is the reference implementation.** The Kotlin code must match the
   shared vectors.

## Changing the protocol

The version 1 wire format is frozen. A change to an existing byte breaks
compatibility between phones and receivers.

To add a message:

1. Choose a new packet type and document its layout in
   [protocol.md](protocol.md).
2. Implement it in Go and Kotlin. Session messages must be authenticated.
3. Add vectors in `desktop/internal/protocol/vectors_test.go`, regenerate the
   file, and review the diff:

   ```sh
   cd desktop
   go test ./internal/protocol -run TestVectors -update
   ```

4. Make the Kotlin `VectorsTest` pass.

If an existing vector changes when you regenerate, you have changed the wire
format.

## Releasing

Releases are built by `.github/workflows/release.yml` and published on the
repository's Releases page.

### Publishing a release

1. Make sure CI passes on `main`.
2. Optionally, do a dry run: in GitHub, open Actions > Release > Run
   workflow. It builds every package without publishing anything; the
   packages appear as artifacts of that run.
3. Tag the commit and push the tag:

   ```sh
   git tag v0.1.0
   git push origin v0.1.0
   ```

The workflow then builds:

| Package | Contents |
|---|---|
| `pocketpad-<tag>-windows-amd64.zip`, `-windows-arm64.zip` | `pocketpad.exe`, `ppsim.exe`, license |
| `pocketpad-<tag>-linux-amd64.zip` | `pocketpad`, `ppsim`, license |
| `pocketpad-<tag>-macos.zip` | `PocketPad.app`, universal `pocketpad` and `ppsim`, license |
| `pocketpad-<tag>.apk` | The Android app, signed with the release key (`-debug.apk` if no key is configured) |

and creates a GitHub release with the notes in `.github/release-notes.md`
followed by the generated change list. Tags starting with `v0.` or
containing a hyphen (for example `v1.0.0-rc1`) are marked as pre-releases.

The tag sets the version everywhere: `pocketpad version`, the Mac app's
`Info.plist` and the APK's version name. The APK's version code is the
number of commits, so it always increases.

### Android signing key

Without a key, releases ship a debug-signed APK. That is fine for testing,
but Android refuses to install an update signed with a different key, so
create the release key before the first public release and keep it
forever.

1. Create a keystore (once):

   ```sh
   keytool -genkeypair -v -keystore pocketpad-release.jks -alias pocketpad \
     -keyalg RSA -keysize 4096 -validity 10000
   ```

2. Store it somewhere safe outside the repository, with a backup. If it is
   lost, existing installs can no longer be updated.
3. In GitHub, open Settings > Secrets and variables > Actions and add:

   | Secret | Value |
   |---|---|
   | `POCKETPAD_KEYSTORE_BASE64` | Output of `base64 -i pocketpad-release.jks` (macOS) or `base64 -w0 pocketpad-release.jks` (Linux) |
   | `POCKETPAD_KEYSTORE_PASSWORD` | The keystore password |
   | `POCKETPAD_KEY_ALIAS` | `pocketpad` |
   | `POCKETPAD_KEY_PASSWORD` | The key password |

To build a signed APK locally, set the same variables, with
`POCKETPAD_KEYSTORE_FILE` pointing to the keystore file, and run
`./gradlew :app:assembleRelease`.

### macOS signing

Release builds of the Mac app are ad-hoc signed and not notarised, so macOS
warns on first launch. Notarisation requires an Apple Developer account; with
one, sign with a Developer ID certificate and notarise with
`xcrun notarytool` before packaging.
