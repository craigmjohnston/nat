import AppKit
import SwiftUI

/// Design tokens for the nat UI theme — every colour the app draws with,
/// named by the role it plays rather than by the colour it happens to be.
///
/// Each one is a *dynamic* colour: it holds both palettes and resolves the
/// one the appearance it is drawn under calls for, so a call site says
/// `DesignTokens.windowBg` and gets Mocha on a dark window and Latte on a
/// light one without knowing there are two. That is what makes the theme
/// switch live and what makes `Theme.system` follow macOS — an unpinned
/// window's appearance changes when the Mac's does, and every one of these
/// colours re-resolves with it.
///
/// The values themselves are `Palette`'s, which is where they are documented
/// and where the rules they are chosen against are asserted. Nothing here
/// holds a number: this file is the mapping from a role to a palette field
/// and nothing else.
public enum DesignTokens {
    // MARK: - Resolution

    /// The palette a colour scheme draws with — the seam every token below
    /// is built over, and the one place the two themes are chosen between.
    public static func palette(for scheme: ColorScheme) -> Palette {
        scheme == .dark ? .mocha : .latte
    }

    /// The same choice made from an AppKit appearance, which is what a
    /// dynamic `NSColor` is handed when it is asked to resolve. Anything
    /// that is not positively dark — including an appearance that matches
    /// neither, such as one of the high-contrast variants this app does not
    /// carry values for — resolves light, because light is the platform's
    /// own default and a wrong guess there is a readable window either way.
    static func palette(for appearance: NSAppearance) -> Palette {
        appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua ? .mocha : .latte
    }

    /// A dynamic `NSColor` over one palette field. There is deliberately no
    /// opacity here any more: every colour the app draws is opaque, because
    /// a colour behind an alpha is not a colour the theme chose — it is
    /// whatever the theme's colour and the accident of what sits behind it
    /// happen to make. What used to be an alpha is a mix into a named
    /// ground; see `Palette.rule(_:on:)` and `Palette.wash(_:of:on:)`.
    static func dynamicNSColor<C: PaletteColor>(_ key: KeyPath<Palette, C>) -> NSColor {
        NSColor(name: nil) { appearance in
            NSColor(hex: DesignTokens.palette(for: appearance)[keyPath: key].hex)
        }
    }

    /// A dynamic colour computed from the whole palette rather than read off
    /// one field — every derived colour below goes through this, so a wash
    /// or a rule re-derives when the appearance changes exactly as a plain
    /// token re-reads.
    private static func derived(_ value: @escaping @Sendable (Palette) -> any PaletteColor) -> Color {
        Color(nsColor: NSColor(name: nil) { appearance in
            NSColor(hex: value(DesignTokens.palette(for: appearance)).hex)
        })
    }

    private static func token<C: PaletteColor>(_ key: KeyPath<Palette, C>) -> Color {
        Color(nsColor: dynamicNSColor(key))
    }

    // MARK: - Background & Surface Colors










    // MARK: - Text Colors

    /// Primary label color.
    public static let label = token(\.label)

    /// Secondary label color.
    public static let labelSecondary = token(\.labelSecondary)

    /// Tertiary label color: meta lines and timestamps.
    public static let labelTertiary = token(\.labelTertiary)

    /// Quaternary label color, deliberately below the body-text bar: the
    /// disabled glyph and the empty-slot rule, never words to read.
    public static let labelQuaternary = token(\.labelQuaternary)

    // MARK: - Accent Colors

    /// Primary accent color.
    public static let accent = token(\.accent)

    /// Text color for content on accent background.
    public static let accentText = token(\.accentText)

    /// The app icon's mark gradient, used sparingly: primary actions and
    /// active accents only. It is the brand's own and not the palette's, so
    /// it is the one thing here that is the same under both themes — a brand
    /// that changed colour with the appearance would not be one.
    public static let brandGradient = LinearGradient(
        colors: [Color(hex: "6f4bf2"), Color(hex: "b558d8"), Color(hex: "ff70c2")],
        startPoint: .topLeading,
        endPoint: .bottomTrailing
    )

    // MARK: - Semantic UI Colors

    /// The quiet border that separates surfaces without drawing attention.
    public static func hairline(on ground: Ground) -> Color {
        derived { $0.rule(.hairline, on: ground) }
    }

    /// The soft fill behind a selected row, in place of a solid accent slab.
    public static func selectionWash(on ground: Ground) -> Color {
        derived { $0.wash(.selection, of: $0.accent, on: ground) }
    }

    /// Separator color: the line between two rows of one list.
    public static func separator(on ground: Ground) -> Color {
        derived { $0.rule(.separator, on: ground) }
    }

