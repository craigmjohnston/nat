import XCTest
import AppKit
import SwiftUI
@testable import NatKit

final class DesignTokensTests: XCTestCase {
    // MARK: - Hex Color Initializer Tests

    func testHexColorInitializerValidBlack() {
        assertColor("000000", isRGB: (0, 0, 0))
    }

    func testHexColorInitializerValidWhite() {
        assertColor("ffffff", isRGB: (1, 1, 1))
    }

    func testHexColorInitializerValidAccent() {
        // The accent color from the design tokens: #cba6f7
        assertColor("cba6f7", isRGB: (Double(0xcb) / 255, Double(0xa6) / 255, Double(0xf7) / 255))
    }

    func testHexColorInitializerValidWindowBg() {
        // Window background color: #1e1e2e
        assertColor("1e1e2e", isRGB: (Double(0x1e) / 255, Double(0x1e) / 255, Double(0x2e) / 255))
    }

    func testHexColorInitializerWithUppercase() {
        // Hex strings should work case-insensitively
        assertColor("CBA6F7", isRGB: (Double(0xcb) / 255, Double(0xa6) / 255, Double(0xf7) / 255))
    }

    func testHexColorInitializerWithLeadingHash() {
        // The initializer should strip the leading hash if present
        assertColor("#cba6f7", isRGB: (Double(0xcb) / 255, Double(0xa6) / 255, Double(0xf7) / 255))
    }

    func testHexColorInitializerTooShort() {
        // 4-character hex is invalid and falls back to the accent.
        assertColor("fff", isRGB: hexFallback)
    }

    func testHexColorInitializerTooLong() {
        // 8-character hex is invalid and falls back to the accent.
        assertColor("ffffffff", isRGB: hexFallback)
    }

    func testHexColorInitializerInvalidCharacters() {
        // Non-hex characters are invalid and fall back to the accent.
        assertColor("gggggg", isRGB: hexFallback)
    }

    /// The fallback is on the palette. It used to be white, the one colour
    /// in the app that belongs to neither theme — so the one frame a parse
    /// failure drew was guaranteed to be the most off-theme thing on screen.
    func testHexColorFallbackIsOnThePalette() {
        let palette = Set(
            [Palette.mocha, Palette.latte].flatMap { palette in
                palette.ansi + [
                    palette.windowBg, palette.controlBg, palette.rowAltBg,
                    palette.controlFace, palette.fieldBg, palette.label,
                    palette.labelSecondary, palette.labelTertiary,
                    palette.hoverWash, palette.labelQuaternary,
                    palette.accent, palette.accentText,
                    palette.systemOrange, palette.systemYellow, palette.systemGreen,
                    palette.systemRed, palette.systemBlue, palette.systemPink,
                    palette.systemTeal, palette.systemGray,
                ]
            }
        )
        let onPalette = palette.contains { hex in
            guard let rgb = rgbComponents(hex: hex) else { return false }
            return rgb == hexFallback
        }
        XCTAssertTrue(onPalette, "an unreadable colour should fall back to one the theme actually holds")
        XCTAssertNotEqual(hexFallback.red + hexFallback.green + hexFallback.blue, 3, "…and never to white")
    }

    /// Which one it is: Mocha's mauve, the app's own accent. The constant is
    /// written out channel by channel because the fallback for a parse
    /// cannot depend on a parse; this is what holds the two in step.
    func testHexFallbackIsTheAccent() {
        let accent = try? XCTUnwrap(rgbComponents(hex: Palette.mocha.accent))
        XCTAssertEqual(accent?.red, hexFallback.red)
        XCTAssertEqual(accent?.green, hexFallback.green)
        XCTAssertEqual(accent?.blue, hexFallback.blue)
    }

