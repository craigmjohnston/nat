import XCTest
@testable import NatKit

// MARK: - Mock Config Reader

final class MockConfigReader: ConfigReaderProtocol, @unchecked Sendable {
    enum Response: Sendable {
        case success(NatProjectConfig)
        case failure
    }

    private var response: Response
    /// The path most recently asked for, so `reloadConfig()`'s own re-read
    /// can be checked against `start()`'s.
    private(set) var lastPath: String?

    init(response: Response) {
        self.response = response
    }

    /// Swaps the response a later `readConfig` returns — what
    /// `reloadConfig()`'s tests use to answer differently the second time.
    func setResponse(_ response: Response) {
        self.response = response
    }

    func readConfig(from path: String) async throws -> NatProjectConfig {
        lastPath = path
        switch response {
        case .success(let config):
            return config
        case .failure:
            throw NSError(domain: "test", code: -1, userInfo: nil)
        }
    }
}

// MARK: - Workshop launch recorder

/// Records the calls an injected workshop launcher receives — a class behind
/// a lock, since the launcher closure is `@Sendable`.
final class WorkshopLaunchRecorder: @unchecked Sendable {
    private let lock = NSLock()
    private(set) var calls: [(projectID: String, model: String?, effort: String?, request: String?)] = []

    func record(projectID: String, model: String?, effort: String?, request: String?) {
        lock.lock()
        defer { lock.unlock() }
        calls.append((projectID, model, effort, request))
    }
}

// MARK: - Workshop activity

/// A status client that reports nothing until the launch has run and the
/// planning agent from then on — which is what a workshop launch does to the
/// tmux server the real poll reads. `launched()` is called by the launcher
/// `workshopModel` wraps around the test's own, so the reading turns over at
/// exactly the moment the session starts existing.
final class PlanningAgentAppearsClient: MockActivityClient, @unchecked Sendable {
    private let lock = NSLock()
    private var isLaunched = false

    init() {
        super.init(response: .agents([]))
    }

    func launched() {
        lock.withLock { isLaunched = true }
    }

    override func status() async throws -> [AgentStatus] {
        guard lock.withLock({ isLaunched }) else { return [] }
        // `TmuxSession.planTag(projectID: "proj-a")` and
        // `TmuxSession.planSessionName(projectID: "proj-a")`, spelled out
        // because this is not the main actor — the poll reads tmux off it.
        return [AgentStatus(sliceID: "plan:proj-a", session: "nat-plan-a", activity: .working)]
    }
}

/// Somewhere for a settle-wait closure to reach the model it is waiting
/// inside — the closure is made before the model it is given to.
@MainActor
final class WorkshopLaunchProbe {
    var model: AppModel?
    /// What `workshopLaunching` read on each turn of the settle wait.
    var launching: [Bool] = []

    func observe() {
        launching.append(model?.workshopLaunching ?? false)
    }
}

// MARK: - Tests

final class AppModelTests: XCTestCase {
    @MainActor
    func testAppModel_initialState() {
        let appModel = AppModel()

        XCTAssertNil(appModel.config)
        XCTAssertNil(appModel.projectStore)
        XCTAssertNil(appModel.selectedSliceID)
    }

