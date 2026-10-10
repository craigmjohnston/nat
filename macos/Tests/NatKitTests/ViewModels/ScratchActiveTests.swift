import XCTest
@testable import NatKit
@testable import NatFixtures

/// The scratch project's work in Active, the titlebar, the run tree and the
/// add menus: one Scratch row with its work nested under it, and Scratch
/// named by its mark rather than a badge.
final class ScratchActiveTests: XCTestCase {
    private let scratch = Fixtures.scratchProjectID

    private func slice(_ id: String, _ status: String) -> Slice {
        Slice(
            id: id, name: "Scratch \(id)", status: status, milestoneID: "Unfiled", assignee: "", pr: "", url: "",
            blocked: false, handedBack: false)
    }

    private func scratchPlan(_ slices: [Slice]) -> ProjectInfo {
        ProjectInfo(
            project: Project(id: scratch, name: "Scratch", conventions: ""),
            milestones: [Milestone(id: "Unfiled", name: "Unfiled", order: 0, status: "Active", unfiled: true)],
            slices: slices)
    }

    /// The fixture project, then Scratch, then the second project — so the
    /// Scratch row has rows both before and after it.
    private func model(
        _ scratchSlices: [Slice], sessions: [Session] = [], liveAgents: [String: AgentActivity] = [:],
        planningAgents: [String: AgentActivity] = [:], pinned: Set<String> = [], reconnecting: Set<String> = []
    ) -> SidebarModel {
        buildSidebarModel(
            projects: [
                SidebarProjectInput(id: Fixtures.projectID, name: "notion-agent-tracker", plan: Fixtures.projectInfo),
                SidebarProjectInput(id: scratch, name: "Scratch", kind: .scratch, plan: scratchPlan(scratchSlices)),
                SidebarProjectInput(id: Fixtures.secondProjectID, name: "gnat", plan: Fixtures.secondProjectInfo),
            ],
            liveAgents: liveAgents, sessions: sessions, sessionsProjectID: scratch, planningAgents: planningAgents,
            pinnedWorkshops: pinned, reconnectingWorkshops: reconnecting)
    }

    func testScratchsActiveSlicesNestUnderOneScratchRowWhereTheFirstWouldHaveBeen() throws {
        let model = model([slice("a", "In progress"), slice("b", "Todo"), slice("c", "In progress")])
        let scratchRows = model.active.filter(\.isScratch)
        XCTAssertEqual(scratchRows.map(\.targetID), ["a", "c"])
        XCTAssertTrue(model.active.filter { $0.projectID != scratch }.allSatisfy { !$0.isScratch })

        let entries = model.activeEntries
        let scratchEntries = entries.filter { if case .scratch = $0 { true } else { false } }
        XCTAssertEqual(scratchEntries.count, 1, "only ever one Scratch item")
        guard case .scratch(let projectID, let rows) = try XCTUnwrap(scratchEntries.first) else { return XCTFail() }
        XCTAssertEqual(projectID, scratch)
        XCTAssertEqual(rows.map(\.targetID), ["a", "c"], "the slices keep their order")
        XCTAssertEqual(scratchEntries.first?.id, "scratch")

        // It stands where its first slice would: everything before that row
        // in Active is before it, everything else after it.
        let firstAt = try XCTUnwrap(model.active.firstIndex(where: \.isScratch))
        let entryAt = try XCTUnwrap(entries.firstIndex(of: scratchEntries[0]))
        XCTAssertEqual(entries[..<entryAt].flatMap(\.rows).map(\.id), model.active[..<firstAt].map(\.id))
        XCTAssertEqual(entries.flatMap(\.rows).count, model.active.count)
    }

    func testNoScratchRowWhenNoScratchSliceIsActive() {
        let model = model([slice("b", "Todo")])
        XCTAssertFalse(model.activeEntries.contains { if case .scratch = $0 { true } else { false } })
    }

    func testScratchsSessionsAndWorkshopNestUnderTheSameRow() throws {
        let session = Fixtures.liveSession
        for (planning, pinned, reconnecting) in [
            ([scratch: AgentActivity.working], Set<String>(), Set<String>()),
            ([:], [scratch], []),
            ([:], [], [scratch]),
        ] {
            let model = model(
                [slice("a", "In progress")], sessions: [session], liveAgents: [session.tag: .working],
                planningAgents: planning, pinned: pinned, reconnecting: reconnecting)
            let entries = model.activeEntries.filter { if case .scratch = $0 { true } else { false } }
            XCTAssertEqual(entries.count, 1)
            XCTAssertEqual(
                Set(try XCTUnwrap(entries.first).rows.map(\.kind)), [.slice, .session, .workshop],
                "every scratch row under the one Scratch row")
        }
    }

    func testMenusOfferTheProjectsThenScratchApart() {
        let model = model([])
        XCTAssertEqual(model.menuTargets.projects.map(\.id), [Fixtures.projectID, Fixtures.secondProjectID])
        XCTAssertEqual(model.menuTargets.scratch?.id, scratch)
        XCTAssertNil(SidebarModel(active: [], projects: []).menuTargets.scratch)
    }

    func testTheTitlebarNamesScratchsWorkByItsMark() {
        let model = model([slice("a", "In progress")])
        let fromRow = titlebarIdentity(
            for: .slice(id: "a", name: "A", state: .todo), projectID: scratch, active: model.active, tags: [:])
        XCTAssertTrue(fromRow.isScratch)
        let noRow = titlebarIdentity(
            for: .slice(id: "z", name: "Z", state: .todo), projectID: scratch, active: [], tags: [:],
            scratchProjectID: scratch)
        XCTAssertTrue(noRow.isScratch)
        let other = titlebarIdentity(
            for: .slice(id: "z", name: "Z", state: .todo), projectID: "p", active: [], tags: [:],
            scratchProjectID: scratch)
        XCTAssertFalse(other.isScratch)
        XCTAssertFalse(fromRow.lastCrumb(afterProjectCrumb: true).isScratch, "the project crumb names it already")
    }

    func testTheRunTreeMarksScratch() async {
        var projects = Fixtures.runsConfig.projects
        projects[scratch] = ProjectConfig(
            name: "Scratch", slicesDSID: "", workingDir: "/Users/craig",
            runs: [RunCommand(label: "Notes", command: "open ~/notes.md", scope: .global)])
        let config = NatProjectConfig(
            projects: projects, agentSplitPercent: 45, pollSeconds: 3600,
            assigneeUserName: "Craig Johnston", scratchProject: scratch)
        let app = await Fixtures.startedAppModel(config: config)
        let runProjects = await app.runProjects
        XCTAssertEqual(runProjects.filter(\.isScratch).map(\.id), [scratch])
        XCTAssertEqual(runProjects.filter { !$0.isScratch }.count, 2)
    }
}
