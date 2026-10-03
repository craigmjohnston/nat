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
        XCTAssertEqual(appModel.titlebarIdentity(for: .slice(id: Fixtures.sourceTodoTaskID, name: "x", state: .todo)).tag, "DM")
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

    func testARefreshReadsTheContainerOnScreenAgain() async {
        let appModel = await started()
        await appModel.selectContainer(Fixtures.sourceCardID, inProject: Fixtures.sourceProjectID)
        await appModel.refresh()
        XCTAssertEqual(
            appModel.containerStore(projectID: Fixtures.sourceProjectID).state(for: Fixtures.sourceCardID).show?.container.id,
            Fixtures.sourceCardID)
    }
}
