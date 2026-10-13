PocketPad turns an Android phone into a game controller for Windows and
macOS. Setup instructions are in
[docs/GETTING_STARTED.md](https://github.com/bhaavyasura7/PocketPad/blob/main/docs/GETTING_STARTED.md).

## Downloads

| File | For |
|---|---|
| `pocketpad-<version>.apk` | The Android app (Android 9 or later). A `-debug` suffix means it is signed with a development key. |
| `pocketpad-<version>-windows-amd64.zip` | Windows on most PCs |
| `pocketpad-<version>-windows-arm64.zip` | Windows on ARM |
| `pocketpad-<version>-macos.zip` | macOS (Apple Silicon and Intel): `PocketPad.app` and the `pocketpad` command |
| `pocketpad-<version>-linux-amd64.zip` | Linux (diagnostic mode only) |

Each archive also contains `ppsim`, a phone simulator for testing.

## Notes

- **Windows** needs the ViGEmBus driver and `ViGEmClient.dll`; see the
  getting started guide.
- **macOS** builds are not notarised. The first time, right-click
  `PocketPad.app` and choose Open, or allow it in System Settings > Privacy &
  Security. On macOS 26, keyboard and mouse control needs the Accessibility
  permission.
- **Bluetooth mode** is experimental and works only on phones with the
  Bluetooth HID Device profile.
