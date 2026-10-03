import XCTest
@testable import NatKit

/// The one titlebar band: the run of readout/actions and tabs lives in the
/// main pane's part alone, anchored right; the identity takes what is left.
final class TitlebarBandLayoutTests: XCTestCase {
    func testTheMainPartIsTheBandLessTheNavigatorsWidth() {
        let layout = TitlebarBandLayout(bandWidth: 1060, navigatorWidth: 330, runWidth: 200)
        XCTAssertEqual(layout.mainWidth, 730)
    }

    func testARunThatFitsIsShownWholeAgainstTheTrailingEdge() {
        let layout = TitlebarBandLayout(bandWidth: 1060, navigatorWidth: 330, runWidth: 200)
        XCTAssertEqual(layout.runShownWidth, 200)
        XCTAssertEqual(layout.runX, 860)
    }

    func testTheIdentityRunsPastTheNavigatorUpToTheRun() {
        let layout = TitlebarBandLayout(bandWidth: 1060, navigatorWidth: 330, runWidth: 200)
        XCTAssertEqual(layout.identityWidth, 860)
        XCTAssertGreaterThan(layout.identityWidth, 330)
    }

    func testARunWiderThanTheMainPartIsCutAtTheSplitNeverCrossingIt() {
        let layout = TitlebarBandLayout(bandWidth: 500, navigatorWidth: 330, runWidth: 300)
        XCTAssertEqual(layout.runShownWidth, 170)
        XCTAssertEqual(layout.runX, 330)
        XCTAssertEqual(layout.identityWidth, 330)
    }

    func testANavigatorWiderThanTheBandLeavesNoMainPartAndNoRun() {
        let layout = TitlebarBandLayout(bandWidth: 300, navigatorWidth: 330, runWidth: 120)
        XCTAssertEqual(layout.mainWidth, 0)
        XCTAssertEqual(layout.runShownWidth, 0)
        XCTAssertEqual(layout.runX, 300)
        XCTAssertEqual(layout.identityWidth, 300)
    }

    func testNoRunLeavesTheIdentityTheWholeBand() {
        let layout = TitlebarBandLayout(bandWidth: 1060, navigatorWidth: 330, runWidth: 0)
        XCTAssertEqual(layout.identityWidth, 1060)
    }

    func testANegativeBandOrRunReadsAsNone() {
        let layout = TitlebarBandLayout(bandWidth: -10, navigatorWidth: 330, runWidth: -5)
        XCTAssertEqual(layout, TitlebarBandLayout(bandWidth: 0, navigatorWidth: 330, runWidth: 0))
        XCTAssertEqual(layout.identityWidth, 0)
    }
}
