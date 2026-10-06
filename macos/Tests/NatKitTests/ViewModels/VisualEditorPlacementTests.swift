import CoreGraphics
import XCTest
@testable import NatKit

final class VisualEditorPlacementTests: XCTestCase {
    private let box = CGSize(width: 320, height: 200)
    private let pane = CGSize(width: 730, height: 760)

    private func pin(_ x: CGFloat, _ y: CGFloat) -> CGRect {
        CGRect(x: x, y: y, width: 20, height: 20)
    }

    private func place(_ pin: CGRect, pane: CGSize? = nil) -> CGPoint {
        VisualEditorPlacement.origin(pin: pin, boxSize: box, pane: pane ?? self.pane, inset: 16)
    }

    /// The box at `origin` leaves the pin uncovered.
    private func assertClear(_ origin: CGPoint, _ pin: CGRect, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertFalse(VisualEditorPlacement.covers(origin, box, pin), "the box is over its pin", file: file, line: line)
    }

    func testItFitsBelowThePinCentredOnIt() {
        let at = pin(300, 100)
        let origin = place(at)
        XCTAssertEqual(origin, CGPoint(x: 150, y: 126))
        assertClear(origin, at)
    }

    func testItFlipsAboveWhereBelowWouldLeaveThePane() {
        let at = pin(300, 650)
        let origin = place(at)
        XCTAssertEqual(origin, CGPoint(x: 150, y: 444), "its bottom the gap above the pin's top")
        assertClear(origin, at)
    }

    func testItIsClampedToTheLeftInset() {
        let at = pin(0, 100)
        let origin = place(at)
        XCTAssertEqual(origin, CGPoint(x: 16, y: 126))
        assertClear(origin, at)
    }

    func testItIsClampedToTheRightInset() {
        let at = pin(720, 100)
        let origin = place(at)
        XCTAssertEqual(origin, CGPoint(x: 394, y: 126), "its trailing edge 16 in from the pane's")
        assertClear(origin, at)
    }

    func testWhereNeitherSideFitsItTakesTheRoomierSideClampedIn() {
        // 198 below the pin against 200 needed, none above: below, clamped
        // up into the pane, still clear of the pin.
        let at = pin(300, 20)
        let origin = place(at, pane: CGSize(width: 730, height: 260))
        XCTAssertEqual(origin, CGPoint(x: 150, y: 44))
        assertClear(origin, at)
    }

    func testWhereTheClampWouldCoverThePinItGoesBesideIt() {
        let short = CGSize(width: 730, height: 300)
        let level = pin(300, 140)
        let beside = place(level, pane: short)
        XCTAssertEqual(beside, CGPoint(x: 326, y: 84), "even room either side: below's side, moved right of the pin")
        assertClear(beside, level)

        let low = pin(300, 180)
        let above = place(low, pane: short)
        XCTAssertEqual(above, CGPoint(x: 326, y: 16), "more room above: clamped to the top, right of the pin")
        assertClear(above, low)

        let nearRight = pin(600, 140)
        let left = place(nearRight, pane: short)
        XCTAssertEqual(left, CGPoint(x: 274, y: 84), "no room right of the pin: left of it")
        assertClear(left, nearRight)
    }

    func testWithNoRoomBesideItKeepsToItsSidePastTheEdge() {
        let at = pin(170, 140)
        let origin = place(at, pane: CGSize(width: 360, height: 300))
        XCTAssertEqual(origin, CGPoint(x: 20, y: 166), "below the pin, running off the pane's foot")
        assertClear(origin, at)
    }

    func testAPinScrolledOutHoldsTheBoxAtTheNearestEdge() {
        XCTAssertEqual(place(pin(300, -200)), CGPoint(x: 150, y: 16), "scrolled off the top: the top inset")
        XCTAssertEqual(place(pin(300, 900)), CGPoint(x: 150, y: 544), "scrolled off the foot: the foot inset")
        XCTAssertEqual(place(pin(-400, 100)), CGPoint(x: 16, y: 126), "scrolled off the side: the side inset")
    }

    func testAPaneNarrowerThanTheBoxKeepsItsLeadingEdgeOnScreen() {
        XCTAssertEqual(place(pin(100, 100), pane: CGSize(width: 300, height: 760)).x, 16)
    }
}
