import XCTest
@testable import NatKit

/// Both palettes, held to the same rules.
///
/// Every assertion here runs over Mocha and Latte alike — the rules are the
/// theme's, not the dark theme's, and a light palette that met a weaker bar
/// would be a light theme people turn back off. The one exception is called
/// out where it sits: Mocha's secondary label clears AAA on the ground and
/// Latte's cannot without reading as the primary, so that one bar stays
/// Mocha's own rather than being lowered for both.
final class PaletteTests: XCTestCase {
    private let palettes: [(String, Palette)] = [
        ("mocha", .mocha),
        ("latte", .latte),
    ]

    private func surfaces(_ palette: Palette) -> [(String, String)] {
        [
            ("fieldBg", palette.fieldBg),
            ("windowBg", palette.windowBg),
            ("controlBg", palette.controlBg),
            ("rowAltBg", palette.rowAltBg),
            ("controlFace", palette.controlFace),
        ]
    }

    // MARK: - Surfaces

    /// The surface ladder is the whole point of a palette: five levels that
    /// have to sit in one order and be visibly apart. Which way the ladder
    /// runs is the theme's — away from the ground, which is up in luminance
    /// on a dark ground and down on a light one — but that it runs one way
    /// throughout, in steps big enough to see, is neither theme's to bend.
    func testSurfaceLadderIsOrderedAndSeparated() {
        for (name, palette) in palettes {
            let ladder = surfaces(palette)
            for (lower, upper) in zip(ladder, ladder.dropFirst()) {
                let step = ContrastMath.luminance(upper.1) - ContrastMath.luminance(lower.1)
                let signed = palette.isDark ? step : -step
                XCTAssertGreaterThan(
                    signed, 0.004,
                    "\(name): \(upper.0) should sit a visible step past \(lower.0)"
                )
            }
        }
    }

    /// Neither theme runs off the end of its own range: the dark one's
    /// deepest surface is not black, because a well that reads as a hole is
    /// what its values were chosen to fix, and its lightest is still a dark
    /// surface; the light one's darkest surface is still a light surface,
    /// and its ground is unambiguously light.
    func testSurfacesStayOnTheirOwnSideOfTheRange() {
        XCTAssertGreaterThan(
            ContrastMath.luminance(Palette.mocha.fieldBg), 0.005,
            "mocha's deepest surface should not be black"
        )
        XCTAssertLessThan(
            ContrastMath.luminance(Palette.mocha.controlFace), 0.2,
            "mocha's lightest surface should still read as a dark surface"
        )
        XCTAssertGreaterThan(
            ContrastMath.luminance(Palette.latte.windowBg), 0.5,
            "latte's ground should read as a light one"
        )
        XCTAssertGreaterThan(
            ContrastMath.luminance(Palette.latte.controlFace), 0.35,
            "latte's darkest surface should still read as a light surface"
        )
    }

    /// The terminal sits at the same level as the app's other well in both
    /// themes, which is what stops it reading as a hole cut in the window.
    func testTerminalSurfaceSitsAtTheFieldLevel() {
        for (name, palette) in palettes {
            XCTAssertEqual(
                palette.terminalBg, palette.fieldBg,
                "\(name): the terminal should sit at the field's level"
            )
        }
    }

    // MARK: - Text

    /// Body text and muted labels clear the WCAG bars on every surface they
    /// are drawn on. `labelQuaternary` is deliberately not in this list: it
    /// is decoration, never words to read.
    func testLabelContrastOnEverySurface() {
        for (name, palette) in palettes {
            for (surfaceName, surface) in surfaces(palette) {
                XCTAssertGreaterThanOrEqual(
                    ContrastMath.ratio(palette.label, surface), 4.5,
                    "\(name): label on \(surfaceName) should clear WCAG AA for body text"
                )
                XCTAssertGreaterThanOrEqual(
                    ContrastMath.ratio(palette.labelSecondary, surface), 3.0,
                    "\(name): labelSecondary on \(surfaceName) should clear WCAG AA for large text"
                )
            }
        }
    }

    /// The tiers that carry sentences clear their bars on the ground the app
    /// is mostly made of: the primary at AAA, the two below it at AA.
    func testLabelContrastOnTheGround() {
        for (name, palette) in palettes {
            XCTAssertGreaterThanOrEqual(
                ContrastMath.ratio(palette.label, palette.windowBg), 7.0,
                "\(name): label should clear WCAG AAA on windowBg"
            )
            XCTAssertGreaterThanOrEqual(
                ContrastMath.ratio(palette.labelSecondary, palette.windowBg), 4.5,
                "\(name): labelSecondary should clear WCAG AA on windowBg"
            )
            XCTAssertGreaterThanOrEqual(
                ContrastMath.ratio(palette.labelTertiary, palette.windowBg), 4.5,
                "\(name): labelTertiary should clear WCAG AA on windowBg"
            )
        }
    }

    /// Mocha's own acceptance, kept: its secondary label clears AAA too.
    /// Latte has no value that could without reading as its primary — the
    /// light ramp simply does not go that far — so this bar stays where it
    /// was earned rather than being lowered to what both can meet.
    func testMochaSecondaryLabelClearsAAA() {
        XCTAssertGreaterThanOrEqual(
            ContrastMath.ratio(Palette.mocha.labelSecondary, Palette.mocha.windowBg), 7.0,
            "mocha's labelSecondary should clear WCAG AAA on windowBg"
        )
    }

