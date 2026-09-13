import XCTest
@testable import NatKit

/// A client that answers a plan per project, a tmux reading, a fresh
/// `nat slice-status` per candidate, and remembers every session it was
/// asked to kill — the whole of what the reaper reads and does.
final class ReapingClient: MockActivityClient, @unchecked Sendable {
    private let lock = NSLock()
    private let plans: [String: ProjectInfo]
    private let agents: [AgentStatus]
    private let refusal: String?
    /// Whether `status()` reports any agent at all yet — false starts the
    /// client quiet, as a test wants when it needs to visit a slice before
    /// any sweep can see its session live; `reveal()` is what a test calls
    /// once it is done setting up.
    private var revealed: Bool
    private var killed: [String] = []
    /// Sessions killed without a refusal: the mock's stand-in for tmux no
    /// longer holding them, so a later reading does not go on reporting a
    /// session that is actually gone.
    private var killedCleanly: Set<String> = []

    /// What `sliceStatus` answers per slice ID — absent means the read
    /// fails, as a `nat` that could not be run would.
    private let statuses: [String: SliceStatusResult]
    private var statusReads: [String] = []

    init(
        plans: [String: ProjectInfo],
        agents: [AgentStatus],
        refusal: String? = nil,
        startsQuiet: Bool = false,
        statuses: [String: SliceStatusResult] = [:]
    ) {
        self.plans = plans
        self.agents = agents
        self.refusal = refusal
        self.revealed = !startsQuiet
        self.statuses = statuses
        super.init(response: .agents(agents))
    }

    /// The slices whose sessions this client was asked to kill, in order.
    var kills: [String] { lock.withLock { killed } }

    /// The slices whose status this client was asked to read fresh, in order.
    var statusReadIDs: [String] { lock.withLock { statusReads } }

    /// Lets `status()` report the live agents from here on — the moment a
    /// session a test built quiet is meant to appear.
    func reveal() { lock.withLock { revealed = true } }

    override func info(projectID: String) async throws -> ProjectInfo {
        guard let info = plans[projectID] else {
            throw NatError.commandFailed("no plan for \(projectID)")
        }
        return info
    }

    override func status() async throws -> [AgentStatus] {
        let (isRevealed, done) = lock.withLock { (revealed, killedCleanly) }
        guard isRevealed else { return [] }
        return agents.filter { !done.contains($0.sliceID) }
    }

    override func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult {
        lock.withLock { statusReads.append(sliceRef) }
        guard let result = statuses[sliceRef] else {
            throw NatError.commandFailed("no status for \(sliceRef)")
        }
        return result
    }

    override func agentKill(projectID: String, sliceRef: String) async throws {
        lock.withLock { killed.append(sliceRef) }
        if let refusal { throw NatError.commandFailed(refusal) }
        lock.withLock { _ = killedCleanly.insert(sliceRef) }
    }
}

final class AgentReapingTests: XCTestCase {
    private static let now = Date(timeIntervalSince1970: 1_700_000_000)

    private static func slice(_ id: String, status: String) -> Slice {
        Slice(
            id: id, name: id, status: status, milestoneID: "m-1",
            assignee: "", pr: "", url: "", blocked: false, handedBack: false
        )
    }

    private static func plan(_ slices: [Slice]) -> ProjectInfo {
        ProjectInfo(
            project: Project(id: "proj-1", name: "Test", conventions: ""),
            milestones: [Milestone(id: "m-1", name: "M1", order: 1, status: "Active")],
            slices: slices
        )
    }

    private static func config(_ ids: String...) -> NatProjectConfig {
        var projects: [String: ProjectConfig] = [:]
        for id in ids {
            projects[id] = ProjectConfig(name: id, slicesDSID: "ds-\(id)", workingDir: "/path/\(id)")
        }
        return NatProjectConfig(projects: projects)
    }

