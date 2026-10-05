import XCTest
@testable import NatKit

// MARK: - Mock Client

private struct PRTestError: Error {}

/// Holds a fake `prView` open until the test opens it, and tells the test
/// when a read has reached it — so a test asserts on the store's state while
/// a read is genuinely in flight, with no sleep racing the read's start.
private actor ReadGate {
    private var entered = false
    private var enterWaiters: [CheckedContinuation<Void, Never>] = []
    private var isOpen = false
    private var held: [CheckedContinuation<Void, Never>] = []

    /// The read's side: say it has begun, then wait for the gate to open.
    func pass() async {
        entered = true
        enterWaiters.forEach { $0.resume() }
        enterWaiters = []
        guard !isOpen else { return }
        await withCheckedContinuation { held.append($0) }
    }

    /// The test's side: returns once a read has reached the gate.
    func waitForRead() async {
        guard !entered else { return }
        await withCheckedContinuation { enterWaiters.append($0) }
    }

    /// Lets every held read, and every later one, through.
    func open() {
        isOpen = true
        held.forEach { $0.resume() }
        held = []
    }
}

private final class MockPRClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(PRDetail)
        case failure
    }

    private var response: Response
    private(set) var viewCallCount = 0
    private(set) var lastSliceRef: String?
    /// Holds `prView` at this gate until the test opens it (`ReadGate`).
    var viewGate: ReadGate?

    private(set) var mergeCalls: [(projectID: String, sliceRef: String)] = []
    var mergeError: Error?

    private(set) var commentCalls: [(projectID: String, sliceRef: String, body: String)] = []
    var commentError: Error?

    private(set) var reviewerCalls: [(add: [String], remove: [String])] = []
    var reviewersError: Error?

    init(response: Response) {
        self.response = response
    }

    func setResponse(_ response: Response) {
        self.response = response
    }

    func info(projectID: String) async throws -> ProjectInfo { throw PRTestError() }
    func status() async throws -> [AgentStatus] { [] }
    func usage() async throws -> UsageReading { .empty }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw PRTestError() }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw PRTestError() }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw PRTestError() }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw PRTestError()
    }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw PRTestError() }
    func agentKill(projectID: String, sliceRef: String) async throws { throw PRTestError() }
    func agentKillWorkshop(projectID: String) async throws { throw PRTestError() }
    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult { throw PRTestError() }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { throw PRTestError() }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw PRTestError()
    }

    func prStatus(projectID: String) async throws -> PRStatusDoc { throw PRTestError() }
    private(set) var sessionViewCalls: [(sessionID: String, url: String)] = []
    func sessionPRView(projectID: String, sessionID: String, prURL: String) async throws -> PRDetail {
        sessionViewCalls.append((sessionID, prURL))
        switch response {
        case .success(let pr): return pr
        case .failure: throw PRTestError()
        }
    }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        viewCallCount += 1
        lastSliceRef = sliceRef
        if let viewGate { await viewGate.pass() }
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

    func prReviewers(projectID: String, sliceRef: String, add: [String], remove: [String]) async throws -> PRReviewers {
        reviewerCalls.append((add, remove))
        if let reviewersError { throw reviewersError }
        return PRReviewers(pr: sliceRef, requested: add, candidates: ["mona"])
    }

    private(set) var checksCalls: [String] = []
    var checksError: Error?

    func sliceChecksRerun(projectID: String, sliceRef: String, mode: ChecksRerunMode) async throws -> ChecksActionResult {
        checksCalls.append("rerun \(mode)")
        if let checksError { throw checksError }
        return ChecksActionResult(cancelled: ["test", "lint"], rerun: ["test", "lint", "build"])
    }

    func sliceChecksCancel(projectID: String, sliceRef: String, checks: [String]) async throws -> ChecksActionResult {
        checksCalls.append("cancel \(checks)")
        if let checksError { throw checksError }
        return ChecksActionResult(cancelled: ["test"])
    }

    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult {
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

    private func mergedPR(state: String = PRLifecycleState.merged) -> PRDetail {
        var pr = openPR()
        pr = PRDetail(
            number: pr.number, title: pr.title, body: pr.body, state: state, isDraft: false,
            author: pr.author, baseRefName: pr.baseRefName, headRefName: pr.headRefName, url: pr.url,
            reviewDecision: pr.reviewDecision, mergeable: pr.mergeable, mergeStateStatus: pr.mergeStateStatus)
        return pr
    }

    /// Waits until `condition` holds, or five seconds pass — for a poll or
    /// background read the test cannot await directly. A slow runner only
    /// makes this take longer; the assertions after it say whether it held.
    @MainActor
    private func waitUntil(_ condition: () -> Bool) async {
        let deadline = ContinuousClock.now + .seconds(5)
        while !condition(), ContinuousClock.now < deadline {
            try? await Task.sleep(for: .milliseconds(2))
        }
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
    func testFetchOfASessionsPullRequestReadsItThroughTheSession() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "https://x/pull/12", sessionID: "sess-1")

        XCTAssertEqual(client.viewCallCount, 0, "not read as a slice")
        XCTAssertEqual(client.sessionViewCalls.count, 1)
        XCTAssertEqual(client.sessionViewCalls.first?.sessionID, "sess-1")
        XCTAssertEqual(client.sessionViewCalls.first?.url, "https://x/pull/12")
        XCTAssertEqual(store.loadState.pr?.number, 12)

        await store.fetch(projectID: "proj-1", sliceRef: "https://x/pull/12", sessionID: "sess-1")
        XCTAssertEqual(client.sessionViewCalls.count, 1, "already loaded")

        await store.refresh()
        XCTAssertEqual(client.sessionViewCalls.count, 2, "a refresh keeps reading through the session")

        store.clear()
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(client.viewCallCount, 1, "clear forgets the session")
    }

    @MainActor
    func testFetchOfASessionsPullRequestFailureIsFailed() async {
        let store = PRStore(client: MockPRClient(response: .failure))
        await store.fetch(projectID: "proj-1", sliceRef: "https://x/pull/12", sessionID: "sess-1")
        XCTAssertNotNil(store.loadState.errorMessage)
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

        let gate = ReadGate()
        client.viewGate = gate
        let task = Task { await store.fetch(projectID: "proj-1", sliceRef: "slice-2") }
        await gate.waitForRead()

        // slice-2 has never been cached, so it is right for this to blank
        // while it reads — the point is that it does not show slice-1's
        // pull request mislabeled as slice-2's while doing so.
        XCTAssertNil(store.loadState.pr)
        await gate.open()
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
        XCTAssertNotNil(store.loadState.pr, "a failed re-read keeps the reading it could not replace")
        XCTAssertNotNil(store.loadState.errorMessage, "and says why what is up is the last one")
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
        let gate = ReadGate()
        client.viewGate = gate
        let task = Task { try? await store.merge() }
        await gate.waitForRead()

        // Still mid-merge's own background re-read — the pull request that
        // was showing before the merge should still be there, not blanked
        // out from under the user while the fresh reading is in flight.
        XCTAssertNotNil(store.loadState.pr)
        await gate.open()
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

    /// The five-second poll runs over a pull request already on screen and
    /// never blanks it; `isRefreshing` is what the pane draws its busy mark
    /// from, in a slot it reserves either way, so a poll moves nothing.
    @MainActor
    func testARefreshKeepsThePullRequestUpAndSaysItIsRunning() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertFalse(store.isRefreshing)

        let gate = ReadGate()
        client.viewGate = gate
        let task = Task { await store.refresh() }
        await gate.waitForRead()

        XCTAssertTrue(store.isRefreshing)
        XCTAssertNotNil(store.loadState.pr, "a refresh never blanks the reading it is replacing")
        XCTAssertFalse(store.loadState.isLoading)
        await gate.open()
        await task.value

        XCTAssertFalse(store.isRefreshing)
    }

    /// A first read has nothing to keep, so it blocks behind the pane's
    /// skeleton rather than wearing the busy mark.
    @MainActor
    func testAFirstReadIsLoadingRatherThanRefreshing() async {
        let client = MockPRClient(response: .success(openPR()))
        let gate = ReadGate()
        client.viewGate = gate
        let store = PRStore(client: client)

        let task = Task { await store.fetch(projectID: "proj-1", sliceRef: "slice-1") }
        await gate.waitForRead()

        XCTAssertTrue(store.loadState.isLoading)
        XCTAssertFalse(store.isRefreshing)
        await gate.open()
        await task.value
    }

    /// A `gh` that failed one poll is a reading that did not happen: the poll
    /// carries on over the reading it kept, and recovers by itself.
    @MainActor
    func testAFailedPollLeavesSomethingWorthPollingOver() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertTrue(store.shouldPoll)

        client.setResponse(.failure)
        await store.refresh()

        XCTAssertTrue(store.shouldPoll, "a failed reading is not news that there is nothing left to watch")
    }

    @MainActor
    func testPRLoadStateAccessors() {
        let pr = openPR()
        XCTAssertEqual(PRLoadState.loaded(pr).pr, pr)
        XCTAssertNil(PRLoadState.loaded(pr).errorMessage)
        XCTAssertTrue(PRLoadState.loading.isLoading)
        XCTAssertFalse(PRLoadState.idle.isLoading)
        XCTAssertEqual(PRLoadState.failed("oops", previous: nil).errorMessage, "oops")
        XCTAssertNil(PRLoadState.failed("oops", previous: nil).pr)
        // A read that failed over a pull request already on screen keeps it:
        // see `PRLoadState`.
        XCTAssertEqual(PRLoadState.failed("oops", previous: pr).pr, pr)
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
    func testShouldPollIsTrueForAnOpenPullRequestWithNoChecksYet() async {
        // GitHub may not have started its first check when the PR is first
        // read: an empty list is no sign of a settled pull request.
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertTrue(store.shouldPoll)
    }

    @MainActor
    func testShouldPollIsTrueForAnOpenPullRequestWithEverythingPassing() async {
        let client = MockPRClient(response: .success(openPR(checks: [PRCheck(name: "build", state: "SUCCESS", link: "")])))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertTrue(store.shouldPoll)
    }

    @MainActor
    func testShouldPollIsFalseOnceClosed() async {
        let client = MockPRClient(response: .success(mergedPR(state: PRLifecycleState.closed)))
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

        // The pull request is merged between the first read and the poll's next one.
        client.setResponse(.success(mergedPR()))
        store.startPolling()

        await waitUntil { client.viewCallCount >= 2 && !store.shouldPoll }

        XCTAssertGreaterThanOrEqual(client.viewCallCount, 2)
        XCTAssertFalse(store.shouldPoll)
        store.stopPolling()
    }

    @MainActor
    func testStartPollingDoesNothingOnceMerged() async {
        let client = MockPRClient(response: .success(mergedPR()))
        let store = PRStore(client: client, pollIntervalNanoseconds: 5_000_000)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        store.startPolling()
        try? await Task.sleep(nanoseconds: 50_000_000)

        // Merged, so the loop should never have started reading again.
        XCTAssertEqual(client.viewCallCount, 1)
        store.stopPolling()
    }

    @MainActor
    func testPollingPicksUpChecksThatAppearAfterAnEmptyFirstReading() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client, pollIntervalNanoseconds: 5_000_000)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(store.loadState.pr?.checks.count, 0)

        client.setResponse(.success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        store.startPolling()
        await waitUntil { store.loadState.pr?.checks.count == 1 }

        XCTAssertEqual(store.loadState.pr?.checks.count, 1)
        XCTAssertTrue(store.shouldPoll, "still open, so still worth watching")
        store.stopPolling()
    }

    @MainActor
    func testACacheHitStillRefreshesInTheBackground() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")
        XCTAssertEqual(client.viewCallCount, 2)

        client.setResponse(.success(openPR(checks: [PRCheck(name: "lint", state: "IN_PROGRESS", link: "")])))
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(store.loadState.pr?.checks.count, 0, "the cached reading shows instantly")

        await waitUntil { store.loadState.pr?.checks.count == 1 }
        XCTAssertEqual(client.viewCallCount, 3)
        XCTAssertEqual(store.loadState.pr?.checks.count, 1, "the background read replaces the stale one")
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
    func testReviewersReadsWithoutEditingOrRereading() async throws {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        let nothing = try await store.reviewers()
        XCTAssertNil(nothing, "nothing fetched, nobody to ask about")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        let answer = try await store.reviewers()

        XCTAssertEqual(answer?.candidates, ["mona"])
        XCTAssertEqual(client.reviewerCalls.count, 1)
        XCTAssertEqual(client.reviewerCalls[0].add, [])
        XCTAssertEqual(client.viewCallCount, 1)
    }

    @MainActor
    func testEditReviewersThenRereads() async throws {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        let answer = try await store.editReviewers(add: ["hubot"], remove: ["octocat"])

        XCTAssertEqual(answer?.requested, ["hubot"])
        XCTAssertEqual(client.reviewerCalls[0].remove, ["octocat"])
        XCTAssertEqual(client.viewCallCount, 2, "an edit should trigger a reread")
    }

    @MainActor
    func testEditReviewersPropagatesFailureWithoutRereading() async {
        let client = MockPRClient(response: .success(openPR()))
        client.reviewersError = PRTestError()
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        do {
            try await store.editReviewers(add: ["hubot"])
            XCTFail("should have thrown")
        } catch {}
        XCTAssertEqual(client.viewCallCount, 1)
    }

    @MainActor
    func testASessionsPullRequestHasNoReviewersToEdit() async throws {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "https://x/pull/7", sessionID: "s1")

        let read = try await store.reviewers()
        let edited = try await store.editReviewers(add: ["hubot"])
        XCTAssertNil(read)
        XCTAssertNil(edited)
        XCTAssertTrue(client.reviewerCalls.isEmpty)
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

    // MARK: - Re-running and cancelling checks

    @MainActor
    func testRerunChecksSaysWhatNatDidThenRereads() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        await store.rerunChecks(.checks(["test"]), from: .rerun("test"))

        XCTAssertEqual(client.checksCalls, ["rerun checks([\"test\"])"])
        XCTAssertEqual(store.checksNotice, ChecksActionNotice(
            text: "Cancelled test and lint, then re-ran test, lint and build.", isError: false))
        XCTAssertNil(store.checksActionSource)
        XCTAssertEqual(client.viewCallCount, 2, "a re-run should trigger a reread")
    }

    @MainActor
    func testCancelChecksRefusalIsTheNotice() async {
        let client = MockPRClient(response: .success(openPR()))
        client.checksError = NatError.commandFailed("slice-checks-cancel: nothing to cancel")
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        await store.cancelChecks([], from: .cancelAll)

        XCTAssertEqual(store.checksNotice, ChecksActionNotice(text: "slice-checks-cancel: nothing to cancel", isError: true))
        XCTAssertEqual(client.viewCallCount, 2)
    }

    @MainActor
    func testChecksCallsNeedASlicesPullRequest() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.rerunChecks(.all, from: .rerunAll)
        await store.fetch(projectID: "proj-1", sliceRef: "https://x/pull/7", sessionID: "s1")
        await store.cancelChecks([], from: .cancelAll)
        XCTAssertEqual(client.checksCalls, [])
        XCTAssertNil(store.checksNotice)
    }

    @MainActor
    func testTheNoticeGoesWithAnotherSlicesPullRequest() async {
        let client = MockPRClient(response: .success(openPR()))
        let store = PRStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.cancelChecks(["test"], from: .cancel("test"))
        XCTAssertNotNil(store.checksNotice)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")
        XCTAssertNil(store.checksNotice)
        await store.cancelChecks(["test"], from: .cancel("test"))
        store.clear()
        XCTAssertNil(store.checksNotice)
    }
}

