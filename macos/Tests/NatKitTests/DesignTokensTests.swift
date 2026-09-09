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
        // Test the accent color from the design tokens: #b1a8f2
        let color = Color(hex: "b1a8f2")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components")
        let expectedR = Double(0xb1) / 255.0
        let expectedG = Double(0xa8) / 255.0
        let expectedB = Double(0xf2) / 255.0
        XCTAssertEqual(components[0], expectedR, accuracy: 0.01, "Red component mismatch")
        XCTAssertEqual(components[1], expectedG, accuracy: 0.01, "Green component mismatch")
        XCTAssertEqual(components[2], expectedB, accuracy: 0.01, "Blue component mismatch")
    }

    func testHexColorInitializerValidWindowBg() {
        // Test window background color: #1e1e23
        let color = Color(hex: "1e1e23")
        let components = color.cgColor?.components ?? []
        XCTAssertEqual(components.count, 4, "RGBA color should have 4 components")
        let expectedR = Double(0x1e) / 255.0
        let expectedG = Double(0x1e) / 255.0
        let expectedB = Double(0x23) / 255.0
        XCTAssertEqual(components[0], expectedR, accuracy: 0.01, "Red component mismatch")
        XCTAssertEqual(components[1], expectedG, accuracy: 0.01, "Green component mismatch")
        XCTAssertEqual(components[2], expectedB, accuracy: 0.01, "Blue component mismatch")
    }

    func testHexColorInitializerWithUppercase() {
        // Hex strings should work case-insensitively
        let color1 = Color(hex: "B1A8F2")
        let color2 = Color(hex: "b1a8f2")
        let components1 = color1.cgColor?.components ?? []
        let components2 = color2.cgColor?.components ?? []
        XCTAssertEqual(components1[0], components2[0], accuracy: 0.01)
        XCTAssertEqual(components1[1], components2[1], accuracy: 0.01)
        XCTAssertEqual(components1[2], components2[2], accuracy: 0.01)
    }

    func testHexColorInitializerWithLeadingHash() {
        // The initializer should strip the leading hash if present
        let color1 = Color(hex: "#b1a8f2")
        let color2 = Color(hex: "b1a8f2")
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
        _ = DesignTokens.label
        _ = DesignTokens.labelSecondary
        _ = DesignTokens.labelTertiary
        _ = DesignTokens.labelQuaternary
        _ = DesignTokens.accent
        _ = DesignTokens.accentText
        _ = DesignTokens.separator
        _ = DesignTokens.controlBorder
        _ = DesignTokens.systemOrange
        _ = DesignTokens.systemYellow
        _ = DesignTokens.systemGreen
        _ = DesignTokens.systemRed
        _ = DesignTokens.systemBlue
        _ = DesignTokens.systemPink
        _ = DesignTokens.systemTeal
        _ = DesignTokens.systemGray
    }
}
