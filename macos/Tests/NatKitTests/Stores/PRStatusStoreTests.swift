import XCTest
@testable import NatKit
@testable import NatFixtures

/// `PRStatusStore`: one `pr-status` reading per project, as the batched
/// GitHub reading hands it over, replaced only by a newer reading of that
/// project, and cached.
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
        let store = PRStatusStore(cache: FakePlanCache())
        await store.apply(doc(red), projectID: "p-1")
        await store.apply(doc(conflicting), projectID: "p-2")

        XCTAssertEqual(store.reading(projectID: "p-1").failingChecks, ["s-red": ["test", "lint"]])
        XCTAssertEqual(store.reading(projectID: "p-1").conflicts, [:])
        XCTAssertEqual(store.reading(projectID: "p-2").conflicts, ["s-x": BranchConflict(base: "main")])
        XCTAssertEqual(store.reading(projectID: "never-read"), .empty)
        XCTAssertEqual(store.marks, [
            "s-red": PRMarks(failingChecks: ["test", "lint"]),
            "s-x": PRMarks(conflict: BranchConflict(base: "main")),
        ])
    }

    /// A newer reading that no longer says a mark takes it away.
    func testAReadingIsReplacedByANewerOne() async {
        let store = PRStatusStore(cache: FakePlanCache())
        await store.apply(doc(red, conflicting), projectID: "p-1")
        XCTAssertEqual(store.marks.count, 2)

        await store.apply(doc(
            PRStatusSlice(sliceID: "s-red", name: "A", pr: "u", readiness: PRStatusSlice.readyToMerge),
            PRStatusSlice(sliceID: "s-x", name: "B", pr: "u", readiness: "unread")), projectID: "p-1")
        XCTAssertEqual(store.marks, [:])
        XCTAssertEqual(store.reading(projectID: "p-1").readiness, ["s-red": PRStatusSlice.readyToMerge])
    }

    /// A failure is held over the project's next reading while the checks
    /// run again, and is never cached.
    func testAFailureIsHeldWhileTheChecksRunAgain() async {
        let cache = FakePlanCache()
        let store = PRStatusStore(cache: cache)
        await store.apply(doc(red), projectID: "p-1")
        let rerun = PRStatusSlice(
            sliceID: "s-red", name: "A", pr: "u", readiness: PRStatusSlice.awaitingReview,
            checks: PRStatusChecks(verdict: PRStatusSlice.checksPending))
        await store.apply(doc(rerun), projectID: "p-1")
        XCTAssertEqual(store.marks, ["s-red": PRMarks(checksRunning: true, heldFailingChecks: ["test", "lint"])])
        XCTAssertEqual(cache.storedPRStatus["p-1"], doc(rerun))

        await store.apply(doc(rerun), projectID: "p-2")
        XCTAssertEqual(store.reading(projectID: "p-2").heldFailingChecks, [:], "only the project's own reading")
    }

    func testEveryReadingThatLandsIsCachedAndRestoredBeforeAnyFresherOne() async {
        let cache = FakePlanCache()
        let first = PRStatusStore(cache: cache)
        await first.apply(doc(red), projectID: "p-1")
        XCTAssertEqual(cache.storedPRStatus["p-1"], doc(red))

        // A launch: the cached reading is up before any fresh one.
        let next = PRStatusStore(cache: cache)
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

    func testAnUnchangedReadingPublishesNothing() async {
        let cache = FakePlanCache()
        let store = PRStatusStore(cache: cache)
        await store.apply(doc(red), projectID: "p-1")
        cache.storedPRStatus["p-1"] = nil

        final class Flag: @unchecked Sendable { var raised = false }
        let changed = Flag()
        withObservationTracking { _ = store.readings } onChange: { changed.raised = true }
        await store.apply(doc(red), projectID: "p-1")
        XCTAssertFalse(changed.raised)
        XCTAssertNil(cache.storedPRStatus["p-1"], "and writes nothing to the cache")

        await store.apply(doc(conflicting), projectID: "p-1")
        XCTAssertTrue(changed.raised)
    }
}
