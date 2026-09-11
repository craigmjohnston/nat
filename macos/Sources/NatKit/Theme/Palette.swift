import Foundation

// MARK: - What a colour may be used for

/// The one thing `Surface`, `Ink` and `Tint` have in common: they are each
/// six hex digits. It exists so `DesignTokens` can resolve any of them
/// without being able to tell them apart — the telling apart is the point of
/// the three types, and it belongs at the call site, not in the resolver.
public protocol PaletteColor: Equatable, Sendable {
    var hex: String { get }
}

/// A colour that may be drawn *behind* something: a pane, a card, a row, a
/// chip's capsule, the fill under the pointer.
///
/// It is a type rather than a naming convention because the app has shipped
/// the same bug three times — `labelQuaternary` as the hover fill, as the
/// project tab's live-count badge and as the loading skeleton's block — and
/// every one of them was a published Catppuccin swatch used in a role it was
/// never meant for. Provenance was never the problem; a flat bag of `String`
/// was. Ground and ink cannot be swapped if they are not the same type.
public struct Surface: PaletteColor {
    public let hex: String
    public init(_ hex: String) { self.hex = hex }
}

/// A colour that may be drawn *on* a ground: text, a glyph, a rule, a border.
/// Never a fill — see `Surface`.
public struct Ink: PaletteColor {
    public let hex: String
    public init(_ hex: String) { self.hex = hex }
}

/// One of the theme's hues — the accent and the outcome colours. It is `Ink`
/// at full strength and the *basis* of a `Surface` when mixed into a ground,
/// but is neither as it stands: a hue laid straight down as a fill is how a
/// chip ends up with its own word unreadable on it.
public struct Tint: PaletteColor {
    public let hex: String
    public init(_ hex: String) { self.hex = hex }

    /// The hue as something to draw with.
    public var ink: Ink { Ink(hex) }

    /// The hue mixed into a ground, as a ground: a chip's capsule, a diff
    /// row's fill, the wash behind a selected row. Opaque, and computed from
    /// the two colours it is made of rather than laid over whatever happens
    /// to be behind — which is the whole difference between a wash that is
    /// part of the theme and an alpha that shows the desktop through.
    public func wash(on ground: Surface, _ share: Double) -> Surface {
        Surface(mix(hex, ground.hex, share))
    }
}

// MARK: - Derivation

/// `amount` of the first colour mixed into the second, channel by channel.
///
/// This is the only way a colour that is not a published swatch comes to
/// exist in this app, and it is deliberately the same operation Catppuccin's
/// own VS Code port derives with (`mix`, `opacity`, `shade` over named
/// swatches — `list.hoverBackground` there is `opacity(surface0, 0.5)` and
/// `tab.hoverBackground` is `shade(base, 0.05)`, neither of which is a
/// published swatch either). What a theme must never contain is a *typed*
/// hex: an expression over two of the theme's own colours ports to a new
/// theme by itself, and a literal has to be invented again for every one.
public func mix(_ color: String, _ into: String, _ amount: Double) -> String {
    guard let first = rgbComponents(hex: color), let second = rgbComponents(hex: into) else {
        return into
    }
    let share = min(max(amount, 0), 1)
    let channels = [
        first.red * share + second.red * (1 - share),
        first.green * share + second.green * (1 - share),
        first.blue * share + second.blue * (1 - share),
    ]
    return channels.map { String(format: "%02x", Int((($0 * 255).rounded()))) }.joined()
}

/// One theme's raw values: every colour the app draws with, as the hex
/// string it is defined by, plus the few opacities that are the theme's own
/// rather than a colour's.
///
/// It is deliberately a plain value type of strings and numbers and not a
/// bag of `Color`s: a `Color` cannot be compared or asserted about, and the
/// two palettes here are exactly the thing that has to be checked against a
/// published spec. `DesignTokens` is what turns one into the dynamic colours
/// SwiftUI draws — see `DesignTokens.palette(for:)`.
///
/// The two palettes are Catppuccin Mocha and Catppuccin Latte, the same
/// family the Go TUI draws with, so both faces of the product read as one
/// product. Both are taken as published: every value below is Catppuccin's
/// own, named by the swatch it comes from, and the job here is choosing
/// which swatch plays which role rather than choosing colours. A palette
/// this widely used has been read in anger by more people than any rule
/// applied here would stand in for, and a value tweaked to satisfy one would
/// no longer be the theme the user recognises.
public struct Palette: Equatable, Sendable {
    // MARK: - Surfaces

