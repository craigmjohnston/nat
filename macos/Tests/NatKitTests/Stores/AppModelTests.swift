import XCTest
@testable import NatKit

// MARK: - Mock Config Reader

final class MockConfigReader: ConfigReaderProtocol, @unchecked Sendable {
    enum Response: Sendable {
        case success(NatProjectConfig)
        case failure
    }

    private var response: Response
    /// The path most recently asked for, so `reloadConfig()`'s own re-read
    /// can be checked against `start()`'s.
    private(set) var lastPath: String?

    init(response: Response) {
        self.response = response
    }

    /// Swaps the response a later `readConfig` returns — what
    /// `reloadConfig()`'s tests use to answer differently the second time.
    func setResponse(_ response: Response) {
        self.response = response
    }

    func readConfig(from path: String) async throws -> NatProjectConfig {
        lastPath = path
        switch response {
        case .success(let config):
            return config
        case .failure:
            throw NSError(domain: "test", code: -1, userInfo: nil)
        }
    }
}

// MARK: - Tests

final class AppModelTests: XCTestCase {
    @MainActor
    func testAppModel_initialState() {
        let appModel = AppModel()

        XCTAssertNil(appModel.config)
        XCTAssertNil(appModel.projectStore)
        XCTAssertNil(appModel.selectedSliceID)
    }

