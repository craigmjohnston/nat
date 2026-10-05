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
}