    /// The app's ground, and the fill of every pane that is not a card.
    public let windowBg: Surface
    /// The face of a card raised off the ground.
    public let controlBg: Surface
    /// The band that has to read apart from a card it sits inside.
    public let rowAltBg: Surface
    /// The face of a control the pointer acts on.
    public let controlFace: Surface
    /// The well text is typed into.
    public let fieldBg: Surface
    /// The fill under the pointer: a rail row, a project tab, a stepper
    /// stage, a ghost button in the header. `surface0` in both themes —
    /// the swatch Catppuccin's own ports hover with — and a role of its own
    /// rather than `controlBg` borrowed, because what a hover has to do is
    /// read as one step off whatever it is drawn over while the label on it
    /// stays a label, which is a rule about text on a surface and not about
    /// card faces. It used to be `labelQuaternary`, a colour documented as
    /// ink and never as ground: under Mocha that put `text` on `overlay0`,
    /// light on light, and the words went to mush exactly where the pointer
    /// was.
    public let hoverWash: Surface

    // MARK: - Terminal

    /// The agent terminal's own surface. It sits at `fieldBg`'s level in
    /// both themes on purpose: a terminal is the same kind of thing as a
    /// text field — a well the app writes into.
    public let terminalBg: Surface
    /// The terminal's default foreground, which is `label` in both themes:
    /// the pane is part of the window rather than a second product embedded
    /// in it.
    public let terminalFg: Ink
    /// The terminal's caret.
    public let terminalCursor: Tint
    /// The wash behind selected terminal text.
    public let terminalSelection: Surface
    /// The sixteen ANSI colours, in the order a terminal numbers them:
    /// black, red, green, yellow, blue, magenta, cyan, white, then the same
    /// eight bright.
    public let ansi: [String]

    // MARK: - Text

    /// Primary label colour.
    public let label: Ink
    /// Secondary label colour — a real colour rather than the primary behind
    /// opacity, so it is the same colour over whatever surface it lands on.
    public let labelSecondary: Ink
    /// Tertiary label colour: meta lines and timestamps.
    public let labelTertiary: Ink
    /// Quaternary label colour: the disabled glyph and the empty-slot rule,
    /// never words to read.
    public let labelQuaternary: Ink

    // MARK: - Accent

    /// Primary accent colour.
    public let accent: Tint
    /// What is written on top of the accent.
    public let accentText: Ink

    // MARK: - Opacities

    /// `label` at this opacity is the quiet border between two surfaces.
    public let hairlineShare: Double
    /// `label` at this opacity is the line between two rows of one list.
    public let separatorShare: Double
    /// `label` at this opacity is the edge of something the pointer acts on.
    public let borderShare: Double
    /// `accent` at this opacity is the fill behind a selected row.
    public let selectionShare: Double

    // The washes below are the second kind of opacity here: not a border's
    // weight but the share of a colour that shows when it is laid on a
    // surface as a band, a chip or a stripe. Each is pressed per theme for
    // the same reason `selectionShare` and the border ramp already
    // are — a colour laid at one alpha does not read the same over a dark
    // ground as over a light one — and in the same two directions:
    //
    //   * a wash of a *hue* — an accent, an outcome colour — is lighter in
    //     Latte, whose mauves, greens and reds are dark, saturated colours
    //     that draw a far heavier slab over a light ground than Mocha's
    //     pastels do over a dark one;
    //   * a wash of `label` is heavier in Latte, since dark ink on a light
    //     ground reads fainter than light ink on a dark one at the same
    //     alpha. That is the border ramp's own rule.
    //
    // Two are the same in both, and say so where they are declared: a mix
    // of two of the palette's own surfaces re-balances by itself, and a
    // colour dimmed to say it is spent is a fraction of the thing beside it
    // rather than a wash over anything.

