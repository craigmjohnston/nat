import SwiftUI

/// Design tokens for the nat UI theme, derived from the mock design at
/// docs/design/nat-ui-v2/nat-ui-v2.html.
public enum DesignTokens {
    // MARK: - Background & Surface Colors

    /// Main window background color (#1e1e23).
    public static let windowBg = Color(hex: "1e1e23")

    /// Control background color (#242429).
    public static let controlBg = Color(hex: "242429")

    /// Alternate row background color (#2b2b31).
    public static let rowAltBg = Color(hex: "2b2b31")

    /// Field/text input background color (#18181d).
    public static let fieldBg = Color(hex: "18181d")

    /// Control face color (#34343b).
    public static let controlFace = Color(hex: "34343b")

    // MARK: - Text Colors

    /// Primary label color (#e9e9ef).
    public static let label = Color(hex: "e9e9ef")

    /// Secondary label color, 66% opacity.
    public static let labelSecondary = Color(hex: "e9e9ef").opacity(0.66)

    /// Tertiary label color, 43% opacity.
    public static let labelTertiary = Color(hex: "e9e9ef").opacity(0.43)

    /// Quaternary label color, 14% opacity.
    public static let labelQuaternary = Color(hex: "e9e9ef").opacity(0.14)

    // MARK: - Accent Colors

    /// Primary accent color (#b1a8f2).
    public static let accent = Color(hex: "b1a8f2")

    /// Text color for content on accent background (#17171c).
    public static let accentText = Color(hex: "17171c")

    // MARK: - Semantic UI Colors

    /// Separator color (rgba(235,235,245,.13)).
    public static let separator = Color(hex: "ebebf5").opacity(0.13)

    /// Control border color (rgba(235,235,245,.16)).
    public static let controlBorder = Color(hex: "ebebf5").opacity(0.16)

    // MARK: - System Color Overrides

    /// Orange system color (#f2a87e).
    public static let systemOrange = Color(hex: "f2a87e")

    /// Yellow system color (#ecd495).
    public static let systemYellow = Color(hex: "ecd495")

    /// Green system color (#a8dea1).
    public static let systemGreen = Color(hex: "a8dea1")

    /// Red system color (#ec8c9e).
    public static let systemRed = Color(hex: "ec8c9e")

    /// Blue system color (#91b7f2).
    public static let systemBlue = Color(hex: "91b7f2")

    /// Pink system color (#efb5e0).
    public static let systemPink = Color(hex: "efb5e0")

    /// Teal system color (#95d8cc).
    public static let systemTeal = Color(hex: "95d8cc")

    /// Gray system color (#9a9aa5).
    public static let systemGray = Color(hex: "9a9aa5")
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
