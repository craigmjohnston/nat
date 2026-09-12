import XCTest
@testable import NatKit

final class RailSkeletonTests: XCTestCase {
    /// The skeleton is the plan's own shape and nothing else's: TODO's
    /// heading is pinned above it whether or not the plan has landed, so a
    /// heading block here would draw a second one under the real one.
    func testOpensWithTheTreeRatherThanAHeading() {
        XCTAssertEqual(RailSkeleton.rows.first?.kind, .folder)
        XCTAssertEqual(Set(RailSkeletonRow.Kind.allCases), [.folder, .slice])
    }

    func testDrawsFoldersAndSlicesUnderTheSectionsOwnHeading() {
        let kinds = Set(RailSkeleton.rows.map(\.kind))
        XCTAssertTrue(kinds.contains(.folder))
        XCTAssertTrue(kinds.contains(.slice))
    }

    func testHeadingsAndFoldersSitAtTheTreesRootAndSlicesOneLevelIn() {
        for row in RailSkeleton.rows {
            switch row.kind {
            case .folder:
                XCTAssertEqual(row.depth, 0, "a folder is a root of the rail's tree")
            case .slice:
                XCTAssertEqual(row.depth, 1, "a slice is drawn one indent inside its folder")
            }
        }
    }

    func testEverySliceFollowsAFolder() {
        var sawFolder = false
        for row in RailSkeleton.rows {
            if row.kind == .folder { sawFolder = true }
            if row.kind == .slice {
                XCTAssertTrue(sawFolder, "a slice placeholder outside any folder is a shape no plan lands in")
            }
        }
    }

    func testTitleWidthsAreFractionsOfTheRail() {
        for row in RailSkeleton.rows {
            XCTAssertGreaterThan(row.titleWidth, 0)
            XCTAssertLessThan(row.titleWidth, 1)
        }
    }

    /// Fixed rather than rolled: the rail redraws on every hover and every
    /// resize, and widths that changed each time would have the column
    /// twitching all through the load.
    func testTheShapeIsTheSameEveryTimeItIsRead() {
        XCTAssertEqual(RailSkeleton.rows, RailSkeleton.rows)
    }

    func testItSaysItIsLoadingForAnyoneWhoCannotSeeTheBlocks() {
        XCTAssertFalse(RailSkeleton.accessibilityLabel.isEmpty)
    }

    func testRowsCarryWhatTheyWereBuiltWith() {
        let row = RailSkeletonRow(kind: .slice, depth: 1, titleWidth: 0.5)

        XCTAssertEqual(row.kind, .slice)
        XCTAssertEqual(row.depth, 1)
        XCTAssertEqual(row.titleWidth, 0.5)
    }
}
