import XCTest
@testable import NatKit

/// The one titlebar band: the run of tabs lives in the main pane's part
/// alone, anchored right; the breadcrumb takes what is left.
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

    func testTheTrailingItemTakesItsWidthBeforeTheTabs() {
        let layout = TitlebarBandLayout(bandWidth: 1060, navigatorWidth: 330, runWidth: 200, trailingWidth: 90)
        XCTAssertEqual(layout.trailingShownWidth, 90)
        XCTAssertEqual(layout.trailingX, 970)
        XCTAssertEqual(layout.runShownWidth, 200)
        XCTAssertEqual(layout.runX, 770)
        XCTAssertEqual(layout.identityWidth, 770)
    }

    func testTabsShareWhatTheTrailingItemLeavesNeverCrossingTheSplit() {
        let layout = TitlebarBandLayout(bandWidth: 500, navigatorWidth: 330, runWidth: 200, trailingWidth: 90)
        XCTAssertEqual(layout.trailingShownWidth, 90)
        XCTAssertEqual(layout.runShownWidth, 80)
        XCTAssertEqual(layout.runX, 330)
        XCTAssertEqual(layout.identityWidth, 330)
    }

    func testATrailingItemWiderThanTheMainPartIsCutAndLeavesTheTabsNothing() {
        let layout = TitlebarBandLayout(bandWidth: 400, navigatorWidth: 330, runWidth: 200, trailingWidth: 90)
        XCTAssertEqual(layout.trailingShownWidth, 70)
        XCTAssertEqual(layout.trailingX, 330)
        XCTAssertEqual(layout.runShownWidth, 0)
        XCTAssertEqual(layout.runX, 330)
    }

    func testTabsFillFromTheRightTheFirstRightmost() {
        let tabs = NavigatorModel(
            slice: Slice(
                id: "s", name: "S", status: "In progress", milestoneID: "M", assignee: "",
                pr: "https://github.com/o/r/pull/1", url: "", branch: "b", blocked: false, handedBack: true),
            agent: .working, hasVisuals: true
        ).tabs
        XCTAssertEqual(tabs, [.terminal, .changes, .visuals, .pr])
        XCTAssertEqual(TitlebarBandLayout.leftToRight(tabs), [.pr, .visuals, .changes, .terminal])
        XCTAssertEqual(
            TitlebarBandLayout.leftToRight(MainPaneTab.forSession(hasPRs: true)), [.pr, .changes, .terminal])
        XCTAssertEqual(
            TitlebarBandLayout.leftToRight(WorkshopTab.available(launched: true, hasProposal: true)),
            [.plan, .terminal])
    }

    func testANegativeBandOrRunReadsAsNone() {
        let layout = TitlebarBandLayout(bandWidth: -10, navigatorWidth: 330, runWidth: -5)
        XCTAssertEqual(layout, TitlebarBandLayout(bandWidth: 0, navigatorWidth: 330, runWidth: 0))
        XCTAssertEqual(layout.identityWidth, 0)
    }
}
