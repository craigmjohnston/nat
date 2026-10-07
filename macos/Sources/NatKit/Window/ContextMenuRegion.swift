import AppKit
import SwiftUI

/// Presents the context menus of the region it is mounted behind through
/// gnat's own path, on right-click and control-click alike, rather than
/// SwiftUI's right-button handling.
///
/// A captured trace of the wedge where right-click stops opening the
/// sidebar's menus (`docs/debugging/context-menus.md`) read
/// `menu-not-presented` on every missed row: SwiftUI's hosting view still
/// answered the row's whole menu from `menu(for:)`, and nothing put it up,
/// while a control-click on the same row — AppKit asking `menu(for:)` itself
/// and presenting the answer — opened it at once. This is that control-click
/// path, taken for every menu click in the region: the hit view's
/// `menu(for:)`, walking up as `RightClickDiagnosis` does, shown with
/// `NSMenu.popUpContextMenu(_:with:for:)`. SwiftUI's `.contextMenu` stays the
/// source of every menu; only who presents it changes. A click it presents
/// for is consumed, so SwiftUI never presents a second menu for it — wedged
/// or healthy, the behaviour is the same.
///
/// Mount it as a `.background` of the region; it draws nothing and is never
/// hit, so it takes no click from what it sits behind. `ContextMenuRegion.install()`
/// puts the one event monitor in place.
public struct ContextMenuRegion: NSViewRepresentable {
    public init() {}

    public func makeNSView(context: Context) -> ContextMenuRegionView {
        ContextMenuRegionView()
    }

    public func updateNSView(_ nsView: ContextMenuRegionView, context: Context) {}

    /// The app's mounted regions. Weak, so a region torn down with its view
    /// stops claiming clicks without having to say so.
    @MainActor static let regions = NSHashTable<ContextMenuRegionView>.weakObjects()
    @MainActor private static var monitor: Any?

    /// Installs the right- and control-click monitor, once. Local monitors
    /// are offered an event in the order they were installed, so installing
    /// this after `MenuDebug`'s leaves the trace seeing every click before
    /// this one consumes it.
    @MainActor public static func install() {
        guard monitor == nil else { return }
        monitor = NSEvent.addLocalMonitorForEvents(matching: [.rightMouseDown, .leftMouseDown]) { event in
            // Delivered on the main thread, so the event never leaves it.
            nonisolated(unsafe) let event = event
            let consumed = MainActor.assumeIsolated {
                handle(event, present: { menu, event, view in
                    NSMenu.popUpContextMenu(menu, with: event, for: view)
                }) == nil
            }
            return consumed ? nil : event
        }
    }

    /// What the monitor does with `event`: `nil` — consumed — where it is a
    /// menu click inside a mounted region and the view under it answers a
    /// menu, which `present` is handed with the view that answered it;
    /// `event` itself, passed on untouched, everywhere else.
    @MainActor static func handle(
        _ event: NSEvent,
        present: (NSMenu, NSEvent, NSView) -> Void
    ) -> NSEvent? {
        guard isMenuClick(event), let window = event.window,
              regions.allObjects.contains(where: { $0.contains(event.locationInWindow, in: window) }),
              let (menu, view) = menu(for: event, in: window) else { return event }
        present(menu, event, view)
        return nil
    }

    /// A right mouse-down, or a left one with control held — the two clicks
    /// AppKit itself treats as asking for a context menu.
    static func isMenuClick(_ event: NSEvent) -> Bool {
        event.type == .rightMouseDown
            || (event.type == .leftMouseDown && event.modifierFlags.contains(.control))
    }

    /// The first menu `menu(for:)` answers walking up from the view the
    /// window hit-tests `event` to (from the frame view down, as `NSWindow`
    /// does), with the view that answered it.
    @MainActor static func menu(for event: NSEvent, in window: NSWindow) -> (NSMenu, NSView)? {
        guard let frame = window.contentView?.superview else { return nil }
        var view = frame.hitTest(frame.convert(event.locationInWindow, from: nil))
        while let current = view {
            if let menu = current.menu(for: event) { return (menu, current) }
            view = current.superview
        }
        return nil
    }
}

/// The marker `ContextMenuRegion` mounts: its frame is the region.
public final class ContextMenuRegionView: NSView {
    override public func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        if window == nil {
            ContextMenuRegion.regions.remove(self)
        } else {
            ContextMenuRegion.regions.add(self)
        }
    }

    /// Never the view a click lands on: the rows it sits behind keep theirs.
    override public func hitTest(_ point: NSPoint) -> NSView? { nil }

    /// Whether `point`, in `window`'s coordinates, falls inside this region
    /// of that window.
    func contains(_ point: NSPoint, in window: NSWindow) -> Bool {
        self.window === window && bounds.contains(convert(point, from: nil))
    }
}
