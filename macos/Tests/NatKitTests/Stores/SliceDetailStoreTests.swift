import XCTest
@testable import NatKit

// MARK: - Mock Client

private struct SliceDetailTestError: Error {}

private final class MockSliceDetailClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(SliceDetail)
        case failure
    }

    /// Per-slice-ref response, checked before `defaultResponse` — lets a
    /// test give two different slices two different answers.
    var responses: [String: Response] = [:]
    var defaultResponse: Response
    /// Holds `sliceShow` open until this many nanoseconds have passed, so a
    /// test can assert on the store's state while a fetch is genuinely still
    /// in flight rather than racing a call that already returned.
    var delayNanoseconds: UInt64 = 0
    private(set) var callCount = 0
    private(set) var sliceRefCalls: [String] = []

    init(response: Response) {
        self.defaultResponse = response
    }

    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        callCount += 1
        sliceRefCalls.append(sliceRef)
        if delayNanoseconds > 0 {
            try? await Task.sleep(nanoseconds: delayNanoseconds)
        }
        switch responses[sliceRef] ?? defaultResponse {
        case .success(let detail): return detail
        case .failure: throw SliceDetailTestError()
        }
    }

    // MARK: Unused by these tests

    func info(projectID: String) async throws -> ProjectInfo { throw SliceDetailTestError() }
    func status() async throws -> [AgentStatus] { [] }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        throw SliceDetailTestError()
    }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        throw SliceDetailTestError()
    }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw SliceDetailTestError()
    }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw SliceDetailTestError()
    }
    func agentInterrupt(projectID: String, sliceRef: String) async throws { throw SliceDetailTestError() }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw SliceDetailTestError() }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { throw SliceDetailTestError() }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail { throw SliceDetailTestError() }
    func prMerge(projectID: String, sliceRef: String) async throws { throw SliceDetailTestError() }
    func prComment(projectID: String, sliceRef: String, body: String) async throws { throw SliceDetailTestError() }
    func workshopLaunch(projectID: String, model: String?, effort: String?) async throws -> WorkshopLaunchResult {
        throw SliceDetailTestError()
    }
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        throw SliceDetailTestError()
    }
    func configShow() async throws -> ConfigDoc { throw SliceDetailTestError() }
    func configSet(key: String, value: String) async throws { throw SliceDetailTestError() }
}

// MARK: - Tests

final class SliceDetailStoreTests: XCTestCase {
    private func makeDetail(id: String = "slice-1", brief: String = "Do the thing") -> SliceDetail {
        SliceDetail(
            id: id, name: "Test slice", url: "https://example.test/\(id)",
            status: "Todo", milestone: "M1", assignee: "",
            blocked: false, handedBack: false, brief: brief
        )
    }

    @MainActor
    func testStateIsIdleForASliceNeverFetched() {
        let client = MockSliceDetailClient(response: .success(makeDetail()))
        let store = SliceDetailStore(projectID: "proj-1", client: client)

        XCTAssertEqual(store.state(for: "slice-1"), .idle)
        XCTAssertNil(store.state(for: "slice-1").detail)
    }

