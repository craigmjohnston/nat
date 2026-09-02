import XCTest
@testable import NatKit

// MARK: - Mock Config Reader

struct MockConfigReader: ConfigReaderProtocol, @unchecked Sendable {
    enum Response: Sendable {
        case success(NatProjectConfig)
        case failure
    }

    private let response: Response

    init(response: Response) {
        self.response = response
    }

    func readConfig(from path: String) async throws -> NatProjectConfig {
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
}
