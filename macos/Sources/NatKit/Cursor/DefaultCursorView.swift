import AppKit
import SwiftUI

/// The window's cursor floor. The app never registered anything with AppKit's
/// cursor-update machinery, so the cursor over the window stayed whatever the
/// last cursor rect set — including one belonging to whatever sat *behind*
/// the app when the mouse crossed into it. This view spans the window content
/// (mounted as the shell's background) and claims `cursorUpdate` for the
/// whole rectangle, answering with the arrow; a view that wants a cursor of
/// its own (the terminal's I-beam, a text field's) registers a deeper cursor
/// rect and still wins over it.
public struct DefaultCursorView: NSViewRepresentable {
    public init() {}

    public func makeNSView(context: Context) -> NSView {
        CursorFloorNSView()
    }

    public func updateNSView(_ nsView: NSView, context: Context) {}
}

/// Internal rather than private so the tests can reach it: the tracking-area
/// options and the hit-test opt-out are the whole behavior, and both are
/// exactly the kind of thing a refactor silently loses.
final class CursorFloorNSView: NSView {
    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        for area in trackingAreas {
            removeTrackingArea(area)
        }
        // `.inVisibleRect` keeps the area sized with the view, so a resize
        // never leaves a stale rectangle; `.activeInKeyWindow` because a
        // background window's cursor is not this window's to set.
        addTrackingArea(NSTrackingArea(
            rect: .zero,
            options: [.cursorUpdate, .activeInKeyWindow, .inVisibleRect],
            owner: self
        ))
    }

    override func cursorUpdate(with event: NSEvent) {
        NSCursor.arrow.set()
    }

    /// Invisible to clicks: the floor is about the cursor alone, and a
    /// background that swallowed a press would take it from the board.
    override func hitTest(_ point: NSPoint) -> NSView? {
        nil
    }
}
