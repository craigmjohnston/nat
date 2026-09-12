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

    /// What there is to share is the container's height less the chrome and
    /// less the air under the last section.
    func testWhatIsLeftToShareIsTheRailLessItsChromeAndItsFootRoom() {
        XCTAssertEqual(RailSectionLayout.available(rail: 840, chrome: 124), 700)
        XCTAssertEqual(RailSectionLayout.available(rail: 840, chrome: 0), 824)
    }

    /// A rail nobody has measured yet has nothing to share, which is exactly
    /// what `heights` reads as "not measured": every section draws what it
    /// holds rather than nothing at all.
    func testAnUnmeasuredRailHasNothingToShare() {
        let available = RailSectionLayout.available(rail: 0, chrome: 0)
        XCTAssertLessThanOrEqual(available, 0)
        XCTAssertEqual(
            RailSectionLayout.heights(open: [900, 50], available: available),
            [900, 50]
        )
    }

    /// The regression the container measurement ends: the rail read off the
    /// column it produces rather than off what the shell offers. Feeding the
    /// first pass's own answer back in as the rail confirms it forever — the
    /// sections keep their whole content, nothing scrolls, and the column
    /// stays taller than the window. The container's height is a fixed point
    /// of the same loop: share it out, and what comes back to share next
    /// time is the same number.
    func testTheShareIsStableOnlyWhenTheRailIsTheContainers() {
        let contents = [900.0, 50.0]
        let chrome = 100.0
        let window = 600.0

        // The column's own height, read off the first pass and fed back in.
        var rail = 0.0
        for _ in 0..<3 {
            let shares = RailSectionLayout.heights(
                open: contents, available: RailSectionLayout.available(rail: rail, chrome: chrome)
            )
            rail = shares.reduce(0, +) + chrome + RailSectionLayout.footRoom
        }
        XCTAssertGreaterThan(rail, window)
        XCTAssertEqual(
            RailSectionLayout.heights(
                open: contents, available: RailSectionLayout.available(rail: rail, chrome: chrome)
            ),
            contents
        )

        // The container's height: settled on the first pass, and the same
        // answer every pass after it.
        let shares = RailSectionLayout.heights(
            open: contents, available: RailSectionLayout.available(rail: window, chrome: chrome)
        )
        XCTAssertEqual(shares, [434, 50])
        XCTAssertLessThanOrEqual(shares.reduce(0, +) + chrome + RailSectionLayout.footRoom, window)
        XCTAssertTrue(RailSectionLayout.scrolls(content: contents[0], height: shares[0]))
        XCTAssertEqual(
            RailSectionLayout.heights(
                open: contents, available: RailSectionLayout.available(rail: window, chrome: chrome)
            ),
            shares
        )
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
