# PocketPad: handoff for the remaining work

This document is written for a developer or coding agent working **locally** with
real hardware (an Android phone, a Windows PC, a Mac) and full tooling (Android
SDK, Xcode command-line tools). It assumes no prior context. Read it top to
bottom once, then work through §4 in order.

The product specification is the original "PocketPad — Technical
Specification v1.0" (summarised in the README). The wire protocol is defined
normatively in [`docs/protocol.md`](protocol.md).

---

## 1. Current state

| Area | Status | Verified where |
|---|---|---|
| Milestone 1: protocol (Go) | **Done.** Codec, sequence numbers with wraparound, HMAC session auth, pairing codes, handshake, heartbeat, rumble, discovery, version negotiation. | Unit tests, fuzzing and zero-allocation tests on Linux; CI on Linux, macOS and Windows |
| Shared test vectors | **Done.** `protocol/testvectors/v1.json`, generated from Go. | Go tests; Kotlin tests (CI only, see §4.0) |
| Milestone 2: receiver | **Done.** `pocketpad serve`: handshake, validation, replay and stale rejection, input watchdog, session timeout, per-IP rate limiting and blocking, rumble forwarding with coalescing and refresh, live diagnostics. `ppsim` phone simulator. | End-to-end UDP tests (30/30 runs under `-race`); manual run at 250 Hz, 0 lost, about 6 µs processing |
| Kotlin protocol module | **Written** (`android/protocol`), a pure JVM library mirroring the Go one. | **Not yet run.** Maven was rate-limited in the cloud; runs in CI job `android-protocol` |
| Milestone 4: Windows backend | **Written** (`desktop/internal/gamepad/vigem_windows.go`): virtual Xbox 360 pad through ViGEmBus, including rumble. Mapping unit-tested. | Compiles for windows/amd64 and arm64. **Not run on a real PC.** |
| Milestone 3: Android app | **Not started.** Full design in §4.2. | n/a |
| Milestone 5: macOS backend | **Not started.** Approach in §4.3; this is the riskiest item. | n/a |
| Milestone 6: Bluetooth HID | **Not started.** Descriptor and design in §4.4. | n/a |
| Milestone 7: feedback and measurements | Receiver side done (rumble forwarding, battery and RTT display). Phone side and measurements remain (§4.5). | n/a |

### Repository layout

```
README.md                    overview and quick start
docs/protocol.md             NORMATIVE wire protocol (read this first)
docs/HANDOFF.md              this file
protocol/testvectors/v1.json vectors shared by Go and Kotlin (generated, never hand-edit)
.github/workflows/ci.yml     Go tests on 3 OSes, fuzzing, binaries, Kotlin protocol tests
desktop/                     Go module github.com/bhaavyasura7/pocketpad/desktop (no third-party deps)
  cmd/pocketpad/             receiver CLI: serve | pair [-reset] | backends | version
  cmd/ppsim/                 phone simulator (patterns: mixed, circle, buttons, hold, idle)
  internal/protocol/         state model, codec, ExtSeq/ReplayWindow, Sealer/Opener, pairing, vectors test
  internal/session/          Server: the receiver core (+ limiter.go). server_test.go = end-to-end tests
  internal/gamepad/          Gamepad interface, registry, mock, xinput.go (mapping), vigem_windows.go
  internal/client/           Go implementation of the PHONE side. Use it as the reference for Kotlin
  internal/config/           pairing secret storage (receiver.json, mode 0600)
  internal/diag/             live terminal display
android/
  protocol/                  standalone Gradle build, pure Kotlin/JVM, package io.pocketpad.protocol
```

### Commands

```sh
# Go (from desktop/)
go vet ./... && go test -race ./...
go test ./internal/protocol -run TestVectors -update      # only after an intentional wire change
go test -run XXX -fuzz FuzzDecoders -fuzztime 30s ./internal/protocol
go run ./cmd/pocketpad pair                                # show pairing code
go run ./cmd/pocketpad serve -backend mock                 # receiver with live diagnostics
go run ./cmd/ppsim -addr 127.0.0.1:47770                   # simulated phone (reads local pairing code)

# Kotlin protocol (from android/protocol/)
./gradlew test
```

