import XCTest
@testable import NatKit

/// The rail's height, shared between the sections that are open.
final class RailSectionLayoutTests: XCTestCase {
    func testQuietSectionsEachKeepTheirOwnHeight() {
        XCTAssertEqual(
            RailSectionLayout.heights(open: [100, 120, 80], available: 800),
            [100, 120, 80]
        )
    }

    /// Three sections all asking for more than the rail has get an even
    /// share each, and none of them can push another off it.
    func testAnOverfullRailIsSharedEvenly() {
        XCTAssertEqual(
            RailSectionLayout.heights(open: [900, 900, 900], available: 600),
            [200, 200, 200]
        )
    }

    /// The point of the water-filling: what a modest section does not want
    /// goes to the section that does, rather than sitting empty beside it.
    func testWhatAModestSectionDoesNotWantGoesToTheGreedyOne() {
        XCTAssertEqual(
            RailSectionLayout.heights(open: [1000, 50, 1000], available: 600),
            [275, 50, 275]
        )
    }

    /// And it goes round again: the second modest section's slack is shared
    /// out too, rather than being handed to whoever was measured first.
    func testTheSlackIsSharedOutAgainEachRound() {
        XCTAssertEqual(
            RailSectionLayout.heights(open: [40, 1000, 20], available: 600),
            [40, 540, 20]
        )
    }

    /// A collapsed section is not passed in at all, so the rest divide the
    /// whole rail between them — the space it was taking comes back by the
    /// same arithmetic that shared it out.
    func testACollapsedSectionYieldsItsSpace() {
        let allOpen = RailSectionLayout.heights(open: [900, 900, 900], available: 600)
        let oneFolded = RailSectionLayout.heights(open: [900, 900], available: 600)
        XCTAssertEqual(allOpen, [200, 200, 200])
        XCTAssertEqual(oneFolded, [300, 300])
    }

    /// One open section takes the whole rail: there is nobody to share with.
    func testASingleSectionTakesWhatThereIs() {
        XCTAssertEqual(RailSectionLayout.heights(open: [2000], available: 600), [600])
        XCTAssertEqual(RailSectionLayout.heights(open: [120], available: 600), [120])
    }

    /// The first layout pass, before either number has landed.
    func testTheUnmeasuredCases() {
        XCTAssertEqual(RailSectionLayout.heights(open: [], available: 800), [])
        XCTAssertEqual(RailSectionLayout.heights(open: [120, 80], available: 0), [120, 80])
        XCTAssertEqual(RailSectionLayout.heights(open: [120, 80], available: -40), [120, 80])
        XCTAssertEqual(RailSectionLayout.heights(open: [0, 0], available: 600), [0, 0])
    }

    /// A height measured as negative is nothing to draw rather than
    /// something to subtract from the rail.
    func testANegativeContentIsNoContent() {
        XCTAssertEqual(RailSectionLayout.heights(open: [-10, 100], available: 600), [0, 100])
    }

    /// Exactly filling the rail is every section drawn whole rather than
    /// every section clipped.
    func testFillingTheRailExactlyIsNoScroll() {
        XCTAssertEqual(
            RailSectionLayout.heights(open: [200, 400], available: 600),
            [200, 400]
        )
        XCTAssertFalse(RailSectionLayout.scrolls(content: 400, height: 400))
    }

    func testItScrollsOnlyOncePastItsShare() {
        XCTAssertTrue(RailSectionLayout.scrolls(content: 900, height: 200))
        XCTAssertFalse(RailSectionLayout.scrolls(content: 120, height: 120))
        // A hair over the share is layout's own rounding, not content to
        // scroll to.
        XCTAssertFalse(RailSectionLayout.scrolls(content: 200.2, height: 200))
    }

    /// The foot room is said in one place rather than typed into the view.
    func testTheFootRoomIsShared() {
        XCTAssertEqual(RailSectionLayout.footRoom, 16)
    }

    /// The three sections, their labels and the icons their headings wear —
    /// one value, so nothing has to be spelled out twice.
    func testEverySectionNamesItselfAndItsIcon() {
        XCTAssertEqual(RailSection.allCases, [.active, .todo, .done])
        XCTAssertEqual(RailSection.allCases.map(\.title), ["ACTIVE", "TODO", "DONE"])
        XCTAssertEqual(RailSection.allCases.map(\.icon), ["bolt", "list.bullet", "checkmark.circle"])
    }
}
