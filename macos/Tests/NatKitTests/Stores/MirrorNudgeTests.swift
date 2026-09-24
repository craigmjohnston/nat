import XCTest
import NatFixtures
@testable import NatKit

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

private let engineering = NotionPlace(id: "ds-1", kind: .database, title: "Engineering / Projects")
private let plans = NotionPlace(id: "pg-1", kind: .page, title: "Plans")

/// A double for the workspace search and the mirror, recording what it is asked.
private final class MirroringClient: WorkspaceWorkshopClient, @unchecked Sendable {
    private let mirrorLock = NSLock()
    private var _queries: [String] = []
    private var _mirrors: [(project: String, parent: NotionPlace)] = []
    var places: [NotionPlace] = [engineering, plans]
    var searchFailure: Error?
    var mirrorFailure: Error?
    var proposal: PlanProposal?
    /// What a search waits for before answering, for the superseded-query test.
    var searchDelay: [String: UInt64] = [:]

    var queries: [String] { mirrorLock.withLock { _queries } }
    var mirrors: [(project: String, parent: NotionPlace)] { mirrorLock.withLock { _mirrors } }

    override func planProposal(workspaceID: String) async throws -> PlanProposal? { proposal }

    override func planAccept(workspaceID: String, name: String) async throws -> PlanAccepted {
        PlanAccepted(project: ProjectEntry(id: "proj-new", name: name), milestones: 1, slices: 2)
    }

    override func notionSearch(query: String) async throws -> [NotionPlace] {
        mirrorLock.withLock { _queries.append(query) }
        if let delay = searchDelay[query] { try? await Task.sleep(nanoseconds: delay) }
        if let searchFailure { throw searchFailure }
        return places.filter { query.isEmpty || $0.title.lowercased().contains(query.lowercased()) }
    }

    override func projectMirror(projectID: String, parent: NotionPlace) async throws -> ProjectMirrored {
        mirrorLock.withLock { _mirrors.append((projectID, parent)) }
        if let mirrorFailure { throw mirrorFailure }
        return ProjectMirrored(
            project: ProjectEntry(id: "proj-notion", name: "Mirrored"),
            replaced: projectID, milestones: 1, slices: 2)
    }
}

final class MirrorNudgeMemoryTests: XCTestCase {
    func testArmingAndDisarmingSurviveANewInstanceOverTheSameDefaults() throws {
        let suite = "nat.tests.mirror.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }

        let first = MirrorNudgeMemory(defaults: defaults)
        XCTAssertEqual(first.pending, [])
        first.arm("a")
        first.arm("a")
        first.arm("b")
        XCTAssertEqual(MirrorNudgeMemory(defaults: defaults).pending, ["a", "b"], "a relaunch still owes them")

        first.disarm("a")
        XCTAssertFalse(MirrorNudgeMemory(defaults: defaults).isPending("a"))
        XCTAssertTrue(MirrorNudgeMemory(defaults: defaults).isPending("b"))
    }

    func testAnInMemoryMemoryIsItsOwn() {
        let one = MirrorNudgeMemory.inMemory()
        one.arm("a")
        XCTAssertTrue(one.isPending("a"))
        XCTAssertEqual(MirrorNudgeMemory.inMemory().pending, [])
    }
}

@MainActor
final class MirrorNudgeFlowTests: XCTestCase {
    private func config() -> NatProjectConfig {
        NatProjectConfig(projects: [
            "proj-a": ProjectConfig(name: "A", slicesDSID: "ds-a", workingDir: "/a"),
            "proj-new": ProjectConfig(name: "From Nat", slicesDSID: "", workingDir: "", backend: .local),
            "proj-notion": ProjectConfig(name: "Mirrored", slicesDSID: "ds-m", workingDir: ""),
        ])
    }

