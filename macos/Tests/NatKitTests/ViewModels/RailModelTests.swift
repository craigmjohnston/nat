import XCTest
@testable import NatKit

final class RailModelTests: XCTestCase {
    /// The entries of the one ACTIVE list that are branches awaiting a
    /// review — what used to be the NEEDS REVIEW section.
    private func reviews(_ model: RailModel) -> [ActiveEntry] {
        model.active.filter { $0.tintRole == .needsReview }
    }

    /// The entries that are slices something is happening on — what the
    /// ACTIVE section held before the other two were folded into it.
    private func worked(_ model: RailModel) -> [ActiveEntry] {
        model.active.filter { $0.kind == .slice && $0.tintRole != .needsReview }
    }

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

        XCTAssertTrue(model.active.isEmpty)
        XCTAssertTrue(model.todoFolders.isEmpty)
        XCTAssertTrue(model.doneFolders.isEmpty)
        XCTAssertNil(model.doneSummary)
    }

    // MARK: - Entries awaiting review

    func testBuildRailModel_reviewSection() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(reviews(model).count, 1)
        XCTAssertEqual(reviews(model)[0].kind, .slice)
        XCTAssertEqual(reviews(model)[0].sliceID, "s-2")
        XCTAssertEqual(reviews(model)[0].name, "Feature A")
        XCTAssertEqual(reviews(model)[0].displayState, "Needs review")
        XCTAssertEqual(reviews(model)[0].metaRole, .stat)
        XCTAssertNil(reviews(model)[0].meta, "no reviewStats given at all should leave the row statless")
        XCTAssertEqual(reviews(model)[0].detail, ["Core"], "the milestone alone with no file count fetched")
    }

    /// A Done slice whose pull request is positively read as open is still in
    /// review — the board marks a slice Done as it opens the pull request,
    /// and the review is not over until that lands. Without a reading it is
    /// out, which is what keeps a project's finished history from flooding
    /// the section on a board nobody has asked gh anything on.
    func testBuildRailModel_doneSliceWithAnOpenPRIsInReview() {
        var slices = testSlices!
        slices.append(Slice(
            id: "s-pr", name: "Awaiting merge", status: "Done", milestoneID: "m-2",
            assignee: "", pr: "https://github.com/x/y/pull/9", url: "", blocked: false, handedBack: false
        ))
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)

        let unread = buildRailModel(from: projectInfo, liveAgents: [:])
        XCTAssertFalse(reviews(unread).contains { $0.sliceID == "s-pr" },
                       "with no reading taken the slice stays out")

        let model = buildRailModel(
            from: projectInfo, liveAgents: [:],
            prReadiness: ["s-pr": "awaiting review"]
        )
        let entry = reviews(model).first { $0.sliceID == "s-pr" }
        XCTAssertNotNil(entry)
        XCTAssertEqual(entry?.meta, "awaiting review",
                       "a PR-open slice has no branch tally; its meta is the reading's own words")
    }

    /// The diff tally wins the meta where the slice has one — a handed-back
    /// slice that also has an open pull request reads as the hand-back it is.
    func testBuildRailModel_reviewStatWinsOverReadinessWords() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(
            from: projectInfo, liveAgents: [:],
            reviewStats: ["s-2": "+10 \u{2212}2"],
            prReadiness: ["s-2": "awaiting review"]
        )

        XCTAssertEqual(reviews(model).first { $0.sliceID == "s-2" }?.meta, "+10 \u{2212}2")
    }

    func testBuildRailModel_reviewSectionCarriesItsStatAndFileCount() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(
            from: projectInfo, liveAgents: [:],
            reviewStats: ["s-2": "+10 \u{2212}3"],
            reviewFileCounts: ["s-2": 4]
        )

        XCTAssertEqual(reviews(model)[0].meta, "+10 \u{2212}3")
        XCTAssertEqual(reviews(model)[0].detail, ["Core", "4 files"])
    }

    func testBuildRailModel_reviewSectionLeavesAnUnfetchedStatNil() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(
            from: projectInfo, liveAgents: [:],
            reviewStats: ["some-other-slice": "+1 \u{2212}1"],
            reviewFileCounts: ["some-other-slice": 1]
        )

        XCTAssertNil(reviews(model)[0].meta)
        XCTAssertEqual(reviews(model)[0].detail, ["Core"])
    }

    func testBuildRailModel_reviewSectionNamesItsMilestone() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(reviews(model)[0].detail.first, "Core")
    }

    func testBuildRailModel_reviewSectionUnknownMilestoneReadsEmpty() {
        var slices = testSlices!
        slices[1] = Slice(
            id: "s-2", name: "Feature A", status: "In progress", milestoneID: "m-gone",
            assignee: "user", pr: "", url: "", branch: "feature-a", blocked: false, handedBack: true
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(reviews(model)[0].detail, [], "an unknown milestone names nothing rather than nothing at all")
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

        XCTAssertEqual(worked(model).count, 1)
        XCTAssertEqual(worked(model)[0].sliceID, "s-3")
        XCTAssertEqual(worked(model)[0].displayState, "Ready to push")
        XCTAssertEqual(worked(model)[0].tintRole, .readyToPush)
        XCTAssertEqual(worked(model)[0].detail, ["Core"])
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

        let entry = worked(model).first { $0.sliceID == "s-4" }
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
        // s-2 is In progress and handed back (it reads "Needs review"). A
        // live agent still on its branch is the review going back to it, not
        // a reason to also draw it in ACTIVE.
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-2": .working])

        XCTAssertFalse(worked(model).contains { $0.sliceID == "s-2" })
        XCTAssertEqual(reviews(model).count, 1)
        XCTAssertEqual(reviews(model)[0].sliceID, "s-2")
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

        XCTAssertEqual(worked(model).count, 1)
        XCTAssertEqual(worked(model)[0].sliceID, "s-3")
        XCTAssertEqual(worked(model)[0].displayState, "Working")
        XCTAssertEqual(worked(model)[0].tintRole, .working)
    }

    func testBuildRailModel_activeSection_liveAgentWaitingWins() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-3": .waiting])

        XCTAssertEqual(worked(model)[0].displayState, "Waiting for input")
        XCTAssertEqual(worked(model)[0].tintRole, .waiting)
    }

    // MARK: - ACTIVE elapsed

    func testBuildRailModel_activeSection_elapsedFromAgentStart() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let now = Date(timeIntervalSince1970: 10_000)
        let model = buildRailModel(
            from: projectInfo, liveAgents: ["s-3": .working],
            agentStarts: ["s-3": now.addingTimeInterval(-14 * 60)],
            now: now
        )

        XCTAssertEqual(worked(model)[0].meta, "14m")
        XCTAssertEqual(worked(model)[0].metaRole, .elapsed)
    }

    func testBuildRailModel_activeSection_noLiveAgentHasNoElapsed() {
        // A stale start for a slice with no live agent must not surface: the
        // elapsed rides the live reading.
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let now = Date(timeIntervalSince1970: 10_000)
        let model = buildRailModel(
            from: projectInfo, liveAgents: [:],
            agentStarts: ["s-3": now.addingTimeInterval(-300)],
            now: now
        )

        XCTAssertNil(worked(model)[0].meta)
    }

    func testBuildRailModel_activeSection_liveAgentWithNoStartHasNoElapsed() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: ["s-3": .working])

        XCTAssertNil(worked(model)[0].meta)
    }

    func testElapsedLabel() {
        let now = Date(timeIntervalSince1970: 100_000)
        XCTAssertEqual(elapsedLabel(from: now.addingTimeInterval(-30), to: now), "<1m")
        XCTAssertEqual(elapsedLabel(from: now.addingTimeInterval(-60), to: now), "1m")
        XCTAssertEqual(elapsedLabel(from: now.addingTimeInterval(-31 * 60), to: now), "31m")
        XCTAssertEqual(elapsedLabel(from: now.addingTimeInterval(-64 * 60), to: now), "1h 4m")
        XCTAssertEqual(elapsedLabel(from: now.addingTimeInterval(-120 * 60), to: now), "2h 0m")
    }

    // MARK: - TODO folders

    func testBuildRailModel_todoFolders() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        // Should have folders for Core and Polish (not Foundation, which is Done)
        XCTAssertEqual(model.todoFolders.count, 2)

        // Core milestone
        let core = model.todoFolders[0]
        XCTAssertEqual(core.title, "Core")
        XCTAssertEqual(core.done, 0)
        XCTAssertEqual(core.total, 3) // Feature A, Feature B, Blocked Task
        // Feature A is handed back — drawn as a review entry, so it does not
        // repeat inside the folder.
        XCTAssertEqual(core.slices.map(\.sliceID), ["s-3", "s-4"])
        XCTAssertEqual(core.inFlightCount, 1)

        // Polish milestone
        let polish = model.todoFolders[1]
        XCTAssertEqual(polish.title, "Polish")
        XCTAssertEqual(polish.done, 0)
        XCTAssertEqual(polish.total, 1)
        XCTAssertEqual(polish.slices.map(\.sliceID), ["s-5"])
        XCTAssertEqual(polish.inFlightCount, 0)
    }

    func testBuildRailModel_todoFolderExcludesActiveSlices() {
        // An In progress slice with nothing out lives in ACTIVE — never also
        // inside its milestone's folder.
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let core = model.todoFolders[0]
        XCTAssertEqual(core.slices.map(\.sliceID), ["s-4"])
        XCTAssertEqual(core.inFlightCount, 2)
    }

    func testBuildRailModel_todoFolderExcludesDoneSlices() {
        // A finished slice lives under DONE, not inside the TODO folder.
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "Done", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let core = model.todoFolders[0]
        XCTAssertEqual(core.done, 1)
        XCTAssertFalse(core.slices.contains { $0.sliceID == "s-3" })
    }

    func testBuildRailModel_currentIsFirstMilestoneWithWorkRemaining() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertTrue(model.todoFolders[0].isCurrent, "Core is the first milestone still holding work")
        XCTAssertFalse(model.todoFolders[1].isCurrent)
    }

    func testBuildRailModel_blockedGlyph() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let blockedSlice = model.todoFolders[0].slices.first { $0.sliceID == "s-4" }

        XCTAssertNotNil(blockedSlice)
        XCTAssertEqual(blockedSlice?.glyph, .blocked)
        XCTAssertTrue(blockedSlice?.isBlocked ?? false)
    }

    func testBuildRailModel_todoGlyph() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        let todoSlice = model.todoFolders[0].slices.first { $0.sliceID == "s-3" }

        XCTAssertNotNil(todoSlice)
        XCTAssertEqual(todoSlice?.glyph, .todo)
    }

    // MARK: - DONE section

    func testBuildRailModel_doneSummaryCountsTheWholePlan() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(model.doneSummary, DoneSummary(doneCount: 1, totalCount: 5))
    }

    func testBuildRailModel_doneFolderForACompleteMilestone() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(model.doneFolders.count, 1)
        let folder = model.doneFolders[0]
        XCTAssertEqual(folder.milestoneID, "m-1")
        XCTAssertEqual(folder.title, "Foundation")
        XCTAssertEqual(folder.done, 1)
        XCTAssertEqual(folder.total, 1)
        XCTAssertTrue(folder.isComplete)
        XCTAssertFalse(folder.isCurrent)
        XCTAssertEqual(folder.slices.map(\.sliceID), ["s-1"])
        XCTAssertEqual(folder.slices[0].glyph, .done)
    }

    func testBuildRailModel_partlyDoneMilestoneAppearsInBothSections() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "Done", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        // Core still stands in TODO with its remaining slice…
        let todoCore = model.todoFolders.first { $0.milestoneID == "m-2" }
        XCTAssertNotNil(todoCore)
        XCTAssertEqual(todoCore?.slices.map(\.sliceID), ["s-4"])

        // …and appears under DONE with its finished one, count shown and
        // checkmark withheld.
        let doneCore = model.doneFolders.first { $0.milestoneID == "m-2" }
        XCTAssertNotNil(doneCore)
        XCTAssertEqual(doneCore?.done, 1)
        XCTAssertEqual(doneCore?.total, 3)
        XCTAssertEqual(doneCore?.isComplete, false)
        XCTAssertEqual(doneCore?.slices.map(\.sliceID), ["s-3"])
    }

    func testBuildRailModel_doneFoldersOrderPartialFirstThenNewest() {
        // Two part-done open milestones and two complete ones: the open pair
        // lead in plan order, the complete pair follow newest-first.
        let milestones = [
            Milestone(id: "m-1", name: "First", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Second", order: 2, status: "Done"),
            Milestone(id: "m-3", name: "Third", order: 3, status: "Active"),
            Milestone(id: "m-4", name: "Fourth", order: 4, status: "Active")
        ]
        let slices = [
            Slice(id: "s-1", name: "A", status: "Done", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-2", name: "B", status: "Done", milestoneID: "m-2", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-3", name: "C", status: "Done", milestoneID: "m-3", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-4", name: "D", status: "Todo", milestoneID: "m-3", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-5", name: "E", status: "Done", milestoneID: "m-4", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-6", name: "F", status: "Todo", milestoneID: "m-4", assignee: "", pr: "", url: "", blocked: false, handedBack: false)
        ]
        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertEqual(model.doneFolders.map(\.milestoneID), ["m-3", "m-4", "m-2", "m-1"])
    }

    func testBuildRailModel_milestoneWithNothingDoneHasNoDoneFolder() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertFalse(model.doneFolders.contains { $0.milestoneID == "m-2" })
        XCTAssertFalse(model.doneFolders.contains { $0.milestoneID == "m-3" })
    }

    func testBuildRailModel_noDoneSummaryWhenNothingIsDone() {
        let milestones = [
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active")
        ]
        let slices = [testSlices[2], testSlices[3]]
        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertNil(model.doneSummary)
        XCTAssertTrue(model.doneFolders.isEmpty)
    }

    /// A Done slice whose pull request is still open reads as work in flight
    /// everywhere the rail counts done-ness: out of the DONE folder and its
    /// counts, out of the summary, and its milestone not folded complete —
    /// the same rule the progress bar applies. It is a review entry
    /// instead, awaiting its merge.
    func testBuildRailModel_openPRHoldsADoneSliceOutOfDone() {
        let milestones = [
            Milestone(id: "m-1", name: "Foundation", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active"),
        ]
        let slices = [
            Slice(id: "s-1", name: "Merged already", status: "Done", milestoneID: "m-1",
                  assignee: "", pr: "https://github.com/o/r/pull/1", url: "", blocked: false, handedBack: false),
            Slice(id: "s-2", name: "Awaiting merge", status: "Done", milestoneID: "m-1",
                  assignee: "", pr: "https://github.com/o/r/pull/2", url: "", blocked: false, handedBack: false),
            Slice(id: "s-3", name: "Feature B", status: "Todo", milestoneID: "m-2",
                  assignee: "", pr: "", url: "", blocked: false, handedBack: false),
        ]
        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let model = buildRailModel(
            from: projectInfo, liveAgents: [:],
            prReadiness: ["s-2": "awaiting review"]
        )

        // The slice awaiting its merge is review work, not a DONE row.
        XCTAssertEqual(reviews(model).map(\.sliceID), ["s-2"])
        XCTAssertEqual(model.doneFolders.count, 1)
        let folder = model.doneFolders[0]
        XCTAssertEqual(folder.slices.map(\.sliceID), ["s-1"])
        XCTAssertEqual(folder.done, 1)
        XCTAssertFalse(folder.isComplete)
        XCTAssertEqual(model.doneSummary, DoneSummary(doneCount: 1, totalCount: 3))
        // The milestone is not folded away: it still holds moving work, so
        // it stays a TODO folder — and the current one, since its work moves.
        XCTAssertEqual(model.todoFolders.map(\.milestoneID), ["m-1", "m-2"])
        XCTAssertTrue(model.todoFolders[0].isCurrent)
    }

    func testMilestoneFolderIsCompleteNeedsSlices() {
        let empty = MilestoneFolder(milestoneID: "m", title: "M", done: 0, total: 0, isCurrent: false, slices: [])
        XCTAssertFalse(empty.isComplete)
        let full = MilestoneFolder(milestoneID: "m", title: "M", done: 2, total: 2, isCurrent: false, slices: [])
        XCTAssertTrue(full.isComplete)
    }

    // MARK: - Workshop entry

    /// The entry as the whole of it, so every field the row draws is pinned
    /// once and the cases below say only what their own reading changes.
    private func workshopEntry(_ state: String, _ tint: ActiveTintRole, elapsed: String? = nil) -> ActiveEntry {
        ActiveEntry(
            kind: .workshop,
            name: "Workshop the plan",
            displayState: state,
            tintRole: tint,
            detail: ["Planning agent"],
            meta: elapsed,
            metaRole: .elapsed
        )
    }

    func testBuildWorkshopEntry_nothingLiveAndNotLaunchingIsNoEntry() {
        XCTAssertNil(buildWorkshopEntry(activity: nil, isLaunching: false))
    }

    func testBuildWorkshopEntry_launching() {
        let entry = buildWorkshopEntry(activity: nil, isLaunching: true)

        XCTAssertEqual(entry, workshopEntry("Launching…", .launching))
    }

    func testBuildWorkshopEntry_workingWithElapsed() {
        let now = Date(timeIntervalSince1970: 1_000_000)
        let entry = buildWorkshopEntry(
            activity: .working,
            isLaunching: false,
            firstSeen: now.addingTimeInterval(-14 * 60),
            now: now
        )

        XCTAssertEqual(entry, workshopEntry("Working", .working, elapsed: "14m"))
    }

    func testBuildWorkshopEntry_waitingWithElapsed() {
        let now = Date(timeIntervalSince1970: 1_000_000)
        let entry = buildWorkshopEntry(
            activity: .waiting,
            isLaunching: false,
            firstSeen: now.addingTimeInterval(-64 * 60),
            now: now
        )

        XCTAssertEqual(entry, workshopEntry("Waiting for input", .waiting, elapsed: "1h 4m"))
    }

    func testBuildWorkshopEntry_liveAgentWithNoStampHasNoElapsed() {
        let entry = buildWorkshopEntry(activity: .working, isLaunching: false, firstSeen: nil)

        XCTAssertNil(entry?.meta)
    }

    func testBuildWorkshopEntry_liveAgentWinsOverTheLaunchingFlag() {
        let entry = buildWorkshopEntry(activity: .waiting, isLaunching: true)

        XCTAssertEqual(entry?.tintRole, .waiting)
    }

    func testBuildWorkshopEntry_selectedWithNothingRunningIsTheComposerRow() {
        let entry = buildWorkshopEntry(activity: nil, isLaunching: false, isSelected: true)

        XCTAssertEqual(entry, workshopEntry("New session", .new))
    }

    func testBuildWorkshopEntry_launchingWinsOverTheComposerRow() {
        let entry = buildWorkshopEntry(activity: nil, isLaunching: true, isSelected: true)

        XCTAssertEqual(entry?.tintRole, .launching)
    }

    /// The entry is keyed by a name of its own rather than a slice id, which
    /// is what lets one list hold it beside the slices.
    func testWorkshopEntryIsIdentifiedByItsOwnKey() {
        XCTAssertEqual(buildWorkshopEntry(activity: .working, isLaunching: false)?.id, workshopEntryID)
        XCTAssertEqual(buildWorkshopEntry(activity: .working, isLaunching: false)?.sliceID, "")
    }

    // MARK: - The one ACTIVE list

    /// Workshop first, then the branches awaiting review, then the slices
    /// being worked: the prominence the three separate sections gave them,
    /// kept as an order inside one.
    func testBuildRailModel_mergesTheThreeKindsInOrder() {
        var slices = testSlices!
        slices[2] = Slice(
            id: "s-3", name: "Feature B", status: "In progress", milestoneID: "m-2",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: slices)
        let model = buildRailModel(
            from: projectInfo,
            liveAgents: ["s-3": .working],
            workshop: buildWorkshopEntry(activity: .working, isLaunching: false)
        )

        XCTAssertEqual(model.active.map(\.id), [workshopEntryID, "s-2", "s-3"])
        XCTAssertEqual(model.active.map(\.kind), [.workshop, .slice, .slice])
        XCTAssertEqual(model.active.map(\.displayState), ["Working", "Needs review", "Working"])
    }

    /// No planning agent is simply no entry: the list is what it was.
    func testBuildRailModel_noWorkshopLeavesTheListAlone() {
        let projectInfo = ProjectInfo(project: testProject, milestones: testMilestones, slices: testSlices)
        let model = buildRailModel(from: projectInfo, liveAgents: [:])

        XCTAssertFalse(model.active.contains { $0.kind == .workshop })
    }

    /// A plan with nothing in flight and a planning agent live is the one
    /// entry alone — what keeps the empty note off a rail that has something
    /// to show.
    func testBuildRailModel_workshopAloneIsNotAnEmptySection() {
        let projectInfo = ProjectInfo(project: testProject, milestones: [], slices: [])
        let model = buildRailModel(
            from: projectInfo,
            liveAgents: [:],
            workshop: buildWorkshopEntry(activity: nil, isLaunching: true)
        )

        XCTAssertEqual(model.active.map(\.kind), [.workshop])
    }
}
