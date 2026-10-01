import XCTest
@testable import NatKit

final class PaneResizeTests: XCTestCase {
    func testTrailingEdgeGrowsWithARightwardDrag() {
        XCTAssertEqual(
            paneResizedWidth(startWidth: 300, translation: 40, edge: .trailing, minWidth: 200, maxWidth: 500),
            340
        )
    }

    func testTrailingEdgeShrinksWithALeftwardDrag() {
        XCTAssertEqual(
            paneResizedWidth(startWidth: 300, translation: -40, edge: .trailing, minWidth: 200, maxWidth: 500),
            260
        )
    }

    func testLeadingEdgeShrinksWithARightwardDrag() {
        XCTAssertEqual(
            paneResizedWidth(startWidth: 300, translation: 40, edge: .leading, minWidth: 200, maxWidth: 500),
            260
        )
    }

    func testLeadingEdgeGrowsWithALeftwardDrag() {
        XCTAssertEqual(
            paneResizedWidth(startWidth: 300, translation: -40, edge: .leading, minWidth: 200, maxWidth: 500),
            340
        )
    }

    func testClampsAtTheMinimum() {
        XCTAssertEqual(
            paneResizedWidth(startWidth: 300, translation: -5000, edge: .trailing, minWidth: 200, maxWidth: 500),
            200
        )
    }

    func testClampsAtTheMaximum() {
        XCTAssertEqual(
            paneResizedWidth(startWidth: 300, translation: 5000, edge: .trailing, minWidth: 200, maxWidth: 500),
            500
        )
    }

    // MARK: - Heights

    func testBottomEdgeGrowsWithADownwardDrag() {
        XCTAssertEqual(
            paneResizedHeight(startHeight: 200, translation: 30, edge: .bottom, minHeight: 80, maxHeight: 400),
            230
        )
    }

    func testTopEdgeShrinksWithADownwardDrag() {
        XCTAssertEqual(
            paneResizedHeight(startHeight: 200, translation: 30, edge: .top, minHeight: 80, maxHeight: 400),
            170
        )
    }

    func testHeightClampsAtBothBounds() {
        XCTAssertEqual(
            paneResizedHeight(startHeight: 200, translation: -5000, edge: .bottom, minHeight: 80, maxHeight: 400),
            80
        )
        XCTAssertEqual(
            paneResizedHeight(startHeight: 200, translation: 5000, edge: .bottom, minHeight: 80, maxHeight: 400),
            400
        )
    }

    // MARK: - A split's upper pane

    func testSplitKeepsAStoredHeightThatFits() {
        XCTAssertEqual(paneSplitHeight(stored: 200, available: 600, minUpper: 80, minLower: 160), 200)
    }

    func testSplitLeavesTheLowerPaneItsFloor() {
        XCTAssertEqual(paneSplitHeight(stored: 500, available: 600, minUpper: 80, minLower: 160), 440)
    }

    func testSplitNeverDropsBelowTheUpperFloor() {
        XCTAssertEqual(paneSplitHeight(stored: 20, available: 600, minUpper: 80, minLower: 160), 80)
        // Too short for both: the upper pane keeps its floor.
        XCTAssertEqual(paneSplitHeight(stored: 200, available: 200, minUpper: 80, minLower: 160), 80)
    }

    // MARK: - Where a drag ended

    func testADragEndedOverTheHandle() {
        XCTAssertTrue(paneDragEndedOverHandle(
            handleFrame: CGRect(x: 360, y: 40, width: 9, height: 600),
            endLocation: CGPoint(x: 364, y: 300)
        ))
    }

    func testADragEndedPastTheHandle() {
        XCTAssertFalse(paneDragEndedOverHandle(
            handleFrame: CGRect(x: 360, y: 40, width: 9, height: 600),
            endLocation: CGPoint(x: 700, y: 300)
        ))
    }

    /// A drag flung upwards out of the window leaves the pointer level with
    /// the divider and nowhere near it.
    func testADragEndedBesideTheHandle() {
        XCTAssertFalse(paneDragEndedOverHandle(
            handleFrame: CGRect(x: 360, y: 40, width: 9, height: 600),
            endLocation: CGPoint(x: 364, y: 10)
        ))
    }

    func testACommitIsTheLiveWidthOnceItDiffersFromThePersistedOne() {
        XCTAssertEqual(paneCommittedWidth(live: 320, persisted: 300), 320)
    }

    func testNothingIsCommittedWithoutADragOrAtTheSameWidth() {
        XCTAssertNil(paneCommittedWidth(live: nil, persisted: 300))
        XCTAssertNil(paneCommittedWidth(live: 300, persisted: 300))
    }

    /// The frame before any geometry has been read: nothing is over it.
    func testAnUnmeasuredHandleIsNeverUnderThePointer() {
        XCTAssertFalse(paneDragEndedOverHandle(handleFrame: .zero, endLocation: .zero))
    }
}
