import XCTest
import NatFixtures
@testable import NatKit

/// A double whose `project-open-folder` answers with a canned entry or refusal
/// and records the folders it was asked about.
final class FolderOpeningClient: WorkspaceWorkshopClient, @unchecked Sendable {
    private let folderLock = NSLock()
    private var _folders: [String] = []
    var folders: [String] { folderLock.withLock { _folders } }
    /// What the command answers, or the failure it throws.
    var folderResult: Result<ProjectEntry, Error> = .failure(NatError.commandFailed("unset"))

    override func projectOpenFolder(path: String) async throws -> ProjectEntry {
        folderLock.withLock { _folders.append(path) }
        return try folderResult.get()
    }
}

/// The starter card's two "open what already exists" affordances: a plan file
/// handed to the workshop, and a folder opened as a project.
@MainActor
final class StarterOpenExistingTests: XCTestCase {
    private var scratch: URL!

    override func setUp() {
        scratch = FileManager.default.temporaryDirectory
            .appendingPathComponent("starter-\(UUID().uuidString)", isDirectory: true)
        try? FileManager.default.createDirectory(at: scratch, withIntermediateDirectories: true)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: scratch)
    }

    private func file(_ name: String, _ content: String) -> URL {
        let url = scratch.appendingPathComponent(name)
        try? content.write(to: url, atomically: true, encoding: .utf8)
        return url
    }

    private func model(client: FolderOpeningClient) async -> (AppModel, String) {
        let config = NatProjectConfig(
            projects: ["proj-a": ProjectConfig(name: "A", slicesDSID: "ds-a", workingDir: "/a")],
            workshopAgent: AgentModel(model: "opus", effort: "high")
        )
        let appModel = AppModel(
            configReader: MockConfigReader(response: .success(config)),
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            launchSettleWait: { await Task.yield() }
        )
        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        return (appModel, appModel.openUntitledTab())
    }

    // MARK: - Plan file

    func testAPlanFileIsHandedToTheAgentAlongsideTheDescription() async {
        let client = FolderOpeningClient()
        let (appModel, tab) = await model(client: client)

        appModel.attachPlanFile(file("plan.md", "# Plan\n- M1"))
        XCTAssertEqual(appModel.workshopPlanFile?.name, "plan.md")
        await appModel.launchWorkshop(request: " Build it small. ")

        XCTAssertEqual(client.launches.count, 1)
        let request = client.launches[0].request
        XCTAssertTrue(request.hasPrefix("Build it small.\n\n"), request)
        XCTAssertTrue(request.contains("----- BEGIN plan.md -----\n# Plan\n- M1\n----- END plan.md -----"), request)
        XCTAssertNil(appModel.workshopPlanFile, "a launch that took uses the file up")
        XCTAssertTrue(appModel.tabHasLiveWorkshop(tab))
    }

    func testAPlanFileAloneLaunchesTheWorkshop() async {
        let client = FolderOpeningClient()
        let (appModel, _) = await model(client: client)

        appModel.attachPlanFile(file("only.md", "just this"))
        await appModel.launchWorkshop(request: "")

        XCTAssertEqual(client.launches.count, 1)
        XCTAssertTrue(client.launches[0].request.hasPrefix("The plan document \"only.md\" follows."))
    }

    func testAFailedLaunchKeepsTheFileForARetry() async {
        let client = FolderOpeningClient()
        client.launchFailure = NatError.commandFailed("tmux is down")
        let (appModel, _) = await model(client: client)

        appModel.attachPlanFile(file("plan.md", "x"))
        await appModel.launchWorkshop(request: "")

        XCTAssertEqual(appModel.workshopLaunchError, "tmux is down")
        XCTAssertNotNil(appModel.workshopPlanFile)
    }

    func testDetachingTakesTheFileOff() async {
        let (appModel, _) = await model(client: FolderOpeningClient())
        appModel.attachPlanFile(file("plan.md", "x"))

        appModel.detachPlanFile()

        XCTAssertNil(appModel.workshopPlanFile)
    }

    func testAFileTooLargeOrNotTextIsRefusedAndKeepsWhatWasAttached() async {
        let (appModel, _) = await model(client: FolderOpeningClient())
        appModel.attachPlanFile(file("good.md", "fine"))

        appModel.attachPlanFile(file("big.md", String(repeating: "a", count: PlanFile.maxBytes + 1)))
        XCTAssertEqual(appModel.workshopLaunchError,
                       StarterCard.planFileTooLarge(name: "big.md", bytes: PlanFile.maxBytes + 1))
        XCTAssertEqual(appModel.workshopPlanFile?.name, "good.md")

        let binary = scratch.appendingPathComponent("blob.bin")
        try? Data([0xff, 0xfe, 0x00, 0xc3, 0x28]).write(to: binary)
        appModel.attachPlanFile(binary)
        XCTAssertEqual(appModel.workshopLaunchError, "blob.bin could not be read as text")

        appModel.attachPlanFile(scratch.appendingPathComponent("missing.md"))
        XCTAssertEqual(appModel.workshopLaunchError, "missing.md could not be read as text")
        XCTAssertEqual(appModel.workshopPlanFile?.name, "good.md")

        appModel.attachPlanFile(file("next.md", "ok"))
        XCTAssertNil(appModel.workshopLaunchError, "a file that took clears the refusal")
    }

    func testTheFileBelongsToItsUntitledTabAndIsIgnoredOnAProject() async {
        let (appModel, first) = await model(client: FolderOpeningClient())
        appModel.attachPlanFile(file("plan.md", "x"))
        let second = appModel.openUntitledTab()
        XCTAssertNil(appModel.workshopPlanFile)

        await appModel.activateProject("proj-a")
        appModel.attachPlanFile(file("other.md", "y"))
        XCTAssertNil(appModel.workshopPlanFile)

        await appModel.activateProject(first)
        XCTAssertEqual(appModel.workshopPlanFile?.name, "plan.md")
        await appModel.closeProject(second)
        await appModel.closeProject(first)
        XCTAssertNil(appModel.workshopPlanFile)
    }

    func testAttachAndDetachDoNothingWithNoTab() {
        let appModel = AppModel()
        appModel.attachPlanFile(file("plan.md", "x"))
        appModel.detachPlanFile()
        XCTAssertNil(appModel.workshopPlanFile)
    }

    // MARK: - Folder

    func testAFolderWithAPlanOpensAsTheTabsProject() async {
        let client = FolderOpeningClient()
        client.folderResult = .success(ProjectEntry(id: "local-1", name: "Found"))
        let (appModel, tab) = await model(client: client)
        let position = appModel.projectTabs.firstIndex { $0.id == tab }

        await appModel.openPlanFolder(URL(fileURLWithPath: "/plans/found"))

        XCTAssertEqual(client.folders, ["/plans/found"])
        XCTAssertNil(appModel.workshopLaunchError)
        XCTAssertFalse(appModel.projectTabs.contains { $0.id == tab }, "the Untitled tab became the project's")
        XCTAssertEqual(appModel.projectTabs.firstIndex { $0.id == "local-1" }, position)
        XCTAssertEqual(appModel.activeProjectID, "local-1")
    }

    func testAFolderWithNoPlanIsRefusedAndTheTabLeftAlone() async {
        let client = FolderOpeningClient()
        client.folderResult = .failure(NatError.commandFailed("no plan found in /x: looked for a nat plan file"))
        let (appModel, tab) = await model(client: client)
        let tabs = appModel.projectTabs.map(\.id)

        await appModel.openPlanFolder(URL(fileURLWithPath: "/x"))

        XCTAssertEqual(appModel.workshopLaunchError, "no plan found in /x: looked for a nat plan file")
        XCTAssertEqual(appModel.projectTabs.map(\.id), tabs)
        XCTAssertEqual(appModel.activeProjectID, tab)
    }

    func testAFolderThatFailsAnyOtherWayIsRefusedWithItsDescription() async {
        let client = FolderOpeningClient()
        client.folderResult = .failure(NatError.missingOutput)
        let (appModel, _) = await model(client: client)
        await appModel.openPlanFolder(URL(fileURLWithPath: "/x"))
        XCTAssertEqual(appModel.workshopLaunchError, NatError.missingOutput.localizedDescription)

        struct Boom: LocalizedError { var errorDescription: String? { "boom" } }
        client.folderResult = .failure(Boom())
        await appModel.openPlanFolder(URL(fileURLWithPath: "/x"))
        XCTAssertEqual(appModel.workshopLaunchError, "boom")
    }

    func testOnAProjectTabNothingIsOpenedIntoIt() async {
        let client = FolderOpeningClient()
        let (appModel, _) = await model(client: client)
        await appModel.activateProject("proj-a")

        await appModel.openPlanFolder(URL(fileURLWithPath: "/x"))

        XCTAssertTrue(client.folders.isEmpty)
    }

    func testTheDefaultClientRefusesWhatItCannotDo() async {
        let client = FixtureNatClient()
        do {
            _ = try await client.projectOpenFolder(path: "/x")
            XCTFail("expected a refusal")
        } catch {
            XCTAssertEqual(error.localizedDescription, "nat: project-open-folder: not supported by this client")
        }
    }
}