    /// The SwiftUI and AppKit initializers are one parse: a colour written
    /// once in a palette and read by both cannot mean two things.
    private func assertColor(
        _ hex: String,
        isRGB expected: (red: Double, green: Double, blue: Double),
        file: StaticString = #filePath,
        line: UInt = #line
    ) {
        let components = Color(hex: hex).cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components", file: file, line: line)
        XCTAssertEqual(Double(components[0]), expected.red, accuracy: 0.01, "Red", file: file, line: line)
        XCTAssertEqual(Double(components[1]), expected.green, accuracy: 0.01, "Green", file: file, line: line)
        XCTAssertEqual(Double(components[2]), expected.blue, accuracy: 0.01, "Blue", file: file, line: line)

        let native = NSColor(hex: hex)
        XCTAssertEqual(Double(native.redComponent), expected.red, accuracy: 0.01, "NSColor red", file: file, line: line)
        XCTAssertEqual(Double(native.greenComponent), expected.green, accuracy: 0.01, "NSColor green", file: file, line: line)
        XCTAssertEqual(Double(native.blueComponent), expected.blue, accuracy: 0.01, "NSColor blue", file: file, line: line)
        XCTAssertEqual(Double(native.alphaComponent), 1, accuracy: 0.01, "NSColor alpha", file: file, line: line)
    }

    // MARK: - Token resolution

    /// The seam every token is built over: which palette a colour scheme
    /// draws with.
    func testSchemeChoosesThePalette() {
        XCTAssertEqual(DesignTokens.palette(for: .dark), .mocha)
        XCTAssertEqual(DesignTokens.palette(for: .light), .latte)
    }

    /// The same choice made from the AppKit appearance a dynamic colour is
    /// handed when it resolves. Anything that is not positively dark
    /// resolves light, which is the platform's own default.
    func testAppearanceChoosesThePalette() {
        for name in [NSAppearance.Name.darkAqua, .vibrantDark] {
            let appearance = try? XCTUnwrap(NSAppearance(named: name))
            XCTAssertEqual(appearance.map(DesignTokens.palette(for:)), .mocha, "\(name.rawValue)")
        }
        for name in [NSAppearance.Name.aqua, .vibrantLight] {
            let appearance = try? XCTUnwrap(NSAppearance(named: name))
            XCTAssertEqual(appearance.map(DesignTokens.palette(for:)), .latte, "\(name.rawValue)")
        }
    }

    /// A token holds both palettes and hands over the one the appearance it
    /// is drawn under calls for — which is the whole of how the theme
    /// switch restyles the app, and how `system` follows macOS.
    func testTokensResolvePerAppearance() {
        let keys: [(String, KeyPath<Palette, String>)] = [
            ("windowBg", \.windowBg),
            ("controlBg", \.controlBg),
            ("rowAltBg", \.rowAltBg),
            ("controlFace", \.controlFace),
            ("fieldBg", \.fieldBg),
            ("hoverWash", \.hoverWash),
            ("terminalBg", \.terminalBg),
            ("label", \.label),
            ("labelSecondary", \.labelSecondary),
            ("labelTertiary", \.labelTertiary),
            ("labelQuaternary", \.labelQuaternary),
            ("accent", \.accent),
            ("accentText", \.accentText),
            ("systemOrange", \.systemOrange),
            ("systemYellow", \.systemYellow),
            ("systemGreen", \.systemGreen),
            ("systemRed", \.systemRed),
            ("systemBlue", \.systemBlue),
            ("systemPink", \.systemPink),
            ("systemTeal", \.systemTeal),
            ("systemGray", \.systemGray),
        ]
        for (name, key) in keys {
            let token = DesignTokens.dynamicNSColor(key)
            assertResolves(token, .darkAqua, to: Palette.mocha[keyPath: key], name: "\(name) (dark)")
            assertResolves(token, .aqua, to: Palette.latte[keyPath: key], name: "\(name) (light)")
        }
    }