    /// `controlBg` at this opacity is a band laid over the window ground:
    /// the pane's header, the brief's footer, a notice row. Half a card, so
    /// it reads as a band of the pane rather than as a card of its own.
    /// The one wash that is a mix of two of the palette's own surfaces
    /// rather than a colour over a ground, so it needs no per-theme
    /// pressing: half way between `base` and `surface0` is half way between
    /// them in either theme.
    public let bandShare: Double
    /// A chip or badge drawn behind its own tint — the PR state capsule, the
    /// change-kind letter, the pending marker. Enough to shape the chip,
    /// never enough to compete with the word inside it.
    public let chipShare: Double
    /// `accent` at this opacity is the disc an avatar's initials sit on.
    /// Heavier than a chip: a disc is small and has to read as a disc.
    public let avatarShare: Double
    /// An outcome colour at this opacity is a diff row's own fill, under the
    /// line's syntax colours rather than instead of them.
    public let diffRowShare: Double
    /// The same outcome colour, pressed harder, in the diff's gutter cell:
    /// the gutter is a stripe a few characters wide and needs to carry the
    /// row's sign on its own.
    public let diffGutterShare: Double
    /// `accent` at this opacity is the gutter beside a comment row — the
    /// faintest mark in the diff, since a comment is an annotation and not a
    /// change.
    public let commentShare: Double
    /// `accent` at this opacity is the veil over the header band, the flat
    /// stand-in for the mock's `color-mix(in srgb, accent 9%, header)`.
    public let headerVeilShare: Double
    /// `accent` at this opacity is a fill that is present but spent: a
    /// finished run of the progress bar, a send button with nothing to send.
    /// A dim of the accent rather than a wash of it — what it is read
    /// against is the full accent beside it, not the ground under it — which
    /// is why it is far heavier than the washes above and why it is the same
    /// fraction in both themes.
    public let mutedShare: Double
    /// `label` at this opacity is the sweep passing over a skeleton block —
    /// brighter than the block and still far under anything drawn as text.
    public let skeletonShare: Double
    /// `accentText` at this opacity is the rule splitting a filled accent
    /// control in two — the launch button and its options chevron. The one
    /// line in the app drawn *on* the accent rather than on a surface, which
    /// is why it is the accent's own ink behind an alpha rather than
    /// `separator`, and why it is the same in both themes: `accentText` is
    /// the maximum-contrast ink over the accent in either.
    public let onAccentRuleShare: Double

    // MARK: - Outcome colours

    public let systemOrange: Tint
    public let systemYellow: Tint
    public let systemGreen: Tint
    public let systemRed: Tint
    public let systemBlue: Tint
    public let systemPink: Tint
    public let systemTeal: Tint
    public let systemGray: Tint

    /// Whether this palette paints light text on dark surfaces — which is
    /// the one thing about a theme that anything outside it needs to know.
    public let isDark: Bool

    public init(
        windowBg: Surface,
        controlBg: Surface,
        rowAltBg: Surface,
        controlFace: Surface,
        fieldBg: Surface,
        hoverWash: Surface,
        terminalBg: Surface,
        terminalFg: Ink,
        terminalCursor: Tint,
        terminalSelection: Surface,
        ansi: [String],
        label: Ink,
        labelSecondary: Ink,
        labelTertiary: Ink,
        labelQuaternary: Ink,
        accent: Tint,
        accentText: Ink,
        hairlineShare: Double,
        separatorShare: Double,
        borderShare: Double,
        selectionShare: Double,
        bandShare: Double,
        chipShare: Double,
        avatarShare: Double,
        diffRowShare: Double,
        diffGutterShare: Double,
        commentShare: Double,
        headerVeilShare: Double,
        mutedShare: Double,
        skeletonShare: Double,
        onAccentRuleShare: Double,
        systemOrange: Tint,
        systemYellow: Tint,
        systemGreen: Tint,
        systemRed: Tint,
        systemBlue: Tint,
        systemPink: Tint,
        systemTeal: Tint,
        systemGray: Tint,
        isDark: Bool
    ) {
        self.windowBg = windowBg
        self.controlBg = controlBg
        self.rowAltBg = rowAltBg
        self.controlFace = controlFace
        self.fieldBg = fieldBg
        self.hoverWash = hoverWash
        self.terminalBg = terminalBg
        self.terminalFg = terminalFg
        self.terminalCursor = terminalCursor
        self.terminalSelection = terminalSelection
        self.ansi = ansi
        self.label = label
        self.labelSecondary = labelSecondary
        self.labelTertiary = labelTertiary
        self.labelQuaternary = labelQuaternary
        self.accent = accent
        self.accentText = accentText
        self.hairlineShare = hairlineShare
        self.separatorShare = separatorShare
        self.borderShare = borderShare
        self.selectionShare = selectionShare
        self.bandShare = bandShare
        self.chipShare = chipShare
        self.avatarShare = avatarShare
        self.diffRowShare = diffRowShare
        self.diffGutterShare = diffGutterShare
        self.commentShare = commentShare
        self.headerVeilShare = headerVeilShare
        self.mutedShare = mutedShare
        self.skeletonShare = skeletonShare
        self.onAccentRuleShare = onAccentRuleShare
        self.systemOrange = systemOrange
        self.systemYellow = systemYellow
        self.systemGreen = systemGreen
        self.systemRed = systemRed
        self.systemBlue = systemBlue
        self.systemPink = systemPink
        self.systemTeal = systemTeal
        self.systemGray = systemGray
        self.isDark = isDark
    }

