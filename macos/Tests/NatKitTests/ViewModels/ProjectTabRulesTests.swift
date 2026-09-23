import XCTest
@testable import NatKit

final class ProjectTabRulesTests: XCTestCase {
    // MARK: - showsClose

    func testShowsClose_severalTabsAllCarryOne() {
        XCTAssertTrue(ProjectTabRules.showsClose(tabCount: 2))
        XCTAssertTrue(ProjectTabRules.showsClose(tabCount: 7))
    }

    /// The last tab standing carries none: `AppModel.closeProject` refuses it,
    /// so a ✕ there would offer the one thing it cannot do.
    func testShowsClose_theLastTabCarriesNone() {
        XCTAssertFalse(ProjectTabRules.showsClose(tabCount: 1))
    }

    /// A strip drawn before any project has landed has no tab to close.
    func testShowsClose_anEmptyStripCarriesNone() {
        XCTAssertFalse(ProjectTabRules.showsClose(tabCount: 0))
    }

    /// The scratch tab is always there and never closable, whatever the strip.
    func testShowsClose_theScratchTabCarriesNone() {
        XCTAssertFalse(ProjectTabRules.showsClose(tabCount: 5, isScratch: true))
    }

    /// The caller counts every tab but the scratch one, so a strip of scratch
    /// plus one project reads as one tab: the last standing, with no ✕.
    func testShowsClose_aLoneProjectBesideScratchCarriesNone() {
        XCTAssertFalse(ProjectTabRules.showsClose(tabCount: 1, isScratch: false))
        XCTAssertTrue(ProjectTabRules.showsClose(tabCount: 2, isScratch: false))
    }

    // MARK: - closeIsVisible

    /// Browser-fashion: the active tab always offers it.
    func testCloseIsVisible_theActiveTabAlwaysShowsIt() {
        XCTAssertTrue(ProjectTabRules.closeIsVisible(isActive: true, isHovered: false))
        XCTAssertTrue(ProjectTabRules.closeIsVisible(isActive: true, isHovered: true))
    }

    func testCloseIsVisible_anInactiveTabShowsItUnderTheMouse() {
        XCTAssertTrue(ProjectTabRules.closeIsVisible(isActive: false, isHovered: true))
    }

    /// And nowhere else — which is also the answer the hit test takes, so a
    /// ✕ nobody can see closes nothing.
    func testCloseIsVisible_anInactiveTabOffTheMouseHidesIt() {
        XCTAssertFalse(ProjectTabRules.closeIsVisible(isActive: false, isHovered: false))
    }
}
