import XCTest
@testable import NatKit

/// Both palettes, held to one rule: they are Catppuccin's, unedited.
///
/// There is deliberately no contrast assertion here. Mocha and Latte are
/// published themes with a great many people reading code in them, and a
/// value bent here to satisfy a ratio would be a colour no other Catppuccin
/// has — wrong beside every other window the user has open, and wrong on the
/// authority of a calculator over the people who made the thing. What is
/// worth asserting is that nothing has drifted from the published values and
/// that each swatch is still playing the role it was given.
final class PaletteTests: XCTestCase {
    private let palettes: [(String, Palette)] = [
        ("mocha", .mocha),
        ("latte", .latte),
    ]

    /// Catppuccin's published Mocha, by swatch name.
    private let mochaSwatches = [
        "base": "1e1e2e", "mantle": "181825", "crust": "11111b",
        "surface0": "313244", "surface1": "45475a", "surface2": "585b70",
        "overlay0": "6c7086", "overlay1": "7f849c", "overlay2": "9399b2",
        "subtext0": "a6adc8", "subtext1": "bac2de", "text": "cdd6f4",
        "mauve": "cba6f7", "red": "f38ba8", "green": "a6e3a1",
        "yellow": "f9e2af", "peach": "fab387", "blue": "89b4fa",
        "pink": "f5c2e7", "teal": "94e2d5",
    ]

    /// Catppuccin's published Latte, by swatch name.
    private let latteSwatches = [
        "base": "eff1f5", "mantle": "e6e9ef", "crust": "dce0e8",
        "surface0": "ccd0da", "surface1": "bcc0cc", "surface2": "acb0be",
        "overlay0": "9ca0b0", "overlay1": "8c8fa1", "overlay2": "7c7f93",
        "subtext0": "6c6f85", "subtext1": "5c5f77", "text": "4c4f69",
        "mauve": "8839ef", "red": "d20f39", "green": "40a02b",
        "yellow": "df8e1d", "peach": "fe640b", "blue": "1e66f5",
        "pink": "ea76cb", "teal": "179299",
    ]

    /// Which swatch plays which role — the whole of what this app decided,
    /// and the same decision in both themes, so the two are one design in
    /// two palettes rather than two designs.
    private let roles: [(String, KeyPath<Palette, String>, String)] = [
        ("windowBg", \.windowBg, "base"),
        ("controlBg", \.controlBg, "surface0"),
        ("controlFace", \.controlFace, "surface1"),
        ("hoverWash", \.hoverWash, "surface0"),
        ("fieldBg", \.fieldBg, "mantle"),
        ("terminalBg", \.terminalBg, "mantle"),
        ("terminalFg", \.terminalFg, "text"),
        ("terminalCursor", \.terminalCursor, "mauve"),
        ("terminalSelection", \.terminalSelection, "surface1"),
        ("label", \.label, "text"),
        ("labelSecondary", \.labelSecondary, "subtext0"),
        ("labelTertiary", \.labelTertiary, "overlay2"),
        ("labelQuaternary", \.labelQuaternary, "overlay0"),
        ("accent", \.accent, "mauve"),
        ("accentText", \.accentText, "crust"),
        ("systemOrange", \.systemOrange, "peach"),
        ("systemYellow", \.systemYellow, "yellow"),
        ("systemGreen", \.systemGreen, "green"),
        ("systemRed", \.systemRed, "red"),
        ("systemBlue", \.systemBlue, "blue"),
        ("systemPink", \.systemPink, "pink"),
        ("systemTeal", \.systemTeal, "teal"),
        ("systemGray", \.systemGray, "overlay2"),
    ]

    // MARK: - Fidelity

    /// Every token is the published swatch its role names, in both themes.
    /// This is the test that would catch the tempting edit: one hex nudged
    /// darker to win an argument with a contrast checker, and the palette is
    /// no longer the one it says it is.
    func testEveryRoleIsThePublishedSwatch() {
        for (name, palette) in palettes {
            let swatches = name == "mocha" ? mochaSwatches : latteSwatches
            for (role, key, swatch) in roles {
                XCTAssertEqual(
                    palette[keyPath: key], swatches[swatch],
                    "\(name): \(role) should be Catppuccin's \(swatch), unedited"
                )
            }
        }
    }