    /// The dark theme: Catppuccin Mocha, exactly the values the app drew
    /// with when it was dark-only.
    ///
    /// The surfaces run `fieldBg` < `windowBg` < `controlBg` < `rowAltBg` <
    /// `controlFace`: the well is Mocha's `mantle` and not black, and every
    /// level above it is one of Mocha's own surfaces bar `rowAltBg`, which
    /// is the step between `surface0` and `surface1` that the palette does
    /// not name.
    public static let mocha = Palette(
        windowBg: Surface("1e1e2e"),          // base
        controlBg: Surface("313244"),         // surface0
        // The one level this ladder needs and Catppuccin does not name,
        // derived from the two swatches either side of it rather than typed
        // as a hex — see `mix`. A literal here is the one thing in a theme
        // that cannot port: a new palette computes this from its own
        // surfaces, where a hex would have to be invented again.
        rowAltBg: Surface(mix("313244", "45475a", 0.5)),  // half surface0 into surface1
        controlFace: Surface("45475a"),       // surface1
        fieldBg: Surface("181825"),           // mantle
        // Mocha's `surface0`, the step Catppuccin's own ports hover with:
        // published, one clear level off `base`, and `text` (#cdd6f4)
        // clears 8.7:1 on it. `surface1` is a wider step and still
        // published, but its Latte twin takes that theme's label to
        // 4.39:1 — under the bar this token exists to hold. Two levels
        // below the `overlay0` a hover used to fill with, which is why
        // every hover in the app now darkens rather than lightening.
        hoverWash: Surface("313244"),         // surface0
        terminalBg: Surface("181825"),
        terminalFg: Ink("cdd6f4"),
        terminalCursor: Tint("cba6f7"),
        terminalSelection: Surface("45475a"),
        // Catppuccin's own published Mocha terminal mapping: surface1 for
        // black, subtext1 for white, surface2 and subtext0 for their bright
        // halves, and the accent hues unchanged between the two — Mocha's
        // accents are already the bright ones.
        ansi: [
            "45475a", "f38ba8", "a6e3a1", "f9e2af",
            "89b4fa", "f5c2e7", "94e2d5", "bac2de",
            "585b70", "f38ba8", "a6e3a1", "f9e2af",
            "89b4fa", "f5c2e7", "94e2d5", "a6adc8",
        ],
        label: Ink("cdd6f4"),             // text
        labelSecondary: Ink("a6adc8"),    // subtext0
        labelTertiary: Ink("9399b2"),     // overlay2
        labelQuaternary: Ink("6c7086"),   // overlay0
        accent: Tint("cba6f7"),            // mauve
        accentText: Ink("11111b"),        // crust
        hairlineShare: 0.10,
        separatorShare: 0.16,
        borderShare: 0.22,
        selectionShare: 0.20,
        bandShare: 0.50,
        chipShare: 0.18,
        avatarShare: 0.30,
        diffRowShare: 0.20,
        diffGutterShare: 0.32,
        commentShare: 0.10,
        headerVeilShare: 0.09,
        mutedShare: 0.45,
        skeletonShare: 0.10,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("fab387"),      // peach
        systemYellow: Tint("f9e2af"),      // yellow
        systemGreen: Tint("a6e3a1"),       // green
        systemRed: Tint("f38ba8"),         // red
        systemBlue: Tint("89b4fa"),        // blue
        systemPink: Tint("f5c2e7"),        // pink
        systemTeal: Tint("94e2d5"),        // teal
        systemGray: Tint("9399b2"),        // overlay2
        isDark: true
    )

