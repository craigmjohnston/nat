#!/bin/bash

# Builds every task-source plugin in the repo's plugins/ into a universal
# nat-source-<name>, and writes the nat-plugins.json manifest a release
# carries beside them — the plugin-source format in
# docs/design/task-sources/README.md, "Installing plugins". `nat
# plugin-install` reads that manifest from the latest release and checks each
# binary it downloads against the sha256 written here.
#
# A plugin is a directory plugins/<name>/ with a plugin.json of
# {"name","title","description"}: static, because a plugin's own describe may
# need a token the release build doesn't have. No plugin directory at all
# still writes a manifest, with an empty plugins list.
#
# Inputs, all via env:
#   APP_VERSION   the release version, e.g. 1.0.42 (no v), stamped into each
#                 plugin as make-app.sh stamps nat, and the manifest's version
#   PLUGINS_DIR   where to stage the binaries and manifest (default
#                 .build/plugins)
#
# Signing changes a binary, so release-plugins.sh rewrites the digests once
# it has signed them; the ones written here are right for an unsigned build.
#
# Requires: go, lipo, jq, shasum.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_VERSION="${APP_VERSION:-1.0}"
PLUGINS_DIR="${PLUGINS_DIR:-$SCRIPT_DIR/.build/plugins}"
MANIFEST="$PLUGINS_DIR/nat-plugins.json"

rm -rf "$PLUGINS_DIR"
mkdir -p "$PLUGINS_DIR/per-arch"

# The same stamp nat gets in make-app.sh, so a plugin and the nat it was
# released beside report one version. A plugin that doesn't link
# internal/version simply has nothing for -X to set.
LDFLAGS="-X github.com/craigmjohnston/nat/internal/version.stamped=$APP_VERSION"

ENTRIES="$PLUGINS_DIR/entries.jsonl"
: > "$ENTRIES"
shopt -s nullglob
for META in "$REPO_ROOT"/plugins/*/plugin.json; do
    DIR="$(dirname "$META")"
    NAME="$(basename "$DIR")"
    # The directory is the plugin's name; a plugin.json that says otherwise
    # is a mistake to stop on, not one to publish.
    if [ "$(jq -r .name "$META")" != "$NAME" ]; then
        echo "error: $META names $(jq -r .name "$META"), not $NAME" >&2
        exit 1
    fi
    ASSET="nat-source-$NAME"
    echo "Building $ASSET (universal)..."
    # Per-arch and lipo'd, exactly as make-app.sh builds nat.
    (cd "$REPO_ROOT" && GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$PLUGINS_DIR/per-arch/$ASSET-arm64" "./plugins/$NAME")
    (cd "$REPO_ROOT" && GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$PLUGINS_DIR/per-arch/$ASSET-amd64" "./plugins/$NAME")
    lipo -create -output "$PLUGINS_DIR/$ASSET" \
        "$PLUGINS_DIR/per-arch/$ASSET-arm64" "$PLUGINS_DIR/per-arch/$ASSET-amd64"
    chmod +x "$PLUGINS_DIR/$ASSET"
    SUM="$(shasum -a 256 "$PLUGINS_DIR/$ASSET" | cut -d' ' -f1)"
    jq -c --arg asset "$ASSET" --arg sha "$SUM" \
        '{name, title, description, asset: $asset, sha256: $sha}' "$META" >> "$ENTRIES"
done

jq -s --arg version "$APP_VERSION" '{version: $version, plugins: .}' "$ENTRIES" > "$MANIFEST"
rm -rf "$PLUGINS_DIR/per-arch" "$ENTRIES"

echo "✓ $(jq '.plugins | length' "$MANIFEST") plugin(s) and nat-plugins.json at $PLUGINS_DIR"
