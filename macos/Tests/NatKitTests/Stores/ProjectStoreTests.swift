import XCTest
@testable import NatKit

// MARK: - Mock Client

struct TestError: Error {}

final class MockNatClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(ProjectInfo)
        case failure
    }

    /// Settable, so a test can have the same store read one plan and then
    /// another — which is what a refresh landing over a cached plan is.
    var response: Response
    private(set) var callCount = 0

    /// Held open by a test that wants to look at the store while a read is
    /// still in flight — which is the whole point of seeding from the cache.
    var gate: Gate?

    init(response: Response, gate: Gate? = nil) {
        self.response = response
        self.gate = gate
    }

    /// Whether each read asked nat to refresh the replica first, in order.
    private(set) var refreshFlags: [Bool] = []

    func info(projectID: String, refresh: Bool) async throws -> ProjectInfo {
        refreshFlags.append(refresh)
        return try await info(projectID: projectID)
    }

    func info(projectID: String) async throws -> ProjectInfo {
        callCount += 1
        if let gate {
            await gate.markAsked()
            await gate.waitUntilOpen()
        }
        switch response {
        case .success(let info):
            return info
        case .failure:
            throw TestError()
        }
    }

    func status() async throws -> [AgentStatus] {
        []
    }

    func usage() async throws -> UsageReading {
        .empty
    }

    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        throw TestError()
    }

    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        throw TestError()
    }

    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        throw TestError()
    }

    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw TestError()
    }

    func agentSend(projectID: String, sliceRef: String, text: String) async throws {
        throw TestError()
    }

    func agentKill(projectID: String, sliceRef: String) async throws {
        throw TestError()
    }

    func agentKillWorkshop(projectID: String) async throws {
        throw TestError()
    }

    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult {
        throw TestError()
    }

    func sliceApprove(projectID: String, sliceRef: String) async throws -> String {
        throw TestError()
    }

    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw TestError()
    }

    func prStatus(projectID: String) async throws -> PRStatusDoc { throw TestError() }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        throw TestError()
    }

    func prMerge(projectID: String, sliceRef: String) async throws {
        throw TestError()
    }

    func prComment(projectID: String, sliceRef: String, body: String) async throws {
        throw TestError()
    }

    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult {
        throw TestError()
    }

    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        throw TestError()
    }

    func configShow() async throws -> ConfigDoc { throw TestError() }
    func configSet(key: String, value: String) async throws { throw TestError() }
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
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

        XCTAssertEqual(store.projectID, "proj-1")
        XCTAssertEqual(store.state, .idle)
    }

    @MainActor
    func testLoadSuccess() async {
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

        await store.load()

        XCTAssertEqual(mockClient.callCount, 1)
        if case .loaded(let info) = store.state {
            XCTAssertEqual(info.project.name, "Test")
        } else {
            XCTFail("Expected loaded state")
        }
    }

    /// The first read takes the replica as it stands; every one after it may
    /// bring a stale one up to date first — and a first read that failed
    /// leaves the next one still the first.
    @MainActor
    func testOnlyReadsAfterTheFirstAskForARefresh() async {
        let mockClient = MockNatClient(response: .failure)
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

        await store.load()
        mockClient.response = .success(testProjectInfo)
        await store.load()
        await store.refresh()

        XCTAssertEqual(mockClient.refreshFlags, [false, false, true])
    }

    @MainActor
    func testLoadFailure() async {
        let mockClient = MockNatClient(response: .failure)
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

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
        // A failure on a store that has already loaded successfully must
        // report the failure over the plan it already has, rather than
        // clearing it the way a first, cold failure does.
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

        await store.load()
        XCTAssertEqual(store.state, .loaded(testProjectInfo))

        mockClient.response = .failure
        await store.refresh()

        if case .failed(_, let previous) = store.state {
            XCTAssertEqual(previous, testProjectInfo)
        } else {
            XCTFail("Expected failed state with the previous load kept")
        }
    }

    /// A load asked for while a read is in flight is owed, not dropped: that
    /// read may have begun before what the caller wants to see, so one more
    /// follows it — one for every request made meanwhile — and each caller
    /// returns only once a read begun after its call has landed.
    @MainActor
    func testLoadsDuringAReadAreOwedOneReadAfterIt() async {
        let gate = Gate()
        let before = ProjectInfo(project: testProject, milestones: [], slices: [])
        let mockClient = MockNatClient(response: .success(before), gate: gate)
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

        let first = Task { await store.load() }
        await gate.waitUntilAsked()
        // The write lands while the first read is in flight; both callers
        // after it are owed a read that sees it.
        let after = ProjectInfo(
            project: testProject, milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")], slices: [])
        mockClient.response = .success(after)
        let second = Task { await store.load(.replica) }
        let third = Task { await store.load(.replica) }
        while store.owedLoads < 2 { await Task.yield() }
        await gate.open()
        await first.value
        await second.value
        await third.value

        XCTAssertEqual(mockClient.callCount, 2, "the two requests made meanwhile share one read")
        XCTAssertEqual(store.state, .loaded(after))
    }

    /// A replica read never asks nat to pull; an owed read pulls if any
    /// request it answers asked for one.
    @MainActor
    func testAReplicaReadNeverPullsAndAnOwedReadPullsIfAnyRequestDid() async {
        let gate = Gate()
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())
        await store.load()
        await store.load(.replica)
        XCTAssertEqual(mockClient.refreshFlags, [false, false])

        mockClient.gate = gate
        let held = Task { await store.load(.replica) }
        await gate.waitUntilAsked()
        let replica = Task { await store.load(.replica) }
        let pull = Task { await store.load(.pull) }
        while store.owedLoads < 2 { await Task.yield() }
        await gate.open()
        await held.value
        await replica.value
        await pull.value

        XCTAssertEqual(mockClient.refreshFlags, [false, false, false, true])
    }

    @MainActor
    func testRefresh() async {
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: FakePlanCache())

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

    // MARK: - The disk cache

    @MainActor
    func testLoadSeedsFromTheCacheAndThenRefreshesInPlace() async {
        let cached = ProjectInfo(
            project: Project(id: "proj-1", name: "From disk", conventions: ""),
            milestones: [],
            slices: []
        )
        let cache = FakePlanCache(stored: ["proj-1": cached])
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: cache)

        await store.load()

        // The cached plan was asked for, and the fresh read replaced it —
        // never a `.loading` in between, which is what would blank the board.
        XCTAssertEqual(cache.reads, ["proj-1"])
        XCTAssertEqual(store.state, .loaded(testProjectInfo))
    }

    @MainActor
    func testSeededPlanIsOnScreenWhileTheFreshReadIsInFlight() async {
        let cached = ProjectInfo(
            project: Project(id: "proj-1", name: "From disk", conventions: ""),
            milestones: [],
            slices: []
        )
        let cache = FakePlanCache(stored: ["proj-1": cached])
        let gate = Gate()
        let client = MockNatClient(response: .success(testProjectInfo), gate: gate)
        let store = ProjectStore(projectID: "proj-1", client: client, cache: cache)

        let load = Task { await store.load() }
        await gate.waitUntilAsked()

        XCTAssertEqual(store.state, .loaded(cached))
        XCTAssertFalse(store.state.isLoading)

        await gate.open()
        await load.value
        XCTAssertEqual(store.state, .loaded(testProjectInfo))
    }

    @MainActor
    func testSeededPlanSurvivesAFailedRefresh() async {
        let cached = ProjectInfo(
            project: Project(id: "proj-1", name: "From disk", conventions: ""),
            milestones: [],
            slices: []
        )
        let cache = FakePlanCache(stored: ["proj-1": cached])
        let store = ProjectStore(
            projectID: "proj-1",
            client: MockNatClient(response: .failure),
            cache: cache
        )

        await store.load()

        // The stale-keeping LoadState semantics reach a disk-seeded plan:
        // the failure is reported over the cached plan rather than instead
        // of it.
        XCTAssertNotNil(store.state.errorMessage)
        XCTAssertEqual(store.state.projectInfo, cached)
    }

    @MainActor
    func testEmptyCacheLoadsCold() async {
        let cache = FakePlanCache()
        let store = ProjectStore(
            projectID: "proj-1",
            client: MockNatClient(response: .failure),
            cache: cache
        )

        await store.load()

        // Nothing to seed from is exactly the load there was before there
        // was a cache: `.loading`, and then the failure with no previous.
        XCTAssertEqual(cache.reads, ["proj-1"])
        XCTAssertNil(store.state.projectInfo)
        if case .failed(_, let previous) = store.state {
            XCTAssertNil(previous)
        } else {
            XCTFail("Expected failed state")
        }
    }

    @MainActor
    func testEveryReadThatLandsIsWrittenToTheCache() async {
        let cache = FakePlanCache()
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: cache)

        await store.load()
        let refreshed = ProjectInfo(
            project: Project(id: "proj-1", name: "Refreshed", conventions: ""),
            milestones: [],
            slices: []
        )
        mockClient.response = .success(refreshed)
        await store.refresh()

        XCTAssertEqual(cache.writes.count, 2)
        XCTAssertEqual(cache.writes.map(\.projectID), ["proj-1", "proj-1"])
        XCTAssertEqual(cache.writes.last?.info, refreshed)
    }

    @MainActor
    func testAFailedReadWritesNothing() async {
        let cache = FakePlanCache()
        let store = ProjectStore(
            projectID: "proj-1",
            client: MockNatClient(response: .failure),
            cache: cache
        )

        await store.load()

        XCTAssertTrue(cache.writes.isEmpty)
    }

    @MainActor
    func testTheCacheIsAskedOnceOnly() async {
        let cache = FakePlanCache()
        let mockClient = MockNatClient(response: .success(testProjectInfo))
        let store = ProjectStore(projectID: "proj-1", client: mockClient, cache: cache)

        await store.load()
        await store.refresh()
        await store.refresh()

        // Once the first read has landed, the plan in hand is fresher than
        // anything on disk.
        XCTAssertEqual(cache.reads, ["proj-1"])
    }
}
