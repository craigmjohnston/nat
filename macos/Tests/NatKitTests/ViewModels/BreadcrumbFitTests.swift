import XCTest
@testable import NatKit

/// The breadcrumb gives way name first (to 80%), then the project to its
/// tag, then the milestone (to 50%), then to the Active row's line alone.
final class BreadcrumbFitTests: XCTestCase {
    private let project = CrumbWidth(group: 150, text: 120)
    private let tag = CrumbWidth(group: 50, text: 30)
    private let milestone = CrumbWidth(group: 140, text: 110)
    // Whole, the row is 150 + 140 + 220 + two gaps of 10 = 530; the name's
    // floor is 20 + 80% of 200 = 180.
    private let title = CrumbWidth(group: 220, text: 200)

    private func fit(
        _ available: Double, project: CrumbWidth?? = nil, tag: CrumbWidth?? = nil, parent: CrumbWidth?? = nil
    ) -> BreadcrumbFit {
        BreadcrumbFit(
            available: available, spacing: 10, project: project ?? self.project, projectTag: tag ?? self.tag,
            parent: parent ?? milestone, title: title)
    }

    func testWithRoomEveryCrumbIsWhole() {
        XCTAssertEqual(
            fit(600), BreadcrumbFit(stage: .full, titleWidth: 220, parentWidth: nil, projectAsTag: false))
    }

    func testTheNameGivesWayFirstDownTo80Percent() {
        XCTAssertEqual(
            fit(500), BreadcrumbFit(stage: .full, titleWidth: 190, parentWidth: nil, projectAsTag: false))
        XCTAssertEqual(fit(490).stage, .full, "exactly at the name's floor")
    }

    func testPastThatTheProjectTurnsToItsTagAndTheNameGetsItsRoomBack() {
        XCTAssertEqual(
            fit(480), BreadcrumbFit(stage: .projectTag, titleWidth: 220, parentWidth: nil, projectAsTag: true))
        XCTAssertEqual(fit(390).titleWidth, 180, "the tag's row at the name's floor")
    }

    func testThenTheMilestoneShortensDownToHalfTheNameHeldAtItsFloor() {
        XCTAssertEqual(
            fit(380),
            BreadcrumbFit(stage: .parentShortened, titleWidth: 180, parentWidth: 130, projectAsTag: true))
        // 50 + 85 + 180 + two gaps: the milestone at exactly half.
        XCTAssertEqual(fit(335).stage, .parentShortened)
    }

    func testPastEveryFloorTheBreadcrumbGoesForTheActiveRowsLine() {
        XCTAssertEqual(
            fit(300), BreadcrumbFit(stage: .minimal, titleWidth: nil, parentWidth: nil, projectAsTag: false))
    }

    func testAProjectWithNoTagKeepsItsNameAndShortensTheMilestone() {
        XCTAssertEqual(
            fit(480, tag: .some(nil)),
            BreadcrumbFit(stage: .parentShortened, titleWidth: 180, parentWidth: 130, projectAsTag: false))
    }

    func testWithNoMilestoneTheTagIsTheLastStepBeforeTheActiveLine() {
        // A workshop's or session's band: the project's name, then the name.
        XCTAssertEqual(fit(400, parent: .some(nil)).stage, .full)
        XCTAssertEqual(fit(300, parent: .some(nil)).stage, .projectTag)
        XCTAssertEqual(fit(200, parent: .some(nil)).stage, .minimal)
    }

    func testATagWithNoProjectCrumbIsNeverDrawn() {
        // A source task's band: its container, then the name — no project
        // crumb to turn into a tag.
        // 140 + 180 + a gap is past 300, so the container shortens to 110.
        let fit = fit(300, project: .some(nil))
        XCTAssertEqual(fit, BreadcrumbFit(stage: .parentShortened, titleWidth: 180, parentWidth: 110, projectAsTag: false))
    }

    func testANameAloneShortensToItsFloorThenGoesToTheActiveLine() {
        XCTAssertEqual(fit(200, project: .some(nil), parent: .some(nil)).titleWidth, 200)
        XCTAssertEqual(fit(170, project: .some(nil), parent: .some(nil)).stage, .minimal)
    }
}
