import XCTest
@testable import NatKit

/// The breadcrumb gives way name first (to 80%), then the milestone (to
/// 50%), then to the Active row's line alone; the project's badge never
/// gives way on its own.
final class BreadcrumbFitTests: XCTestCase {
    private let project = 50.0
    private let milestone = CrumbWidth(group: 140, text: 110)
    // Whole, the row is 50 + 140 + 220 + two gaps of 10 = 430; the name's
    // floor is 20 + 80% of 200 = 180.
    private let title = CrumbWidth(group: 220, text: 200)

    private func fit(_ available: Double, project: Double?? = nil, parent: CrumbWidth?? = nil) -> BreadcrumbFit {
        BreadcrumbFit(
            available: available, spacing: 10, project: project ?? self.project, parent: parent ?? milestone,
            title: title)
    }

    func testWithRoomEveryCrumbIsWhole() {
        XCTAssertEqual(fit(500), BreadcrumbFit(stage: .full, titleWidth: 220, parentWidth: nil))
    }

    func testTheNameGivesWayFirstDownTo80Percent() {
        XCTAssertEqual(fit(420), BreadcrumbFit(stage: .full, titleWidth: 210, parentWidth: nil))
        XCTAssertEqual(fit(390), BreadcrumbFit(stage: .full, titleWidth: 180, parentWidth: nil), "exactly at the name's floor")
    }

    func testThenTheMilestoneShortensDownToHalfTheNameHeldAtItsFloor() {
        XCTAssertEqual(fit(380), BreadcrumbFit(stage: .parentShortened, titleWidth: 180, parentWidth: 130))
        // 50 + 85 + 180 + two gaps: the milestone at exactly half.
        XCTAssertEqual(fit(335).stage, .parentShortened)
    }

    func testPastEveryFloorTheBreadcrumbGoesForTheActiveRowsLine() {
        XCTAssertEqual(fit(330), BreadcrumbFit(stage: .minimal, titleWidth: nil, parentWidth: nil))
    }

    func testWithNoMilestoneTheNameIsTheLastStepBeforeTheActiveLine() {
        // A workshop's or session's band: the project's badge, then the name.
        XCTAssertEqual(fit(300, parent: .some(nil)), BreadcrumbFit(stage: .full, titleWidth: 220, parentWidth: nil))
        XCTAssertEqual(fit(240, parent: .some(nil)).stage, .full)
        XCTAssertEqual(fit(200, parent: .some(nil)).stage, .minimal)
    }

    func testWithNoProjectCrumbTheContainerShortens() {
        // A source task's band: its container, then the name — no project
        // crumb. 140 + 180 + a gap is past 300, so the container shortens
        // to 110.
        XCTAssertEqual(
            fit(300, project: .some(nil)), BreadcrumbFit(stage: .parentShortened, titleWidth: 180, parentWidth: 110))
    }

    func testANameAloneShortensToItsFloorThenGoesToTheActiveLine() {
        XCTAssertEqual(fit(200, project: .some(nil), parent: .some(nil)).titleWidth, 200)
        XCTAssertEqual(fit(170, project: .some(nil), parent: .some(nil)).stage, .minimal)
    }
}