    /// The one value neither palette publishes: the level between
    /// `surface0` and `surface1` that a band inside a card needs. It is
    /// interpolated rather than invented, which is what "between" means
    /// channel by channel.
    func testRowAltIsInterpolatedBetweenTheTwoSurfaces() {
        for (name, palette) in palettes {
            let swatches = name == "mocha" ? mochaSwatches : latteSwatches
            let row = try? XCTUnwrap(rgbComponents(hex: palette.rowAltBg))
            let low = try? XCTUnwrap(rgbComponents(hex: swatches["surface0"] ?? ""))
            let high = try? XCTUnwrap(rgbComponents(hex: swatches["surface1"] ?? ""))
            guard let row, let low, let high else { return XCTFail("\(name): unreadable swatch") }
            for (channel, values) in [
                ("red", (row.red, low.red, high.red)),
                ("green", (row.green, low.green, high.green)),
                ("blue", (row.blue, low.blue, high.blue)),
            ] {
                let (value, first, second) = values
                XCTAssertGreaterThanOrEqual(value, min(first, second), "\(name): rowAltBg \(channel)")
                XCTAssertLessThanOrEqual(value, max(first, second), "\(name): rowAltBg \(channel)")
            }
        }
    }

    /// The five surfaces are five: a role mapped to a swatch another role
    /// already has is a level of the design silently gone.
    func testSurfacesAreDistinct() {
        for (name, palette) in palettes {
            let surfaces = [
                palette.fieldBg, palette.windowBg, palette.controlBg,
                palette.rowAltBg, palette.controlFace,
            ]
            XCTAssertEqual(Set(surfaces).count, surfaces.count, "\(name): every surface should be its own level")
        }
    }

    /// The four label tiers are Catppuccin's own ramp and stay in its order:
    /// each recedes further from the ground than the one above it, which is
    /// what makes a meta line read as a meta line. Which colours those are
    /// is the theme's; that they are in order is the mapping's.
    func testLabelTiersRecede() {
        for (name, palette) in palettes {
            let tiers = [
                palette.label, palette.labelSecondary,
                palette.labelTertiary, palette.labelQuaternary,
            ].map(luminance)
            let ground = luminance(palette.windowBg)
            for (above, below) in zip(tiers, tiers.dropFirst()) {
                XCTAssertGreaterThan(
                    abs(above - ground), abs(below - ground),
                    "\(name): each label tier should recede further than the one above it"
                )
            }
        }
    }

    /// The hover fill is a surface and not a label. It used to be
    /// `labelQuaternary`, which is `overlay0` — ink, drawn as ground — and
    /// under Mocha that left `text` sitting on a mid-grey. What is asserted
    /// is the two things that choice has to satisfy in either theme: the
    /// primary label reads further off the hover fill than it did off the
    /// colour it replaced, and the fill still parts from the ground it is
    /// laid on, or a hover would be invisible.
    func testHoverIsASurfaceTheLabelStaysReadableOn() {
        for (name, palette) in palettes {
            let label = luminance(palette.label)
            XCTAssertGreaterThan(
                abs(label - luminance(palette.hoverWash)),
                abs(label - luminance(palette.labelQuaternary)),
                "\(name): a label should read further off the hover fill than off the ink it replaced"
            )
            XCTAssertNotEqual(
                palette.hoverWash, palette.windowBg,
                "\(name): a hover fill that matched the ground would not be a hover"
            )
        }
    }

    /// And it moves the way its own theme moves: Mocha's surfaces rise off
    /// `base` and Latte's sink below it, so a hover raises in the dark theme
    /// and deepens in the light one. Either way it is a step off the ground
    /// rather than a step towards the label, which is what the old fill was.
    func testHoverFollowsItsThemesOwnDirection() {
        XCTAssertGreaterThan(
            luminance(Palette.mocha.hoverWash), luminance(Palette.mocha.windowBg),
            "mocha: a hover should rise off the ground"
        )
        XCTAssertLessThan(
            luminance(Palette.latte.hoverWash), luminance(Palette.latte.windowBg),
            "latte: a hover should deepen from the ground"
        )
    }

