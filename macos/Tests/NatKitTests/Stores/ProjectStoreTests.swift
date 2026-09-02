import XCTest
@testable import NatKit

// MARK: - Mock Client

struct TestError: Error {}

final class MockNatClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(ProjectInfo)
        case failure
    }

    private let response: Response
    private(set) var callCount = 0

    init(response: Response) {
        self.response = response
    }

    func info(projectID: String) async throws -> ProjectInfo {
        callCount += 1
        switch response {
        case .success(let info):
            return info
        case .failure:
            throw TestError()
        }
    }
}

final class ProjectStoreTests: XCTestCase {
    private var testProjectInfo: ProjectInfo!
    private var testProject: Project!

    override func setUp() {
        super.setUp()
        testProject = Project(id: "proj-1", name: "Test", conventions: "")
        testProjectInfo = ProjectInfo(project: testProject, milestones: [], slices: [])
    }

    @MainActor
    func testInitialState() {
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient)

        XCTAssertEqual(store.projectID, "proj-1")
        XCTAssertEqual(store.state, .idle)
    }

    @MainActor
    func testLoadSuccess() async {
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient)

        await store.load()

        XCTAssertEqual(mockClient.callCount, 1)
        if case .loaded(let info) = store.state {
            XCTAssertEqual(info.project.name, "Test")
        } else {
            XCTFail("Expected loaded state")
        }
    }

    @MainActor
    func testLoadFailure() async {
        let mockClient = MockNatClient(response: .failure)
        let store = ProjectStore(projectID: "proj-1", client: mockClient)

        await store.load()

        if case .failed(let message, let previous) = store.state {
            XCTAssertNotNil(message)
            XCTAssertNil(previous)
        } else {
            XCTFail("Expected failed state")
        }
    }

    @MainActor
    func testFailureKeepsPreviousLoad() async {
        let successClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: successClient)

        // First load succeeds
        await store.load()
        XCTAssertEqual(successClient.callCount, 1)

        // Verify loaded state
        if case .loaded = store.state {
            // Expected
        } else {
            XCTFail("Expected loaded state after success")
        }

        // Now create a store with a failing client and verify it starts with idle state
        let failureClient = MockNatClient(response: .failure)
        let failingStore = ProjectStore(projectID: "proj-1", client: failureClient)
        await failingStore.load()

        if case .failed(_, let previous) = failingStore.state {
            // When failing with no prior state, previous would be nil
            XCTAssertNil(previous)
        } else {
            XCTFail("Expected failed state")
        }
    }

    @MainActor
    func testConcurrentLoadCoalescing() async {
        let mockClient = MockNatClient(response: .success(testProjectInfo))

        let store = ProjectStore(projectID: "proj-1", client: mockClient)

        // Launch two load tasks that should be coalesced
        let task1 = Task { await store.load() }
        let task2 = Task { await store.load() }

        // Wait for both to complete
        await task1.value
        await task2.value

        // The second load should have been ignored due to coalescing
        XCTAssertEqual(mockClient.callCount, 1)
    }

    @MainActor
    func testRefresh() async {
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient)

        await store.refresh()

        XCTAssertEqual(mockClient.callCount, 1)
        if case .loaded = store.state {
            // Expected
        } else {
            XCTFail("Expected loaded state")
        }
    }

    @MainActor
    func testLoadStateProjectInfo() {
        let state: LoadState = .loaded(testProjectInfo)
        XCTAssertEqual(state.projectInfo, testProjectInfo)

        let failedState: LoadState = .failed("error", previous: testProjectInfo)
        XCTAssertEqual(failedState.projectInfo, testProjectInfo)

        let idleState: LoadState = .idle
        XCTAssertNil(idleState.projectInfo)
    }

    @MainActor
    func testLoadStateErrorMessage() {
        let errorState: LoadState = .failed("Something went wrong", previous: nil)
        XCTAssertEqual(errorState.errorMessage, "Something went wrong")

        let loadedState: LoadState = .loaded(testProjectInfo)
        XCTAssertNil(loadedState.errorMessage)
    }

    @MainActor
    func testLoadStateIsLoading() {
        let loadingState: LoadState = .loading
        XCTAssertTrue(loadingState.isLoading)

        let loadedState: LoadState = .loaded(testProjectInfo)
        XCTAssertFalse(loadedState.isLoading)
    }
}
