import XCTest
@testable import NatKit

final class MilestoneMenuRulesTests: XCTestCase {
    private func plan() -> [Milestone] {
        [
            Milestone(id: "M1", name: "M1: Client", order: 0, status: "Done"),
            Milestone(id: "M2", name: "M2: Board", order: 1, status: "Active"),
            Milestone(id: "M3", name: "M3: Agents", order: 2, status: "Queued")
        ]
    }

    /// A milestone in the middle of the plan moves either way, each named by
    /// the neighbour it would land beside.
    func testActions_middleMilestoneMovesEitherWay() {
        let actions = MilestoneMenuRules.actions(for: "M2: Board", in: plan(), sliceCount: 3)

        XCTAssertEqual(actions.moveBefore, "M1: Client")
        XCTAssertEqual(actions.moveAfter, "M3: Agents")
    }

    /// The first milestone has nothing above it to go before.
    func testActions_firstMilestoneOffersNoMoveUp() {
        let actions = MilestoneMenuRules.actions(for: "M1: Client", in: plan(), sliceCount: 1)

        XCTAssertNil(actions.moveBefore)
        XCTAssertEqual(actions.moveAfter, "M2: Board")
    }

    /// And the last nothing below it to go after.
    func testActions_lastMilestoneOffersNoMoveDown() {
        let actions = MilestoneMenuRules.actions(for: "M3: Agents", in: plan(), sliceCount: 0)

        XCTAssertEqual(actions.moveBefore, "M2: Board")
        XCTAssertNil(actions.moveAfter)
    }

    /// The plan's own order decides the neighbours, whatever order the
    /// milestones arrive in: the rail draws them sorted and a menu that read
    /// them unsorted would move one somewhere other than where the arrow
    /// pointed.
    func testActions_neighboursComeFromPlanOrderNotArrayOrder() {
        let shuffled = [plan()[2], plan()[0], plan()[1]]

        let actions = MilestoneMenuRules.actions(for: "M2: Board", in: shuffled, sliceCount: 0)

        XCTAssertEqual(actions.moveBefore, "M1: Client")
        XCTAssertEqual(actions.moveAfter, "M3: Agents")
    }

    /// Only one there is nothing filed under: `milestone-remove` refuses the
    /// rest, naming the slices.
    func testActions_deleteOnlyWhenEmpty() {
        XCTAssertTrue(MilestoneMenuRules.actions(for: "M2: Board", in: plan(), sliceCount: 0).canDelete)
        XCTAssertFalse(MilestoneMenuRules.actions(for: "M2: Board", in: plan(), sliceCount: 1).canDelete)
    }

    /// A single milestone is both the first and the last: nowhere to move it.
    func testActions_theOnlyMilestoneMovesNowhere() {
        let only = [Milestone(id: "M1", name: "M1: Client", order: 0, status: "Queued")]

        let actions = MilestoneMenuRules.actions(for: "M1: Client", in: only, sliceCount: 0)

        XCTAssertNil(actions.moveBefore)
        XCTAssertNil(actions.moveAfter)
        XCTAssertTrue(actions.canDelete)
    }

    /// A row drawn from a reading the plan has since moved past: no move at
    /// all, since there is nowhere in the plan to place it relative to.
    func testActions_aMilestoneThePlanDoesNotHoldMovesNowhere() {
        let actions = MilestoneMenuRules.actions(for: "M9: Gone", in: plan(), sliceCount: 0)

        XCTAssertNil(actions.moveBefore)
        XCTAssertNil(actions.moveAfter)
        XCTAssertTrue(actions.canDelete)
    }

    func testMilestoneMenuActions_isEquatable() {
        XCTAssertEqual(
            MilestoneMenuActions(moveBefore: "a", moveAfter: "b", canDelete: true),
            MilestoneMenuActions(moveBefore: "a", moveAfter: "b", canDelete: true)
        )
    }
}