    /// The four tiers are a ramp and read as one: each recedes further from
    /// the ground than the one above it.
    func testLabelTiersRecede() {
        for (name, palette) in palettes {
            let tiers = [
                palette.label,
                palette.labelSecondary,
                palette.labelTertiary,
                palette.labelQuaternary,
            ]
            for (above, below) in zip(tiers, tiers.dropFirst()) {
                XCTAssertGreaterThan(
                    ContrastMath.ratio(above, palette.windowBg),
                    ContrastMath.ratio(below, palette.windowBg),
                    "\(name): each label tier should recede further than the one above it"
                )
            }
        }
    }

    // MARK: - Accent and outcomes

    /// The accent has to read as text on the ground, and what is written on
    /// top of the accent has to read on it.
    func testAccentContrast() {
        for (name, palette) in palettes {
            XCTAssertGreaterThanOrEqual(
                ContrastMath.ratio(palette.accent, palette.windowBg), 4.5,
                "\(name): accent should clear WCAG AA on windowBg"
            )
            XCTAssertGreaterThanOrEqual(
                ContrastMath.ratio(palette.accentText, palette.accent), 4.5,
                "\(name): accentText should clear WCAG AA on accent"
            )
        }
    }

    /// Every outcome colour is drawn as text on the app's ground, so each
    /// one is held to the same bar as a label.
    func testSystemColorContrastOnTheGround() {
        for (name, palette) in palettes {
            let outcomes: [(String, String)] = [
                ("systemOrange", palette.systemOrange),
                ("systemYellow", palette.systemYellow),
                ("systemGreen", palette.systemGreen),
                ("systemRed", palette.systemRed),
                ("systemBlue", palette.systemBlue),
                ("systemPink", palette.systemPink),
                ("systemTeal", palette.systemTeal),
                ("systemGray", palette.systemGray),
            ]
            for (colorName, color) in outcomes {
                XCTAssertGreaterThanOrEqual(
                    ContrastMath.ratio(color, palette.windowBg), 4.5,
                    "\(name): \(colorName) should clear WCAG AA on windowBg"
                )
            }
        }
    }

    // MARK: - Terminal

    /// Sixteen colours, in the order a terminal numbers them, every one of
    /// them a colour rather than the white `Color(hex:)` falls back to.
    func testAnsiPaletteIsSixteenReadableHexValues() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.ansi.count, 16, "\(name): a terminal takes sixteen ANSI colours")
            for hex in palette.ansi {
                XCTAssertNotNil(
                    rgbComponents(hex: hex),
                    "\(name): \(hex) should be six hex digits"
                )
            }
        }
    }

    /// What an agent writes is words to read: the default foreground clears
    /// AAA on the terminal's own surface, and so does every ANSI colour a
    /// program actually writes text in. The two blacks are exempt and only
    /// them — ANSI black is what a program paints behind text, not what it
    /// writes in.
    func testAnsiColorsReadOnTheTerminalSurface() {
        for (name, palette) in palettes {
            XCTAssertGreaterThanOrEqual(
                ContrastMath.ratio(palette.terminalFg, palette.terminalBg), 7.0,
                "\(name): the terminal's foreground should clear WCAG AAA on its surface"
            )
            for (index, hex) in palette.ansi.enumerated() where index % 8 != 0 {
                XCTAssertGreaterThanOrEqual(
                    ContrastMath.ratio(hex, palette.terminalBg), 4.5,
                    "\(name): ANSI \(index) (\(hex)) should clear WCAG AA on the terminal surface"
                )
            }
        }
    }

    /// The terminal's foreground is the app's own primary label and its
    /// caret the app's own accent: the pane is part of the window rather
    /// than a second product embedded in it.
    func testTerminalTakesTheAppsOwnColors() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.terminalFg, palette.label, "\(name): terminal foreground")
            XCTAssertEqual(palette.terminalCursor, palette.accent, "\(name): terminal caret")
        }
    }

    // MARK: - Opacities

    /// The borders are a ramp too, and the wash is a wash: each opacity sits
    /// where a reader would expect it and none of them is opaque.
    func testBorderOpacitiesAreOrderedAndSubtle() {
        for (name, palette) in palettes {
            XCTAssertLessThan(palette.hairlineOpacity, palette.separatorOpacity, "\(name): hairline vs separator")
            XCTAssertLessThan(palette.separatorOpacity, palette.controlBorderOpacity, "\(name): separator vs border")
            for opacity in [
                palette.hairlineOpacity,
                palette.separatorOpacity,
                palette.controlBorderOpacity,
                palette.selectionWashOpacity,
                palette.headerOpacity,
            ] {
                XCTAssertGreaterThan(opacity, 0, "\(name): no token should be invisible")
                XCTAssertLessThanOrEqual(opacity, 1, "\(name): no opacity should exceed one")
            }
        }
    }

    /// The two palettes are two: nothing here is one value shared by
    /// accident, which is what a half-written light theme would look like.
    func testThePalettesDiffer() {
        XCTAssertNotEqual(Palette.mocha, Palette.latte)
        XCTAssertTrue(Palette.mocha.isDark)
        XCTAssertFalse(Palette.latte.isDark)
    }
}
