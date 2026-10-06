import XCTest
@testable import NatKit
@testable import NatFixtures

/// Holds every wait the store asks for until the test lets it go, recording
/// how long each asked to be — so a test says when a tick or a settle read's
/// delay is up instead of waiting it out.
private final class Sleeps: @unchecked Sendable {
    private let lock = NSLock()
    private var waiting: [CheckedContinuation<Void, Never>] = []
    private var asked: [Duration] = []

    func sleep(_ duration: Duration) async {
        await withCheckedContinuation { continuation in
            lock.withLock {
                asked.append(duration)
                waiting.append(continuation)
            }
        }
    }

    /// Every wait asked for, in order.
    var requested: [Duration] { lock.withLock { asked } }

    /// How many waits are being held.
    var held: Int { lock.withLock { waiting.count } }

    /// Lets every held wait end.
    func release() {
        let woken = lock.withLock {
            let all = waiting
            waiting = []
            return all
        }
        woken.forEach { $0.resume() }
    }
}

/// `GitHubReadingStore`: the one batched reading, on a tick and after an
/// action, never two at once, handed on whole.
@MainActor
final class GitHubReadingStoreTests: XCTestCase {
    private let doc = PRStatusDoc(slices: [
        PRStatusSlice(sliceID: "s", name: "S", pr: "u", readiness: PRStatusSlice.awaitingReview),
    ])

    private func store(
        _ client: FixtureNatClient, request: GitHubReadingStore.Request? = .init(projectIDs: ["p-1", "p-2"]),
        tick: Duration? = nil, sleeps: Sleeps = Sleeps(), delivered: @escaping @MainActor (GitHubReading) -> Void = { _ in }
    ) -> GitHubReadingStore {
        GitHubReadingStore(
            client: client, request: { request }, deliver: { delivered($0) }, tick: tick,
            sleep: { await sleeps.sleep($0) })
    }

    /// Waits for what a task the test cannot await is expected to do — on the
    /// clock, since a held `pr-status` lets go on a sleep of its own.
    private func waitUntil(_ condition: () -> Bool, file: StaticString = #filePath, line: UInt = #line) async {
        let deadline = ContinuousClock.now + .seconds(10)
        while !condition(), ContinuousClock.now < deadline { try? await Task.sleep(for: .milliseconds(2)) }
        XCTAssertTrue(condition(), "timed out", file: file, line: line)
    }