/// The PR section's Updated badge: the pull request's head as the user last
/// opened the section, against the head read now.
@MainActor
final class PRStoreUpdatedTests: XCTestCase {
    private func pr(head: String) -> PRDetail {
        PRDetail(
            number: 7, title: "T", body: "", state: "OPEN", isDraft: false, author: "a", baseRefName: "main",
            headRefName: "b", url: "https://github.test/o/r/pull/7", reviewDecision: "", mergeable: "MERGEABLE",
            mergeStateStatus: "CLEAN", headRefOid: head)
    }

    func testUpdatedOnceTheHeadHasMovedSinceTheSectionWasLastOpen() async {
        let seen = SeenMemory.inMemory()
        let client = MockPRClient(response: .success(pr(head: "aaa")))
        let store = PRStore(client: client, seen: seen)
        XCTAssertNil(store.badge(sliceID: "s"), "nothing read")
        await store.fetch(projectID: "p", sliceRef: "s")
        XCTAssertNil(store.badge(sliceID: "s"), "the first reading is the first look")

        client.setResponse(.success(pr(head: "bbb")))
        await store.refresh()
        XCTAssertEqual(store.badge(sliceID: "s"), .updated)
        XCTAssertNil(store.badge(sliceID: "other"), "another slice's section reads nothing off this pull request")
        XCTAssertEqual(
            PRStore(client: client, seen: seen).badge(sliceID: "s"), nil, "a store holding no reading says nothing")

        store.markSeen(sliceID: "other")
        XCTAssertEqual(store.badge(sliceID: "s"), .updated, "only its own section sees it")
        store.markSeen(sliceID: "s")
        XCTAssertNil(store.badge(sliceID: "s"))
        XCTAssertEqual(seen.snapshot(projectID: "p", sliceID: "s", .pr), [PRStore.seenHead: "bbb"])
        store.stopPolling()
    }

