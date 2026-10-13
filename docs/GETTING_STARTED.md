# PocketPad — getting started

Use your Android phone as a controller for your computer. This guide takes
you from zero to playing in one pass, with no flags to memorise.

## What you need

- An Android phone (Android 9+) and a computer on the **same Wi-Fi network**.
- About five minutes.

## 1. Start the receiver on your computer

### The Mac app (easiest)

Build the one-time bundle and double-click it:

```sh
./scripts/build-mac-app.sh
open dist/PocketPad.app
```

A window opens with a **big QR code**: scan it with the phone app (below)
and everything is filled in automatically. The window also shows the
pairing code and the live connection status. Options go through `open`:

```sh
open dist/PocketPad.app --args -profile racing
```

### Command line (all systems)

Build it once (requires [Go](https://go.dev/dl/) 1.24+):

```sh
cd desktop
go build -o pocketpad ./cmd/pocketpad       # Windows: pocketpad.exe
```

Then start it:

```sh
./pocketpad serve
```

That is the whole computer side. The receiver picks the best backend for
your OS automatically, prints your LAN address and shows your **pairing
code**:

```
PocketPad receiver dev listening on UDP port 47770 as "Mac"
  address for the phone: 192.168.1.20:47770

  Pairing code:  AVBB-QGJC-GNVM-T9DT-G7EM-VKR7
  Enter it in the PocketPad app (same Wi-Fi network). Press Ctrl+C to stop.
```

If your firewall asks, allow incoming connections (UDP port 47770).

| OS | What games/apps see | Notes |
|---|---|---|
| **Windows** | a real Xbox 360 controller, rumble included | Install [ViGEmBus](https://github.com/nefarius/ViGEmBus/releases) and put `ViGEmClient.dll` (x64) next to `pocketpad.exe` once |
| **macOS 15 and older** | a real HID gamepad | Start it with `sudo ./pocketpad serve` |
| **macOS 26** | keyboard & mouse control | The receiver asks for **Accessibility** the first time: System Settings → Privacy & Security → Accessibility → enable **Terminal**. One toggle, once |
| **Linux** | diagnostics only (no virtual gamepad yet) | `./pocketpad serve -backend mock` |

## 2. Install the phone app

Build it (requires JDK 17 and the Android SDK):

```sh
cd android
./gradlew :app:assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

## 3. Pair and connect

1. Open **PocketPad**. It finds the receiver automatically — tap it.
2. Type the **pairing code** from the receiver banner.
3. Tap **Connect**.

The controller screen opens in landscape. The chip at the top shows the
connection, round-trip time and your phone's battery. The pairing code is
saved per computer, so the next time it is one tap.

## 4. Play

### Profiles (macOS keyboard/mouse mode)

The mapping depends on the kind of game. Pick a profile when starting the
receiver, for example:

```sh
./pocketpad serve -profile racing
```

| Profile | For | Highlights |
|---|---|---|
| `general` (default) | desktop, browsers, Minecraft | Right stick = mouse, left stick = WASD, A = Space, triggers = left/right click |
| `racing` | arcade racers (NFS, Trackmania, older F1 games…) | **Triggers are throttle and brake**, stick or d-pad steers, Space = handbrake |
| `gta` | GTA V and similar open-world/TPS games | Right stick = mouse look, LT/RT = aim and fire, X = F (enter/exit vehicle), Y = Q (cover) |

Full tables:

**general**

| Phone | Mac | Phone | Mac |
|---|---|---|---|
| Left stick | W A S D | Right stick | mouse |
| D-pad | arrow keys | LT / RT | left / right click |
| A | Space | B | Shift |
| X | E | Y | Q |
| LB | Tab | RB | Esc |
| Back | M | Start | Enter |
| Guide | F1 | LS / RS | C / R |

**racing**

| Phone | Mac | Phone | Mac |
|---|---|---|---|
| Left stick | steer + W/S | D-pad | arrows |
| **RT** | **W (accelerate)** | **LT** | **S (brake/reverse)** |
| A | Space (handbrake) | B | Shift |
| X | C (camera) | Y | R (reset car) |
| LB / RB | Q / E (look) | Start | Enter |
| Back | M | Guide | Esc |
| LS / RS | H (horn) / V | Right stick | unused |

**gta**

| Phone | Mac | Phone | Mac |
|---|---|---|---|
| Left stick | W A S D | Right stick | mouse look |
| **LT** | **right click (aim)** | **RT** | **left click (fire)** |
| A | Space (jump) | B | Shift (sprint) |
| X | F (enter/exit vehicle) | Y | Q (cover) |
| LB | E (interact/horn) | RB | V (camera) |
| Back | M | Start | Esc |
| Guide | Tab | LS / RS | C / R |

Rumble only exists on the Windows backend (games drive the phone's
vibrator). Toggle it under Settings in the app.

## Troubleshooting

| What you see | What to do |
|---|---|
| "No answer from the computer" | Check the address and the pairing code; both devices must be on the same Wi-Fi; allow UDP 47770 through the firewall; get a fresh code with `./pocketpad pair` |
| "Another phone is already connected" | One phone per receiver; disconnect the other phone first |
| macOS: nothing moves the cursor | Grant Accessibility to **Terminal** (System Settings → Privacy & Security → Accessibility) and restart the receiver |
| macOS: "macOS refused to create the virtual HID device" | Expected on macOS 26 without Apple's entitlement — plain `./pocketpad serve` falls back to keyboard/mouse automatically; only `-backend machid` shows this error |
| "This phone does not support Bluetooth controller mode" | Your phone's firmware has no HID Device profile (common); use Wi-Fi mode |
| Choppy input / high RTT | Use a 5 GHz Wi-Fi band and stay close to the router |
| Buttons stuck after app switch | Nothing to do — the receiver's watchdog releases everything within 300 ms |

## Going deeper

- The full protocol: [`docs/protocol.md`](protocol.md)
- Architecture, milestones and the remaining hardware work:
  [`README.md`](../README.md) and [`docs/HANDOFF.md`](HANDOFF.md)
- Custom mappings: profiles live in
  `desktop/internal/gamepad/mackeys.go` (`macKeyProfiles`), one small table
  each — copy one and adjust.