    /// The tokens that are a colour behind an opacity resolve both halves
    /// per appearance: light ink at Mocha's alpha, dark ink at Latte's
    /// heavier one.
    func testOpacityTokensResolvePerAppearance() {
        let keys: [(String, KeyPath<Palette, String>, KeyPath<Palette, Double>)] = [
            ("headerBg", \.windowBg, \.headerOpacity),
            ("hairline", \.label, \.hairlineOpacity),
            ("separator", \.label, \.separatorOpacity),
            ("controlBorder", \.label, \.controlBorderOpacity),
            ("selectionWash", \.accent, \.selectionWashOpacity),
            ("headerAccentVeil", \.accent, \.headerAccentOpacity),
            ("bandBg", \.controlBg, \.bandOpacity),
            ("skeletonHighlight", \.label, \.skeletonHighlightOpacity),
            ("onAccentSeparator", \.accentText, \.onAccentSeparatorOpacity),
            ("accentMuted", \.accent, \.mutedAccentOpacity),
            ("avatarWash", \.accent, \.avatarWashOpacity),
            ("accentWash", \.accent, \.tintWashOpacity),
            ("systemRedWash", \.systemRed, \.tintWashOpacity),
            ("systemGreenWash", \.systemGreen, \.tintWashOpacity),
            ("systemYellowWash", \.systemYellow, \.tintWashOpacity),
            ("systemOrangeWash", \.systemOrange, \.tintWashOpacity),
            ("labelSecondaryWash", \.labelSecondary, \.tintWashOpacity),
            ("diffAddedRowBg", \.systemGreen, \.diffRowWashOpacity),
            ("diffRemovedRowBg", \.systemRed, \.diffRowWashOpacity),
            ("diffAddedGutterBg", \.systemGreen, \.diffGutterWashOpacity),
            ("diffRemovedGutterBg", \.systemRed, \.diffGutterWashOpacity),
            ("diffCommentGutterBg", \.accent, \.commentWashOpacity),
        ]
        for (name, key, opacity) in keys {
            let token = DesignTokens.dynamicNSColor(key, opacity: opacity)
            for (appearance, palette) in [(NSAppearance.Name.darkAqua, Palette.mocha), (.aqua, .latte)] {
                assertResolves(token, appearance, to: palette[keyPath: key], name: "\(name) \(appearance.rawValue)")
                let resolved = resolve(token, appearance)
                XCTAssertEqual(
                    Double(resolved?.alphaComponent ?? 0), palette[keyPath: opacity],
                    accuracy: 0.01,
                    "\(name) on \(appearance.rawValue) should carry that palette's own opacity"
                )
            }
        }
    }

    private func resolve(_ color: NSColor, _ name: NSAppearance.Name) -> NSColor? {
        guard let appearance = NSAppearance(named: name) else { return nil }
        var resolved: NSColor?
        appearance.performAsCurrentDrawingAppearance {
            resolved = color.usingColorSpace(.sRGB)
        }
        return resolved
    }

    private func assertResolves(
        _ color: NSColor,
        _ appearance: NSAppearance.Name,
        to hex: String,
        name: String,
        file: StaticString = #filePath,
        line: UInt = #line
    ) {
        guard let resolved = resolve(color, appearance) else {
            return XCTFail("\(name): could not resolve", file: file, line: line)
        }
        let expected = NSColor(hex: hex)
        XCTAssertEqual(Double(resolved.redComponent), Double(expected.redComponent), accuracy: 0.01, "\(name) red", file: file, line: line)
        XCTAssertEqual(Double(resolved.greenComponent), Double(expected.greenComponent), accuracy: 0.01, "\(name) green", file: file, line: line)
        XCTAssertEqual(Double(resolved.blueComponent), Double(expected.blueComponent), accuracy: 0.01, "\(name) blue", file: file, line: line)
    }

    // MARK: - DesignTokens Availability Tests

    func testDesignTokensColorAvailability() {
        // Verify that all design tokens are accessible
        _ = DesignTokens.windowBg
        _ = DesignTokens.controlBg
        _ = DesignTokens.rowAltBg
        _ = DesignTokens.fieldBg
        _ = DesignTokens.hoverWash
        _ = DesignTokens.controlFace
        _ = DesignTokens.terminalBg
        _ = DesignTokens.headerBg
        _ = DesignTokens.label
        _ = DesignTokens.labelSecondary
        _ = DesignTokens.labelTertiary
        _ = DesignTokens.labelQuaternary
        _ = DesignTokens.accent
        _ = DesignTokens.accentText
        _ = DesignTokens.hairline
        _ = DesignTokens.selectionWash
        _ = DesignTokens.separator
        _ = DesignTokens.controlBorder
        _ = DesignTokens.brandGradient
        _ = DesignTokens.systemOrange
        _ = DesignTokens.systemYellow
        _ = DesignTokens.systemGreen
        _ = DesignTokens.systemRed
        _ = DesignTokens.systemBlue
        _ = DesignTokens.systemPink
        _ = DesignTokens.systemTeal
        _ = DesignTokens.systemGray
        _ = DesignTokens.headerAccentVeil
        _ = DesignTokens.bandBg
        _ = DesignTokens.skeletonHighlight
        _ = DesignTokens.onAccentSeparator
        _ = DesignTokens.accentMuted
        _ = DesignTokens.avatarWash
        _ = DesignTokens.accentWash
        _ = DesignTokens.systemRedWash
        _ = DesignTokens.systemGreenWash
        _ = DesignTokens.systemYellowWash
        _ = DesignTokens.systemOrangeWash
        _ = DesignTokens.labelSecondaryWash
        _ = DesignTokens.diffAddedRowBg
        _ = DesignTokens.diffRemovedRowBg
        _ = DesignTokens.diffAddedGutterBg
        _ = DesignTokens.diffRemovedGutterBg
        _ = DesignTokens.diffCommentGutterBg
    }

