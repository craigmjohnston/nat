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

    init(
        plan: ProjectInfo,
        agents: [AgentStatus],
        refusal: String? = nil,
        quietFirstReading: Bool = false
    ) {
        self.plan = plan
        self.agents = agents
        self.refusal = refusal
        self.quietFirstReading = quietFirstReading
        super.init(response: .agents(agents))
    }

    /// The slices whose sessions this client was asked to kill, in order.
    var kills: [String] { lock.withLock { killed } }

    override func info(projectID: String) async throws -> ProjectInfo { plan }

    override func status() async throws -> [AgentStatus] {
        let first = lock.withLock { () -> Bool in
            readings += 1
            return readings == 1
        }
        return quietFirstReading && first ? [] : agents
    }

    override func agentKill(projectID: String, sliceRef: String) async throws {
        lock.withLock { killed.append(sliceRef) }
        if let refusal { throw NatError.commandFailed(refusal) }
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
