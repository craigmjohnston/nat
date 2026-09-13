import XCTest
@testable import NatKit

// MARK: - Mock Client for ActivityStore

/// Not final: `PlanningAgentAppearsClient` in `AppModelTests` overrides
/// `status()` to answer differently on the reading a launch kicks off.
class MockActivityClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case agents([AgentStatus])
        case failure(Error)
    }

    private let response: Response
    private(set) var callCount = 0

    init(response: Response) {
        self.response = response
    }

    func info(projectID: String) async throws -> ProjectInfo {
        throw NSError(domain: "test", code: -1)
    }

    func status() async throws -> [AgentStatus] {
        callCount += 1
        switch response {
        case .agents(let statuses):
            return statuses
        case .failure(let error):
            throw error
        }
    }

    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        throw NSError(domain: "test", code: -1)
    }

    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        throw NSError(domain: "test", code: -1)
    }

    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        throw NSError(domain: "test", code: -1)
    }

    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw NSError(domain: "test", code: -1)
    }

    func agentSend(projectID: String, sliceRef: String, text: String) async throws {
        throw NSError(domain: "test", code: -1)
    }

    func agentKill(projectID: String, sliceRef: String) async throws {
        throw NSError(domain: "test", code: -1)
    }

    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult {
        throw NSError(domain: "test", code: -1)
    }

    func sliceApprove(projectID: String, sliceRef: String) async throws -> String {
        throw NSError(domain: "test", code: -1)
    }

    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw NSError(domain: "test", code: -1)
    }

    func prStatus(projectID: String) async throws -> PRStatusDoc {
        throw NSError(domain: "test", code: -1)
    }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        throw NSError(domain: "test", code: -1)
    }

    func prMerge(projectID: String, sliceRef: String) async throws {
        throw NSError(domain: "test", code: -1)
    }

    func prComment(projectID: String, sliceRef: String, body: String) async throws {
        throw NSError(domain: "test", code: -1)
    }

    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult {
        throw NSError(domain: "test", code: -1)
    }

    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        throw NSError(domain: "test", code: -1)
    }

    func configShow() async throws -> ConfigDoc {
        throw NSError(domain: "test", code: -1)
    }

    func configSet(key: String, value: String) async throws {
        throw NSError(domain: "test", code: -1)
    }
}

/// A `status()` fake whose answer depends on how many times it has already
/// been called, so a test can observe a single store living through more
/// than one kind of reading (a success followed by a failure, an empty
/// reading followed by a non-empty one) rather than standing up a fresh
/// store per reading and asserting nothing about the transition between them.
final class SequencedActivityClient: NatClientProtocol, @unchecked Sendable {
    private let responses: [MockActivityClient.Response]
    private(set) var callCount = 0

    init(_ responses: [MockActivityClient.Response]) {
        self.responses = responses
    }

    func info(projectID: String) async throws -> ProjectInfo { throw NSError(domain: "test", code: -1) }

    func status() async throws -> [AgentStatus] {
        defer { callCount += 1 }
        let response = responses[min(callCount, responses.count - 1)]
        switch response {
        case .agents(let statuses):
            return statuses
        case .failure(let error):
            throw error
        }
    }

    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw NSError(domain: "test", code: -1) }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw NSError(domain: "test", code: -1) }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw NSError(domain: "test", code: -1) }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult { throw NSError(domain: "test", code: -1) }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw NSError(domain: "test", code: -1) }
    func agentKill(projectID: String, sliceRef: String) async throws { throw NSError(domain: "test", code: -1) }
    func sliceStatus(projectID: String, sliceRef: String) async throws -> SliceStatusResult { throw NSError(domain: "test", code: -1) }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { throw NSError(domain: "test", code: -1) }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult { throw NSError(domain: "test", code: -1) }
    func prStatus(projectID: String) async throws -> PRStatusDoc { throw NSError(domain: "test", code: -1) }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail { throw NSError(domain: "test", code: -1) }
    func prMerge(projectID: String, sliceRef: String) async throws { throw NSError(domain: "test", code: -1) }
    func prComment(projectID: String, sliceRef: String, body: String) async throws { throw NSError(domain: "test", code: -1) }
    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult { throw NSError(domain: "test", code: -1) }
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult { throw NSError(domain: "test", code: -1) }
    func configShow() async throws -> ConfigDoc { throw NSError(domain: "test", code: -1) }
    func configSet(key: String, value: String) async throws { throw NSError(domain: "test", code: -1) }
}

// MARK: - ActivityStore Tests

final class ActivityStoreTests: XCTestCase {
    @MainActor
    func testInitialState() {
        let client = MockActivityClient(response: .agents([]))
        let store = ActivityStore(client: client)

        XCTAssertEqual(store.agents, [:])
        XCTAssertEqual(store.firstSeen, [:])
    }

    // MARK: - First-seen tracking

    func testMergeFirstSeenStampsANewAgent() {
        let now = Date(timeIntervalSince1970: 1_000)
        let merged = ActivityStore.mergeFirstSeen(existing: [:], sliceIDs: ["slice-1"], now: now)

        XCTAssertEqual(merged, ["slice-1": now])
    }

    func testMergeFirstSeenKeepsAnExistingStamp() {
        let earlier = Date(timeIntervalSince1970: 1_000)
        let now = Date(timeIntervalSince1970: 2_000)
        let merged = ActivityStore.mergeFirstSeen(
            existing: ["slice-1": earlier], sliceIDs: ["slice-1", "slice-2"], now: now
        )

        XCTAssertEqual(merged, ["slice-1": earlier, "slice-2": now])
    }

