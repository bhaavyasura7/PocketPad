#!/bin/sh
# Creates a self-signed code-signing certificate ("PocketPad Dev") in the
# login keychain. macOS ties the Accessibility permission to an app's code
# signature, so signing every build with the same certificate keeps the
# user's grant across updates. Run once per machine; the certificate stays
# in the keychain.
set -eu

NAME="PocketPad Dev"

if security find-identity -v -p codesigning 2>/dev/null | grep -q "$NAME"; then
	echo "A \"$NAME\" identity already exists; nothing to do."
	exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/req.cnf" <<'EOF'
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = PocketPad Dev
O = PocketPad
[ext]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
EOF

openssl req -new -newkey rsa:2048 -x509 -days 3650 -nodes \
	-keyout "$TMP/key.pem" -out "$TMP/cert.pem" -config "$TMP/req.cnf" >/dev/null 2>&1

# Import the PEM key and certificate directly; a PKCS#12 round trip fails
# between OpenSSL's modern MAC defaults and Apple's Security framework.
security import "$TMP/key.pem" -k "$HOME/Library/Keychains/login.keychain-db" \
	-T /usr/bin/codesign -T /usr/bin/security >/dev/null
security import "$TMP/cert.pem" -k "$HOME/Library/Keychains/login.keychain-db" \
	-T /usr/bin/codesign >/dev/null

# User-domain trust for code signing (no admin rights needed).
security add-trusted-cert -r trustRoot -p codeSign -k "$HOME/Library/Keychains/login.keychain-db" \
	"$TMP/cert.pem" 2>/dev/null || echo "note: trust settings skipped; signing usually still works"

echo "Available code-signing identities:"
security find-identity -v -p codesigning | grep "$NAME"
