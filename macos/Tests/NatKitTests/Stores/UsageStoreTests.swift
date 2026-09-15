import XCTest
@testable import NatKit

/// A `usage()` fake whose answer depends on how many times it has already
/// been called, so a test can watch a single store live through more than
/// one kind of reading (a success, then a failure, then another success)
/// without standing up a fresh store per reading.
final class SequencedUsageClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case reading(UsageReading)
        case failure(Error)
    }

    private let responses: [Response]
    private(set) var callCount = 0

    init(_ responses: [Response]) {
        self.responses = responses
    }

    func usage() async throws -> UsageReading {
        defer { callCount += 1 }
        switch responses[min(callCount, responses.count - 1)] {
        case .reading(let reading):
            return reading
        case .failure(let error):
            throw error
        }
    }

    func info(projectID: String) async throws -> ProjectInfo { throw NSError(domain: "test", code: -1) }
    func status() async throws -> [AgentStatus] { [] }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw NSError(domain: "test", code: -1) }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw NSError(domain: "test", code: -1) }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw NSError(domain: "test", code: -1) }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult { throw NSError(domain: "test", code: -1) }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw NSError(domain: "test", code: -1) }
    func agentKill(projectID: String, sliceRef: String) async throws { throw NSError(domain: "test", code: -1) }
    func agentKillWorkshop(projectID: String) async throws { throw NSError(domain: "test", code: -1) }
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

/// A `UsageCaching` held in memory, for tests that want to see what
/// `UsageStore` reads and writes without touching disk.
final class FakeUsageCache: UsageCaching, @unchecked Sendable {
    private(set) var reads = 0
    private(set) var writes: [UsageReading] = []
    var stored: UsageReading?

    init(stored: UsageReading? = nil) {
        self.stored = stored
    }

    func read() async -> UsageReading? {
        reads += 1
        return stored
    }

    func write(_ reading: UsageReading) async {
        writes.append(reading)
        stored = reading
    }
}

final class UsageStoreTests: XCTestCase {
    @MainActor
    func testStartLoadsTheCacheImmediately() async {
        let cached = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 20, resetsAt: Date()))
        let cache = FakeUsageCache(stored: cached)
        let client = SequencedUsageClient([.failure(TestError())])
        let store = UsageStore(client: client, cache: cache, refreshIntervalSeconds: 3600)
        defer { store.stop() }

        await store.start()

        // The cache's own reading survives a failed first probe.
        XCTAssertEqual(store.reading, cached)
        XCTAssertEqual(cache.reads, 1)
    }

    @MainActor
    func testStartProbesAndWritesTheFreshReading() async {
        let fresh = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 55, resetsAt: Date()))
        let cache = FakeUsageCache()
        let client = SequencedUsageClient([.reading(fresh)])
        let store = UsageStore(client: client, cache: cache, refreshIntervalSeconds: 3600)
        defer { store.stop() }

        await store.start()

        XCTAssertEqual(store.reading, fresh)
        XCTAssertEqual(cache.writes, [fresh])
    }

    @MainActor
    func testAnEmptyProbeIsNotWrittenOverTheCache() async {
        let cached = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 20, resetsAt: Date()))
        let cache = FakeUsageCache(stored: cached)
        let client = SequencedUsageClient([.reading(.empty)])
        let store = UsageStore(client: client, cache: cache, refreshIntervalSeconds: 3600)
        defer { store.stop() }

        await store.start()

        XCTAssertEqual(store.reading, cached)
        XCTAssertTrue(cache.writes.isEmpty)
    }

    @MainActor
    func testAFailedProbeKeepsTheLastReading() async {
        let first = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 30, resetsAt: Date()))
        let client = SequencedUsageClient([.reading(first), .failure(TestError())])
        let store = UsageStore(client: client, cache: FakeUsageCache(), refreshIntervalSeconds: 3600)
        defer { store.stop() }

        await store.start()
        XCTAssertEqual(store.reading, first)

        await store.refresh()
        XCTAssertEqual(store.reading, first)
    }

    @MainActor
    func testRefreshSkipsWhileAProbeIsAlreadyInFlight() async {
        let gate = Gate()
        let client = GatedUsageClient(gate: gate, reading: UsageReading(fiveHour: UsageRateLimit(usedPercentage: 1, resetsAt: Date())))
        let store = UsageStore(client: client, cache: FakeUsageCache(), refreshIntervalSeconds: 3600)
        defer { store.stop() }

        async let firstRefresh: Void = store.refresh()
        await gate.waitUntilAsked()

        // A second refresh while the first is still gated must not start a
        // second probe: the whole point of the guard is that two never race.
        await store.refresh()
        XCTAssertEqual(client.callCount, 1)

        await gate.open()
        await firstRefresh
        XCTAssertEqual(client.callCount, 1)
    }

    @MainActor
    func testStopPreventsTheNextTimerTick() async throws {
        let client = SequencedUsageClient([.reading(UsageReading(fiveHour: UsageRateLimit(usedPercentage: 1, resetsAt: Date())))])
        let store = UsageStore(client: client, cache: FakeUsageCache(), refreshIntervalSeconds: 0)

        await store.start()
        XCTAssertEqual(client.callCount, 1)

        store.stop()
        try await Task.sleep(nanoseconds: 300_000_000)
        // callCount may have ticked once more between start() arming the
        // timer and stop() cancelling it; what matters is it does not keep
        // climbing after stop() has returned.
        let afterStop = client.callCount
        try await Task.sleep(nanoseconds: 300_000_000)
        XCTAssertEqual(client.callCount, afterStop)
    }
}

/// A `usage()` fake that blocks on a `Gate` until told to answer, so a test
/// can observe that a second call while one is in flight never happens.
private final class GatedUsageClient: NatClientProtocol, @unchecked Sendable {
    private let gate: Gate
    private let reading: UsageReading
    private(set) var callCount = 0

    init(gate: Gate, reading: UsageReading) {
        self.gate = gate
        self.reading = reading
    }

    func usage() async throws -> UsageReading {
        callCount += 1
        await gate.markAsked()
        await gate.waitUntilOpen()
        return reading
    }

    func info(projectID: String) async throws -> ProjectInfo { throw NSError(domain: "test", code: -1) }
    func status() async throws -> [AgentStatus] { [] }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw NSError(domain: "test", code: -1) }
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff { throw NSError(domain: "test", code: -1) }
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw NSError(domain: "test", code: -1) }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult { throw NSError(domain: "test", code: -1) }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw NSError(domain: "test", code: -1) }
    func agentKill(projectID: String, sliceRef: String) async throws { throw NSError(domain: "test", code: -1) }
    func agentKillWorkshop(projectID: String) async throws { throw NSError(domain: "test", code: -1) }
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
