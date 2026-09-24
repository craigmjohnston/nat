import XCTest
import NatFixtures
@testable import NatKit

/// A stub `nat` that answers one canned stdout (or refusal) and records what it
/// was asked, for the two proposal commands' argument shapes.
private final class StubRunner: CommandRunning, @unchecked Sendable {
    var stdout = ""
    var stderr = ""
    var exitCode: Int32 = 0
    private(set) var lastArguments: [String] = []

    func run(
        executable: String, arguments: [String], workingDirectory: String?, standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastArguments = arguments
        return (Data(stdout.utf8), Data(stderr.utf8), exitCode)
    }
}

private let proposalJSON = """
{"proposal": {"workspace": "ws-1", "name": "rust-importer", "plan": {
  "milestones": [{"name": "M1: Parser"}, {"name": " M2: Ledger "}, {"name": "M3: Empty"}],
  "slices": [
    {"title": "Tokenize", "milestone": "M1: Parser"},
    {"title": " Post rows ", "milestone": "m2: ledger"},
    {"title": "Parse", "milestone": " M1: Parser ", "description": "ignored", "depends_on": ["Tokenize"]}
  ]
}}}
"""

/// The proposal as the model holds it, and the two commands it is read and
/// accepted through.
final class PlanProposalModelTests: XCTestCase {
    func testTheProposalFileGroupsSlicesUnderTheirMilestonesTheWayTheCLIMatchesThem() async throws {
        let runner = StubRunner()
        runner.stdout = proposalJSON

        let proposal = try await NatClient(commandRunner: runner).planProposal(workspaceID: "ws-1")

        XCTAssertEqual(runner.lastArguments, ["plan-proposal", "--workspace", "ws-1", "--json"])
        XCTAssertEqual(proposal?.name, "rust-importer")
        XCTAssertEqual(proposal?.milestones, [
            .init(name: "M1: Parser", slices: ["Tokenize", "Parse"]),
            .init(name: "M2: Ledger", slices: ["Post rows"]),
            .init(name: "M3: Empty", slices: []),
        ])
        XCTAssertEqual(proposal?.milestoneCount, 3)
        XCTAssertEqual(proposal?.sliceCount, 3)
    }

    func testNoProposalYetReadsAsNil() async throws {
        let runner = StubRunner()
        runner.stdout = #"{"proposal": null}"#
        let proposal = try await NatClient(commandRunner: runner).planProposal(workspaceID: "ws-1")
        XCTAssertNil(proposal)
    }

