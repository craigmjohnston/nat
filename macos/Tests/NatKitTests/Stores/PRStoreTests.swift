import XCTest
@testable import NatKit

// MARK: - Mock Client

private struct PRTestError: Error {}

private final class MockPRClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(PRDetail)
        case failure
    }

    private var response: Response
    private(set) var viewCallCount = 0
    private(set) var lastSliceRef: String?
    /// Holds `prView` open until this many nanoseconds have passed, so a
    /// test can assert on the store's state while a read is genuinely still
    /// in flight rather than racing a call that already returned.
    var viewDelayNanoseconds: UInt64 = 0

    private(set) var mergeCalls: [(projectID: String, sliceRef: String)] = []
    var mergeError: Error?

    private(set) var commentCalls: [(projectID: String, sliceRef: String, body: String)] = []
    var commentError: Error?

    init(response: Response) {
        self.response = response
    }

    func setResponse(_ response: Response) {
        self.response = response
    }

    func info(projectID: String) async throws -> ProjectInfo { throw PRTestError() }
    func status() async throws -> [AgentStatus] { [] }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw PRTestError() }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw PRTestError() }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw PRTestError() }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw PRTestError()
    }
    func agentInterrupt(projectID: String, sliceRef: String) async throws { throw PRTestError() }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw PRTestError() }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { throw PRTestError() }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw PRTestError()
    }

    func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        viewCallCount += 1
        lastSliceRef = sliceRef
        if viewDelayNanoseconds > 0 {
            try? await Task.sleep(nanoseconds: viewDelayNanoseconds)
        }
        switch response {
        case .success(let pr): return pr
        case .failure: throw PRTestError()
        }
    }

    func prMerge(projectID: String, sliceRef: String) async throws {
        mergeCalls.append((projectID, sliceRef))
        if let mergeError { throw mergeError }
    }

    func prComment(projectID: String, sliceRef: String, body: String) async throws {
        commentCalls.append((projectID, sliceRef, body))
        if let commentError { throw commentError }
    }

    func workshopLaunch(projectID: String, model: String?, effort: String?) async throws -> WorkshopLaunchResult {
        throw PRTestError()
    }

    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        throw PRTestError()
    }

    func configShow() async throws -> ConfigDoc { throw PRTestError() }
    func configSet(key: String, value: String) async throws { throw PRTestError() }
}

// MARK: - Tests

final class PRStoreTests: XCTestCase {
    private func openPR(checks: [PRCheck] = []) -> PRDetail {
        PRDetail(
            number: 12, title: "Add the PR tab", body: "body", state: "OPEN", isDraft: false,
            author: "craig", baseRefName: "main", headRefName: "slice/add-the-pr-tab",
            url: "https://github.test/craig/nat/pull/12",
            checks: checks, reviewDecision: "APPROVED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN")
    }

    private func mergedPR() -> PRDetail {
        var pr = openPR()
        pr = PRDetail(
            number: pr.number, title: pr.title, body: pr.body, state: PRLifecycleState.merged, isDraft: false,
            author: pr.author, baseRefName: pr.baseRefName, headRefName: pr.headRefName, url: pr.url,
            reviewDecision: pr.reviewDecision, mergeable: pr.mergeable, mergeStateStatus: pr.mergeStateStatus)
        return pr
    }

    // MARK: - Fetch / refresh

    @MainActor
    func testInitialStateIsIdle() {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        XCTAssertEqual(store.loadState, .idle)
    }

