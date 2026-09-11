import XCTest
import SwiftUI
@testable import NatKit

/// The component layer: the vocabulary the view code actually writes in.
///
/// What these hold is that saying what a thing is *for* — a card, a chip, a
/// secondary line — is enough to get it drawn correctly on whatever it has
/// landed on. The view layer names no colour at all now, so the guarantee has
/// to live here.
@MainActor
final class ComponentsTests: XCTestCase {
    private let roles: [InkRole] = [
        .primary, .secondary, .tertiary, .quaternary, .onAccent,
        .accent, .success, .danger, .warning, .info,
    ]

    /// Every role resolves to a real, opaque colour on every ground.
    func testEveryInkRoleResolvesOnEveryGround() {
        for role in roles {
            for ground in Ground.allCases {
                let token = NSColor(DesignTokens.ink(role, on: ground))
                for name in [NSAppearance.Name.darkAqua, .aqua] {
                    let resolved = NSAppearance(named: name).flatMap { appearance -> NSColor? in
                        var color: NSColor?
                        appearance.performAsCurrentDrawingAppearance {
                            color = token.usingColorSpace(.sRGB)
                        }
                        return color
                    }
                    XCTAssertNotNil(resolved, "\(role) on \(ground.rawValue)")
                    XCTAssertEqual(resolved?.alphaComponent, 1, "\(role) on \(ground.rawValue) should be opaque")
                }
            }
        }
    }

    /// And every ground has a fill.
    func testEveryGroundHasAFill() {
        for ground in Ground.allCases {
            XCTAssertNotNil(NSColor(DesignTokens.fill(ground)).usingColorSpace(.sRGB), ground.rawValue)
        }
    }

    /// A tone names an outcome; this is the hue each one reaches for. Asserted
    /// rather than assumed because it is the one place `.success` becomes
    /// green — a theme may disagree about which green, but not about
    /// `.success` meaning the palette's own.
    func testToneMapsToItsHue() {
        let pairs: [(Tone, String)] = [
            (.accent, Palette.mocha.accent.hex),
            (.success, Palette.mocha.systemGreen.hex),
            (.danger, Palette.mocha.systemRed.hex),
            (.warning, Palette.mocha.systemYellow.hex),
            (.neutral, Palette.mocha.labelSecondary.hex),
        ]
        for (tone, hex) in pairs {
            XCTAssertEqual(tone.chipTint.tint(in: .mocha).hex, hex, "\(tone)")
        }
    }

    /// Every chip tone is legible on every ground in both themes — the pairing
    /// `PairingTests` holds at the palette level, asserted here through the
    /// component's own vocabulary.
    func testEveryChipToneIsLegibleOnEveryGround() {
        for tone in [Tone.accent, .success, .danger, .warning, .neutral] {
            for ground in Ground.allCases {
                for palette in [Palette.mocha, Palette.latte] {
                    let tint = tone.chipTint.tint(in: palette)
                    let capsule = palette.wash(.chip, of: tint, on: ground)
                    let word = palette.chipInk(of: tint, on: ground)
                    XCTAssertGreaterThanOrEqual(
                        contrastRatio(word.hex, capsule.hex), 4.5,
                        "\(tone) chip on \(ground.rawValue)"
                    )
                }
            }
        }
    }

    /// A view not inside anything is on the window, which is what makes the
    /// default right: every call site that never says otherwise is drawn on
    /// the app's own ground.
    func testTheDefaultGroundIsTheWindow() {
        XCTAssertEqual(EnvironmentValues().ground, .window)
    }

    /// The components build. A SwiftUI body cannot be asserted about without a
    /// renderer, but it can be demanded to exist — which catches the modifier
    /// that stopped compiling against a token that moved.
    func testComponentsBuild() {
        _ = Card { Text("card") }.body
        _ = Card(radius: 6, border: false) { Text("plain") }.body
        _ = Band { Text("band") }.body
        _ = Field { Text("field") }.body
        _ = Chip("open", tone: .success).body
        _ = Rule().body
        _ = Rule(.hairline, axis: .vertical).body
        _ = Text("x").surface(.card, radius: 8)
        _ = Text("x").surface(.window)
        _ = Text("x").ink(.secondary)
    }
}