    func testAPlanWithNoListsDecodesAsEmpty() throws {
        let proposal = try JSONDecoder().decode(
            PlanProposal.self, from: Data(#"{"name": "n", "plan": {}}"#.utf8))
        XCTAssertEqual(proposal.milestones, [])
        XCTAssertEqual(proposal.sliceCount, 0)
    }

    func testTheFoldersAreTheRailsOwnAllTodo() {
        let folders = Fixtures.proposal.folders

        XCTAssertEqual(folders.count, 4)
        XCTAssertEqual(folders[0].title, "M1: Parser core")
        XCTAssertEqual(folders[0].milestoneID, "M1: Parser core")
        XCTAssertEqual(folders[0].total, 4)
        XCTAssertEqual(folders[0].done, 0)
        XCTAssertFalse(folders[0].isCurrent)
        XCTAssertEqual(folders[0].slices.map(\.glyph), Array(repeating: .todo, count: 4))
        XCTAssertEqual(folders[0].slices[1].name, "Parse rows into typed records")
        let ids = folders.flatMap { $0.slices.map(\.sliceID) }
        XCTAssertEqual(Set(ids).count, ids.count, "every proposed slice row has an id of its own")
    }

    func testAcceptPassesTheNameAndDecodesWhatWentIn() async throws {
        let runner = StubRunner()
        runner.stdout = """
        {"project": {"id": "p-1", "name": "Mine", "backend": "local"}, "milestones": 4, "slices": 14}
        """

        let accepted = try await NatClient(commandRunner: runner).planAccept(workspaceID: "ws-1", name: "Mine")

        XCTAssertEqual(
            runner.lastArguments, ["plan-accept", "--workspace", "ws-1", "--name", "Mine", "--json"])
        XCTAssertEqual(accepted.project.id, "p-1")
        XCTAssertEqual(accepted.milestones, 4)
        XCTAssertEqual(accepted.slices, 14)
    }

    func testARefusedAcceptCarriesNatsWords() async {
        let runner = StubRunner()
        runner.stderr = "no proposal for this workspace"
        runner.exitCode = 1
        do {
            _ = try await NatClient(commandRunner: runner).planAccept(workspaceID: "ws-1", name: "Mine")
            XCTFail("should refuse")
        } catch let error as NatError {
            guard case .commandFailed(let message) = error else { return XCTFail("\(error)") }
            XCTAssertTrue(message.contains("no proposal"))
        } catch {
            XCTFail("\(error)")
        }
    }

    func testTheWordsAreThePluralisedMocksOwn() {
        XCTAssertEqual(ProposalText.counts(milestones: 4, slices: 14), "4 milestones · 14 slices")
        XCTAssertEqual(ProposalText.counts(milestones: 1, slices: 1), "1 milestone · 1 slice")
        XCTAssertEqual(
            ProposalText.acceptCaption(name: "rust-importer"),
            "Accepting writes the plan to local storage as “rust-importer”.")
        XCTAssertEqual(
            ProposalText.acceptedSubtitle(milestones: 4, slices: 14),
            "4 milestones · 14 slices written locally. Select a slice to begin.")
    }

    func testAClientThatDoesNotKnowTheCommandsRefusesThem() async {
        let client = MockActivityClient(response: .agents([]))
        do {
            _ = try await client.planProposal(workspaceID: "ws")
            XCTFail("should refuse")
        } catch {}
        do {
            _ = try await client.planAccept(workspaceID: "ws", name: "n")
            XCTFail("should refuse")
        } catch {}
    }

    func testTheFixtureClientProposesAndAccepts() async throws {
        let client = FixtureNatClient()
        let none = try await client.planProposal(workspaceID: "ws")
        XCTAssertNil(none)

        client.setProposal(Fixtures.proposal)
        let proposed = try await client.planProposal(workspaceID: "ws")
        XCTAssertEqual(proposed, Fixtures.proposal)

        let accepted = try await client.planAccept(workspaceID: "ws", name: "Mine")
        XCTAssertEqual(accepted.project.name, "Mine")
        XCTAssertEqual(accepted.slices, 14)
        XCTAssertEqual(client.writes, ["plan-accept --workspace ws --name Mine"])
        let after = try await client.planProposal(workspaceID: "ws")
        XCTAssertNil(after, "an accepted proposal is spent")

        let bare = try await client.planAccept(workspaceID: "ws", name: "Empty")
        XCTAssertEqual(bare.milestones, 0)
    }
}

/// A double serving proposals a test sets and recording every accept.
private final class ProposingClient: WorkspaceWorkshopClient, @unchecked Sendable {
    private let proposalLock = NSLock()
    private var _proposal: PlanProposal?
    private var _accepts: [(workspace: String, name: String)] = []
    var readFailure: Error?
    var acceptResult: Result<PlanAccepted, Error> = .success(
        PlanAccepted(project: ProjectEntry(id: "proj-new", name: "From Nat"), milestones: 4, slices: 14))

    var proposal: PlanProposal? {
        get { proposalLock.withLock { _proposal } }
        set { proposalLock.withLock { _proposal = newValue } }
    }
    var accepts: [(workspace: String, name: String)] { proposalLock.withLock { _accepts } }

    override func planProposal(workspaceID: String) async throws -> PlanProposal? {
        if let readFailure { throw readFailure }
        return proposal
    }

    override func planAccept(workspaceID: String, name: String) async throws -> PlanAccepted {
        proposalLock.withLock { _accepts.append((workspaceID, name)) }
        return try acceptResult.get()
    }
}

/// An Untitled tab's proposal: read into the rail, revised in place, and
/// accepted into a local project the tab becomes.
@MainActor
final class PlanProposalFlowTests: XCTestCase {
    private func model(
        client: ProposingClient, nudgePath: String = "/fake/nudge"
    ) async -> (AppModel, String) {
        let config = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A", slicesDSID: "ds-a", workingDir: "/a"),
                "proj-new": ProjectConfig(name: "From Nat", slicesDSID: "", workingDir: ""),
            ],
            workshopAgent: AgentModel(model: "opus", effort: "high")
        )
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            launchSettleWait: { await Task.yield() }
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: nudgePath)
        return (appModel, appModel.openUntitledTab())
    }

    private func proposal(_ name: String = "importer", slices: [String] = ["One", "Two"]) -> PlanProposal {
        PlanProposal(name: name, milestones: [.init(name: "M1", slices: slices)])
    }

    /// Launches the workshop so the session is live to be ended by an accept.
    private func workshop(_ appModel: AppModel) async {
        await appModel.launchWorkshop(request: "A plan.")
    }

    func testAProposalLandsInTheRailAndARevisedOneReplacesItInPlace() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        XCTAssertNil(appModel.activeProposal)

        client.proposal = proposal("first")
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.activeProposal?.name, "first")
        XCTAssertEqual(appModel.proposalName, "first")

        client.proposal = proposal("second", slices: ["Only"])
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.activeProposal?.sliceCount, 1)
        XCTAssertEqual(appModel.proposalName, "second", "an unedited name follows the agent's suggestion")
    }

    func testANameTheUserTypedSurvivesARevision() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        client.proposal = proposal("first")
        await appModel.refreshProposals()

        appModel.proposalName = "My Name"
        client.proposal = proposal("second")
        await appModel.refreshProposals()

        XCTAssertEqual(appModel.proposalName, "My Name")
    }

    func testAProposalThatWillNotReadLeavesTheRailAsItWas() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        client.proposal = proposal("first")
        await appModel.refreshProposals()

        client.readFailure = NatError.commandFailed("the proposal is not valid JSON")
        client.proposal = proposal("second")
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.activeProposal?.name, "first")

        client.readFailure = nil
        client.proposal = nil
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.activeProposal?.name, "first", "no file is no news")
    }

    func testTheNameFieldHasNothingToEditWithoutAProposal() async {
        let (appModel, _) = await model(client: ProposingClient())
        appModel.proposalName = "ignored"
        XCTAssertEqual(appModel.proposalName, "")
    }

    func testAProjectTabHasNoProposalAndNoName() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        await appModel.activateProject("proj-a")
        XCTAssertNil(appModel.activeProposal)
        XCTAssertEqual(appModel.proposalName, "")
    }

    func testKeepWorkshoppingAsksTheTerminalForFocusAndLeavesTheTree() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()
        let before = appModel.terminalFocusRequest

        appModel.keepWorkshopping()

        XCTAssertEqual(appModel.terminalFocusRequest, before + 1)
        XCTAssertNotNil(appModel.activeProposal)
    }

    func testAcceptMakesTheProjectTakesTheTabOverAndEndsTheWorkshop() async {
        let client = ProposingClient()
        let (appModel, tab) = await model(client: client)
        await workshop(appModel)
        client.proposal = proposal()
        await appModel.refreshProposals()
        let workspace = appModel.workspaceID(forTab: tab)!
        appModel.proposalName = "  From Nat  "

        await appModel.acceptProposal()

        XCTAssertEqual(client.accepts.map(\.workspace), [workspace])
        XCTAssertEqual(client.accepts.map(\.name), ["From Nat"])
        XCTAssertEqual(client.kills, [workspace], "accepting is the workshop's goodbye")
        XCTAssertFalse(appModel.projectTabs.contains { $0.id == tab })
        XCTAssertEqual(appModel.activeProjectID, "proj-new")
        XCTAssertEqual(appModel.projectTabs.last?.name, "From Nat")
        XCTAssertFalse(appModel.activeTabIsUntitled)
        XCTAssertNil(appModel.workspaceID(forTab: tab))
        XCTAssertNil(appModel.proposals[tab])
        XCTAssertFalse(appModel.proposalAccepting)
        XCTAssertNil(appModel.proposalError)

        XCTAssertEqual(appModel.acceptedPlanShown?.slices, 14)
        appModel.workshopSelected = true
        XCTAssertNil(appModel.acceptedPlanShown, "the accepted state gives way to whatever is selected")
    }

    func testAcceptedStateGivesWayToASelectedSlice() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()
        await appModel.acceptProposal()
        XCTAssertNotNil(appModel.acceptedPlanShown)

        appModel.selectedSliceID = "s-1"

        XCTAssertNil(appModel.acceptedPlanShown)
    }

    func testAnAcceptWithNoLiveSessionKillsNothing() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()

        await appModel.acceptProposal()

        XCTAssertEqual(client.kills, [])
        XCTAssertEqual(appModel.activeProjectID, "proj-new")
    }

    func testASessionThatWillNotDieDoesNotUndoAnAcceptedPlan() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        await workshop(appModel)
        client.proposal = proposal()
        await appModel.refreshProposals()
        client.killRefusal = "tmux would not"

        await appModel.acceptProposal()

        XCTAssertEqual(appModel.activeProjectID, "proj-new")
        XCTAssertNil(appModel.proposalError)
    }

    func testAnEmptyNameRefusesInlineAndWritesNothing() async {
        let client = ProposingClient()
        let (appModel, tab) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()
        appModel.proposalName = "   "

        await appModel.acceptProposal()

        XCTAssertEqual(appModel.proposalError, ProposalText.emptyNameError)
        XCTAssertTrue(client.accepts.isEmpty)
        XCTAssertEqual(appModel.activeProjectID, tab)

        appModel.proposalName = "Something"
        XCTAssertNil(appModel.proposalError, "typing takes the refusal down")
    }

    func testARefusalFromNatLeavesTheTabAndItsProposalAndSaysWhy() async {
        let client = ProposingClient()
        let (appModel, tab) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()
        client.acceptResult = .failure(NatError.commandFailed("the plan creates nothing"))

        await appModel.acceptProposal()

        XCTAssertEqual(appModel.proposalError, "the plan creates nothing")
        XCTAssertEqual(appModel.activeProjectID, tab)
        XCTAssertNotNil(appModel.activeProposal)
        XCTAssertFalse(appModel.proposalAccepting)
    }

    func testOtherFailuresAreSaidToo() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()

        client.acceptResult = .failure(NatError.invalidJSON("garbled", details: "not json"))
        await appModel.acceptProposal()
        XCTAssertNotNil(appModel.proposalError)

        struct Other: LocalizedError { var errorDescription: String? { "something else" } }
        client.acceptResult = .failure(Other())
        await appModel.acceptProposal()
        XCTAssertEqual(appModel.proposalError, "something else")
    }

    func testAcceptDoesNothingWithoutAProposalOrOnAProjectTab() async {
        let client = ProposingClient()
        let (appModel, _) = await model(client: client)
        await appModel.acceptProposal()
        XCTAssertTrue(client.accepts.isEmpty)

        await appModel.activateProject("proj-a")
        await appModel.acceptProposal()
        XCTAssertTrue(client.accepts.isEmpty)
    }

    func testClosingTheLastUntitledTabDropsItsProposal() async {
        let client = ProposingClient()
        let (appModel, tab) = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()
        XCTAssertNotNil(appModel.proposals[tab])

        await appModel.closeProject(tab)

        XCTAssertNil(appModel.proposals[tab])
    }

    /// The watch is the real one: a marker touched after the app started is
    /// what puts the proposal in the rail, inside its one-second poll.
    func testTouchingTheNudgeMarkerBringsTheProposalIn() async throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("proposal-watch-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let nudge = dir.appendingPathComponent("nudge")
        try Data().write(to: nudge)

        let client = ProposingClient()
        let (appModel, _) = await model(client: client, nudgePath: nudge.path)
        client.proposal = proposal("watched")

        try FileManager.default.setAttributes(
            [.modificationDate: Date().addingTimeInterval(60)], ofItemAtPath: nudge.path)

        for _ in 0..<60 where appModel.activeProposal == nil {
            try await Task.sleep(nanoseconds: 100_000_000)
        }
        XCTAssertEqual(appModel.activeProposal?.name, "watched")
    }
}