    /// The light theme: Catppuccin Latte, mapped role for role onto the
    /// same names Mocha fills, and taken as published.
    ///
    /// Nothing here is adjusted to meet a contrast number. Latte is a theme
    /// thousands of people read code in every day and its authors chose
    /// these values deliberately; a hex "corrected" here would be a colour
    /// nobody else's Latte has, and would read as wrong beside every other
    /// Latte the user has open. The one value that is not published is
    /// `rowAltBg`, which is the level between `surface0` and `surface1` that
    /// this ladder needs and the palette does not name — the same
    /// interpolated step Mocha takes, for the same reason.
    ///
    /// Latte sinks and raises in the same direction: `mantle` and `crust`
    /// sit below `base` and so do the surfaces, which is simply what a light
    /// Catppuccin is. The five levels are therefore distinct rather than
    /// monotone, and that is the theme's own arrangement rather than
    /// something to iron out.
    public static let latte = Palette(
        windowBg: Surface("eff1f5"),          // base
        controlBg: Surface("ccd0da"),         // surface0
        rowAltBg: Surface(mix("ccd0da", "bcc0cc", 0.5)),  // half surface0 into surface1
        controlFace: Surface("bcc0cc"),       // surface1
        fieldBg: Surface("e6e9ef"),           // mantle
        // Latte's `surface0`, the same swatch Mocha hovers with, which in a
        // light Catppuccin sinks rather than rises — Latte's surfaces all
        // sit below its `base` — and so is a hover that deepens, as a light
        // theme's should. `text` (#4c4f69) clears 5.2:1 on it, where
        // `surface1` manages only 4.39:1.
        hoverWash: Surface("ccd0da"),         // surface0
        terminalBg: Surface("e6e9ef"),
        terminalFg: Ink("4c4f69"),
        terminalCursor: Tint("8839ef"),
        terminalSelection: Surface("bcc0cc"),
        // Catppuccin's own published Latte terminal mapping, exactly as its
        // ports write it: surface1 and surface2 for the two blacks, subtext1
        // and subtext0 for the two whites, and the accent hues unchanged
        // between the normal and bright halves.
        ansi: [
            "bcc0cc", "d20f39", "40a02b", "df8e1d",
            "1e66f5", "ea76cb", "179299", "5c5f77",
            "acb0be", "d20f39", "40a02b", "df8e1d",
            "1e66f5", "ea76cb", "179299", "6c6f85",
        ],
        label: Ink("4c4f69"),             // text
        labelSecondary: Ink("6c6f85"),    // subtext0
        labelTertiary: Ink("7c7f93"),     // overlay2
        labelQuaternary: Ink("9ca0b0"),   // overlay0
        accent: Tint("8839ef"),            // mauve
        accentText: Ink("dce0e8"),        // crust
        // The one place the two themes differ by more than their palettes:
        // dark ink on a light ground reads fainter than light ink on a dark
        // one at the same alpha, so the borders here are a couple of points
        // heavier and the selection wash a couple lighter — Latte's mauve is
        // a dark colour, and the same alpha would draw a far heavier slab.
        // These are the theme's own material rather than Catppuccin's, which
        // says nothing about how hard to press a hairline.
        hairlineShare: 0.12,
        separatorShare: 0.18,
        borderShare: 0.26,
        selectionShare: 0.16,
        bandShare: 0.50,
        // Latte's mauve, green and red are dark saturated colours, so the
        // same share of one over a light ground is a much heavier slab than
        // Mocha's pastels make over a dark one: every hue wash here is a few
        // points lighter than its Mocha twin, exactly as the selection wash
        // above already is.
        chipShare: 0.14,
        avatarShare: 0.24,
        diffRowShare: 0.16,
        diffGutterShare: 0.26,
        commentShare: 0.08,
        headerVeilShare: 0.07,
        mutedShare: 0.45,
        // And back the other way for the one wash of `label`, which is the
        // border ramp's rule: dark ink reads fainter than light ink at the
        // same alpha.
        skeletonShare: 0.12,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("fe640b"),      // peach
        systemYellow: Tint("df8e1d"),      // yellow
        systemGreen: Tint("40a02b"),       // green
        systemRed: Tint("d20f39"),         // red
        systemBlue: Tint("1e66f5"),        // blue
        systemPink: Tint("ea76cb"),        // pink
        systemTeal: Tint("179299"),        // teal
        systemGray: Tint("7c7f93"),        // overlay2
        isDark: false
    )
}