    // MARK: - Button metrics

    // The point of the button grammar is that one submit is the shape of
    // every other, so what these assert is the invariants a call site would
    // otherwise be free to break: primary and secondary share a height and a
    // radius, a ghost button is on the same baseline, and the dimming reads
    // as dimming.

    func testButtonMetricsHeightAndRadiusArePositive() {
        XCTAssertGreaterThan(ButtonMetrics.height, 0)
        XCTAssertGreaterThan(ButtonMetrics.cornerRadius, 0)
        // A radius past half the height would round the ends into a capsule,
        // which is a different control.
        XCTAssertLessThanOrEqual(ButtonMetrics.cornerRadius, ButtonMetrics.height / 2)
    }

    func testButtonMetricsGhostIsInsetLessThanAFilledButton() {
        XCTAssertGreaterThan(ButtonMetrics.horizontalPadding, 0)
        XCTAssertGreaterThan(ButtonMetrics.ghostHorizontalPadding, 0)
        XCTAssertLessThan(ButtonMetrics.ghostHorizontalPadding, ButtonMetrics.horizontalPadding)
    }

    func testButtonMetricsOpacitiesDim() {
        XCTAssertGreaterThan(ButtonMetrics.disabledOpacity, 0)
        XCTAssertLessThan(ButtonMetrics.disabledOpacity, 1)
        XCTAssertGreaterThan(ButtonMetrics.pressedOpacity, 0)
        XCTAssertLessThan(ButtonMetrics.pressedOpacity, 1)
        // Disabled is the deeper of the two: pressed is a moment, unavailable
        // is a state.
        XCTAssertLessThan(ButtonMetrics.disabledOpacity, ButtonMetrics.pressedOpacity)
    }

    // MARK: - Type ramp

    func testTypoRampDescends() {
        XCTAssertGreaterThan(Typo.headline, Typo.body)
        XCTAssertGreaterThan(Typo.body, Typo.code)
        XCTAssertGreaterThan(Typo.code, Typo.subhead)
        XCTAssertGreaterThan(Typo.subhead, Typo.caption)
    }

    func testMotionStateChangeIsDefined() {
        XCTAssertNotNil(Motion.stateChange)
    }

    // MARK: - Hover

    /// The one thing the hover fill exists to guarantee: a row's label is
    /// still a label while the pointer is on it. This is a threshold rather
    /// than a comparison — unlike `PaletteTests`, which refuses to hold
    /// Catppuccin's own values to one — because what it tests is this app's
    /// choice of which swatch plays hover, not the swatch itself. The bar is
    /// WCAG AA for body text, and `labelQuaternary`, the ink this used to be
    /// filled with, is asserted to fail it: that is the bug the token was
    /// added for.
    func testLabelClearsAAOnTheHoverFill() {
        for (name, palette) in [("mocha", Palette.mocha), ("latte", Palette.latte)] {
            XCTAssertGreaterThanOrEqual(
                contrast(palette.label, palette.hoverWash), 4.5,
                "\(name): a label on the hover fill should clear AA"
            )
            XCTAssertLessThan(
                contrast(palette.label, palette.labelQuaternary), 4.5,
                "\(name): the ink the hover fill replaced should be why it was replaced"
            )
        }
    }

    /// WCAG's contrast ratio between two opaque colours.
    private func contrast(_ one: String, _ other: String) -> Double {
        let first = relativeLuminance(one)
        let second = relativeLuminance(other)
        return (max(first, second) + 0.05) / (min(first, second) + 0.05)
    }

    private func relativeLuminance(_ hex: String) -> Double {
        let rgb = rgbComponents(hex: hex) ?? (1, 1, 1)
        let channels = [rgb.red, rgb.green, rgb.blue].map { value -> Double in
            value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    }
}
