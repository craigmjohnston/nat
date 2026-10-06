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
/// The same colour, lighter or darker: its lightness moved by `magnitude`
/// with its hue and saturation untouched, so a shaded green is still that
/// green. Catppuccin's own VS Code port derives with exactly this — its
/// `tab.hoverBackground` is `shade(base, 0.05)` — and it is what lets a
/// theme's hue be made readable without being made a different colour.
public func shade(_ color: String, _ magnitude: Double) -> String {
    guard let rgb = rgbComponents(hex: color) else { return color }
    var (hue, lightness, saturation) = hslComponents(rgb)
    lightness = min(max(lightness + magnitude, 0), 1)
    let shaded = rgbFromHSL(hue: hue, lightness: lightness, saturation: saturation)
    return [shaded.red, shaded.green, shaded.blue]
        .map { String(format: "%02x", Int((($0 * 255).rounded()))) }
        .joined()
}

/// WCAG's contrast ratio between two opaque colours — the one number that
/// says whether something can be read on something else, and the reason
/// `ink(of:on:)` below can answer per theme instead of per hand-tuned value.
public func contrastRatio(_ one: String, _ other: String) -> Double {
    let first = relativeLuminance(one)
    let second = relativeLuminance(other)
    return (max(first, second) + 0.05) / (min(first, second) + 0.05)
}

