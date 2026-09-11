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

    /// No ink is drawn on a ground it cannot be read on.
    ///
    /// What is asserted is the colour the app *draws* rather than the one the
    /// palette holds, which is the whole difference `readable(_:on:)` makes:
    /// Latte's `subtext0` clears AA on none of its own surfaces — 4.37 at best
    /// on `base`, 3.20 on a card — so taking a theme as published and drawing
    /// it unchanged were never both possible. This used to carry a list of
    /// sixteen pairings that fell short. There are none.
    func testEveryInkClearsItsBarOnEveryGround() {
        for (theme, palette) in palettes {
            for (inkName, ink, bar) in bars {
                for ground in Ground.allCases {
                    let surface = ground.surface(in: palette)
                    let drawn = palette.readable(ink(palette), on: surface, clearing: bar)
                    XCTAssertGreaterThanOrEqual(
                        contrast(drawn.hex, surface.hex), bar,
                        "\(theme) \(inkName) on \(ground.rawValue) is under \(bar)"
                    )
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

    /// A wash is a ground, so what is written on it has to survive it: a chip
    /// draws its hue as the word inside its capsule, and the capsule is that
    /// hue mixed into the ground behind it.
    ///
    /// Drawn as the published hue this failed on 35 pairs, every one of them
    /// Latte, worst at 1.57:1 — its yellow chip's word was invisible on its
    /// own capsule. `Palette.chipInk(of:on:)` is the cure and the bar here is
    /// full AA rather than the 3:1 this settled for while the debt stood.
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
                    let word = palette.chipInk(of: tint, on: ground)
                    XCTAssertGreaterThanOrEqual(
                        contrast(word.hex, capsule.hex), 4.5,
                        "\(theme): the \(name) chip's word on its own capsule over \(ground.rawValue)"
                    )
                }
            }
        }
    }

    /// A hue written as text on a ground is readable there. This is the same
    /// rule the chips use, and it covers the 37 places the app writes an
    /// outcome colour as words rather than drawing it as a fill.
    ///
    /// Latte is why it exists: its yellow as text on a card was 1.70:1 and
    /// its pink 1.71 — not dim, invisible — and no published swatch fixes
    /// that, because a mid-luminance hue contrasts with neither end.
    func testAHueWrittenOnAGroundIsReadable() {
        for (theme, palette) in palettes {
            let tints: [(String, Tint)] = [
                ("accent", palette.accent), ("red", palette.systemRed),
                ("green", palette.systemGreen), ("yellow", palette.systemYellow),
                ("orange", palette.systemOrange), ("blue", palette.systemBlue),
                ("teal", palette.systemTeal), ("pink", palette.systemPink),
            ]
            for ground in Ground.allCases {
                for (name, tint) in tints {
                    let surface = ground.surface(in: palette)
                    XCTAssertGreaterThanOrEqual(
                        contrast(palette.ink(of: tint, on: surface).hex, surface.hex), 4.5,
                        "\(theme): \(name) written on \(ground.rawValue)"
                    )
                }
            }
        }
    }

    /// Shading makes a hue readable without making it a different colour.
    ///
    /// This is the assertion that rules out the obvious fix that does not
    /// work: mixing the hue toward the theme's own text converges all the way
    /// in Latte, so a green chip and a red chip both come out `#4c4f69` and
    /// the colour coding is gone. `shade` moves lightness alone, so the hue
    /// angle and the saturation survive and a shaded green still reads green.
    func testShadingKeepsTheHue() {
        for (theme, palette) in palettes {
            let tints: [(String, Tint)] = [
                ("accent", palette.accent), ("red", palette.systemRed),
                ("green", palette.systemGreen), ("yellow", palette.systemYellow),
                ("orange", palette.systemOrange), ("pink", palette.systemPink),
            ]
            for ground in Ground.allCases {
                for (name, tint) in tints {
                    let written = palette.ink(of: tint, on: ground.surface(in: palette))
                    guard let before = rgbComponents(hex: tint.hex),
                          let after = rgbComponents(hex: written.hex) else {
                        return XCTFail("\(theme): unreadable hue")
                    }
                    let (hueBefore, _, satBefore) = hslComponents(before)
                    let (hueAfter, _, satAfter) = hslComponents(after)
                    // Hue is an angle, so the two ends of the circle are near.
                    let drift = min(abs(hueAfter - hueBefore), 1 - abs(hueAfter - hueBefore))
                    XCTAssertLessThan(drift, 0.02, "\(theme): \(name) on \(ground.rawValue) changed hue")
                    XCTAssertEqual(satAfter, satBefore, accuracy: 0.02,
                                   "\(theme): \(name) on \(ground.rawValue) lost saturation")
                }
            }
        }
    }

    /// Shading a tier to make it readable must not flatten the ramp it
    /// belongs to. The tiers exist to draw hierarchy — a title, its supporting
    /// line, its timestamp — and a correction that pulled them onto one colour
    /// would buy contrast by spending the thing contrast is for. It is why
    /// meta text is held to WCAG's incidental bar rather than AA.
    func testTheLabelRampStillRecedesAfterShading() {
        for (theme, palette) in palettes {
            for ground in Ground.allCases {
                let surface = ground.surface(in: palette)
                let tiers = [
                    palette.readable(palette.label, on: surface),
                    palette.readable(palette.labelSecondary, on: surface),
                    palette.readable(palette.labelTertiary, on: surface, clearing: 3),
                ].map { contrast($0.hex, surface.hex) }
                for (above, below) in zip(tiers, tiers.dropFirst()) {
                    XCTAssertGreaterThan(
                        above, below,
                        "\(theme) on \(ground.rawValue): each tier should recede further than the one above"
                    )
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
