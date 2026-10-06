import XCTest
@testable import NatKit

/// Every palette the app ships, held to one rule: each is its source's values,
/// token for token, in the roles the gnat design's `gnat.css` names — the
/// light one the design's own tokens, the dark ones the community themes
/// they are taken from.
///
/// What is asserted is that nothing has drifted from those values, that each
/// is still playing the role it was given, and that the two rule colours
/// (`--line`, `--line-2`) come out of the shares that derive them.
final class PaletteTests: XCTestCase {
    private let palettes: [(String, Palette)] = PaletteChoice.allCases.map { ($0.rawValue, $0.palette) }

    /// Iceberg's published values, its ground lifted a step (`#161821` is
    /// the chrome).
    private let icebergTokens = [
        "bg": "1b1d28", "chrome": "161821", "line": "262a3c", "line-2": "33374c",
        "ink": "c6c8d1", "ink-2": "a3a6b7", "ink-3": "6b7089", "ink-4": "3e445e",
        "accent": "84a0c6", "hot": "e2a478", "add": "b4be82", "del": "e27878",
    ]

    /// One Light's published values, its accent shaded a step darker.
    private let oneLightTokens = [
        "bg": "fafafa", "chrome": "f0f0f1", "line": "e0e0e2", "line-2": "d4d4d7",
        "ink": "383a42", "ink-2": "696c77", "ink-3": "a0a1a7", "ink-4": "d4d4d6",
        "accent": "2f6cf1", "hot": "d0721f", "add": "50a14f", "del": "e45649",
    ]

    /// Tokyo Night Day's published values, its ink and accent shaded a step
    /// darker.
    private let tokyoDayTokens = [
        "bg": "e1e2e7", "chrome": "d0d5e3", "line": "c4c8da", "line-2": "b6bcd2",
        "ink": "2f52a3", "ink-2": "6172b0", "ink-3": "848cb5", "ink-4": "b4b9cf",
        "accent": "1c72e7", "hot": "b15c00", "add": "587539", "del": "f52a65",
    ]

    /// Slate's grounds under Kanagawa's ink and hues.
    private let slateInkTokens = [
        "bg": "252838", "chrome": "1f2231", "line": "313549", "line-2": "3c4057",
        "ink": "dcd7ba", "ink-2": "a8a594", "ink-3": "7a7a8a", "ink-4": "444862",
        "accent": "7e9cd8", "hot": "ffa066", "add": "98bb6c", "del": "e46876",
    ]

    /// The design's `.win.light` tokens.
    private let lightTokens = [
        "bg": "faf9f7", "chrome": "f3f2ef", "line": "d9d9e2", "line-2": "c9c9d4",
        "ink": "151632", "ink-2": "55566d", "ink-3": "9090a2", "ink-4": "d1d1db",
        "accent": "1f44a3", "hot": "d75f09", "add": "2e7d3a", "del": "b23a48",
    ]

    /// Which token plays which role — the same decision in every palette.
    /// The light palette's cards and control faces are its chrome and its
    /// line; the dark ones take those from levels of their own theme's ramp.
    private let roles: [(String, (Palette) -> String, String)] = [
        ("windowBg", { $0.windowBg.hex }, "bg"),
        ("chromeBg", { $0.chromeBg.hex }, "chrome"),
        ("titlebarBg", { $0.titlebarBg.hex }, "chrome"),
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
        switch PaletteChoice(rawValue: name) {
        case .oneLight: oneLightTokens
        case .tokyoDay: tokyoDayTokens
        case .iceberg: icebergTokens
        case .slateInk: slateInkTokens
        case .light, nil: lightTokens
        }
    }

    // MARK: - Fidelity

    /// A light palette's highlight is its accent, and its cards are its
    /// chrome — the design's light structure, which every light palette
    /// keeps. The design's own control face is its line.
    func testTheLightPalettesAreTheDesignsStructure() {
        for choice in PaletteChoice.choices(dark: false) {
            XCTAssertEqual(choice.palette.rowWashTint, choice.palette.accent, "\(choice)")
            XCTAssertEqual(choice.palette.controlBg, choice.palette.chromeBg, "\(choice)")
        }
        XCTAssertEqual(Palette.light.controlFace.hex, lightTokens["line"])
    }