func relativeLuminance(_ hex: String) -> Double {
    guard let rgb = rgbComponents(hex: hex) else { return 1 }
    let channels = [rgb.red, rgb.green, rgb.blue].map { value -> Double in
        value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
    }
    return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

func hslComponents(_ rgb: (red: Double, green: Double, blue: Double)) -> (Double, Double, Double) {
    let high = max(rgb.red, rgb.green, rgb.blue)
    let low = min(rgb.red, rgb.green, rgb.blue)
    let lightness = (high + low) / 2
    guard high != low else { return (0, lightness, 0) }
    let delta = high - low
    let saturation = lightness > 0.5 ? delta / (2 - high - low) : delta / (high + low)
    var hue: Double
    switch high {
    case rgb.red: hue = (rgb.green - rgb.blue) / delta + (rgb.green < rgb.blue ? 6 : 0)
    case rgb.green: hue = (rgb.blue - rgb.red) / delta + 2
    default: hue = (rgb.red - rgb.green) / delta + 4
    }
    return (hue / 6, lightness, saturation)
}

func rgbFromHSL(hue: Double, lightness: Double, saturation: Double)
    -> (red: Double, green: Double, blue: Double) {
    guard saturation != 0 else { return (lightness, lightness, lightness) }
    let second = lightness < 0.5
        ? lightness * (1 + saturation)
        : lightness + saturation - lightness * saturation
    let first = 2 * lightness - second
    func channel(_ offset: Double) -> Double {
        var t = hue + offset
        if t < 0 { t += 1 }
        if t > 1 { t -= 1 }
        if t < 1.0 / 6 { return first + (second - first) * 6 * t }
        if t < 1.0 / 2 { return second }
        if t < 2.0 / 3 { return first + (second - first) * (2.0 / 3 - t) * 6 }
        return first
    }
    return (channel(1.0 / 3), channel(0), channel(-1.0 / 3))
}

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
/// Every palette has the gnat hi-fi design's structure: two surfaces
/// (`--bg` and `--chrome`), a four-step ink ramp, two rules, one accent and
/// one "hot" colour for whatever needs the user. `light` is the
/// design's own `.win.light` block (whose values were violet; the app is
/// navy). The others are published community themes laid onto the same
/// roles — `oneLight`, `tokyoDay`, `iceberg` and `slateInk` — and which of them a window draws
/// with is the user's choice, `PaletteChoice`.
public struct Palette: Equatable, Sendable {
    // MARK: - Surfaces

    /// The app's ground, and the fill of every pane that is not a card.
    public let windowBg: Surface
    /// The design's `--chrome`: the sidebar, the navigator, every titlebar
    /// and the status bar — the second of its two surfaces.
    public let chromeBg: Surface
    /// The three titlebars' own ground — darker than the chrome in the dark
    /// theme, the chrome itself in the light one.
    public let titlebarBg: Surface
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
    /// The design's `--hot`: what needs the user — a waiting agent, a branch
    /// to review, an open pull request.
    public let hot: Tint

    // MARK: - Opacities

    /// The accent at this opacity is `--sel`: the fill under the pointer on a
    /// row — an indigo wash rather than the design's grey, which went muddy
    /// on the near-black frame.
    public let rowHoverShare: Double
    /// The accent at this opacity is `--sel-2`: the fill behind the selected
    /// row.
    public let rowSelectedShare: Double
    /// What the row washes are mixed from: the ink in the dark theme, so a
    /// highlight is a neutral lift and the navy stays the primary colour
    /// alone; the accent in the light one.
    public let rowWashTint: Tint

    /// The design's `--line`: the quiet border between two surfaces and the
    /// line between two rows of one list — one navy colour on
    /// either surface, as the design draws it, rather than a grey mix.
    public let line: Ink
    /// The design's `--line-2`: the edge of something the pointer acts on.
    public let line2: Ink
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

    /// The project colours this palette has no outcome hue for, or whose
    /// outcome hue would draw two of them alike: a hue of its own for each.
    /// Every other project colour is its namesake outcome hue (`projectTint`).
    public let projectHues: [ProjectColor: Tint]

    public init(
        windowBg: Surface,
        chromeBg: Surface,
        titlebarBg: Surface,
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
        hot: Tint,
        rowHoverShare: Double,
        rowSelectedShare: Double,
        rowWashTint: Tint,
        line: Ink,
        line2: Ink,
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
        isDark: Bool,
        projectHues: [ProjectColor: Tint] = [:]
    ) {
        self.windowBg = windowBg
        self.chromeBg = chromeBg
        self.titlebarBg = titlebarBg
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
        self.hot = hot
        self.rowHoverShare = rowHoverShare
        self.rowSelectedShare = rowSelectedShare
        self.rowWashTint = rowWashTint
        self.line = line
        self.line2 = line2
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
        self.projectHues = projectHues
    }

    /// Iceberg (cocopon/iceberg.vim): a cold, desaturated indigo with grey
    /// ink. Its ground is lifted one step off the published `#161821`, which
    /// becomes the chrome, so the window reads as dark rather than black.
    ///
    /// Rules, hover and selection are taken from the theme's own ramp; the
    /// row washes are the accent, as the light theme's are.
    public static let iceberg = Palette(
        windowBg: Surface("1b1d28"),
        chromeBg: Surface("161821"),
        titlebarBg: Surface("161821"),
        controlBg: Surface("1e2132"),
        rowAltBg: Surface(mix("262a3c", "1e2132", 0.5)),  // half the line into the card
        controlFace: Surface("2a3158"),
        fieldBg: Surface("1b1d28"),
        hoverWash: Surface(mix("84a0c6", "1b1d28", 0.08)),
        terminalBg: Surface("1b1d28"),
        terminalFg: Ink("c6c8d1"),
        terminalCursor: Tint("84a0c6"),
        terminalSelection: Surface(mix("84a0c6", "1b1d28", 0.16)),
        // Iceberg's own published terminal mapping.
        ansi: [
            "1e2132", "e27878", "b4be82", "e2a478",
            "84a0c6", "a093c7", "89b8c2", "c6c8d1",
            "6b7089", "e27878", "b4be82", "e2a478",
            "84a0c6", "a093c7", "89b8c2", "d2d4de",
        ],
        label: Ink("c6c8d1"),
        labelSecondary: Ink("a3a6b7"),
        labelTertiary: Ink("6b7089"),
        labelQuaternary: Ink("3e445e"),
        accent: Tint("84a0c6"),
        accentText: Ink("161821"),
        hot: Tint("e2a478"),
        rowHoverShare: 0.08,
        rowSelectedShare: 0.16,
        rowWashTint: Tint("84a0c6"),
        line: Ink("262a3c"),
        line2: Ink("33374c"),
        selectionShare: 0.16,
        bandShare: 0.50,
        chipShare: 0.24,
        avatarShare: 0.30,
        diffRowShare: 0.12,
        diffGutterShare: 0.22,
        commentShare: 0.10,
        headerVeilShare: 0.0,
        mutedShare: 0.45,
        skeletonShare: 0.10,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("e2a478"),
        systemYellow: Tint("e2a478"),
        systemGreen: Tint("b4be82"),
        systemRed: Tint("e27878"),
        systemBlue: Tint("84a0c6"),
        systemPink: Tint("a093c7"),
        systemTeal: Tint("89b8c2"),
        systemGray: Tint("a3a6b7"),
        isDark: true,
        // Its yellow is its orange, and its pink a violet (purple's): a
        // yellow and a pink of its own; the rest are its outcome hues.
        projectHues: [.yellow: Tint("d8c27e"), .purple: Tint("a093c7"), .pink: Tint("d495b8")]
    )

    /// Slate ink: a blue-grey slate ground under Kanagawa's (rebelot/
    /// kanagawa.nvim) cream ink, crystal-blue accent and hues — the warm ink
    /// takes the edge off the blue.
    public static let slateInk = Palette(
        windowBg: Surface("252838"),
        chromeBg: Surface("1f2231"),
        titlebarBg: Surface("1f2231"),
        controlBg: Surface("2b2e40"),
        rowAltBg: Surface(mix("313549", "2b2e40", 0.5)),  // half the line into the card
        controlFace: Surface("363a4f"),
        fieldBg: Surface("252838"),
        hoverWash: Surface(mix("9aaaf0", "252838", 0.07)),
        terminalBg: Surface("252838"),
        terminalFg: Ink("dcd7ba"),
        terminalCursor: Tint("7e9cd8"),
        terminalSelection: Surface(mix("9aaaf0", "252838", 0.14)),
        // Kanagawa's own published terminal mapping, its black lifted to
        // read on the slate.
        ansi: [
            "2a2e42", "c34043", "76946a", "c0a36e",
            "7e9cd8", "957fb8", "6a9589", "c8c093",
            "727169", "c34043", "76946a", "c0a36e",
            "7e9cd8", "957fb8", "6a9589", "dcd7ba",
        ],
        label: Ink("dcd7ba"),
        labelSecondary: Ink("a8a594"),
        labelTertiary: Ink("7a7a8a"),
        labelQuaternary: Ink("444862"),
        accent: Tint("7e9cd8"),
        accentText: Ink("1f2231"),
        hot: Tint("ffa066"),
        rowHoverShare: 0.07,
        rowSelectedShare: 0.14,
        rowWashTint: Tint("9aaaf0"),
        line: Ink("313549"),
        line2: Ink("3c4057"),
        selectionShare: 0.16,
        bandShare: 0.50,
        chipShare: 0.24,
        avatarShare: 0.30,
        diffRowShare: 0.12,
        diffGutterShare: 0.22,
        commentShare: 0.10,
        headerVeilShare: 0.0,
        mutedShare: 0.45,
        skeletonShare: 0.10,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("ffa066"),
        systemYellow: Tint("e6c384"),
        systemGreen: Tint("98bb6c"),
        systemRed: Tint("e46876"),
        systemBlue: Tint("7fb4ca"),
        systemPink: Tint("957fb8"),
        systemTeal: Tint("7aa89f"),
        systemGray: Tint("a8a594"),
        isDark: true,
        // Its pink is a violet, purple's; the rest are its outcome hues.
        projectHues: [.purple: Tint("957fb8"), .pink: Tint("d27e99")]
    )

    /// The light theme: the design's `.win.light` block, role for role onto
    /// the names the dark one fills.
    public static let light = Palette(
        windowBg: Surface("faf9f7"),          // --bg
        chromeBg: Surface("f3f2ef"),          // --chrome
        titlebarBg: Surface("f3f2ef"),        // --chrome
        controlBg: Surface("f3f2ef"),         // --chrome
        rowAltBg: Surface(mix("d9d9e2", "f3f2ef", 0.5)),  // half --line into --chrome
        controlFace: Surface("d9d9e2"),       // --line
        fieldBg: Surface("faf9f7"),           // --bg
        hoverWash: Surface(mix("1f44a3", "faf9f7", 0.06)),  // --sel: the accent, not grey
        terminalBg: Surface("faf9f7"),        // --bg
        terminalFg: Ink("151632"),            // --ink
        terminalCursor: Tint("1f44a3"),       // --accent
        terminalSelection: Surface(mix("1f44a3", "faf9f7", 0.12)),  // --sel-2
        // Catppuccin Latte's own published terminal mapping, as before.
        ansi: [
            "bcc0cc", "d20f39", "40a02b", "df8e1d",
            "1e66f5", "ea76cb", "179299", "5c5f77",
            "acb0be", "d20f39", "40a02b", "df8e1d",
            "1e66f5", "ea76cb", "179299", "6c6f85",
        ],
        label: Ink("151632"),             // --ink
        labelSecondary: Ink("55566d"),    // --ink-2
        labelTertiary: Ink("9090a2"),     // --ink-3
        labelQuaternary: Ink("d1d1db"),   // --ink-4
        accent: Tint("1f44a3"),            // --accent: the brand, the icon's ink at its midpoint
        accentText: Ink("faf9f7"),        // --bg
        hot: Tint("d75f09"),               // --hot
        rowHoverShare: 0.06,               // --sel
        rowSelectedShare: 0.12,            // --sel-2
        rowWashTint: Tint("1f44a3"),       // --accent
        line: Ink("d9d9e2"),               // --line
        line2: Ink("c9c9d4"),              // --line-2
        selectionShare: 0.12,              // --accent-dim
        bandShare: 0.50,
        chipShare: 0.12,                   // --accent-dim / --hot-dim
        avatarShare: 0.24,
        diffRowShare: 0.10,                // --add-bg / --del-bg
        diffGutterShare: 0.18,
        commentShare: 0.08,
        headerVeilShare: 0.0,
        mutedShare: 0.45,
        skeletonShare: 0.12,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("d75f09"),      // --hot
        systemYellow: Tint("df8e1d"),
        systemGreen: Tint("2e7d3a"),       // --add
        systemRed: Tint("b23a48"),         // --del
        systemBlue: Tint("1e66f5"),
        systemPink: Tint("ea76cb"),
        systemTeal: Tint("179299"),
        systemGray: Tint("55566d"),        // --ink-2
        isDark: false,
        // Its pink is a pink, so purple is its mauve; the rest are its outcome hues.
        projectHues: [.purple: Tint("8839ef")]
    )
    /// One Light (Atom): a neutral off-white with soft charcoal ink and a
    /// clear blue accent. It names no orange of its own, so `hot` is its
    /// yellow taken toward orange; its accent (`#4078f2`) is shaded a step
    /// darker so white reads on a filled button.
    public static let oneLight = Palette(
        windowBg: Surface("fafafa"),
        chromeBg: Surface("f0f0f1"),
        titlebarBg: Surface("f0f0f1"),
        controlBg: Surface("f0f0f1"),
        rowAltBg: Surface(mix("e0e0e2", "f0f0f1", 0.5)),  // half the line into the card
        controlFace: Surface("e5e5e6"),
        fieldBg: Surface("fafafa"),
        hoverWash: Surface(mix("2f6cf1", "fafafa", 0.06)),
        terminalBg: Surface("fafafa"),
        terminalFg: Ink("383a42"),
        terminalCursor: Tint("2f6cf1"),
        terminalSelection: Surface(mix("2f6cf1", "fafafa", 0.12)),
        // One Light's own published terminal mapping.
        ansi: [
            "383a42", "e45649", "50a14f", "c18401",
            "4078f2", "a626a4", "0184bc", "a0a1a7",
            "4f525e", "e45649", "50a14f", "c18401",
            "4078f2", "a626a4", "0184bc", "fafafa",
        ],
        label: Ink("383a42"),
        labelSecondary: Ink("696c77"),
        labelTertiary: Ink("a0a1a7"),
        labelQuaternary: Ink("d4d4d6"),
        accent: Tint("2f6cf1"),
        accentText: Ink("ffffff"),
        hot: Tint("d0721f"),
        rowHoverShare: 0.06,
        rowSelectedShare: 0.12,
        rowWashTint: Tint("2f6cf1"),
        line: Ink("e0e0e2"),
        line2: Ink("d4d4d7"),
        selectionShare: 0.12,
        bandShare: 0.50,
        chipShare: 0.12,
        avatarShare: 0.24,
        diffRowShare: 0.10,
        diffGutterShare: 0.18,
        commentShare: 0.08,
        headerVeilShare: 0.0,
        mutedShare: 0.45,
        skeletonShare: 0.12,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("d0721f"),
        systemYellow: Tint("c18401"),
        systemGreen: Tint("50a14f"),
        systemRed: Tint("e45649"),
        systemBlue: Tint("4078f2"),
        systemPink: Tint("a626a4"),
        systemTeal: Tint("0184bc"),
        systemGray: Tint("696c77"),
        isDark: false,
        // Its pink is a violet, purple's; the rest are its outcome hues.
        projectHues: [.purple: Tint("a626a4"), .pink: Tint("d74a96")]
    )

    /// Tokyo Night Day (folke/tokyonight.nvim): lavender-grey paper and
    /// blue ink — the light twin of the Tokyo Night family. Its ink
    /// (`#3760bf`) and accent (`#2e7de9`) are each shaded a step darker: the
    /// published ink sits too close to its own secondary tier once that is
    /// made readable, and white on the published accent is under 4.5:1.
    public static let tokyoDay = Palette(
        windowBg: Surface("e1e2e7"),
        chromeBg: Surface("d0d5e3"),
        titlebarBg: Surface("d0d5e3"),
        controlBg: Surface("d0d5e3"),
        rowAltBg: Surface(mix("c4c8da", "d0d5e3", 0.5)),  // half the line into the card
        controlFace: Surface("c4c8da"),
        fieldBg: Surface("e1e2e7"),
        hoverWash: Surface(mix("1c72e7", "e1e2e7", 0.06)),
        terminalBg: Surface("e1e2e7"),
        terminalFg: Ink("2f52a3"),
        terminalCursor: Tint("1c72e7"),
        terminalSelection: Surface(mix("1c72e7", "e1e2e7", 0.12)),
        // Tokyo Night Day's own published terminal mapping.
        ansi: [
            "e9e9ed", "f52a65", "587539", "8c6c3e",
            "2e7de9", "9854f1", "007197", "6172b0",
            "a1a6c5", "f52a65", "587539", "8c6c3e",
            "2e7de9", "9854f1", "007197", "3760bf",
        ],
        label: Ink("2f52a3"),
        labelSecondary: Ink("6172b0"),
        labelTertiary: Ink("848cb5"),
        labelQuaternary: Ink("b4b9cf"),
        accent: Tint("1c72e7"),
        accentText: Ink("ffffff"),
        hot: Tint("b15c00"),
        rowHoverShare: 0.06,
        rowSelectedShare: 0.12,
        rowWashTint: Tint("1c72e7"),
        line: Ink("c4c8da"),
        line2: Ink("b6bcd2"),
        selectionShare: 0.12,
        bandShare: 0.50,
        chipShare: 0.12,
        avatarShare: 0.24,
        diffRowShare: 0.10,
        diffGutterShare: 0.18,
        commentShare: 0.08,
        headerVeilShare: 0.0,
        mutedShare: 0.45,
        skeletonShare: 0.12,
        onAccentRuleShare: 0.25,
        systemOrange: Tint("b15c00"),
        systemYellow: Tint("8c6c3e"),
        systemGreen: Tint("587539"),
        systemRed: Tint("f52a65"),
        systemBlue: Tint("2e7de9"),
        systemPink: Tint("9854f1"),
        systemTeal: Tint("007197"),
        systemGray: Tint("6172b0"),
        isDark: false,
        // Its pink is a violet, purple's; the rest are its outcome hues.
        projectHues: [.purple: Tint("9854f1"), .pink: Tint("c94f9e")]
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
    case window, chrome, card, rowAlt, control, field, band, header, terminal, hover

    public func surface(in palette: Palette) -> Surface {
        switch self {
        case .window: palette.windowBg
        case .chrome: palette.chromeBg
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

    /// The header band: every titlebar.
    public var headerBg: Surface {
        titlebarBg
    }

    /// A row's wash on a named ground: `--sel` under the pointer, `--sel-2`
    /// behind the selected row. `rowWashTint` mixed into the ground, so it is
    /// the same opaque step whichever surface the row sits on.
    public func rowWash(selected: Bool, on ground: Ground) -> Surface {
        Surface(mix(rowWashTint.hex, ground.surface(in: self).hex, selected ? rowSelectedShare : rowHoverShare))
    }

    /// A rule at a weight. The design draws its two line colours on every
    /// surface alike, so the ground is taken for the call sites' sake and
    /// does not change the line.
    public func rule(_ weight: RuleWeight, on ground: Ground) -> Ink {
        _ = ground
        switch weight {
        case .hairline, .separator: return line
        case .border: return line2
        }
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

    /// A project colour in this palette: its own hue where `projectHues`
    /// gives one, else the outcome hue of that name.
    public func projectTint(_ color: ProjectColor) -> Tint {
        if let hue = projectHues[color] { return hue }
        return switch color {
        case .red: systemRed
        case .orange: systemOrange
        case .yellow: systemYellow
        case .green: systemGreen
        case .teal: systemTeal
        case .blue: systemBlue
        case .purple, .pink: systemPink
        }
    }

    /// The sweep passing over a loading skeleton block — the one wash whose
    /// ground is not a surface of the app but the block itself.
    public func skeletonHighlight(on block: Surface) -> Surface {
        Surface(mix(label.hex, block.hex, skeletonShare))
    }

    /// What a hue reads as when it is *written* on a ground: the theme's own
    /// colour, shaded only as far as it must be to be readable there.
    ///
    /// This is the one rule that makes an outcome colour survive a light
    /// theme. Mocha's accents are pastels on a dark ground and already clear
    /// the bar, so the shade is nil or imperceptible and nothing about that
    /// theme changes. Latte's are dark saturated colours whose *lightness*
    /// sits in the middle, which is the worst place to be: its yellow as text
    /// on a card is 1.70:1 and its pink 1.71 — not dim, invisible. Nothing in
    /// Catppuccin fixes that, because the palette publishes no darker
    /// variants and both of the inks one might reach for are worse: light on
    /// a mid hue is 1.98–2.96, and the theme's own `text` is 2.39–3.05.
    ///
    /// Mixing toward `text` was the obvious answer and is the wrong one: in
    /// Latte it converges all the way, so a green chip and a red chip both
    /// come out `#4c4f69` and the colour coding — the entire point of an
    /// outcome colour — is gone. `shade` moves lightness alone, so a shaded
    /// green is still that green: `#40a02b` becomes `#2d701e` and reads as
    /// green at 4.69:1.
    ///
    /// It answers per theme rather than per hand-tuned constant, which is the
    /// whole reason a third palette can be added by filling in its swatches:
    /// a theme whose hues already read is left alone, and one whose hues do
    /// not is corrected by its own colours without anyone choosing a number.
    public func ink(of tint: Tint, on ground: Surface, clearing bar: Double = 4.5) -> Ink {
        // Away from the ground: darken a hue on a light one, lighten it on a
        // dark one. Which of those a theme needs is the theme's business and
        // not something written down per palette.
        let step = relativeLuminance(ground.hex) > relativeLuminance(tint.hex) ? -0.02 : 0.02
        var magnitude = 0.0
        var candidate = tint.hex
        while contrastRatio(candidate, ground.hex) < bar && abs(magnitude) < 0.6 {
            magnitude += step
            candidate = shade(tint.hex, magnitude)
        }
        return Ink(candidate)
    }

    /// An ink shaded only as far as it must be to be read on a ground —
    /// `ink(of:on:)` for something that is already ink rather than a hue.
    ///
    /// It is the same operation and exists for the same reason: Latte's
    /// `subtext0` clears AA on none of its own surfaces, 4.37 at best on
    /// `base` and 3.20 on a card, so a theme cannot be taken as published and
    /// also be readable everywhere this app draws.
    public func readable(_ ink: Ink, on ground: Surface, clearing bar: Double = 4.5) -> Ink {
        let step = relativeLuminance(ground.hex) > relativeLuminance(ink.hex) ? -0.02 : 0.02
        var magnitude = 0.0
        var candidate = ink.hex
        while contrastRatio(candidate, ground.hex) < bar && abs(magnitude) < 0.6 {
            magnitude += step
            candidate = shade(ink.hex, magnitude)
        }
        return Ink(candidate)
    }

    /// The word inside a chip, whose ground is the chip's own capsule rather
    /// than the surface behind it — a chip is one hue drawn twice, and the
    /// word has to survive the wash it sits on.
    public func chipInk(of tint: Tint, on ground: Ground) -> Ink {
        ink(of: tint, on: wash(.chip, of: tint, on: ground))
    }

    /// The rule splitting a filled accent control in two. Its ground is the
    /// accent rather than any surface, which is why it is the accent's own
    /// ink rather than `label`.
    public var onAccentRule: Ink {
        Ink(mix(accentText.hex, accent.hex, onAccentRuleShare))
    }
}