    @MainActor
    func testAppModel_loadConfigSuccess() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")
            ],
            agentSplitPercent: nil,
            pollSeconds: nil
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertNotNil(appModel.config)
        XCTAssertEqual(appModel.config?.projects.count, 1)
        XCTAssertNotNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_loadConfigFailure() async {
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertNil(appModel.config)
        XCTAssertNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_selectsFirstProject() async {
        let testConfig = NatProjectConfig(
            projects: [
                "z-proj": ProjectConfig(name: "Z Project", slicesDSID: "ds-z", workingDir: "/path/z"),
                "a-proj": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Should pick "a-proj" (first when sorted alphabetically)
        XCTAssertEqual(appModel.projectStore?.projectID, "a-proj")
    }

    @MainActor
    func testAppModel_refreshPublic() async {
        _ = Project(id: "proj-1", name: "Test", conventions: "")
        let testConfig = NatProjectConfig(
            projects: [
                "proj-1": ProjectConfig(name: "Project", slicesDSID: "ds-1", workingDir: "/path")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // refresh should not throw
        await appModel.refresh()

        XCTAssertNotNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_refreshWithoutProjectStore() async {
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // refresh should not crash
        await appModel.refresh()

        XCTAssertNil(appModel.projectStore)
    }

    // MARK: - Multi-Project Tests

    @MainActor
    func testAppModel_multipleProjects() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-z": ProjectConfig(name: "Z Project", slicesDSID: "ds-z", workingDir: "/path/z"),
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-m": ProjectConfig(name: "M Project", slicesDSID: "ds-m", workingDir: "/path/m")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Should create tabs sorted by project ID
        XCTAssertEqual(appModel.projectTabs.count, 3)
        XCTAssertEqual(appModel.projectTabs[0].id, "proj-a")
        XCTAssertEqual(appModel.projectTabs[0].name, "A Project")
        XCTAssertEqual(appModel.projectTabs[1].id, "proj-m")
        XCTAssertEqual(appModel.projectTabs[2].id, "proj-z")
    }

    @MainActor
    func testAppModel_activatesFirstProject() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-z": ProjectConfig(name: "Z Project", slicesDSID: "ds-z", workingDir: "/path/z"),
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Should activate the first project alphabetically
        XCTAssertEqual(appModel.activeProjectID, "proj-a")
        XCTAssertNotNil(appModel.projectStore)
        XCTAssertEqual(appModel.projectStore?.projectID, "proj-a")
    }

    // MARK: - Close Tab Tests

    @MainActor
    private func threeProjectModel() async -> AppModel {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b"),
                "proj-c": ProjectConfig(name: "C Project", slicesDSID: "ds-c", workingDir: "/path/c")
            ]
        )
        let appModel = AppModel(configReader: MockConfigReader(response: .success(testConfig)))
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return appModel
    }

    @MainActor
    func testCloseProject_removesTheTab() async {
        let appModel = await threeProjectModel()

        await appModel.closeProject("proj-b")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-a", "proj-c"])
    }

    @MainActor
    func testCloseProject_inactiveTabLeavesTheActiveOneAlone() async {
        let appModel = await threeProjectModel()

        await appModel.closeProject("proj-c")

        XCTAssertEqual(appModel.activeProjectID, "proj-a")
    }

    @MainActor
    func testCloseProject_activeTabActivatesTheTabAfterIt() async {
        let appModel = await threeProjectModel()

        await appModel.closeProject("proj-a")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-b", "proj-c"])
        XCTAssertEqual(appModel.activeProjectID, "proj-b")
    }

    @MainActor
    func testCloseProject_lastPositionFallsBackToTheTabBefore() async {
        let appModel = await threeProjectModel()
        await appModel.activateProject("proj-c")

        await appModel.closeProject("proj-c")

        XCTAssertEqual(appModel.activeProjectID, "proj-b")
    }

    @MainActor
    func testCloseProject_refusesTheLastTab() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )
        let appModel = AppModel(configReader: MockConfigReader(response: .success(testConfig)))
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        await appModel.closeProject("proj-a")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-a"])
        XCTAssertEqual(appModel.activeProjectID, "proj-a")
    }

    @MainActor
    func testCloseProject_unknownIDChangesNothing() async {
        let appModel = await threeProjectModel()

        await appModel.closeProject("proj-nope")

        XCTAssertEqual(appModel.projectTabs.count, 3)
        XCTAssertEqual(appModel.activeProjectID, "proj-a")
    }

    @MainActor
    func testAppModel_perProjectSelectedSliceID() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Set a selected slice for the active project
        appModel.selectedSliceID = "slice-1"
        XCTAssertEqual(appModel.selectedSliceID, "slice-1")

        // Switch to another project
        await appModel.activateProject("proj-b")
        XCTAssertEqual(appModel.activeProjectID, "proj-b")
        // Selected slice should be nil for the new project
        XCTAssertNil(appModel.selectedSliceID)

        // Switch back to the first project
        await appModel.activateProject("proj-a")
        XCTAssertEqual(appModel.activeProjectID, "proj-a")
        // Should restore the previously selected slice
        XCTAssertEqual(appModel.selectedSliceID, "slice-1")
    }

    @MainActor
    func testAppModel_attentionCalculation() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // A project whose plan has not landed reads as nothing at all: no
        // pill, and the neutral dot.
        XCTAssertEqual(appModel.attention(projectID: "proj-a"), .none)
        XCTAssertNil(appModel.attention(projectID: "proj-a").badge)
    }

    @MainActor
    func testAppModel_lazyLoadingOfProjectStores() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // proj-a should be loaded (active)
        XCTAssertNotNil(appModel.projectStore)
        XCTAssertEqual(appModel.projectStore?.projectID, "proj-a")

        // Activate proj-b
        await appModel.activateProject("proj-b")

        // Now proj-b should be loaded
        XCTAssertEqual(appModel.projectStore?.projectID, "proj-b")
    }

    // MARK: - Per-project detail/diff/PR stores

    @MainActor
    func testAppModel_sliceDetailStoreIsCreatedOnceAndReused() {
        let appModel = AppModel()

        let first = appModel.sliceDetailStore(projectID: "proj-1")
        let second = appModel.sliceDetailStore(projectID: "proj-1")

        XCTAssertTrue(first === second, "the same project should always get the same cache, not a fresh one per call")
    }

    @MainActor
    func testAppModel_sliceDetailStoreIsSeparatePerProject() {
        let appModel = AppModel()

        let a = appModel.sliceDetailStore(projectID: "proj-a")
        let b = appModel.sliceDetailStore(projectID: "proj-b")

        XCTAssertFalse(a === b)
    }

    @MainActor
    func testAppModel_diffStoreIsCreatedOnceAndReused() {
        let appModel = AppModel()

        let first = appModel.diffStore(projectID: "proj-1")
        let second = appModel.diffStore(projectID: "proj-1")

        XCTAssertTrue(first === second)
    }

    @MainActor
    func testAppModel_prStoreIsCreatedOnceAndReused() {
        let appModel = AppModel()

        let first = appModel.prStore(projectID: "proj-1")
        let second = appModel.prStore(projectID: "proj-1")

        XCTAssertTrue(first === second)
    }

    // MARK: - Onboarding

    @MainActor
    func testAppModel_needsOnboardingBeforeStart() {
        let appModel = AppModel()
        XCTAssertTrue(appModel.needsOnboarding)
    }

    @MainActor
    func testAppModel_needsOnboardingWhenNoConfigFile() async {
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertTrue(appModel.needsOnboarding)
    }

    @MainActor
    func testAppModel_needsOnboardingWhenProjectsMapIsEmpty() async {
        let testConfig = NatProjectConfig(projects: [:])
        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertTrue(appModel.needsOnboarding)
        XCTAssertNotNil(appModel.config)
        XCTAssertNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_doesNotNeedOnboardingWithAProject() async {
        let testConfig = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")]
        )
        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertFalse(appModel.needsOnboarding)
    }

    // MARK: - Reload config

    @MainActor
    func testAppModel_reloadConfigPicksUpNewValues() async {
        let original = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")],
            pollSeconds: 30
        )
        let mockReader = MockConfigReader(response: .success(original))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        XCTAssertEqual(appModel.config?.pollSeconds, 30)

        let updated = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")],
            pollSeconds: 90
        )
        mockReader.setResponse(.success(updated))

        await appModel.reloadConfig()

        XCTAssertEqual(appModel.config?.pollSeconds, 90)
        // reloadConfig re-reads the same path start() loaded from.
        XCTAssertEqual(mockReader.lastPath, "/fake/config.json")
    }

    @MainActor
    func testAppModel_reloadConfigBeforeStartDoesNothing() async {
        let mockReader = MockConfigReader(response: .success(NatProjectConfig(projects: [:])))
        let appModel = AppModel(configReader: mockReader)

        // Never started, so there is no path to re-read from.
        await appModel.reloadConfig()

        XCTAssertNil(appModel.config)
        XCTAssertNil(mockReader.lastPath)
    }

    // MARK: - Workshop

    /// A model whose launches are the given closure's. `planningAgentAppears`
    /// is whether the activity poll behind it ever reports the session a
    /// launch starts — false is a launch nothing comes of, which is what the
    /// settle wait gives up on. That wait is a no-op here, so a test never
    /// spends thirty seconds finding out.
    @MainActor
    private func workshopModel(
        planningAgentAppears: Bool = false,
        settleWait: @escaping @MainActor @Sendable () async -> Void = { await Task.yield() },
        launcher: @escaping @Sendable (String, String?, String?, String?) async throws -> WorkshopLaunchResult
    ) async -> AppModel {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b")
            ],
            workshopAgent: AgentModel(model: "opus", effort: "high")
        )
        let appearing = planningAgentAppears ? PlanningAgentAppearsClient() : nil
        let client: NatClientProtocol = appearing ?? MockActivityClient(response: .agents([]))
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(testConfig)),
            workshopLauncher: { projectID, model, effort, request in
                let result = try await launcher(projectID, model, effort, request)
                appearing?.launched()
                return result
            },
            activityStoreFactory: { ActivityStore(client: client) },
            launchSettleWait: settleWait
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return appModel
    }

    /// A model whose activity poll always reports the given agents, for the
    /// rules about which of them is *this* project's planning agent.
    @MainActor
    private func planningAgentModel(_ agents: [AgentStatus]) async -> AppModel {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b")
            ]
        )
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(testConfig)),
            activityStoreFactory: { ActivityStore(client: MockActivityClient(response: .agents(agents))) }
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        // One turn of the poll, so `activityStore.agents` holds the reading.
        while appModel.activityStore?.agents.isEmpty ?? true {
            await Task.yield()
        }
        return appModel
    }

    // A planning agent belongs to one project, so the pane draws the active
    // project's and never another's — switching tabs switches the workshop
    // with everything else.
    @MainActor
    func testPlanningAgent_isTheActiveProjectsOwn() async {
        let appModel = await planningAgentModel([
            AgentStatus(
                sliceID: TmuxSession.planTag(projectID: "proj-a"),
                session: TmuxSession.planSessionName(projectID: "proj-a"),
                activity: .working
            ),
            AgentStatus(
                sliceID: TmuxSession.planTag(projectID: "proj-b"),
                session: TmuxSession.planSessionName(projectID: "proj-b"),
                activity: .waiting
            )
        ])

        await appModel.activateProject("proj-a")
        XCTAssertEqual(appModel.planningAgent?.session, TmuxSession.planSessionName(projectID: "proj-a"))
        XCTAssertEqual(appModel.planningAgentKey, TmuxSession.planTag(projectID: "proj-a"))

        await appModel.activateProject("proj-b")
        XCTAssertEqual(appModel.planningAgent?.session, TmuxSession.planSessionName(projectID: "proj-b"))
        XCTAssertEqual(appModel.planningAgentKey, TmuxSession.planTag(projectID: "proj-b"))
    }

    // Another project's planning agent is nobody else's.
    @MainActor
    func testPlanningAgent_isNilWithOnlyAnotherProjects() async {
        let appModel = await planningAgentModel([
            AgentStatus(
                sliceID: TmuxSession.planTag(projectID: "proj-b"),
                session: TmuxSession.planSessionName(projectID: "proj-b"),
                activity: .working
            )
        ])

        await appModel.activateProject("proj-a")

        XCTAssertNil(appModel.planningAgent)
        XCTAssertNil(appModel.planningAgentKey)
    }

    // A session a pre-upgrade nat left running carries the bare sentinel and
    // belongs to no project, so it is read rather than orphaned — by whichever
    // project is active. Its own outranks it where there is one.
    @MainActor
    func testPlanningAgent_readsALegacyBareSession() async {
        let appModel = await planningAgentModel([
            AgentStatus(sliceID: TmuxSession.planSentinel, session: TmuxSession.planSession, activity: .working)
        ])

        await appModel.activateProject("proj-a")
        XCTAssertEqual(appModel.planningAgent?.session, TmuxSession.planSession)
        XCTAssertEqual(appModel.planningAgentKey, TmuxSession.planSentinel)

        await appModel.activateProject("proj-b")
        XCTAssertEqual(appModel.planningAgent?.session, TmuxSession.planSession)
    }

    @MainActor
    func testPlanningAgent_prefersItsOwnOverALegacySession() async {
        let appModel = await planningAgentModel([
            AgentStatus(sliceID: TmuxSession.planSentinel, session: TmuxSession.planSession, activity: .working),
            AgentStatus(
                sliceID: TmuxSession.planTag(projectID: "proj-a"),
                session: TmuxSession.planSessionName(projectID: "proj-a"),
                activity: .working
            )
        ])

        await appModel.activateProject("proj-a")

        XCTAssertEqual(appModel.planningAgent?.session, TmuxSession.planSessionName(projectID: "proj-a"))
    }

    @MainActor
    func testOpenWorkshop_selectsWithoutLaunching() async {
        let recorder = WorkshopLaunchRecorder()
        let appModel = await workshopModel { projectID, model, effort, request in
            recorder.record(projectID: projectID, model: model, effort: effort, request: request)
            return WorkshopLaunchResult(session: "nat-plan", workdir: "/path/a", wishlist: false)
        }
        appModel.selectedSliceID = "slice-1"

        appModel.openWorkshop()

        XCTAssertTrue(recorder.calls.isEmpty)
        XCTAssertTrue(appModel.workshopSelected)
        // One selected row: the workshop takes the slice's place.
        XCTAssertNil(appModel.selectedSliceID)
    }

    @MainActor
    func testLaunchWorkshop_launchesWithTheConfigPairAndTheTrimmedRequest() async {
        let recorder = WorkshopLaunchRecorder()
        let appModel = await workshopModel(planningAgentAppears: true) { projectID, model, effort, request in
            recorder.record(projectID: projectID, model: model, effort: effort, request: request)
            return WorkshopLaunchResult(session: "nat-plan", workdir: "/path/a", wishlist: false)
        }

        await appModel.launchWorkshop(request: "  Add dark mode.  \n")

        XCTAssertEqual(recorder.calls.count, 1)
        XCTAssertEqual(recorder.calls[0].projectID, "proj-a")
        XCTAssertEqual(recorder.calls[0].model, "opus")
        XCTAssertEqual(recorder.calls[0].effort, "high")
        XCTAssertEqual(recorder.calls[0].request, "Add dark mode.")
        XCTAssertTrue(appModel.workshopSelected)
        // Settled: the poll has reported the session, so the pane has the
        // terminal to draw and the launching state is over.
        XCTAssertFalse(appModel.workshopLaunching)
        XCTAssertNil(appModel.workshopLaunchError)
        XCTAssertEqual(appModel.planningAgent?.session, "nat-plan-a")
    }

    @MainActor
    func testLaunchWorkshop_staysLaunchingUntilThePollReportsTheSession() async {
        let probe = WorkshopLaunchProbe()
        let appModel = await workshopModel(
            planningAgentAppears: true,
            settleWait: {
                probe.observe()
                await Task.yield()
            }
        ) { _, _, _, _ in
            WorkshopLaunchResult(session: "nat-plan", workdir: "/path/a", wishlist: false)
        }
        probe.model = appModel

        await appModel.launchWorkshop(request: "")

        // The command returning is not the pane's cue: the launching state is
        // held over every turn of the wait, so the composer never comes back
        // between `workshop-launch` and the terminal.
        XCTAssertFalse(probe.launching.isEmpty)
        XCTAssertTrue(probe.launching.allSatisfy { $0 })
        XCTAssertNotNil(appModel.planningAgent)
        XCTAssertFalse(appModel.workshopLaunching)
        XCTAssertNil(appModel.workshopLaunchError)
    }

    @MainActor
    func testLaunchWorkshop_givesUpOnASessionThatNeverAppears() async {
        // The activity poll reports nothing, ever — a session that exited on
        // the spot, or a tmux the poll cannot read.
        let appModel = await workshopModel { _, _, _, _ in
            WorkshopLaunchResult(session: "nat-plan", workdir: "/path/a", wishlist: false)
        }

        await appModel.launchWorkshop(request: "")

        XCTAssertFalse(appModel.workshopLaunching)
        XCTAssertEqual(
            appModel.workshopLaunchError,
            "the workshop session was launched but has not appeared — check `nat status`"
        )
    }

    @MainActor
    func testLaunchWorkshop_aFailedLaunchDoesNotWaitOnAnAgent() async {
        let probe = WorkshopLaunchProbe()
        let appModel = await workshopModel(settleWait: { probe.observe() }) { _, _, _, _ in
            throw NatError.commandFailed("boom")
        }
        probe.model = appModel

        await appModel.launchWorkshop(request: "")

        // Straight back to the composer with the failure on it: there is no
        // session for the poll to find.
        XCTAssertTrue(probe.launching.isEmpty)
        XCTAssertFalse(appModel.workshopLaunching)
        XCTAssertEqual(appModel.workshopLaunchError, "boom")
    }

    @MainActor
    func testLaunchWorkshop_commandFailureKeepsItsOwnMessage() async {
        let appModel = await workshopModel { _, _, _, _ in
            throw NatError.commandFailed("a planning agent is already live: nat-plan")
        }

        await appModel.launchWorkshop(request: "")

        XCTAssertEqual(appModel.workshopLaunchError, "a planning agent is already live: nat-plan")
        XCTAssertFalse(appModel.workshopLaunching)
        XCTAssertTrue(appModel.workshopSelected)
    }

    @MainActor
    func testLaunchWorkshop_otherNatErrorFallsBackToItsDescription() async {
        let appModel = await workshopModel { _, _, _, _ in
            throw NatError.missingOutput
        }

        await appModel.launchWorkshop(request: "")

        XCTAssertEqual(appModel.workshopLaunchError, NatError.missingOutput.localizedDescription)
    }

    @MainActor
    func testLaunchWorkshop_arbitraryErrorFallsBackToItsDescription() async {
        let appModel = await workshopModel { _, _, _, _ in
            throw NSError(domain: "test", code: 7, userInfo: [NSLocalizedDescriptionKey: "boom"])
        }

        await appModel.launchWorkshop(request: "")

        XCTAssertEqual(appModel.workshopLaunchError, "boom")
    }

    @MainActor
    func testWorkshop_withoutAnActiveProjectDoesNothing() async {
        let recorder = WorkshopLaunchRecorder()
        let appModel = AppModel(
            configReader: MockConfigReader(response: .failure),
            workshopLauncher: { projectID, model, effort, request in
                recorder.record(projectID: projectID, model: model, effort: effort, request: request)
                return WorkshopLaunchResult(session: "nat-plan", workdir: "/", wishlist: false)
            }
        )

        appModel.openWorkshop()
        await appModel.launchWorkshop(request: "anything")

        XCTAssertTrue(recorder.calls.isEmpty)
        XCTAssertFalse(appModel.workshopSelected)
    }

    @MainActor
    func testSelectingASliceDeselectsTheWorkshopAndDismissesItsError() async {
        let appModel = await workshopModel { _, _, _, _ in
            throw NatError.commandFailed("boom")
        }
        await appModel.launchWorkshop(request: "")
        XCTAssertTrue(appModel.workshopSelected)
        XCTAssertNotNil(appModel.workshopLaunchError)

        appModel.selectedSliceID = "slice-1"

        XCTAssertFalse(appModel.workshopSelected)
        XCTAssertNil(appModel.workshopLaunchError)
        XCTAssertEqual(appModel.selectedSliceID, "slice-1")
    }

    @MainActor
    func testWorkshopSelected_isPerProject() async {
        let appModel = await workshopModel { _, _, _, _ in
            WorkshopLaunchResult(session: "nat-plan", workdir: "/path/a", wishlist: false)
        }

        appModel.workshopSelected = true
        XCTAssertTrue(appModel.workshopSelected)

        await appModel.activateProject("proj-b")
        XCTAssertFalse(appModel.workshopSelected)

        await appModel.activateProject("proj-a")
        XCTAssertTrue(appModel.workshopSelected)
    }

    @MainActor
    func testWorkshopSelected_setterCanDeselect() async {
        let appModel = await workshopModel { _, _, _, _ in
            WorkshopLaunchResult(session: "nat-plan", workdir: "/path/a", wishlist: false)
        }

        appModel.workshopSelected = true
        appModel.workshopSelected = false

        XCTAssertFalse(appModel.workshopSelected)
    }

    @MainActor
    func testWorkshopSelected_withoutAnActiveProjectIsFalseAndUnsettable() {
        let appModel = AppModel()

        XCTAssertFalse(appModel.workshopSelected)
        appModel.workshopSelected = true
        XCTAssertFalse(appModel.workshopSelected)
    }

    @MainActor
    func testPlanningAgent_isNilWithoutAnActivityReading() {
        let appModel = AppModel()

        XCTAssertNil(appModel.planningAgent)
    }

    @MainActor
    func testAppModel_reloadConfigKeepsThePreviousConfigOnReadFailure() async {
        let original = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")]
        )
        let mockReader = MockConfigReader(response: .success(original))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        mockReader.setResponse(.failure)

        await appModel.reloadConfig()

        XCTAssertEqual(appModel.config, original)
    }

    // MARK: - Adding a project

    /// A paths provider that never spawns `nat` — `start()` falls back to
    /// nat's own default locations, which the mock config reader ignores.
    private static let noPaths: @Sendable () async throws -> NatPaths = {
        throw NatError.missingOutput
    }

    @MainActor
    func testAppModel_addProjectGivesTheNewEntryATabAndActivatesIt() async {
        let config = NatProjectConfig(projects: [
            "proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1"),
        ])
        let mockReader = MockConfigReader(response: .success(config))
        let appModel = AppModel(configReader: mockReader, pathsProvider: Self.noPaths)
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // The config the reload will see: the opened project is in it now.
        mockReader.setResponse(.success(NatProjectConfig(projects: [
            "proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1"),
            "proj-2": ProjectConfig(name: "Opened Project", slicesDSID: "ds-2", workingDir: ""),
        ])))

        await appModel.addProject(id: "proj-2", name: "whatever the command said")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-1", "proj-2"])
        // The config's own name, which is what every other tab is labelled with.
        XCTAssertEqual(appModel.projectTabs.last?.name, "Opened Project")
        XCTAssertEqual(appModel.activeProjectID, "proj-2")
        XCTAssertFalse(appModel.needsOnboarding)
    }

    @MainActor
    func testAppModel_addProjectDoesNotDuplicateATabItAlreadyHas() async {
        let config = NatProjectConfig(projects: [
            "proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1"),
            "proj-2": ProjectConfig(name: "Project 2", slicesDSID: "ds-2", workingDir: ""),
        ])
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            pathsProvider: Self.noPaths
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        await appModel.addProject(id: "proj-2", name: "Project 2")

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-1", "proj-2"])
        XCTAssertEqual(appModel.activeProjectID, "proj-2")
    }

    @MainActor
    func testAppModel_addProjectIsTheStartThatWasMissedOnAnOnboardingMachine() async {
        // No config to read at start(): the welcome pane's own state.
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader, pathsProvider: Self.noPaths)
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        XCTAssertTrue(appModel.needsOnboarding)

        // project-create wrote one, so there is a config file now.
        mockReader.setResponse(.success(NatProjectConfig(projects: [
            "proj-9": ProjectConfig(name: "Fresh Project", slicesDSID: "ds-9", workingDir: "/src/fresh"),
        ])))

        await appModel.addProject(id: "proj-9", name: "Fresh Project")

        XCTAssertFalse(appModel.needsOnboarding)
        XCTAssertEqual(appModel.projectTabs.map(\.id), ["proj-9"])
        XCTAssertEqual(appModel.activeProjectID, "proj-9")
        XCTAssertNotNil(appModel.activityStore)
        XCTAssertNotNil(appModel.reviewStatsStore)
    }

    @MainActor
    func testAppModel_addProjectLeavesTheWelcomePaneUpWhenConfigStillCannotBeRead() async {
        let appModel = AppModel(
            configReader: MockConfigReader(response: .failure),
            pathsProvider: Self.noPaths
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        await appModel.addProject(id: "proj-9", name: "Fresh Project")

        XCTAssertTrue(appModel.needsOnboarding)
        XCTAssertTrue(appModel.projectTabs.isEmpty)
        XCTAssertNil(appModel.activeProjectID)
    }

    @MainActor
    func testAppModel_activePlanIsEmptyIsFalseUntilAPlanHasLanded() {
        // No store, and so no reading: an unloaded plan is not an empty one,
        // which is what keeps the empty-state note off a board that is still
        // loading or has just failed to load.
        XCTAssertFalse(AppModel().activePlanIsEmpty)
    }

    @MainActor
    func testAppModel_activeProjectNeedsWorkingDir() async {
        let config = NatProjectConfig(projects: [
            "proj-1": ProjectConfig(name: "Configured", slicesDSID: "ds-1", workingDir: "/path/1"),
            "proj-2": ProjectConfig(name: "Just Opened", slicesDSID: "ds-2", workingDir: ""),
            "proj-3": ProjectConfig(name: "Whitespace", slicesDSID: "ds-3", workingDir: "  "),
        ])
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            pathsProvider: Self.noPaths
        )

        // No active project at all: no entry to be missing a directory.
        XCTAssertFalse(appModel.activeProjectNeedsWorkingDir)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        XCTAssertEqual(appModel.activeProjectID, "proj-1")
        XCTAssertFalse(appModel.activeProjectNeedsWorkingDir)

        await appModel.addProject(id: "proj-2", name: "Just Opened")
        XCTAssertTrue(appModel.activeProjectNeedsWorkingDir)

        await appModel.addProject(id: "proj-3", name: "Whitespace")
        XCTAssertTrue(appModel.activeProjectNeedsWorkingDir)
    }

    // MARK: - The disk cache

    @MainActor
    func testAppModel_startDrawsTheBoardFromTheCachedPlan() async {
        let config = NatProjectConfig(projects: [
            "proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")
        ])
        let cached = ProjectInfo(
            project: Project(id: "proj-1", name: "Cached", conventions: ""),
            milestones: [],
            slices: []
        )
        let cache = FakePlanCache(stored: ["proj-1": cached])
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            planCache: cache,
            pathsProvider: Self.noPaths
        )

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // The store the activation made reads through the injected cache,
        // so the plan is on the board whatever the fresh read did — here it
        // fails, since no `nat` in a test knows "proj-1".
        XCTAssertEqual(cache.reads, ["proj-1"])
        XCTAssertEqual(appModel.projectStore?.state.projectInfo, cached)
        appModel.cleanup()
    }
}