    /// What is written on the accent: paper or white on a light theme's
    /// strong accent, the chrome on a dark theme's pale one.
    func testTheAccentsTextIsTheContrastingSurface() {
        XCTAssertEqual(Palette.light.accentText.hex, lightTokens["bg"])
        XCTAssertEqual(Palette.oneLight.accentText.hex, "ffffff")
        XCTAssertEqual(Palette.tokyoDay.accentText.hex, "ffffff")
        XCTAssertEqual(Palette.iceberg.accentText, Ink(Palette.iceberg.chromeBg.hex))
        XCTAssertEqual(Palette.slateInk.accentText, Ink(Palette.slateInk.chromeBg.hex))
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

    /// The one level no theme names — a band inside a card — derived from
    /// the two either side of it rather than typed.
    func testRowAltIsInterpolatedBetweenLineAndCard() {
        for (name, palette) in palettes {
            XCTAssertEqual(
                palette.rowAltBg.hex, mix(tokens(name)["line"] ?? "", palette.controlBg.hex, 0.5),
                "\(name): rowAltBg should be derived, not typed")
        }
    }

    /// The titlebars are the chrome, and the header ground is the titlebar.
    func testTheTitlebarsAreTheChrome() {
        for (name, palette) in palettes {
            XCTAssertEqual(palette.headerBg, palette.titlebarBg, "\(name)")
            XCTAssertEqual(Ground.chrome.surface(in: palette), palette.chromeBg, "\(name)")
            XCTAssertEqual(Ground.header.surface(in: palette), palette.titlebarBg, "\(name)")
        }
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

    /// `--sel` and `--sel-2`: the row tint mixed into the ground, the
    /// selected row's the heavier, both still a step off the ground.
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
        for (name, palette) in palettes {
            XCTAssertEqual(Palette.light.bandShare, palette.bandShare, "\(name)")
            XCTAssertEqual(Palette.light.onAccentRuleShare, palette.onAccentRuleShare, "\(name)")
            XCTAssertEqual(Palette.light.mutedShare, palette.mutedShare, "\(name)")
        }
    }

    func testThePalettesDiffer() {
        XCTAssertEqual(Set(palettes.map { $0.1.windowBg.hex }).count, palettes.count)
        XCTAssertTrue(Palette.iceberg.isDark)
        XCTAssertTrue(Palette.slateInk.isDark)
        XCTAssertFalse(Palette.light.isDark)
        XCTAssertFalse(Palette.oneLight.isDark)
        XCTAssertFalse(Palette.tokyoDay.isDark)
    }

    // MARK: - Helpers

    private func luminance(_ hex: String) -> Double {
        let rgb = rgbComponents(hex: hex) ?? (1, 1, 1)
        let channels = [rgb.red, rgb.green, rgb.blue].map { value -> Double in
            value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    }

    /// Every palette draws eight project colours told apart by eye: no two
    /// share a hue, each palette's own where its outcome hues fall short.
    func testEveryPaletteDrawsEightDistinctProjectColours() {
        for (name, palette) in palettes {
            let hues = ProjectColor.allCases.map { palette.projectTint($0).hex }
            XCTAssertEqual(Set(hues).count, ProjectColor.allCases.count, "\(name): \(hues)")
        }
    }

    /// A colour with no hue of the palette's own is its namesake outcome hue.
    func testAProjectColourIsItsOutcomeHueUnlessThePaletteGivesOne() {
        let iceberg = Palette.iceberg
        XCTAssertEqual(iceberg.projectTint(.red), iceberg.systemRed)
        XCTAssertEqual(iceberg.projectTint(.orange), iceberg.systemOrange)
        XCTAssertEqual(iceberg.projectTint(.green), iceberg.systemGreen)
        XCTAssertEqual(iceberg.projectTint(.teal), iceberg.systemTeal)
        XCTAssertEqual(iceberg.projectTint(.blue), iceberg.systemBlue)
        XCTAssertNotEqual(iceberg.projectTint(.yellow), iceberg.systemYellow, "iceberg's yellow is its orange")
        XCTAssertEqual(Palette.light.projectTint(.pink), Palette.light.systemPink)

        let bare = Palette(
            windowBg: iceberg.windowBg, chromeBg: iceberg.chromeBg, titlebarBg: iceberg.titlebarBg,
            controlBg: iceberg.controlBg, rowAltBg: iceberg.rowAltBg, controlFace: iceberg.controlFace,
            fieldBg: iceberg.fieldBg, hoverWash: iceberg.hoverWash, terminalBg: iceberg.terminalBg,
            terminalFg: iceberg.terminalFg, terminalCursor: iceberg.terminalCursor,
            terminalSelection: iceberg.terminalSelection, ansi: iceberg.ansi, label: iceberg.label,
            labelSecondary: iceberg.labelSecondary, labelTertiary: iceberg.labelTertiary,
            labelQuaternary: iceberg.labelQuaternary, accent: iceberg.accent, accentText: iceberg.accentText,
            hot: iceberg.hot, rowHoverShare: 0, rowSelectedShare: 0, rowWashTint: iceberg.rowWashTint,
            line: iceberg.line, line2: iceberg.line2, selectionShare: 0, bandShare: 0, chipShare: 0,
            avatarShare: 0, diffRowShare: 0, diffGutterShare: 0, commentShare: 0, headerVeilShare: 0,
            mutedShare: 0, skeletonShare: 0, onAccentRuleShare: 0, systemOrange: iceberg.systemOrange,
            systemYellow: iceberg.systemYellow, systemGreen: iceberg.systemGreen, systemRed: iceberg.systemRed,
            systemBlue: iceberg.systemBlue, systemPink: iceberg.systemPink, systemTeal: iceberg.systemTeal,
            systemGray: iceberg.systemGray, isDark: true)
        XCTAssertEqual(bare.projectTint(.purple), iceberg.systemPink)
        XCTAssertEqual(bare.projectTint(.pink), iceberg.systemPink)
        XCTAssertEqual(bare.projectTint(.yellow), iceberg.systemYellow)
    }
}
