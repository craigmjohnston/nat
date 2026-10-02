import XCTest
@testable import NatKit

/// Both palettes, held to one rule: they are the app's navy scheme, token for
/// token in the roles the gnat design's `gnat.css` names.
///
/// What is asserted is that nothing has drifted from the design's tokens,
/// that each token is still playing the role it was given, and that the two
/// rule colours the design names (`--line`, `--line-2`) come out of the
/// shares that derive them.
final class PaletteTests: XCTestCase {
    private let palettes: [(String, Palette)] = [
        ("dark", .dark),
        ("light", .light),
    ]

    /// The design's `:root` tokens.
    private let darkTokens = [
        "bg": "19161f", "chrome": "1e1b25", "line": "2c2935", "line-2": "36333f",
        "ink": "f4f0e9", "ink-2": "9a9ca5", "ink-3": "5f616a", "ink-4": "393b43",
        "accent": "2c5ed7", "hot": "fca05f", "add": "9fd2a4", "del": "e49aa0",
    ]

    /// The design's `.win.light` tokens.
    private let lightTokens = [
        "bg": "faf9f7", "chrome": "f3f2ef", "line": "d9d9e2", "line-2": "c9c9d4",
        "ink": "151632", "ink-2": "55566d", "ink-3": "9090a2", "ink-4": "d1d1db",
        "accent": "1f44a3", "hot": "d75f09", "add": "2e7d3a", "del": "b23a48",
    ]

    /// Which token plays which role — the same decision in both themes.
    private let roles: [(String, (Palette) -> String, String)] = [
        ("windowBg", { $0.windowBg.hex }, "bg"),
        ("chromeBg", { $0.chromeBg.hex }, "chrome"),
        ("controlBg", { $0.controlBg.hex }, "chrome"),
        ("controlFace", { $0.controlFace.hex }, "line"),
        ("fieldBg", { $0.fieldBg.hex }, "bg"),
        ("terminalBg", { $0.terminalBg.hex }, "bg"),
        ("terminalFg", { $0.terminalFg.hex }, "ink"),
        ("terminalCursor", { $0.terminalCursor.hex }, "accent"),
        ("label", { $0.label.hex }, "ink"),
        ("labelSecondary", { $0.labelSecondary.hex }, "ink-2"),
        ("labelTertiary", { $0.labelTertiary.hex }, "ink-3"),
        ("labelQuaternary", { $0.labelQuaternary.hex }, "ink-4"),
        ("accent", { $0.accent.hex }, "accent"),
        ("hot", { $0.hot.hex }, "hot"),
        ("systemOrange", { $0.systemOrange.hex }, "hot"),
        ("systemGreen", { $0.systemGreen.hex }, "add"),
        ("systemRed", { $0.systemRed.hex }, "del"),
        ("systemGray", { $0.systemGray.hex }, "ink-2"),
    ]

    private func tokens(_ name: String) -> [String: String] {
        name == "dark" ? darkTokens : lightTokens
    }

    // MARK: - Fidelity

    /// What is written on the accent: the ink on the dark theme's deep navy,
    /// the paper on the light theme's.
    func testTheDarkHighlightIsNeutralAndTheLightOneTheAccent() {
        XCTAssertEqual(Palette.dark.rowWashTint.hex, darkTokens["ink"])
        XCTAssertEqual(Palette.light.rowWashTint, Palette.light.accent)
    }

    func testTheAccentsTextIsTheContrastingSurface() {
        XCTAssertEqual(Palette.dark.accentText.hex, darkTokens["ink"])
        XCTAssertEqual(Palette.light.accentText.hex, lightTokens["bg"])
    }

    func testEveryRoleIsTheDesignsToken() {
        for (name, palette) in palettes {
            for (role, value, token) in roles {
                XCTAssertEqual(value(palette), tokens(name)[token], "\(name): \(role) should be --\(token)")
            }
        }
    }

    /// The design's two rule colours, `--line` and `--line-2`, are the
    /// rules on every ground alike.
    func testTheRulesAreTheDesignsLines() {
        for (name, palette) in palettes {
            for ground in Ground.allCases {
                XCTAssertEqual(palette.rule(.hairline, on: ground).hex, tokens(name)["line"], "\(name): \(ground)")
                XCTAssertEqual(palette.rule(.separator, on: ground).hex, tokens(name)["line"], "\(name): \(ground)")
                XCTAssertEqual(palette.rule(.border, on: ground).hex, tokens(name)["line-2"], "\(name): \(ground)")
            }
        }
    }

    /// The one level the design does not name — a band inside a card —
    /// derived from the two either side of it rather than typed.
    func testRowAltIsInterpolatedBetweenLineAndChrome() {
        for (name, palette) in palettes {
            XCTAssertEqual(
                palette.rowAltBg.hex, mix(tokens(name)["line"] ?? "", tokens(name)["chrome"] ?? "", 0.5),
                "\(name): rowAltBg should be derived, not typed")
        }
    }

