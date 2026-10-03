import XCTest
@testable import NatKit
@testable import NatFixtures

/// A project's workshop over a client whose reads a test can hold open, so
/// readings and an Accept can be made to finish in whichever order would
/// once have put the wrong thing on screen.
private final class GatedProposalClient: NatClientProtocol, @unchecked Sendable {
    private let lock = NSLock()
    private var _proposal: PlanProposal?
    private var _plan: ProjectInfo
    private var _proposalGates: [Gate] = []
    private var _infoGate: Gate?
    private var _refreshFlags: [Bool] = []
    private var _accepts = 0

    init(plan: ProjectInfo) { _plan = plan }

    var proposal: PlanProposal? {
        get { lock.withLock { _proposal } }
        set { lock.withLock { _proposal = newValue } }
    }
    var plan: ProjectInfo {
        get { lock.withLock { _plan } }
        set { lock.withLock { _plan = newValue } }
    }
    var refreshFlags: [Bool] { lock.withLock { _refreshFlags } }
    var accepts: Int { lock.withLock { _accepts } }

    /// The next proposal reading answers as the file stood when it was asked,
    /// but only once `gate` opens.
    func holdNextProposalReading(_ gate: Gate) { lock.withLock { _proposalGates.append(gate) } }
    /// The next plan read is held until `gate` opens.
    func holdNextInfo(_ gate: Gate) { lock.withLock { _infoGate = gate } }

    func planProposal(projectID: String) async throws -> PlanProposal? {
        let (answer, gate) = lock.withLock { (_proposal, _proposalGates.isEmpty ? nil : _proposalGates.removeFirst()) }
        if let gate {
            await gate.markAsked()
            await gate.waitUntilOpen()
        }
        return answer
    }

    func planAccept(projectID: String) async throws -> PlanAccepted {
        lock.withLock {
            _accepts += 1
            // As nat does: the plan goes in, then the proposal file goes.
            let added = (_proposal?.milestones ?? []).enumerated().map {
                Milestone(id: $1.name, name: $1.name, order: Double(100 + $0), status: "Active")
            }
            _plan = ProjectInfo(project: _plan.project, milestones: _plan.milestones + added, slices: _plan.slices)
            _proposal = nil
        }
        return PlanAccepted(project: ProjectEntry(id: projectID, name: "P"), milestones: 1, slices: 1)
    }

    func info(projectID: String, refresh: Bool) async throws -> ProjectInfo {
        let gate = lock.withLock { () -> Gate? in
            _refreshFlags.append(refresh)
            defer { _infoGate = nil }
            return _infoGate
        }
        if let gate {
            await gate.markAsked()
            await gate.waitUntilOpen()
        }
        return plan
    }

    func info(projectID: String) async throws -> ProjectInfo { try await info(projectID: projectID, refresh: false) }
    func status() async throws -> [AgentStatus] { [] }
    func usage() async throws -> UsageReading { .empty }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw NatError.missingOutput }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw NatError.missingOutput }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw NatError.missingOutput }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult { throw NatError.missingOutput }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult { throw NatError.missingOutput }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws {}
    func agentKill(projectID: String, sliceRef: String) async throws {}
    func agentKillWorkshop(projectID: String) async throws {}
    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult { throw NatError.missingOutput }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { "" }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail { throw NatError.missingOutput }
    func prStatus(projectID: String) async throws -> PRStatusDoc { throw NatError.missingOutput }
    func prMerge(projectID: String, sliceRef: String) async throws {}
    func prComment(projectID: String, sliceRef: String, body: String) async throws {}
    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult { throw NatError.missingOutput }
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult { throw NatError.missingOutput }
    func configShow() async throws -> ConfigDoc { throw NatError.missingOutput }
    func configSet(key: String, value: String) async throws {}
}

/// Readings and Accept on a project's workshop, finishing in the orders that
/// once raced: what is drawn follows the state machine, never the timing.
@MainActor
final class ProposalRaceTests: XCTestCase {
    private let projectID = "proj-a"
    private let first = PlanProposal(name: "", milestones: [.init(name: "M7", slices: ["A"])])
    private let second = PlanProposal(name: "", milestones: [.init(name: "M8", slices: ["B", "C"])])

    private func model() async -> (AppModel, GatedProposalClient) {
        let client = GatedProposalClient(plan: ProjectInfo(
            project: Project(id: projectID, name: "A", conventions: ""), milestones: [], slices: []))
        let config = NatProjectConfig(projects: [projectID: ProjectConfig(name: "A", slicesDSID: "ds", workingDir: "/a")])
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            planCache: NullPlanCache(),
            pollIntervalSeconds: 3600,
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) })
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        appModel.openWorkshop()
        return (appModel, client)
    }

    func testAnOlderReadingFinishingLastCannotPutAnOlderProposalBack() async {
        let (appModel, client) = await model()
        let held = Gate()
        client.proposal = first
        client.holdNextProposalReading(held)
        let older = Task { await appModel.refreshProposals() }
        await held.waitUntilAsked()

        client.proposal = second
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.activeProposal, second)

        await held.open()
        await older.value
        XCTAssertEqual(appModel.activeProposal, second, "the reading that began first is stale")
    }

    func testAReadingBegunBeforeAnAcceptCannotBringTheAcceptedProposalBack() async {
        let (appModel, client) = await model()
        client.proposal = first
        await appModel.refreshProposals()
        let held = Gate()
        client.holdNextProposalReading(held)
        let before = Task { await appModel.refreshProposals() }
        await held.waitUntilAsked()

        await appModel.acceptProposal()
        XCTAssertNil(appModel.activeProposal)

        await held.open()
        await before.value
        XCTAssertNil(appModel.activeProposal, "it read the file before nat dropped it")
        XCTAssertEqual(client.accepts, 1)
    }

    func testAcceptReadsTheReplicaAndEndsOnlyOnceThePlanIsIn() async {
        let (appModel, client) = await model()
        client.proposal = first
        await appModel.refreshProposals()
        let flagsBefore = client.refreshFlags.count
        let planRead = Gate()
        client.holdNextInfo(planRead)

        let accept = Task { await appModel.acceptProposal() }
        await planRead.waitUntilAsked()
        XCTAssertTrue(appModel.proposalAccepting, "still accepting while the plan is read")
        XCTAssertEqual(appModel.activeProposal, first, "the Plan section stays until the tree has the plan")

        await planRead.open()
        await accept.value
        XCTAssertFalse(appModel.proposalAccepting)
        XCTAssertNil(appModel.activeProposal)
        XCTAssertEqual(appModel.plan(projectID: projectID)?.milestones.map(\.name), ["M7"])
        XCTAssertEqual(Array(client.refreshFlags.dropFirst(flagsBefore)), [false], "the replica, never a pull")
    }

    func testAProposalAcceptedElsewhereGoesAtTheNextReading() async {
        let (appModel, client) = await model()
        client.proposal = first
        await appModel.refreshProposals()

        client.proposal = nil
        await appModel.refreshProposals()

        XCTAssertNil(appModel.activeProposal)
    }

    func testANudgeReadsTheReplica() async {
        let (appModel, client) = await model()
        let before = client.refreshFlags.count

        await appModel.refresh(.replica)
        await appModel.refresh()

        XCTAssertEqual(Array(client.refreshFlags.dropFirst(before)), [false, true])
    }
}
