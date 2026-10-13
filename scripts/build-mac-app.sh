#!/bin/sh
# Builds dist/PocketPad.app — double-click to run. macOS only.
set -eu
cd "$(dirname "$0")/.."

# Optional environment:
#   VERSION=v0.1.0   version shown by "pocketpad version" and in Finder
#   UNIVERSAL=1      build one binary for Apple Silicon and Intel Macs
VERSION="${VERSION:-dev}"
BUNDLE_VERSION="$(printf '%s' "$VERSION" | sed 's/^v//')"
[ "$BUNDLE_VERSION" = "dev" ] && BUNDLE_VERSION="1.0"

APP="dist/PocketPad.app"
BIN="$APP/Contents/MacOS/pocketpad"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"

LDFLAGS="-s -w -X main.version=$VERSION"
if [ "${UNIVERSAL:-0}" = "1" ]; then
	TMP_BUILD="$(mktemp -d)"
	CGO_ENABLED=1 GOARCH=arm64 CC="clang -arch arm64" go build -C desktop -trimpath \
		-ldflags "$LDFLAGS" -o "$TMP_BUILD/pocketpad-arm64" ./cmd/pocketpad
	CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64" go build -C desktop -trimpath \
		-ldflags "$LDFLAGS" -o "$TMP_BUILD/pocketpad-amd64" ./cmd/pocketpad
	lipo -create -output "$BIN" "$TMP_BUILD/pocketpad-arm64" "$TMP_BUILD/pocketpad-amd64"
	rm -rf "$TMP_BUILD"
else
	CGO_ENABLED=1 go build -C desktop -trimpath -ldflags "$LDFLAGS" \
		-o "../$BIN" ./cmd/pocketpad
fi

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

cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>PocketPad</string>
	<key>CFBundleDisplayName</key><string>PocketPad</string>
	<key>CFBundleIdentifier</key><string>io.pocketpad.macapp</string>
	<key>CFBundleVersion</key><string>$BUNDLE_VERSION</string>
	<key>CFBundleShortVersionString</key><string>$BUNDLE_VERSION</string>
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
	# Ad-hoc signature: required for the bundle to launch on Apple Silicon,
	# but it changes with every build.
	codesign --force --deep --sign - "$APP"
	echo "Note: run scripts/make-signing-cert.sh once so future rebuilds keep"
	echo "the macOS Accessibility permission (otherwise it must be re-granted)."
fi

echo "Open it with:  open $APP          (or double-click in Finder)"
echo "Options, e.g.: open $APP --args -profile racing"
