import XCTest
@testable import NatKit

final class RightClickTraceTests: XCTestCase {
    func testAClickNoMenuFollowedSettlesAsWaiting() {
        var trace = RightClickTrace()
        let click = trace.clicked()
        XCTAssertEqual(trace.settle(click), true)
    }

    func testAClickAMenuFollowedSettlesAsAnswered() {
        var trace = RightClickTrace()
        let click = trace.clicked()
        trace.menuBegan()
        XCTAssertEqual(trace.settle(click), false)
    }

    func testASupersededClickSaysNothing() {
        var trace = RightClickTrace()
        let first = trace.clicked()
        let second = trace.clicked()
        XCTAssertNil(trace.settle(first))
        XCTAssertEqual(trace.settle(second), true)
    }

    /// A menu that began before a click (a menu-bar menu, say) does not
    /// count as that click's.
    func testAMenuBeforeTheClickDoesNotAnswerIt() {
        var trace = RightClickTrace()
        trace.menuBegan()
        let click = trace.clicked()
        XCTAssertEqual(trace.settle(click), true)
    }

    func testAClickSettlesOnce() {
        var trace = RightClickTrace()
        let click = trace.clicked()
        XCTAssertEqual(trace.settle(click), true)
        XCTAssertEqual(trace.settle(click), false)
    }

    func testDiagnosisOfAClickThatMissedSwiftUI() {
        let diagnosis = RightClickDiagnosis(hit: "NSVisualEffectView", insideHostingView: false, menuItems: nil)
        XCTAssertEqual(diagnosis, .notSwiftUI(hit: "NSVisualEffectView"))
        XCTAssertEqual(diagnosis.summary, "not-swiftui: the click was hit-tested to NSVisualEffectView, not SwiftUI's hosting view")
    }

    func testDiagnosisOfAClickThatHitNothing() {
        XCTAssertEqual(RightClickDiagnosis(hit: nil, insideHostingView: false, menuItems: ["x"]), .notSwiftUI(hit: "nothing"))
    }

    func testDiagnosisOfAHostingViewWithNoMenu() {
        let diagnosis = RightClickDiagnosis(hit: "NSHostingView<Root>", insideHostingView: true, menuItems: nil)
        XCTAssertEqual(diagnosis, .noMenuFromSwiftUI)
        XCTAssertEqual(diagnosis.summary, "no-menu-from-swiftui: the hosting view's menu(for:) answered nil")
    }

    func testDiagnosisOfAMenuNeverPresented() {
        let diagnosis = RightClickDiagnosis(hit: "NSHostingView<Root>", insideHostingView: true, menuItems: ["Launch", "Edit"])
        XCTAssertEqual(diagnosis, .menuNotPresented(items: ["Launch", "Edit"]))
        XCTAssertEqual(diagnosis.summary, "menu-not-presented: menu(for:) answered [\"Launch\", \"Edit\"] but no menu began tracking")
    }
}
