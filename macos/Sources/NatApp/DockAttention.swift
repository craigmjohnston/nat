import AppKit
import NatKit

/// gnat's dock icon over `AppModel.dockAttention`: the badge counting what
/// waits on the user, the menu listing it, and one bounce when something
/// new arrives while the app is in the background. What counts, how the menu
/// groups it and what is an arrival are all NatKit's (`attentionItems`,
/// `dockMenuSections`, `AttentionChange`); this only hands them to AppKit.
/// Stories cannot render the dock, which is why it is kept this thin.
@MainActor
final class DockAttention: NSObject {
    static let shared = DockAttention()

    private weak var model: AppModel?
    /// The last reading applied — what the next one's arrivals are read
    /// against.
    private var shown: [AttentionItem] = []
    /// The badge as last set, so it is set only when it changes.
    private var badge: String?
    /// The rows of the menu last built, by tag.
    private var rows: [DockMenuRow] = []

    /// Follows `model` for the life of the app: every plan read, activity
    /// re-read and `pr-status` reading that changes `dockAttention` lands
    /// here through observation, with no poll of its own.
    func start(_ model: AppModel) {
        self.model = model
        observe()
    }

    private func observe() {
        guard let model else { return }
        let items = withObservationTracking { model.dockAttention } onChange: { [weak self] in
            Task { @MainActor in self?.observe() }
        }
        apply(items)
    }

    private func apply(_ items: [AttentionItem]) {
        // The pill's own rule: a plain number, none at zero.
        let label = ProjectAttention(count: items.count, role: .idle).badge.map(String.init)
        if label != badge {
            badge = label
            NSApp.dockTile.badgeLabel = label
        }
        // macOS ignores the request while the app is active, and none is
        // faked in its place.
        if !NSApp.isActive, !AttentionChange.arrivals(from: shown, to: items).isEmpty {
            NSApp.requestUserAttention(.informationalRequest)
        }
        shown = items
    }

    /// The dock icon's menu, built on demand so it is always current: a
    /// heading per kind, then a row per item. Plain titles only: the Dock
    /// draws the menu itself and drops an `attributedTitle`'s fonts and
    /// colours (tried: small caps and a secondary colour came out plain). Nil with nothing waiting,
    /// leaving only the default entries.
    func menu() -> NSMenu? {
        guard let model else { return nil }
        let sections = model.dockMenu
        guard !sections.isEmpty else { return nil }
        let menu = NSMenu()
        rows = []
        for section in sections {
            menu.addItem(NSMenuItem.sectionHeader(title: section.heading))
            for row in section.rows {
                let item = NSMenuItem(title: row.title, action: #selector(choose(_:)), keyEquivalent: "")
                item.target = self
                item.tag = rows.count
                rows.append(row)
                menu.addItem(item)
            }
        }
        return menu
    }

    /// Brings the window forward on the row's slice — or session, or
    /// workshop.
    @objc private func choose(_ sender: NSMenuItem) {
        guard let model, rows.indices.contains(sender.tag) else { return }
        let item = rows[sender.tag].item
        NSApp.activate()
        let windows = NSApp.windows.filter(\.canBecomeMain)
        if let window = windows.first(where: { $0.identifier?.rawValue == "main" }) ?? windows.first {
            window.deminiaturize(nil)
            window.makeKeyAndOrderFront(nil)
        }
        Task { await model.select(item) }
    }
}

/// The app delegate SwiftUI forwards to — here only for the dock menu.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDockMenu(_ sender: NSApplication) -> NSMenu? {
        DockAttention.shared.menu()
    }
}