    /// Control border color: the edge of something the pointer acts on,
    /// which has to read as an edge and not as a suggestion of one.
    public static func controlBorder(on ground: Ground) -> Color {
        derived { $0.rule(.border, on: ground) }
    }

    /// The sweep passing over a loading skeleton block. `Skeleton` names the
    /// block itself and the arithmetic; the colour is the theme's, like
    /// every other.
    public static func skeletonHighlight(on block: Ground) -> Color {
        derived { $0.skeletonHighlight(on: block.surface(in: $0)) }
    }

    /// The rule splitting a filled accent control in two: the launch
    /// button and the chevron that opens its options. The one line in the
    /// app drawn on the accent rather than on a surface, so it is the
    /// accent's own ink behind an alpha rather than `separator`, which is
    /// `label` and would be the wrong ink entirely on a light accent.
    public static let onAccentSeparator = derived { $0.onAccentRule }

    /// The accent as a fill that is present but spent: a finished run of the
    /// progress bar, a send button with nothing to send.
    public static func accentMuted(on ground: Ground) -> Color {
        derived { $0.wash(.muted, of: $0.accent, on: ground) }
    }

    /// The disc an avatar's initials sit on.
    public static func avatarWash(on ground: Ground) -> Color {
        derived { $0.wash(.avatar, of: $0.accent, on: ground) }
    }

    // MARK: - Chip & Badge Washes

    // A chip is its own tint drawn twice: the word at full strength and the
    // capsule behind it at `chipShare`. Each of these is that pair's
    // second half, named for the colour it washes so a call site that has
    // the tint can ask for its wash and cannot pick a different number.

    /// The accent behind its own word.
    public static func accentWash(on ground: Ground) -> Color {
        derived { $0.wash(.chip, of: $0.accent, on: ground) }
    }
    /// Red behind its own word: a closed pull request.
    public static func systemRedWash(on ground: Ground) -> Color {
        derived { $0.wash(.chip, of: $0.systemRed, on: ground) }
    }
    /// Green behind its own word: an open pull request, an added file.
    public static func systemGreenWash(on ground: Ground) -> Color {
        derived { $0.wash(.chip, of: $0.systemGreen, on: ground) }
    }
    /// Yellow behind its own word: a comment not yet sent.
    public static func systemYellowWash(on ground: Ground) -> Color {
        derived { $0.wash(.chip, of: $0.systemYellow, on: ground) }
    }
    /// Orange behind its own word: a modified or renamed file.
    public static func systemOrangeWash(on ground: Ground) -> Color {
        derived { $0.wash(.chip, of: $0.systemOrange, on: ground) }
    }
    /// The secondary label behind its own word: a draft, which is the one
    /// chip state that is deliberately not an outcome colour.
    public static func labelSecondaryWash(on ground: Ground) -> Color {
        derived { Tint($0.labelSecondary.hex).wash(on: ground.surface(in: $0), $0.chipShare) }
    }

    // MARK: - Diff Washes

    /// An added row's own fill, under the line's syntax colours rather than
    /// instead of them.
    public static func diffAddedRowBg(on ground: Ground) -> Color {
        derived { $0.wash(.diffRow, of: $0.systemGreen, on: ground) }
    }
    /// A removed row's own fill.
    public static func diffRemovedRowBg(on ground: Ground) -> Color {
        derived { $0.wash(.diffRow, of: $0.systemRed, on: ground) }
    }
    /// The gutter cell beside an added row — the same green pressed harder,
    /// since a stripe a few characters wide has to carry the sign alone.
    public static func diffAddedGutterBg(on ground: Ground) -> Color {
        derived { $0.wash(.diffGutter, of: $0.systemGreen, on: ground) }
    }
    /// The gutter cell beside a removed row.
    public static func diffRemovedGutterBg(on ground: Ground) -> Color {
        derived { $0.wash(.diffGutter, of: $0.systemRed, on: ground) }
    }
    /// The gutter cell beside a comment row: the faintest mark in the diff,
    /// since a comment is an annotation and not a change.
    public static func diffCommentGutterBg(on ground: Ground) -> Color {
        derived { $0.wash(.comment, of: $0.accent, on: ground) }
    }

    // MARK: - The two resolvers the component layer draws with

    /// The fill of a named ground. This and `ink(_:on:)` below are what the
    /// whole view layer paints with; everything else in this file is the
    /// vocabulary they are built from.
    public static func fill(_ ground: Ground) -> Color {
        derived { ground.surface(in: $0) }
    }

