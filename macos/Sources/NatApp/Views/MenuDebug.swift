import AppKit
import NatKit

/// A diagnostic-only trace of right-clicks and context menus, for the
/// intermittent wedge where gnat's SwiftUI `.contextMenu`s stop presenting
/// app-wide until relaunch. It fixes nothing itself: it logs every
/// right-click (and control-click) the app is offered, whether a menu began
/// tracking after it, and — for one that got none — where it stopped
/// (`RightClickDiagnosis`), alongside what happened before it: sleep and
/// wake, activation, key-window and screen changes, sheets.
/// `docs/debugging/context-menus.md` says how to turn it on and read it.
///
/// Gated on `NAT_MENU_DEBUG=1` or the `NatMenuDebug` user default, read once
/// at launch — the default so the installed app, launched from the Finder
/// with no environment of its own, can carry it for the days a wedge may
/// take to come. With neither set `startIfAsked` installs nothing.
@MainActor
enum MenuDebug {
    static let enabled = ProcessInfo.processInfo.environment["NAT_MENU_DEBUG"] == "1"
        || UserDefaults.standard.bool(forKey: "NatMenuDebug")

    /// How long a right-click waits for a menu before it is called a miss.
    /// SwiftUI opens one inside the click's own dispatch, so this only has to
    /// outlast that; it is generous so a busy main thread does not read as a
    /// miss.
    private static let grace: TimeInterval = 0.75

    private static var trace = RightClickTrace()
    private static var openMenus = 0
    /// Kept for the life of the process: the trace runs until quit.
    private static var tokens: [Any] = []

    static func log(_ message: @autoclosure () -> String) {
        guard enabled else { return }
        NSLog("nat menu-debug: %@", message())
    }

    /// Installs the right-click monitor and the observers, once; a no-op
    /// unless `enabled`. `NatApp` is the only caller.
    static func startIfAsked() {
        guard enabled, tokens.isEmpty else { return }
        log("enabled on macOS \(ProcessInfo.processInfo.operatingSystemVersionString)")

        // Offered every mouse-down before any view sees it, and always
        // passing it on: this watches, it never decides.
        if let monitor = NSEvent.addLocalMonitorForEvents(matching: [.rightMouseDown, .leftMouseDown], handler: { event in
            if event.type == .rightMouseDown || event.modifierFlags.contains(.control) {
                MainActor.assumeIsolated { clicked(event) }
            }
            return event
        }) {
            tokens.append(monitor)
        }

        let center = NotificationCenter.default
        observe(center, NSMenu.didBeginTrackingNotification) { note in
            openMenus += 1
            trace.menuBegan()
            log("menu began tracking (open: \(openMenus)) \(menuTitles(note.object as? NSMenu))")
        }
        observe(center, NSMenu.didEndTrackingNotification) { note in
            openMenus -= 1
            log("menu ended tracking (open: \(openMenus)) \(menuTitles(note.object as? NSMenu))")
        }
        for name in [
            NSApplication.didBecomeActiveNotification, NSApplication.didResignActiveNotification,
            NSApplication.didChangeScreenParametersNotification,
            NSWindow.didBecomeKeyNotification, NSWindow.didResignKeyNotification,
            NSWindow.willBeginSheetNotification, NSWindow.didEndSheetNotification,
            NSWindow.didChangeScreenNotification, NSWindow.didChangeBackingPropertiesNotification,
        ] {
            observe(center, name) { note in
                let window = note.object as? NSWindow
                // A menu's own popup and the menu bar's windows post these on
                // every open; the menu tracking lines already say that.
                if let window, String(describing: type(of: window)).contains("Menu") { return }
                log("\(name.rawValue) \(windowName(window))")
            }
        }
        let workspace = NSWorkspace.shared.notificationCenter
        for name in [
            NSWorkspace.willSleepNotification, NSWorkspace.didWakeNotification,
            NSWorkspace.screensDidSleepNotification, NSWorkspace.screensDidWakeNotification,
            NSWorkspace.sessionDidResignActiveNotification, NSWorkspace.sessionDidBecomeActiveNotification,
        ] {
            observe(workspace, name) { _ in log(name.rawValue) }
        }
    }

