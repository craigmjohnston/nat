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

    /// A line stands in for the ink of a run of type rather than for the row
    /// it is set on, so it is drawn at about the cap height of it.
    func testALineIsDrawnAtTheCapHeightOfTheTypeItStandsInFor() {
        XCTAssertEqual(SkeletonLayout.lineThickness(forTextOf: 14), 9)
        XCTAssertEqual(SkeletonLayout.lineThickness(forTextOf: 13), 8)
        XCTAssertEqual(SkeletonLayout.lineThickness(forTextOf: 12), 7)
    }

    /// Whole points: a block drawn on a half point is a blurred one.
    func testAThicknessIsAWholeNumberOfPoints() {
        for size in stride(from: CGFloat(8), through: 24, by: 1) {
            let thickness = SkeletonLayout.lineThickness(forTextOf: size)
            XCTAssertEqual(thickness, thickness.rounded(), "\(size)")
        }
    }

    /// Bigger type inks a thicker line, all the way up the ramp.
    func testBiggerTypeInksAThickerLine() {
        XCTAssertGreaterThan(
            SkeletonLayout.lineThickness(forTextOf: 15),
            SkeletonLayout.lineThickness(forTextOf: 11)
        )
    }
}