    func testAHeadNatDidNotSayTakesNoPart() async {
        let seen = SeenMemory.inMemory()
        let store = PRStore(client: MockPRClient(response: .success(pr(head: ""))), seen: seen)
        await store.fetch(projectID: "p", sliceRef: "s")
        XCTAssertNil(seen.snapshot(projectID: "p", sliceID: "s", .pr))
        XCTAssertNil(store.badge(sliceID: "s"))
        store.stopPolling()
    }

    func testASessionsPullRequestTakesNoPart() async {
        let seen = SeenMemory.inMemory()
        let store = PRStore(client: MockPRClient(response: .success(pr(head: "aaa"))), seen: seen)
        await store.fetch(projectID: "p", sliceRef: "https://github.test/o/r/pull/7", sessionID: "sess")
        XCTAssertNil(seen.snapshot(projectID: "p", sliceID: "https://github.test/o/r/pull/7", .pr))
        XCTAssertNil(store.badge(sliceID: "https://github.test/o/r/pull/7"))
        store.stopPolling()
    }

    func testPRHeadDecodesAndDefaultsEmpty() throws {
        let base = #""number":1,"title":"t","body":"","state":"OPEN","is_draft":false,"author":"a","base_ref_name":"main","head_ref_name":"b","url":"u","checks":[],"reviews":[],"comments":[],"review_decision":"","mergeable":"","merge_state_status":"""#
        XCTAssertEqual(try JSONDecoder().decode(PRDetail.self, from: Data("{\(base),\"head_ref_oid\":\"abc\"}".utf8)).headRefOid, "abc")
        XCTAssertEqual(try JSONDecoder().decode(PRDetail.self, from: Data("{\(base)}".utf8)).headRefOid, "")
    }
}
