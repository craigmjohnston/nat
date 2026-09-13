import XCTest
@testable import NatKit

/// A client that answers a plan and a tmux reading, and remembers every
/// session it was asked to kill — the two halves of the reaper, and what it
/// did with them.
final class ReapingClient: MockActivityClient, @unchecked Sendable {
    private let lock = NSLock()
    private let plan: ProjectInfo
    private let agents: [AgentStatus]
    private let refusal: String?
    /// Whether the very first reading is empty — a session launched after
    /// the app started, so the start's own sweep has nothing to find.
    private let quietFirstReading: Bool
    private var killed: [String] = []
    private var readings = 0

    /// What `prView` answers with — a state, or nothing at all for a gh that
    /// could not be asked.
    private let prState: String?
    private var prReads: [String] = []

    init(
        plan: ProjectInfo,
        agents: [AgentStatus],
        refusal: String? = nil,
        quietFirstReading: Bool = false,
        prState: String? = PRLifecycleState.merged
    ) {
        self.plan = plan
        self.agents = agents
        self.refusal = refusal
        self.quietFirstReading = quietFirstReading
        self.prState = prState
        super.init(response: .agents(agents))
    }

    /// The slices whose sessions this client was asked to kill, in order.
    var kills: [String] { lock.withLock { killed } }

    /// The slices whose pull requests this client was asked to read.
    var pullRequestReads: [String] { lock.withLock { prReads } }

    override func info(projectID: String) async throws -> ProjectInfo { plan }

    override func status() async throws -> [AgentStatus] {
        let first = lock.withLock { () -> Bool in
            readings += 1
            return readings == 1
        }
        return quietFirstReading && first ? [] : agents
    }

    override func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        lock.withLock { prReads.append(sliceRef) }
        guard let prState else { throw NatError.commandFailed("no pull request found") }
        return PRDetail(
            number: 1, title: "t", body: "", state: prState, isDraft: false,
            author: "craig", baseRefName: "main", headRefName: "slice/x",
            url: "https://github.test/craig/nat/pull/1",
            reviewDecision: "APPROVED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN"
        )
    }

    override func agentKill(projectID: String, sliceRef: String) async throws {
        lock.withLock { killed.append(sliceRef) }
        if let refusal { throw NatError.commandFailed(refusal) }
    }
}

final class AgentReapingTests: XCTestCase {
    private static let now = Date(timeIntervalSince1970: 1_700_000_000)

    private static func slice(_ id: String, status: String, pr: String = "") -> Slice {
        Slice(
            id: id, name: id, status: status, milestoneID: "m-1",
            assignee: "", pr: pr, url: "", blocked: false, handedBack: false
        )
    }

    private static func plan(_ slices: [Slice]) -> ProjectInfo {
        ProjectInfo(
            project: Project(id: "proj-1", name: "Test", conventions: ""),
            milestones: [Milestone(id: "m-1", name: "M1", order: 1, status: "Active")],
            slices: slices
        )
    }

    private static let config = NatProjectConfig(
        projects: ["proj-1": ProjectConfig(name: "Project", slicesDSID: "ds-1", workingDir: "/path")]
    )

    @MainActor
    private func startedModel(client: ReapingClient, grace: TimeInterval = 300) async -> AppModel {
        let model = AppModel(
            configReader: MockConfigReader(response: .success(Self.config)),
            planCache: NullTestPlanCache(),
            pollIntervalSeconds: 3600,
            pathsProvider: { NatPaths(config: "/fake/config.json", logDir: "/fake", nudge: "/fake/nudge") },
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            now: { Self.now },
            reapGrace: grace
        )
        await model.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return model
    }

    /// The app starting on a project whose finished slice still has a session
    /// running: nothing selected it this run, so it is dangling and goes.
    @MainActor
    func testStartReapsADanglingSessionOnAFinishedSlice() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testStartLeavesAnUnfinishedSlicesSessionAlone() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "In progress")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, [])
    }

    /// Clicking away from a finished slice starts its grace period rather
    /// than killing it on the spot — the very next sweep leaves it alone.
    @MainActor
    func testASliceJustClickedAwayFromKeepsItsSessionForTheGracePeriod() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done"), Self.slice("s-2", status: "Todo")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            quietFirstReading: true
        )
        let model = await startedModel(client: client)
        model.selectedSliceID = "s-1"
        model.selectedSliceID = "s-2"

        await model.refresh()

        XCTAssertEqual(client.kills, [])
    }

    /// The same slice, once the grace period is nothing: clicking away is
    /// what the sweep was waiting for.
    @MainActor
    func testASliceClickedAwayFromIsReapedOnceItsGraceIsUp() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done"), Self.slice("s-2", status: "Todo")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            quietFirstReading: true
        )
        let model = await startedModel(client: client, grace: 0)
        model.selectedSliceID = "s-1"
        model.selectedSliceID = "s-2"

        await model.refresh()

        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testTheSliceOnScreenSurvivesTheSweep() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)]
        )
        let model = await startedModel(client: client, grace: 0)
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
            plan: Self.plan([Self.slice("s-1", status: "Done")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            refusal: "no live session for s-1"
        )

        let model = await startedModel(client: client)
        await model.refresh()

        XCTAssertEqual(client.kills, ["s-1", "s-1"])
    }

    // MARK: - The pull request has the last word

    private static let prURL = "https://github.test/craig/nat/pull/1"

    @MainActor
    func testASessionIsReapedOnceItsPullRequestHasMerged() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done", pr: Self.prURL)]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            prState: PRLifecycleState.merged
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.pullRequestReads, ["s-1"])
        XCTAssertEqual(client.kills, ["s-1"])
    }

    /// A pull request closed without merging is over too — nobody is waiting
    /// on it, and there is no review left for the session to answer.
    @MainActor
    func testASessionIsReapedOnceItsPullRequestIsClosed() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done", pr: Self.prURL)]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            prState: PRLifecycleState.closed
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, ["s-1"])
    }

    /// The slice reads Done and no readiness listing said otherwise — but
    /// GitHub says the pull request is open, and that is the word that counts.
    @MainActor
    func testASessionSurvivesAPullRequestStillOpen() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done", pr: Self.prURL)]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            prState: "OPEN"
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.pullRequestReads, ["s-1"])
        XCTAssertEqual(client.kills, [])
    }

    /// No gh, no authentication, no network: the one reading standing between
    /// a session and a kill did not happen, so nothing is killed.
    @MainActor
    func testASessionSurvivesAPullRequestNobodyCouldRead() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done", pr: Self.prURL)]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)],
            prState: nil
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.kills, [])
    }

    /// A finished slice that never produced a pull request — a docs or
    /// research slice — has no merge coming and is asked nothing.
    @MainActor
    func testASliceWithNoPullRequestIsReapedWithoutAskingGitHub() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "Done")]),
            agents: [AgentStatus(sliceID: "s-1", session: "nat-s-1", activity: .waiting)]
        )

        _ = await startedModel(client: client)

        XCTAssertEqual(client.pullRequestReads, [])
        XCTAssertEqual(client.kills, ["s-1"])
    }

    @MainActor
    func testKillAgentNamesTheSlice() async {
        let client = ReapingClient(
            plan: Self.plan([Self.slice("s-1", status: "In progress")]),
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
            plan: Self.plan([Self.slice("s-1", status: "In progress")]),
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
}

/// A plan cache that remembers nothing, so these tests never read or write a
/// real project's cached plan.
struct NullTestPlanCache: PlanCaching {
    func read(projectID: String) async -> ProjectInfo? { nil }
    func write(_ info: ProjectInfo, projectID: String) async {}
}