    // MARK: - Terminal

    /// Sixteen colours, in the order a terminal numbers them, every one of
    /// them a colour rather than the white `Color(hex:)` falls back to.
    func testAnsiPaletteIsSixteenReadableHexValues() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.ansi.count, 16, "\(name): a terminal takes sixteen ANSI colours")
            for hex in palette.ansi {
                XCTAssertNotNil(rgbComponents(hex: hex), "\(name): \(hex) should be six hex digits")
            }
        }
    }

    /// Catppuccin's own terminal mapping, which its ports all write the same
    /// way: the two surfaces for the two blacks, the two subtexts for the
    /// two whites, and the accent hues repeated between the halves.
    func testAnsiPaletteIsCatppuccinsTerminalMapping() {
        for (name, palette) in palettes {
            let swatches = name == "mocha" ? mochaSwatches : latteSwatches
            XCTAssertEqual(palette.ansi[0], swatches["surface1"], "\(name): ANSI black")
            XCTAssertEqual(palette.ansi[8], swatches["surface2"], "\(name): ANSI bright black")
            XCTAssertEqual(palette.ansi[7], swatches["subtext1"], "\(name): ANSI white")
            XCTAssertEqual(palette.ansi[15], swatches["subtext0"], "\(name): ANSI bright white")
            for hue in 1...6 {
                XCTAssertEqual(
                    palette.ansi[hue], palette.ansi[hue + 8],
                    "\(name): ANSI \(hue) should be the same hue bright as normal"
                )
            }
        }
    }

    /// The terminal is drawn in the app's own colours: its surface is the
    /// app's other well, its foreground the app's primary label and its
    /// caret the app's accent. The pane is part of the window rather than a
    /// second product embedded in it.
    func testTerminalTakesTheAppsOwnColors() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.terminalBg, palette.fieldBg, "\(name): terminal surface")
            XCTAssertEqual(palette.terminalFg, palette.label, "\(name): terminal foreground")
            XCTAssertEqual(palette.terminalCursor, palette.accent, "\(name): terminal caret")
            XCTAssertEqual(palette.terminalSelection, palette.controlFace, "\(name): terminal selection")
        }
    }

    // MARK: - Opacities

    /// The borders are a ramp and the wash is a wash. These are the one
    /// thing Catppuccin says nothing about — how hard to press a hairline —
    /// so they are the theme's own, and all that is asserted is that they
    /// stay in order and stay translucent.
    func testBorderOpacitiesAreOrderedAndSubtle() {
        for (name, palette) in palettes {
            XCTAssertLessThan(palette.hairlineOpacity, palette.separatorOpacity, "\(name): hairline vs separator")
            XCTAssertLessThan(palette.separatorOpacity, palette.controlBorderOpacity, "\(name): separator vs border")
            for opacity in [
                palette.hairlineOpacity, palette.separatorOpacity,
                palette.controlBorderOpacity, palette.selectionWashOpacity,
                palette.headerOpacity,
            ] {
                XCTAssertGreaterThan(opacity, 0, "\(name): no token should be invisible")
                XCTAssertLessThanOrEqual(opacity, 1, "\(name): no opacity should exceed one")
            }
        }
    }

    /// Every opacity in the palette is an opacity: visible, and not a
    /// number typed past one. The washes are not in the ramp above, since
    /// they answer to their own roles rather than to each other — what is
    /// asserted of them is only that they are washes.
    func testEveryWashIsTranslucent() {
        let washes: [(String, KeyPath<Palette, Double>)] = [
            ("bandOpacity", \.bandOpacity),
            ("tintWashOpacity", \.tintWashOpacity),
            ("avatarWashOpacity", \.avatarWashOpacity),
            ("diffRowWashOpacity", \.diffRowWashOpacity),
            ("diffGutterWashOpacity", \.diffGutterWashOpacity),
            ("commentWashOpacity", \.commentWashOpacity),
            ("headerAccentOpacity", \.headerAccentOpacity),
            ("mutedAccentOpacity", \.mutedAccentOpacity),
            ("skeletonHighlightOpacity", \.skeletonHighlightOpacity),
            ("onAccentSeparatorOpacity", \.onAccentSeparatorOpacity),
        ]
        for (name, palette) in palettes {
            for (wash, key) in washes {
                let opacity = palette[keyPath: key]
                XCTAssertGreaterThan(opacity, 0, "\(name): \(wash) should be visible")
                XCTAssertLessThan(opacity, 1, "\(name): a wash at full strength is not a wash")
            }
        }
    }

    /// The diff's two weights of the same outcome colour stay in order: a
    /// gutter cell is a stripe a few characters wide and has to carry the
    /// row's sign on its own, so it is the heavier of the two, and a comment
    /// is an annotation rather than a change and is the lightest mark in the
    /// box.
    func testDiffWashesAreOrdered() {
        for (name, palette) in palettes {
            XCTAssertLessThan(palette.commentWashOpacity, palette.diffRowWashOpacity, "\(name): comment vs row")
            XCTAssertLessThan(palette.diffRowWashOpacity, palette.diffGutterWashOpacity, "\(name): row vs gutter")
        }
    }

    /// Each wash is pressed for the ground it lands on, which is the rule
    /// `selectionWashOpacity` and the border ramp already follow: a wash of
    /// a hue is lighter in Latte, whose accents are dark saturated colours
    /// over a light ground, and a wash of `label` is heavier, since dark ink
    /// reads fainter than light ink at the same alpha.
    func testWashesArePressedForTheirGround() {
        let hues: [(String, KeyPath<Palette, Double>)] = [
            ("selectionWashOpacity", \.selectionWashOpacity),
            ("tintWashOpacity", \.tintWashOpacity),
            ("avatarWashOpacity", \.avatarWashOpacity),
            ("diffRowWashOpacity", \.diffRowWashOpacity),
            ("diffGutterWashOpacity", \.diffGutterWashOpacity),
            ("commentWashOpacity", \.commentWashOpacity),
            ("headerAccentOpacity", \.headerAccentOpacity),
        ]
        for (name, key) in hues {
            XCTAssertLessThan(
                Palette.latte[keyPath: key], Palette.mocha[keyPath: key],
                "\(name): a hue wash should be lighter in Latte"
            )
        }
        for (name, key) in [
            ("hairlineOpacity", \Palette.hairlineOpacity),
            ("separatorOpacity", \Palette.separatorOpacity),
            ("controlBorderOpacity", \Palette.controlBorderOpacity),
            ("skeletonHighlightOpacity", \Palette.skeletonHighlightOpacity),
        ] {
            XCTAssertGreaterThan(
                Palette.latte[keyPath: key], Palette.mocha[keyPath: key],
                "\(name): a wash of `label` should be heavier in Latte"
            )
        }
    }

    /// The two washes that are deliberately the same in both themes, and
    /// the comments beside them are the reason: `bandOpacity` mixes two of
    /// the palette's own surfaces, so it re-balances by itself, and
    /// `onAccentSeparatorOpacity` is the accent's own maximum-contrast ink
    /// over the accent, which is what `accentText` is in either theme.
    func testTheTwoGroundlessWashesMatchAcrossThemes() {
        XCTAssertEqual(Palette.latte.bandOpacity, Palette.mocha.bandOpacity)
        XCTAssertEqual(Palette.latte.onAccentSeparatorOpacity, Palette.mocha.onAccentSeparatorOpacity)
        // A dim is read against the full colour beside it rather than
        // against the ground under it, so it is the same fraction too.
        XCTAssertEqual(Palette.latte.mutedAccentOpacity, Palette.mocha.mutedAccentOpacity)
    }

    /// The two palettes are two: nothing here is one value shared by
    /// accident, which is what a half-written light theme would look like.
    func testThePalettesDiffer() {
        XCTAssertNotEqual(Palette.mocha, Palette.latte)
        XCTAssertTrue(Palette.mocha.isDark)
        XCTAssertFalse(Palette.latte.isDark)
    }

    /// Relative luminance, used here only to compare two colours against
    /// each other — never against a threshold.
    private func luminance(_ hex: String) -> Double {
        let rgb = rgbComponents(hex: hex) ?? (1, 1, 1)
        let channels = [rgb.red, rgb.green, rgb.blue].map { value -> Double in
            value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    }
}
