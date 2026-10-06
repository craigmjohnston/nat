import XCTest
@testable import NatKit
@testable import NatFixtures

/// What a `HeldStatusClient` pair shares: the calls both saw, in order, and
/// how the workshop's proposal reads and its Accept answers.
private final class Shared: @unchecked Sendable {
    private let lock = NSLock()
    private var entries: [String] = []
    private var proposalValue: PlanProposal?
    private var proposalFailsValue = false
    private var launchesValue: [String] = []

    func record(_ entry: String) { lock.withLock { entries.append(entry) } }
    var calls: [String] { lock.withLock { entries } }
    var proposal: PlanProposal? {
        get { lock.withLock { proposalValue } }
        set { lock.withLock { proposalValue = newValue } }
    }
    var proposalFails: Bool {
        get { lock.withLock { proposalFailsValue } }
        set { lock.withLock { proposalFailsValue = newValue } }
    }
    var launches: [String] { lock.withLock { launchesValue } }
    func launched(_ request: String) { lock.withLock { launchesValue.append(request) } }
}

/// A client whose `status` is held open until the test answers it — the
/// activity poll's readings, caught before they land — and whose `info`
/// answers the fixture plan a moment later, recording when it completes.
/// `held` false answers `status` at once with no agents (the reaper's own
/// reading, which `start` awaits). The project's proposal and its Accept
/// answer from `shared`.
private final class HeldStatusClient: MockActivityClient, @unchecked Sendable {
    private let lock = NSLock()
    private var reply: Result<[AgentStatus], Error>?
    private let held: Bool
    let shared: Shared

    init(shared: Shared, held: Bool) {
        self.shared = shared
        self.held = held
        super.init(response: .agents([]))
    }

    func answer(_ reply: Result<[AgentStatus], Error>) {
        lock.withLock { self.reply = reply }
    }

    override func info(projectID: String) async throws -> ProjectInfo {
        try await Task.sleep(nanoseconds: 50_000_000)
        shared.record("info done")
        return Fixtures.projectInfo
    }

    override func status() async throws -> [AgentStatus] {
        guard held else { return [] }
        shared.record("status")
        while true {
            if let reply = lock.withLock({ reply }) {
                lock.withLock { self.reply = nil }
                return try reply.get()
            }
            try await Task.sleep(nanoseconds: 5_000_000)
        }
    }

    override func planProposal(projectID: String) async throws -> PlanProposal? {
        if shared.proposalFails { throw TestError() }
        return shared.proposal
    }

    override func planAccept(projectID: String) async throws -> PlanAccepted {
        shared.proposal = nil
        return PlanAccepted(project: ProjectEntry(id: projectID, name: "Fixture"), milestones: 1, slices: 1)
    }
}

/// A launched workshop restored at startup — drawn from the last run's
/// request, "Reconnecting…", until the activity poll's first reading lands —
/// and a workshop whose agent has ended, degraded so nothing unsaved is lost.
@MainActor
final class WorkshopReconnectTests: XCTestCase {
    private let projectID = Fixtures.projectID

    private var planner: AgentStatus {
        AgentStatus(
            sliceID: TmuxSession.planTag(projectID: projectID),
            session: TmuxSession.planSessionName(projectID: projectID), activity: .waiting)
    }

