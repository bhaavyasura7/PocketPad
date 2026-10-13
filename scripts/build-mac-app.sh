#!/bin/sh
# Builds dist/PocketPad.app — double-click to run. macOS only.
set -eu
cd "$(dirname "$0")/.."

APP="dist/PocketPad.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"

CGO_ENABLED=1 go build -C desktop -trimpath -ldflags "-s -w" \
	-o "../$APP/Contents/MacOS/pocketpad" ./cmd/pocketpad

# Dock icon from the official logo in the repository root.
mkdir -p "$APP/Contents/Resources"
ICONSET="$(mktemp -d)/AppIcon.iconset"
mkdir -p "$ICONSET"
for SZ in 16 32 64 128 256 512; do
	sips -z "$SZ" "$SZ" PocketPad.png --out "$ICONSET/icon_${SZ}x${SZ}.png" >/dev/null
	DOUBLE=$((SZ * 2))
	sips -z "$DOUBLE" "$DOUBLE" PocketPad.png --out "$ICONSET/icon_${SZ}x${SZ}@2x.png" >/dev/null
done
sips -z 1024 1024 PocketPad.png --out "$ICONSET/icon_512x512@2x.png" >/dev/null
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
rm -rf "$(dirname "$ICONSET")"

cat > "$APP/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>PocketPad</string>
	<key>CFBundleDisplayName</key><string>PocketPad</string>
	<key>CFBundleIdentifier</key><string>io.pocketpad.macapp</string>
	<key>CFBundleVersion</key><string>1.0</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>CFBundleExecutable</key><string>pocketpad</string>
	<key>CFBundleIconFile</key><string>AppIcon</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
	<key>NSHumanReadableCopyright</key><string>PocketPad</string>
</dict>
</plist>
EOF

echo "Built $APP"

# Sign with the persistent local identity when it exists (run
# scripts/make-signing-cert.sh once). macOS ties the Accessibility grant to
# the signature, so a stable identity means the permission survives every
# rebuild and update.
CERT="PocketPad Dev"
if security find-identity -v -p codesigning 2>/dev/null | grep -q "$CERT"; then
	codesign --force --sign "$CERT" "$APP/Contents/MacOS/pocketpad"
	codesign --force --sign "$CERT" "$APP"
	echo "Signed with \"$CERT\": permission grants now survive rebuilds."
else
	echo "Note: run scripts/make-signing-cert.sh once so future rebuilds keep"
	echo "the macOS Accessibility permission (otherwise it must be re-granted)."
fi

echo "Open it with:  open $APP          (or double-click in Finder)"
echo "Options, e.g.: open $APP --args -profile racing"
