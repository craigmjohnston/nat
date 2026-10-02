#!/bin/bash

# Renders the gnat app icons from their SVG design sources into the .icns
# files the app bundle (and the dev run's dock icon) uses: the paper icon for
# light mode (AppIcon.icns, also the bundle's Finder icon) and the dark-navy
# paper icon for dark mode (AppIconDark.icns, which the app swaps onto the
# dock while the appearance is dark). Run it after editing either source; the
# generated icns files are committed, so a machine without rsvg-convert can
# still build the app.
#
# Requires: rsvg-convert (brew install librsvg) and iconutil (macOS's own).

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$SCRIPT_DIR/Sources/NatApp/Resources"

render() {
    local svg="$1" icns="$2"
    local iconset
    iconset="$(mktemp -d)/gnat.iconset"
    mkdir -p "$iconset"
    for size in 16 32 128 256 512; do
        rsvg-convert -w "$size" -h "$size" "$svg" -o "$iconset/icon_${size}x${size}.png"
        double=$((size * 2))
        rsvg-convert -w "$double" -h "$double" "$svg" -o "$iconset/icon_${size}x${size}@2x.png"
    done
    mkdir -p "$(dirname "$icns")"
    iconutil -c icns "$iconset" -o "$icns"
    rm -rf "$(dirname "$iconset")"
    echo "✓ Icon written to $icns"
}

render "$SCRIPT_DIR/Resources/gnat-paper.svg" "$OUT/AppIcon.icns"
render "$SCRIPT_DIR/Resources/gnat-paper-dark-navy.svg" "$OUT/AppIconDark.icns"
