import AppKit
import ObjectiveC

/// A diagnostic-only walker for the persistent I-beam cursor bug: it exists
/// to find who is setting the cursor, not to fix anything itself.
///
/// Two AppKit mechanisms put a cursor on screen — a tracking area with
/// `.cursorUpdate` (or one of the AppKit conveniences built on it), and a
/// view overriding `resetCursorRects` to add one directly (SwiftTerm's
/// `TerminalView` does exactly this, per `cursorUpdate(with:)` setting
/// `.iBeam`). Either is a candidate for a cursor stuck showing something a
/// view drawn on top no longer overrides, so every view answering yes to
/// either is what gets logged, walked from the key window's content view
/// down through every subview.
///
/// Entirely gated on `NAT_CURSOR_DEBUG=1`: `startIfAsked()` is the one call
/// `NatApp` makes, and with the env var absent it does nothing at all — no
/// timer, no walk, no log line.
@MainActor
enum CursorDebugWalker {
    /// Starts the two-second walk if `NAT_CURSOR_DEBUG=1` is set; otherwise a
    /// no-op. Meant to be called once, at launch — `NatApp` is this file's
    /// only caller.
    static func startIfAsked() {
        guard ProcessInfo.processInfo.environment["NAT_CURSOR_DEBUG"] == "1" else { return }
        NSLog("nat cursor-debug: enabled (NAT_CURSOR_DEBUG=1)")

        let timer = Timer(timeInterval: 2, repeats: true) { _ in
            Task { @MainActor in walkOnce() }
        }
        RunLoop.main.add(timer, forMode: .common)
        // Fire once immediately rather than waiting out the first interval.
        Task { @MainActor in walkOnce() }
    }

    /// One tick: every qualifying view under the key window's content view,
    /// then the system's own idea of the current cursor.
    private static func walkOnce() {
        guard let contentView = NSApp.keyWindow?.contentView else {
            NSLog("nat cursor-debug: no key window")
            return
        }
        walk(contentView)
        NSLog("nat cursor-debug: NSCursor.current = %@", String(describing: NSCursor.current))
    }

    private static func walk(_ view: NSView) {
        let trackingCount = view.trackingAreas.count
        let ownsResetCursorRects = overridesResetCursorRects(type(of: view))

        if trackingCount > 0 || ownsResetCursorRects {
            let frameInWindow = view.convert(view.bounds, to: nil)
            NSLog(
                "nat cursor-debug: %@ trackingAreas=%d resetCursorRectsOverridden=%@ frame=%@",
                String(describing: type(of: view)),
                trackingCount,
                ownsResetCursorRects ? "yes" : "no",
                NSStringFromRect(frameInWindow)
            )
        }

        for subview in view.subviews {
            walk(subview)
        }
    }

    /// Whether `cls` (or something between it and `NSView`) supplies its own
    /// `resetCursorRects`, found the same way the runtime resolves the
    /// method call itself: the implementation `class_getInstanceMethod`
    /// finds for `cls` compared against the one it finds for `NSView`
    /// directly. Equal means nothing between `cls` and `NSView` overrode it.
    private static let baseResetCursorRectsIMP: IMP? = {
        class_getInstanceMethod(NSView.self, #selector(NSView.resetCursorRects))
            .map(method_getImplementation)
    }()

    private static func overridesResetCursorRects(_ cls: AnyClass) -> Bool {
        guard let method = class_getInstanceMethod(cls, #selector(NSView.resetCursorRects)) else {
            return false
        }
        return method_getImplementation(method) != baseResetCursorRectsIMP
    }
}
