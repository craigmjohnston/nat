import XCTest
@testable import NatKit

final class RailModelTests: XCTestCase {
    private var testProject: Project!
    private var testMilestones: [Milestone]!
    private var testSlices: [Slice]!

    override func setUp() {
        super.setUp()
        testProject = Project(id: "proj-1", name: "Test Project", conventions: "")

        testMilestones = [
            Milestone(id: "m-1", name: "Foundation", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active"),
            Milestone(id: "m-3", name: "Polish", order: 3, status: "Queued")
        ]

        testSlices = [
            // Foundation milestone - all done
            Slice(
                id: "s-1", name: "Setup", status: "Done", milestoneID: "m-1",
                assignee: "user", pr: "", url: "", blocked: false, handedBack: false
            ),
            // Core milestone
            Slice(
                id: "s-2", name: "Feature A", status: "In progress", milestoneID: "m-2",
                assignee: "user", pr: "", url: "", branch: "feature-a", blocked: false, handedBack: true
            ),
            Slice(
                id: "s-3", name: "Feature B", status: "Todo", milestoneID: "m-2",
                assignee: "", pr: "", url: "", blocked: false, handedBack: false
            ),
            Slice(
                id: "s-4", name: "Blocked Task", status: "Todo", milestoneID: "m-2",
                assignee: "", pr: "", url: "", blocked: true, handedBack: false
            ),
            // Polish milestone
            Slice(
                id: "s-5", name: "Polish Item", status: "Todo", milestoneID: "m-3",
                assignee: "", pr: "", url: "", blocked: false, handedBack: false
            )
        ]
    }

    func testBuildRailModel_emptyProject() {
        let projectInfo = ProjectInfo(project: testProject, milestones: [], slices: [])
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertTrue(model.needsReview.isEmpty)
        XCTAssertTrue(model.active.isEmpty)
        XCTAssertTrue(model.milestoneCards.isEmpty)
        XCTAssertNil(model.doneSummary)
    }

    func testBuildRailModel_reviewSection() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(model.needsReview.count, 1)
        XCTAssertEqual(model.needsReview[0].sliceID, "s-2")
        XCTAssertEqual(model.needsReview[0].name, "Feature A")
    }

    func testBuildRailModel_activeSection() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let liveAgents = ["s-3": AgentActivity.working]
        let model = buildRailModel(from: projectInfo, liveAgents: liveAgents)

        XCTAssertEqual(model.active.count, 1)
        XCTAssertEqual(model.active[0].sliceID, "s-3")
        XCTAssertEqual(model.active[0].name, "Feature B")
        XCTAssertEqual(model.active[0].activity, .working)
        XCTAssertEqual(model.active[0].displayState, "Working")
    }

    func testBuildRailModel_activeSectionWaiting() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let liveAgents = ["s-3": AgentActivity.waiting]
        let model = buildRailModel(from: projectInfo, liveAgents: liveAgents)

        XCTAssertEqual(model.active[0].activity, .waiting)
        XCTAssertEqual(model.active[0].displayState, "Waiting for input")
    }

    func testBuildRailModel_milestoneCards() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        // Should have cards for Core and Polish (not Foundation, which is Done)
        XCTAssertEqual(model.milestoneCards.count, 2)

        // Core milestone
        let coreCard = model.milestoneCards[0]
        XCTAssertEqual(coreCard.title, "Core")
        XCTAssertEqual(coreCard.number, "2")
        XCTAssertEqual(coreCard.done, 0)
        XCTAssertEqual(coreCard.total, 3) // Feature A, Feature B, Blocked Task
        XCTAssertEqual(coreCard.visibleSlices.count, 3)

        // Polish milestone
        let polishCard = model.milestoneCards[1]
        XCTAssertEqual(polishCard.title, "Polish")
        XCTAssertEqual(polishCard.number, "3")
        XCTAssertEqual(polishCard.done, 0)
        XCTAssertEqual(polishCard.total, 1)
    }

    func testBuildRailModel_blockedGlyph() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let coreCard = model.milestoneCards[0]
        let blockedSlice = coreCard.visibleSlices.first { $0.sliceID == "s-4" }

        XCTAssertNotNil(blockedSlice)
        XCTAssertEqual(blockedSlice?.glyph, .blocked)
        XCTAssertTrue(blockedSlice?.isBlocked ?? false)
    }

    func testBuildRailModel_inProgressGlyph() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let coreCard = model.milestoneCards[0]
        let inProgressSlice = coreCard.visibleSlices.first { $0.sliceID == "s-2" }

        XCTAssertNotNil(inProgressSlice)
        XCTAssertEqual(inProgressSlice?.glyph, .inProgress)
    }

    func testBuildRailModel_todoGlyph() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let coreCard = model.milestoneCards[0]
        let todoSlice = coreCard.visibleSlices.first { $0.sliceID == "s-3" }

        XCTAssertNotNil(todoSlice)
        XCTAssertEqual(todoSlice?.glyph, .todo)
    }

    func testBuildRailModel_doneSummary() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertNotNil(model.doneSummary)
        XCTAssertEqual(model.doneSummary?.milestoneCount, 1)
        XCTAssertEqual(model.doneSummary?.sliceCount, 1)
    }

    func testBuildRailModel_noDoneSummary() {
        let allActiveMilestones = [
            Milestone(id: "m-1", name: "Active", order: 1, status: "Active")
        ]
        let projectInfo = ProjectInfo(project: testProject, milestones: allActiveMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertNil(model.doneSummary)
    }

    func testMilestoneCardFraction() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let coreCard = model.milestoneCards[0]
        XCTAssertEqual(coreCard.fraction, 0.0)

        let doneSlice = testSlices[2]
        var updatedSlices = testSlices!
        updatedSlices[2] = Slice(
            id: doneSlice.id, name: doneSlice.name, status: "Done", milestoneID: doneSlice.milestoneID,
            assignee: doneSlice.assignee, pr: doneSlice.pr, url: doneSlice.url,
            branch: doneSlice.branch, repo: doneSlice.repo, dependsOn: doneSlice.dependsOn,
            blocked: doneSlice.blocked, handedBack: doneSlice.handedBack, state: doneSlice.state
        )

        let updatedProjectInfo = ProjectInfo(
            project: testProject,
            milestones: testMilestones,
            slices: updatedSlices
        )
        let updatedModel = buildRailModel(from: updatedProjectInfo, liveAgents: [:])

        let updatedCoreCard = updatedModel.milestoneCards[0]
        XCTAssertEqual(updatedCoreCard.done, 1)
        XCTAssertEqual(updatedCoreCard.fraction, 1.0 / 3.0)
    }
}
