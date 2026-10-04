/// The bookkeeping behind gnat's right-click diagnostics (`MenuDebug` in
/// NatApp): which right-click is still waiting for a menu, and what the
/// probe run on one that never got one says about where it died.
///
/// It exists for the intermittent wedge where SwiftUI's `.contextMenu`s stop
/// presenting app-wide until relaunch, while menu-bar menus and text-field
/// menus go on working — so the question each failed right-click has to
/// answer is which of three places it stopped in, and that is all this
/// decides.
public struct RightClickTrace: Sendable {
    private var lastClick = 0
    private var awaiting: Int?

    public init() {}

    /// Records a right-click (or control-click) that reached a window and
    /// answers the number its later `settle` is asked with.
    public mutating func clicked() -> Int {
        lastClick += 1
        awaiting = lastClick
        return lastClick
    }

    /// Records that a menu began tracking: whatever click was waiting got
    /// one.
    public mutating func menuBegan() {
        awaiting = nil
    }

    /// What became of click `click` once its grace period is over: `true`
    /// where it is still waiting — no menu began since — and `false` where
    /// one did. A click a later one has superseded answers `nil`, since that
    /// later click's own settle is the one that speaks for the moment.
    public mutating func settle(_ click: Int) -> Bool? {
        guard click == lastClick else { return nil }
        defer { awaiting = nil }
        return awaiting == click
    }
}

/// Where a right-click that opened no menu stopped, read off the view the
/// window hit-tested it to and what that view's `menu(for:)` answered.
///
/// SwiftUI serves a `.contextMenu` through its hosting view's `menu(for:)`
/// (checked on macOS 15.7), so the three cases are three different faults:
/// the click never reached SwiftUI at all, SwiftUI no longer has a menu for
/// that point, or it has one and AppKit never put it up.
public enum RightClickDiagnosis: Equatable, Sendable {
    /// The window sent the click to a view outside SwiftUI's hosting
    /// view — something is covering it.
    case notSwiftUI(hit: String)
    /// SwiftUI's hosting view got the click but answers no menu for it.
    case noMenuFromSwiftUI
    /// SwiftUI answers a menu for the point, and it was never presented.
    case menuNotPresented(items: [String])

    /// `hit` is the hit view's type name (`nil` where nothing was hit),
    /// `insideHostingView` whether that view is SwiftUI's hosting view or
    /// sits inside one, and `menuItems` the titles of the first menu
    /// `menu(for:)` answered walking up from it (`nil` for no menu).
    public init(hit: String?, insideHostingView: Bool, menuItems: [String]?) {
        if !insideHostingView {
            self = .notSwiftUI(hit: hit ?? "nothing")
        } else if let menuItems {
            self = .menuNotPresented(items: menuItems)
        } else {
            self = .noMenuFromSwiftUI
        }
    }

    /// One log line's worth, led by the case's name so a log search finds
    /// every occurrence of one fault.
    public var summary: String {
        switch self {
        case .notSwiftUI(let hit):
            "not-swiftui: the click was hit-tested to \(hit), not SwiftUI's hosting view"
        case .noMenuFromSwiftUI:
            "no-menu-from-swiftui: the hosting view's menu(for:) answered nil"
        case .menuNotPresented(let items):
            "menu-not-presented: menu(for:) answered \(items) but no menu began tracking"
        }
    }
}
