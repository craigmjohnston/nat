import XCTest
import SwiftUI
@testable import NatKit

final class DesignTokensTests: XCTestCase {
    // MARK: - Hex Color Initializer Tests

    func testHexColorInitializerValidBlack() {
        let color = Color(hex: "000000")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components")
        XCTAssertEqual(components[0], 0.0, accuracy: 0.01, "Red should be 0")
        XCTAssertEqual(components[1], 0.0, accuracy: 0.01, "Green should be 0")
        XCTAssertEqual(components[2], 0.0, accuracy: 0.01, "Blue should be 0")
    }

    func testHexColorInitializerValidWhite() {
        let color = Color(hex: "ffffff")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components")
        XCTAssertEqual(components[0], 1.0, accuracy: 0.01, "Red should be 1")
        XCTAssertEqual(components[1], 1.0, accuracy: 0.01, "Green should be 1")
        XCTAssertEqual(components[2], 1.0, accuracy: 0.01, "Blue should be 1")
    }

    func testHexColorInitializerValidAccent() {
        // Test the accent color from the design tokens: #cba6f7
        let color = Color(hex: "cba6f7")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components")
        let expectedR = Double(0xcb) / 255.0
        let expectedG = Double(0xa6) / 255.0
        let expectedB = Double(0xf7) / 255.0
        XCTAssertEqual(components[0], expectedR, accuracy: 0.01, "Red component mismatch")
        XCTAssertEqual(components[1], expectedG, accuracy: 0.01, "Green component mismatch")
        XCTAssertEqual(components[2], expectedB, accuracy: 0.01, "Blue component mismatch")
    }

    func testHexColorInitializerValidWindowBg() {
        // Test window background color: #1e1e2e
        let color = Color(hex: "1e1e2e")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components")
        let expectedR = Double(0x1e) / 255.0
        let expectedG = Double(0x1e) / 255.0
        let expectedB = Double(0x2e) / 255.0
        XCTAssertEqual(components[0], expectedR, accuracy: 0.01, "Red component mismatch")
        XCTAssertEqual(components[1], expectedG, accuracy: 0.01, "Green component mismatch")
        XCTAssertEqual(components[2], expectedB, accuracy: 0.01, "Blue component mismatch")
    }

    func testHexColorInitializerWithUppercase() {
        // Hex strings should work case-insensitively
        let color1 = Color(hex: "CBA6F7")
        let color2 = Color(hex: "cba6f7")
        let components1 = color1.cgColor?.components ?? []
        let components2 = color2.cgColor?.components ?? []
        XCTAssertEqual(components1[0], components2[0], accuracy: 0.01)
        XCTAssertEqual(components1[1], components2[1], accuracy: 0.01)
        XCTAssertEqual(components1[2], components2[2], accuracy: 0.01)
    }

    func testHexColorInitializerWithLeadingHash() {
        // The initializer should strip the leading hash if present
        let color1 = Color(hex: "#cba6f7")
        let color2 = Color(hex: "cba6f7")
        let components1 = color1.cgColor?.components ?? []
        let components2 = color2.cgColor?.components ?? []
        XCTAssertEqual(components1[0], components2[0], accuracy: 0.01)
        XCTAssertEqual(components1[1], components2[1], accuracy: 0.01)
        XCTAssertEqual(components1[2], components2[2], accuracy: 0.01)
    }

