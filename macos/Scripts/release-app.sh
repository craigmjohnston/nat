#!/bin/bash

# Signs the .app make-app.sh built, packages it into a notarized .dmg, and
# staples the ticket — the local half of a release, runnable end-to-end
# before any of it runs in CI.
#
# Inputs, all via env:
#   SIGNING_IDENTITY   "Developer ID Application: Name (TEAMID)"
#   APP_VERSION        the version this dmg is named for, e.g. 1.0.42
#   ASC_KEY_ID         App Store Connect API key ID (notarization)
#   ASC_ISSUER_ID      App Store Connect issuer ID (notarization)
#   ASC_API_KEY_PATH   path to the ASC API key's .p8 file
#
# Requires: the app already built at .build/gnat.app (run make-app.sh first).

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$SCRIPT_DIR/.build"
APP_NAME="gnat"
APP_BUNDLE="$BUILD_DIR/$APP_NAME.app"

: "${SIGNING_IDENTITY:?SIGNING_IDENTITY must be set (e.g. 'Developer ID Application: Name (TEAMID)')}"
: "${APP_VERSION:?APP_VERSION must be set (used to name the dmg)}"
: "${ASC_KEY_ID:?ASC_KEY_ID must be set}"
: "${ASC_ISSUER_ID:?ASC_ISSUER_ID must be set}"
: "${ASC_API_KEY_PATH:?ASC_API_KEY_PATH must be set}"

if [ ! -d "$APP_BUNDLE" ]; then
    echo "error: $APP_BUNDLE not found — run make-app.sh first" >&2
    exit 1
fi

DMG_PATH="$BUILD_DIR/$APP_NAME-$APP_VERSION.dmg"
SPARKLE_FRAMEWORK="$APP_BUNDLE/Contents/Frameworks/Sparkle.framework"

sign() {
    codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$@"
}

echo "Signing inside-out (never --deep, so every nested item gets exactly this identity and hardened runtime)..."

# Sparkle's own nested helpers, deepest first — notarization rejects the
# framework if any of these is missing a hardened-runtime signature.
# Downloader.xpc ships with its own sandbox entitlements already embedded;
# --preserve-metadata=entitlements is what keeps a fresh sign from dropping
# them.
sign "$SPARKLE_FRAMEWORK/Versions/B/XPCServices/Installer.xpc"
sign --preserve-metadata=entitlements "$SPARKLE_FRAMEWORK/Versions/B/XPCServices/Downloader.xpc"
sign "$SPARKLE_FRAMEWORK/Versions/B/Autoupdate"
sign "$SPARKLE_FRAMEWORK/Versions/B/Updater.app"
sign "$SPARKLE_FRAMEWORK"

# The bundled nat, then the app bundle itself last — signing the bundle
# without --deep seals its resources and the gnat executable, and leaves
# every nested item exactly as it was just signed above.
sign "$APP_BUNDLE/Contents/MacOS/nat"
sign "$APP_BUNDLE"

echo "Verifying signature..."
codesign --verify --strict "$APP_BUNDLE"

echo "Building dmg..."
STAGING_DIR="$(mktemp -d)"
trap 'rm -rf "$STAGING_DIR"' EXIT
cp -R "$APP_BUNDLE" "$STAGING_DIR/"
ln -s /Applications "$STAGING_DIR/Applications"
rm -f "$DMG_PATH"
hdiutil create -volname "$APP_NAME" -srcfolder "$STAGING_DIR" -ov -format UDZO "$DMG_PATH"

echo "Signing dmg..."
sign "$DMG_PATH"

echo "Submitting for notarization (this can take a while)..."
# On failure, `xcrun notarytool log <submission-id> --key "$ASC_API_KEY_PATH"
# --key-id "$ASC_KEY_ID" --issuer "$ASC_ISSUER_ID"` prints exactly which
# nested item Apple rejected and why — notarytool's own submit output names
# the id.
xcrun notarytool submit "$DMG_PATH" \
    --key "$ASC_API_KEY_PATH" \
    --key-id "$ASC_KEY_ID" \
    --issuer "$ASC_ISSUER_ID" \
    --wait --timeout 30m

echo "Stapling ticket..."
xcrun stapler staple "$DMG_PATH"
xcrun stapler validate "$DMG_PATH"

echo "✓ Notarized dmg at $DMG_PATH"