    @MainActor
    func testAppModel_loadConfigSuccess() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")
            ],
            agentSplitPercent: nil,
            pollSeconds: nil
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertNotNil(appModel.config)
        XCTAssertEqual(appModel.config?.projects.count, 1)
        XCTAssertNotNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_loadConfigFailure() async {
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertNil(appModel.config)
        XCTAssertNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_selectsFirstProject() async {
        let testConfig = NatProjectConfig(
            projects: [
                "z-proj": ProjectConfig(name: "Z Project", slicesDSID: "ds-z", workingDir: "/path/z"),
                "a-proj": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Should pick "a-proj" (first when sorted alphabetically)
        XCTAssertEqual(appModel.projectStore?.projectID, "a-proj")
    }

    @MainActor
    func testAppModel_refreshPublic() async {
        _ = Project(id: "proj-1", name: "Test", conventions: "")
        let testConfig = NatProjectConfig(
            projects: [
                "proj-1": ProjectConfig(name: "Project", slicesDSID: "ds-1", workingDir: "/path")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // refresh should not throw
        await appModel.refresh()

        XCTAssertNotNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_refreshWithoutProjectStore() async {
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // refresh should not crash
        await appModel.refresh()

        XCTAssertNil(appModel.projectStore)
    }

    // MARK: - Multi-Project Tests

    @MainActor
    func testAppModel_multipleProjects() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-z": ProjectConfig(name: "Z Project", slicesDSID: "ds-z", workingDir: "/path/z"),
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-m": ProjectConfig(name: "M Project", slicesDSID: "ds-m", workingDir: "/path/m")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Should create tabs sorted by project ID
        XCTAssertEqual(appModel.projectTabs.count, 3)
        XCTAssertEqual(appModel.projectTabs[0].id, "proj-a")
        XCTAssertEqual(appModel.projectTabs[0].name, "A Project")
        XCTAssertEqual(appModel.projectTabs[1].id, "proj-m")
        XCTAssertEqual(appModel.projectTabs[2].id, "proj-z")
    }

    @MainActor
    func testAppModel_activatesFirstProject() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-z": ProjectConfig(name: "Z Project", slicesDSID: "ds-z", workingDir: "/path/z"),
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Should activate the first project alphabetically
        XCTAssertEqual(appModel.activeProjectID, "proj-a")
        XCTAssertNotNil(appModel.projectStore)
        XCTAssertEqual(appModel.projectStore?.projectID, "proj-a")
    }

    @MainActor
    func testAppModel_perProjectSelectedSliceID() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // Set a selected slice for the active project
        appModel.selectedSliceID = "slice-1"
        XCTAssertEqual(appModel.selectedSliceID, "slice-1")

        // Switch to another project
        await appModel.activateProject("proj-b")
        XCTAssertEqual(appModel.activeProjectID, "proj-b")
        // Selected slice should be nil for the new project
        XCTAssertNil(appModel.selectedSliceID)

        // Switch back to the first project
        await appModel.activateProject("proj-a")
        XCTAssertEqual(appModel.activeProjectID, "proj-a")
        // Should restore the previously selected slice
        XCTAssertEqual(appModel.selectedSliceID, "slice-1")
    }

    @MainActor
    func testAppModel_liveCountCalculation() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // LiveCount should be 0 when no agents are running
        XCTAssertEqual(appModel.liveCount(projectID: "proj-a"), 0)
    }

    @MainActor
    func testAppModel_lazyLoadingOfProjectStores() async {
        let testConfig = NatProjectConfig(
            projects: [
                "proj-a": ProjectConfig(name: "A Project", slicesDSID: "ds-a", workingDir: "/path/a"),
                "proj-b": ProjectConfig(name: "B Project", slicesDSID: "ds-b", workingDir: "/path/b")
            ]
        )

        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        // proj-a should be loaded (active)
        XCTAssertNotNil(appModel.projectStore)
        XCTAssertEqual(appModel.projectStore?.projectID, "proj-a")

        // Activate proj-b
        await appModel.activateProject("proj-b")

        // Now proj-b should be loaded
        XCTAssertEqual(appModel.projectStore?.projectID, "proj-b")
    }

    // MARK: - Per-project detail/diff/PR stores

    @MainActor
    func testAppModel_sliceDetailStoreIsCreatedOnceAndReused() {
        let appModel = AppModel()

        let first = appModel.sliceDetailStore(projectID: "proj-1")
        let second = appModel.sliceDetailStore(projectID: "proj-1")

        XCTAssertTrue(first === second, "the same project should always get the same cache, not a fresh one per call")
    }

    @MainActor
    func testAppModel_sliceDetailStoreIsSeparatePerProject() {
        let appModel = AppModel()

        let a = appModel.sliceDetailStore(projectID: "proj-a")
        let b = appModel.sliceDetailStore(projectID: "proj-b")

        XCTAssertFalse(a === b)
    }

    @MainActor
    func testAppModel_diffStoreIsCreatedOnceAndReused() {
        let appModel = AppModel()

        let first = appModel.diffStore(projectID: "proj-1")
        let second = appModel.diffStore(projectID: "proj-1")

        XCTAssertTrue(first === second)
    }

    @MainActor
    func testAppModel_prStoreIsCreatedOnceAndReused() {
        let appModel = AppModel()

        let first = appModel.prStore(projectID: "proj-1")
        let second = appModel.prStore(projectID: "proj-1")

        XCTAssertTrue(first === second)
    }

    // MARK: - Onboarding

    @MainActor
    func testAppModel_needsOnboardingBeforeStart() {
        let appModel = AppModel()
        XCTAssertTrue(appModel.needsOnboarding)
    }

    @MainActor
    func testAppModel_needsOnboardingWhenNoConfigFile() async {
        let mockReader = MockConfigReader(response: .failure)
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertTrue(appModel.needsOnboarding)
    }

    @MainActor
    func testAppModel_needsOnboardingWhenProjectsMapIsEmpty() async {
        let testConfig = NatProjectConfig(projects: [:])
        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertTrue(appModel.needsOnboarding)
        XCTAssertNotNil(appModel.config)
        XCTAssertNil(appModel.projectStore)
    }

    @MainActor
    func testAppModel_doesNotNeedOnboardingWithAProject() async {
        let testConfig = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")]
        )
        let mockReader = MockConfigReader(response: .success(testConfig))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")

        XCTAssertFalse(appModel.needsOnboarding)
    }

    // MARK: - Reload config

    @MainActor
    func testAppModel_reloadConfigPicksUpNewValues() async {
        let original = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")],
            pollSeconds: 30
        )
        let mockReader = MockConfigReader(response: .success(original))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        XCTAssertEqual(appModel.config?.pollSeconds, 30)

        let updated = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")],
            pollSeconds: 90
        )
        mockReader.setResponse(.success(updated))

        await appModel.reloadConfig()

        XCTAssertEqual(appModel.config?.pollSeconds, 90)
        // reloadConfig re-reads the same path start() loaded from.
        XCTAssertEqual(mockReader.lastPath, "/fake/config.json")
    }

    @MainActor
    func testAppModel_reloadConfigBeforeStartDoesNothing() async {
        let mockReader = MockConfigReader(response: .success(NatProjectConfig(projects: [:])))
        let appModel = AppModel(configReader: mockReader)

        // Never started, so there is no path to re-read from.
        await appModel.reloadConfig()

        XCTAssertNil(appModel.config)
        XCTAssertNil(mockReader.lastPath)
    }

    @MainActor
    func testAppModel_reloadConfigKeepsThePreviousConfigOnReadFailure() async {
        let original = NatProjectConfig(
            projects: ["proj-1": ProjectConfig(name: "Project 1", slicesDSID: "ds-1", workingDir: "/path/1")]
        )
        let mockReader = MockConfigReader(response: .success(original))
        let appModel = AppModel(configReader: mockReader)

        await appModel.start(configPath: "/fake/config.json", nudgePath: "/fake/nudge")
        mockReader.setResponse(.failure)

        await appModel.reloadConfig()

        XCTAssertEqual(appModel.config, original)
    }
}