    private func model(
        client: MirroringClient, memory: MirrorNudgeMemory = .inMemory()
    ) async -> AppModel {
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config())),
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            launchSettleWait: { await Task.yield() },
            mirrorNudgeMemory: memory
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return appModel
    }

    /// The Untitled tab's proposal accepted, which is what arms the card.
    private func accepted(_ appModel: AppModel, _ client: MirroringClient) async {
        appModel.openUntitledTab()
        client.proposal = PlanProposal(name: "importer", milestones: [.init(name: "M1", slices: ["One", "Two"])])
        await appModel.refreshProposals()
        await appModel.acceptProposal()
    }

    func testAcceptingAPlanArmsTheCardOnTheNewProjectAlone() async {
        let client = MirroringClient()
        let appModel = await model(client: client)
        XCTAssertFalse(appModel.mirrorNudgeShown)

        await accepted(appModel, client)

        XCTAssertEqual(appModel.activeProjectID, "proj-new")
        XCTAssertTrue(appModel.mirrorNudgeShown)
        await appModel.activateProject("proj-a")
        XCTAssertFalse(appModel.mirrorNudgeShown, "another project was never asked")
    }

    func testDismissingIsRememberedAcrossARelaunch() async {
        let client = MirroringClient()
        let memory = MirrorNudgeMemory.inMemory()
        let appModel = await model(client: client, memory: memory)
        await accepted(appModel, client)

        let relaunched = await model(client: client, memory: memory)
        await relaunched.activateProject("proj-new")
        XCTAssertTrue(relaunched.mirrorNudgeShown, "not answered yet, so still owed")

        appModel.dismissMirrorNudge()
        XCTAssertFalse(appModel.mirrorNudgeShown)

        let again = await model(client: client, memory: memory)
        await again.activateProject("proj-new")
        XCTAssertFalse(again.mirrorNudgeShown, "the ✕ is for good")
    }

    func testDismissingWithNoProjectIsNothing() async {
        let appModel = AppModel(mirrorNudgeMemory: .inMemory())
        appModel.dismissMirrorNudge()
        XCTAssertFalse(appModel.mirrorNudgeShown)
    }

    func testAProjectThatAlreadyMirrorsIsNeverAsked() async {
        let memory = MirrorNudgeMemory.inMemory()
        memory.arm("proj-notion")
        let appModel = await model(client: MirroringClient(), memory: memory)
        await appModel.activateProject("proj-notion")
        XCTAssertFalse(appModel.mirrorNudgeShown)
    }

    func testAnUntitledTabIsNeverAsked() async {
        let memory = MirrorNudgeMemory.inMemory()
        let appModel = await model(client: MirroringClient(), memory: memory)
        let tab = appModel.openUntitledTab()
        memory.arm(tab)
        XCTAssertFalse(appModel.mirrorNudgeShown)
    }

    func testMirroringHandsTheTabToTheNewProjectAndStopsAsking() async {
        let client = MirroringClient()
        let memory = MirrorNudgeMemory.inMemory()
        let appModel = await model(client: client, memory: memory)
        await accepted(appModel, client)
        let position = appModel.projectTabs.firstIndex { $0.id == "proj-new" }

        let refusal = await appModel.mirrorActiveProject(into: engineering)

        XCTAssertNil(refusal)
        XCTAssertEqual(client.mirrors.map(\.project), ["proj-new"])
        XCTAssertEqual(client.mirrors.map(\.parent), [engineering])
        XCTAssertEqual(appModel.activeProjectID, "proj-notion")
        XCTAssertNil(appModel.projectTabs.first { $0.id == "proj-new" })
        XCTAssertEqual(appModel.projectTabs.firstIndex { $0.id == "proj-notion" }, position, "in the place it held")
        XCTAssertFalse(appModel.mirrorNudgeShown)
        XCTAssertFalse(memory.isPending("proj-new"))
        XCTAssertNil(appModel.acceptedPlanShown)
    }

    func testMirroringOntoATabAlreadyOpenJustClosesTheOldOne() async {
        let client = MirroringClient()
        let appModel = await model(client: client)
        await accepted(appModel, client)
        XCTAssertTrue(appModel.projectTabs.contains { $0.id == "proj-notion" }, "the config already names it")

        _ = await appModel.mirrorActiveProject(into: plans)

        XCTAssertEqual(appModel.projectTabs.filter { $0.id == "proj-notion" }.count, 1)
        XCTAssertNil(appModel.projectTabs.first { $0.id == "proj-new" })
    }

    func testMirroringWithNoTabForTheProjectAddsOne() async {
        let client = MirroringClient()
        let appModel = await model(client: client)
        await accepted(appModel, client)
        // The active project's tab is gone from the strip (closed elsewhere).
        await appModel.closeProject("proj-notion")
        _ = await appModel.mirrorActiveProject(into: plans)
        XCTAssertTrue(appModel.projectTabs.contains { $0.id == "proj-notion" })
    }

    func testARefusedMirrorLeavesTheCardAndTheTabAsTheyWere() async {
        let client = MirroringClient()
        client.mirrorFailure = NatError.commandFailed("no access to that page")
        let appModel = await model(client: client)
        await accepted(appModel, client)

        let refusal = await appModel.mirrorActiveProject(into: engineering)

        XCTAssertEqual(refusal, "no access to that page")
        XCTAssertEqual(appModel.activeProjectID, "proj-new")
        XCTAssertTrue(appModel.mirrorNudgeShown)
    }

    func testAnUnexpectedFailureStillSaysSomething() async {
        let client = MirroringClient()
        client.mirrorFailure = NatError.missingOutput
        let appModel = await model(client: client)
        await accepted(appModel, client)
        let missing = await appModel.mirrorActiveProject(into: engineering)
        XCTAssertNotNil(missing)

        client.mirrorFailure = NSError(domain: "test", code: 1, userInfo: [NSLocalizedDescriptionKey: "boom"])
        let other = await appModel.mirrorActiveProject(into: engineering)
        XCTAssertEqual(other, "boom")
    }

    func testThereIsNothingToMirrorOnAnUntitledTab() async {
        let appModel = await model(client: MirroringClient())
        appModel.openUntitledTab()
        let refusal = await appModel.mirrorActiveProject(into: engineering)
        XCTAssertEqual(refusal, "There is no project open to mirror.")
    }

    func testThePickerIsOverTheSameClient() async {
        let client = MirroringClient()
        let appModel = await model(client: client)
        let picker = appModel.makeNotionPicker()
        await picker.search()
        XCTAssertEqual(picker.places, [engineering, plans])
        XCTAssertFalse(appModel.mirrorPickerPresented)
    }
}