    @MainActor
    func testFetchLoadsTheDetail() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "the brief")))
        let store = SliceDetailStore(projectID: "proj-1", client: client)

        await store.fetch(sliceRef: "slice-1")

        XCTAssertEqual(client.callCount, 1)
        XCTAssertEqual(store.state(for: "slice-1").detail?.brief, "the brief")
    }

    @MainActor
    func testASecondFetchOfTheSameSliceStillReadsFreshInTheBackground() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "first")))
        let store = SliceDetailStore(projectID: "proj-1", client: client)

        await store.fetch(sliceRef: "slice-1")
        client.defaultResponse = .success(makeDetail(brief: "second"))
        await store.fetch(sliceRef: "slice-1")

        // Re-selecting a slice always re-reads it — unlike DiffStore/PRStore,
        // a brief is Notion content that can change independent of any
        // external poll, so it is kept honest on every re-selection.
        XCTAssertEqual(client.callCount, 2)
        XCTAssertEqual(store.state(for: "slice-1").detail?.brief, "second")
    }

    @MainActor
    func testCachedDetailStaysVisibleWhileARereadIsInFlight() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "cached")))
        client.delayNanoseconds = 50_000_000
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")

        // A second fetch of the same slice starts a genuine background
        // read (unlike DiffStore/PRStore, a brief always re-reads) — the
        // cached value stays visible throughout rather than being cleared
        // the instant the read starts.
        let task = Task { await store.fetch(sliceRef: "slice-1") }
        try? await Task.sleep(nanoseconds: 10_000_000)
        XCTAssertEqual(store.state(for: "slice-1").detail?.brief, "cached")
        XCTAssertTrue(store.state(for: "slice-1").isLoading)
        await task.value
    }

    @MainActor
    func testASecondConcurrentFetchOfTheSameSliceIsNotStartedTwice() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "one")))
        client.delayNanoseconds = 50_000_000
        let store = SliceDetailStore(projectID: "proj-1", client: client)

        async let first: Void = store.fetch(sliceRef: "slice-1")
        async let second: Void = store.fetch(sliceRef: "slice-1")
        _ = await (first, second)

        XCTAssertEqual(client.callCount, 1, "a fetch already in flight for a slice should not be started again")
    }

    @MainActor
    func testEachSliceIsCachedIndependently() async {
        let client = MockSliceDetailClient(response: .success(makeDetail()))
        client.responses = [
            "slice-1": .success(makeDetail(id: "slice-1", brief: "one")),
            "slice-2": .success(makeDetail(id: "slice-2", brief: "two"))
        ]
        let store = SliceDetailStore(projectID: "proj-1", client: client)

        await store.fetch(sliceRef: "slice-1")
        await store.fetch(sliceRef: "slice-2")

        XCTAssertEqual(store.state(for: "slice-1").detail?.brief, "one")
        XCTAssertEqual(store.state(for: "slice-2").detail?.brief, "two")
    }

    @MainActor
    func testReselectingASliceRendersItsCacheInstantlyEvenWhileTheRereadIsStillInFlight() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "one")))
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")
        await store.fetch(sliceRef: "slice-2")

        // Switching back to slice-1 sees its cached detail the instant
        // `fetch` is called — before the background read this kicks off
        // has any chance to land.
        client.delayNanoseconds = 50_000_000
        let fetchTask = Task { await store.fetch(sliceRef: "slice-1") }
        XCTAssertEqual(store.state(for: "slice-1").detail?.brief, "one")
        await fetchTask.value
    }

    @MainActor
    func testFailedFetchKeepsThePreviousDetailVisible() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "good")))
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")

        client.defaultResponse = .failure
        await store.fetch(sliceRef: "slice-1")

        let state = store.state(for: "slice-1")
        XCTAssertEqual(state.detail?.brief, "good", "a failed read should keep the last good brief visible, not blank it")
        XCTAssertNotNil(state.errorMessage)
    }

    @MainActor
    func testFailedFirstFetchHasNoPreviousDetailToKeep() async {
        let client = MockSliceDetailClient(response: .failure)
        let store = SliceDetailStore(projectID: "proj-1", client: client)

        await store.fetch(sliceRef: "slice-1")

        let state = store.state(for: "slice-1")
        XCTAssertNil(state.detail)
        XCTAssertNotNil(state.errorMessage)
    }

    @MainActor
    func testInvalidateCacheDropsEveryCachedSlice() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "one")))
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")

        store.invalidateCache()

        XCTAssertEqual(store.state(for: "slice-1"), .idle)
    }

    @MainActor
    func testInvalidateCacheKeepingASliceLeavesItAlone() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "one")))
        client.responses = [
            "slice-1": .success(makeDetail(id: "slice-1", brief: "one")),
            "slice-2": .success(makeDetail(id: "slice-2", brief: "two"))
        ]
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")
        await store.fetch(sliceRef: "slice-2")

        store.invalidateCache(keeping: "slice-2")

        XCTAssertEqual(store.state(for: "slice-1"), .idle, "a slice not currently on screen is dropped")
        XCTAssertEqual(store.state(for: "slice-2").detail?.brief, "two", "the slice on screen is left showing what it has")
    }

    @MainActor
    func testInvalidateCacheKeepingASliceNotInTheCacheIsAFullWipe() async {
        let client = MockSliceDetailClient(response: .success(makeDetail(brief: "one")))
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")

        store.invalidateCache(keeping: "slice-2") // never fetched

        XCTAssertEqual(store.state(for: "slice-1"), .idle)
    }

    @MainActor
    func testClearDropsEverything() async {
        let client = MockSliceDetailClient(response: .success(makeDetail()))
        let store = SliceDetailStore(projectID: "proj-1", client: client)
        await store.fetch(sliceRef: "slice-1")

        store.clear()

        XCTAssertEqual(store.state(for: "slice-1"), .idle)

        // A fetch works again after a clear — nothing about the store is
        // left in a stuck "still fetching" state.
        await store.fetch(sliceRef: "slice-1")
        XCTAssertEqual(client.callCount, 2)
    }

    @MainActor
    func testLoadStateAccessors() {
        let detail = makeDetail(brief: "x")
        XCTAssertEqual(SliceDetailLoadState.loaded(detail).detail, detail)
        XCTAssertNil(SliceDetailLoadState.loaded(detail).errorMessage)
        XCTAssertFalse(SliceDetailLoadState.loaded(detail).isLoading)

        XCTAssertTrue(SliceDetailLoadState.loading(stale: nil).isLoading)
        XCTAssertEqual(SliceDetailLoadState.loading(stale: detail).detail, detail)

        XCTAssertEqual(SliceDetailLoadState.failed("oops", previous: detail).detail, detail)
        XCTAssertEqual(SliceDetailLoadState.failed("oops", previous: nil).errorMessage, "oops")

        XCTAssertNil(SliceDetailLoadState.idle.detail)
        XCTAssertFalse(SliceDetailLoadState.idle.isLoading)
    }
}