    /// The titlebars have their own ground: well below the chrome in the dark
    /// theme, the chrome itself in the light one.
    func testTheTitlebarsHaveTheirOwnGround() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.headerBg, palette.titlebarBg, "\(name)")
            XCTAssertEqual(Ground.chrome.surface(in: palette), palette.chromeBg, "\(name)")
            XCTAssertEqual(Ground.header.surface(in: palette), palette.titlebarBg, "\(name)")
        }
        XCTAssertLessThan(luminance(Palette.dark.titlebarBg.hex), luminance(Palette.dark.chromeBg.hex))
        XCTAssertEqual(Palette.light.titlebarBg, Palette.light.chromeBg)
    }

    /// The four ink tiers stay in order: each recedes further from the
    /// ground than the one above it.
    func testLabelTiersRecede() {
        for (name, palette) in palettes {
            let tiers = [
                palette.label.hex, palette.labelSecondary.hex,
                palette.labelTertiary.hex, palette.labelQuaternary.hex,
            ].map(luminance)
            let ground = luminance(palette.windowBg.hex)
            for (above, below) in zip(tiers, tiers.dropFirst()) {
                XCTAssertGreaterThan(abs(above - ground), abs(below - ground), "\(name): ink tiers should recede")
            }
        }
    }

    // MARK: - Row washes

    /// `--sel` and `--sel-2`: the row tint mixed into the ground — the ink in
    /// the dark theme, the accent in the light one — the selected
    /// row's the heavier, both still a step off the ground.
    func testRowWashesAreTheInkOverTheGround() {
        for (name, palette) in palettes {
            let hover = palette.rowWash(selected: false, on: .window)
            let selected = palette.rowWash(selected: true, on: .window)
            XCTAssertEqual(hover.hex, mix(palette.rowWashTint.hex, palette.windowBg.hex, palette.rowHoverShare), "\(name)")
            XCTAssertEqual(selected.hex, mix(palette.rowWashTint.hex, palette.windowBg.hex, palette.rowSelectedShare), "\(name)")
            XCTAssertLessThan(palette.rowHoverShare, palette.rowSelectedShare, "\(name): hover is the lighter")
            XCTAssertNotEqual(hover.hex, palette.windowBg.hex, "\(name): a hover the ground hides is no hover")
            XCTAssertEqual(palette.hoverWash, hover, "\(name): the hover surface is --sel")
        }
    }

    // MARK: - Terminal

    func testAnsiPaletteIsSixteenRepeatedHues() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.ansi.count, 16, "\(name)")
            for hex in palette.ansi {
                XCTAssertNotNil(rgbComponents(hex: hex), "\(name): \(hex)")
            }
            for hue in 1...6 {
                XCTAssertEqual(palette.ansi[hue], palette.ansi[hue + 8], "\(name): ANSI \(hue)")
            }
        }
    }

    /// The terminal sits on the window ground in the window's own ink, with
    /// the accent for a caret and `--sel-2` behind a selection.
    func testTerminalTakesTheAppsOwnColors() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.terminalBg, palette.windowBg, "\(name): terminal surface")
            XCTAssertEqual(palette.terminalFg.hex, palette.label.hex, "\(name): terminal foreground")
            XCTAssertEqual(palette.terminalCursor, palette.accent, "\(name): terminal caret")
            XCTAssertEqual(palette.terminalSelection, palette.rowWash(selected: true, on: .window), "\(name)")
        }
    }

    // MARK: - Shares

    /// `--line-2` is the heavier of the two: it stands further off the
    /// window ground than `--line` does.
    func testTheBorderIsTheHeavierLine() {
        for (name, palette) in palettes {
            let ground = luminance(palette.windowBg.hex)
            XCTAssertGreaterThan(
                abs(luminance(palette.line2.hex) - ground), abs(luminance(palette.line.hex) - ground), "\(name)")
        }
    }

    func testEveryWashIsTranslucent() {
        let washes: [(String, KeyPath<Palette, Double>)] = [
            ("bandShare", \.bandShare), ("chipShare", \.chipShare), ("avatarShare", \.avatarShare),
            ("diffRowShare", \.diffRowShare), ("diffGutterShare", \.diffGutterShare),
            ("commentShare", \.commentShare), ("mutedShare", \.mutedShare),
            ("skeletonShare", \.skeletonShare), ("onAccentRuleShare", \.onAccentRuleShare),
            ("rowHoverShare", \.rowHoverShare), ("rowSelectedShare", \.rowSelectedShare),
        ]
        for (name, palette) in palettes {
            for (wash, key) in washes {
                XCTAssertGreaterThan(palette[keyPath: key], 0, "\(name): \(wash)")
                XCTAssertLessThan(palette[keyPath: key], 1, "\(name): \(wash)")
            }
        }
    }

    func testDiffWashesAreOrdered() {
        for (name, palette) in palettes {
            XCTAssertLessThanOrEqual(palette.commentShare, palette.diffRowShare, "\(name)")
            XCTAssertLessThan(palette.diffRowShare, palette.diffGutterShare, "\(name)")
        }
    }

    func testTheGroundlessWashesMatchAcrossThemes() {
        XCTAssertEqual(Palette.light.bandShare, Palette.dark.bandShare)
        XCTAssertEqual(Palette.light.onAccentRuleShare, Palette.dark.onAccentRuleShare)
        XCTAssertEqual(Palette.light.mutedShare, Palette.dark.mutedShare)
    }

    func testThePalettesDiffer() {
        XCTAssertNotEqual(Palette.dark, Palette.light)
        XCTAssertTrue(Palette.dark.isDark)
        XCTAssertFalse(Palette.light.isDark)
    }

    // MARK: - Helpers

    private func luminance(_ hex: String) -> Double {
        let rgb = rgbComponents(hex: hex) ?? (1, 1, 1)
        let channels = [rgb.red, rgb.green, rgb.blue].map { value -> Double in
            value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    }
}
