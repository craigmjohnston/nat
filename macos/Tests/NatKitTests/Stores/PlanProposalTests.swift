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
    {"title": "Parse", "milestone": " M1: Parser ", "description": "  Read **rows**.\\n\\n- one\\n", "depends_on": [" Tokenize "]}
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
            .init(name: "M1: Parser", slices: [
                "Tokenize",
                .init(name: "Parse", brief: "Read **rows**.\n\n- one", dependsOn: ["Tokenize"]),
            ]),
            .init(name: "M2: Ledger", slices: ["Post rows"]),
            .init(name: "M3: Empty", slices: []),
        ])
        XCTAssertEqual(proposal?.milestoneCount, 3)
        XCTAssertEqual(proposal?.sliceCount, 3)
    }

    /// A project's proposal may file every slice into milestones the project
    /// already has and create none — the Plan section still has to draw it.
    func testSlicesUnderMilestonesTheProposalDoesNotCreateAreGroupedAfterTheNewOnes() throws {
        let proposal = try JSONDecoder().decode(PlanProposal.self, from: Data("""
        {"project": "p-1", "name": "", "plan": {
          "milestones": [{"name": "M9: New"}],
          "slices": [
            {"title": "Open only partly-done milestones", "milestone": "M53: App interaction fixes"},
            {"title": "Into the new one", "milestone": "m9: new"},
            {"title": "A later one", "milestone": " M12: Later "},
            {"title": "One titlebar band", "milestone": " m53: App Interaction Fixes "}
          ]
        }}
        """.utf8))

        XCTAssertEqual(proposal.milestones, [
            .init(name: "M9: New", slices: ["Into the new one"]),
            .init(name: "M53: App interaction fixes", slices: ["Open only partly-done milestones", "One titlebar band"], isNew: false),
            .init(name: "M12: Later", slices: ["A later one"], isNew: false),
        ])
        XCTAssertEqual(proposal.milestoneCount, 1, "accepting creates one milestone, not three")
        XCTAssertEqual(proposal.sliceCount, 4)
        XCTAssertEqual(proposal.folders.map(\.title), ["M9: New", "M53: App interaction fixes", "M12: Later"])
        XCTAssertEqual(proposal.folders.map(\.total), [1, 2, 1])
        XCTAssertEqual(proposal.folders.map(\.isNew), [true, false, false], "only the milestone accepting creates is new")
    }

    /// A project's proposal that supersedes work already planned carries
    /// what it removes, moves and edits, trimmed as everything else is.
    func testTheProposalCarriesItsRemovalsMovesAndEdits() throws {
        let proposal = try JSONDecoder().decode(PlanProposal.self, from: Data("""
        {"project": "p-1", "name": "", "plan": {"milestones": [{"name": "M9: New"}], "slices": [],
          "remove": [" Old work "],
          "move": [{"slice": "Wandering", "milestone": " M9: New "}],
          "edit": [{"slice": "Rewritten", "description": "  A new **brief**.\\n"}]
        }}
        """.utf8))

        XCTAssertEqual(proposal.removals, ["Old work"])
        XCTAssertEqual(proposal.moves, [.init(name: "Wandering", milestone: "M9: New")])
        XCTAssertEqual(proposal.edits, [.init(name: "Rewritten", brief: "A new **brief**.")])
        XCTAssertTrue(proposal.changesBoard)
        XCTAssertEqual(ProposalText.removalWarning(count: 1), "Accepting also removes 1 task already planned.")
        XCTAssertEqual(ProposalText.removalWarning(count: 2), "Accepting also removes 2 tasks already planned.")
        XCTAssertEqual(ProposalText.moveDestination("M9: New"), "→ M9: New")
    }

    /// An edit may rename alone (nat writes its description empty), rewrite
    /// alone, or both; a blank title is no rename.
    func testAnEditCarriesItsNewTitle() throws {
        let proposal = try JSONDecoder().decode(PlanProposal.self, from: Data("""
        {"project": "p-1", "name": "", "plan": {"slices": [], "edit": [
          {"slice": "A", "title": " B ", "description": ""},
          {"slice": "C", "title": "D", "description": "New."},
          {"slice": "E", "title": "  ", "description": "Newer."}
        ]}}
        """.utf8))

        XCTAssertEqual(proposal.edits, [
            .init(name: "A", title: "B", brief: ""),
            .init(name: "C", title: "D", brief: "New."),
            .init(name: "E", brief: "Newer."),
        ])
        XCTAssertEqual(ProposalText.renamedFrom("A"), "Renamed from A")
    }

    func testAProposalWithoutTheListsChangesNothingOnTheBoard() throws {
        let proposal = try JSONDecoder().decode(PlanProposal.self, from: Data("""
        {"project": "p-1", "name": "", "plan": {"milestones": [{"name": "M9"}], "slices": []}}
        """.utf8))

        XCTAssertEqual(proposal.removals, [])
        XCTAssertEqual(proposal.moves, [])
        XCTAssertEqual(proposal.edits, [])
        XCTAssertFalse(proposal.changesBoard)
    }

    func testAProposalCreatingNoMilestoneStillHoldsItsSlices() throws {
        let proposal = try JSONDecoder().decode(PlanProposal.self, from: Data("""
        {"project": "p-1", "name": "", "plan": {"milestones": [], "slices": [
          {"title": "One", "milestone": "M53: App interaction fixes"},
          {"title": "Two", "milestone": "M53: App interaction fixes"}
        ]}}
        """.utf8))

        XCTAssertEqual(proposal.milestoneCount, 0)
        XCTAssertEqual(proposal.sliceCount, 2)
        XCTAssertEqual(proposal.milestones, [.init(name: "M53: App interaction fixes", slices: ["One", "Two"], isNew: false)])
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

    func testASliceWithNoBriefOrDependenciesDecodesEmpty() throws {
        let proposal = try JSONDecoder().decode(PlanProposal.self, from: Data("""
        {"name": "n", "plan": {"milestones": [{"name": "M1"}], "slices": [
          {"title": "Bare", "milestone": "M1", "description": "", "depends_on": []},
          {"title": "Null", "milestone": "M1", "description": null, "depends_on": null}
        ]}}
        """.utf8))

        let slices = proposal.milestones[0].slices
        XCTAssertEqual(slices, [.init(name: "Bare"), .init(name: "Null")])
        XCTAssertEqual(slices.map(\.brief), ["", ""])
        XCTAssertEqual(slices.map(\.dependsOn), [[], []])
    }

    func testTheFixturesCarryBriefsAndDependencies() {
        let slices = Fixtures.proposal.milestones.flatMap(\.slices)
        XCTAssertEqual(slices.count, 14)
        XCTAssertTrue(slices.contains { $0.brief.isEmpty }, "one slice has no brief, as a plan may")
        XCTAssertGreaterThan(slices.filter { !$0.brief.isEmpty }.count, 10)
        let titles = Set(slices.map(\.name))
        XCTAssertTrue(slices.contains { !$0.dependsOn.isEmpty })
        XCTAssertTrue(slices.allSatisfy { $0.dependsOn.allSatisfy(titles.contains) }, "every dependency is a proposed slice")
        let revision = Fixtures.revisionProposal.milestones.flatMap(\.slices)
        XCTAssertTrue(revision.allSatisfy { !$0.brief.isEmpty })
        XCTAssertEqual(revision.first?.dependsOn, ["Diff tab remembers its scroll"])
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
        XCTAssertEqual(folders[1].slices[2].sliceID, PlanProposal.sliceID(milestone: 1, slice: 2))
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
        XCTAssertEqual(ProposalText.counts(milestones: 4, slices: 14), "4 milestones · 14 tasks")
        XCTAssertEqual(ProposalText.counts(milestones: 1, slices: 1), "1 milestone · 1 task")
        XCTAssertEqual(
            ProposalText.acceptCaption(name: "rust-importer"),
            "Accepting writes the plan to local storage as “rust-importer”.")
        XCTAssertEqual(
            ProposalText.acceptedSubtitle(milestones: 4, slices: 14),
            "4 milestones · 14 tasks written locally. Select a task to begin.")
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

    private func proposal(_ name: String = "importer", slices: [PlanProposal.ProposedSlice] = ["One", "Two"]) -> PlanProposal {
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
        XCTAssertNil(appModel.activeProposal, "no file is a proposal accepted elsewhere — the file is the one source")
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

/// The workshop pane's tabs: which there are, and which is up.
final class WorkshopTabTests: XCTestCase {
    func testNoTabsBeforeLaunchOrOnceTheSessionHasEnded() {
        XCTAssertEqual(WorkshopTab.available(launched: false, hasProposal: false), [])
        XCTAssertEqual(WorkshopTab.available(launched: false, hasProposal: true), [], "a proposal outliving its session has no tabs")
    }

    func testTerminalFromLaunchAndPlanBesideItOnceThereIsAProposal() {
        XCTAssertEqual(WorkshopTab.available(launched: true, hasProposal: false), [.terminal])
        XCTAssertEqual(WorkshopTab.available(launched: true, hasProposal: true), [.terminal, .plan])
    }

    func testEveryTabMapsToATitlebarTabOfItsOwn() {
        XCTAssertEqual(WorkshopTab.terminal.titlebarTab.label, "Terminal")
        XCTAssertEqual(WorkshopTab.plan.titlebarTab.label, "Plan")
        let ids = WorkshopTab.allCases.map(\.titlebarTab.id) + MainPaneTab.allCases.map(\.titlebarTab.id)
        XCTAssertEqual(Set(ids).count, ids.count, "a workshop tab never shares an id with a slice's")
        XCTAssertEqual(MainPaneTab.changes.titlebarTab.label, "Changes")
    }
}

/// The workshop pane's tab as the app moves it: Terminal at launch, Plan on
/// a proposal's first arrival, left alone by a revision.
@MainActor
final class WorkshopTabFlowTests: XCTestCase {
    private func model(client: ProposingClient) async -> AppModel {
        let config = NatProjectConfig(
            projects: ["proj-a": ProjectConfig(name: "A", slicesDSID: "ds-a", workingDir: "/a")],
            workshopAgent: AgentModel(model: "opus", effort: "high"))
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            launchSettleWait: { await Task.yield() })
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        _ = appModel.openUntitledTab()
        return appModel
    }

    private func proposal(_ slices: [PlanProposal.ProposedSlice] = ["One", "Two"]) -> PlanProposal {
        PlanProposal(name: "p", milestones: [.init(name: "M1", slices: slices)])
    }

    /// Waits, bounded, for the activity poll a kill kicks to see the
    /// planner gone.
    private func sessionGone(_ appModel: AppModel) async {
        for _ in 0..<250 where appModel.planningAgent != nil {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
    }

    func testBeforeLaunchThereAreNoTabsAndNothingToShow() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        XCTAssertFalse(appModel.workshopLaunched)
        XCTAssertEqual(appModel.workshopTabs, [])
        XCTAssertNil(appModel.workshopTab)

        appModel.showWorkshopTab(.terminal)
        XCTAssertNil(appModel.workshopTab, "a tab the workshop does not have is never put up")
    }

    func testLaunchPutsTheTerminalUpAndAProposalsFirstArrivalPutsThePlanUp() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        XCTAssertTrue(appModel.workshopLaunched)
        XCTAssertEqual(appModel.workshopTabs, [.terminal])
        XCTAssertEqual(appModel.workshopTab, .terminal)

        client.proposal = proposal()
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.workshopTabs, [.terminal, .plan])
        XCTAssertEqual(appModel.workshopTab, .plan)
    }

    func testARevisionReplacesThePlanWithoutSwitchingTabs() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()

        appModel.showWorkshopTab(.terminal)
        client.proposal = proposal(["Revised"])
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.workshopTab, .terminal, "a revision leaves the terminal up")
        XCTAssertEqual(appModel.activeProposal?.milestones.first?.slices, ["Revised"])

        appModel.showWorkshopTab(.plan)
        client.proposal = proposal(["Again"])
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.workshopTab, .plan, "and the plan up")

        await appModel.refreshProposals()
        XCTAssertEqual(appModel.workshopTab, .plan, "the same proposal read again changes nothing")
    }

    func testAProposalGoneTakesThePlanTabWithIt() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()

        client.proposal = nil
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.workshopTabs, [.terminal])
        XCTAssertEqual(appModel.workshopTab, .terminal)
    }

    func testKeepWorkshoppingGoesBackToTheTerminal() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()
        XCTAssertEqual(appModel.workshopTab, .plan)

        appModel.keepWorkshopping()
        XCTAssertEqual(appModel.workshopTab, .terminal)
    }

    func testEndingTheSessionTakesBothTabsAway() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()

        let refusal = await appModel.closeWorkshopTab()
        XCTAssertNil(refusal)
        await sessionGone(appModel)
        XCTAssertFalse(appModel.workshopLaunched)
        XCTAssertEqual(appModel.workshopTabs, [])
        XCTAssertNil(appModel.workshopTab)
    }

    func testARelaunchStartsBackOnTheTerminal() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()
        _ = await appModel.closeWorkshopTab()
        await sessionGone(appModel)

        await appModel.launchWorkshop(request: "Again.")
        XCTAssertEqual(appModel.workshopTab, .terminal, "the proposal is still there, but a launch opens on the terminal")
    }

    func testAPlanRowPutsThePlanUpScrolledToItsSlice() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()
        appModel.showWorkshopTab(.terminal)

        appModel.showProposedSlice("proposed-0-1")
        XCTAssertEqual(appModel.workshopTab, .plan)
        XCTAssertEqual(appModel.workshopPlanScroll, WorkshopPlanScroll(sliceID: "proposed-0-1", token: 1))

        appModel.showProposedSlice("proposed-0-1")
        XCTAssertEqual(appModel.workshopPlanScroll?.token, 2, "asking for the same slice again still scrolls")
    }

    func testAnEditRowUnfoldsAndFoldsItsBrief() async {
        let appModel = await model(client: ProposingClient())

        appModel.toggleProposalEdit("Rewritten")
        XCTAssertEqual(appModel.expandedProposalEdits, ["Rewritten"])
        appModel.toggleProposalEdit("Rewritten")
        XCTAssertEqual(appModel.expandedProposalEdits, [])
    }

    func testAPlanBoxFoldsAndOpensOnItsHeader() async {
        let appModel = await model(client: ProposingClient())

        XCTAssertEqual(appModel.foldedProposedSlices, [], "every box starts open")
        appModel.toggleProposedSliceFold("proposed-0-1")
        XCTAssertEqual(appModel.foldedProposedSlices, ["proposed-0-1"])
        appModel.toggleProposedSliceFold("proposed-0-1")
        XCTAssertEqual(appModel.foldedProposedSlices, [])
    }

    func testAPlanRowUnfoldsTheBoxItScrollsTo() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        await appModel.launchWorkshop(request: "A plan.")
        client.proposal = proposal()
        await appModel.refreshProposals()
        appModel.toggleProposedSliceFold("proposed-0-0")
        appModel.toggleProposedSliceFold("proposed-0-1")

        appModel.showProposedSlice("proposed-0-1")
        XCTAssertEqual(appModel.foldedProposedSlices, ["proposed-0-0"], "only the box scrolled to opens")
    }

    func testAPlanRowWithNoPlanTabAsksForNoScroll() async {
        let client = ProposingClient()
        let appModel = await model(client: client)
        client.proposal = proposal()
        await appModel.refreshProposals()

        appModel.showProposedSlice("proposed-0-0")
        XCTAssertNil(appModel.workshopTab)
        XCTAssertNil(appModel.workshopPlanScroll)
    }
}
