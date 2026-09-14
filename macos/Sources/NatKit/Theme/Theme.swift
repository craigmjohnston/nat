import SwiftUI

/// Which of the two palettes the app draws with, as the user has chosen it:
/// one of the two outright, or `system`, which is macOS's own answer and
/// changes under the app when the Mac does.
///
/// It is a preference of this app's face and not of the tracker, so it is
/// held in `UserDefaults` (`@AppStorage`) rather than in nat's config file:
/// nothing headless has a colour, and a second machine opening the same
/// projects has its own eyes and its own room to sit in.
public enum Theme: String, CaseIterable, Identifiable, Sendable {
    case system
    case dark
    case light

    /// The `@AppStorage` key the chosen theme is persisted under.
    public static let storageKey = "appTheme"

    public var id: String { rawValue }

    /// What the switcher calls this option.
    public var title: String {
        switch self {
        case .system: return "System"
        case .dark: return "Dark"
        case .light: return "Light"
        }
    }

    /// The scheme to pin the window to, or nil to leave it to macOS — which
    /// is the whole of what `system` means, since an unpinned window follows
    /// the Mac's appearance by itself and goes on following it as it changes.
    public var colorScheme: ColorScheme? {
        switch self {
        case .system: return nil
        case .dark: return .dark
        case .light: return .light
        }
    }

    /// Reads a stored value back. Anything that is not one of the three —
    /// a key never written, or one left by a build that named its options
    /// differently — is `system`, which is the setting that needs no
    /// explaining.
    public init(stored value: String?) {
        self = value.flatMap(Theme.init(rawValue:)) ?? .system
    }
}