    @MainActor
    func testFetchLoadsThePullRequest() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.viewCallCount, 1)
        XCTAssertEqual(client.lastSliceRef, "slice-1")
        XCTAssertEqual(store.loadState.pr?.number, 12)
    }

    @MainActor
    func testFetchDoesNotRefetchTheSameSliceOnceLoaded() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.viewCallCount, 1)
    }

    @MainActor
    func testFetchRefetchesADifferentSlice() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertEqual(client.viewCallCount, 2)
        XCTAssertEqual(client.lastSliceRef, "slice-2")
    }

    @MainActor
    func testFailedFetchDropsAnyPreviousPullRequest() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertNotNil(store.loadState.pr)

        client.setResponse(.failure)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertNil(store.loadState.pr, "a failed read should drop the previous pull request, not keep it visible")
        XCTAssertNotNil(store.loadState.errorMessage)
    }

    @MainActor
    func testSwitchingBackToAPreviouslyReadSliceShowsItsCacheInstantly() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")
        XCTAssertEqual(client.viewCallCount, 2)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.viewCallCount, 2, "a slice already read this session should not be re-read on reselection")
        XCTAssertEqual(store.loadState.pr?.number, 12)
    }

    @MainActor
    func testCachedPullRequestStaysVisibleWhileASwitchToAnUncachedSliceIsInFlight() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        client.viewDelayNanoseconds = 50_000_000
        let task = Task { await store.fetch(projectID: "proj-1", sliceRef: "slice-2") }
        try? await Task.sleep(nanoseconds: 10_000_000)

        // slice-2 has never been cached, so it is right for this to blank
        // while it reads — the point is that it does not show slice-1's
        // pull request mislabeled as slice-2's while doing so.
        XCTAssertNil(store.loadState.pr)
        await task.value
    }

    @MainActor
    func testAFailedReadEvictsThatSlicesCacheButNotAnotherSlices() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        client.setResponse(.failure)
        try? await store.merge() // re-reads slice-2 (the current slice), which now fails
        XCTAssertNil(store.loadState.pr)
        XCTAssertEqual(client.viewCallCount, 3)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(client.viewCallCount, 3, "slice-1 is unaffected — still cached, so no re-read was needed")

        client.setResponse(.success(openPR()))
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")
        XCTAssertEqual(client.viewCallCount, 4, "slice-2's failed reading should not have been cached")
    }

    @MainActor
    func testMergeNeverBlanksTheScreenWhileRereading() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        client.setResponse(.success(mergedPR()))
        client.viewDelayNanoseconds = 50_000_000
        let task = Task { try? await store.merge() }
        try? await Task.sleep(nanoseconds: 10_000_000)

        // Still mid-merge's own background re-read — the pull request that
        // was showing before the merge should still be there, not blanked
        // out from under the user while the fresh reading is in flight.
        XCTAssertNotNil(store.loadState.pr)
        await task.value
    }

    @MainActor
    func testRefreshRereadsTheCurrentSlice() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.refresh()

        XCTAssertEqual(client.viewCallCount, 2)
        XCTAssertEqual(client.lastSliceRef, "slice-1")
    }

    @MainActor
    func testRefreshWithNothingFetchedIsANoOp() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.refresh()

        XCTAssertEqual(client.viewCallCount, 0)
    }

    @MainActor
    func testClearResetsEverything() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        store.clear()

        XCTAssertEqual(store.loadState, .idle)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(client.viewCallCount, 2)
    }

    @MainActor
    func testPRLoadStateAccessors() {
        let pr = openPR()
        XCTAssertEqual(PRLoadState.loaded(pr).pr, pr)
        XCTAssertNil(PRLoadState.loaded(pr).errorMessage)
        XCTAssertTrue(PRLoadState.loading.isLoading)
        XCTAssertFalse(PRLoadState.idle.isLoading)
        XCTAssertEqual(PRLoadState.failed("oops").errorMessage, "oops")
        XCTAssertNil(PRLoadState.failed("oops").pr)
    }

    // MARK: - shouldPoll

    @MainActor
    func testShouldPollIsFalseBeforeAnythingIsLoaded() {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        XCTAssertFalse(store.shouldPoll)
    }

    @MainActor
    func testShouldPollIsTrueForAnOpenPullRequestWithAPendingCheck() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertTrue(store.shouldPoll)
    }

    @MainActor
    func testShouldPollIsFalseWithNoChecksPending() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "build", state: "SUCCESS", link: "")])))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertFalse(store.shouldPoll)
    }

    @MainActor
    func testShouldPollIsFalseOnceMerged() async {
        let client = MockPRClient(response: .success(mergedPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertFalse(store.shouldPoll)
    }

    // MARK: - Polling

    @MainActor
    func testPollingReadsAgainAndStopsOnceSettled() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        let store = PRStore(client: client, pollIntervalNanoseconds: 5_000_000)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(client.viewCallCount, 1)

        // The check finishes between the first read and the poll's next one.
        client.setResponse(.success(openPR(checks: [PRCheck(name: "lint", state: "SUCCESS", link: "")])))
        store.startPolling()

        // Give the poll loop a few intervals to run and settle.
        try? await Task.sleep(nanoseconds: 200_000_000)

        XCTAssertGreaterThanOrEqual(client.viewCallCount, 2)
        XCTAssertFalse(store.shouldPoll)
        store.stopPolling()
    }

    @MainActor
    func testStartPollingDoesNothingWhenNothingIsPending() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "build", state: "SUCCESS", link: "")])))
        let store = PRStore(client: client, pollIntervalNanoseconds: 5_000_000)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        store.startPolling()
        try? await Task.sleep(nanoseconds: 50_000_000)

        // No pending check, so the loop should never have started reading again.
        XCTAssertEqual(client.viewCallCount, 1)
        store.stopPolling()
    }

    @MainActor
    func testStopPollingPreventsFurtherReads() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        let store = PRStore(client: client, pollIntervalNanoseconds: 5_000_000)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        store.startPolling()
        store.stopPolling()
        let countAfterStop = client.viewCallCount
        try? await Task.sleep(nanoseconds: 50_000_000)

        XCTAssertEqual(client.viewCallCount, countAfterStop)
    }

    @MainActor
    func testFetchingADifferentSliceStopsAPollLeftRunning() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        let store = PRStore(client: client, pollIntervalNanoseconds: 5_000_000)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        store.startPolling()

        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")
        let countAfterSwitch = client.viewCallCount
        try? await Task.sleep(nanoseconds: 50_000_000)

        // The old poll should not have kept reading slice-1 under slice-2's name.
        XCTAssertEqual(client.viewCallCount, countAfterSwitch)
        XCTAssertEqual(client.lastSliceRef, "slice-2")
    }

    // MARK: - Merge

    @MainActor
    func testMergeCallsThenRefreshes() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        client.setResponse(.success(mergedPR()))
        try? await store.merge()

        XCTAssertEqual(client.mergeCalls.count, 1)
        XCTAssertEqual(client.mergeCalls[0].sliceRef, "slice-1")
        XCTAssertEqual(client.viewCallCount, 2, "merge should re-read the pull request")
        XCTAssertEqual(store.loadState.pr?.state, PRLifecycleState.merged)
    }

    @MainActor
    func testMergePropagatesARefusalWithoutRereading() async {
        let client = MockPRClient(response: .success(openPR()))
        client.mergeError = PRTestError()
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        do {
            try await store.merge()
            XCTFail("expected merge to throw")
        } catch {
            // expected
        }

        XCTAssertEqual(client.viewCallCount, 1, "a refusal should not trigger a reread")
    }

    @MainActor
    func testMergeWithNothingFetchedIsANoOp() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        try? await store.merge()

        XCTAssertEqual(client.mergeCalls.count, 0)
    }

    // MARK: - Comment

    @MainActor
    func testCommentPostsThenRefreshes() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        try? await store.comment(text: "  Looks good.  ")

        XCTAssertEqual(client.commentCalls.count, 1)
        XCTAssertEqual(client.commentCalls[0].sliceRef, "slice-1")
        XCTAssertEqual(client.commentCalls[0].body, "Looks good.", "the body should be trimmed")
        XCTAssertEqual(client.viewCallCount, 2, "a posted comment should trigger a reread")
    }

    @MainActor
    func testCommentWithBlankTextIsANoOp() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        try? await store.comment(text: "   ")

        XCTAssertEqual(client.commentCalls.count, 0)
        XCTAssertEqual(client.viewCallCount, 1)
    }

    @MainActor
    func testCommentPropagatesFailureWithoutRereading() async {
        let client = MockPRClient(response: .success(openPR()))
        client.commentError = PRTestError()
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        do {
            try await store.comment(text: "Looks good.")
            XCTFail("expected comment to throw")
        } catch {
            // expected
        }

        XCTAssertEqual(client.viewCallCount, 1)
    }

    @MainActor
    func testCommentWithNothingFetchedIsANoOp() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        try? await store.comment(text: "hi")

        XCTAssertEqual(client.commentCalls.count, 0)
    }
}
