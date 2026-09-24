import XCTest
@testable import NatKit

final class DiffScrollAnchorTests: XCTestCase {
    private typealias Key = DiffScrollAnchor.Key
    private let a = Key(path: "a.go", rowID: "1")
    private let b = Key(path: "a.go", rowID: "2")
    private let c = Key(path: "b.go", rowID: "1")

    func testKeepsTheTopmostRowStillOnScreen() {
        var anchor = DiffScrollAnchor()
        anchor.update(rows: [(b, 20, 40), (a, -10, 10), (c, 40, 60)])
        XCTAssertEqual(anchor.key, a)
    }

    func testARowWhollyAboveTheTopIsNotOnScreen() {
        var anchor = DiffScrollAnchor()
        anchor.update(rows: [(a, -30, 0), (b, 0, 20)])
        XCTAssertEqual(anchor.key, b)
    }

    func testNoRowsClearsTheAnchor() {
        var anchor = DiffScrollAnchor()
        anchor.update(rows: [(a, 0, 20)])
        anchor.update(rows: [])
        XCTAssertNil(anchor.key)
    }

    func testScrollIDDiffersAcrossFilesAndFromABarePath() {
        XCTAssertNotEqual(a.scrollID, c.scrollID)
        XCTAssertNotEqual(a.scrollID, "a.go")
        XCTAssertEqual(Key(path: "a.go", rowID: "1").scrollID, a.scrollID)
    }

    func testRestoreFreezesTheAnchorUntilEnded() {
        var anchor = DiffScrollAnchor()
        anchor.canScroll = true
        anchor.update(rows: [(a, 0, 20)])

        XCTAssertEqual(anchor.beginRestore(), a)
        XCTAssertTrue(anchor.isRestoring)

        anchor.update(rows: [(c, 0, 20)])
        XCTAssertEqual(anchor.key, a)

        anchor.endRestore()
        anchor.update(rows: [(c, 0, 20)])
        XCTAssertEqual(anchor.key, c)
    }

    func testNothingToRestoreWhenTheDiffCannotScroll() {
        var anchor = DiffScrollAnchor()
        anchor.update(rows: [(a, 0, 20)])
        XCTAssertNil(anchor.beginRestore())
        XCTAssertFalse(anchor.isRestoring)
    }

    func testNothingToRestoreWithoutAnAnchor() {
        var anchor = DiffScrollAnchor()
        anchor.canScroll = true
        XCTAssertNil(anchor.beginRestore())
        XCTAssertFalse(anchor.isRestoring)
    }
}