@MainActor
final class NotionPickerModelTests: XCTestCase {
    func testTheFirstSearchListsEverythingAndATypedOneNarrowsIt() async {
        let client = MirroringClient()
        let picker = NotionPickerModel(client: client)
        await picker.search()
        XCTAssertEqual(picker.places.count, 2)
        XCTAssertFalse(picker.isSearching)

        picker.query = "  plans "
        await picker.search()
        XCTAssertEqual(picker.places, [plans])
        XCTAssertEqual(client.queries, ["", "plans"], "the query is trimmed")
    }

    func testAChosenRowTheNewListLacksIsUnchosen() async {
        let picker = NotionPickerModel(client: MirroringClient())
        await picker.search()
        picker.selectedID = "ds-1"
        XCTAssertEqual(picker.selection, engineering)
        XCTAssertTrue(picker.canCreate)

        picker.query = "plans"
        await picker.search()
        XCTAssertNil(picker.selectedID)
        XCTAssertFalse(picker.canCreate)

        picker.selectedID = "pg-1"
        picker.query = ""
        await picker.search()
        XCTAssertEqual(picker.selectedID, "pg-1", "a row still listed stays chosen")
    }

    func testAFailedSearchSaysWhyAndClearsTheList() async {
        let client = MirroringClient()
        let picker = NotionPickerModel(client: client)
        await picker.search()
        picker.selectedID = "ds-1"

        client.searchFailure = NatError.commandFailed("no token")
        await picker.search()
        XCTAssertEqual(picker.searchError, "no token")
        XCTAssertEqual(picker.places, [])
        XCTAssertNil(picker.selectedID)

        client.searchFailure = NSError(domain: "test", code: 1, userInfo: [NSLocalizedDescriptionKey: "offline"])
        await picker.search()
        XCTAssertEqual(picker.searchError, "offline")

        client.searchFailure = nil
        await picker.search()
        XCTAssertNil(picker.searchError)
    }

