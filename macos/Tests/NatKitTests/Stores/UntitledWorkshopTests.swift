import XCTest
@testable import NatKit

/// A double that reports the planning agents it has been told about and
/// records every workspace launch and kill, so the Untitled tab's session can
/// be launched, attached and ended without a tmux.
final class WorkspaceWorkshopClient: MockActivityClient, @unchecked Sendable {
    struct Launch: Equatable {
        let workspaceID: String
        let model: String?
        let effort: String?
        let request: String
    }

    private let lock = NSLock()
    private var live: [AgentStatus] = []
    private var _launches: [Launch] = []
    private var _kills: [String] = []
    /// What a launch throws, or nil to succeed.
    var launchFailure: Error?
    /// The refusal a kill throws as `NatError.commandFailed`, or nil to succeed.
    var killRefusal: String?
    /// Whether a launch makes its agent appear in the next `status()` reading.
    var launchAppears = true

    var launches: [Launch] { lock.withLock { _launches } }
    var kills: [String] { lock.withLock { _kills } }

    init() { super.init(response: .agents([])) }

    /// Puts an agent in the readings without a launch having made it.
    func addLive(tag: String) {
        lock.withLock { live.append(AgentStatus(sliceID: tag, session: "nat-" + tag, activity: .working)) }
    }

    override func status() async throws -> [AgentStatus] {
        lock.withLock { live }
    }

    override func workspaceLaunch(
        workspaceID: String, model: String?, effort: String?, request: String
    ) async throws -> WorkshopLaunchResult {
        if let launchFailure { throw launchFailure }
        lock.withLock {
            _launches.append(Launch(workspaceID: workspaceID, model: model, effort: effort, request: request))
            if launchAppears {
                live.append(AgentStatus(
                    sliceID: TmuxSession.planTag(projectID: workspaceID),
                    session: TmuxSession.planSessionName(projectID: workspaceID),
                    activity: .working))
            }
        }
        return WorkshopLaunchResult(session: TmuxSession.planSessionName(projectID: workspaceID), workdir: "/scratch")
    }

    override func agentKillWorkspace(workspaceID: String) async throws {
        lock.withLock { _kills.append(workspaceID) }
        if let killRefusal { throw NatError.commandFailed(killRefusal) }
        lock.withLock { live.removeAll { $0.sliceID == TmuxSession.planTag(projectID: workspaceID) } }
    }
}

/// The starter card's "Workshop the plan": the Untitled tab's planning agent,
/// keyed by the tab's workspace id and never by a project.
@MainActor
final class UntitledWorkshopTests: XCTestCase {
    private func model(
        client: WorkspaceWorkshopClient
    ) async -> (AppModel, String) {
        let config = NatProjectConfig(
            projects: ["proj-a": ProjectConfig(name: "A", slicesDSID: "ds-a", workingDir: "/a")],
            workshopAgent: AgentModel(model: "opus", effort: "high")
        )
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            workshopLauncher: { _, _, _, _ in
                XCTFail("an Untitled tab must never launch through a project")
                return WorkshopLaunchResult(session: "", workdir: "")
            },
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            launchSettleWait: { await Task.yield() }
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        let tab = appModel.openUntitledTab()
        return (appModel, tab)
    }

    /// Reads the activity poll once so the model sees what the double reports.
    private func poll(_ appModel: AppModel) async {
        appModel.activityStore?.kick()
        for _ in 0..<200 {
            await Task.yield()
            try? await Task.sleep(nanoseconds: 2_000_000)
            if appModel.planningAgent != nil { return }
        }
    }

    func testEachUntitledTabMintsItsOwnWorkspaceAndAProjectHasNone() async {
        let (appModel, first) = await model(client: WorkspaceWorkshopClient())
        let second = appModel.openUntitledTab()

        let a = appModel.workspaceID(forTab: first)
        let b = appModel.workspaceID(forTab: second)
        XCTAssertNotNil(a)
        XCTAssertNotNil(b)
        XCTAssertNotEqual(a, b)
        XCTAssertNil(appModel.workspaceID(forTab: "proj-a"))
    }

