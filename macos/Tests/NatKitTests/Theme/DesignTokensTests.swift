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
                    palette.windowBg.hex, palette.controlBg.hex, palette.rowAltBg.hex,
                    palette.controlFace.hex, palette.fieldBg.hex, palette.label.hex,
                    palette.labelSecondary.hex, palette.labelTertiary.hex,
                    palette.hoverWash.hex, palette.labelQuaternary.hex,
                    palette.accent.hex, palette.accentText.hex,
                    palette.systemOrange.hex, palette.systemYellow.hex, palette.systemGreen.hex,
                    palette.systemRed.hex, palette.systemBlue.hex, palette.systemPink.hex,
                    palette.systemTeal.hex, palette.systemGray.hex,
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
        let accent = try? XCTUnwrap(rgbComponents(hex: Palette.mocha.accent.hex))
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
        let keys: [(String, NSColor, (Palette) -> String)] = [
            ("windowBg", DesignTokens.dynamicNSColor(\.windowBg), { $0.windowBg.hex }),
            ("controlBg", DesignTokens.dynamicNSColor(\.controlBg), { $0.controlBg.hex }),
            ("rowAltBg", DesignTokens.dynamicNSColor(\.rowAltBg), { $0.rowAltBg.hex }),
            ("controlFace", DesignTokens.dynamicNSColor(\.controlFace), { $0.controlFace.hex }),
            ("fieldBg", DesignTokens.dynamicNSColor(\.fieldBg), { $0.fieldBg.hex }),
            ("hoverWash", DesignTokens.dynamicNSColor(\.hoverWash), { $0.hoverWash.hex }),
            ("terminalBg", DesignTokens.dynamicNSColor(\.terminalBg), { $0.terminalBg.hex }),
            ("label", DesignTokens.dynamicNSColor(\.label), { $0.label.hex }),
            ("labelSecondary", DesignTokens.dynamicNSColor(\.labelSecondary), { $0.labelSecondary.hex }),
            ("labelTertiary", DesignTokens.dynamicNSColor(\.labelTertiary), { $0.labelTertiary.hex }),
            ("labelQuaternary", DesignTokens.dynamicNSColor(\.labelQuaternary), { $0.labelQuaternary.hex }),
            ("accent", DesignTokens.dynamicNSColor(\.accent), { $0.accent.hex }),
            ("accentText", DesignTokens.dynamicNSColor(\.accentText), { $0.accentText.hex }),
            ("systemOrange", DesignTokens.dynamicNSColor(\.systemOrange), { $0.systemOrange.hex }),
            ("systemYellow", DesignTokens.dynamicNSColor(\.systemYellow), { $0.systemYellow.hex }),
            ("systemGreen", DesignTokens.dynamicNSColor(\.systemGreen), { $0.systemGreen.hex }),
            ("systemRed", DesignTokens.dynamicNSColor(\.systemRed), { $0.systemRed.hex }),
            ("systemBlue", DesignTokens.dynamicNSColor(\.systemBlue), { $0.systemBlue.hex }),
            ("systemPink", DesignTokens.dynamicNSColor(\.systemPink), { $0.systemPink.hex }),
            ("systemTeal", DesignTokens.dynamicNSColor(\.systemTeal), { $0.systemTeal.hex }),
            ("systemGray", DesignTokens.dynamicNSColor(\.systemGray), { $0.systemGray.hex }),
        ]
        for (name, token, value) in keys {
            assertResolves(token, .darkAqua, to: value(.mocha), name: "\(name) (dark)")
            assertResolves(token, .aqua, to: value(.latte), name: "\(name) (light)")
        }
    }

    /// Every derived colour — a rule, a wash, a band — resolves per
    /// appearance to exactly what the palette derives, and resolves *opaque*.
    ///
    /// The opacity these replaced was the bug: a wash laid down behind an
    /// alpha shows whatever happens to be behind it, so one `separator`
    /// rendered as a different colour in every pane it landed in, none of
    /// them a colour the theme chose. Mixed into a named ground instead, it
    /// is one value the theme decided and this asserts it is that value.
    func testDerivedColorsResolveOpaquePerAppearance() {
        var checks: [(String, NSColor, (Palette) -> String)] = [
            ("bandBg", NSColor(DesignTokens.fill(.band)), { $0.bandBg.hex }),
            ("headerBg", NSColor(DesignTokens.fill(.header)), { $0.headerBg.hex }),
            ("onAccentSeparator", NSColor(DesignTokens.onAccentSeparator), { $0.onAccentRule.hex }),
        ]
        for ground in Ground.allCases {
            for (name, weight) in [("hairline", RuleWeight.hairline), ("separator", .separator), ("controlBorder", .border)] {
                let token = [
                    "hairline": DesignTokens.hairline(on: ground),
                    "separator": DesignTokens.separator(on: ground),
                    "controlBorder": DesignTokens.controlBorder(on: ground),
                ][name]!
                checks.append(("\(name) on \(ground.rawValue)", NSColor(token), { $0.rule(weight, on: ground).hex }))
            }
            checks.append(("selectionWash on \(ground.rawValue)", NSColor(DesignTokens.selectionWash(on: ground)),
                           { $0.wash(.selection, of: $0.accent, on: ground).hex }))
            checks.append(("avatarWash on \(ground.rawValue)", NSColor(DesignTokens.avatarWash(on: ground)),
                           { $0.wash(.avatar, of: $0.accent, on: ground).hex }))
            checks.append(("accentMuted on \(ground.rawValue)", NSColor(DesignTokens.accentMuted(on: ground)),
                           { $0.wash(.muted, of: $0.accent, on: ground).hex }))
            checks.append(("accentWash on \(ground.rawValue)", NSColor(DesignTokens.accentWash(on: ground)),
                           { $0.wash(.chip, of: $0.accent, on: ground).hex }))
            checks.append(("systemRedWash on \(ground.rawValue)", NSColor(DesignTokens.systemRedWash(on: ground)),
                           { $0.wash(.chip, of: $0.systemRed, on: ground).hex }))
            checks.append(("systemGreenWash on \(ground.rawValue)", NSColor(DesignTokens.systemGreenWash(on: ground)),
                           { $0.wash(.chip, of: $0.systemGreen, on: ground).hex }))
            checks.append(("systemYellowWash on \(ground.rawValue)", NSColor(DesignTokens.systemYellowWash(on: ground)),
                           { $0.wash(.chip, of: $0.systemYellow, on: ground).hex }))
            checks.append(("systemOrangeWash on \(ground.rawValue)", NSColor(DesignTokens.systemOrangeWash(on: ground)),
                           { $0.wash(.chip, of: $0.systemOrange, on: ground).hex }))
            checks.append(("diffAddedRowBg on \(ground.rawValue)", NSColor(DesignTokens.diffAddedRowBg(on: ground)),
                           { $0.wash(.diffRow, of: $0.systemGreen, on: ground).hex }))
            checks.append(("diffRemovedRowBg on \(ground.rawValue)", NSColor(DesignTokens.diffRemovedRowBg(on: ground)),
                           { $0.wash(.diffRow, of: $0.systemRed, on: ground).hex }))
            checks.append(("diffAddedGutterBg on \(ground.rawValue)", NSColor(DesignTokens.diffAddedGutterBg(on: ground)),
                           { $0.wash(.diffGutter, of: $0.systemGreen, on: ground).hex }))
            checks.append(("diffRemovedGutterBg on \(ground.rawValue)", NSColor(DesignTokens.diffRemovedGutterBg(on: ground)),
                           { $0.wash(.diffGutter, of: $0.systemRed, on: ground).hex }))
            checks.append(("diffCommentGutterBg on \(ground.rawValue)", NSColor(DesignTokens.diffCommentGutterBg(on: ground)),
                           { $0.wash(.comment, of: $0.accent, on: ground).hex }))
            checks.append(("skeletonHighlight on \(ground.rawValue)", NSColor(DesignTokens.skeletonHighlight(on: ground)),
                           { $0.skeletonHighlight(on: ground.surface(in: $0)).hex }))
        }
        for (name, token, value) in checks {
            for (appearance, palette) in [(NSAppearance.Name.darkAqua, Palette.mocha), (.aqua, .latte)] {
                assertResolves(token, appearance, to: value(palette), name: "\(name) \(appearance.rawValue)")
                XCTAssertEqual(
                    resolve(token, appearance)?.alphaComponent, 1,
                    "\(name) \(appearance.rawValue): a derived colour should be opaque"
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
        for ground in Ground.allCases { _ = DesignTokens.fill(ground) }
        _ = DesignTokens.label
        _ = DesignTokens.labelSecondary
        _ = DesignTokens.labelTertiary
        _ = DesignTokens.labelQuaternary
        _ = DesignTokens.accent
        _ = DesignTokens.accentText
        _ = DesignTokens.hairline(on: .window)
        _ = DesignTokens.selectionWash(on: .window)
        _ = DesignTokens.separator(on: .window)
        _ = DesignTokens.controlBorder(on: .window)
        _ = DesignTokens.brandGradient
        _ = DesignTokens.systemOrange
        _ = DesignTokens.systemYellow
        _ = DesignTokens.systemGreen
        _ = DesignTokens.systemRed
        _ = DesignTokens.systemBlue
        _ = DesignTokens.systemPink
        _ = DesignTokens.systemTeal
        _ = DesignTokens.systemGray
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

    // MARK: - Terminal type

    /// The pane is the app writing code on screen, so it writes it in the
    /// font the diff pane writes code in — same family, same size. A
    /// terminal a point off the ramp is the thing this exists to stop.
    func testTerminalFontIsTheRampsCodeFace() {
        let font = TerminalType.font
        XCTAssertEqual(font.pointSize, Typo.code)
        XCTAssertEqual(font, NSFont.monospacedSystemFont(ofSize: Typo.code, weight: .regular))
    }

    /// A monospaced face, because every column of a terminal is one cell
    /// wide and a proportional one would not line up at all.
    func testTerminalFontIsMonospaced() {
        XCTAssertTrue(TerminalType.font.isFixedPitch)
    }

    /// The crispness fix, stated as the assertion it is: smoothing fattens
    /// every stroke, which on the pane's dark ground reads as a halo rather
    /// than as weight. Nothing else in the window draws with it.
    func testTerminalDoesNotSmoothFonts() {
        XCTAssertFalse(TerminalType.smoothsFonts)
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
                contrast(palette.label.hex, palette.hoverWash.hex), 4.5,
                "\(name): a label on the hover fill should clear AA"
            )
            XCTAssertLessThan(
                contrast(palette.label.hex, palette.labelQuaternary.hex), 4.5,
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
