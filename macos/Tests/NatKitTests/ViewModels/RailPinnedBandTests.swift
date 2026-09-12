import XCTest
@testable import NatKit

/// The rail's pinned band: as tall as what it holds, and no taller than its
/// share of the rail.
final class RailPinnedBandTests: XCTestCase {
    func testAQuietBandIsTheHeightOfItsContent() {
        XCTAssertEqual(RailPinnedBand.height(content: 120, rail: 800), 120)
    }

    func testAFullBandStopsAtItsShareOfTheRail() {
        XCTAssertEqual(RailPinnedBand.height(content: 700, rail: 800), 400)
    }

    /// Exactly at the cap is the last height that is still all of the
    /// content, so it is the band drawn whole rather than the band clipped.
    func testTheCapItselfIsTheWholeContent() {
        XCTAssertEqual(RailPinnedBand.height(content: 400, rail: 800), 400)
        XCTAssertFalse(RailPinnedBand.scrolls(content: 400, rail: 800))
    }

    /// The first layout pass, before either number has landed: a band given
    /// nothing to show is no band at all, and one on a rail nobody has
    /// measured yet draws what it holds rather than nothing.
    func testTheUnmeasuredCases() {
        XCTAssertEqual(RailPinnedBand.height(content: 0, rail: 800), 0)
        XCTAssertEqual(RailPinnedBand.height(content: 0, rail: 0), 0)
        XCTAssertEqual(RailPinnedBand.height(content: 120, rail: 0), 120)
        XCTAssertFalse(RailPinnedBand.scrolls(content: 120, rail: 0))
    }

    func testItScrollsOnlyOncePastTheCap() {
        XCTAssertTrue(RailPinnedBand.scrolls(content: 700, rail: 800))
        XCTAssertFalse(RailPinnedBand.scrolls(content: 120, rail: 800))
        // A hair over the cap is layout's own rounding, not content to
        // scroll to.
        XCTAssertFalse(RailPinnedBand.scrolls(content: 400.2, rail: 800))
    }

    /// Half is the share, and it is said in one place rather than typed into
    /// the view beside it.
    func testTheShareIsHalfTheRail() {
        XCTAssertEqual(RailPinnedBand.maxShare, 0.5)
    }
}
