import AppKit
import SwiftUI

/// The horizontal-resize cursor over a pane divider's drag handle. It wins
/// over the window's cursor floor the way the terminal's I-beam does: with a
/// cursor rect (`resetCursorRects`), the one mechanism `DefaultCursorView`
/// documents itself yielding to — a `cursorUpdate` tracking area of the
/// handle's own merely overlapped the floor's and lost, leaving the arrow up
/// while hovering. Mounted as the handle's background; invisible to clicks,
/// so the SwiftUI drag gesture drawn over it still receives the mouse.
public struct ResizeCursorView: NSViewRepresentable {
    public init() {}

    public func makeNSView(context: Context) -> NSView {
        ResizeCursorNSView()
    }

    public func updateNSView(_ nsView: NSView, context: Context) {}
}

/// Internal rather than private so the tests can reach it — see
/// `CursorFloorNSView` for why the cursor answer and the hit-test opt-out
/// are the parts worth pinning.
final class ResizeCursorNSView: NSView {
    /// What the last `resetCursorRects` registered — recorded because AppKit
    /// offers no read-back of a view's cursor rects, and the rect covering
    /// the whole bounds is exactly what a refactor could silently shrink.
    private(set) var lastCursorRect: NSRect?

    override func resetCursorRects() {
        addCursorRect(bounds, cursor: .resizeLeftRight)
        lastCursorRect = bounds
    }

    /// Entering the cursor rect arrives as a `cursorUpdate` event; answering
    /// it here keeps the cursor ours even when the default implementation's
    /// own rect lookup is what fields it.
    override func cursorUpdate(with event: NSEvent) {
        NSCursor.resizeLeftRight.set()
    }

    /// Invisible to clicks: the cursor is this view's whole job, and the drag
    /// itself belongs to the SwiftUI gesture laid over it.
    override func hitTest(_ point: NSPoint) -> NSView? {
        nil
    }
}
