#!/bin/bash

# Signs and notarizes the plugins make-plugins.sh built, then rewrites
# nat-plugins.json's digests to the signed binaries — release-app.sh's
# sibling for the plugins a release carries.
#
# Signing is what lets a downloaded plugin run at all: a binary fetched from
# the internet can carry the quarantine flag, and Gatekeeper blocks an
# unsigned, unnotarized one the moment nat execs it. A bare executable can't
# be stapled, so notarization is checked online on first run instead.
#
# Inputs, all via env:
#   SIGNING_IDENTITY   "Developer ID Application: Name (TEAMID)"; unset
#                      skips signing and notarizing (a local dry run), and
#                      the manifest keeps make-plugins.sh's digests
#   ASC_KEY_ID         App Store Connect API key ID (notarization)
#   ASC_ISSUER_ID      App Store Connect issuer ID (notarization)
#   ASC_API_KEY_PATH   path to the ASC API key's .p8 file
#   PLUGINS_DIR        where make-plugins.sh staged them (default
#                      .build/plugins)
#
# Requires: make-plugins.sh run first.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGINS_DIR="${PLUGINS_DIR:-$SCRIPT_DIR/.build/plugins}"
MANIFEST="$PLUGINS_DIR/nat-plugins.json"

if [ ! -f "$MANIFEST" ]; then
    echo "error: $MANIFEST not found — run make-plugins.sh first" >&2
    exit 1
fi

ASSETS=()
while IFS= read -r ASSET; do
    ASSETS+=("$ASSET")
done < <(jq -r '.plugins[].asset' "$MANIFEST")

if [ "${#ASSETS[@]}" -eq 0 ]; then
    echo "No plugins to sign."
    exit 0
fi
if [ -z "$SIGNING_IDENTITY" ]; then
    echo "SIGNING_IDENTITY unset: leaving ${#ASSETS[@]} plugin(s) unsigned."
    exit 0
fi
: "${ASC_KEY_ID:?ASC_KEY_ID must be set}"
: "${ASC_ISSUER_ID:?ASC_ISSUER_ID must be set}"
: "${ASC_API_KEY_PATH:?ASC_API_KEY_PATH must be set}"

echo "Signing ${#ASSETS[@]} plugin(s)..."
for ASSET in "${ASSETS[@]}"; do
    # Hardened runtime and a timestamp, as release-app.sh signs nat.
    codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$PLUGINS_DIR/$ASSET"
    codesign --verify --strict "$PLUGINS_DIR/$ASSET"
done

echo "Submitting plugins for notarization..."
# notarytool takes a zip, not a bare executable; the zip is only the
# envelope, and is thrown away once Apple has the ticket.
NOTARY_ZIP="$(mktemp -d)/plugins.zip"
trap 'rm -rf "$(dirname "$NOTARY_ZIP")"' EXIT
(cd "$PLUGINS_DIR" && zip -q "$NOTARY_ZIP" "${ASSETS[@]}")
xcrun notarytool submit "$NOTARY_ZIP" \
    --key "$ASC_API_KEY_PATH" \
    --key-id "$ASC_KEY_ID" \
    --issuer "$ASC_ISSUER_ID" \
    --wait --timeout 30m

echo "Rewriting nat-plugins.json's digests to the signed binaries..."
# codesign rewrote every binary, so the digests make-plugins.sh took would
# match nothing anyone downloads.
UPDATED="$(mktemp)"
cp "$MANIFEST" "$UPDATED"
for ASSET in "${ASSETS[@]}"; do
    SUM="$(shasum -a 256 "$PLUGINS_DIR/$ASSET" | cut -d' ' -f1)"
    jq --arg asset "$ASSET" --arg sha "$SUM" \
        '.plugins |= map(if .asset == $asset then .sha256 = $sha else . end)' "$UPDATED" > "$UPDATED.next"
    mv "$UPDATED.next" "$UPDATED"
done
mv "$UPDATED" "$MANIFEST"

echo "✓ Signed and notarized ${#ASSETS[@]} plugin(s) at $PLUGINS_DIR"