    /// The colour of text in a role, on a ground.
    ///
    /// A label tier is the palette's own ink, shaded only if it cannot be read
    /// where it has landed — which in Mocha is never and in Latte is on the
    /// raised surfaces, whose ramp Catppuccin never meant content to sit on. A
    /// tone is the matching hue under the same rule. See
    /// `Palette.ink(of:on:clearing:)`.
    public static func ink(_ role: InkRole, on ground: Ground) -> Color {
        derived { palette in
            let surface = ground.surface(in: palette)
            switch role {
            case .primary: return palette.readable(palette.label, on: surface)
            case .secondary: return palette.readable(palette.labelSecondary, on: surface)
            // Meta text is held to WCAG's incidental bar rather than AA, which
            // is what keeps the ramp a ramp: shading every tier to 4.5 would
            // pull secondary and tertiary onto the same colour and the
            // hierarchy they exist to draw would be gone.
            case .tertiary: return palette.readable(palette.labelTertiary, on: surface, clearing: 3)
            // Deliberately unshaded: the disabled glyph and the empty-slot
            // rule are meant to recede, and WCAG exempts them.
            case .quaternary: return palette.labelQuaternary
            case .onAccent: return palette.accentText
            case .accent: return palette.ink(of: palette.accent, on: surface)
            case .success: return palette.ink(of: palette.systemGreen, on: surface)
            case .danger: return palette.ink(of: palette.systemRed, on: surface)
            case .warning: return palette.ink(of: palette.systemYellow, on: surface)
            case .info: return palette.ink(of: palette.systemBlue, on: surface)
            }
        }
    }

    // MARK: - A hue written on a ground

    // A hue used as a *fill* is the tint itself — the tokens below — since a
    // filled dot or a filled button is the colour and has nothing written on
    // it. A hue used as *ink* has to survive the ground it is written on, and
    // that is what these are: the theme's own colour, shaded only as far as
    // it must be. See `Palette.ink(of:on:clearing:)` for why this exists and
    // why mixing toward the theme's text was the wrong answer.

    public static func accentInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.accent, on: ground.surface(in: $0)) }
    }
    public static func systemRedInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemRed, on: ground.surface(in: $0)) }
    }
    public static func systemGreenInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemGreen, on: ground.surface(in: $0)) }
    }
    public static func systemYellowInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemYellow, on: ground.surface(in: $0)) }
    }
    public static func systemOrangeInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemOrange, on: ground.surface(in: $0)) }
    }
    public static func systemBlueInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemBlue, on: ground.surface(in: $0)) }
    }
    public static func systemTealInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemTeal, on: ground.surface(in: $0)) }
    }
    public static func systemPinkInk(on ground: Ground) -> Color {
        derived { $0.ink(of: $0.systemPink, on: ground.surface(in: $0)) }
    }

    /// The capsule behind a chip's word.
    public static func chipWash(_ tint: ChipTint, on ground: Ground) -> Color {
        derived { $0.wash(.chip, of: tint.tint(in: $0), on: ground) }
    }

    /// A rule at one of the three weights, on a ground.
    public static func rule(_ weight: RuleWeight, on ground: Ground) -> Color {
        derived { $0.rule(weight, on: ground) }
    }

    /// The word inside a chip, over the chip's own capsule. Which hue is
    /// named rather than passed as a key path, since a `KeyPath` is not
    /// `Sendable` and this closure outlives the call.
    public enum ChipTint: Sendable {
        case accent, red, green, yellow, orange, labelSecondary

        func tint(in palette: Palette) -> Tint {
            switch self {
            case .accent: palette.accent
            case .red: palette.systemRed
            case .green: palette.systemGreen
            case .yellow: palette.systemYellow
            case .orange: palette.systemOrange
            case .labelSecondary: Tint(palette.labelSecondary.hex)
            }
        }
    }

    public static func chipInk(_ tint: ChipTint, on ground: Ground) -> Color {
        derived { $0.chipInk(of: tint.tint(in: $0), on: ground) }
    }

    // MARK: - System Color Overrides

    /// Orange system color.
    public static let systemOrange = token(\.systemOrange)

    /// Yellow system color.
    public static let systemYellow = token(\.systemYellow)

    /// Green system color.
    public static let systemGreen = token(\.systemGreen)

    /// Red system color.
    public static let systemRed = token(\.systemRed)

    /// Blue system color.
    public static let systemBlue = token(\.systemBlue)

    /// Pink system color.
    public static let systemPink = token(\.systemPink)

    /// Teal system color.
    public static let systemTeal = token(\.systemTeal)

    /// Gray system color.
    public static let systemGray = token(\.systemGray)
}

/// The type ramp, the mock's own sizes grown one point after real use read
/// them as too small on a big display. Every font in the app comes off this
/// ramp — changing how big the app reads is changing these numbers and
/// nothing else.
public enum Typo {
    /// Section headings and the PR title (mock's headline, 15).
    public static let headline: CGFloat = 15
    /// Body text and row labels (mock's 13px body, grown to 14).
    public static let body: CGFloat = 14
    /// Secondary rows, meta lines, tab labels (mock's 11px subheadline → 12).
    public static let subhead: CGFloat = 12
    /// Badges, timestamps, section labels (mock's 10px caption2 → 11).
    public static let caption: CGFloat = 11
    /// Monospaced code and diff text (mock's 12px code → 13).
    public static let code: CGFloat = 13
}