    func testHexColorInitializerTooShort() {
        // 4-character hex should be treated as invalid and return white
        let color = Color(hex: "fff")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components[0], 1.0, accuracy: 0.01)
        XCTAssertEqual(components[1], 1.0, accuracy: 0.01)
        XCTAssertEqual(components[2], 1.0, accuracy: 0.01)
    }

    func testHexColorInitializerTooLong() {
        // 8-character hex should be treated as invalid and return white
        let color = Color(hex: "ffffffff")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components[0], 1.0, accuracy: 0.01)
        XCTAssertEqual(components[1], 1.0, accuracy: 0.01)
        XCTAssertEqual(components[2], 1.0, accuracy: 0.01)
    }

    func testHexColorInitializerInvalidCharacters() {
        // Non-hex characters should be treated as invalid
        let color = Color(hex: "gggggg")
        let components = color.cgColor?.components ?? []
        // Should default to white
        XCTAssertEqual(components[0], 1.0, accuracy: 0.01)
        XCTAssertEqual(components[1], 1.0, accuracy: 0.01)
        XCTAssertEqual(components[2], 1.0, accuracy: 0.01)
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

    // MARK: - Palette Tests

    /// The surface ladder is the whole point of the palette: five levels
    /// that have to sit in one order and be visibly apart. Asserting the
    /// order alone would pass on the values this replaced, which were in
    /// order and within nine points of each other, so the gap is asserted
    /// too.
    func testSurfaceLadderIsOrderedAndSeparated() {
        let ladder: [(String, Color)] = [
            ("fieldBg", DesignTokens.fieldBg),
            ("windowBg", DesignTokens.windowBg),
            ("controlBg", DesignTokens.controlBg),
            ("rowAltBg", DesignTokens.rowAltBg),
            ("controlFace", DesignTokens.controlFace),
        ]
        for (lower, upper) in zip(ladder, ladder.dropFirst()) {
            let gap = relativeLuminance(upper.1) - relativeLuminance(lower.1)
            XCTAssertGreaterThan(
                gap, 0.004,
                "\(upper.0) should sit a visible step above \(lower.0)"
            )
        }
    }

    /// The dark theme stays in the dark grey range at both ends: the
    /// deepest surface is not black, and the lightest is still a dark
    /// surface rather than a light one.
    func testSurfacesStayInTheDarkGreyRange() {
        XCTAssertGreaterThan(
            relativeLuminance(DesignTokens.fieldBg), 0.005,
            "the deepest surface should not be black"
        )
        XCTAssertLessThan(
            relativeLuminance(DesignTokens.controlFace), 0.2,
            "the lightest surface should still read as a dark surface"
        )
    }

    /// The terminal sits at the same level as the app's other well, which
    /// is what stops it reading as a hole cut in the window.
    func testTerminalSurfaceSitsAtTheFieldLevel() {
        XCTAssertEqual(
            relativeLuminance(DesignTokens.terminalBg),
            relativeLuminance(DesignTokens.fieldBg),
            accuracy: 0.0001
        )
    }

    /// Body text and muted labels clear the WCAG bars on every surface they
    /// are drawn on. `labelQuaternary` is deliberately not in this list: it
    /// is decoration, never words to read.
    func testLabelContrastOnEverySurface() {
        let surfaces: [(String, Color)] = [
            ("fieldBg", DesignTokens.fieldBg),
            ("windowBg", DesignTokens.windowBg),
            ("controlBg", DesignTokens.controlBg),
            ("rowAltBg", DesignTokens.rowAltBg),
            ("controlFace", DesignTokens.controlFace),
        ]
        for (name, surface) in surfaces {
            XCTAssertGreaterThanOrEqual(
                contrastRatio(DesignTokens.label, surface), 4.5,
                "label on \(name) should clear WCAG AA for body text"
            )
            XCTAssertGreaterThanOrEqual(
                contrastRatio(DesignTokens.labelSecondary, surface), 3.0,
                "labelSecondary on \(name) should clear WCAG AA for large text"
            )
        }
    }

    /// The two label tiers that carry sentences clear AA on the ground the
    /// app is mostly made of, which is the acceptance this slice was
    /// written against.
    func testLabelContrastOnTheGround() {
        XCTAssertGreaterThanOrEqual(
            contrastRatio(DesignTokens.label, DesignTokens.windowBg), 7.0,
            "label should clear WCAG AAA on windowBg"
        )
        XCTAssertGreaterThanOrEqual(
            contrastRatio(DesignTokens.labelSecondary, DesignTokens.windowBg), 7.0,
            "labelSecondary should clear WCAG AAA on windowBg"
        )
        XCTAssertGreaterThanOrEqual(
            contrastRatio(DesignTokens.labelTertiary, DesignTokens.windowBg), 4.5,
            "labelTertiary should clear WCAG AA on windowBg"
        )
    }

    /// The accent has to read as text on the ground, and what is written on
    /// top of the accent has to read on it.
    func testAccentContrast() {
        XCTAssertGreaterThanOrEqual(
            contrastRatio(DesignTokens.accent, DesignTokens.windowBg), 4.5,
            "accent should clear WCAG AA on windowBg"
        )
        XCTAssertGreaterThanOrEqual(
            contrastRatio(DesignTokens.accentText, DesignTokens.accent), 4.5,
            "accentText should clear WCAG AA on accent"
        )
    }

    /// Every outcome colour is drawn as text on the app's ground, so each
    /// one is held to the same bar as a label.
    func testSystemColorContrastOnTheGround() {
        let systemColors: [(String, Color)] = [
            ("systemOrange", DesignTokens.systemOrange),
            ("systemYellow", DesignTokens.systemYellow),
            ("systemGreen", DesignTokens.systemGreen),
            ("systemRed", DesignTokens.systemRed),
            ("systemBlue", DesignTokens.systemBlue),
            ("systemPink", DesignTokens.systemPink),
            ("systemTeal", DesignTokens.systemTeal),
            ("systemGray", DesignTokens.systemGray),
        ]
        for (name, color) in systemColors {
            XCTAssertGreaterThanOrEqual(
                contrastRatio(color, DesignTokens.windowBg), 4.5,
                "\(name) should clear WCAG AA on windowBg"
            )
        }
    }

    // MARK: - Contrast Helpers

    /// WCAG 2.1 relative luminance of an opaque sRGB colour.
    private func relativeLuminance(_ color: Color) -> Double {
        let components = color.cgColor?.components ?? [0, 0, 0, 1]
        let channels = components.prefix(3).map { channel -> Double in
            let value = Double(channel)
            return value <= 0.03928
                ? value / 12.92
                : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    }

    /// WCAG 2.1 contrast ratio between two opaque colours.
    private func contrastRatio(_ a: Color, _ b: Color) -> Double {
        let la = relativeLuminance(a)
        let lb = relativeLuminance(b)
        return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
    }
}
