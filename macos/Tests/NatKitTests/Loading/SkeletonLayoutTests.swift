import CoreGraphics
import XCTest
@testable import NatKit

final class SkeletonLayoutTests: XCTestCase {
    func testALineTakesItsFractionOfTheRoomItIsGiven() {
        XCTAssertEqual(SkeletonLayout.lineWidth(0.5, in: 400), 200)
    }

    /// A block wider than its parent pushes the layout wider than the pane,
    /// which is the very reflow a skeleton is there to prevent.
    func testALineNeverRunsPastTheRoomItIsGiven() {
        XCTAssertEqual(SkeletonLayout.lineWidth(1.5, in: 300), 300)
    }

    func testAVeryNarrowPaneStillShowsABlock() {
        XCTAssertEqual(SkeletonLayout.lineWidth(0.01, in: 400), SkeletonLayout.minimumLineWidth)
    }

    /// The floor gives way to the cap rather than the other way round: a pane
    /// narrower than the floor itself has to draw inside it.
    func testAPaneNarrowerThanTheFloorDrawsInsideItAnyway() {
        XCTAssertEqual(SkeletonLayout.lineWidth(0.5, in: 10), 10)
    }

    /// The first pass of a layout is where a `GeometryReader` reports zero.
    func testAParentThatHasLeftNoRoomDrawsNothing() {
        XCTAssertEqual(SkeletonLayout.lineWidth(0.5, in: 0), 0)
        XCTAssertEqual(SkeletonLayout.lineWidth(0.5, in: -20), 0)
    }

    func testAParagraphIsItsLinesAndTheSpacingBetweenThem() {
        XCTAssertEqual(SkeletonLayout.paragraphHeight(3, height: 10, spacing: 8), 46)
    }

    func testOneLineHasNoSpacingUnderIt() {
        XCTAssertEqual(SkeletonLayout.paragraphHeight(1, height: 10, spacing: 8), 10)
    }

    func testNoLinesTakeNoHeightAtAll() {
        XCTAssertEqual(SkeletonLayout.paragraphHeight(0, height: 10, spacing: 8), 0)
    }
}
