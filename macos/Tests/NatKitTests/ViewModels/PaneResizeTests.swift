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

    /// The frame before any geometry has been read: nothing is over it.
    func testAnUnmeasuredHandleIsNeverUnderThePointer() {
        XCTAssertFalse(paneDragEndedOverHandle(handleFrame: .zero, endLocation: .zero))
    }
}