// MARK: - Grounds

/// A surface something is drawn *on*, named so a call site can say what it
/// is drawing over.
///
/// This is the fact an alpha was standing in for. A wash laid down at 16%
/// does not know what is behind it — it lets through whatever happens to be
/// there, which is why one `separator` token rendered as five different
/// colours depending on which pane it landed in, none of them chosen. Named
/// here, the ground is an input to the colour instead of an accident of the
/// view hierarchy, so what comes out is one opaque value the theme decided.
public enum Ground: String, CaseIterable, Sendable {
    case window, card, rowAlt, control, field, band, header, terminal, hover

    public func surface(in palette: Palette) -> Surface {
        switch self {
        case .window: palette.windowBg
        case .card: palette.controlBg
        case .rowAlt: palette.rowAltBg
        case .control: palette.controlFace
        case .field: palette.fieldBg
        case .band: palette.bandBg
        case .header: palette.headerBg
        case .terminal: palette.terminalBg
        case .hover: palette.hoverWash
        }
    }
}

/// How heavily a rule is drawn: the three weights of line the app separates
/// things with, in order.
public enum RuleWeight: CaseIterable, Sendable {
    /// The quiet edge between two surfaces.
    case hairline
    /// The line between two rows of one list.
    case separator
    /// The edge of something the pointer acts on, which has to read as an
    /// edge and not as a suggestion of one.
    case border
}

/// What a hue is being washed into a ground *for*. Each is a share of the
/// hue, and they are named by role because a chip and a diff gutter want
/// different weights of the same colour for reasons that have nothing to do
/// with each other.
public enum WashRole: CaseIterable, Sendable {
    case selection, chip, avatar, diffRow, diffGutter, comment, headerVeil, muted
}

extension Palette {
    /// A band laid over the window ground at half a card's weight: a pane's
    /// header, the brief's footer, a notice row. Derived rather than typed,
    /// like every other colour here that Catppuccin does not publish.
    public var bandBg: Surface {
        Surface(mix(controlBg.hex, windowBg.hex, bandShare))
    }

    /// The header band: the accent veiled over the window ground. It used to
    /// be two layers — the ground at 85% with the veil on top — which let
    /// the desktop through a window that paints no material behind it. One
    /// opaque colour says the same thing and means it.
    public var headerBg: Surface {
        Surface(mix(accent.hex, windowBg.hex, headerVeilShare))
    }

    /// A rule drawn on a named ground: the primary label mixed into it, at
    /// the weight the rule is for. Opaque, so the line is the same line
    /// wherever the view it belongs to is placed.
    public func rule(_ weight: RuleWeight, on ground: Ground) -> Ink {
        let share = switch weight {
        case .hairline: hairlineShare
        case .separator: separatorShare
        case .border: borderShare
        }
        return Ink(mix(label.hex, ground.surface(in: self).hex, share))
    }

    /// A hue washed into a named ground, as a ground: a chip's capsule, a
    /// diff row's fill, the wash behind a selected row.
    public func wash(_ role: WashRole, of tint: Tint, on ground: Ground) -> Surface {
        let share = switch role {
        case .selection: selectionShare
        case .chip: chipShare
        case .avatar: avatarShare
        case .diffRow: diffRowShare
        case .diffGutter: diffGutterShare
        case .comment: commentShare
        case .headerVeil: headerVeilShare
        case .muted: mutedShare
        }
        return tint.wash(on: ground.surface(in: self), share)
    }

    /// The sweep passing over a loading skeleton block — the one wash whose
    /// ground is not a surface of the app but the block itself.
    public func skeletonHighlight(on block: Surface) -> Surface {
        Surface(mix(label.hex, block.hex, skeletonShare))
    }

    /// The rule splitting a filled accent control in two. Its ground is the
    /// accent rather than any surface, which is why it is the accent's own
    /// ink rather than `label`.
    public var onAccentRule: Ink {
        Ink(mix(accentText.hex, accent.hex, onAccentRuleShare))
    }
}
