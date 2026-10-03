import XCTest
@testable import NatKit
@testable import NatFixtures

/// A runner answering one canned stdout and recording what it was asked —
/// the project-keyed proposal commands' and `info --refresh`'s argument shapes.
private final class RecordingRunner: CommandRunning, @unchecked Sendable {
    var stdout = ""
    private(set) var lastArguments: [String] = []
    private(set) var lastStandardInput: Data?

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        lastStandardInput = standardInput
        return (Data(stdout.utf8), Data(), 0)
    }
}

/// The workshop row pinned to Active while the workshop is open and not
/// launched, and a project workshop's proposal read and accepted by project.
@MainActor
final class WorkshopPinAndProposalTests: XCTestCase {
    private func workshopRows(_ model: AppModel) -> [SidebarActiveRow] {
        model.sidebarModel.active.filter { $0.kind == .workshop }
    }

    // MARK: - The pinned row

    func testOpeningTheWorkshopPinsItsRowAndClickingAwayKeepsItAndTheDraft() async {
        let model = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []))
        model.openWorkshop()
        model.workshopDraft = "Tidy the review flow."

        model.selectedSliceID = Fixtures.mergeBoxSliceID

        XCTAssertFalse(model.workshopSelected)
        XCTAssertTrue(model.isWorkshopPinned(Fixtures.projectID))
        let rows = workshopRows(model)
        XCTAssertEqual(rows.map(\.projectID), [Fixtures.projectID])
        XCTAssertEqual(rows.first?.state, .todo)
        XCTAssertEqual(rows.first?.live, false)

        await model.selectWorkshop(inProject: Fixtures.projectID)
        XCTAssertEqual(model.workshopDraft, "Tidy the review flow.", "the draft survives the click away")
    }

    func testDismissingThePinnedRowDropsItAndItsDraft() async {
        let model = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []))
        model.openWorkshop()
        model.workshopDraft = "Tidy the review flow."

        model.dismissWorkshop(inProject: Fixtures.projectID)

        XCTAssertFalse(model.isWorkshopPinned(Fixtures.projectID))
        XCTAssertFalse(model.workshopSelected)
        XCTAssertEqual(model.workshopDraft, "")
        XCTAssertTrue(workshopRows(model).isEmpty)
    }

    func testTheRowsCrossOnTheWorkshopUnpinsItToo() async {
        let model = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []))
        model.openWorkshop()

        let refusal = await model.closeWorkshopTab()

        XCTAssertNil(refusal)
        XCTAssertFalse(model.isWorkshopPinned(Fixtures.projectID))
        XCTAssertNil(model.workshopRequest)
    }

    func testALaunchShowsItsRequestAndHandsTheRowToTheLiveAgent() async {
        // An Untitled tab's launch is the one the fixture client starts a
        // planning agent for, so the hand-over is there to watch.
        let model = await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true)
        let tab = model.activeProjectID ?? ""
        model.workshopDraft = "  A habit tracker.  "

        await model.launchWorkshop(request: model.workshopDraft)

        XCTAssertEqual(model.workshopRequest, "A habit tracker.")
        XCTAssertEqual(model.workshopDraft, "")
        XCTAssertFalse(model.isWorkshopPinned(tab), "the live agent's row takes over")
        XCTAssertEqual(workshopRows(model).map(\.live), [true])
    }

    func testAnAttachedPlanFileIsNamedInTheRequest() async throws {
        let model = await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true)
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("pin-test-plan.md")
        try "# Plan\n".write(to: url, atomically: true, encoding: .utf8)
        model.attachPlanFile(url)

        await model.launchWorkshop(request: "Build it.")

        XCTAssertEqual(model.workshopRequest, "Build it.\n\nAttached: pin-test-plan.md")
    }

    func testAFailedLaunchTakesItsRequestBackOff() async {
        let model = Fixtures.appModel(client: FixtureNatClient(behaviour: .refusing("no tmux")))
        await Fixtures.start(model)
        model.openWorkshop()

        await model.launchWorkshop(request: "Split the importer.")

        XCTAssertNil(model.workshopRequest)
        XCTAssertTrue(model.isWorkshopPinned(Fixtures.projectID))
        XCTAssertNotNil(model.workshopLaunchError)
    }

    func testClosingAProjectUnpinsItsWorkshop() async {
        let model = await Fixtures.startedAppModel(
            client: FixtureNatClient(agents: []), config: Fixtures.twoProjectConfig)
        await model.selectWorkshop(inProject: Fixtures.secondProjectID)
        XCTAssertTrue(model.isWorkshopPinned(Fixtures.secondProjectID))

        await model.closeProject(Fixtures.secondProjectID)

        XCTAssertFalse(model.isWorkshopPinned(Fixtures.secondProjectID))
    }

    func testAPinnedRowIsDrawnLaunchingWhileItsLaunchIsInFlight() {
        let input = SidebarProjectInput(id: "p", name: "Plan", plan: nil)
        let pinned = buildSidebarModel(projects: [input], liveAgents: [:], pinnedWorkshops: ["p"])
        XCTAssertEqual(pinned.active.map(\.state), [.todo])
        let launching = buildSidebarModel(projects: [input], liveAgents: [:], launchingWorkshop: "p")
        XCTAssertEqual(launching.active.map(\.state), [.working])
        XCTAssertEqual(launching.active.map(\.live), [false])
        let live = buildSidebarModel(
            projects: [input], liveAgents: [:], planningAgents: ["p": .waiting], pinnedWorkshops: ["p"])
        XCTAssertEqual(live.active.map(\.state), [.waiting], "a live agent wins over the pin")
        XCTAssertEqual(live.active.count, 1)
    }

    // MARK: - A project's proposal

    func testAProjectWorkshopsProposalIsReadAndAcceptedByProject() async {
        let client = FixtureNatClient(agents: [])
        client.setProposal(Fixtures.proposal, forProject: Fixtures.projectID)
        let model = await Fixtures.startedAppModel(client: client)
        model.openWorkshop()

        await model.refreshProposals()
        XCTAssertEqual(model.activeProposal, Fixtures.proposal)
        XCTAssertEqual(model.proposal(forTab: Fixtures.projectID), Fixtures.proposal)

        await model.acceptProposal()

        XCTAssertTrue(client.writes.contains("plan-accept --project \(Fixtures.projectID)"))
        XCTAssertNil(model.activeProposal)
        XCTAssertNil(model.proposalError)
        XCTAssertTrue(model.workshopSelected, "the project's workshop stays where it was")
    }

    func testAProjectWithNoWorkshopGoingIsNotAskedForAProposal() async {
        let client = FixtureNatClient(agents: [])
        client.setProposal(Fixtures.proposal, forProject: Fixtures.projectID)
        let model = await Fixtures.startedAppModel(client: client)

        await model.refreshProposals()

        XCTAssertNil(model.proposal(forTab: Fixtures.projectID))
    }

    func testARefusedProjectAcceptKeepsTheProposalAndSaysWhy() async {
        let client = FixtureNatClient(agents: [])
        client.setProposal(Fixtures.proposal, forProject: Fixtures.projectID)
        client.refuseAccepts("plan-accept: milestone \"M9\" is not in the plan")
        let model = await Fixtures.startedAppModel(client: client)
        model.openWorkshop()
        await model.refreshProposals()

        await model.acceptProposal()

        XCTAssertEqual(model.activeProposal, Fixtures.proposal)
        XCTAssertEqual(model.proposalError, "plan-accept: milestone \"M9\" is not in the plan")
        XCTAssertFalse(model.proposalAccepting)
    }

    func testAProjectProposalWithNoNameDecodes() throws {
        let proposal = try JSONDecoder().decode(
            PlanProposal.self, from: Data(#"{"project": "p", "plan": {"milestones": [{"name": "M1"}]}}"#.utf8))
        XCTAssertEqual(proposal.name, "")
        XCTAssertEqual(proposal.milestoneCount, 1)
    }

    func testTheProjectCaptionNamesTheProject() {
        XCTAssertEqual(
            ProposalText.projectAcceptCaption(project: "gnat"),
            "Accepting files these milestones and tasks into “gnat”.")
    }

    // MARK: - The commands

    func testTheProjectKeyedProposalCommands() async throws {
        let runner = RecordingRunner()
        runner.stdout = #"{"proposal": null}"#
        let client = NatClient(commandRunner: runner)

        let none = try await client.planProposal(projectID: "p-1")
        XCTAssertNil(none)
        XCTAssertEqual(runner.lastArguments, ["plan-proposal", "--project", "p-1", "--json"])

        runner.stdout = #"{"project": {"id": "p-1", "name": "gnat"}, "milestones": 2, "slices": 5}"#
        let accepted = try await client.planAccept(projectID: "p-1")
        XCTAssertEqual(runner.lastArguments, ["plan-accept", "--project", "p-1", "--json"])
        XCTAssertEqual(accepted.slices, 5)
    }

    func testInfoAsksForARefreshOnlyWhenTold() async throws {
        let runner = RecordingRunner()
        runner.stdout = #"{"project": {"id": "p", "name": "n", "conventions": ""}, "milestones": [], "slices": []}"#
        let client = NatClient(commandRunner: runner)

        _ = try await client.info(projectID: "p")
        XCTAssertEqual(runner.lastArguments, ["info", "--project", "p", "--json"])
        _ = try await client.info(projectID: "p", refresh: true)
        XCTAssertEqual(runner.lastArguments, ["info", "--project", "p", "--json", "--refresh"])
    }

    func testAClientWithoutProjectProposalsRefuses() async {
        let client = MockActivityClientForProposals()
        do {
            _ = try await client.planProposal(projectID: "p")
            XCTFail("expected a refusal")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("plan-proposal --project"))
        }
        do {
            _ = try await client.planAccept(projectID: "p")
            XCTFail("expected a refusal")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("plan-accept --project"))
        }
    }
}

/// A client implementing nothing it need not — what the protocol's defaults
/// answer for.
private struct MockActivityClientForProposals: NatClientProtocol {
    func info(projectID: String) async throws -> ProjectInfo { throw NatError.missingOutput }
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
