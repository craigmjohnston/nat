import XCTest
@testable import NatKit
@testable import NatFixtures

final class SidebarModelTests: XCTestCase {
    private func slice(
        _ id: String, status: String = "Todo", milestone: String = "M1", branch: String? = nil,
        handedBack: Bool = false, pr: String = "", blocked: Bool = false, resumed: Bool = false
    ) -> Slice {
        Slice(
            id: id, name: "Slice \(id)", status: status, milestoneID: milestone, assignee: "", pr: pr, url: "",
            branch: branch, blocked: blocked, handedBack: handedBack, resumed: resumed)
    }

    private func plan(_ slices: [Slice], milestones: [String] = ["M1", "M2"]) -> ProjectInfo {
        ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: milestones.enumerated().map { Milestone(id: $1, name: $1, order: Double($0), status: "Active") },
            slices: slices)
    }

    private func session(_ id: String, tag: String, startedAt: Date, prs: [SessionPR] = []) -> Session {
        Session(id: id, tag: tag, live: false, startedAt: startedAt, dir: "/tmp", branch: "session/\(id)", prs: prs)
    }

    // MARK: - Display state

    func testEveryStageReadsAsItsDisplayState() {
        XCTAssertEqual(displayState(for: slice("a"), agent: nil), .todo)
        XCTAssertEqual(displayState(for: slice("a", blocked: true), agent: nil), .blocked)
        XCTAssertEqual(displayState(for: slice("a", status: "In progress"), agent: nil), .working)
        XCTAssertEqual(displayState(for: slice("a", status: "In progress"), agent: .working), .working)
        XCTAssertEqual(displayState(for: slice("a", status: "In progress"), agent: .waiting), .waiting)
        XCTAssertEqual(
            displayState(for: slice("a", status: "In progress", branch: "b", handedBack: true), agent: .waiting),
            .review, "a live session never moves a handed-back slice")
        XCTAssertEqual(displayState(for: slice("a", status: "In progress", pr: "https://x/pull/1"), agent: nil), .pr)
        XCTAssertEqual(displayState(for: slice("a", status: "In progress", pr: "https://x/pull/1", resumed: true), agent: .working), .working)
        XCTAssertEqual(displayState(for: slice("a", status: "In progress", pr: "https://x/pull/1", resumed: true), agent: .waiting), .waiting)
        XCTAssertEqual(displayState(for: slice("a", status: "Done", pr: "https://x/pull/1"), agent: .working), .done)
    }

    func testTheStatesFlagsAreTheDesigns() {
        XCTAssertEqual(SliceDisplayState.allCases.filter(\.needsYou), [.waiting, .review, .pr])
        XCTAssertEqual(SliceDisplayState.allCases.filter { !$0.isLaunched }, [.todo, .blocked])
        XCTAssertEqual(SliceDisplayState.allCases.filter(\.isInFlight), [.working, .waiting, .review, .pr])
    }

    // MARK: - The tree

    func testMilestonesHoldTheirSlicesInPlanOrderWithTheirCounts() {
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan([
                slice("1"), slice("2", status: "Done"), slice("3", milestone: "M2"), slice("4", milestone: "Gone"),
            ]))],
            liveAgents: [:])
        let project = model.projects[0]
        XCTAssertEqual(project.status, .loaded)
        XCTAssertEqual(project.milestones.map(\.name), ["M1", "M2", ""])
        XCTAssertEqual(project.milestones[0].slices.map(\.sliceID), ["1", "2"])
        XCTAssertEqual(project.milestones[0].done, 1)
        XCTAssertEqual(project.milestones[0].total, 2)
        XCTAssertEqual(project.milestones[2].slices.map(\.sliceID), ["4"], "an unfiled slice is still drawn")
        XCTAssertTrue(project.contains(sliceID: "3"))
        XCTAssertEqual(project.milestones[0].id, "M1")
        XCTAssertEqual(project.milestones[0].slices[0].id, "1")
        XCTAssertFalse(project.contains(sliceID: "9"))
    }

    func testAFinishedMilestoneMovesToDoneAndAFinishedSliceStaysPut() {
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan([
                slice("1", status: "Done"), slice("2", status: "Done"),
                slice("3", status: "Done", milestone: "M2"), slice("4", milestone: "M2"),
            ]))],
            liveAgents: [:])
        let project = model.projects[0]
        XCTAssertEqual(project.milestones.map(\.name), ["M2"])
        XCTAssertEqual(project.milestones[0].slices.map(\.sliceID), ["4", "3"],
                       "a done slice waits for its milestone, at its foot")
        XCTAssertEqual(project.doneMilestones.map(\.name), ["M1"])
        XCTAssertTrue(project.doneMilestones[0].isComplete)
        XCTAssertTrue(project.doneContains(sliceID: "1"))
        XCTAssertFalse(project.doneContains(sliceID: "3"))
        XCTAssertTrue(project.contains(sliceID: "1"))
    }

    func testBlockedSlicesSitBelowTheRestOfTheirMilestoneAndDoneOnesBelowThem() {
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan([
                slice("a", blocked: true), slice("b"), slice("c", blocked: true), slice("d", status: "Done"),
                slice("e"), slice("f", status: "Done"),
            ]))],
            liveAgents: [:])
        XCTAssertEqual(model.projects[0].milestones[0].slices.map(\.sliceID), ["b", "e", "a", "c", "d", "f"])
    }

    func testAnEmptyMilestoneIsStillARow() {
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan([slice("1")]))], liveAgents: [:])
        XCTAssertEqual(model.projects[0].milestones.map(\.total), [1, 0])
    }

    func testOnlyAPartlyDoneMilestoneOrTheSelectionsOpensByDefault() {
        func row(_ id: String) -> SidebarSliceRow {
            SidebarSliceRow(sliceID: id, projectID: "p", title: id, state: .todo, live: false)
        }
        let partly = SidebarMilestone(name: "partly", done: 1, total: 2, slices: [row("a"), row("b")])
        let untouched = SidebarMilestone(name: "untouched", done: 0, total: 2, slices: [row("c"), row("d")])
        let finished = SidebarMilestone(name: "finished", done: 2, total: 2, slices: [row("e"), row("f")])
        let empty = SidebarMilestone(name: "empty", done: 0, total: 0, slices: [])

        XCTAssertTrue(partly.opensByDefault(selecting: nil))
        XCTAssertFalse(untouched.opensByDefault(selecting: nil))
        XCTAssertFalse(untouched.opensByDefault(selecting: "a"))
        XCTAssertTrue(untouched.opensByDefault(selecting: "d"))
        XCTAssertFalse(finished.opensByDefault(selecting: nil))
        XCTAssertTrue(finished.opensByDefault(selecting: "e"))
        XCTAssertFalse(empty.opensByDefault(selecting: "a"))
    }

    func testEachPlanStatusIsReadOffTheLoad() {
        let inputs = [
            SidebarProjectInput(id: "a", name: "A", plan: nil, isLoading: true),
            SidebarProjectInput(id: "b", name: "B", plan: nil, isLoading: false, errorMessage: "boom"),
            SidebarProjectInput(id: "c", name: "C", plan: plan([slice("1")]), errorMessage: "stale"),
            SidebarProjectInput(id: "d", name: "D", plan: plan([])),
            SidebarProjectInput(id: "e", name: "E", kind: .untitled, plan: nil),
            SidebarProjectInput(id: "f", name: "F", plan: nil),
        ]
        let model = buildSidebarModel(projects: inputs, liveAgents: [:])
        XCTAssertEqual(model.projects.map(\.status), [
            .loading, .failed("boom"), .stale("stale"), .empty, .none, .loading,
        ])
        XCTAssertEqual(model.projects[4].kind, .untitled)
    }

    func testTheScratchProjectIsItsOwnFoldNotAProjectRow() {
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "s", name: "Scratch", kind: .scratch, plan: plan([slice("1")])),
                SidebarProjectInput(id: "p", name: "P", plan: plan([slice("2")])),
            ],
            liveAgents: [:])
        XCTAssertEqual(model.projects.map(\.id), ["p"])
        XCTAssertEqual(model.scratch?.id, "s")
        XCTAssertTrue(model.scratch?.contains(sliceID: "1") ?? false)
    }

    func testNoScratchProjectMeansNoScratchFold() {
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan([]))], liveAgents: [:])
        XCTAssertNil(model.scratch)
    }

    // MARK: - Active

    func testActiveListsWhatIsInFlightNeedsYouFirstAcrossProjects() {
        let first = SidebarProjectInput(id: "p", name: "P", plan: plan([
            slice("working", status: "In progress"),
            slice("todo"),
            slice("review", status: "In progress", branch: "b", handedBack: true),
            slice("done", status: "Done"),
        ]))
        let second = SidebarProjectInput(id: "q", name: "Q", plan: plan([
            slice("waiting", status: "In progress"),
            slice("pr", status: "In progress", pr: "https://x/pull/2"),
        ]))
        let model = buildSidebarModel(
            projects: [first, second], liveAgents: ["working": .working, "waiting": .waiting])
        XCTAssertEqual(model.active.map(\.targetID), ["review", "waiting", "pr", "working"])
        XCTAssertEqual(model.active.map(\.projectName), ["P", "Q", "Q", "P"])
        XCTAssertEqual(model.needsYouCount, 3)
        XCTAssertEqual(model.projects.map(\.needsYou), [1, 2])
        XCTAssertTrue(model.active.last?.live == true)
        XCTAssertEqual(model.active.first?.id, "slice:review")
    }

    func testPlanningAgentsAndSessionsJoinActive() {
        let now = Date()
        let live = session("s1", tag: "session:p:s1", startedAt: now)
        let review = session(
            "s2", tag: "session:p:s2", startedAt: now.addingTimeInterval(-60),
            prs: [SessionPR(number: 1, title: "t", url: "u", state: "OPEN")])
        let ended = session("s3", tag: "session:p:s3", startedAt: now.addingTimeInterval(-120))
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "p", name: "P", plan: plan([])),
                SidebarProjectInput(id: "q", name: "Q", plan: plan([])),
            ],
            liveAgents: ["session:p:s1": .waiting],
            sessions: [ended, review, live],
            sessionsProjectID: "p",
            planningAgents: ["q": .working, "p": .waiting])
        XCTAssertEqual(model.active.map(\.id), ["workshop:p", "session:s1", "session:s2", "workshop:q"])
        XCTAssertEqual(model.active.map(\.state), [.waiting, .waiting, .review, .working])
        XCTAssertEqual(model.active[0].title, workshopRowTitle)
        XCTAssertEqual(model.active[1].title, sessionRowTitle)
        XCTAssertEqual(model.active[1].kind, .session)
        XCTAssertEqual(model.active[3].kind, .workshop)
        XCTAssertEqual(model.projects.map(\.needsYou), [3, 0])
    }

    func testAWorkshopWithAProposalUpSaysPlanReady() {
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "p", name: "P", plan: plan([slice("1", status: "In progress")])),
                SidebarProjectInput(id: "q", name: "Q", plan: plan([])),
                SidebarProjectInput(id: "untitled-1", name: "Untitled", kind: .untitled, plan: nil),
                SidebarProjectInput(id: "untitled-2", name: "Untitled", kind: .untitled, plan: nil),
            ],
            liveAgents: [:],
            planningAgents: ["p": .waiting, "untitled-1": .working],
            pinnedWorkshops: ["q", "untitled-2"],
            proposedWorkshops: ["p", "untitled-1", "1"])
        let ready = Dictionary(uniqueKeysWithValues: model.active.map { ($0.id, $0.planReady) })
        XCTAssertEqual(ready, [
            "workshop:p": true, "workshop:untitled-1": true, "slice:1": false,
            "workshop:q": false, "workshop:untitled-2": false,
        ], "a project's by its id, an Untitled tab's by its tab id; never a slice row")
        XCTAssertFalse(
            SidebarActiveRow(
                kind: .session, targetID: "s", projectID: "p", projectName: "P", title: "t", state: .working,
                live: true, planReady: true
            ).planReady, "never a session row")
        XCTAssertEqual(planReadyLabel, "Plan ready")
    }

    func testAProjectsTagIsItsFirstThreeLetters() {
        XCTAssertEqual(projectTags([("a", "notion-agent-tracker"), ("b", "gnat")]), ["a": "NOT", "b": "GNA"])
        XCTAssertEqual(projectTags([("a", "Go")]), ["a": "GO"])
    }

    func testTagsThatWouldCollideTakeANumberInstead() {
        XCTAssertEqual(
            projectTags([("a", "notion"), ("b", "gnat"), ("c", "nothing"), ("d", "Notes app")]),
            ["a": "NO1", "b": "GNA", "c": "NO2", "d": "NO3"])
    }

    func testActiveRowsCarryTheirProjectsTag() {
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "p", name: "notion", plan: plan([slice("1", status: "In progress")])),
                SidebarProjectInput(id: "q", name: "nothing", plan: plan([slice("2", status: "In progress")])),
            ],
            liveAgents: [:])
        XCTAssertEqual(model.active.map(\.projectTag), ["NO1", "NO2"])
        let lone = SidebarActiveRow(
            kind: .slice, targetID: "x", projectID: "p", projectName: "gnat", title: "t", state: .working, live: false)
        XCTAssertEqual(lone.projectTag, "GNA")
    }

    func testSessionsOfAnotherProjectAreNotItsOwn() {
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan([]))],
            liveAgents: ["t": .working],
            sessions: [session("s", tag: "t", startedAt: Date())],
            sessionsProjectID: "other")
        XCTAssertTrue(model.active.isEmpty)
    }

    func testEveryStateHasItsOwnWord() {
        let words = SliceDisplayState.allCases.map(\.word)
        XCTAssertEqual(words, ["To do", "Working", "Waiting for you", "In review", "PR open", "Blocked", "Done"])
    }

    /// Hiding done items drops the Done folder and every done slice still
    /// under an open milestone, and leaves the milestone's count alone.
    func testHidingDoneDropsDoneSlicesAndTheDoneFolder() {
        func row(_ id: String, _ state: SliceDisplayState) -> SidebarSliceRow {
            SidebarSliceRow(sliceID: id, projectID: "p", title: id, state: state, live: false)
        }
        let finished = SidebarMilestone(name: "M1", done: 1, total: 1, slices: [row("a", .done)])
        let open = SidebarMilestone(name: "M2", done: 1, total: 3, slices: [row("b", .done), row("c", .todo), row("d", .review)])
        let project = SidebarProject(
            id: "p", name: "P", kind: .project, status: .loaded,
            milestones: [open], doneMilestones: [finished], needsYou: 1)

        let hidden = project.hidingDone()

        XCTAssertEqual(hidden.milestones, [SidebarMilestone(name: "M2", done: 1, total: 3, slices: [row("c", .todo), row("d", .review)])])
        XCTAssertTrue(hidden.doneMilestones.isEmpty)
        XCTAssertEqual(hidden.needsYou, 1)
        XCTAssertEqual(hidden.id, "p")
    }

    func testTheUnfiledMilestonesSlicesAreLooseNotAFolder() {
        let info = ProjectInfo(
            project: Project(id: "s", name: "Scratch", conventions: ""),
            milestones: [
                Milestone(id: "M1", name: "M1", order: 0, status: "Active"),
                Milestone(id: "U", name: "Unfiled", order: 1, status: "Queued", unfiled: true),
            ],
            slices: [slice("1", milestone: "U"), slice("2"), slice("3", status: "Done", milestone: "U")])
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "s", name: "Scratch", kind: .scratch, plan: info)], liveAgents: [:])

        let scratch = try! XCTUnwrap(model.scratch)
        XCTAssertEqual(scratch.milestones.map(\.name), ["M1"])
        XCTAssertEqual(scratch.loose.map(\.sliceID), ["1", "3"])
        XCTAssertTrue(scratch.contains(sliceID: "1"))
        XCTAssertEqual(scratch.hidingDone().loose.map(\.sliceID), ["1"])
    }

    // MARK: - Project colours

    /// Every row drawn for a project carries its colour — its project row
    /// and each of its Active rows, slice, session and workshop alike — and
    /// an Untitled row, which has no config entry, none.
    func testEveryRowCarriesItsProjectsColour() {
        let sessions = [session("s1", tag: "nat-s1", startedAt: Date())]
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(
                    id: "p", name: "P", plan: plan([slice("a", status: "In progress")]), color: .teal),
                SidebarProjectInput(id: "q", name: "Q", plan: plan([]), color: nil),
                SidebarProjectInput(id: "u", name: "Untitled", kind: .untitled, plan: nil, color: .red),
            ],
            liveAgents: ["nat-s1": .working],
            sessions: sessions, sessionsProjectID: "p",
            planningAgents: ["p": .working])

        XCTAssertEqual(model.projects.map(\.color), [.teal, nil, nil])
        XCTAssertEqual(model.active.map(\.kind), [.workshop, .session, .slice])
        XCTAssertEqual(model.active.map(\.color), [.teal, .teal, .teal])
        XCTAssertEqual(model.projects[0].hidingDone().color, .teal, "hiding done work keeps the colour")
    }

    /// A pinned or reconnecting workshop row is its project's colour too.
    func testAWorkshopRowNotYetRunningCarriesItsProjectsColour() {
        let inputs = [
            SidebarProjectInput(id: "p", name: "P", plan: plan([]), color: .blue),
            SidebarProjectInput(id: "q", name: "Q", plan: plan([]), color: .pink),
        ]
        let model = buildSidebarModel(
            projects: inputs, liveAgents: [:], pinnedWorkshops: ["p"], reconnectingWorkshops: ["q"])
        XCTAssertEqual(model.active.map(\.color), [.blue, .pink])
    }

    /// Every project row carries its badge's word — the tag its Active rows
    /// carry, a source project's its plugin's — and an Untitled row none.
    func testEveryProjectRowCarriesItsTag() {
        let model = buildSidebarModel(
            projects: [
                SidebarProjectInput(id: "p", name: "Pancake", plan: plan([slice("a", status: "In progress")])),
                SidebarProjectInput(id: "w", name: "Work", plan: Fixtures.sourceProjectInfo(), isSource: true),
                SidebarProjectInput(id: "u", name: "Untitled", kind: .untitled, plan: nil),
            ],
            liveAgents: [:])

        XCTAssertEqual(model.projects.map(\.tag), ["PAN", ""])
        XCTAssertEqual(model.sources.map(\.tag), ["DM"])
        XCTAssertEqual(model.active.first { $0.projectID == "p" }?.projectTag, "PAN")
        XCTAssertEqual(model.projects[0].hidingDone().tag, "PAN", "hiding done work keeps the tag")
    }
}
