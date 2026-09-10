import SwiftUI

/// Design tokens for the nat UI theme.
///
/// The values are Catppuccin Mocha, the same documented palette the Go TUI
/// draws with (`internal/tui/styles.go`), so both faces of the product read
/// as one product and every colour here can be checked against a published
/// spec rather than against a screenshot. The mock at
/// docs/design/nat-ui-v2/nat-ui-v2.html is what the *layout* comes from; its
/// own colours sat within a few points of each other — the surfaces inside
/// nine levels of grey, the labels behind opacity — and read as flat and
/// dim on a real display, which is what these values replace.
///
/// Two rules the palette is chosen against and that any later edit has to
/// keep:
///
/// - The surface ladder is `fieldBg` < `windowBg` < `controlBg` < `rowAltBg`
///   < `controlFace`, and each step is a visible one. It stays in the
///   dark-grey range at both ends: the deepest surface is Mocha's `mantle`
///   and not black, because a well that reads as a hole is the thing being
///   fixed.
/// - Text is measured against the surface it lands on. `label` on `windowBg`
///   is 11.3:1, `labelSecondary` 7.4:1 and `labelTertiary` 5.8:1; on the
///   raised surfaces every one of them stays at or above where the old
///   values sat. Only `labelQuaternary` is below the WCAG body-text bar, and
///   it is decoration — a placeholder rule, an empty slot — never words to
///   read.
public enum DesignTokens {
    // MARK: - Background & Surface Colors

    /// The app's ground, and the fill of every pane that is not a card
    /// (Mocha `base`, #1e1e2e).
    public static let windowBg = Color(hex: "1e1e2e")

    /// The face of a card raised off the ground: rail cards, the diff file
    /// box, a tab's own band (Mocha `surface0`, #313244).
    public static let controlBg = Color(hex: "313244")

    /// The band that has to read apart from a card it sits inside: a file
    /// box's header row, a comment row, the diff gutter. One deliberate
    /// step between Mocha's `surface0` and `surface1`, which is the one
    /// level this ladder needs and the palette does not name (#3b3d4f).
    public static let rowAltBg = Color(hex: "3b3d4f")

    /// The face of a control the pointer acts on — a button, a picker
    /// (Mocha `surface1`, #45475a).
    public static let controlFace = Color(hex: "45475a")

    /// The well text is typed into, sunk below the ground rather than
    /// raised off it (Mocha `mantle`, #181825).
    public static let fieldBg = Color(hex: "181825")

    /// The agent terminal's own surface, as a hex string because SwiftTerm
    /// takes an `NSColor` rather than a SwiftUI `Color`. It sits at
    /// `fieldBg`'s level on purpose: a terminal is the same kind of thing as
    /// a text field — a well the app writes into — and the near-black it
    /// used to be (#121216) read as a hole cut in the window rather than as
    /// a panel of it.
    public static let terminalBgHex = "181825"

    /// `terminalBgHex` as a SwiftUI `Color`, for the surface laid full-bleed
    /// behind the terminal view.
    public static let terminalBg = Color(hex: terminalBgHex)

    /// The header band's own material: the ground at 85%, the flat stand-in
    /// for the mock's backdrop blur, which is a blur of what sits behind the
    /// window and not of the band's own paint.
    public static let headerBg = windowBg.opacity(0.85)

    // MARK: - Text Colors

    /// Primary label color (Mocha `text`, #cdd6f4) — 11.3:1 on `windowBg`.
    public static let label = Color(hex: "cdd6f4")

    /// Secondary label color (Mocha `subtext0`, #a6adc8) — 7.4:1 on
    /// `windowBg`. A real colour rather than the primary behind opacity, so
    /// it holds its contrast over whatever surface it lands on.
    public static let labelSecondary = Color(hex: "a6adc8")

    /// Tertiary label color (Mocha `overlay2`, #9399b2) — 5.8:1 on
    /// `windowBg`, which is what makes meta lines and timestamps readable
    /// rather than merely present.
    public static let labelTertiary = Color(hex: "9399b2")

