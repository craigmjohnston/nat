import AppKit

/// What double-clicking the shell's custom title bar does. With the system
/// title bar hidden the header row is the title bar, but only AppKit's own
/// bar answers a double-click with the zoom the system promises — this is
/// that answer, honouring the user's "double-click a window's title bar to"
/// setting (`AppleActionOnDoubleClick` in the global defaults domain) the
/// way the real bar does: minimize when it says so, nothing when it says
/// none, and otherwise zoom — which is the unset default, and what both
/// "Maximize" and the newer "Fill" amount to for a window without the
/// system's tiling.
public enum TitlebarDoubleClick {
    public enum Action: Equatable {
        case zoom
        case minimize
        case none
    }

    public static func action(for setting: String?) -> Action {
        switch setting {
        case "Minimize": return .minimize
        case "None": return .none
        default: return .zoom
        }
    }

    @MainActor
    public static func perform(on window: NSWindow?) {
        guard let window else { return }
        switch action(for: UserDefaults.standard.string(forKey: "AppleActionOnDoubleClick")) {
        case .zoom: window.zoom(nil)
        case .minimize: window.miniaturize(nil)
        case .none: break
        }
    }
}
