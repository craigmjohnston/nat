#!/bin/bash

set -e

# Navigate to the macos directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$SCRIPT_DIR"

PACKAGE_PATH="$SCRIPT_DIR"
BUILD_DIR="$SCRIPT_DIR/.build"
RELEASE_DIR="$BUILD_DIR/release"
APP_NAME="NatApp"
APP_BUNDLE="$BUILD_DIR/NatApp.app"
EXECUTABLE_PATH="$APP_BUNDLE/Contents/MacOS/$APP_NAME"

echo "Building release binary..."
swift build -c release --package-path "$SCRIPT_DIR"

echo "Creating macOS app bundle..."
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"

# Copy the executable
cp "$RELEASE_DIR/$APP_NAME" "$EXECUTABLE_PATH"
chmod +x "$EXECUTABLE_PATH"

# Create minimal Info.plist
cat > "$APP_BUNDLE/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleExecutable</key>
	<string>NatApp</string>
	<key>CFBundleIdentifier</key>
	<string>com.craigmjohnston.nat.NatApp</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>nat</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSHighResolutionCapable</key>
	<true/>
	<key>LSMinimumSystemVersion</key>
	<string>15.0</string>
	<key>LSUIElement</key>
	<false/>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright 2025 Craig Johnston</string>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
</dict>
</plist>
EOF

echo "✓ App bundle created at $APP_BUNDLE"
