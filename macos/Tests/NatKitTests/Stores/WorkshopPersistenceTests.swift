import XCTest
@testable import NatKit
@testable import NatFixtures

/// An unlaunched workshop, and the Untitled tabs, kept across relaunches
/// through `WorkshopCaching` — every model here on an in-memory cache, never
/// the real Application Support file.
@MainActor
final class WorkshopPersistenceTests: XCTestCase {
    private var projectID: String { Fixtures.projectID }

    /// A second launch over what the first kept.
    private func relaunch(
        _ cache: WorkshopCaching, client: FixtureNatClient = FixtureNatClient(agents: []),
        config: NatProjectConfig = Fixtures.config, toolsReady: Bool = false
    ) async -> AppModel {
        await Fixtures.startedAppModel(client: client, config: config, toolsReady: toolsReady, workshopCache: cache)
    }

    private func kept(_ cache: InMemoryWorkshopCache) -> WorkshopSnapshot {
        cache.read() ?? WorkshopSnapshot()
    }

    private func planFileURL(_ name: String = "kept-plan.md") throws -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(name)
        try "# Plan\n".write(to: url, atomically: true, encoding: .utf8)
        return url
    }

    /// Reads the activity poll until the model sees a planning agent.
    private func poll(_ model: AppModel) async {
        model.activityStore?.kick()
        for _ in 0..<200 where model.planningAgent == nil {
            try? await Task.sleep(nanoseconds: 2_000_000)
        }
    }

    // MARK: - Save and restore

    func testAProjectsTypedBriefIsBackPinnedAfterARelaunch() async {
        let cache = InMemoryWorkshopCache()
        let first = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []), workshopCache: cache)
        first.openWorkshop()
        first.workshopDraft = "Tidy the review flow."
        first.selectedSliceID = Fixtures.mergeBoxSliceID
        first.flushWorkshops()

        let second = await relaunch(cache)

        XCTAssertTrue(second.isWorkshopPinned(projectID))
        XCTAssertEqual(second.sidebarModel.active.filter { $0.kind == .workshop }.map(\.projectID), [projectID])
        await second.selectWorkshop(inProject: projectID)
        XCTAssertEqual(second.workshopDraft, "Tidy the review flow.")
    }

    func testAnUntitledTabComesBackWithItsWorkspaceDraftAndPlanFile() async throws {
        let cache = InMemoryWorkshopCache()
        let first = await Fixtures.startedAppModel(workshopCache: cache)
        _ = first.openUntitledTab()
        let tab = first.openUntitledTab()
        first.workshopDraft = "A habit tracker."
        first.attachPlanFile(try planFileURL())
        first.flushWorkshops()
        let workspace = first.workspaceID(forTab: tab)

        let second = await relaunch(cache)

        XCTAssertEqual(second.projectTabs.map(\.id).suffix(2), ["untitled-1", tab])
        XCTAssertEqual(second.projectTabs.last?.name, AppModel.untitledName)
        XCTAssertEqual(second.workspaceID(forTab: tab), workspace)
        await second.activateProject(tab)
        XCTAssertEqual(second.workshopDraft, "A habit tracker.")
        XCTAssertEqual(second.workshopPlanFile?.name, "kept-plan.md")
        XCTAssertEqual(second.openUntitledTab(), "untitled-3", "numbering restarts past the highest restored")
    }

    func testARestoredUntitledTabIsWhereANoProjectLaunchStarts() async {
        let cache = InMemoryWorkshopCache(WorkshopSnapshot(
            untitledTabs: [.init(id: "untitled-4", workspaceID: "ws-4")],
            workshops: ["untitled-4": .init(draft: "Kept.")]))

        let model = await relaunch(cache, config: Fixtures.emptyConfig, toolsReady: true)

        XCTAssertEqual(model.projectTabs.map(\.id), ["untitled-4"], "no second tab is opened beside it")
        XCTAssertEqual(model.activeProjectID, "untitled-4")
        XCTAssertEqual(model.workshopDraft, "Kept.")
    }

    func testARunningProjectWorkshopIsShownAgainWithItsBrief() async {
        let cache = InMemoryWorkshopCache(WorkshopSnapshot(workshops: [projectID: .init(request: "Split the importer.")]))
        let planner = AgentStatus(
            sliceID: TmuxSession.planTag(projectID: projectID),
            session: TmuxSession.planSessionName(projectID: projectID), activity: .working)

        let model = await relaunch(cache, client: FixtureNatClient(agents: [planner]))
        await model.selectWorkshop(inProject: projectID)
        await poll(model)

        XCTAssertNotNil(model.planningAgent)
        XCTAssertEqual(model.workshopRequest, "Split the importer.")
    }

    func testARunningUntitledWorkshopIsFoundAgainUnderItsWorkspace() async {
        let cache = InMemoryWorkshopCache(WorkshopSnapshot(
            untitledTabs: [.init(id: "untitled-2", workspaceID: "ws-kept")],
            workshops: ["untitled-2": .init(request: "A habit tracker.")]))
        let planner = AgentStatus(
            sliceID: TmuxSession.planTag(projectID: "ws-kept"),
            session: TmuxSession.planSessionName(projectID: "ws-kept"), activity: .working)

        let model = await relaunch(cache, client: FixtureNatClient(agents: [planner]))
        await model.activateProject("untitled-2")
        await poll(model)

        XCTAssertNotNil(model.planningAgent)
        XCTAssertTrue(model.tabHasLiveWorkshop("untitled-2"))
        XCTAssertEqual(model.workshopRequest, "A habit tracker.")
    }

    func testAProjectNoLongerInConfigIsDroppedAtRestore() async {
        let cache = InMemoryWorkshopCache(WorkshopSnapshot(
            untitledTabs: [.init(id: "not-untitled", workspaceID: "ws")],
            workshops: [
                "gone-project": .init(pinned: true, draft: "Lost."),
                "untitled-9": .init(draft: "No tab."),
                projectID: .init(pinned: true),
            ]))

        let model = await relaunch(cache)

        XCTAssertFalse(model.isWorkshopPinned("gone-project"))
        XCTAssertFalse(model.projectTabs.contains { $0.id == "not-untitled" || $0.id == "untitled-9" })
        XCTAssertTrue(model.isWorkshopPinned(projectID))
        model.flushWorkshops()
        XCTAssertEqual(Set(kept(cache).workshops.keys), [projectID])
    }

    func testAMissingFileRestoresNothing() async {
        let model = await relaunch(InMemoryWorkshopCache())

        XCTAssertTrue(model.workshopPinnedProjects.isEmpty)
        XCTAssertFalse(model.projectTabs.contains { model.isUntitledTab($0.id) })
    }

    // MARK: - When it is written

    func testNothingIsWrittenBeforeTheCacheHasBeenRead() {
        let cache = InMemoryWorkshopCache(WorkshopSnapshot(workshops: ["x": .init(draft: "Kept.")]))
        let model = Fixtures.appModel(workshopCache: cache)

        model.openUntitledTab()
        model.workshopDraft = "Typed before start."
        model.flushWorkshops()

        XCTAssertEqual(cache.writes, 0)
        XCTAssertEqual(kept(cache).workshops["x"]?.draft, "Kept.")
    }

    func testTypingIsWrittenOnceItPauses() async {
        let cache = InMemoryWorkshopCache()
        let client = FixtureNatClient(agents: [])
        let model = AppModel(
            configReader: FixtureConfigReader(config: Fixtures.emptyConfig),
            planCache: NullPlanCache(),
            pathsProvider: { Fixtures.paths },
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            usageStoreFactory: { UsageStore(client: client, cache: NullUsageCache()) },
            toolsReady: { true },
            workshopCache: cache,
            workshopSaveWait: { try? await Task.sleep(nanoseconds: 50_000_000) }
        )
        await Fixtures.start(model)
        for _ in 0..<200 where cache.writes == 0 { try? await Task.sleep(nanoseconds: 5_000_000) }
        let before = cache.writes

        model.workshopDraft = "A"
        model.workshopDraft = "A h"
        model.workshopDraft = "A habit"
        for _ in 0..<200 where cache.writes == before { try? await Task.sleep(nanoseconds: 5_000_000) }

        XCTAssertEqual(cache.writes, before + 1, "three keystrokes, one write")
        XCTAssertEqual(kept(cache).workshops[model.activeProjectID ?? ""]?.draft, "A habit")
    }

    func testAChangeIsWrittenWithoutAFlush() async {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []), workshopCache: cache)

        model.openWorkshop()
        for _ in 0..<200 where kept(cache).workshops[projectID] == nil { await Task.yield() }

        XCTAssertEqual(kept(cache).workshops[projectID]?.pinned, true)
    }

    // MARK: - What clears it

    func testTheRowsDismissClearsWhatWasKept() async {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []), workshopCache: cache)
        model.openWorkshop()
        model.workshopDraft = "Gone."

        model.dismissWorkshop(inProject: projectID)
        model.flushWorkshops()

        XCTAssertNil(kept(cache).workshops[projectID])
        let again = await relaunch(cache)
        XCTAssertFalse(again.isWorkshopPinned(projectID), "the ✕ is for good")
    }

    func testClosingTheWorkshopClearsEverythingForTheTab() async throws {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true, workshopCache: cache)
        let tab = model.activeProjectID ?? ""
        model.openWorkshop()
        model.workshopDraft = "Gone."
        model.attachPlanFile(try planFileURL())

        let refusal = await model.closeWorkshopTab()
        model.flushWorkshops()

        XCTAssertNil(refusal)
        XCTAssertNil(kept(cache).workshops[tab])
        XCTAssertNil(model.workshopPlanFile)
    }

    func testALaunchThatTakesKeepsTheRequestTheDraftAndTheFile() async throws {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(config: Fixtures.emptyConfig, toolsReady: true, workshopCache: cache)
        let tab = model.activeProjectID ?? ""
        model.workshopDraft = "Build it."
        model.attachPlanFile(try planFileURL())

        await model.launchWorkshop(request: model.workshopDraft)
        model.flushWorkshops()

        XCTAssertEqual(kept(cache).workshops[tab], .init(
            draft: "Build it.", planFile: PlanFile(name: "kept-plan.md", content: "# Plan\n"),
            request: "Build it.\n\nAttached: kept-plan.md"))
    }

    func testAFailedLaunchKeepsTheDraft() async {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(
            client: FixtureNatClient(behaviour: .refusing("no tmux")), workshopCache: cache)
        model.openWorkshop()
        model.workshopDraft = "Split the importer."

        await model.launchWorkshop(request: model.workshopDraft)
        model.flushWorkshops()

        XCTAssertEqual(kept(cache).workshops[projectID], .init(pinned: true, draft: "Split the importer."))
    }

    func testClosingAnUntitledTabClearsIt() async {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(workshopCache: cache)
        let tab = model.openUntitledTab()
        model.workshopDraft = "Gone."

        await model.closeProject(tab)
        model.flushWorkshops()

        XCTAssertTrue(kept(cache).untitledTabs.isEmpty)
        XCTAssertNil(kept(cache).workshops[tab])
    }

    func testAnUntitledTabTurnedIntoAProjectClearsIt() async {
        let cache = InMemoryWorkshopCache()
        let model = await Fixtures.startedAppModel(
            config: Fixtures.scratchConfigWithSecondProject, workshopCache: cache)
        let tab = model.openUntitledTab()
        model.workshopDraft = "Gone."

        await model.addProject(id: Fixtures.secondProjectID, name: "x", replacing: tab)
        model.flushWorkshops()

        XCTAssertTrue(kept(cache).untitledTabs.isEmpty)
        XCTAssertNil(kept(cache).workshops[tab])
    }

    // MARK: - The file

    private func diskCache() -> DiskWorkshopCache {
        DiskWorkshopCache(fileURL: FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
            .appendingPathComponent("workshops.json"))
    }

    func testTheFileRoundTrips() {
        let cache = diskCache()
        let snapshot = WorkshopSnapshot(
            untitledTabs: [.init(id: "untitled-1", workspaceID: "ws")],
            workshops: ["untitled-1": .init(
                pinned: true, draft: "Draft", planFile: PlanFile(name: "p.md", content: "# P"), request: "Sent")])

        cache.write(snapshot)

        XCTAssertEqual(cache.read(), snapshot)
    }

    func testAMissingUnreadableOrOlderFileReadsAsNothing() throws {
        let cache = diskCache()
        XCTAssertNil(cache.read(), "missing")

        try FileManager.default.createDirectory(
            at: cache.fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("not json".utf8).write(to: cache.fileURL)
        XCTAssertNil(cache.read(), "unreadable")

        try Data(#"{"version":0,"untitledTabs":[],"workshops":{}}"#.utf8).write(to: cache.fileURL)
        XCTAssertNil(cache.read(), "another build's shape")
    }

    func testTheDefaultFileIsInApplicationSupport() {
        let url = DiskWorkshopCache().fileURL
        XCTAssertEqual(url.lastPathComponent, "workshops.json")
        XCTAssertEqual(url.deletingLastPathComponent().lastPathComponent, DiskPlanCache.bundleID)
    }

    // MARK: - The row

    func testTheWorkshopRowAndCrumbCarryTheWand() {
        let input = SidebarProjectInput(id: "p", name: "Plan", plan: nil)
        let model = buildSidebarModel(projects: [input], liveAgents: [:], pinnedWorkshops: ["p"])
        XCTAssertEqual(model.active.map(\.title), ["Workshop"])
        XCTAssertEqual(model.active.map(\.symbol), ["wand.and.stars"])

        let identity = titlebarIdentity(for: .workshop, projectID: "p", active: model.active, tags: [:])
        XCTAssertEqual(identity.symbol, "wand.and.stars")
        XCTAssertEqual(identity.lastCrumb(afterProjectCrumb: true).symbol, "wand.and.stars")
        XCTAssertEqual(
            titlebarIdentity(for: .workshop, projectID: "q", active: [], tags: [:]).symbol, "wand.and.stars")
        XCTAssertNil(titlebarIdentity(
            for: .slice(id: "s", name: "S", state: .todo), projectID: "p", active: [], tags: [:]).symbol)
        XCTAssertEqual(StarterCard.workshopLabel, "Workshop")
    }
}
