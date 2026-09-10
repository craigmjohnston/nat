import Foundation

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
    /// the pane is part of the window rather than a second product embedded
    /// in it.
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
    /// opacity, so it is the same colour over whatever surface it lands on.
    public let labelSecondary: String
    /// Tertiary label colour: meta lines and timestamps.
    public let labelTertiary: String
    /// Quaternary label colour: the disabled glyph and the empty-slot rule,
    /// never words to read.
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

    /// Whether this palette paints light text on dark surfaces — which is
    /// the one thing about a theme that anything outside it needs to know.
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
    /// The surfaces run `fieldBg` < `windowBg` < `controlBg` < `rowAltBg` <
    /// `controlFace`: the well is Mocha's `mantle` and not black, and every
    /// level above it is one of Mocha's own surfaces bar `rowAltBg`, which
    /// is the step between `surface0` and `surface1` that the palette does
    /// not name.
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
        windowBg: "eff1f5",          // base
        controlBg: "ccd0da",         // surface0
        rowAltBg: "c4c8d4",          // between surface0 and surface1
        controlFace: "bcc0cc",       // surface1
        fieldBg: "e6e9ef",           // mantle
        terminalBg: "e6e9ef",
        terminalFg: "4c4f69",
        terminalCursor: "8839ef",
        terminalSelection: "bcc0cc",
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
        label: "4c4f69",             // text
        labelSecondary: "6c6f85",    // subtext0
        labelTertiary: "7c7f93",     // overlay2
        labelQuaternary: "9ca0b0",   // overlay0
        accent: "8839ef",            // mauve
        accentText: "dce0e8",        // crust
        headerOpacity: 0.85,
        // The one place the two themes differ by more than their palettes:
        // dark ink on a light ground reads fainter than light ink on a dark
        // one at the same alpha, so the borders here are a couple of points
        // heavier and the selection wash a couple lighter — Latte's mauve is
        // a dark colour, and the same alpha would draw a far heavier slab.
        // These are the theme's own material rather than Catppuccin's, which
        // says nothing about how hard to press a hairline.
        hairlineOpacity: 0.12,
        separatorOpacity: 0.18,
        controlBorderOpacity: 0.26,
        selectionWashOpacity: 0.16,
        systemOrange: "fe640b",      // peach
        systemYellow: "df8e1d",      // yellow
        systemGreen: "40a02b",       // green
        systemRed: "d20f39",         // red
        systemBlue: "1e66f5",        // blue
        systemPink: "ea76cb",        // pink
        systemTeal: "179299",        // teal
        systemGray: "7c7f93",        // overlay2
        isDark: false
    )
}
