import XCTest
@testable import NatKit
@testable import NatFixtures

/// `AppModel`'s every-project-at-once surface: what the sidebar draws, and
/// selecting across projects.
@MainActor
final class AppModelSidebarTests: XCTestCase {
    private let secondSliceID = "f1x75333-0000-4000-8000-000000000001"

    /// Every project's plan is read at start, the background ones off the
    /// main path; this waits for the second project's to land.
    private func startedTwoProjectModel(agents: [AgentStatus] = Fixtures.agentStatuses) async -> AppModel {
        let model = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: agents), config: Fixtures.twoProjectConfig)
        for _ in 0..<200 where model.plan(projectID: Fixtures.secondProjectID) == nil {
            await Task.yield()
        }
        return model
    }

    func testTheSidebarDrawsEveryOpenProject() async {
        let model = await startedTwoProjectModel()
        let inputs = model.sidebarInputs
        XCTAssertEqual(inputs.map(\.id), [Fixtures.projectID, Fixtures.secondProjectID])
        XCTAssertEqual(inputs.map(\.kind), [.project, .project])
        XCTAssertEqual(inputs[1].plan, Fixtures.secondProjectInfo)

        let sidebar = model.sidebarModel
        XCTAssertTrue(sidebar.projects[1].contains(sliceID: secondSliceID))
        XCTAssertTrue(sidebar.active.contains { $0.targetID == secondSliceID && $0.projectName == "gnat" })
    }

    func testAnUntitledAndAScratchProjectAreTheirOwnKinds() async {
        let model = await Fixtures.startedAppModel(config: Fixtures.scratchConfig)
        model.openUntitledTab()
        let kinds = model.sidebarInputs.map(\.kind)
        XCTAssertEqual(kinds.first, .scratch)
        XCTAssertEqual(kinds.last, .untitled)
        XCTAssertFalse(model.sidebarInputs.last?.isLoading ?? true, "an Untitled row has nothing to load")
    }

    func testSelectingASliceOfAnotherProjectActivatesIt() async {
        let model = await startedTwoProjectModel()
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        await model.selectSlice(secondSliceID, inProject: Fixtures.secondProjectID)
        XCTAssertEqual(model.activeProjectID, Fixtures.secondProjectID)
        XCTAssertEqual(model.selectedSliceID, secondSliceID)

        await model.selectSlice(Fixtures.mergeBoxSliceID, inProject: Fixtures.projectID)
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        XCTAssertEqual(model.selectedSliceID, Fixtures.mergeBoxSliceID)
    }

    /// A click lands the moment it is made, not once the project it
    /// activates has re-read its plan: a second click made in the meantime
    /// is the one left selected, not overwritten by the first finishing.
    func testASecondClickDuringAnActivationIsTheOneThatSticks() async {
        let model = await startedTwoProjectModel()
        let thirdSliceID = "f1x75333-0000-4000-8000-000000000003"

        let first = Task { await model.selectSlice(secondSliceID, inProject: Fixtures.secondProjectID) }
        await Task.yield()
        XCTAssertEqual(model.activeProjectID, Fixtures.secondProjectID)
        XCTAssertEqual(model.selectedSliceID, secondSliceID, "selected before the activation finishes")

        await model.selectSlice(thirdSliceID, inProject: Fixtures.secondProjectID)
        await first.value
        XCTAssertEqual(model.selectedSliceID, thirdSliceID)
    }

    /// An activation overtaken by a click in another project never writes
    /// its slice into that project's selection when it finishes.
    func testAnOvertakenActivationLeavesTheNewSelectionAlone() async {
        let model = await startedTwoProjectModel()
        let first = Task { await model.selectSlice(secondSliceID, inProject: Fixtures.secondProjectID) }
        await Task.yield()
        await model.selectSlice(Fixtures.mergeBoxSliceID, inProject: Fixtures.projectID)
        await first.value
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        XCTAssertEqual(model.selectedSliceID, Fixtures.mergeBoxSliceID)
    }

    func testSelectingASessionOrTheWorkshopActivatesItsProject() async {
        let model = await startedTwoProjectModel()
        await model.selectWorkshop(inProject: Fixtures.secondProjectID)
        XCTAssertEqual(model.activeProjectID, Fixtures.secondProjectID)
        XCTAssertTrue(model.workshopSelected)

        await model.selectSession(Fixtures.liveSessionID, inProject: Fixtures.projectID)
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        XCTAssertEqual(model.selectedSessionID, Fixtures.liveSessionID)
        XCTAssertFalse(model.workshopSelected)

        // Already active: nothing to switch, the selection is all it is.
        await model.selectWorkshop(inProject: Fixtures.projectID)
        XCTAssertTrue(model.workshopSelected)
        await model.selectSession(Fixtures.liveSessionID, inProject: Fixtures.projectID)
        XCTAssertEqual(model.selectedSessionID, Fixtures.liveSessionID)
    }

    func testEachProjectsPlanningAgentIsItsOwn() async {
        let planner = AgentStatus(
            sliceID: TmuxSession.planTag(projectID: Fixtures.secondProjectID),
            session: "nat-plan-2", activity: .waiting)
        let model = await startedTwoProjectModel(agents: [planner])
        for _ in 0..<200 where model.activityStore?.agents.isEmpty != false {
            try? await Task.sleep(nanoseconds: 5_000_000)
        }
        XCTAssertEqual(model.planningAgents.keys.sorted(), [Fixtures.secondProjectID])
        XCTAssertEqual(model.planningAgents[Fixtures.secondProjectID], .waiting)
        XCTAssertTrue(model.sidebarModel.active.contains { $0.kind == .workshop && $0.projectID == Fixtures.secondProjectID })
    }

    func testARefreshRereadsTheOtherProjectsToo() async {
        let model = await startedTwoProjectModel()
        await model.refresh()
        for _ in 0..<200 where model.plan(projectID: Fixtures.secondProjectID) == nil {
            await Task.yield()
        }
        XCTAssertEqual(model.plan(projectID: Fixtures.secondProjectID), Fixtures.secondProjectInfo)
        XCTAssertNil(model.plan(projectID: "nope"))
    }
}