---

## 2. Invariants: do not break these

1. **The wire format is frozen** for protocol v1. If you change any byte, the
   vectors test fails. Treat that as a wire break, not something to
   regenerate around. Any new message needs a new packet type, documented in
   `docs/protocol.md`, with vectors added in `vectors_test.go`.
2. **Go is the reference implementation.** The Kotlin code must pass the same
   vectors. Keep the Kotlin `VectorsTest` green.
3. **Never trust LAN packets.** Every session packet carries an 8-byte
   HMAC tag, and new message types must be authenticated too.
   `-insecure` is a development flag only and must never become the default.
4. **Safety:** a lost release must never leave an input held. The receiver
   already has a watchdog, a session timeout and neutral-on-new-session. The
   phone must also: release everything when the controller screen stops,
   resend held state every ≤ 50 ms, and send neutral then DISCONNECT on exit.
5. **Hot path:** no JSON, disk I/O, unbounded queues or blocking calls on
   the UI thread, and no per-packet allocation. The newest state always wins.
6. **Secrets:** never log the pairing code, pairing key or session keys. Store
   them with owner-only or app-private permissions.
7. **Drivers:** PocketPad never installs a driver by itself. The user
   installs ViGEmBus or grants macOS permissions explicitly (spec §7).
8. **Gamepad interface:** `internal/gamepad.Gamepad` intentionally uses
   `SetRumbleHandler(fn)` instead of the spec's `SetRumble` (rumble flows
   game → phone) and adds `Capabilities()`. Backends must call the rumble
   handler from their own thread. It is non-blocking and lock-free on the
   receiver side, so calling it inside `Update` is also safe.

---

## 3. Protocol summary (see docs/protocol.md for the full details)

- UDP port 47770. A 6-byte header (`'P' 'P'`, version 1, type, seq u16 LE).
- STATE: 20 bytes plus an 8-byte tag. Buttons: bit0 A, 1 B, 2 X, 3 Y, 4 LB,
  5 RB, 6 Back, 7 Start, 8 LS, 9 RS, 10 Guide. D-pad: 0 neutral, 1–8 clockwise
  from up. Axes are int16 ±32767 with **+Y up**. Triggers are u8.
- Pairing: 120-bit secret shown as `XXXX-XXXX-XXXX-XXXX-XXXX-XXXX` (Crockford
  base32). `pairKey = HMAC(key="PocketPad v1 pairing key", secret)`.
- Handshake: HELLO (fresh nonce each attempt) → WELCOME (bound to client
  nonce) → the first session packet activates the session. Per-direction keys
  `c2s` and `s2c`.
- Tag = `HMAC-SHA256(dirKey, ROC u32 ‖ body)[:8]` over an implicit 64-bit
  extended sequence number. Only strictly newer packets are accepted.
- The phone sends a HEARTBEAT every `heartbeatMs` (default 500). The receiver
  echoes it immediately, and the phone computes RTT on its own clock.
- Receiver → phone: RUMBLE (left, right u16 and duration ≤ 2000 ms). The
  receiver refreshes a running motor every 500 ms with a 750 ms duration, and
  `0,0,0` means stop. DISCONNECT has reasons 0 user, 1 shutdown, 2 timeout,
  3 replaced.
- REJECT reasons: 1 version, 2 busy (another phone is connected), 3 rate
  limited. **A wrong pairing code gets no reply at all.** After about 4
  silent attempts the phone should say "check the address and pairing code".

---

## 4. Remaining work, in order

### 4.0 Verify CI (15 minutes)

1. Open GitHub Actions for the latest commit. All four jobs must be green:
   `Desktop (ubuntu/macos/windows)` and `Kotlin protocol (shared vectors)`.
