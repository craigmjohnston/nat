import Foundation

/// One theme's raw values: every colour the app draws with, as the hex
/// string it is defined by, plus the few opacities that are the theme's own
/// rather than a colour's.
///
/// It is deliberately a plain value type of strings and numbers and not a
/// bag of `Color`s: a `Color` cannot be compared, measured for contrast or
/// asserted about, and the two palettes here are exactly the thing that has
/// to be checked against a published spec. `DesignTokens` is what turns one
/// into the dynamic colours SwiftUI draws — see `DesignTokens.palette(for:)`.
///
/// The two palettes are Catppuccin Mocha and Catppuccin Latte, the same
/// family the Go TUI draws with, so both faces of the product read as one
/// product. Where Latte's own published accent misses WCAG AA on Latte's own
/// base — which most of them do, Catppuccin having chosen them for hue rather
/// than for contrast — the value here is that accent scaled down until it
/// clears 4.5:1, which lands them within a few points of GitHub's own light
/// theme. The rule is written down rather than eyeballed so a later edit can
/// reproduce it.
public struct Palette: Equatable, Sendable {
    // MARK: - Surfaces

    /// The app's ground, and the fill of every pane that is not a card.
    public let windowBg: String
    /// The face of a card raised off the ground.
    public let controlBg: String
    /// The band that has to read apart from a card it sits inside.
    public let rowAltBg: String
    /// The face of a control the pointer acts on.
    public let controlFace: String
    /// The well text is typed into.
    public let fieldBg: String

    // MARK: - Terminal

    /// The agent terminal's own surface. It sits at `fieldBg`'s level in
    /// both themes on purpose: a terminal is the same kind of thing as a
    /// text field — a well the app writes into.
    public let terminalBg: String
    /// The terminal's default foreground, which is `label` in both themes:
    /// what an agent writes is words to read.
    public let terminalFg: String
    /// The terminal's caret.
    public let terminalCursor: String
    /// The wash behind selected terminal text.
    public let terminalSelection: String
    /// The sixteen ANSI colours, in the order a terminal numbers them:
    /// black, red, green, yellow, blue, magenta, cyan, white, then the same
    /// eight bright.
    public let ansi: [String]

    // MARK: - Text

    /// Primary label colour.
    public let label: String
    /// Secondary label colour — a real colour rather than the primary behind
    /// opacity, so it holds its contrast over whatever surface it lands on.
    public let labelSecondary: String
    /// Tertiary label colour: meta lines and timestamps.
    public let labelTertiary: String
    /// Quaternary label colour, deliberately below the body-text bar: the
    /// disabled glyph and the empty-slot rule, never words to read.
    public let labelQuaternary: String

    // MARK: - Accent

    /// Primary accent colour.
    public let accent: String
    /// What is written on top of the accent.
    public let accentText: String

    // MARK: - Opacities

    /// The ground's own opacity in the header band, the flat stand-in for
    /// the mock's backdrop blur.
    public let headerOpacity: Double
    /// `label` at this opacity is the quiet border between two surfaces.
    public let hairlineOpacity: Double
    /// `label` at this opacity is the line between two rows of one list.
    public let separatorOpacity: Double
    /// `label` at this opacity is the edge of something the pointer acts on.
    public let controlBorderOpacity: Double
    /// `accent` at this opacity is the fill behind a selected row.
    public let selectionWashOpacity: Double

    // MARK: - Outcome colours

    public let systemOrange: String
    public let systemYellow: String
    public let systemGreen: String
    public let systemRed: String
    public let systemBlue: String
    public let systemPink: String
    public let systemTeal: String
    public let systemGray: String

    /// Whether this palette paints light text on dark surfaces. It is what
    /// the surface-ladder rule reads its direction from: the ladder climbs
    /// away from the ground in luminance under Mocha and descends under
    /// Latte, which is the same step in both.
    public let isDark: Bool

