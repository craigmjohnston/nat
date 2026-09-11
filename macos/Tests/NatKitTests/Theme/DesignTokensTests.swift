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
        // 4-character hex should be treated as invalid and return white
        assertColor("fff", isRGB: (1, 1, 1))
    }

    func testHexColorInitializerTooLong() {
        // 8-character hex should be treated as invalid and return white
        assertColor("ffffffff", isRGB: (1, 1, 1))
    }

    func testHexColorInitializerInvalidCharacters() {
        // Non-hex characters should be treated as invalid
        assertColor("gggggg", isRGB: (1, 1, 1))
    }

    /// The SwiftUI and AppKit initializers are one parse: a colour written
    /// once in a palette and read by both cannot mean two things.
    private func assertColor(
        _ hex: String,
        isRGB expected: (Double, Double, Double),
        file: StaticString = #filePath,
        line: UInt = #line
    ) {
        let components = Color(hex: hex).cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components", file: file, line: line)
        XCTAssertEqual(Double(components[0]), expected.0, accuracy: 0.01, "Red", file: file, line: line)
        XCTAssertEqual(Double(components[1]), expected.1, accuracy: 0.01, "Green", file: file, line: line)
        XCTAssertEqual(Double(components[2]), expected.2, accuracy: 0.01, "Blue", file: file, line: line)

        let native = NSColor(hex: hex)
        XCTAssertEqual(Double(native.redComponent), expected.0, accuracy: 0.01, "NSColor red", file: file, line: line)
        XCTAssertEqual(Double(native.greenComponent), expected.1, accuracy: 0.01, "NSColor green", file: file, line: line)
        XCTAssertEqual(Double(native.blueComponent), expected.2, accuracy: 0.01, "NSColor blue", file: file, line: line)
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
}