    /// Quaternary label color (Mocha `overlay0`, #6c7086) — 3.4:1, below
    /// the body-text bar and deliberately so: this is the disabled glyph and
    /// the empty-slot rule, never words to read.
    public static let labelQuaternary = Color(hex: "6c7086")

    // MARK: - Accent Colors

    /// Primary accent color (Mocha `mauve`, #cba6f7) — 8.1:1 on `windowBg`.
    public static let accent = Color(hex: "cba6f7")

    /// Text color for content on accent background (Mocha `crust`,
    /// #11111b) — 9.2:1 on `accent`.
    public static let accentText = Color(hex: "11111b")

    /// The app icon's mark gradient, used sparingly: primary actions and active
    /// accents only. It is the brand's own and not the palette's, so it is
    /// the one thing here Mocha does not set.
    public static let brandGradient = LinearGradient(
        colors: [Color(hex: "6f4bf2"), Color(hex: "b558d8"), Color(hex: "ff70c2")],
        startPoint: .topLeading,
        endPoint: .bottomTrailing
    )

    // MARK: - Semantic UI Colors

    /// The quiet border that separates surfaces without drawing attention.
    public static let hairline = Color(hex: "cdd6f4").opacity(0.10)

    /// The soft fill behind a selected row, in place of a solid accent slab.
    public static let selectionWash = Color(hex: "cba6f7").opacity(0.20)

    /// Separator color: the line between two rows of one list.
    public static let separator = Color(hex: "cdd6f4").opacity(0.16)

    /// Control border color: the edge of something the pointer acts on,
    /// which has to read as an edge and not as a suggestion of one.
    public static let controlBorder = Color(hex: "cdd6f4").opacity(0.22)

    // MARK: - System Color Overrides

    /// Orange system color (Mocha `peach`, #fab387).
    public static let systemOrange = Color(hex: "fab387")

    /// Yellow system color (Mocha `yellow`, #f9e2af).
    public static let systemYellow = Color(hex: "f9e2af")

    /// Green system color (Mocha `green`, #a6e3a1).
    public static let systemGreen = Color(hex: "a6e3a1")

    /// Red system color (Mocha `red`, #f38ba8).
    public static let systemRed = Color(hex: "f38ba8")

    /// Blue system color (Mocha `blue`, #89b4fa).
    public static let systemBlue = Color(hex: "89b4fa")

    /// Pink system color (Mocha `pink`, #f5c2e7).
    public static let systemPink = Color(hex: "f5c2e7")

    /// Teal system color (Mocha `teal`, #94e2d5).
    public static let systemTeal = Color(hex: "94e2d5")

    /// Gray system color (Mocha `overlay2`, #9399b2).
    public static let systemGray = Color(hex: "9399b2")
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

/// Motion, per the design system's own rules (state changes at 0.15–0.35s
/// ease-out, nothing else) and Craig's read that anything slower drags: the
/// fast end, in one place, so turning animation off entirely is making
/// `stateChange` nil here and nowhere else.
public enum Motion {
    /// The one animation state changes (expand/collapse, selection) use.
    /// nil disables them app-wide.
    public static let stateChange: Animation? = .easeOut(duration: 0.15)
}

// MARK: - Hex Color Initializer

extension Color {
    /// Initialize a Color from a hex string (6 characters, e.g., "1e1e23").
    /// Invalid input (non-hex characters, wrong length) defaults to white.
    public init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        guard hex.count == 6 else {
            self = .white
            return
        }

        let scanner = Scanner(string: hex)
        var rgb: UInt64 = 0
        guard scanner.scanHexInt64(&rgb) else {
            self = .white
            return
        }

        let red = Double((rgb >> 16) & 0xFF) / 255.0
        let green = Double((rgb >> 8) & 0xFF) / 255.0
        let blue = Double(rgb & 0xFF) / 255.0

        self.init(red: red, green: green, blue: blue)
    }
}
