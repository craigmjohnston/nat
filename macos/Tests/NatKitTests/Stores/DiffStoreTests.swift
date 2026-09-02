import XCTest
@testable import NatKit

// MARK: - Mock Client

private struct DiffTestError: Error {}

private final class MockDiffClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(SliceDiff)
        case failure
    }

    private var response: Response
    private(set) var callCount = 0
    private(set) var lastSliceRef: String?

    init(response: Response) {
        self.response = response
    }

    func setResponse(_ response: Response) {
        self.response = response
    }

    func info(projectID: String) async throws -> ProjectInfo {
        throw DiffTestError()
    }

    func status() async throws -> [AgentStatus] {
        []
    }

    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        throw DiffTestError()
    }

    func sliceDiff(projectID: String, sliceRef: String) async throws -> SliceDiff {
        callCount += 1
        lastSliceRef = sliceRef
        switch response {
        case .success(let diff):
            return diff
        case .failure:
            throw DiffTestError()
        }
    }

    func agentInterrupt(projectID: String, sliceRef: String) async throws {
        throw DiffTestError()
    }
}

// MARK: - Tests

final class DiffStoreTests: XCTestCase {
    private func makeDiff(file path: String = "a.go") -> SliceDiff {
        SliceDiff(
            base: "main",
            branch: "nat/example",
            files: [
                SliceDiffFile(
                    path: path, oldPath: path, adds: 1, dels: 0, described: false,
                    lines: [
                        "diff --git a/\(path) b/\(path)",
                        "--- a/\(path)",
                        "+++ b/\(path)",
                        "@@ -0,0 +1,1 @@",
                        "+hello"
                    ]
                )
            ]
        )
    }

    @MainActor
    func testInitialStateIsIdle() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        XCTAssertEqual(store.loadState, .idle)
        XCTAssertTrue(store.viewedFiles.isEmpty)
        XCTAssertTrue(store.collapsedFiles.isEmpty)
    }

    @MainActor
    func testFetchLoadsTheDiff() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.callCount, 1)
        XCTAssertEqual(client.lastSliceRef, "slice-1")
        XCTAssertEqual(store.loadState.diff?.branch, "nat/example")
    }

    @MainActor
    func testFetchDoesNotRefetchTheSameSliceOnceLoaded() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.callCount, 1)
    }

    @MainActor
    func testFetchRefetchesADifferentSlice() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertEqual(client.callCount, 2)
        XCTAssertEqual(client.lastSliceRef, "slice-2")
    }

    @MainActor
    func testFailedFetchDropsAnyPreviousDiff() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertNotNil(store.loadState.diff)

        client.setResponse(.failure)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertNil(store.loadState.diff, "a failed read should drop the previous diff, not keep it visible")
        XCTAssertNotNil(store.loadState.errorMessage)
    }

    @MainActor
    func testRefreshRereadsTheCurrentSlice() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.refresh()

        XCTAssertEqual(client.callCount, 2)
        XCTAssertEqual(client.lastSliceRef, "slice-1")
    }

    @MainActor
    func testRefreshWithNothingFetchedIsANoOp() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.refresh()

        XCTAssertEqual(client.callCount, 0)
    }

    @MainActor
    func testViewedAndCollapsedAreClearedByARefresh() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        store.toggleViewed("a.go")
        XCTAssertTrue(store.isViewed("a.go"))
        XCTAssertTrue(store.isCollapsed("a.go"))

        await store.refresh()

        XCTAssertFalse(store.isViewed("a.go"))
        XCTAssertFalse(store.isCollapsed("a.go"))
    }

    @MainActor
    func testToggleViewedMarksAndCollapsesGitHubFashion() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        store.toggleViewed("a.go")
        XCTAssertTrue(store.isViewed("a.go"))
        XCTAssertTrue(store.isCollapsed("a.go"))

        store.toggleViewed("a.go")
        XCTAssertFalse(store.isViewed("a.go"))
        // Un-marking viewed leaves the fold as it was: asking to look again is
        // not the same as asking to expand it.
        XCTAssertTrue(store.isCollapsed("a.go"))
    }

    @MainActor
    func testToggleCollapsedIsIndependentOfViewed() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        store.toggleCollapsed("a.go")
        XCTAssertTrue(store.isCollapsed("a.go"))
        XCTAssertFalse(store.isViewed("a.go"))

        store.toggleCollapsed("a.go")
        XCTAssertFalse(store.isCollapsed("a.go"))
    }

    @MainActor
    func testClearResetsEverything() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        store.toggleViewed("a.go")
        store.clear()

        XCTAssertEqual(store.loadState, .idle)
        XCTAssertTrue(store.viewedFiles.isEmpty)
        XCTAssertTrue(store.collapsedFiles.isEmpty)

        // fetch() re-fetches after a clear even for the same slice ref.
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(client.callCount, 2)
    }

    @MainActor
    func testDiffLoadStateAccessors() {
        let diff = DiffModel(base: "main", branch: "b", files: [])
        XCTAssertEqual(DiffLoadState.loaded(diff).diff, diff)
        XCTAssertNil(DiffLoadState.loaded(diff).errorMessage)
        XCTAssertTrue(DiffLoadState.loading.isLoading)
        XCTAssertFalse(DiffLoadState.idle.isLoading)
        XCTAssertEqual(DiffLoadState.failed("oops").errorMessage, "oops")
        XCTAssertNil(DiffLoadState.failed("oops").diff)
    }
}
