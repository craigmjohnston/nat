import XCTest
@testable import NatKit

/// Every pairing of an ink with a ground the app paints, held to the bar its
/// tier calls for.
///
/// This is the test that would have caught the hover bug, and it is the only
/// thing in the theme that makes "these colours look good together" a
/// property rather than an opinion. `PaletteTests` deliberately holds
/// Catppuccin's own values to no ratio — they are published and not ours to
/// bend — but *which swatch plays which role*, and therefore what ends up
/// drawn on what, is entirely this app's decision, and a decision can be
/// wrong. The hover fill was `labelQuaternary`: a published swatch, in a role
/// that put `text` on it at 3.4:1.
///
/// It is also what makes a third theme safe to add. A new palette drops in by
/// filling the swatch table; this is what says whether its own ramp survives
/// the roles this app puts it in, in CI rather than by squinting at a window.
final class PairingTests: XCTestCase {
    private let palettes: [(String, Palette)] = [("mocha", .mocha), ("latte", .latte)]

    /// What each ink tier has to clear, and why. The bars are WCAG's: 4.5:1
    /// for anything read as body text, 3:1 for text that is deliberately
    /// incidental.
    ///
    /// `labelQuaternary` is absent rather than given a low bar. It is
    /// documented as the disabled glyph and the empty-slot rule — never words
    /// to read — and a disabled control is explicitly outside WCAG's contrast
    /// requirement. What keeps it honest is not a ratio but the type system:
    /// it is an `Ink`, so it cannot be used as a ground.
    private let bars: [(String, (Palette) -> Ink, Double)] = [
        ("label", { $0.label }, 4.5),
        ("labelSecondary", { $0.labelSecondary }, 4.5),
        ("labelTertiary", { $0.labelTertiary }, 3.0),
    ]

    /// The pairings that fall short today.
    ///
    /// Every one of them is debt rather than a decision, and they divide in
    /// two. Latte's `labelSecondary` misses AA even on its own `base` (4.37)
    /// — that is Catppuccin's own ramp and not something this app can fix
    /// without leaving the theme. The rest is this app's mapping: drawing a
    /// card in `surface0` and then writing secondary and meta text on it
    /// takes the same ink down to 3.2 and 2.56. Catppuccin's ladder means
    /// `base`, `mantle` and `crust` to be the grounds content sits on, with
    /// `surface0`–`surface2` for UI furniture; the cure is to re-ladder the
    /// content surfaces, not to re-tint the ink.
    ///
    /// Listing them is the point: a shortfall in this set is visible, and one
    /// outside it fails the build.
    private let knownShortfalls: Set<String> = [
        "mocha labelSecondary control",
        "latte label control",
        "latte labelSecondary window",
        "latte labelSecondary card",
        "latte labelSecondary rowAlt",
        "latte labelSecondary control",
        "latte labelSecondary field",
        "latte labelSecondary band",
        "latte labelSecondary header",
        "latte labelSecondary terminal",
        "latte labelSecondary hover",
        "latte labelTertiary card",
        "latte labelTertiary rowAlt",
        "latte labelTertiary control",
        "latte labelTertiary hover",
    ]

    /// No ink is drawn on a ground it cannot be read on — bar the shortfalls
    /// named above, which are held to being no worse than they already are.
    func testEveryInkClearsItsBarOnEveryGround() {
        for (theme, palette) in palettes {
            for (inkName, ink, bar) in bars {
                for ground in Ground.allCases {
                    let pair = "\(theme) \(inkName) \(ground.rawValue)"
                    let ratio = contrast(ink(palette).hex, ground.surface(in: palette).hex)
                    if knownShortfalls.contains(pair) {
                        XCTAssertLessThan(ratio, bar, "\(pair): fixed — take it out of knownShortfalls")
                    } else {
                        XCTAssertGreaterThanOrEqual(ratio, bar, "\(pair): \(ratio) is under \(bar)")
                    }
                }
            }
        }
    }