2. The Kotlin job has **never run successfully** (it was written without
   being executed). If it fails, fix the Kotlin code, not the vectors. Likely
   suspects: rounding in `Codec.encodeAxis` (must round half away from zero,
   like Go's `math.Round`), `Pairing.parse`, and `deviceId` parsing as an
   unsigned 64-bit value in the test.
3. Locally: `cd android/protocol && ./gradlew test`.

### 4.1 Validate the Windows backend on a real PC (Milestone 4)

**Setup**
1. Install ViGEmBus from https://github.com/nefarius/ViGEmBus/releases (the
   user must agree to the driver install). It is retired upstream but works
   on Windows 10 and 11. If it proves unusable, implement an alternative
   backend behind the same interface; nothing else changes.
2. Get `ViGEmClient.dll` matching the architecture (x64 for most PCs).
   ViGEmClient upstream is source-only (https://github.com/nefarius/ViGEmClient,
   build the DLL configuration with Visual Studio). The `vgamepad` Python
   package also bundles prebuilt DLLs under `vgamepad/win/vigem/client/x64/`.
   Place it next to `pocketpad.exe`.
3. Build: `cd desktop && go build -o pocketpad.exe ./cmd/pocketpad`, or use
   the `pocketpad-Windows` CI artifact.

**Test**
1. `pocketpad.exe serve` (backend `auto` = `vigem`). Expect no error, and the
   "Xbox 360 Controller" appears in `joy.cpl` (Win+R → `joy.cpl`).
2. On the same PC: `ppsim.exe -addr 127.0.0.1:47770 -pattern buttons`. In
   `joy.cpl` → Properties, every button and D-pad direction lights up in turn.
   Then use `-pattern mixed` to check sticks, triggers and A+B together.
3. Ctrl+C `ppsim`, and confirm everything returns to neutral within 300 ms.
4. Rumble: use any XInput rumble tester (e.g. "XInput Test" tools, or a game).
   `ppsim` prints `rumble: left …% right …%`.
5. Test 2–3 real XInput games and Steam's controller settings.

**Known risks to check**
- `vigem_target_x360_update` takes `XUSB_REPORT` **by value**. The code passes
  a pointer on x64 (the Windows x64 ABI passes 12-byte structs by reference)
  and two registers on ARM64. If buttons look scrambled on ARM64, the arm64
  branch is wrong. Test on x64 first.
- The rumble callback uses `syscall.NewCallback` with 6 uintptr args.
- Windows Defender Firewall must allow UDP 47770 on *Private* networks for
  the phone to connect. Document it in the README, or add a `pocketpad
  firewall` helper that prints the `netsh advfirewall` command for the user
  to run themselves. Do not run it silently.

**Acceptance:** Windows sees one controller, all controls work at the same
time, input stops within 300 ms when the phone disappears, rumble reaches the
phone, and a reconnect never brings back a held button.

### 4.2 Android app, Wi-Fi mode (Milestone 3)

Location: `android/` (a new Gradle build next to `android/protocol`).
Package `io.pocketpad.app`.

**Gradle setup** (versions that are known to work together; newer is fine)
- Gradle wrapper 8.11.1 (copy `android/protocol/gradle*` up to `android/`).
- AGP 8.7.x, Kotlin 2.1.0 with `org.jetbrains.kotlin.plugin.compose` 2.1.0,
  Compose BOM 2024.12.01, `androidx.activity:activity-compose`, Material 3,
  `kotlinx-coroutines-android`.
- `compileSdk 35`, `targetSdk 35`, **`minSdk 28`** (`BluetoothHidDevice` is
  API 28).
- `android/settings.gradle.kts`: `include(":app")` and
  `includeBuild("protocol")`. In the app,
  `implementation("io.pocketpad:protocol:1.0")` (the composite build
  substitutes it).