/// The geometry every button in the app is drawn to. One place, because the
/// point of a button grammar is that a primary submit is the same shape
/// wherever it is pressed — the footer of the brief pane, the foot of a
/// sheet, the split control that launches an agent — and three files each
/// picking their own padding is how five different submit buttons happened.
///
/// The height is stated rather than left to the label's padding: a button
/// whose height falls out of its font and its insets is one that changes
/// size when the label does, and a spinner appearing beside the label is
/// exactly such a change. `AsyncActionLabel` holds the width; this holds the
/// height.
public enum ButtonMetrics {
    /// The height of every primary and secondary button.
    public static let height: CGFloat = 22
    /// The corner radius of their fills and strokes.
    public static let cornerRadius: CGFloat = 6
    /// The inset either side of the label.
    public static let horizontalPadding: CGFloat = 12
    /// The inset either side of a ghost button's label, which carries no
    /// fill and so needs less room around it to read as one thing.
    public static let ghostHorizontalPadding: CGFloat = 8
    /// What a button is dimmed to while it is disabled. The split control
    /// dims as a whole to this too, rather than through each half's own
    /// disabled state, since half a dimmed control reads as half of it being
    /// unavailable.
    public static let disabledOpacity: Double = 0.55
    /// What a filled button is dimmed to while it is held down.
    public static let pressedOpacity: Double = 0.85
}

/// Motion, per the design system's own rules (state changes at 0.15–0.35s
/// ease-out, nothing else) and Craig's read that anything slower drags: the
/// fast end, in one place, so turning animation off entirely is making
/// `stateChange` nil here and nowhere else.
public enum Motion {
    /// The one animation state changes (expand/collapse, selection) use.
    /// nil disables them app-wide.
    public static let stateChange: Animation? = .easeOut(duration: 0.15)
}

// MARK: - Hex Color Initializers

/// The six hex digits of a colour, or nil for anything that is not exactly
/// that — the one parse both colour initializers below are built on, so a
/// `Color` and an `NSColor` made from the same string can never disagree
/// about what it means.
func rgbComponents(hex: String) -> (red: Double, green: Double, blue: Double)? {
    let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
    guard hex.count == 6 else { return nil }

    let scanner = Scanner(string: hex)
    var rgb: UInt64 = 0
    guard scanner.scanHexInt64(&rgb) else { return nil }

    return (
        Double((rgb >> 16) & 0xFF) / 255.0,
        Double((rgb >> 8) & 0xFF) / 255.0,
        Double(rgb & 0xFF) / 255.0
    )
}

/// What a hex that will not parse resolves to: Catppuccin Mocha's mauve,
/// which is the app's own accent.
///
/// It used to be white — the one colour in the app guaranteed to belong to
/// neither palette, and so the one whose appearance says "this is a bug"
/// only to somebody who already knows it is the fallback. A colour that
/// cannot be read *is* a bug, and it is caught by `PaletteTests` rather than
/// by a glance at the window; what the fallback is for is the frame drawn
/// before anybody looks at the test, and a frame drawn in the accent is one
/// that still reads as this app. Written out channel by channel rather than
/// parsed from `Palette.mocha.accent`, because the fallback for a parse
/// cannot itself depend on a parse succeeding — `hexFallbackIsTheAccent`
/// asserts the two agree.
let hexFallback: (red: Double, green: Double, blue: Double) = (
    Double(0xcb) / 255.0, Double(0xa6) / 255.0, Double(0xf7) / 255.0
)

extension Color {
    /// Initialize a Color from a hex string (6 characters, e.g., "1e1e23").
    /// Invalid input (non-hex characters, wrong length) falls back to
    /// `hexFallback`, the accent — never to a colour off the palette.
    public init(hex: String) {
        let rgb = rgbComponents(hex: hex) ?? hexFallback
        self.init(red: rgb.red, green: rgb.green, blue: rgb.blue)
    }
}

extension NSColor {
    /// The AppKit half of `Color(hex:)`, for the places a native colour is
    /// what is wanted: the dynamic tokens above, and the terminal view,
    /// which takes `NSColor`s rather than SwiftUI ones. Same parse, same
    /// fallback.
    public convenience init(hex: String) {
        let rgb = rgbComponents(hex: hex) ?? hexFallback
        self.init(srgbRed: rgb.red, green: rgb.green, blue: rgb.blue, alpha: 1)
    }
}