    /// A reading names every project of the request in one run, and is handed
    /// on whole — the detail with it.
    func testAReadingIsOneRunHandedOnWhole() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc, "p-2": doc])
        var got: [GitHubReading] = []
        let reading = store(
            client, request: .init(projectIDs: ["p-1", "p-2"], detail: "https://github.test/o/r/pull/1"),
            delivered: { got.append($0) })

        await reading.read()

        XCTAssertEqual(client.prStatusRuns, ["p-1,p-2 https://github.test/o/r/pull/1"])
        XCTAssertEqual(got.count, 1)
        XCTAssertEqual(got.first?.projects, ["p-1": doc, "p-2": doc])
        XCTAssertEqual(got.first?.detail, Fixtures.prGreen)
    }

    /// The rate limit a reading carries is kept; one that carries none keeps
    /// the last.
    func testTheRateLimitIsKept() async {
        let limit = GitHubRateLimit(limit: 5000, remaining: 12, resetAt: Date(timeIntervalSince1970: 0))
        let client = FixtureNatClient()
        client.setRateLimit(limit)
        let reading = store(client)
        await reading.read()
        XCTAssertEqual(reading.rateLimit, limit)
        client.setRateLimit(nil)
        await reading.read()
        XCTAssertEqual(reading.rateLimit, limit)
    }

    /// Nothing to ask is no run at all; a run that fails hands nothing on.
    func testNothingAskedOrAFailedRunChangesNothing() async {
        let client = FixtureNatClient()
        var delivered = 0
        await store(client, request: nil).read()
        await store(client, request: .init(projectIDs: [])).read()
        XCTAssertEqual(client.prStatusRuns, [])

        client.setPRStatus(nil, forProject: "p-1")
        await store(client, delivered: { _ in delivered += 1 }).read()
        XCTAssertEqual(client.prStatusRuns.count, 1)
        XCTAssertEqual(delivered, 0)
    }

    /// Never two in flight: a tick that finds one running leaves it to finish.
    func testATickWhileOneRunsReadsNothing() async {
        let client = FixtureNatClient()
        client.holdPRStatus()
        let reading = store(client)
        let first = Task { await reading.read() }
        await waitUntil { client.prStatusRuns.count == 1 }

        await reading.read()
        XCTAssertEqual(client.prStatusRuns.count, 1)
        client.releasePRStatus()
        await first.value
    }

    /// The tick reads once per interval, whatever else is going on.
    func testTheTickReadsOnItsInterval() async {
        let client = FixtureNatClient()
        let sleeps = Sleeps()
        let reading = store(client, tick: .seconds(30), sleeps: sleeps)
        reading.start()
        await waitUntil { sleeps.held == 1 }
        XCTAssertEqual(sleeps.requested, [.seconds(30)])
        XCTAssertEqual(client.prStatusRuns.count, 0)

        sleeps.release()
        await waitUntil { client.prStatusRuns.count == 1 && sleeps.held == 1 }
        XCTAssertEqual(sleeps.requested, [.seconds(30), .seconds(30)])
        reading.stop()
    }

    /// A settle read waits five seconds, every action inside that window
    /// folds into it — one read for two approves — and the tick restarts
    /// from it.
    func testASettleReadWaitsFiveSecondsAndFoldsWhatComesInsideIt() async {
        let client = FixtureNatClient()
        let sleeps = Sleeps()
        let reading = GitHubReadingStore(
            client: client, request: { .init(projectIDs: ["p-1"]) }, deliver: { _ in }, tick: .seconds(30),
            sleep: { await sleeps.sleep($0) })
        reading.start()
        await waitUntil { sleeps.held == 1 }

        reading.scheduleSettle()
        reading.scheduleSettle()
        XCTAssertTrue(reading.isSettlePending)
        await waitUntil { sleeps.held == 2 }
        XCTAssertEqual(sleeps.requested, [.seconds(30), .seconds(5)])

        sleeps.release()
        await waitUntil { client.prStatusRuns.count == 1 && sleeps.held == 1 }
        XCTAssertFalse(reading.isSettlePending)
        XCTAssertEqual(client.prStatusRuns.count, 1, "the cancelled tick read nothing; the two approves read once")
        XCTAssertEqual(sleeps.requested.last, .seconds(30), "the tick restarts from the settle read")
        reading.stop()
    }

    /// A settle read whose wait ends while a reading runs waits for it, then
    /// reads: what the action changed is never left to the next tick.
    func testASettleReadWaitsForOneInFlight() async {
        let client = FixtureNatClient()
        let sleeps = Sleeps()
        let reading = store(client, sleeps: sleeps)
        client.holdPRStatus()
        let first = Task { await reading.read() }
        await waitUntil { client.prStatusRuns.count == 1 }

        reading.scheduleSettle()
        await waitUntil { sleeps.held == 1 }
        sleeps.release()
        for _ in 0..<50 { await Task.yield() }
        XCTAssertEqual(client.prStatusRuns.count, 1, "never two at once")

        client.releasePRStatus()
        await first.value
        await waitUntil { client.prStatusRuns.count == 2 }
    }

    /// Stopping drops the tick and a pending settle read.
    func testStopDropsEverythingPending() async {
        let client = FixtureNatClient()
        let sleeps = Sleeps()
        let reading = store(client, tick: .seconds(30), sleeps: sleeps)
        reading.start()
        reading.scheduleSettle()
        await waitUntil { sleeps.held >= 1 }
        reading.stop()
        XCTAssertFalse(reading.isSettlePending)
        sleeps.release()
        for _ in 0..<50 { await Task.yield() }
        XCTAssertEqual(client.prStatusRuns, [])
    }

    /// The tick sleeps for what the last reading's budget asked for —
    /// `poll_after_seconds` — and for its own interval again once a reading
    /// says nothing of it.
    func testTheTickSleepsForWhatTheReadingSays() async {
        let client = FixtureNatClient()
        client.setRateLimit(GitHubRateLimit(
            limit: 5000, remaining: 412, resetAt: Date(timeIntervalSince1970: 0), throttled: true, pollAfterSeconds: 300))
        let sleeps = Sleeps()
        let reading = store(client, tick: .seconds(30), sleeps: sleeps)
        reading.start()
        await waitUntil { sleeps.held == 1 }
        sleeps.release()
        await waitUntil { client.prStatusRuns.count == 1 && sleeps.held == 1 }
        XCTAssertEqual(sleeps.requested, [.seconds(30), .seconds(300)])

        client.setRateLimit(nil)
        sleeps.release()
        await waitUntil { client.prStatusRuns.count == 2 && sleeps.held == 1 }
        XCTAssertEqual(sleeps.requested.last, .seconds(30))
        reading.stop()
    }

    /// The tick's reads are polls; a settle read after an action runs past
    /// nat's throttle and pause (`--settle`), one for a plan that loaded does
    /// not — unless an action folds into it.
    func testASettleReadAfterAnActionGoesThroughThePause() async {
        let client = FixtureNatClient()
        client.setRateLimit(GitHubRateLimit(
            limit: 5000, remaining: 0, resetAt: Date(timeIntervalSince1970: 0),
            pausedUntil: Date(timeIntervalSince1970: 0), pollAfterSeconds: 2760))
        let reading = GitHubReadingStore(client: client, request: { .init(projectIDs: ["p-1"]) }, deliver: { _ in },
                                         settleDelay: .zero)
        await reading.read()
        reading.scheduleSettle(afterAction: false)
        await reading.idle()
        reading.actionRan()
        await reading.idle()
        reading.scheduleSettle(afterAction: false)
        reading.scheduleSettle()
        await reading.idle()
        XCTAssertEqual(client.prStatusKinds, ["poll", "poll", "settle", "settle"])
    }

    /// gnat's own spend this session: every reading's cost, one per action,
    /// and the readings and actions counted; a failed reading counts nothing.
    /// The launch time is read once, off the clock it is given.
    func testTheSessionTallyAddsEachReadingsCostAndOnePerAction() async {
        let client = FixtureNatClient()
        client.setRateLimit(GitHubRateLimit(limit: 5000, remaining: 4000, resetAt: Date(timeIntervalSince1970: 0), cost: 2))
        let launched = Date(timeIntervalSince1970: 1_000)
        let reading = GitHubReadingStore(client: client, request: { .init(projectIDs: ["p-1"]) }, deliver: { _ in },
                                         settleDelay: .zero, now: { launched })
        XCTAssertEqual(reading.launchedAt, launched)
        await reading.read()
        await reading.read()
        reading.actionRan()
        await reading.idle()
        client.setPRStatus(nil, forProject: "p-1")
        await reading.read()
        XCTAssertEqual(reading.sessionReadings, 3, "two reads and the settle read after the action")
        XCTAssertEqual(reading.sessionActions, 1)
        XCTAssertEqual(reading.sessionPoints, 3 * 2 + 1)
    }

    /// nat's answer decodes for one project named — its doc at the top — and
    /// for several, keyed by ID, the rate limit and detail beside them.
    func testTheAnswerDecodesInBothShapes() throws {
        let slice = #"{"slice_id":"s","name":"S","pr":"u","readiness":"awaiting review","conflicting":false}"#
        let limit = #""rate_limit":{"limit":5000,"remaining":4321,"reset_at":"2026-10-06T13:00:00Z"}"#
        let one = try GitHubReading.decode(
            Data(#"{"slices":[\#(slice)],"branches":[],"sessions":[],\#(limit)}"#.utf8), projectIDs: ["p-1"])
        XCTAssertEqual(one.projects["p-1"]?.slices.map(\.sliceID), ["s"])
        XCTAssertEqual(one.rateLimit?.remaining, 4321)
        XCTAssertNil(one.detail)

        let many = try GitHubReading.decode(Data(#"""
            {"projects":{"p-1":{"slices":[\#(slice)],"branches":[]},"p-2":{"slices":[],"branches":[]}},\#(limit)}
            """#.utf8), projectIDs: ["p-1", "p-2"])
        XCTAssertEqual(Set(many.projects.keys), ["p-1", "p-2"])
        XCTAssertEqual(many.rateLimit?.limit, 5000)

        XCTAssertThrowsError(try GitHubReading.decode(
            Data(#"{"slices":[],"rate_limit":{"limit":1,"remaining":1,"reset_at":"soon"}}"#.utf8), projectIDs: ["p"]))
    }
}