    func testANewerQuerySupersedesAnOlderOneStillInFlight() async {
        let client = MirroringClient()
        client.searchDelay = ["eng": 150_000_000]
        let picker = NotionPickerModel(client: client)

        picker.query = "eng"
        async let slow: Void = picker.search()
        try? await Task.sleep(nanoseconds: 20_000_000)
        picker.query = "plans"
        await picker.search()
        await slow

        XCTAssertEqual(picker.places, [plans], "the answer for text no longer typed is dropped")
        XCTAssertFalse(picker.isSearching)
    }

    func testASupersededFailureIsDroppedToo() async {
        let client = MirroringClient()
        client.searchDelay = ["eng": 100_000_000]
        client.searchFailure = NatError.commandFailed("late")
        let picker = NotionPickerModel(client: client)
        picker.query = "eng"
        async let slow: Void = picker.search()
        try? await Task.sleep(nanoseconds: 20_000_000)
        client.searchFailure = nil
        picker.query = "plans"
        await picker.search()
        await slow
        XCTAssertNil(picker.searchError)
        XCTAssertEqual(picker.places, [plans])
    }

    func testCreateNeedsARowAndReportsWhatItRefusedWith() async {
        let picker = NotionPickerModel(client: MirroringClient())
        await picker.search()
        var asked: [NotionPlace] = []

        let none = await picker.create { asked.append($0); return nil }
        XCTAssertFalse(none, "no row chosen")
        XCTAssertEqual(asked, [])

        picker.selectedID = "ds-1"
        let refused = await picker.create { asked.append($0); return "no access" }
        XCTAssertFalse(refused)
        XCTAssertEqual(picker.createError, "no access")
        XCTAssertEqual(picker.selectedID, "ds-1", "still chosen, to try again")
        XCTAssertFalse(picker.isCreating)

        let took = await picker.create { asked.append($0); return nil }
        XCTAssertTrue(took)
        XCTAssertNil(picker.createError)
        XCTAssertEqual(asked, [engineering, engineering])
    }
}

final class MirrorClientTests: XCTestCase {
    func testSearchPassesTheQueryAndReadsTheListing() async throws {
        let runner = StubRunner()
        runner.stdout = #"{"places":[{"id":"ds-1","kind":"database","title":"Projects"},{"id":"pg","kind":"page","title":"Plans"}]}"#
        let client = NatClient(commandRunner: runner)

        let places = try await client.notionSearch(query: "pro")

        XCTAssertEqual(runner.lastArguments, ["notion-search", "--json", "--query", "pro"])
        XCTAssertEqual(places, [
            NotionPlace(id: "ds-1", kind: .database, title: "Projects"),
            NotionPlace(id: "pg", kind: .page, title: "Plans"),
        ])
        _ = try await client.notionSearch(query: "")
        XCTAssertEqual(runner.lastArguments, ["notion-search", "--json"])
    }

    func testMirrorNamesTheProjectAndTheParent() async throws {
        let runner = StubRunner()
        runner.stdout = #"{"project":{"id":"page-1","name":"Mine","slices_ds_id":"ds"},"replaced":"local-1","milestones":2,"slices":5}"#

        let mirrored = try await NatClient(commandRunner: runner)
            .projectMirror(projectID: "local-1", parent: plans)

        XCTAssertEqual(runner.lastArguments, [
            "project-mirror", "--project", "local-1", "--json", "--parent", "pg-1", "--parent-kind", "page",
        ])
        XCTAssertEqual(mirrored.project.id, "page-1")
        XCTAssertEqual(mirrored.replaced, "local-1")
        XCTAssertEqual(mirrored.slices, 5)
    }

    func testARefusalCarriesNatsWords() async {
        let runner = StubRunner()
        runner.exitCode = 1
        runner.stderr = "project-mirror: \"A\" has been started"
        do {
            _ = try await NatClient(commandRunner: runner).projectMirror(projectID: "x", parent: engineering)
            XCTFail("expected a refusal")
        } catch NatError.commandFailed(let message) {
            XCTAssertTrue(message.contains("has been started"))
        } catch {
            XCTFail("\(error)")
        }
    }

}