    /// A started model over a snapshot whose workshop was running at quit
    /// (`kept`), with its activity client's `status` held.
    private func started(
        _ kept: WorkshopSnapshot.Workshop = .init(request: "Split the importer."),
        proposal: PlanProposal? = nil, proposalFails: Bool = false
    ) async -> (AppModel, HeldStatusClient) {
        let shared = Shared()
        shared.proposal = proposal
        shared.proposalFails = proposalFails
        let activity = HeldStatusClient(shared: shared, held: true)
        let other = HeldStatusClient(shared: shared, held: false)
        let planner = self.planner
        let model = AppModel(
            configReader: FixtureConfigReader(config: NatProjectConfig(
                projects: [projectID: ProjectConfig(name: "Fixture", slicesDSID: "ds", workingDir: "/tmp")])),
            planCache: NullPlanCache(),
            pollIntervalSeconds: 3600,
            pathsProvider: { Fixtures.paths },
            workshopLauncher: { projectID, _, _, request in
                shared.launched(request ?? "")
                activity.answer(.success([planner]))
                return WorkshopLaunchResult(session: TmuxSession.planSessionName(projectID: projectID), workdir: "/tmp")
            },
            clientFactory: { other },
            activityStoreFactory: { ActivityStore(client: activity) },
            usageStoreFactory: { UsageStore(client: other, cache: NullUsageCache()) },
            launchSettleWait: { try? await Task.sleep(nanoseconds: 5_000_000) },
            toolsReady: { false },
            workshopCache: InMemoryWorkshopCache(WorkshopSnapshot(workshops: [projectID: kept])),
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
            activity.shared.calls.firstIndex(of: "status").map { $0 < (activity.shared.calls.firstIndex(of: "info done") ?? 0) },
            true, "status asked before the plan read completes: \(activity.shared.calls)")
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

    // MARK: - An agent found gone

    func testAnAgentGoneWithNothingProposedHandsBackTheComposerWithTheBrief() async {
        let (model, activity) = await started(.init(draft: "Split the importer, please.", request: "Split the importer."))
        defer { model.activityStore?.stop() }

        activity.answer(.success([]))
        await waitUntil { model.workshopRequest == nil }

        XCTAssertFalse(model.workshopReconnecting)
        XCTAssertFalse(model.workshopLaunched, "the composer, not a terminal")
        XCTAssertEqual(model.workshopDraft, "Split the importer, please.")
        XCTAssertTrue(model.isWorkshopPinned(projectID))
        XCTAssertEqual(workshopRows(model).map(\.live), [false])
        XCTAssertEqual(workshopRows(model).first?.reconnecting, false)
    }

    func testAWorkshopKeptWithoutADraftGetsItsRequestBackAsOne() async {
        let (model, activity) = await started()
        defer { model.activityStore?.stop() }

        activity.answer(.success([]))
        await waitUntil { model.workshopRequest == nil }

        XCTAssertEqual(model.workshopDraft, "Split the importer.")
    }

    func testAnAgentGoneWithAPlanUpKeepsThePlanInFront() async {
        let (model, activity) = await started(proposal: Fixtures.proposal)
        defer { model.activityStore?.stop() }

        activity.answer(.success([]))
        await waitUntil { model.workshopEnded }

        XCTAssertTrue(model.workshopEnded)
        XCTAssertTrue(model.workshopLaunched)
        XCTAssertEqual(model.workshopTabs, [.terminal, .plan])
        XCTAssertEqual(model.workshopTab, .plan)
        XCTAssertEqual(model.workshopRequest, "Split the importer.")
        XCTAssertEqual(workshopRows(model).map(\.planReady), [true])
        XCTAssertNil(model.workshopEndConfirmation(forTab: projectID), "no agent to end")
    }

    func testKeepWorkshoppingOnAnEndedWorkshopStartsANewAgentOnThePlan() async {
        let (model, activity) = await started(proposal: Fixtures.proposal)
        defer { model.activityStore?.stop() }
        activity.answer(.success([]))
        await waitUntil { model.workshopEnded }

        await model.continueEndedWorkshop()

        let sent = activity.shared.launches
        XCTAssertEqual(sent.count, 1)
        XCTAssertTrue(sent.first?.hasPrefix("Split the importer.\n\n") == true, sent.first ?? "")
        XCTAssertTrue(
            sent.first?.contains("`nat plan-proposal --project \(projectID) --json`") == true, sent.first ?? "")
        XCTAssertEqual(model.workshopRequest, "Split the importer.", "the Brief shows what was first sent")
        XCTAssertNotNil(model.planningAgent)
        XCTAssertFalse(model.workshopEnded)
        XCTAssertFalse(model.isWorkshopPinned(projectID), "the live agent's row takes over")
    }

    func testAnAgentGoneAfterAnAcceptedPlanTrashesTheWorkshop() async {
        let (model, activity) = await started(.init(draft: "Kept.", request: "Split the importer.", accepted: true))
        defer { model.activityStore?.stop() }

        activity.answer(.success([]))
        await waitUntil { model.workshopRequest == nil }

        XCTAssertTrue(workshopRows(model).isEmpty)
        XCTAssertEqual(model.workshopDraft, "")
        XCTAssertNil(model.workshopSnapshot.workshops[projectID])
    }

    func testAProposalThatWillNotReadKeepsEverything() async {
        let (model, activity) = await started(
            .init(draft: "Kept.", request: "Split the importer.", accepted: true), proposalFails: true)
        defer { model.activityStore?.stop() }

        activity.answer(.success([]))
        await waitUntil { !model.workshopReconnecting }
        try? await Task.sleep(nanoseconds: 50_000_000)

        XCTAssertEqual(model.workshopRequest, "Split the importer.")
        XCTAssertEqual(model.workshopDraft, "Kept.")
        XCTAssertTrue(model.isWorkshopPinned(projectID))
        XCTAssertEqual(model.workshopSnapshot.workshops[projectID]?.accepted, true)
    }

    func testAnAgentEndingWhileTheAppRunsDegradesTheSameWay() async {
        let (model, activity) = await started(.init(draft: "Split it.", request: "Split the importer."))
        defer { model.activityStore?.stop() }
        activity.answer(.success([planner]))
        await waitUntil { model.planningAgent != nil }
        XCTAssertFalse(model.isWorkshopPinned(projectID))

        activity.answer(.success([]))
        model.activityStore?.reread()
        await waitUntil { model.workshopRequest == nil }

        XCTAssertNil(model.planningAgent)
        XCTAssertEqual(model.workshopDraft, "Split it.")
        XCTAssertTrue(model.isWorkshopPinned(projectID))
    }

    func testAcceptingMarksTheSessionAndItsEndThenTrashesIt() async {
        let (model, activity) = await started(proposal: Fixtures.proposal)
        defer { model.activityStore?.stop() }
        activity.answer(.success([planner]))
        await waitUntil { model.planningAgent != nil }
        await model.refreshProposals()
        XCTAssertNotNil(model.activeProposal)

        await model.acceptProposal()
        XCTAssertEqual(model.workshopSnapshot.workshops[projectID]?.accepted, true)
        XCTAssertEqual(model.workshopRequest, "Split the importer.", "the session goes on")

        activity.answer(.success([]))
        model.activityStore?.reread()
        await waitUntil { model.workshopRequest == nil }

        XCTAssertTrue(workshopRows(model).isEmpty)
        XCTAssertNil(model.workshopSnapshot.workshops[projectID])
    }

    func testAcceptingAPlanWhoseAgentHasEndedTrashesTheWorkshopAtOnce() async {
        let (model, activity) = await started(proposal: Fixtures.proposal)
        defer { model.activityStore?.stop() }
        activity.answer(.success([]))
        await waitUntil { model.workshopEnded }

        await model.acceptProposal()

        XCTAssertNil(model.workshopRequest)
        XCTAssertTrue(workshopRows(model).isEmpty)
    }

    func testANewLaunchClearsAnAcceptedMark() async {
        let (model, activity) = await started(.init(draft: "Again.", accepted: true))
        defer { model.activityStore?.stop() }

        await model.launchWorkshop(request: model.workshopDraft)
        _ = activity

        XCTAssertNil(model.workshopSnapshot.workshops[projectID]?.accepted)
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