    @MainActor
    private func startedModel(
        client: ReapingClient, config: NatProjectConfig = AgentReapingTests.config("proj-1"),
        visitHold: TimeInterval = 300
    ) async -> AppModel {
        let model = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            planCache: NullTestPlanCache(),
            pollIntervalSeconds: 3600,
            pathsProvider: { NatPaths(config: "/fake/config.json", logDir: "/fake", nudge: "/fake/nudge") },
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            now: { Self.now },
            visitHold: visitHold
        )
        await model.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return model
    }

    // MARK: - The candidate rule, wired through the app

    /// The app starting on a project whose finished slice still has a session
    /// running: nothing visited it this run, so it is dangling and goes.
    @MainActor
    func testStartReapsADanglingSessionOnAFinishedSlice() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .found(status: "Done", trashed: false)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testStartLeavesAnUnfinishedSlicesSessionAlone() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "In progress")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)]
        )

        _ = await startedModel(client: client)

        // Never a candidate at all, so nothing is even asked about.
        XCTAssertEqual(client.statusReadIDs, [])
        XCTAssertEqual(client.kills, [])
    }

    /// Visiting a finished slice holds its session for the hold period rather
    /// than killing it on the spot — the very next sweep leaves it alone.
    @MainActor
    func testASliceJustVisitedKeepsItsSessionForTheHoldPeriod() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done"), Self.slice("s-2", status: "Todo")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            startsQuiet: true,
            statuses: ["s-1": .found(status: "Done", trashed: false)]
        )
        let model = await startedModel(client: client)
        client.reveal()
        model.selectedSliceID = "s-1"
        model.selectedSliceID = nil

        await model.refresh()

        XCTAssertEqual(client.kills, [])
    }

    /// The same slice, once its hold is nothing: visiting it is what the
    /// sweep was waiting out.
    @MainActor
    func testASliceIsReapedOnceItsHoldHasExpired() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done"), Self.slice("s-2", status: "Todo")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            startsQuiet: true,
            statuses: ["s-1": .found(status: "Done", trashed: false)]
        )
        let model = await startedModel(client: client, visitHold: 0)
        client.reveal()
        model.selectedSliceID = "s-1"
        model.selectedSliceID = nil

        await model.refresh()

        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testTheSliceOnScreenSurvivesTheSweep() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .found(status: "Done", trashed: false)]
        )
        let model = await startedModel(client: client, visitHold: 0)
        // The start's own sweep killed it; select it and sweep again, which
        // is a session being looked at rather than a dangling one.
        model.selectedSliceID = "s-1"
        await model.refresh()

        XCTAssertEqual(client.kills, ["s-1"])
    }

    /// A sweep is nobody's key press, so a refusal is logged and the board is
    /// left exactly as it was — including the sweep trying again next time.
    @MainActor
    func testARefusedReapIsQuietAndTriedAgain() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            refusal: "no live session for s-1",
            statuses: ["s-1": .found(status: "Done", trashed: false)]
        )

        let model = await startedModel(client: client)
        await model.refresh()

        XCTAssertEqual(client.kills, ["s-1", "s-1"])
    }

    // MARK: - The fresh read has the last word

    /// A candidate is asked about fresh right before the kill, and Done is
    /// one of the answers that goes ahead with it.
    @MainActor
    func testASessionIsReapedWhenTheFreshReadSaysDone() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .found(status: "Done", trashed: false)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.statusReadIDs, ["s-1"])
        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testASessionIsReapedWhenTheFreshReadSaysTodo() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .found(status: "Todo", trashed: false)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, ["s-1"])
    }

    /// The plan reads Done, but the fresh read says In progress — closing the
    /// exact race this verification exists for: a claim is always written
    /// before the session that holds it exists, so this is what a session
    /// launched after the plan was last read looks like.
    @MainActor
    func testASessionSurvivesWhenTheFreshReadSaysInProgress() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .found(status: "In progress", trashed: false)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.statusReadIDs, ["s-1"])
        XCTAssertEqual(client.kills, [])
    }

    /// No `nat`, no network, no news: the one reading standing between a
    /// session and a kill did not happen, so nothing is killed.
    @MainActor
    func testASessionSurvivesWhenTheStatusReadFails() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: [:]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, [])
    }

    /// A page trashed for good is reaped, whatever the plan's own stale copy
    /// of its status says.
    @MainActor
    func testASliceReadAsTrashedIsReaped() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .found(status: "Done", trashed: true)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, ["s-1"])
    }

    /// A page Notion has no record of at all — one trashed for good — is
    /// reaped the same way.
    @MainActor
    func testASliceReadAsGoneIsReaped() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Done")])],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            statuses: ["s-1": .gone]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testKillAgentNamesTheSlice() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "In progress")])],
            agents: []
        )
        let model = await startedModel(client: client)

        let refusal = await model.killAgent(sliceID: "s-1")

        XCTAssertNil(refusal)
        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testKillAgentAnswersWithNatsOwnRefusal() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "In progress")])],
            agents: [],
            refusal: "no live session for s-1"
        )
        let model = await startedModel(client: client)

        let refusal = await model.killAgent(sliceID: "s-1")

        XCTAssertEqual(refusal, "no live session for s-1")
    }

    @MainActor
    func testKillAgentWithoutAProjectSaysSo() async {
        let model = AppModel()

        let refusal = await model.killAgent(sliceID: "s-1")

        XCTAssertEqual(refusal, "No project loaded")
    }

    // MARK: - Every open tab is swept, not only the active one

    /// A background tab's own In-progress slice is never treated as absent
    /// just because it is not the active tab — its plan is in the merge the
    /// same as any open tab's.
    @MainActor
    func testABackgroundTabsInProgressSliceIsNeverACandidate() async {
        let client = ReapingClient(
            plans: [
                "proj-1": Self.plan([Self.slice("s-1", status: "Todo")]),
                "proj-2": Self.plan([Self.slice("s-2", status: "In progress")]),
            ],
            agents: [AgentStatus(sliceID: "s-2", session: "nat-s-2", activity: .waiting)]
        )

        let model = await startedModel(client: client, config: Self.config("proj-1", "proj-2"))
        await model.activateProject("proj-2")
        await model.activateProject("proj-1")

        XCTAssertEqual(model.activeProjectID, "proj-1")
        XCTAssertEqual(client.kills, [])
    }

    /// A session dangling on a tab nobody has clicked back to since is
    /// exactly what this sweep exists to catch — it must not wait for the
    /// user to switch there again.
    @MainActor
    func testASweepReapsADanglingSessionOnABackgroundTab() async {
        let client = ReapingClient(
            plans: [
                "proj-1": Self.plan([Self.slice("s-1", status: "Todo")]),
                "proj-2": Self.plan([Self.slice("s-2", status: "Done")]),
            ],
            agents: [AgentStatus(sliceID: "s-2", session: "nat-s-2", activity: .waiting)],
            statuses: ["s-2": .found(status: "Done", trashed: false)]
        )

        // proj-2's plan is loaded once (as opening its tab would), then it
        // is left in the background — its tab is never switched to again.
        let model = await startedModel(client: client, config: Self.config("proj-1", "proj-2"))
        await model.activateProject("proj-2")
        await model.activateProject("proj-1")

        XCTAssertEqual(model.activeProjectID, "proj-1")
        XCTAssertEqual(client.kills, ["s-2"])
    }

    /// A candidate verified as belonging to a live In-progress session this
    /// run has no open tab for is cached, so a later sweep does not pay for
    /// asking about it again.
    @MainActor
    func testAVerifiedElsewhereCandidateIsNotReReadNextSweep() async {
        let client = ReapingClient(
            plans: ["proj-1": Self.plan([Self.slice("s-1", status: "Todo")])],
            agents: [AgentStatus(sliceID: "other-project-slice", session: "nat-other", activity: .waiting)],
            statuses: ["other-project-slice": .found(status: "In progress", trashed: false)]
        )

        // The start's own sweep reads it once and finds it belongs to a live
        // In-progress session no open tab here holds.
        let model = await startedModel(client: client)
        XCTAssertEqual(client.statusReadIDs, ["other-project-slice"])
        XCTAssertEqual(client.kills, [])

        // Every later sweep still nominates it — nothing here has changed —
        // but the cache is what keeps it from being asked about again.
        await model.refresh()
        await model.refresh()
        XCTAssertEqual(client.statusReadIDs, ["other-project-slice"])
    }

    // MARK: - Closing a tab

    /// Closing a project's tab runs one final sweep for it, ignoring visit
    /// holds entirely: once the tab is gone, its plan stops being one the
    /// ordinary sweep considers, so this is the last chance for a while.
    @MainActor
    func testClosingATabReapsItsOwnDanglingSessionIgnoringHolds() async {
        let client = ReapingClient(
            plans: [
                "proj-1": Self.plan([Self.slice("s-1", status: "Todo")]),
                "proj-2": Self.plan([Self.slice("s-2", status: "Done")]),
            ],
            agents: [AgentStatus(sliceID: "s-2", session: "nat-s-2", activity: .waiting)],
            // Quiet until revealed, so nothing sees the session live before
            // it is visited — visiting it is what sets its hold rather than
            // a sweep beating the selection to it.
            startsQuiet: true,
            statuses: ["s-2": .found(status: "Done", trashed: false)]
        )

        let model = await startedModel(client: client, config: Self.config("proj-1", "proj-2"))
        await model.activateProject("proj-2")
        client.reveal()
        // Visiting proj-2's slice holds its session — an ordinary sweep must
        // leave it alone from here.
        model.selectedSliceID = "s-2"
        await model.activateProject("proj-1")
        XCTAssertEqual(client.kills, [], "an ordinary sweep must respect the hold just set")

        await model.closeProject("proj-2")

        XCTAssertEqual(client.kills, ["s-2"])
    }

    /// Closing one tab's own sweep still merges every other open tab's plan
    /// in exactly the way the ordinary sweep does, so another project's live
    /// In-progress session is never swept up as though it belonged to the
    /// tab being closed.
    @MainActor
    func testClosingATabDoesNotTouchAnotherOpenProjectsSession() async {
        let client = ReapingClient(
            plans: [
                "proj-1": Self.plan([Self.slice("s-1", status: "In progress")]),
                "proj-2": Self.plan([Self.slice("s-2", status: "Todo")]),
            ],
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)]
        )

        let model = await startedModel(client: client, config: Self.config("proj-1", "proj-2"))

        await model.closeProject("proj-2")

        XCTAssertEqual(client.kills, [])
    }
}

/// A plan cache that remembers nothing, so these tests never read or write a
/// real project's cached plan.
struct NullTestPlanCache: PlanCaching {
    func read(projectID: String) async -> ProjectInfo? { nil }
    func write(_ info: ProjectInfo, projectID: String) async {}
}
