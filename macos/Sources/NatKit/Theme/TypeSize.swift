import Foundation
import Synchronization

/// The two text sizes the user picks in Settings ▸ General: the size the
/// app's own type is set at, and the size code is.
///
/// Like `Theme` and `PaletteChoice` they are this Mac's appearance rather
/// than nat's configuration, so they live in `UserDefaults` (`@AppStorage`)
/// and nat never hears of them.
///
/// Each is one number, in whole points, and means what the user can see:
/// `ui` is the size a sidebar slice row is drawn at (`Typo.body`), and every
/// other size on the ramp follows it in proportion; `mono` is the size a
/// line of code is drawn at in the agent terminal and in the diff.
public struct TypeSize: Equatable, Sendable {
    /// The `@AppStorage` key the UI size is persisted under.
    public static let uiStorageKey = "uiFontSize"
    /// The `@AppStorage` key the monospace size is persisted under.
    public static let monoStorageKey = "monoFontSize"

    /// What each size is until the user picks otherwise. The UI size is the
    /// ramp's own body, so the default draws the app exactly as it was; the
    /// monospace size is a point over the `Typo.code` the terminal and the
    /// diff used to share, on purpose.
    public static let defaultUI = 14
    public static let defaultMono = 14

    /// The sizes either field offers. The floor keeps the ramp's caption
    /// legible; the UI ceiling is what a fixed-height row (a sidebar row, a
    /// section head) still holds a line of, and the monospace one what the
    /// diff's header band does.
    public static let uiRange = 10...20
    public static let monoRange = 10...24

    public static let `default` = TypeSize(ui: defaultUI, mono: defaultMono)

    public var ui: Int
    public var mono: Int

    /// Reads the two stored values back, each clamped to its range — a
    /// value a hand-edited defaults file or a later build put there out of
    /// range is the nearest one in it, never a window drawn at 2pt.
    public init(ui: Int, mono: Int) {
        self.ui = min(max(ui, Self.uiRange.lowerBound), Self.uiRange.upperBound)
        self.mono = min(max(mono, Self.monoRange.lowerBound), Self.monoRange.upperBound)
    }

    /// One identity for the pair, for the window content's `.id` — see
    /// `NatApp.paletteIdentity`, which this joins.
    public var identity: String { "\(ui)/\(mono)" }
}

/// The sizes the app is drawing at right now, read by every size on the
/// `Typo` ramp as it is asked for.
///
/// Process-wide rather than an environment value for the reason
/// `PaletteSelection` is: the ramp is read as plain numbers, from SwiftUI
/// bodies and AppKit drawing alike, with nothing to carry an environment
/// through. The app writes it from its stored preferences (`NatApp`); a test
/// writes and then restores it.
public final class TypeSizeSelection: Sendable {
    public static let shared = TypeSizeSelection()

    private let size = Mutex(TypeSize.default)

    init() {}

    /// The sizes being drawn at.
    public var current: TypeSize {
        size.withLock { $0 }
    }

    /// Draws at these sizes from here on.
    public func select(_ newSize: TypeSize) {
        size.withLock { $0 = newSize }
    }
}
