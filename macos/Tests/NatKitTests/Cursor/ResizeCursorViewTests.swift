import AppKit
import XCTest
@testable import NatKit

final class ResizeCursorViewTests: XCTestCase {
    @MainActor
    func testResetCursorRectsCoversTheWholeBounds() {
        let view = ResizeCursorNSView(frame: NSRect(x: 0, y: 0, width: 9, height: 100))

        view.resetCursorRects()

        XCTAssertEqual(view.lastCursorRect, NSRect(x: 0, y: 0, width: 9, height: 100))
    }

    @MainActor
    func testResetCursorRectsTracksAResize() {
        let view = ResizeCursorNSView(frame: NSRect(x: 0, y: 0, width: 9, height: 100))
        view.resetCursorRects()

        view.setFrameSize(NSSize(width: 9, height: 250))
        view.resetCursorRects()

        XCTAssertEqual(view.lastCursorRect, NSRect(x: 0, y: 0, width: 9, height: 250))
    }

    @MainActor
    func testCursorUpdateSetsTheHorizontalResizeCursor() {
        let view = ResizeCursorNSView(frame: NSRect(x: 0, y: 0, width: 9, height: 100))
        NSCursor.arrow.set()

        view.cursorUpdate(with: NSEvent())

        XCTAssertEqual(NSCursor.current, NSCursor.resizeLeftRight)
    }

    @MainActor
    func testInvisibleToHitTesting() {
        let view = ResizeCursorNSView(frame: NSRect(x: 0, y: 0, width: 9, height: 100))

        XCTAssertNil(view.hitTest(NSPoint(x: 4, y: 50)))
    }

    /// Mounted the way SwiftUI itself mounts it — see
    /// `DefaultCursorViewTests` for why a hosting view stands in for the
    /// unconstructible representable context.
    @MainActor
    func testRepresentableMountsTheCursorView() {
        let hosting = NSHostingView(rootView: ResizeCursorView())
        hosting.frame = NSRect(x: 0, y: 0, width: 9, height: 100)
        hosting.layoutSubtreeIfNeeded()

        XCTAssertNotNil(findCursorView(in: hosting))
    }

    @MainActor
    private func findCursorView(in view: NSView) -> ResizeCursorNSView? {
        if let found = view as? ResizeCursorNSView { return found }
        for subview in view.subviews {
            if let found = findCursorView(in: subview) { return found }
        }
        return nil
    }
}