    func testMergeFirstSeenDropsAGoneAgent() {
        let earlier = Date(timeIntervalSince1970: 1_000)
        let merged = ActivityStore.mergeFirstSeen(
            existing: ["slice-1": earlier], sliceIDs: [] as [String], now: Date(timeIntervalSince1970: 2_000)
        )

        XCTAssertEqual(merged, [:])
    }

    @MainActor
    func testPollStampsFirstSeen() async {
        let status = AgentStatus(sliceID: "slice-1", session: "nat-abc123", activity: .working)
        let client = MockActivityClient(response: .agents([status]))
        let pinned = Date(timeIntervalSince1970: 42)
        let store = ActivityStore(client: client, now: { pinned })

        store.kick()
        defer { store.stop() }
        try? await Task.sleep(nanoseconds: 100_000_000)

        XCTAssertEqual(store.firstSeen, ["slice-1": pinned])
    }

    @MainActor
    func testPollUpdatesAgents() async {
        let status = AgentStatus(sliceID: "slice-1", session: "nat-abc123", activity: .working)
        let client = MockActivityClient(response: .agents([status]))
        let store = ActivityStore(client: client)

        store.kick()
        defer { store.stop() }
        try? await Task.sleep(nanoseconds: 100_000_000) // 0.1 seconds

        XCTAssertEqual(store.agents.count, 1)
        XCTAssertEqual(store.agents["slice-1"]?.activity, .working)
    }

    @MainActor
    func testPollStopsWhenNoAgents() async {
        let client = MockActivityClient(response: .agents([]))
        let store = ActivityStore(client: client)

        store.kick()
        try? await Task.sleep(nanoseconds: 100_000_000) // 0.1 seconds

        XCTAssertEqual(store.agents, [:])
    }

    @MainActor
    func testKickReArmsPolling() async throws {
        // While a poll with agents is still live, kicking again must not
        // trigger a second read: it is asleep for its 2-second interval.
        let status = AgentStatus(sliceID: "slice-1", session: "nat-abc123", activity: .waiting)
        let liveClient = MockActivityClient(response: .agents([status]))
        let liveStore = ActivityStore(client: liveClient)

        liveStore.kick()
        try await waitUntil { liveClient.callCount == 1 }
        liveStore.kick()
        try await Task.sleep(nanoseconds: 200_000_000)
        XCTAssertEqual(liveClient.callCount, 1)
        liveStore.stop()

        // Once a reading with no agents has stopped the loop, kicking again
        // must genuinely start a new one rather than staying stopped.
        let emptyClient = MockActivityClient(response: .agents([]))
        let emptyStore = ActivityStore(client: emptyClient)

        emptyStore.kick()
        try await waitUntil { emptyClient.callCount == 1 }
        emptyStore.kick()
        try await waitUntil { emptyClient.callCount == 2 }
        XCTAssertEqual(emptyClient.callCount, 2)
    }

    @MainActor
    func testFailedReadingKeepsPreviousState() async throws {
        // A failure on a store that has already loaded agents must keep
        // what it last saw rather than clearing it.
        let status = AgentStatus(sliceID: "slice-1", session: "nat-abc123", activity: .working)
        let client = SequencedActivityClient([.agents([status]), .failure(TestError())])
        let store = ActivityStore(client: client)

        store.kick()
        defer { store.stop() }
        try await waitUntil { client.callCount == 1 }
        XCTAssertEqual(store.agents.count, 1)

        try await waitUntil(timeout: 4) { client.callCount == 2 }
        XCTAssertEqual(store.agents.count, 1)
        XCTAssertEqual(store.agents["slice-1"]?.activity, .working)
    }

    @MainActor
    func testFailingReadsWithNoAgentsStopThePoll() async {
        // A client that only ever fails, on a store that knows of no agents,
        // stops the loop rather than retrying (and logging) every two seconds
        // forever. That the loop has genuinely ended is observable through
        // kick(), which re-arms a stopped loop and is a no-op on a live one:
        // the second kick produces a second read only because the first
        // loop ended.
        let client = MockActivityClient(response: .failure(TestError()))
        let store = ActivityStore(client: client)

        store.kick()
        defer { store.stop() }
        try? await Task.sleep(nanoseconds: 100_000_000)
        XCTAssertEqual(client.callCount, 1)

        store.kick()
        try? await Task.sleep(nanoseconds: 100_000_000)
        XCTAssertEqual(client.callCount, 2)
    }

    @MainActor
    func testStopClearsPolling() async throws {
        // A store whose agent is still live would poll again after 2 seconds;
        // stop() must prevent that next read from ever happening.
        let status = AgentStatus(sliceID: "slice-1", session: "nat-abc123", activity: .working)
        let client = MockActivityClient(response: .agents([status]))
        let store = ActivityStore(client: client)

        store.kick()
        try await waitUntil { client.callCount == 1 }

        store.stop()

        try await Task.sleep(nanoseconds: 2_500_000_000)
        XCTAssertEqual(client.callCount, 1)
    }
}

/// Polls `condition` until it is true or `timeout` elapses, so a test waits
/// exactly as long as the async loop under test actually takes rather than a
/// fixed guess that is either flaky (too short) or slow (too long).
@MainActor
private func waitUntil(
    timeout: TimeInterval = 2,
    _ condition: () -> Bool
) async throws {
    let deadline = Date().addingTimeInterval(timeout)
    while !condition() {
        if Date() >= deadline {
            XCTFail("condition not met within \(timeout)s")
            return
        }
        try await Task.sleep(nanoseconds: 20_000_000)
    }
}
