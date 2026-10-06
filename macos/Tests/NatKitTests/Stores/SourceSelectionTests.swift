import XCTest
@testable import NatKit
@testable import NatFixtures

/// A source container as a selection, its detail cache, its lazy groups and
/// its plugin's actions, over the fixture Work project.
@MainActor
final class SourceSelectionTests: XCTestCase {
    private func started(_ client: FixtureNatClient = FixtureNatClient()) async -> AppModel {
        await Fixtures.startedAppModel(client: client, config: Fixtures.sourceConfig)
    }

    func testAContainerIsASelectionExclusiveWithEveryOther() async {
        let appModel = await started()
        let work = Fixtures.sourceProjectID
        await appModel.selectContainer(Fixtures.sourceCardID, inProject: work)
        XCTAssertEqual(appModel.activeProjectID, work)
        XCTAssertEqual(appModel.selectedContainerID, Fixtures.sourceCardID)
        XCTAssertEqual(appModel.selectedContainer?.title, "Improve diff review ergonomics")

        appModel.selectedSliceID = Fixtures.sourceTodoTaskID
        XCTAssertNil(appModel.selectedContainerID)

        appModel.selectedContainerID = Fixtures.sourceCardID
        XCTAssertNil(appModel.selectedSliceID)
        appModel.selectedSessionID = "s1"
        XCTAssertNil(appModel.selectedContainerID)

        appModel.selectedContainerID = Fixtures.sourceCardID
        XCTAssertNil(appModel.selectedSessionID)
        appModel.workshopSelected = true
        XCTAssertNil(appModel.selectedContainerID)

        appModel.selectedContainerID = Fixtures.sourceCardID
        XCTAssertFalse(appModel.workshopSelected)
        XCTAssertNil(appModel.acceptedPlanShown)
    }

    func testAContainerIsNamedByThePluginElseTheCacheElseItsID() async {
        let appModel = await started()
        let work = Fixtures.sourceProjectID
        await appModel.activateProject(work)
        XCTAssertEqual(appModel.containerTitle(Fixtures.sourceCardID, inProject: work), "Improve diff review ergonomics")
        XCTAssertEqual(appModel.containerTitle("nope", inProject: work), "nope")
        XCTAssertNil(appModel.selectedContainer)
        XCTAssertNil(appModel.source(ofProject: Fixtures.projectID))
        XCTAssertEqual(appModel.source(ofProject: work)?.tag, "DM")
        let task = appModel.titlebarIdentity(for: .slice(id: Fixtures.sourceTodoTaskID, name: "x", state: .todo))
        XCTAssertEqual(task.tag, "", "a source project takes no tag")
        XCTAssertEqual(task.cardBadge, Fixtures.sourceMobileApp, "its card's badge instead")
        XCTAssertEqual(task.cardIcon, Fixtures.sourceInfo().icon)
    }

    func testTheContainerStoreKeepsItsStaleReadingOnAFailedRead() async {
        let store = ContainerStore(projectID: Fixtures.sourceProjectID, client: FixtureNatClient())
        XCTAssertEqual(store.state(for: Fixtures.sourceCardID), .idle)
        await store.fetch(containerID: Fixtures.sourceCardID)
        let show = Fixtures.sourceContainerShow(id: Fixtures.sourceCardID)
        XCTAssertEqual(store.state(for: Fixtures.sourceCardID), .loaded(show))

        await store.fetch(containerID: Fixtures.sourceMineCardID)
        store.invalidateCache(keeping: Fixtures.sourceCardID)
        XCTAssertEqual(store.state(for: Fixtures.sourceMineCardID), .idle)
        XCTAssertEqual(store.state(for: Fixtures.sourceCardID).show, show)
        store.invalidateCache(keeping: nil)
        XCTAssertEqual(store.state(for: Fixtures.sourceCardID), .idle)

        let failing = ContainerStore(projectID: "p", client: FixtureNatClient(behaviour: .refusing("plugin down")))
        await failing.fetch(containerID: "c")
        XCTAssertEqual(failing.state(for: "c").errorMessage, "nat: plugin down")
        XCTAssertNil(failing.state(for: "c").show)
        XCTAssertNil(ContainerLoadState.loading(stale: show).errorMessage)
        XCTAssertEqual(ContainerLoadState.failed("x", previous: show).show, show)
    }

    func testOpeningALazyGroupReadsThePlanWithItExpanded() async {
        let appModel = await started()
        let work = Fixtures.sourceProjectID
        XCTAssertFalse(appModel.isSourceGroupExpanded("done", inProject: work))
        XCTAssertEqual(appModel.source(ofProject: work)?.groups.last?.containers, [])

        await appModel.setSourceGroup("done", expanded: true, inProject: work)
        XCTAssertTrue(appModel.isSourceGroupExpanded("done", inProject: work))
        XCTAssertEqual(appModel.source(ofProject: work)?.groups.last?.containers.map(\.id), [Fixtures.sourceDoneCardID])

        await appModel.setSourceGroup("done", expanded: false, inProject: work)
        XCTAssertEqual(appModel.source(ofProject: work)?.groups.last?.containers, [])

        // A project with no store yet only remembers it.
        await appModel.setSourceGroup("x", expanded: true, inProject: "unknown")
        XCTAssertTrue(appModel.isSourceGroupExpanded("x", inProject: "unknown"))
    }

