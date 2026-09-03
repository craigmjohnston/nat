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
        XCTAssertNil(model.needsReview[0].stat, "no reviewStats given at all should leave the row statless")
    }

    func testBuildRailModel_reviewSectionCarriesItsStat() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:], reviewStats: ["s-2": "+10 \u{2212}3"])

        XCTAssertEqual(model.needsReview[0].stat, "+10 \u{2212}3")
    }

    func testBuildRailModel_reviewSectionLeavesAnUnfetchedStatNil() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:], reviewStats: ["some-other-slice": "+1 \u{2212}1"])

        XCTAssertNil(model.needsReview[0].stat)
    }

    // MARK: - ACTIVE membership

    // ACTIVE is a status/handedBack/pr rule on the slice's own page, never
    // "has a live tmux session" — these cases pin that down independently of
    // whatever a live agent map says.

    func testBuildRailModel_activeSection_inProgressNoSessionIsReadyToPush() {
        // s-3 is In progress, not handed back, no PR, not blocked, and no
        // live agent names it: a session that ended without pushing
        // anything, which is included and reads "Ready to push".
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(model.active.count, 1)
        XCTAssertEqual(model.active[0].sliceID, "s-3")
        XCTAssertEqual(model.active[0].displayState, "Ready to push")
        XCTAssertEqual(model.active[0].tintRole, .readyToPush)
    }

    func testBuildRailModel_activeSection_inProgressBlockedNoSessionIsBlocked() {
        // s-4 is In progress, blocked on a dependency, and nothing is
        // running on it: included, and reads "Blocked" rather than "Ready to
        // push".
        var slices = testSlices!
        slices[3] = Slice(
            id: "s-4", name: "Blocked Task", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: true, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let entry = model.active.first { $0.sliceID == "s-4" }
        XCTAssertNotNil(entry)
        XCTAssertEqual(entry?.displayState, "Blocked")
        XCTAssertEqual(entry?.tintRole, .blocked)
    }

    func testBuildRailModel_activeSection_doneSliceWithLiveSessionIsExcluded() {
        // s-1 is Done; a live agent still attached to it (an idle session on
        // finished work) must not resurrect it into ACTIVE.
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-1": .working])

        XCTAssertFalse(model.active.contains { $0.sliceID == "s-1" })
    }

    func testBuildRailModel_activeSection_handedBackWithLiveSessionIsExcluded() {
        // s-2 is In progress and handed back (it lives in NEEDS REVIEW). A
        // live agent still on its branch is the review going back to it, not
        // a reason to also draw it in ACTIVE.
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-2": .working])

        XCTAssertFalse(model.active.contains { $0.sliceID == "s-2" })
        XCTAssertEqual(model.needsReview.count, 1)
        XCTAssertEqual(model.needsReview[0].sliceID, "s-2")
    }

    func testBuildRailModel_activeSection_prRecordedIsExcluded() {
        // A slice with a pull request recorded is work already out, however
        // its status reads — not ACTIVE's to draw.
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "https://github.com/example/pr/1", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertFalse(model.active.contains { $0.sliceID == "s-3" })
    }

    func testBuildRailModel_activeSection_liveAgentWorkingWins() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-3": .working])

        XCTAssertEqual(model.active.count, 1)
        XCTAssertEqual(model.active[0].sliceID, "s-3")
        XCTAssertEqual(model.active[0].displayState, "Working")
        XCTAssertEqual(model.active[0].tintRole, .working)
    }

    func testBuildRailModel_activeSection_liveAgentWaitingWins() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-3": .waiting])

        XCTAssertEqual(model.active[0].displayState, "Waiting for input")
        XCTAssertEqual(model.active[0].tintRole, .waiting)
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

    // MARK: - MilestoneCard doneSlices / inFlightSlices

    func testMilestoneCard_hiddenDoneSlicesAreExposed() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "Done", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let coreCard = model.milestoneCards[0]
        XCTAssertEqual(coreCard.hiddenDoneCount, 1)
        XCTAssertEqual(coreCard.doneSlices.count, 1)
        XCTAssertEqual(coreCard.doneSlices[0].sliceID, "s-3")
        XCTAssertEqual(coreCard.doneSlices[0].glyph, .done)
    }

    func testMilestoneCard_inFlightSlicesAreExposed() {
        // s-2 (Feature A) is handed back, so it is counted and listed as
        // "in flight elsewhere" (it is drawn in NEEDS REVIEW).
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let coreCard = model.milestoneCards[0]
        XCTAssertEqual(coreCard.inFlightElsewhereCount, 1)
        XCTAssertEqual(coreCard.inFlightSlices.count, 1)
        XCTAssertEqual(coreCard.inFlightSlices[0].sliceID, "s-2")
    }

    func testMilestoneCard_noHiddenOrInFlightSlicesAreEmptyLists() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        // Polish has neither a done slice nor one shown elsewhere.
        let polishCard = model.milestoneCards[1]
        XCTAssertTrue(polishCard.doneSlices.isEmpty)
        XCTAssertTrue(polishCard.inFlightSlices.isEmpty)
    }
}