    public init(
        windowBg: String,
        controlBg: String,
        rowAltBg: String,
        controlFace: String,
        fieldBg: String,
        terminalBg: String,
        terminalFg: String,
        terminalCursor: String,
        terminalSelection: String,
        ansi: [String],
        label: String,
        labelSecondary: String,
        labelTertiary: String,
        labelQuaternary: String,
        accent: String,
        accentText: String,
        headerOpacity: Double,
        hairlineOpacity: Double,
        separatorOpacity: Double,
        controlBorderOpacity: Double,
        selectionWashOpacity: Double,
        systemOrange: String,
        systemYellow: String,
        systemGreen: String,
        systemRed: String,
        systemBlue: String,
        systemPink: String,
        systemTeal: String,
        systemGray: String,
        isDark: Bool
    ) {
        self.windowBg = windowBg
        self.controlBg = controlBg
        self.rowAltBg = rowAltBg
        self.controlFace = controlFace
        self.fieldBg = fieldBg
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
        self.headerOpacity = headerOpacity
        self.hairlineOpacity = hairlineOpacity
        self.separatorOpacity = separatorOpacity
        self.controlBorderOpacity = controlBorderOpacity
        self.selectionWashOpacity = selectionWashOpacity
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
    /// The surface ladder is `fieldBg` < `windowBg` < `controlBg` <
    /// `rowAltBg` < `controlFace`, each step a visible one, and it stays in
    /// the dark-grey range at both ends: the deepest surface is Mocha's
    /// `mantle` and not black, because a well that reads as a hole is the
    /// thing this palette was chosen to fix.
    public static let mocha = Palette(
        windowBg: "1e1e2e",          // base
        controlBg: "313244",         // surface0
        // One deliberate step between Mocha's `surface0` and `surface1`,
        // which is the one level this ladder needs and the palette does not
        // name.
        rowAltBg: "3b3d4f",
        controlFace: "45475a",       // surface1
        fieldBg: "181825",           // mantle
        terminalBg: "181825",
        terminalFg: "cdd6f4",
        terminalCursor: "cba6f7",
        terminalSelection: "45475a",
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
        label: "cdd6f4",             // text
        labelSecondary: "a6adc8",    // subtext0
        labelTertiary: "9399b2",     // overlay2
        labelQuaternary: "6c7086",   // overlay0
        accent: "cba6f7",            // mauve
        accentText: "11111b",        // crust
        headerOpacity: 0.85,
        hairlineOpacity: 0.10,
        separatorOpacity: 0.16,
        controlBorderOpacity: 0.22,
        selectionWashOpacity: 0.20,
        systemOrange: "fab387",      // peach
        systemYellow: "f9e2af",      // yellow
        systemGreen: "a6e3a1",       // green
        systemRed: "f38ba8",         // red
        systemBlue: "89b4fa",        // blue
        systemPink: "f5c2e7",        // pink
        systemTeal: "94e2d5",        // teal
        systemGray: "9399b2",        // overlay2
        isDark: true
    )

    /// The light theme: Catppuccin Latte, with the same roles as Mocha and
    /// the same rules applied in the other direction.
    ///
    /// The ladder descends rather than climbs — `fieldBg` > `windowBg` >
    /// `controlBg` > `rowAltBg` > `controlFace` — because a well in a light
    /// UI is lighter than its ground and a raised surface is darker, which
    /// is the opposite of what raising and sinking mean on a dark ground.
    /// The well is white rather than one of Latte's own greys for that
    /// reason: Latte's `mantle` and `crust` sit *below* its base and so
    /// belong on the raised side of this ladder, not the sunk one.
    public static let latte = Palette(
        windowBg: "eff1f5",          // base
        controlBg: "e6e9ef",         // mantle
        rowAltBg: "dce0e8",          // crust
        controlFace: "ccd0da",       // surface0
        fieldBg: "ffffff",
        terminalBg: "ffffff",
        terminalFg: "4c4f69",
        terminalCursor: "8839ef",
        terminalSelection: "ccd0da",
        // Latte's own terminal mapping for the greys (surface1/surface2 for
        // black, subtext1/subtext0 for white) and the darkened accents below
        // for the hues, since a terminal is words to read on a white well
        // and Latte's published accents do not clear AA there. The bright
        // half repeats the normal one hue for hue, exactly as Mocha's does:
        // a brighter version of a colour chosen for contrast against a light
        // ground is a less readable one, and an agent's output is the last
        // place to spend contrast on a distinction nobody reads.
        ansi: [
            "bcc0cc", "d20f39", "317c21", "976013",
            "1d62ed", "9f508a", "12777d", "5c5f77",
            "acb0be", "d20f39", "317c21", "976013",
            "1d62ed", "9f508a", "12777d", "6c6f85",
        ],
        label: "4c4f69",             // text
        labelSecondary: "5c5f77",    // subtext1
        // A step below Latte's `subtext0`, which lands at 4.4:1 on the
        // ground and so misses the bar meta lines are held to by a hair.
        labelTertiary: "63667d",
        labelQuaternary: "8c8fa1",   // overlay1
        accent: "8839ef",            // mauve
        accentText: "ffffff",
        headerOpacity: 0.85,
        // Dark ink on a light ground reads fainter than light ink on a dark
        // one at the same alpha, so every border here is a couple of points
        // heavier than Mocha's.
        hairlineOpacity: 0.12,
        separatorOpacity: 0.18,
        controlBorderOpacity: 0.26,
        // And the selection wash a couple lighter: Latte's mauve is a dark
        // colour, so the same alpha would draw a far heavier slab.
        selectionWashOpacity: 0.16,
        systemOrange: "b94908",      // peach, darkened to AA
        systemYellow: "976013",      // yellow, darkened to AA
        systemGreen: "317c21",       // green, darkened to AA
        systemRed: "d20f39",         // red
        systemBlue: "1d62ed",        // blue, darkened to AA
        systemPink: "9f508a",        // pink, darkened to AA
        systemTeal: "12777d",        // teal, darkened to AA
        systemGray: "696b7c",        // overlay2, darkened to AA
        isDark: false
    )
}
