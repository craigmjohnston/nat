import XCTest
@testable import NatKit
@testable import NatFixtures

/// `PRStatusStore`: one `pr-status` reading per project, replaced only by a
/// newer reading of that project, kept through a failed one, and cached.
@MainActor
final class PRStatusStoreTests: XCTestCase {
    private func doc(_ slices: PRStatusSlice...) -> PRStatusDoc { PRStatusDoc(slices: slices) }

    private let red = PRStatusSlice(
        sliceID: "s-red", name: "A", pr: "u", readiness: PRStatusSlice.checksFailing,
        checks: PRStatusChecks(verdict: "failing", failing: [
            PRStatusCheck(name: "test", url: "https://ci/1"), PRStatusCheck(name: "lint", url: "https://ci/2"),
        ]))
    private let conflicting = PRStatusSlice(
        sliceID: "s-x", name: "B", pr: "u", readiness: PRStatusSlice.awaitingReview, conflicting: true, base: "main")

    func testEachProjectKeepsItsOwnReading() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(red), "p-2": doc(conflicting)])
        let store = PRStatusStore(client: client, cache: FakePlanCache())
        await store.update(projectID: "p-1")
        await store.update(projectID: "p-2")

        XCTAssertEqual(store.reading(projectID: "p-1").failingChecks, ["s-red": ["test", "lint"]])
        XCTAssertEqual(store.reading(projectID: "p-1").conflicts, [:])
        XCTAssertEqual(store.reading(projectID: "p-2").conflicts, ["s-x": BranchConflict(base: "main")])
        XCTAssertEqual(store.reading(projectID: "never-read"), .empty)
        XCTAssertEqual(store.marks, [
            "s-red": PRMarks(failingChecks: ["test", "lint"]),
            "s-x": PRMarks(conflict: BranchConflict(base: "main")),
        ])
    }

    /// A reading that fails changes nothing; a newer one that no longer says
    /// a mark takes it away.
    func testAReadingIsReplacedOnlyByANewerOne() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(red, conflicting)])
        let store = PRStatusStore(client: client, cache: FakePlanCache())
        await store.update(projectID: "p-1")

        client.setPRStatus(nil, forProject: "p-1")
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.marks.count, 2, "a reading that failed leaves the last standing")

        client.setPRStatus(doc(
            PRStatusSlice(sliceID: "s-red", name: "A", pr: "u", readiness: PRStatusSlice.readyToMerge),
            PRStatusSlice(sliceID: "s-x", name: "B", pr: "u", readiness: "unread")), forProject: "p-1")
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.marks, [:])
        XCTAssertEqual(store.reading(projectID: "p-1").readiness, ["s-red": PRStatusSlice.readyToMerge])
    }

    func testEveryReadingThatLandsIsCachedAndRestoredBeforeAnyFresherOne() async {
        let cache = FakePlanCache()
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(red)])
        let first = PRStatusStore(client: client, cache: cache)
        await first.update(projectID: "p-1")
        XCTAssertEqual(cache.storedPRStatus["p-1"], doc(red))

        // A launch: the cached reading is up before any fresh one.
        let next = PRStatusStore(client: client, cache: cache)
        await next.restore(projectID: "p-1")
        XCTAssertEqual(next.marks, ["s-red": PRMarks(failingChecks: ["test", "lint"])])

        // Never over a reading already taken.
        cache.storedPRStatus["p-1"] = doc(conflicting)
        await next.restore(projectID: "p-1")
        XCTAssertEqual(next.reading(projectID: "p-1"), PRReading(doc(red)))
        await next.restore(projectID: "uncached")
        XCTAssertNil(next.readings["uncached"])

        next.forget(projectID: "p-1")
        XCTAssertEqual(next.marks, [:])
    }

    /// A failed reading writes nothing to the cache either.
    func testAFailedReadingIsNotCached() async {
        let cache = FakePlanCache()
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(red)])
        client.setPRStatus(nil, forProject: "p-1")
        await PRStatusStore(client: client, cache: cache).update(projectID: "p-1")
        XCTAssertNil(cache.storedPRStatus["p-1"])
    }

    // MARK: - The loop

    private let pending = PRStatusSlice(
        sliceID: "s-p", name: "C", pr: "u", readiness: PRStatusSlice.awaitingReview,
        checks: PRStatusChecks(verdict: PRStatusSlice.checksPending))
    private let passing = PRStatusSlice(
        sliceID: "s-p", name: "C", pr: "u", readiness: PRStatusSlice.readyToMerge,
        checks: PRStatusChecks(verdict: PRStatusSlice.checksPassing))
    private let cadence = PRStatusStore.Cadence(fast: .milliseconds(20), slow: .seconds(60))

    /// Waits, in milliseconds, for what the loop is expected to do.
    private func waitUntil(_ condition: () -> Bool, file: StaticString = #filePath, line: UInt = #line) async {
        for _ in 0..<400 where !condition() { try? await Task.sleep(for: .milliseconds(5)) }
        XCTAssertTrue(condition(), "timed out", file: file, line: line)
    }

    func testRunningChecksReadFastUntilTheVerdictLandsThenSlow() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(pending)])
        let store = PRStatusStore(client: client, cache: FakePlanCache(), cadence: cadence)
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.intervals["p-1"], cadence.fast)
        XCTAssertEqual(store.marks, ["s-p": PRMarks(checksRunning: true)])

        // The loop reads again on its own, at the fast cadence.
        await waitUntil { client.prStatusReads.count >= 3 }
        client.setPRStatus(doc(passing), forProject: "p-1")
        await waitUntil { store.marks == ["s-p": PRMarks(checksPassing: true)] }
        XCTAssertEqual(store.intervals["p-1"], cadence.slow, "a verdict in: back to the plan poll's cadence")
        let reads = client.prStatusReads.count
        try? await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(client.prStatusReads.count, reads, "nothing read at the slow cadence within it")
        store.stop()
    }

    func testALiveAgentOnAnOpenPullRequestReadsFast() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(passing)])
        var live: Set<String> = []
        let store = PRStatusStore(
            client: client, cache: FakePlanCache(), cadence: cadence, liveSliceIDs: { live })
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.intervals["p-1"], cadence.slow)
        live = ["elsewhere"]
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.intervals["p-1"], cadence.slow, "an agent on a slice with no open pull request")
        live = ["s-p"]
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.intervals["p-1"], cadence.fast)
        XCTAssertEqual(
            store.interval(for: PRReading(doc(PRStatusSlice(
                sliceID: "s-p", name: "C", pr: "u", readiness: "unread",
                checks: PRStatusChecks(verdict: PRStatusSlice.checksPending)))), cadence: cadence),
            cadence.slow, "running checks on a pull request no longer open")
        store.stop()
    }

    func testAReadingUnderWayIsJoinedNotDoubled() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(red)])
        let store = PRStatusStore(client: client, cache: FakePlanCache())
        client.holdPRStatus()
        async let first: Void = store.update(projectID: "p-1")
        async let second: Void = store.update(projectID: "p-1")
        await waitUntil { !client.prStatusReads.isEmpty }
        client.releasePRStatus()
        _ = await (first, second)
        XCTAssertEqual(client.prStatusReads, ["p-1"])
        XCTAssertEqual(store.marks.count, 1)
        XCTAssertTrue(store.intervals.isEmpty, "no cadence, no loop")
    }

    func testAnUnchangedReadingPublishesNothing() async {
        let cache = FakePlanCache()
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(red)])
        let store = PRStatusStore(client: client, cache: cache)
        await store.update(projectID: "p-1")
        cache.storedPRStatus["p-1"] = nil

        final class Flag: @unchecked Sendable { var raised = false }
        let changed = Flag()
        withObservationTracking { _ = store.readings } onChange: { changed.raised = true }
        await store.update(projectID: "p-1")
        XCTAssertFalse(changed.raised)
        XCTAssertNil(cache.storedPRStatus["p-1"], "and writes nothing to the cache")

        client.setPRStatus(doc(conflicting), forProject: "p-1")
        await store.update(projectID: "p-1")
        XCTAssertTrue(changed.raised)
    }

    func testForgettingAProjectStopsItsLoopAndDropsItsReadingUnderWay() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(pending)])
        let store = PRStatusStore(client: client, cache: FakePlanCache(), cadence: cadence)
        await store.update(projectID: "p-1")
        store.forget(projectID: "p-1")
        XCTAssertNil(store.intervals["p-1"])
        let reads = client.prStatusReads.count
        try? await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(client.prStatusReads.count, reads, "the loop stopped with the tab")

        // A reading under way when the tab closes lands nowhere.
        client.holdPRStatus()
        let late = Task { await store.update(projectID: "p-1") }
        await waitUntil { client.prStatusReads.count > reads }
        store.forget(projectID: "p-1")
        client.releasePRStatus()
        await late.value
        XCTAssertNil(store.readings["p-1"])
        XCTAssertNil(store.intervals["p-1"])

        // Reopened: a reading starts the loop again.
        await store.update(projectID: "p-1")
        XCTAssertEqual(store.intervals["p-1"], cadence.fast)
        store.stop()
        XCTAssertTrue(store.intervals.isEmpty)
    }

    func testTheLoopSkipsAProjectWithNothingToRead() async {
        let client = FixtureNatClient(prStatusByProject: ["p-1": doc(pending)])
        var worth = true
        let store = PRStatusStore(
            client: client, cache: FakePlanCache(), cadence: cadence, shouldRead: { _ in worth })
        await store.update(projectID: "p-1")
        worth = false
        let reads = client.prStatusReads.count
        try? await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(client.prStatusReads.count, reads, "nothing read while nothing is worth reading")
        XCTAssertEqual(store.intervals["p-1"], cadence.fast, "still looking")
        worth = true
        await waitUntil { client.prStatusReads.count > reads }
        store.stop()
    }
}
