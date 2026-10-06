import XCTest
@testable import NatKit
@testable import NatFixtures

/// The calls a `HeldStatusClient` pair saw, in order, across both of them.
private final class CallLog: @unchecked Sendable {
    private let lock = NSLock()
    private var entries: [String] = []

    func record(_ entry: String) { lock.withLock { entries.append(entry) } }
    var calls: [String] { lock.withLock { entries } }
}

/// A client whose `status` is held open until the test answers it — the
/// activity poll's first reading at startup, caught before it lands — and
/// whose `info` answers the fixture plan a moment later, recording when it
/// completes. `held` false answers `status` at once with no agents (the
/// reaper's own reading, which `start` awaits).
private final class HeldStatusClient: MockActivityClient, @unchecked Sendable {
    private let lock = NSLock()
    private var reply: Result<[AgentStatus], Error>?
    private let held: Bool
    let log: CallLog

    init(log: CallLog, held: Bool) {
        self.log = log
        self.held = held
        super.init(response: .agents([]))
    }

    func answer(_ reply: Result<[AgentStatus], Error>) {
        lock.withLock { self.reply = reply }
    }

    override func info(projectID: String) async throws -> ProjectInfo {
        try await Task.sleep(nanoseconds: 50_000_000)
        log.record("info done")
        return Fixtures.projectInfo
    }

    override func status() async throws -> [AgentStatus] {
        guard held else { return [] }
        log.record("status")
        while true {
            if let reply = lock.withLock({ reply }) {
                lock.withLock { self.reply = nil }
                return try reply.get()
            }
            try await Task.sleep(nanoseconds: 5_000_000)
        }
    }
}

/// A launched workshop restored at startup: drawn from the last run's
/// request, "Reconnecting…", until the activity poll's first reading lands.
@MainActor
final class WorkshopReconnectTests: XCTestCase {
    private let projectID = Fixtures.projectID

    private var planner: AgentStatus {
        AgentStatus(
            sliceID: TmuxSession.planTag(projectID: projectID),
            session: TmuxSession.planSessionName(projectID: projectID), activity: .waiting)
    }

    /// A started model over a snapshot whose workshop was running at quit,
    /// with its activity client's `status` held.
    private func started() async -> (AppModel, HeldStatusClient) {
        let log = CallLog()
        let activity = HeldStatusClient(log: log, held: true)
        let other = HeldStatusClient(log: log, held: false)
        let model = AppModel(
            configReader: FixtureConfigReader(config: NatProjectConfig(
                projects: [projectID: ProjectConfig(name: "Fixture", slicesDSID: "ds", workingDir: "/tmp")])),
            planCache: NullPlanCache(),
            pollIntervalSeconds: 3600,
            pathsProvider: { Fixtures.paths },
            clientFactory: { other },
            activityStoreFactory: { ActivityStore(client: activity) },
            usageStoreFactory: { UsageStore(client: other, cache: NullUsageCache()) },
            toolsReady: { false },
            workshopCache: InMemoryWorkshopCache(WorkshopSnapshot(
                workshops: [projectID: .init(request: "Split the importer.")])),
            workshopSaveWait: { await Task.yield() })
        await model.start(configPath: Fixtures.paths.config, nudgePath: Fixtures.paths.nudge)
        model.openWorkshop()
        return (model, activity)
    }

    private func workshopRows(_ model: AppModel) -> [SidebarActiveRow] {
        model.sidebarModel.active.filter { $0.kind == .workshop }
    }

    private func waitUntil(_ condition: () -> Bool) async {
        for _ in 0..<400 where !condition() {
            try? await Task.sleep(nanoseconds: 5_000_000)
        }
    }

    func testTheRowIsThereAtOnceAsReconnecting() async {
        let (model, activity) = await started()
        defer { model.activityStore?.stop() }

        XCTAssertEqual(model.activityStore?.hasRead, false)
        let rows = workshopRows(model)
        XCTAssertEqual(rows.map(\.projectID), [projectID])
        XCTAssertEqual(rows.first?.reconnecting, true)
        XCTAssertEqual(rows.first?.live, false)
        XCTAssertTrue(model.workshopReconnecting)
        XCTAssertTrue(model.workshopLaunched)
        XCTAssertEqual(model.workshopTabs, [.terminal])
        XCTAssertFalse(model.isWorkshopPinned(projectID), "selecting a reconnecting workshop does not pin it")
        XCTAssertEqual(
            buildWorkshopEntry(activity: nil, isLaunching: false, isReconnecting: model.workshopReconnecting)?.displayState,
            "Reconnecting…")
        XCTAssertEqual(
            activity.log.calls.firstIndex(of: "status").map { $0 < (activity.log.calls.firstIndex(of: "info done") ?? 0) },
            true, "status asked before the plan read completes: \(activity.log.calls)")
    }

    func testAReadingHoldingTheAgentMakesTheRowLive() async {
        let (model, activity) = await started()
        defer { model.activityStore?.stop() }

        activity.answer(.success([planner]))
        await waitUntil { model.activityStore?.hasRead == true }

        XCTAssertFalse(model.workshopReconnecting)
        XCTAssertNotNil(model.planningAgent)
        XCTAssertTrue(model.workshopLaunched)
        let rows = workshopRows(model)
        XCTAssertEqual(rows.map(\.live), [true])
        XCTAssertEqual(rows.first?.reconnecting, false)
        XCTAssertEqual(rows.first?.state, .waiting)
        XCTAssertEqual(
            buildWorkshopEntry(activity: model.planningAgent.map { AgentActivity($0.activity) }, isLaunching: false)?
                .displayState,
            "Waiting for input")
        XCTAssertEqual(model.workshopRequest, "Split the importer.")
    }

    func testAReadingWithoutTheAgentEndsIt() async {
        let (model, activity) = await started()
        defer { model.activityStore?.stop() }

        activity.answer(.success([]))
        await waitUntil { model.activityStore?.hasRead == true }

        XCTAssertFalse(model.workshopReconnecting)
        XCTAssertTrue(workshopRows(model).isEmpty)
        XCTAssertNil(model.workshopRequest)
        XCTAssertFalse(model.workshopLaunched)
    }

    func testAFailedReadingLeavesItReconnecting() async {
        let (model, activity) = await started()
        defer { model.activityStore?.stop() }

        activity.answer(.failure(TestError()))
        try? await Task.sleep(nanoseconds: 100_000_000)

        XCTAssertEqual(model.activityStore?.hasRead, false)
        XCTAssertTrue(model.workshopReconnecting)
        XCTAssertEqual(workshopRows(model).first?.reconnecting, true)
    }

    func testWorkshopEndConfirmationReadsTheTab() async {
        let (model, activity) = await started()
        defer { model.activityStore?.stop() }
        XCTAssertNil(model.workshopEndConfirmation(forTab: projectID), "no agent read yet")

        activity.answer(.success([AgentStatus(
            sliceID: planner.sliceID, session: planner.session, activity: .working)]))
        await waitUntil { model.planningAgent != nil }

        XCTAssertEqual(model.workshopEndConfirmation(forTab: projectID), WorkshopEndRules.workingMessage)
    }
}
