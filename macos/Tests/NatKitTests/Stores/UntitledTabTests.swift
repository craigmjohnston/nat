import XCTest
import NatFixtures
@testable import NatKit

/// The Untitled tab as `AppModel` sees it: opened by the "+" and by a launch
/// with no projects, closable, never backed by a store, and replaced by the
/// project opened into it.
@MainActor
final class UntitledTabTests: XCTestCase {
    private let projectID = Fixtures.projectID

    private func started(
        config: NatProjectConfig = Fixtures.config, toolsReady: Bool = true
    ) async -> AppModel {
        await Fixtures.startedAppModel(config: config, toolsReady: toolsReady)
    }

    // MARK: - Opening

    func testOpenUntitledTabAppendsAndActivatesIt() async {
        let appModel = await started()

        let id = appModel.openUntitledTab()

        XCTAssertEqual(appModel.projectTabs.map(\.id), [projectID, id])
        XCTAssertEqual(appModel.projectTabs.last?.name, "Untitled")
        XCTAssertEqual(appModel.activeProjectID, id)
        XCTAssertTrue(appModel.isUntitledTab(id))
        XCTAssertTrue(appModel.activeTabIsUntitled)
        XCTAssertNil(appModel.projectStore, "no project, so no store")
    }

    func testMoreThanOneUntitledTabMayExistAndEachHasItsOwnID() async {
        let appModel = await started()
        let first = appModel.openUntitledTab()
        let second = appModel.openUntitledTab()

        XCTAssertNotEqual(first, second)
        XCTAssertEqual(appModel.closableTabCount, 3)
        XCTAssertFalse(appModel.isUntitledTab(projectID))
    }

    func testActivatingAProjectLeavesTheUntitledTabBehind() async {
        let appModel = await started()
        let untitled = appModel.openUntitledTab()

        await appModel.activateProject(projectID)
        XCTAssertFalse(appModel.activeTabIsUntitled)

        await appModel.activateProject(untitled)
        XCTAssertEqual(appModel.activeProjectID, untitled)
        XCTAssertNil(appModel.projectStore)
    }

    // MARK: - Closing

    func testAnUntitledTabClosesAndItsNeighbourTakesOver() async {
        let appModel = await started()
        let untitled = appModel.openUntitledTab()

        await appModel.closeProject(untitled)

        XCTAssertEqual(appModel.projectTabs.map(\.id), [projectID])
        XCTAssertEqual(appModel.activeProjectID, projectID)
    }

    func testTheLoneUntitledTabIsNotClosable() async {
        let appModel = await started(config: Fixtures.emptyConfig)
        let only = appModel.activeProjectID!

        await appModel.closeProject(only)

        XCTAssertEqual(appModel.projectTabs.map(\.id), [only])
    }

    // MARK: - Launch

    func testALaunchWithNoProjectsOpensOneUntitledTab() async {
        let appModel = await started(config: Fixtures.emptyConfig)

        XCTAssertFalse(appModel.needsOnboarding)
        XCTAssertEqual(appModel.projectTabs.count, 1)
        XCTAssertTrue(appModel.activeTabIsUntitled)
        XCTAssertNotNil(appModel.activityStore)
        XCTAssertNotNil(appModel.usageStore)
    }

    func testStartingAgainDoesNotOpenAnotherUntitledTab() async {
        let appModel = await started(config: Fixtures.emptyConfig)

        await Fixtures.start(appModel)

        XCTAssertEqual(appModel.projectTabs.count, 1)
    }

    func testMissingToolsLeaveTheOnboardingChecklist() async {
        let appModel = await started(config: Fixtures.emptyConfig, toolsReady: false)

        XCTAssertTrue(appModel.needsOnboarding)
        XCTAssertTrue(appModel.projectTabs.isEmpty)
    }

    // MARK: - Opening a project into the tab

    func testAProjectOpenedFromAnUntitledTabTakesItsPlace() async {
        let appModel = await started(config: Fixtures.scratchConfigWithSecondProject)
        let untitled = appModel.openUntitledTab()
        let before = appModel.projectTabs.map(\.id)
        XCTAssertEqual(before.last, untitled)

        await appModel.addProject(id: Fixtures.secondProjectID, name: "x", replacing: untitled)

        XCTAssertFalse(appModel.projectTabs.contains { $0.id == untitled })
        XCTAssertEqual(appModel.activeProjectID, Fixtures.secondProjectID)
        XCTAssertEqual(appModel.projectTabs.filter { $0.id == Fixtures.secondProjectID }.count, 1)
    }

    func testTheReplacementKeepsThePlaceTheUntitledTabHeld() async {
        let appModel = await started(config: Fixtures.emptyConfig)
        let untitled = appModel.activeProjectID!
        let second = appModel.openUntitledTab()
        XCTAssertEqual(appModel.projectTabs.map(\.id), [untitled, second])

        // The config reader hands the fixture config back, which names the
        // fixture project; it is the one "opened".
        await appModel.addProject(id: "ignored", name: "Opened", replacing: untitled)

        XCTAssertEqual(appModel.projectTabs.map(\.id), ["ignored", second])
    }

    func testAProjectAlreadyOnTheStripJustClosesTheUntitledTab() async {
        let appModel = await started()
        let untitled = appModel.openUntitledTab()

        await appModel.addProject(id: projectID, name: "Fixture", replacing: untitled)

        XCTAssertEqual(appModel.projectTabs.map(\.id), [projectID])
        XCTAssertEqual(appModel.activeProjectID, projectID)
    }

    func testReplacingANonUntitledTabIDIsIgnored() async {
        let appModel = await started(config: Fixtures.scratchConfigWithSecondProject)

        await appModel.addProject(id: "new", name: "New", replacing: projectID)

        XCTAssertTrue(appModel.projectTabs.contains { $0.id == projectID })
    }

    // MARK: - Copy

    func testTheStagedControlsNameTheirSlices() {
        XCTAssertEqual(StarterCard.workshopStaging, "Launch the planning agent from the starter card")
        XCTAssertEqual(StarterCard.openPlanStaging, "Open existing plans from the starter")
        XCTAssertEqual(StarterCard.filesystemStaging, "Open existing plans from the starter")
        XCTAssertEqual(StarterCard.railExplainer,
                       "Milestones and slices appear here once the project has a plan.")
    }
}
