# Getting started

This guide takes you from nothing installed to playing a game with your
phone as the controller.

## Contents

1. [Requirements](#requirements)
2. [Set up the computer](#set-up-the-computer)
3. [Install the phone app](#install-the-phone-app)
4. [Pair and connect](#pair-and-connect)
5. [Playing](#playing)
6. [Keyboard and mouse profiles (macOS 26)](#keyboard-and-mouse-profiles-macos-26)
7. [Bluetooth mode](#bluetooth-mode)
8. [App settings](#app-settings)
9. [Receiver options](#receiver-options)
10. [Troubleshooting](#troubleshooting)

## Requirements

- An Android phone running Android 9 or later.
- A Windows or macOS computer on the **same Wi-Fi network** as the phone.
- To build from source: [Go](https://go.dev/dl/) 1.24 or later for the
  receiver, and JDK 17 with the Android SDK (platform 35) for the app. CI
  also publishes ready-made builds; see below.

## Set up the computer

The receiver is a single program, `pocketpad`. It listens on UDP port 47770,
so allow it through the firewall when the operating system asks.

Ready-made receiver builds for each platform are attached to every CI run on
GitHub (artifacts `pocketpad-Windows`, `pocketpad-macOS` and
`pocketpad-Linux`). To build it yourself:

```sh
cd desktop
go build -o pocketpad ./cmd/pocketpad        # on Windows: -o pocketpad.exe
```

### Windows

Windows sees PocketPad as an Xbox 360 controller, with rumble.

1. Install the ViGEmBus driver from
   <https://github.com/nefarius/ViGEmBus/releases>. ViGEmBus is no longer
   developed but still works on Windows 10 and 11.
2. Place `ViGEmClient.dll` (x64 for most PCs) next to `pocketpad.exe`. The
   upstream project ships it as source
   (<https://github.com/nefarius/ViGEmClient>); prebuilt copies are also
   bundled with the `vgamepad` Python package.
3. Start the receiver:

   ```sh
   pocketpad.exe serve
   ```

4. Check that "Xbox 360 Controller" appears under **Game Controllers**
   (press Win+R and run `joy.cpl`).

### macOS

What the Mac receives depends on the macOS version:

| macOS version | Result | How to start it |
|---|---|---|
| 15 and earlier | A real HID gamepad | `sudo ./pocketpad serve` |
| 26 | Keyboard and mouse control, using a game profile | `./pocketpad serve`, then grant Accessibility |

On macOS 26, Apple requires a restricted entitlement
(`com.apple.developer.hid.virtual.device`) before any app can create a
virtual gamepad, even when run as root. The receiver detects this and
switches to keyboard and mouse control automatically.

**The Mac app (recommended).** Build the app bundle once, then open it:

```sh
./scripts/build-mac-app.sh
open dist/PocketPad.app
```

A window shows a QR code for pairing, the pairing code and the connection
status. To pass options, use `--args`:

```sh
open dist/PocketPad.app --args -profile racing
```

Run **`scripts/make-signing-cert.sh` once**: it creates a local
"PocketPad Dev" signing identity, and the build script signs every build
with it. The Accessibility grant is tied to the app's signature, so a
stable identity keeps the permission across every rebuild; without it, each
update needs a fresh grant.

**Accessibility permission (macOS 26).** Keyboard and mouse control needs
permission once. When macOS asks, or in System Settings > Privacy & Security
> Accessibility, turn on **PocketPad** if you use the Mac app, or
**Terminal** if you run `./pocketpad serve` from Terminal. If input does not
start within a few seconds, quit the receiver and open it again.

### Linux

There is no virtual gamepad backend for Linux yet. The receiver runs in
diagnostic mode, which shows every input live in the terminal:

```sh
./pocketpad serve -backend mock
```

### What the receiver shows

When it starts, the receiver prints the address the phone should use and
the pairing code:

```
PocketPad receiver dev listening on UDP port 47770 as "Studio-Mac"
  address for the phone: 192.168.1.20:47770

  Pairing code:  XXXX-XXXX-XXXX-XXXX-XXXX-XXXX
```

Keep the pairing code private: anyone on your network who has it can send
input to this computer. To issue a new code, which disconnects every paired
phone, run:

```sh
./pocketpad pair -reset
```

## Install the phone app

Download the APK from the `pocketpad-android-debug` artifact of the latest CI
run and install it on the phone, or build and install it over USB:

```sh
cd android
./gradlew :app:assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

## Pair and connect

1. Open PocketPad on the phone. Receivers on the network appear
   automatically.
2. Pair using one of these methods:
   - **QR code:** tap **Scan QR code** and scan the code in the Mac app
     window. The address and pairing code are filled in for you.
   - **Pairing code:** tap the discovered computer and type the pairing
     code. If the computer is not listed, enter its address by hand.
3. Tap **Connect**.

The pairing code is saved for each computer, so later connections take one
tap.

## Playing

The controller screen opens in landscape. It has two analog sticks, two
analog triggers, A, B, X and Y, a D-pad, two bumpers, Back, Start, Guide and
both stick clicks. Any number of controls can be pressed at the same time.

The bar at the top shows the connection status, round-trip time, update rate
and the phone's battery level.

When you leave the controller screen, lock the phone or lose the connection,
the computer releases every input within 300 ms. When you come back, the
app reconnects automatically, and no button comes back pressed.

**Rumble.** On Windows, games can vibrate the phone. Rumble is not available
on macOS or in Bluetooth mode. To turn it off, use **Vibrate on rumble** in
Settings.

## Keyboard and mouse profiles (macOS 26)

In keyboard and mouse mode, the left stick works as four keys: it presses a
key once it is pushed past about a third of its range. Triggers press when
held past halfway. The right stick moves the pointer, except in the `racing`
profile.

Choose a profile when starting the receiver:

```sh
./pocketpad serve -profile gta
```

| Profile | Intended for |
|---|---|
| `general` (default) | The desktop, browsers and most keyboard and mouse games |
| `racing` | Arcade racing games: triggers act as throttle and brake |
| `gta` | GTA V and similar third-person games |

The app shows the same tables under **Controls**.

### general

| Control | Key or action | Control | Key or action |
|---|---|---|---|
| Left stick | W, A, S, D | Right stick | Mouse pointer |
| LT | Left click | RT | Right click |
| A | Space | B | Left Shift |
| X | E | Y | Q |
| LB | Tab | RB | Esc |
| Back | M | Start | Return |
| Guide | F1 | LS / RS | C / R |
| D-pad | Arrow keys | | |

### racing

| Control | Key or action | Control | Key or action |
|---|---|---|---|
| RT | W (accelerate) | LT | S (brake, reverse) |
| Left stick | W, A, S, D (steer) | Right stick | Not used |
| A | Space (handbrake) | B | Left Shift |
| X | C (camera) | Y | R (reset car) |
| LB / RB | Q / E (look left, right) | D-pad | Arrow keys |
| Back | M (map) | Start | Return |
| Guide | Esc | LS / RS | H (horn) / V |

### gta

| Control | Key or action | Control | Key or action |
|---|---|---|---|
| Left stick | W, A, S, D (move) | Right stick | Mouse (look) |
| LT | Right click (aim) | RT | Left click (fire) |
| A | Space (jump) | B | Left Shift (sprint) |
| X | F (enter or exit vehicle) | Y | Q (cover) |
| LB | E (interact, horn) | RB | V (camera) |
| Back | M (interaction menu) | Start | Esc (pause) |
| Guide | Tab | LS / RS | C / R |
| D-pad | Arrow keys | | |

Because keys are either pressed or not, steering and throttle are on or off
in this mode. For analog control, use Bluetooth mode or a Windows computer.

To add a profile, copy an entry in `macKeyProfiles` in
`desktop/internal/gamepad/mackeys.go` and change the mapping.

## Bluetooth mode

In Bluetooth mode the phone pairs with the computer as a standard game
controller. Nothing needs to run on the computer, and analog sticks and
triggers are kept.

1. In the app, switch the mode to **Bluetooth** and grant the Bluetooth
   permissions.
2. Make the phone discoverable when the app asks.
3. On the computer, add **PocketPad** in its Bluetooth settings.
4. In the app, pick the computer from the list of paired devices and
   connect. Next time, pick it again; there is no need to pair again.

Bluetooth mode needs the Bluetooth HID Device profile, which many phones do
not include. If the phone lacks it, the app reports "This phone does not
support Bluetooth controller mode" within about five seconds; use Wi-Fi mode
instead. Rumble is not available over Bluetooth.

Some games that only use Apple's GameController framework do not recognise a
generic Bluetooth controller. Steam, SDL-based games and browsers do.

## App settings

| Setting | Effect |
|---|---|
| Stick dead zone | How far a stick must move before it registers |
| Analog triggers | Off: any touch on a trigger is a full press |
| Updates per second | How often input is sent while you are moving, capped by the receiver (250 by default) |
| Haptic tick on buttons | A short vibration when a button is pressed |
| Vibrate on rumble | Lets games vibrate the phone (Windows only) |
| Layout size | Scales the on-screen controls |
| Left-handed layout | Mirrors the layout |
| Stick click by double-tap | Off: use the separate L3 and R3 buttons |

**Reset to defaults** restores all settings.

## Receiver options

Run `pocketpad serve -h` for the full list.

| Option | Default | Purpose |
|---|---|---|
| `-backend` | `auto` | `auto`, `vigem` (Windows), `machid` or `mackeys` (macOS), or `mock` |
| `-profile` | `general` | Keyboard and mouse profile: `general`, `racing`, `gta` |
| `-name` | Host name | Name shown on the phone |
| `-listen` | `:47770` | UDP address to listen on |
| `-rate` | `250` | Highest input rate, in Hz, requested from the phone |
| `-watchdog` | `300ms` | Release held inputs after this long without input |
| `-session-timeout` | `3s` | End the session after this long without any packet |
| `-diag` | `auto` | Live input display: `auto`, `live`, `log` or `off` |
| `-v` | Off | Verbose logging |

Other commands:

| Command | Purpose |
|---|---|
| `pocketpad pair` | Show the pairing code |
| `pocketpad pair -reset` | Generate a new pairing code |
| `pocketpad app` | macOS window with the pairing QR code (used by `PocketPad.app`) |
| `pocketpad backends` | List the available backends |
| `pocketpad version` | Show the version |

## Troubleshooting

| Problem | Solution |
|---|---|
| The phone says "No answer from the computer" | Check that both devices are on the same Wi-Fi network, the address is correct and UDP port 47770 is allowed through the firewall. Re-enter the pairing code from `pocketpad pair`. |
| "Another phone is already connected" | A receiver serves one phone at a time. Disconnect the other phone first. |
| Windows: the controller does not appear in `joy.cpl` | Install ViGEmBus and place `ViGEmClient.dll` next to `pocketpad.exe`, then restart the receiver. |
| macOS: nothing moves | Grant Accessibility to PocketPad (Mac app) or Terminal (`serve`), then quit and reopen the receiver. |
| macOS: "macOS refused to create the virtual HID device" | Expected on macOS 26 when `-backend machid` is requested. Run `./pocketpad serve` without `-backend` to use keyboard and mouse control. |
| "This phone does not support Bluetooth controller mode" | The phone lacks the Bluetooth HID Device profile. Use Wi-Fi mode. |
| Input is choppy or delayed | Use 5 GHz Wi-Fi and stay close to the router. The round-trip time in the app should normally be a few milliseconds. |
| A button seemed stuck after switching apps | The receiver releases every input within 300 ms when input stops. No action is needed. |
