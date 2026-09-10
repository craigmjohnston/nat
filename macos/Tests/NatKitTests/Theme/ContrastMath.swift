import Foundation
@testable import NatKit

/// WCAG 2.1's own two numbers, over the hex strings a `Palette` is written
/// in — which is what the palettes are asserted about, since a `Color` can
/// neither be compared nor measured.
enum ContrastMath {
    /// Relative luminance of an opaque sRGB colour.
    static func luminance(_ hex: String) -> Double {
        let rgb = rgbComponents(hex: hex) ?? (1, 1, 1)
        let channels = [rgb.red, rgb.green, rgb.blue].map { value -> Double in
            value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    }

    /// Contrast ratio between two opaque colours.
    static func ratio(_ a: String, _ b: String) -> Double {
        let la = luminance(a)
        let lb = luminance(b)
        return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
    }
}
