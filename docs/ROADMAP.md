# Roadmap

This document records the project's status and the work that remains. Each
remaining task lists its steps and how to tell when it is done. For how the
code is organised, see [DEVELOPMENT.md](DEVELOPMENT.md).

## Status

| Area | Status | Verification |
|---|---|---|
| Protocol (Go and Kotlin) | Complete | Unit tests, fuzzing and shared vectors in CI |
| Desktop receiver | Complete | End-to-end tests in CI on Linux, macOS and Windows |
| Android app, Wi-Fi mode | Complete | Unit tests and lint in CI. Tested on a Motorola Edge 70 Fusion: all 21 controls, no lost packets |
| Phone rumble, battery and connection indicators | Complete | Rumble tested with the mock backend; with a real game it needs the Windows test below |
| Windows backend (`vigem`) | Implemented, not yet tested on hardware | Builds and passes mapping tests in CI |
| macOS keyboard and mouse backend (`mackeys`) | Complete | Tested on macOS 26 |
| macOS HID backend (`machid`) | Works only with root on macOS 15 and earlier | Blocked on macOS 26 without Apple's entitlement |
| macOS app with QR pairing | Complete | Tested on macOS |
| Bluetooth HID mode | Implemented, not yet tested on hardware | Report mapping unit tested; the test phone lacks the HID Device profile |
| Performance measurements | Not started | |
| Release packaging | Not started | |

## Remaining work

### 1. Test the Windows backend on a real PC

Steps:

1. Install ViGEmBus and place `ViGEmClient.dll` next to `pocketpad.exe`, as
   described in [GETTING_STARTED.md](GETTING_STARTED.md#windows).
2. Run `pocketpad.exe serve` and confirm the Xbox 360 controller appears in
   `joy.cpl`.
3. On the same PC, run `ppsim.exe -addr 127.0.0.1:47770 -pattern buttons`
   and confirm every button and D-pad direction lights up in `joy.cpl`. Then
   use `-pattern mixed` to check the sticks, the triggers and simultaneous
   presses.
4. Stop `ppsim` and confirm every input returns to neutral within 300 ms.
5. Connect the phone and test two or three XInput games, including rumble.

Points to watch:

- `vigem_target_x360_update` takes its report by value. The code passes a
  pointer on x64 and two registers on ARM64. Test x64 first; if buttons look
  scrambled on ARM64, that branch is wrong.
- Windows Defender Firewall must allow UDP 47770 on private networks.

Done when: Windows shows one controller, every control works alone and
together, input stops within 300 ms when the phone disconnects, rumble
reaches the phone, and reconnecting never brings back a held button.

### 2. Test Bluetooth mode on a supported phone

Many phones ship without the Bluetooth HID Device profile. Check a phone
first with any "Bluetooth keyboard and mouse" app from the Play Store: if
that app works, the phone supports Bluetooth mode.

Steps:

1. Pair the phone with a Windows PC and check every control in `joy.cpl`.
2. Pair it with a Mac and check every control in a browser gamepad tester
   and in an SDL-based game.
3. Turn Bluetooth off on the phone while holding a button and confirm the
   computer releases it.
4. Disconnect and reconnect from the app.

Done when: both operating systems recognise the controller, all controls
work together, and the tested phone models are listed in
[GETTING_STARTED.md](GETTING_STARTED.md#bluetooth-mode).

### 3. Record performance measurements

Write the results to `docs/measurements.md`, with the phone model, computer,
operating system, Wi-Fi band and distance from the router.

1. Round-trip time, jitter and packet loss over 5 minutes of play at 120,
   166 and 250 Hz: median, 95th and 99th percentile. The phone measures
   round-trip time; the receiver's diagnostic display shows loss and jitter
   (`-diag log` writes one line per second).
2. Touch to send: the time from `MotionEvent.getEventTime()` to just before
   the packet is sent, measured in the app.
3. Receiver processing: the `proc p50/p99` values in the diagnostic display.
4. Backend update time: time `pad.Update` separately.
5. CPU, memory and battery use over 30 minutes: Android Studio's profiler or
   `adb shell dumpsys batterystats` on the phone, Task Manager or Activity
   Monitor on the computer.
6. Optional: end-to-end latency with a high-speed camera filming the finger
   and a gamepad tester on screen.

Only optimise after these baselines exist.

### 4. Package releases

1. Add a `release.yml` workflow that runs on `v*` tags and builds:
   - receiver binaries for Windows (amd64, arm64), macOS (arm64, amd64, built
     on macOS runners for cgo) and Linux (amd64), with
     `-ldflags "-X main.version=$TAG"`;
   - `PocketPad.app` for macOS;
   - a signed release APK. Keep the signing key in repository secrets, never
     in the repository.
2. Ship `ViGEmClient.dll` with the Windows build after checking its
   license, and link to the ViGEmBus installer instead of bundling it.
3. Sign and notarise the macOS app if an Apple Developer account is
   available.
4. Attach everything to a GitHub release with installation notes.

### 5. Optional improvements

- **macOS gamepad entitlement.** Apply to Apple for
  `com.apple.developer.hid.virtual.device` (requires a paid developer
  account). With it, `machid` would provide a real gamepad on macOS 26.
- **Analog control in keyboard mode.** Pulse keys at a rate proportional to
  stick deflection, so steering and throttle feel less on/off.
- **User-defined profiles.** Load `mackeys` profiles from the receiver's
  configuration file instead of compiling them in.
- **Linux backend.** A `uinput` virtual gamepad.
- **Multiple controllers.** The receiver currently serves one phone; local
  multiplayer would need one session and one virtual pad per phone.

## Acceptance criteria for version 1

From the original specification:

- [ ] One Android app connects to both Windows and macOS receivers.
- [ ] Wi-Fi mode produces a controller the operating system recognises.
      Windows: `vigem`. macOS: `machid` where permitted, otherwise
      `mackeys`.
- [ ] Bluetooth mode works as a direct HID controller on validated phones.
- [x] All required controls work independently and simultaneously.
- [x] Disconnecting releases every held input.
- [x] Reconnecting never restores stale presses.
- [ ] Rumble works wherever the backend, computer and phone support it.
- [x] Protocol tests pass on both platforms.
- [ ] Performance measurements are recorded under repeatable conditions.

## Out of scope for version 1

Kernel drivers, internet relay servers, accounts, game memory manipulation,
anti-cheat bypasses, guaranteed compatibility with every cloud gaming
service, and advanced motion controls.
