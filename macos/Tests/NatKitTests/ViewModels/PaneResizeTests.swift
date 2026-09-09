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
}