- Add a CI job: set up JDK 17, then `cd android && ./gradlew :app:assembleDebug
  :app:testDebugUnitTest lint`, and upload `app/build/outputs/apk/debug/*.apk`.

**Permissions** (`AndroidManifest.xml`): `INTERNET`, `ACCESS_NETWORK_STATE`,
`ACCESS_WIFI_STATE`, `VIBRATE`, plus Bluetooth for §4.4: `BLUETOOTH` and
`BLUETOOTH_ADMIN` with `maxSdkVersion=30`, `BLUETOOTH_CONNECT` and
`BLUETOOTH_ADVERTISE` (runtime-requested on API 31+).

**Architecture**

```
ui/ (Compose)       ConnectScreen, ControllerScreen (hosts ControllerView), SettingsScreen
core/ (no Compose)  InputState         latest ControllerState, written by the UI thread
                    ControllerView     custom View: draws the pad, handles multi-touch
                    Layout             control rectangles in normalized coordinates
                    Transport          interface: start(), stop(), status: StateFlow, onRumble
                    WifiTransport      UDP socket, handshake, sender and receiver threads
                    BluetoothHidTransport (§4.4)
                    Discovery          DISCOVER broadcast, collects DISCOVER_REPLY for 1 s
                    Rumbler            Vibrator / VibratorManager wrapper
                    Prefs              SharedPreferences: device ID, receivers, pairing secret, settings
```

Use a **custom `View`** for the controller surface, not Compose gestures.
`onTouchEvent` with `MotionEvent` pointer indices gives reliable simultaneous
multi-touch and costs nothing per frame when idle. Compose hosts it via
`AndroidView`. Redraw (`invalidate()`) only when the state changes.

**Touch handling (ControllerView)**
- Track each pointer ID → the control it is on. Handle `ACTION_DOWN`,
  `POINTER_DOWN`, `MOVE` (iterate over all pointers), `POINTER_UP`, `UP`
  and **`CANCEL`** (clear every pointer).
- **Sticks** (left: bottom-left, right: bottom-right-centre): a pointer that
  goes down in a stick zone captures it until lifted. Vector = (pos −
  centre) / radius, clamped to length 1. Apply a radial dead zone
  (configurable, default 0.10) with rescaling:
  `m' = (m − dz) / (1 − dz)`. **Invert Y** (screen Y grows downwards,
  protocol +Y is up).
- **Stick clicks:** separate small LS and RS buttons next to the sticks (the
  default). An optional setting enables double-tap-and-hold on the stick.
- **Triggers** (top corners): analog. The value is the vertical position
  within the trigger rectangle, mapped so that the top 10% gives about 0.15
  and the bottom 60% or more gives 1.0. Add a setting: "Triggers: analog /
  full press".
- **Buttons** (ABXY diamond on the right, LB and RB above the triggers,
  Back, Start and Guide in the centre): a pointer presses the button under
  its *current* position, so sliding a thumb from A to B works. Hit-test
  circles with a +15% radius. One pointer presses at most one button.
- **D-pad** (left, above the stick): one zone. The angle from its centre picks
  one of 8 directions (45° sectors), with a neutral centre dead zone (25% of
  the radius). The pointer follows its current position.
- After each event, write the new `ControllerState` into `InputState` and
  wake the sender. Optional: `performHapticFeedback(KEYBOARD_TAP)` on a
  button down (setting, default on).
- Clear all inputs in `onWindowFocusChanged(false)`, `onDetachedFromWindow`
  and when the activity stops.

**WifiTransport** (mirror `desktop/internal/client/client.go`)
- `DatagramSocket` connected to the receiver. Handshake as in the Go client:
  up to 4 HELLO attempts with a fresh nonce each and timeouts of 250, 500,
  1000 and 2000 ms. On REJECT, show the reason (busy or version). On
  silence, show "No answer: check the address and pairing code".
