#!/bin/bash

# Renders the gnat app icons from their SVG design sources into the .icns
# files the app bundle (and the dev run's dock icon) uses: the paper icon for
# light mode (AppIcon.icns, also the bundle's Finder icon) and the dark-navy
# paper icon for dark mode (AppIconDark.icns, which the app swaps onto the
# dock while the appearance is dark). Run it after editing either source; the
# generated icns files are committed, so a machine without rsvg-convert can
# still build the app.
#
# It also renders each source's ink alone into AppIcon.icon, the layered icon
# macOS 26 draws itself — plate, rim and shadow are the system's there, and
# the light/dark choice follows the system with the app closed (make-app.sh
# compiles it into the bundle). icon.json beside the layers is written by
# hand: the two paper colours as its fill, the two layers as its one mark.
#
# Requires: rsvg-convert (brew install librsvg), and iconutil and sips
# (macOS's own).

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

# The ink of one source on a transparent 1024px canvas: the plate's shadow,
# mesh and rim hidden, and the sheet masked down to the ink so the paper it
# is blended into survives only under the line. The source's squircle is 824
# of its 1024 units, and a layer's canvas is the whole icon, so it is rendered
# at 1024/824 and the centre 1024px kept.
layer() {
    local svg="$1" png="$2"
    local tmp
    tmp="$(mktemp -d)"
    cat > "$tmp/layer.css" <<'CSS'
#plate-shadow, #mesh, #rim-stroke { display: none; }
#sheet { mask: url(#ink-only); }
CSS
    rsvg-convert -w 1273 -h 1273 -s "$tmp/layer.css" "$svg" -o "$tmp/layer.png"
    mkdir -p "$(dirname "$png")"
    sips -c 1024 1024 "$tmp/layer.png" --out "$png" >/dev/null
    rm -rf "$tmp"
    echo "✓ Layer written to $png"
}

render "$SCRIPT_DIR/Resources/gnat-paper.svg" "$OUT/AppIcon.icns"
render "$SCRIPT_DIR/Resources/gnat-paper-dark-navy.svg" "$OUT/AppIconDark.icns"
layer "$SCRIPT_DIR/Resources/gnat-paper.svg" "$SCRIPT_DIR/Resources/AppIcon.icon/Assets/mark.png"
layer "$SCRIPT_DIR/Resources/gnat-paper-dark-navy.svg" "$SCRIPT_DIR/Resources/AppIcon.icon/Assets/mark-dark.png"