    private static func observe(_ center: NotificationCenter, _ name: Notification.Name, _ body: @escaping @MainActor (Notification) -> Void) {
        tokens.append(center.addObserver(forName: name, object: nil, queue: .main) { note in
            // Delivered on the main queue, so the note never leaves it.
            nonisolated(unsafe) let note = note
            MainActor.assumeIsolated { body(note) }
        })
    }

    private static func clicked(_ event: NSEvent) {
        let click = trace.clicked()
        let hit = hitView(event)
        log("right-click #\(click) \(event.type == .rightMouseDown ? "right" : "control") at \(event.locationInWindow) in \(windowName(event.window)) hit \(ancestry(hit))")
        // Common modes, so a menu's own tracking loop cannot hold it back;
        // a main run loop timer, so the event never leaves it.
        nonisolated(unsafe) let event = event
        let timer = Timer(timeInterval: grace, repeats: false) { _ in
            MainActor.assumeIsolated { settle(click, event: event, hit: hit) }
        }
        RunLoop.main.add(timer, forMode: .common)
    }

    private static func settle(_ click: Int, event: NSEvent, hit: NSView?) {
        guard let missed = trace.settle(click) else { return }
        guard missed else {
            log("right-click #\(click) presented a menu")
            return
        }
        // Asked only after a miss, so the probe can never disturb a
        // right-click that was going to work.
        var menuItems: [String]?
        var view = hit
        while let current = view, menuItems == nil {
            menuItems = current.menu(for: event)?.items.map(\.title)
            view = current.superview
        }
        let diagnosis = RightClickDiagnosis(
            hit: hit.map { String(describing: type(of: $0)) },
            insideHostingView: ancestors(hit).contains { isHostingView($0) },
            menuItems: menuItems
        )
        let window = event.window
        log("""
            NO MENU for right-click #\(click): \(diagnosis.summary) \
            | open menus \(openMenus), modal \(windowName(NSApp.modalWindow)), \
            sheet \(windowName(window?.attachedSheet)), key \(windowName(NSApp.keyWindow)), \
            active \(NSApp.isActive), first responder \(window?.firstResponder.map { String(describing: type(of: $0)) } ?? "nil")
            """)
    }

    /// The view the window would send `event` to — what `NSWindow` hit-tests
    /// a mouse-down to, from the frame view down.
    private static func hitView(_ event: NSEvent) -> NSView? {
        guard let frame = event.window?.contentView?.superview else { return nil }
        return frame.hitTest(frame.convert(event.locationInWindow, from: nil))
    }

    private static func ancestors(_ view: NSView?) -> [NSView] {
        var out: [NSView] = []
        var current = view
        while let next = current {
            out.append(next)
            current = next.superview
        }
        return out
    }

    private static func isHostingView(_ view: NSView) -> Bool {
        String(describing: type(of: view)).contains("HostingView")
    }

    /// The hit view and the views it sits in, innermost first, as far as
    /// the first hosting view.
    private static func ancestry(_ view: NSView?) -> String {
        var names: [String] = []
        for next in ancestors(view) {
            names.append(String(describing: type(of: next)))
            if isHostingView(next) || names.count == 8 { break }
        }
        return names.isEmpty ? "nothing" : names.joined(separator: " < ")
    }

    private static func windowName(_ window: NSWindow?) -> String {
        guard let window else { return "none" }
        return "\(String(describing: type(of: window)))#\(window.windowNumber)"
    }

    private static func menuTitles(_ menu: NSMenu?) -> String {
        guard let menu else { return "[]" }
        return "\(menu.title.isEmpty ? "" : "\"\(menu.title)\" ")\(menu.items.prefix(4).map(\.title))"
    }
}