- **Sender thread** (`Process.THREAD_PRIORITY_URGENT_DISPLAY` or `DISPLAY`):
  ```
  loop:
    wait on a lock with a timeout until (dirty) or the next snapshot/heartbeat is due
    if dirty and now - lastSent >= 1/maxRateHz  -> send STATE (latest state only)
    else if state non-neutral and now - lastSent >= 50 ms -> send STATE with FLAG_SNAPSHOT
    if now - lastHeartbeat >= heartbeatMs -> send HEARTBEAT (timeUs, echoUs, rttUs, battery)
  ```
  Use one preallocated `ByteArray(128)` and one `DatagramPacket`, with
  `Sealer` / `Codec.putState`, so nothing is allocated per packet.
  `maxRateHz` comes from WELCOME (the receiver default is 250; the spec range
  is 120–250). Send no STATE while neutral and unchanged.
- **Receiver thread:** blocking `receive` into a preallocated buffer.
  `Codec.checkHeader`, then `Opener.open`. On HEARTBEAT, store `echo` and
  compute `rtt = max(nowUs − echoUs, 1)` (`nowUs` = `(SystemClock.elapsedRealtimeNanos()/1000).toInt() or 1`:
  0 means "no timestamp" on the wire, so never send it). On RUMBLE, call `Rumbler`. On DISCONNECT, end the
  session and show why.
- **Liveness:** with no authenticated packet for 3 s, the session is lost. The
  UI shows "Reconnecting…" and the transport redoes the handshake with
  backoff (0.5, 1, 2, 4 s; max 5 s). Keep the user's current touches: a
  finger still on the screen is live input, not stale input.
- `stop()`: send a neutral STATE, then DISCONNECT(0), then close the socket.
  Called from `onStop()` of the controller screen.
