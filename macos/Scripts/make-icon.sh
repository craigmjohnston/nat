#!/bin/bash

# Renders the gnat app icon from its SVG design source into the .icns the app
# bundle (and the dev run's dock icon) uses. Run it after editing
# Resources/gnat.svg; the generated icns is committed, so a machine without
# rsvg-convert can still build the app.
#
# Requires: rsvg-convert (brew install librsvg) and iconutil (macOS's own).

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SVG="$SCRIPT_DIR/Resources/gnat.svg"
ICNS="$SCRIPT_DIR/Sources/NatApp/Resources/AppIcon.icns"
ICONSET="$(mktemp -d)/gnat.iconset"

mkdir -p "$ICONSET"

for size in 16 32 128 256 512; do
    rsvg-convert -w "$size" -h "$size" "$SVG" -o "$ICONSET/icon_${size}x${size}.png"
    double=$((size * 2))
    rsvg-convert -w "$double" -h "$double" "$SVG" -o "$ICONSET/icon_${size}x${size}@2x.png"
done

mkdir -p "$(dirname "$ICNS")"
iconutil -c icns "$ICONSET" -o "$ICNS"
rm -rf "$(dirname "$ICONSET")"

echo "✓ Icon written to $ICNS"
