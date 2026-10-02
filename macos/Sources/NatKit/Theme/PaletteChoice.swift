import Foundation
import Synchronization

/// One of the palettes the app ships, by the name the user picks it by.
///
/// `Theme` says whether a window is dark or light (or follows the Mac);
/// this says *which* dark palette and *which* light one. The two are
/// separate preferences because they answer separate questions: a user on
/// `system` gets their dark choice at night and their light choice by day,
/// without picking again each time the Mac switches.
public enum PaletteChoice: String, CaseIterable, Identifiable, Sendable {
    case light
    case oneLight
    case tokyoDay
    case iceberg
    case slateInk

    /// The `@AppStorage` key the dark slot's choice is persisted under.
    public static let darkStorageKey = "darkPalette"
    /// The `@AppStorage` key the light slot's choice is persisted under.
    public static let lightStorageKey = "lightPalette"

    /// What each slot draws with until the user picks otherwise.
    public static let defaultDark = PaletteChoice.iceberg
    public static let defaultLight = PaletteChoice.oneLight

    public var id: String { rawValue }

    public var palette: Palette {
        switch self {
        case .light: .light
        case .oneLight: .oneLight
        case .tokyoDay: .tokyoDay
        case .iceberg: .iceberg
        case .slateInk: .slateInk
        }
    }

    /// What the settings picker calls this palette.
    public var title: String {
        switch self {
        case .light: "Light"
        case .oneLight: "One Light"
        case .tokyoDay: "Tokyo Night Day"
        case .iceberg: "Iceberg"
        case .slateInk: "Slate ink"
        }
    }

    /// The palettes one slot offers: the dark ones for the dark slot, the
    /// light ones for the light.
    public static func choices(dark: Bool) -> [PaletteChoice] {
        allCases.filter { $0.palette.isDark == dark }
    }

    /// Reads a slot's stored value back. Anything that is not a palette of
    /// that slot — a key never written, a palette a later build removed, or
    /// a light palette stored under the dark key — is the slot's default, so
    /// the dark slot can never hand a dark window a light palette.
    public init(stored value: String?, dark: Bool) {
        let fallback = dark ? Self.defaultDark : Self.defaultLight
        guard let choice = value.flatMap(PaletteChoice.init(rawValue:)),
              choice.palette.isDark == dark
        else {
            self = fallback
            return
        }
        self = choice
    }
}

/// The palette each slot is drawing with right now, read by every dynamic
/// token as it resolves.
///
/// It is process-wide state rather than an environment value because the
/// tokens are AppKit dynamic colours: they are asked to resolve with an
/// appearance and nothing else, from whatever thread AppKit draws on, so
/// what they read has to be reachable from there and safe to read from
/// there. The app writes it from its stored preferences (`NatApp`); the
/// gallery writes it for a `--palette` run.
public final class PaletteSelection: Sendable {
    public static let shared = PaletteSelection()

    private let slots = Mutex((dark: PaletteChoice.defaultDark, light: PaletteChoice.defaultLight))

    init() {}

    /// The palette a dark or a light window draws with.
    public func palette(dark: Bool) -> Palette {
        slots.withLock { dark ? $0.dark.palette : $0.light.palette }
    }

    /// Puts a palette in its own slot — a dark palette in the dark slot, a
    /// light one in the light — and leaves the other slot as it was.
    public func select(_ choice: PaletteChoice) {
        slots.withLock {
            if choice.palette.isDark { $0.dark = choice } else { $0.light = choice }
        }
    }
}
