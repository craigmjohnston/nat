import AppKit
import XCTest
@testable import NatKit

final class DefaultCursorViewTests: XCTestCase {
    @MainActor
    func testTrackingAreaClaimsCursorUpdateOverTheVisibleRect() {
        let view = CursorFloorNSView(frame: NSRect(x: 0, y: 0, width: 100, height: 100))
        view.updateTrackingAreas()

        XCTAssertEqual(view.trackingAreas.count, 1)
        let options = view.trackingAreas[0].options
        XCTAssertTrue(options.contains(.cursorUpdate))
        XCTAssertTrue(options.contains(.activeInKeyWindow))
        XCTAssertTrue(options.contains(.inVisibleRect))
    }

    @MainActor
    func testUpdateTrackingAreasReplacesRatherThanAccumulates() {
        let view = CursorFloorNSView(frame: NSRect(x: 0, y: 0, width: 100, height: 100))
        view.updateTrackingAreas()
        view.updateTrackingAreas()

        XCTAssertEqual(view.trackingAreas.count, 1)
    }

    @MainActor
    func testCursorUpdateSetsTheArrow() {
        let view = CursorFloorNSView(frame: NSRect(x: 0, y: 0, width: 100, height: 100))
        NSCursor.iBeam.set()

        view.cursorUpdate(with: NSEvent())

        XCTAssertEqual(NSCursor.current, NSCursor.arrow)
    }

    @MainActor
    func testInvisibleToHitTesting() {
        let view = CursorFloorNSView(frame: NSRect(x: 0, y: 0, width: 100, height: 100))

        XCTAssertNil(view.hitTest(NSPoint(x: 50, y: 50)))
    }

    /// `NSViewRepresentableContext` cannot be constructed directly, so the
    /// representable is exercised the way SwiftUI itself does it: mounted in
    /// a hosting view, then the floor view found in the AppKit hierarchy it
    /// produced.
    @MainActor
    func testRepresentableMountsTheFloorView() {
        let hosting = NSHostingView(rootView: DefaultCursorView())
        hosting.frame = NSRect(x: 0, y: 0, width: 100, height: 100)
        hosting.layoutSubtreeIfNeeded()

        XCTAssertNotNil(findFloor(in: hosting))
    }

    @MainActor
    private func findFloor(in view: NSView) -> CursorFloorNSView? {
        if let floor = view as? CursorFloorNSView { return floor }
        for subview in view.subviews {
            if let floor = findFloor(in: subview) { return floor }
        }
        return nil
    }
}
