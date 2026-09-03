import XCTest
@testable import NatKit

// MARK: - Mock Client

private struct ReviewStatsTestError: Error {}

private final class MockReviewStatsClient: NatClientProtocol, @unchecked Sendable {
    /// What `sliceDiff` answers with, by slice ref — a slice ref with no
    /// entry throws.
    var diffs: [String: SliceDiff] = [:]
    private(set) var callCount = 0
    private(set) var callsBySliceRef: [String: Int] = [:]

    func info(projectID: String) async throws -> ProjectInfo { throw ReviewStatsTestError() }
    func status() async throws -> [AgentStatus] { [] }
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail { throw ReviewStatsTestError() }

    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        callCount += 1
        callsBySliceRef[sliceRef, default: 0] += 1
        guard let diff = diffs[sliceRef] else { throw ReviewStatsTestError() }
        return diff
    }

    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc { throw ReviewStatsTestError() }
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw ReviewStatsTestError()
    }
    func agentInterrupt(projectID: String, sliceRef: String) async throws { throw ReviewStatsTestError() }
    func agentSend(projectID: String, sliceRef: String, text: String) async throws { throw ReviewStatsTestError() }
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String { throw ReviewStatsTestError() }
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw ReviewStatsTestError()
    }
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail { throw ReviewStatsTestError() }
    func prMerge(projectID: String, sliceRef: String) async throws { throw ReviewStatsTestError() }
    func prComment(projectID: String, sliceRef: String, body: String) async throws { throw ReviewStatsTestError() }
    func workshopLaunch(projectID: String, model: String?, effort: String?) async throws -> WorkshopLaunchResult {
        throw ReviewStatsTestError()
    }
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        throw ReviewStatsTestError()
    }
    func configShow() async throws -> ConfigDoc { throw ReviewStatsTestError() }
    func configSet(key: String, value: String) async throws { throw ReviewStatsTestError() }
}

// MARK: - Tests

final class ReviewStatsStoreTests: XCTestCase {
    private func diff(adds: Int, dels: Int, files: Int = 1) -> SliceDiff {
        SliceDiff(
            base: "main", branch: "b",
            files: (0..<files).map { i in
                SliceDiffFile(path: "f\(i).go", adds: adds, dels: dels, described: false, lines: [])
            }
        )
    }

    @MainActor
    func testInitialStateHasNoStats() {
        let store = ReviewStatsStore(client: MockReviewStatsClient())
        XCTAssertTrue(store.stats.isEmpty)
    }

    @MainActor
    func testUpdateFetchesAStatForANewlyHandedBackSlice() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 10, dels: 3)
        let store = ReviewStatsStore(client: client)

        await store.update(projectID: "proj-1", handedBack: [
            .init(sliceID: "slice-1", branch: "nat/example")
        ])

        XCTAssertEqual(store.stats["slice-1"], "+10 \u{2212}3")
        XCTAssertEqual(client.callCount, 1)
    }

    @MainActor
    func testUpdateSumsAcrossEveryFile() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 5, dels: 2, files: 3)
        let store = ReviewStatsStore(client: client)

        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])

        XCTAssertEqual(store.stats["slice-1"], "+15 \u{2212}6")
    }

    @MainActor
    func testUpdateDoesNotRefetchTheSameBranchTwice() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 1, dels: 1)
        let store = ReviewStatsStore(client: client)

        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])

        XCTAssertEqual(client.callCount, 1)
    }

    @MainActor
    func testUpdateRefetchesWhenTheBranchChanges() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 1, dels: 1)
        let store = ReviewStatsStore(client: client)
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])

        client.diffs["slice-1"] = diff(adds: 9, dels: 9)
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b2")])

        XCTAssertEqual(client.callCount, 2)
        XCTAssertEqual(store.stats["slice-1"], "+9 \u{2212}9")
    }

    @MainActor
    func testUpdateDropsAStatForASliceNoLongerHandedBack() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 1, dels: 1)
        let store = ReviewStatsStore(client: client)
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])
        XCTAssertNotNil(store.stats["slice-1"])

        await store.update(projectID: "proj-1", handedBack: [])

        XCTAssertNil(store.stats["slice-1"])
    }

    @MainActor
    func testAFailedFetchLeavesTheSliceStatless() async {
        let client = MockReviewStatsClient() // no diff registered -> throws
        let store = ReviewStatsStore(client: client)

        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])

        XCTAssertNil(store.stats["slice-1"])
    }

    @MainActor
    func testAFailedFetchIsRetriedOnTheNextUpdateForTheSameBranch() async {
        let client = MockReviewStatsClient() // still no diff registered
        let store = ReviewStatsStore(client: client)

        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])

        XCTAssertEqual(client.callCount, 2, "a quiet failure should not stop the next update from trying again")
    }

    @MainActor
    func testMultipleSlicesEachGetTheirOwnStat() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 1, dels: 0)
        client.diffs["slice-2"] = diff(adds: 0, dels: 4)
        let store = ReviewStatsStore(client: client)

        await store.update(projectID: "proj-1", handedBack: [
            .init(sliceID: "slice-1", branch: "b1"),
            .init(sliceID: "slice-2", branch: "b2")
        ])

        XCTAssertEqual(store.stats["slice-1"], "+1 \u{2212}0")
        XCTAssertEqual(store.stats["slice-2"], "+0 \u{2212}4")
    }

    @MainActor
    func testClearResetsEverything() async {
        let client = MockReviewStatsClient()
        client.diffs["slice-1"] = diff(adds: 1, dels: 1)
        let store = ReviewStatsStore(client: client)
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])
        XCTAssertFalse(store.stats.isEmpty)

        store.clear()
        XCTAssertTrue(store.stats.isEmpty)

        // Clearing forgets the branch it already fetched too, so the same
        // branch is read again rather than skipped.
        await store.update(projectID: "proj-1", handedBack: [.init(sliceID: "slice-1", branch: "b")])
        XCTAssertEqual(client.callCount, 2)
    }
}
