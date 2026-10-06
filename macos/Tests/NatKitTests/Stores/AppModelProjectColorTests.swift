import XCTest
@testable import NatKit
@testable import NatFixtures

/// gnat never picks a colour: once config is read, each project with none
/// is given one by nat (`config-set project.<id>.color auto`), once a run.
@MainActor
final class AppModelProjectColorTests: XCTestCase {
    /// `twoProjectConfig` with the second project's colour taken off.
    private var oneUncoloured: NatProjectConfig {
        var projects = Fixtures.twoProjectConfig.projects
        let second = projects[Fixtures.secondProjectID]!
        projects[Fixtures.secondProjectID] = ProjectConfig(
            name: second.name, slicesDSID: second.slicesDSID, workingDir: second.workingDir)
        return NatProjectConfig(projects: projects, pollSeconds: 3600)
    }

    /// An app over the fixture client that asks for colours, as only
    /// `NatApp`'s does.
    private func model(client: FixtureNatClient, config: NatProjectConfig, assigns: Bool = true) -> AppModel {
        AppModel(
            configReader: FixtureConfigReader(config: config),
            planCache: NullPlanCache(),
            pollIntervalSeconds: 3600,
            pathsProvider: { Fixtures.paths },
            clientFactory: { client },
            activityStoreFactory: { ActivityStore(client: client) },
            usageStoreFactory: { UsageStore(client: client, cache: NullUsageCache()) },
            toolsReady: { false },
            assignsProjectColors: assigns)
    }

    private func colourWrites(_ client: FixtureNatClient) -> [String] {
        client.configSetValues.filter { $0.contains(".color=") }
    }

    func testStartingAsksForAColourForEachProjectWithNone() async {
        let client = FixtureNatClient()
        let model = model(client: client, config: oneUncoloured)
        await model.start(configPath: Fixtures.paths.config, nudgePath: Fixtures.paths.nudge)

        XCTAssertEqual(colourWrites(client), ["project.\(Fixtures.secondProjectID).color=auto"])
    }

    func testStartingAsksNothingWhereEveryProjectHasAColour() async {
        let client = FixtureNatClient()
        let model = model(client: client, config: Fixtures.twoProjectConfig)
        await model.start(configPath: Fixtures.paths.config, nudgePath: Fixtures.paths.nudge)

        XCTAssertEqual(colourWrites(client), [])
        XCTAssertEqual(model.projectColor(ofProject: Fixtures.projectID), .teal)
        XCTAssertEqual(model.projectColor(ofProject: Fixtures.secondProjectID), .orange)
        XCTAssertEqual(model.sidebarInputs.map(\.color), [.teal, .orange])
    }

    /// Asked once a run: a project still without one after its ask — nat
    /// refused, or config not yet showing it — is not asked again until
    /// the next launch, since a second `auto` would choose afresh.
    func testAProjectIsAskedOnceARun() async {
        let client = FixtureNatClient()
        let model = model(client: client, config: oneUncoloured)
        await model.start(configPath: Fixtures.paths.config, nudgePath: Fixtures.paths.nudge)
        await model.addProject(id: Fixtures.secondProjectID, name: "gnat")

        XCTAssertEqual(colourWrites(client).count, 1)
    }

    /// A refusal is logged and nothing else: the project draws no puck.
    func testARefusalLeavesTheProjectWithNoColour() async {
        let model = model(client: FixtureNatClient(behaviour: .refusing("no")), config: oneUncoloured)
        await model.start(configPath: Fixtures.paths.config, nudgePath: Fixtures.paths.nudge)

        XCTAssertNil(model.projectColor(ofProject: Fixtures.secondProjectID))
    }

    /// Off in every test and fixture, which drive the machine's real `nat`.
    func testNothingIsAskedUnlessTheAppAsks() async {
        let client = FixtureNatClient()
        let model = model(client: client, config: oneUncoloured, assigns: false)
        await model.start(configPath: Fixtures.paths.config, nudgePath: Fixtures.paths.nudge)

        XCTAssertEqual(colourWrites(client), [])
    }
}