- Battery: `BatteryManager.BATTERY_PROPERTY_CAPACITY` and charging state,
  read every heartbeat at most (it's cheap), or from a sticky
  `ACTION_BATTERY_CHANGED` intent.
- Networking off the main thread only (StrictMode in debug builds helps).

**Rumbler**
- On API 31+, use `VibratorManager.defaultVibrator`, otherwise `Vibrator`.
- strength = `max(left, right) / 65535`. If below 0.02, `cancel()`.
  Otherwise, if `hasAmplitudeControl()`, use
  `VibrationEffect.createOneShot(durationMs, (strength*255).coerceIn(1,255))`.
  Without amplitude control, vibrate only when strength > 0.3.
- Ignore updates that arrive < 15 ms after the previous one unless they stop
  the motor. Cancel on DISCONNECT and in `stop()`. The duration from the
  packet bounds each one-shot, so a lost stop is self-limiting.
- Use the user setting "Vibrate on rumble" (default on).

**ConnectScreen**
- A list of discovered receivers (broadcast DISCOVER to
  `255.255.255.255:47770`, collect for 1 s, with a refresh button) plus
  "Enter address manually" (host or IP, port defaults to 47770).
- Pairing code entry (24 characters, auto-uppercase, auto-insert dashes)
  validated with `Pairing.parse`. Save the **secret** per receiver in
  app-private SharedPreferences (optionally encrypted with the Android
  Keystore). Never log it.
- Generate a random 64-bit device ID once on first launch and store it.
  The device name is `Build.MODEL`.
- Mode toggle: Wi-Fi / Bluetooth (§4.4).
- Show the status (Connecting / Connected · RTT · rate / Reconnecting /
  Error). Show the last error clearly.

**ControllerScreen:** landscape, immersive full screen
(`WindowInsetsControllerCompat`, hide the system bars), keep the screen on
(`FLAG_KEEP_SCREEN_ON`). A small overlay shows the connection dot, RTT, the
phone battery and a menu button (with a long-press to avoid accidental exits).

**SettingsScreen:** dead zone, trigger mode, update rate (120 / 166 / 250 Hz,
capped by WELCOME), haptic tick, vibrate on rumble, layout scale (0.8–1.2),
left-handed swap.

**Unit tests (JVM):** dead zone math, D-pad sector mapping, trigger mapping,
the sender scheduling logic (make it a pure class with an injectable clock),
and Rumbler decision logic.

**Acceptance (with a real phone)**
1. `pocketpad serve -backend mock` on the PC. The phone discovers it, pairs
   with the code from `pocketpad pair`, and the diagnostic line shows the
   device.
2. All controls move in the diagnostic display, including 4 or more
   simultaneous touches (e.g. both sticks + A + RT).
3. Rate ≈ the configured Hz while moving and 20 Hz while holding still with
   something pressed. While idle, the counter stays flat except for heartbeats.
4. Lock the phone or switch apps: the receiver releases everything within
   300 ms, then disconnects within 3 s. Coming back reconnects automatically
   with nothing stuck.
5. Turn Wi-Fi off and on: it reconnects, and no button comes back pressed.
6. Wrong pairing code: a clear error, and the receiver logs "wrong pairing code".
7. Rumble: run `pocketpad serve -backend mock -test-rumble`, which cycles
   rumble strong → weak → stop, one step per second. The phone must vibrate in step and
   stop each time. Then repeat through a real game on Windows (ViGEm).
8. Loss and latency: note the diagnostic line's `lost`, `rtt` and `jitter`
   over 5 minutes of play on your Wi-Fi. Record them in `docs/measurements.md`.

### 4.3 macOS backend (Milestone 5): the riskiest item, do a spike first

macOS has no public, unprivileged API for creating a virtual game controller.
Options, in recommended order:

1. **`IOHIDUserDeviceCreateWithProperties`** (IOKit, `IOHIDUserDevice.h`) from
   Go via cgo. Create a HID device with a gamepad report descriptor (reuse the
   one in §4.4, same report layout), then call `IOHIDUserDeviceHandleReportWithTimeStamp`
   for each `Update`. Since macOS 11 this requires the
   `com.apple.developer.hid.virtual.device` entitlement (a restricted
   entitlement Apple must grant). In practice it usually works for processes
   run as **root** (`sudo pocketpad serve`) or ad-hoc signed with the
   entitlement while SIP is reduced. **Spike this first:** a 100-line cgo
   program that creates the device and toggles one button, then check whether
   it appears in System Information → USB or Game Controllers, and in
   https://hardwaretester.com/gamepad in Safari or Chrome. Try both as root and
   as a normal user, on the macOS version you target.
2. **Make the device look like a supported controller** if the generic gamepad
   is ignored. Apple's GameController framework only recognises known
   controllers (Xbox, DualShock, DualSense, MFi). SDL and Steam read raw HID
   and generally accept a generic HID gamepad, so check those first. Spoofing
   another vendor's VID/PID is legally and technically fragile; prefer
   generic. Document which games and frameworks see it.
3. **DriverKit virtual HID** (`HIDDriverKit`, like Karabiner-DriverKit-VirtualHIDDevice):
   the robust long-term route. It needs an Apple Developer account, the DriverKit
   entitlements requested from Apple, a system extension, and a user approval
   in System Settings → Privacy & Security. Plan for weeks, not days.

**Implementation shape** (any option): `desktop/internal/gamepad/hid_darwin.go`
with `//go:build darwin && cgo`, registered as `"machid"` and set as
`platformDefault`. `Connect` creates the device, `Update` builds the 13-byte
report (+ report ID), and `Close` sends neutral and then releases the device.
Rumble is not available for a generic HID pad on macOS:
`Capabilities{Rumble: false}`. The CI macOS job already builds with cgo, so
keep it compiling. Write clear user-facing errors that name the missing
permission and the exact fix.

**Acceptance:** with `ppsim -pattern buttons`, every control is visible in a
browser gamepad tester and in at least one SDL-based game. Installation and
permission steps are written in the README.

### 4.4 Bluetooth HID mode (Milestone 6)

**`BluetoothHidTransport`** (Android, `core/`):
1. Check `BluetoothAdapter.getProfileProxy(ctx, listener, BluetoothProfile.HID_DEVICE)`.
   If the proxy never connects or `registerApp` fails, show "This phone does
   not support Bluetooth controller mode" and fall back to Wi-Fi. Many phones
   ship with the HID Device profile disabled.
2. `registerApp(sdp, null, qos, executor, callback)` with
   `BluetoothHidDeviceAppSdpSettings("PocketPad", "PocketPad gamepad", "PocketPad",
   BluetoothHidDevice.SUBCLASS1_NONE or BluetoothHidDevice.SUBCLASS2_GAMEPAD, DESCRIPTOR)`.
3. Pairing: request discoverability (`ACTION_REQUEST_DISCOVERABLE`, 120 s).
   The user adds the device from the PC or Mac Bluetooth settings. Later,
   reconnect with `hidDevice.connect(bondedHost)`. Remember the last host
   address.
4. `onConnectionStateChanged`: on CONNECTED, start sending. On DISCONNECTED,
   clear the state and show the status.
5. Send `sendReport(host, 1, report)` on every change, capped at 125 Hz
   (Bluetooth intervals are about 7.5–11.25 ms), and resend every 100 ms
   while non-neutral. Reuse the 13-byte array.
6. `onSetReport`, `onGetReport` and `onInterruptData`: reply to GET_REPORT
   with the current report. Rumble over generic HID is **not** supported by
   Windows or macOS without force-feedback descriptors, so leave it out of v1
   and say so in the UI.
7. Unregister the app in `stop()`, and send a neutral report first.

**Report descriptor** (report ID 1, 13 data bytes):

```
0x05,0x01,        // Usage Page (Generic Desktop)
0x09,0x05,        // Usage (Game Pad)
0xA1,0x01,        // Collection (Application)
0x85,0x01,        //   Report ID (1)
0x05,0x09,        //   Usage Page (Button)
0x19,0x01,0x29,0x10, //   Usage Min 1, Max 16
0x15,0x00,0x25,0x01, //   Logical 0..1
0x75,0x01,0x95,0x10, //   16 x 1 bit
0x81,0x02,        //   Input (Data,Var,Abs)              -> bytes 0-1: buttons
0x05,0x01,        //   Usage Page (Generic Desktop)
0x09,0x39,        //   Usage (Hat switch)
0x15,0x00,0x25,0x07, //   Logical 0..7
0x35,0x00,0x46,0x3B,0x01, // Physical 0..315
0x65,0x14,        //   Unit (Degrees)
0x75,0x04,0x95,0x01,
0x81,0x42,        //   Input (Data,Var,Abs,Null State)   -> byte 2 low nibble
0x65,0x00,        //   Unit (None)
0x35,0x00,0x45,0x00, // Physical reset
0x75,0x04,0x95,0x01,
0x81,0x03,        //   Input (Const) padding             -> byte 2 high nibble
0x09,0x30,0x09,0x31,0x09,0x32,0x09,0x35, // X, Y, Z, Rz
0x16,0x01,0x80,0x26,0xFF,0x7F,           // Logical -32767..32767
0x75,0x10,0x95,0x04,
0x81,0x02,        //   Input                              -> bytes 3-10: LX, LY, RX, RY (int16 LE)
0x05,0x02,        //   Usage Page (Simulation Controls)
0x09,0xC5,0x09,0xC4, // Brake (LT), Accelerator (RT)
0x15,0x00,0x26,0xFF,0x00, // Logical 0..255
0x75,0x08,0x95,0x02,
0x81,0x02,        //   Input                              -> bytes 11-12: LT, RT
0xC0              // End Collection
```

Report mapping from `ControllerState`:
- bytes 0–1 = `buttons and 0x07FF` (bit order identical to the protocol: HID
  button 1 = A … 11 = Guide).
- byte 2 = `if (dpad == 0) 8 else dpad - 1` (8 is outside 0..7, so it reads
  as null/neutral).
- **HID Y grows downwards:** LY and RY are sent as `-encodeAxis(ly)` and
  `-encodeAxis(ry)`.
- Triggers use `encodeTrigger`.

Put the descriptor and the mapping in a pure Kotlin class with unit tests
(a known state → known 13 bytes).

**Acceptance:** pair and reconnect on Windows (check `joy.cpl`) and macOS
(browser gamepad tester and an SDL game). All controls work simultaneously.
Turning Bluetooth off on the phone releases everything on the host. Record
which phone models work in the README.

### 4.5 Feedback, indicators and measurements (Milestone 7)

- Phone: battery and connection indicators on the controller overlay
  (RTT, rate, receiver name).
- Receiver: already shows battery, RTT, jitter, loss and processing p50/p99.
  Add `pocketpad serve -diag log` output to files for measurement runs.
- **Measurement procedure** (write the results to `docs/measurements.md`, with
  phone model, PC, OS, Wi-Fi band, distance and channel congestion):
  1. RTT, jitter and loss: 5 minutes of `mixed` play at 120, 166 and 250 Hz,
     recording median, p95 and p99. The phone already measures RTT; add a
     small in-app ring buffer for p95/p99.
  2. Touch → send: `MotionEvent.eventTime` (uptime clock) vs.
     `SystemClock.uptimeMillis()` right before `send()`. Record the histogram
     in the app.
  3. Receiver processing: the diagnostic `proc p50/p99` (read → backend
     updated).
  4. Backend update time: time `pad.Update` separately in the diagnostics.
  5. End to end (optional): a high-speed phone camera filming the finger and a
     gamepad tester on the monitor.
  6. CPU, memory and battery: Android Studio profiler or
     `adb shell dumpsys batterystats` over 30 minutes, plus Task Manager or
     Activity Monitor for the receiver.
- Optimise only after you have this baseline.

### 4.6 Packaging and release

- A GitHub Actions `release.yml` on tags `v*` that builds:
  - `pocketpad` binaries for windows/amd64 and arm64, darwin/arm64 and amd64
    (cgo, on macOS runners), and linux/amd64. Use `-ldflags "-X main.version=$TAG"`.
  - A **signed** release APK. Keep the keystore in repository secrets, never
    committed. `.gitignore` already excludes `*.jks` and `*.keystore`.
- Windows: ship `ViGEmClient.dll` next to the exe (check its license, MIT at
  the time of writing) and link to the ViGEmBus installer. Do not bundle the
  driver installer silently.
- macOS: sign and notarise if you obtain the entitlement. Otherwise document
  the root or permission requirement.
- README: user guide (install, pair, firewall, troubleshooting).

---

## 5. Final acceptance checklist (from spec §10)

- [ ] One Android app connects to both Windows and macOS receivers.
- [ ] Wi-Fi mode produces a gamepad recognised by the OS through the backend
      (Windows: ViGEm; macOS: §4.3).
- [ ] Bluetooth mode works as a direct HID controller on validated phones
      (list them).
- [ ] All required controls work independently and simultaneously.
- [ ] Disconnecting releases every held input (watchdog, timeout, DISCONNECT, app stop).
- [ ] Reconnection never restores stale presses.
- [ ] Rumble works wherever the backend, host and phone support it
      (ViGEm → phone yes; generic HID no; document it).
- [ ] Protocol tests pass on both platforms (Go on 3 OSes, Kotlin vectors).
- [ ] Performance measurements recorded under repeatable conditions
      (`docs/measurements.md`).

## 6. Out of scope for v1 (spec §11)

Kernel drivers, internet relays, accounts, game memory manipulation, anti-cheat
bypass, guaranteed cloud-gaming compatibility, advanced motion controls.
Multiple simultaneous phones are also out of scope: the receiver deliberately
serves one controller and answers others with REJECT(busy).