    func testASourceActionRunsThenReadsAgainAndAnswersARefusal() async {
        let client = FixtureNatClient()
        let appModel = await started(client)
        let work = Fixtures.sourceProjectID
        let refresh = SourceAction(id: "refresh", label: "Refresh")

        // Work opens as the active project; a project in the background
        // takes an action too, then a container in the active one.
        let background = await appModel.runSourceAction(projectID: Fixtures.projectID, action: refresh)
        XCTAssertNil(background)
        await appModel.selectContainer(Fixtures.sourceCardID, inProject: work)
        let onCard = await appModel.runSourceAction(
            projectID: work, action: SourceAction(id: "comment", label: "Comment", input: .text),
            container: Fixtures.sourceCardID, input: "Looks good")
        XCTAssertNil(onCard)
        XCTAssertEqual(client.writes.filter { $0.hasPrefix("source-action") }, [
            "source-action refresh", "source-action comment --container \(Fixtures.sourceCardID)",
        ])
        XCTAssertNotNil(appModel.containerStore(projectID: work).state(for: Fixtures.sourceCardID).show)

        let refusing = await Fixtures.startedAppModel(
            client: FixtureNatClient(behaviour: .refusing("no token")), config: Fixtures.sourceConfig)
        let refused = await refusing.runSourceAction(projectID: work, action: refresh)
        XCTAssertEqual(refused, "no token")
    }

    func testThePluginsAreReadOnceAndAFailureLeavesNone() async {
        let appModel = await started()
        await appModel.loadSourcePlugins()
        XCTAssertEqual(appModel.sourcePlugins, Fixtures.sourcePlugins)
        await appModel.loadSourcePlugins()
        XCTAssertEqual(appModel.sourcePlugins.count, 2)

        let refusing = Fixtures.appModel(client: FixtureNatClient(behaviour: .refusing("no nat")))
        await refusing.loadSourcePlugins()
        XCTAssertEqual(refusing.sourcePlugins, [])
    }

    /// A connected plugin, as the Shortcut one is once its token is set.
    private func connected(_ name: String, set: Bool? = true) -> SourcePlugin {
        SourcePlugin(name: name, path: "/p/\(name)", describe: SourceDescribe(
            name: name, title: name == "shortcut" ? "Shortcut" : name, tag: "SC", iconSymbol: "s",
            containerNoun: "card", taskNoun: "task",
            setup: [PluginSetupField(id: "token", label: "API token", input: "secret", set: set)]))
    }

    func testConnectingAPluginMakesItsSectionOnce() async {
        let client = FixtureNatClient(sources: [
            connected("shortcut"), connected("jira", set: false), connected("demo"),
            SourcePlugin(name: "broken", path: "/p/broken", error: "no"),
        ])
        // sourceConfig already has a demo project; Shortcut has none.
        let appModel = await started(client)
        await appModel.loadSourcePlugins()
        XCTAssertEqual(client.writes.filter { $0.hasPrefix("project-create") }, [],
                       "only the app's own start makes one, never a start a test drives")
        await appModel.ensureSourceProjects()
        XCTAssertEqual(client.writes.filter { $0.hasPrefix("project-create") }, ["project-create Shortcut --source shortcut"])
        let made = "f1x8500c-0000-4000-8000-shortcut"
        XCTAssertTrue(appModel.projectTabs.contains { $0.id == made && $0.name == "Shortcut" })
        XCTAssertNotEqual(appModel.activeProjectID, made, "taken in, not opened")

        // Read again — the config, here, still not naming it — nothing more.
        await appModel.reloadSourcePlugins()
        await appModel.ensureSourceProjects()
        XCTAssertEqual(client.writes.filter { $0.hasPrefix("project-create") }.count, 1)
        XCTAssertEqual(appModel.projectTabs.filter { $0.id == made }.count, 1)
    }

    func testNothingIsMadeBeforeConfigOrOnARefusal() async {
        let early = Fixtures.appModel(client: FixtureNatClient(sources: [connected("shortcut")]))
        await early.loadSourcePlugins()
        await early.ensureSourceProjects()
        XCTAssertFalse(early.projectTabs.contains { $0.name == "Shortcut" }, "no config read yet")

        let refusing = FixtureNatClient(behaviour: .refusing("nope"), sources: [connected("shortcut")])
        let appModel = Fixtures.appModel(client: refusing, config: Fixtures.sourceConfig)
        await Fixtures.start(appModel)
        await appModel.ensureSourceProjects()
        XCTAssertFalse(appModel.projectTabs.contains { $0.name == "Shortcut" })
    }

    func testTheFilterActionIsReadFromTheTreeAsItIsNow() async {
        let appModel = await started()
        let source = appModel.source(ofProject: Fixtures.sourceProjectID)
        XCTAssertEqual(source?.filterAction(group: nil)?.fields.first { $0.id == "project" }?.value, ["30"])
        XCTAssertEqual(source?.filterAction(group: "ready/mine")?.fields.first { $0.id == "team" }?.value, ["board"])
        XCTAssertNil(source?.filterAction(group: "doing"))
        XCTAssertNil(source?.filterAction(group: "nope"))
        await appModel.rereadSource(projectID: Fixtures.sourceProjectID)
        await appModel.rereadSource(projectID: Fixtures.secondProjectID)
    }

    func testARefreshReadsTheContainerOnScreenAgain() async {
        let appModel = await started()
        await appModel.selectContainer(Fixtures.sourceCardID, inProject: Fixtures.sourceProjectID)
        await appModel.refresh()
        XCTAssertEqual(
            appModel.containerStore(projectID: Fixtures.sourceProjectID).state(for: Fixtures.sourceCardID).show?.container.id,
            Fixtures.sourceCardID)
    }
}