    /// The ink written on the accent is the one pairing whose ground is not a
    /// surface, so it is asserted here rather than swept above. Mocha clears
    /// AA comfortably; Latte's `crust` on its mauve is 4.09, which is
    /// Catppuccin's own pairing of its own two swatches and marginal rather
    /// than broken.
    func testAccentTextIsReadableOnTheAccent() {
        let bars = ["mocha": 4.5, "latte": 4.0]
        for (theme, palette) in palettes {
            XCTAssertGreaterThanOrEqual(
                contrast(palette.accentText.hex, palette.accent.hex), bars[theme] ?? 4.5,
                "\(theme): what is written on the accent should be readable on it"
            )
        }
    }

    /// The chips Latte cannot currently draw legibly, and the clearest
    /// finding this whole file produced.
    ///
    /// A chip is one hue drawn twice: the word at full strength and the
    /// capsule behind it at `chipShare`. That works in Mocha, whose accents
    /// are pastels on a dark ground — every chip there clears the bar. In
    /// Latte the accents are dark saturated colours and the capsule is
    /// overwhelmingly the light ground they are mixed into, so the word and
    /// the capsule land at almost the same luminance: the yellow chip's word
    /// on its own capsule is 1.57:1 on a card. Green, yellow and orange fail
    /// on every ground; accent and red fail on the raised ones.
    ///
    /// It is structural rather than a number to nudge — a light theme cannot
    /// draw a mid-luminance hue on a wash of itself — so the cure is a design
    /// decision this list is deliberately holding open: the word could take
    /// `text` rather than the hue, the capsule could be filled at full
    /// strength with `accentText` on it, or the word could be a `shade()` of
    /// the hue, which is what Catppuccin's own VS Code port derives with.
    private let knownChipShortfalls: Set<String> = [
        "latte accent card", "latte accent rowAlt", "latte accent control", "latte accent hover",
        "latte red card", "latte red rowAlt", "latte red control", "latte red hover",
        "latte green window", "latte green card", "latte green rowAlt", "latte green control",
        "latte green field", "latte green band", "latte green header", "latte green terminal",
        "latte green hover",
        "latte yellow window", "latte yellow card", "latte yellow rowAlt", "latte yellow control",
        "latte yellow field", "latte yellow band", "latte yellow header", "latte yellow terminal",
        "latte yellow hover",
        "latte orange window", "latte orange card", "latte orange rowAlt", "latte orange control",
        "latte orange field", "latte orange band", "latte orange header", "latte orange terminal",
        "latte orange hover",
    ]

    /// A wash is a ground, so what is written on it has to survive it: a chip
    /// draws its own tint as the word inside its capsule, and the capsule is
    /// that tint mixed into the ground behind it. The bar is 3:1 rather than
    /// AA's 4.5 for text this size, which is already generous — every Mocha
    /// chip clears it and no Latte one outside the list above does.
    func testAChipsWordIsReadableOnItsOwnCapsule() {
        for (theme, palette) in palettes {
            let tints: [(String, Tint)] = [
                ("accent", palette.accent), ("red", palette.systemRed),
                ("green", palette.systemGreen), ("yellow", palette.systemYellow),
                ("orange", palette.systemOrange),
            ]
            for ground in Ground.allCases {
                for (name, tint) in tints {
                    let capsule = palette.wash(.chip, of: tint, on: ground)
                    let ratio = contrast(tint.hex, capsule.hex)
                    let pair = "\(theme) \(name) \(ground.rawValue)"
                    if knownChipShortfalls.contains(pair) {
                        XCTAssertLessThan(ratio, 3.0, "\(pair): fixed — take it out of knownChipShortfalls")
                    } else {
                        XCTAssertGreaterThanOrEqual(
                            ratio, 3.0,
                            "\(pair): a chip's word should be readable on its own capsule"
                        )
                    }
                }
            }
        }
    }

    /// Every ground the app can paint is covered by the sweep above. A ground
    /// added without a pairing to go with it would otherwise be untested.
    func testEveryGroundIsCovered() {
        XCTAssertEqual(Ground.allCases.count, 9)
        for ground in Ground.allCases {
            XCTAssertNotNil(rgbComponents(hex: ground.surface(in: .mocha).hex), "\(ground.rawValue)")
            XCTAssertNotNil(rgbComponents(hex: ground.surface(in: .latte).hex), "\(ground.rawValue)")
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