    func testWorkshoppingLaunchesTheWorkspaceWithTheTrimmedRequestAndShowsTheSession() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, tab) = await model(client: client)
        XCTAssertFalse(appModel.untitledWorkshopVisible)

        await appModel.launchWorkshop(request: "  A habit tracker.\n")

        XCTAssertEqual(client.launches, [.init(
            workspaceID: appModel.workspaceID(forTab: tab)!, model: "opus", effort: "high",
            request: "A habit tracker.")])
        XCTAssertNil(appModel.workshopLaunchError)
        XCTAssertFalse(appModel.workshopLaunching)
        XCTAssertNotNil(appModel.planningAgent)
        XCTAssertTrue(appModel.untitledWorkshopVisible)
        XCTAssertTrue(appModel.tabHasLiveWorkshop(tab))
    }

    func testAnEmptyDescriptionLaunchesNothing() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, _) = await model(client: client)

        await appModel.launchWorkshop(request: " \n ")

        XCTAssertTrue(client.launches.isEmpty)
        XCTAssertFalse(appModel.untitledWorkshopVisible)
    }

    func testASecondPressAttachesInsteadOfLaunchingAgain() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, _) = await model(client: client)
        await appModel.launchWorkshop(request: "A plan")

        await appModel.launchWorkshop(request: "A plan")

        XCTAssertEqual(client.launches.count, 1)
        XCTAssertNotNil(appModel.planningAgent)
    }

    func testAFailedLaunchLeavesTheStarterCardWithTheError() async {
        let client = WorkspaceWorkshopClient()
        client.launchFailure = NatError.commandFailed("a planning agent is already live: nat-plan-x")
        let (appModel, _) = await model(client: client)

        await appModel.launchWorkshop(request: "A plan")

        XCTAssertEqual(appModel.workshopLaunchError, "a planning agent is already live: nat-plan-x")
        XCTAssertFalse(appModel.untitledWorkshopVisible)
    }

    func testASessionThatNeverAppearsIsGivenUpOn() async {
        let client = WorkspaceWorkshopClient()
        client.launchAppears = false
        let (appModel, _) = await model(client: client)

        await appModel.launchWorkshop(request: "A plan")

        XCTAssertNotNil(appModel.workshopLaunchError)
        XCTAssertFalse(appModel.untitledWorkshopVisible)
    }

    func testTheTabTakesOnlyItsOwnPlanningAgent() async {
        let client = WorkspaceWorkshopClient()
        client.addLive(tag: TmuxSession.planSentinel)
        client.addLive(tag: TmuxSession.planTag(projectID: "proj-a"))
        client.addLive(tag: TmuxSession.planTag(projectID: "some-other-workspace"))
        let (appModel, tab) = await model(client: client)
        await poll(appModel)
        while appModel.activityStore?.agents.count != 3 { await Task.yield() }

        XCTAssertNil(appModel.planningAgent, "neither the legacy session nor another's is this tab's")
        XCTAssertFalse(appModel.tabHasLiveWorkshop(tab))
        XCTAssertFalse(appModel.untitledWorkshopVisible)
    }

    func testAnAgentAlreadyLiveOnTheWorkspaceIsAttachedWithoutALaunch() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, tab) = await model(client: client)
        client.addLive(tag: TmuxSession.planTag(projectID: appModel.workspaceID(forTab: tab)!))
        await poll(appModel)

        XCTAssertTrue(appModel.untitledWorkshopVisible)
        await appModel.launchWorkshop(request: "A plan")
        XCTAssertTrue(client.launches.isEmpty)
    }

    func testClosingATabWithALiveSessionKillsItFirstAndForgetsTheTab() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, tab) = await model(client: client)
        await appModel.launchWorkshop(request: "A plan")
        let workspace = appModel.workspaceID(forTab: tab)!

        let refusal = await appModel.closeProject(tab)

        XCTAssertNil(refusal)
        XCTAssertEqual(client.kills, [workspace])
        XCTAssertFalse(appModel.projectTabs.contains { $0.id == tab })
        XCTAssertNil(appModel.workspaceID(forTab: tab))
    }

    func testASessionThatWillNotDieKeepsTheTabOpen() async {
        let client = WorkspaceWorkshopClient()
        client.killRefusal = "tmux failed"
        let (appModel, tab) = await model(client: client)
        await appModel.launchWorkshop(request: "A plan")

        let refusal = await appModel.closeProject(tab)

        XCTAssertEqual(refusal, "tmux failed")
        XCTAssertTrue(appModel.projectTabs.contains { $0.id == tab })
        XCTAssertNotNil(appModel.workspaceID(forTab: tab))
    }

    func testClosingATabWithNoSessionKillsNothing() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, tab) = await model(client: client)
        XCTAssertFalse(appModel.tabHasLiveWorkshop(tab))

        await appModel.closeProject(tab)

        XCTAssertTrue(client.kills.isEmpty)
        XCTAssertFalse(appModel.projectTabs.contains { $0.id == tab })
    }

    func testTheRailsCloseEndsTheWorkspaceSessionAndReturnsToTheStarterCard() async {
        let client = WorkspaceWorkshopClient()
        let (appModel, tab) = await model(client: client)
        await appModel.launchWorkshop(request: "A plan")

        let refusal = await appModel.closeWorkshopTab()
        XCTAssertNil(refusal)
        XCTAssertEqual(client.kills, [appModel.workspaceID(forTab: tab)!])
        await poll(appModel)
        while appModel.planningAgent != nil { await Task.yield() }
        XCTAssertFalse(appModel.untitledWorkshopVisible)
    }

    func testAProjectTabIsNeverUntitledWorkshopVisible() async {
        let (appModel, _) = await model(client: WorkspaceWorkshopClient())
        await appModel.activateProject("proj-a")

        XCTAssertFalse(appModel.untitledWorkshopVisible)
        XCTAssertFalse(appModel.tabHasLiveWorkshop("proj-a"))
    }
}
