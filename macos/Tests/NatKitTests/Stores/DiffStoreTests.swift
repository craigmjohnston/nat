import XCTest
@testable import NatKit

// MARK: - Mock Client

private struct DiffTestError: Error {}

private final class MockDiffClient: NatClientProtocol, @unchecked Sendable {
    enum Response {
        case success(SliceDiff)
        case failure
    }

    private var response: Response
    private(set) var callCount = 0
    private(set) var lastSliceRef: String?

    /// Every `agent-send` call this client received, in order.
    private(set) var sentPrompts: [(projectID: String, sliceRef: String, text: String)] = []
    var sendError: Error?

    /// Every `slice-approve` call this client received, in order.
    private(set) var approveCalls: [(projectID: String, sliceRef: String)] = []
    var approveResult: Result<String, Error> = .success("https://github.test/craig/nat/pull/1")

    init(response: Response) {
        self.response = response
    }

    func setResponse(_ response: Response) {
        self.response = response
    }

    func info(projectID: String) async throws -> ProjectInfo {
        throw DiffTestError()
    }

    func status() async throws -> [AgentStatus] {
        []
    }

    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        throw DiffTestError()
    }

    func sliceDiff(projectID: String, sliceRef: String) async throws -> SliceDiff {
        callCount += 1
        lastSliceRef = sliceRef
        switch response {
        case .success(let diff):
            return diff
        case .failure:
            throw DiffTestError()
        }
    }

    func agentInterrupt(projectID: String, sliceRef: String) async throws {
        throw DiffTestError()
    }

    func agentSend(projectID: String, sliceRef: String, text: String) async throws {
        sentPrompts.append((projectID, sliceRef, text))
        if let sendError {
            throw sendError
        }
    }

    func sliceApprove(projectID: String, sliceRef: String) async throws -> String {
        approveCalls.append((projectID, sliceRef))
        return try approveResult.get()
    }

    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        throw DiffTestError()
    }
}

// MARK: - Tests

final class DiffStoreTests: XCTestCase {
    private func makeDiff(file path: String = "a.go") -> SliceDiff {
        SliceDiff(
            base: "main",
            branch: "nat/example",
            files: [
                SliceDiffFile(
                    path: path, oldPath: path, adds: 1, dels: 0, described: false,
                    lines: [
                        "diff --git a/\(path) b/\(path)",
                        "--- a/\(path)",
                        "+++ b/\(path)",
                        "@@ -0,0 +1,1 @@",
                        "+hello"
                    ]
                )
            ]
        )
    }

    @MainActor
    func testInitialStateIsIdle() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        XCTAssertEqual(store.loadState, .idle)
        XCTAssertTrue(store.viewedFiles.isEmpty)
        XCTAssertTrue(store.collapsedFiles.isEmpty)
    }

    @MainActor
    func testFetchLoadsTheDiff() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.callCount, 1)
        XCTAssertEqual(client.lastSliceRef, "slice-1")
        XCTAssertEqual(store.loadState.diff?.branch, "nat/example")
    }

    @MainActor
    func testFetchDoesNotRefetchTheSameSliceOnceLoaded() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.callCount, 1)
    }

    @MainActor
    func testFetchRefetchesADifferentSlice() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertEqual(client.callCount, 2)
        XCTAssertEqual(client.lastSliceRef, "slice-2")
    }

    @MainActor
    func testFailedFetchDropsAnyPreviousDiff() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertNotNil(store.loadState.diff)

        client.setResponse(.failure)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertNil(store.loadState.diff, "a failed read should drop the previous diff, not keep it visible")
        XCTAssertNotNil(store.loadState.errorMessage)
    }

    @MainActor
    func testRefreshRereadsTheCurrentSlice() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.refresh()

        XCTAssertEqual(client.callCount, 2)
        XCTAssertEqual(client.lastSliceRef, "slice-1")
    }

    @MainActor
    func testRefreshWithNothingFetchedIsANoOp() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.refresh()

        XCTAssertEqual(client.callCount, 0)
    }

    @MainActor
    func testViewedAndCollapsedAreClearedByARefresh() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        store.toggleViewed("a.go")
        XCTAssertTrue(store.isViewed("a.go"))
        XCTAssertTrue(store.isCollapsed("a.go"))

        await store.refresh()

        XCTAssertFalse(store.isViewed("a.go"))
        XCTAssertFalse(store.isCollapsed("a.go"))
    }

    @MainActor
    func testToggleViewedMarksAndCollapsesGitHubFashion() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        store.toggleViewed("a.go")
        XCTAssertTrue(store.isViewed("a.go"))
        XCTAssertTrue(store.isCollapsed("a.go"))

        store.toggleViewed("a.go")
        XCTAssertFalse(store.isViewed("a.go"))
        // Un-marking viewed leaves the fold as it was: asking to look again is
        // not the same as asking to expand it.
        XCTAssertTrue(store.isCollapsed("a.go"))
    }

    @MainActor
    func testToggleCollapsedIsIndependentOfViewed() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        store.toggleCollapsed("a.go")
        XCTAssertTrue(store.isCollapsed("a.go"))
        XCTAssertFalse(store.isViewed("a.go"))

        store.toggleCollapsed("a.go")
        XCTAssertFalse(store.isCollapsed("a.go"))
    }

    @MainActor
    func testClearResetsEverything() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        store.toggleViewed("a.go")
        store.clear()

        XCTAssertEqual(store.loadState, .idle)
        XCTAssertTrue(store.viewedFiles.isEmpty)
        XCTAssertTrue(store.collapsedFiles.isEmpty)

        // fetch() re-fetches after a clear even for the same slice ref.
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertEqual(client.callCount, 2)
    }

    @MainActor
    func testDiffLoadStateAccessors() {
        let diff = DiffModel(base: "main", branch: "b", files: [])
        XCTAssertEqual(DiffLoadState.loaded(diff).diff, diff)
        XCTAssertNil(DiffLoadState.loaded(diff).errorMessage)
        XCTAssertTrue(DiffLoadState.loading.isLoading)
        XCTAssertFalse(DiffLoadState.idle.isLoading)
        XCTAssertEqual(DiffLoadState.failed("oops").errorMessage, "oops")
        XCTAssertNil(DiffLoadState.failed("oops").diff)
    }

    // MARK: - Comments: add / edit / delete

    private func makeMultiRowDiff(file path: String = "a.go") -> SliceDiff {
        SliceDiff(
            base: "main",
            branch: "nat/example",
            files: [
                SliceDiffFile(
                    path: path, oldPath: path, adds: 3, dels: 0, described: false,
                    lines: [
                        "diff --git a/\(path) b/\(path)",
                        "--- a/\(path)",
                        "+++ b/\(path)",
                        "@@ -0,0 +1,3 @@",
                        "+one",
                        "+two",
                        "+three"
                    ]
                )
            ]
        )
    }

    @MainActor
    func testSetCommentAddsAPendingComment() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id

        let comment = store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "  clamp this  ")

        XCTAssertEqual(store.pendingCommentCount, 1)
        XCTAssertEqual(comment?.text, "clamp this", "the text should be trimmed")
        XCTAssertEqual(store.comment(path: "a.go", anchorRowIDs: [rowID])?.text, "clamp this")
    }

    @MainActor
    func testSetCommentOnTheSameRunEditsRatherThanDuplicates() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id

        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "first thought")
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "second thought")

        XCTAssertEqual(store.pendingCommentCount, 1)
        XCTAssertEqual(store.comments.first?.text, "second thought")
    }

    @MainActor
    func testSetCommentWithEmptyTextTakesItBack() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id

        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "x")
        XCTAssertEqual(store.pendingCommentCount, 1)

        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "   ")
        XCTAssertEqual(store.pendingCommentCount, 0)
    }

    @MainActor
    func testSetCommentRefusesAnEmptyPathOrAnchor() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)

        XCTAssertNil(store.setComment(path: "", anchorRowIDs: ["1"], text: "x"))
        XCTAssertNil(store.setComment(path: "a.go", anchorRowIDs: [], text: "x"))
        XCTAssertEqual(store.pendingCommentCount, 0)
    }

    @MainActor
    func testDeleteCommentByID() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        let comment = store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "x")!

        store.deleteComment(id: comment.id)

        XCTAssertEqual(store.pendingCommentCount, 0)
    }

    @MainActor
    func testCommentsOrderedByFileThenPositionWithinFile() async {
        let diff = SliceDiff(base: "main", branch: "nat/two-files", files: [
            makeMultiRowDiff(file: "b.go").files[0],
            makeMultiRowDiff(file: "a.go").files[0]
        ])
        let client = MockDiffClient(response: .success(diff))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        let bRows = store.loadState.diff!.files[0].rows // b.go, drawn first
        let aRows = store.loadState.diff!.files[1].rows // a.go, drawn second

        // Left out of file/line order on purpose.
        store.setComment(path: "a.go", anchorRowIDs: [aRows[0].id], text: "a first")
        store.setComment(path: "b.go", anchorRowIDs: [bRows[2].id], text: "b third")
        store.setComment(path: "b.go", anchorRowIDs: [bRows[0].id], text: "b first")

        let texts = store.comments.map(\.text)
        XCTAssertEqual(texts, ["b first", "b third", "a first"])
    }

    // MARK: - Comments: re-anchoring across a refresh

    @MainActor
    func testRefreshKeepsACommentWhoseLinesAreUnchanged() async {
        let client = MockDiffClient(response: .success(makeMultiRowDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rows = store.loadState.diff!.files[0].rows
        store.setComment(path: "a.go", anchorRowIDs: [rows[0].id, rows[1].id], text: "these two")

        await store.refresh()

        XCTAssertEqual(store.pendingCommentCount, 1, "unchanged lines should still be found by the same IDs")
        XCTAssertEqual(store.lastDroppedCommentCount, 0)
        XCTAssertEqual(store.comments.first?.text, "these two")
    }

    @MainActor
    func testRefreshDropsACommentWhoseFileIsGone() async {
        let client = MockDiffClient(response: .success(makeDiff(file: "a.go")))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "gone with the file")

        client.setResponse(.success(makeDiff(file: "b.go")))
        await store.refresh()

        XCTAssertEqual(store.pendingCommentCount, 0)
        XCTAssertEqual(store.lastDroppedCommentCount, 1)
    }

    @MainActor
    func testRefreshDropsACommentWhoseLinesNoLongerFormOneRun() async {
        let client = MockDiffClient(response: .success(makeMultiRowDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rows = store.loadState.diff!.files[0].rows
        // A run across "one" and "three", skipping "two" — never something the
        // UI would build itself, but it stands in for a comment whose lines no
        // longer sit next to each other after a re-read.
        store.setComment(path: "a.go", anchorRowIDs: [rows[0].id, rows[2].id], text: "no longer contiguous")

        await store.refresh()

        XCTAssertEqual(store.pendingCommentCount, 0)
        XCTAssertEqual(store.lastDroppedCommentCount, 1)
    }

    @MainActor
    func testRefreshDropsOnlyTheCommentThatNoLongerFitsAndCountsIt() async {
        let diff = SliceDiff(base: "main", branch: "nat/two-files", files: [
            makeMultiRowDiff(file: "a.go").files[0],
            makeDiff(file: "b.go").files[0]
        ])
        let client = MockDiffClient(response: .success(diff))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let aRows = store.loadState.diff!.files[0].rows
        let bRowID = store.loadState.diff!.files[1].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [aRows[0].id], text: "kept")
        store.setComment(path: "b.go", anchorRowIDs: [bRowID], text: "dropped")

        // b.go disappears from the next read; a.go is untouched.
        let onlyA = SliceDiff(base: "main", branch: "nat/two-files", files: [makeMultiRowDiff(file: "a.go").files[0]])
        client.setResponse(.success(onlyA))
        await store.refresh()

        XCTAssertEqual(store.pendingCommentCount, 1)
        XCTAssertEqual(store.comments.first?.text, "kept")
        XCTAssertEqual(store.lastDroppedCommentCount, 1)
    }

    @MainActor
    func testAFailedRefreshKeepsEveryCommentPending() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "still here")

        client.setResponse(.failure)
        await store.refresh()

        XCTAssertNil(store.loadState.diff)
        XCTAssertEqual(store.pendingCommentCount, 1, "a failed read should not swallow the pending comments")
        XCTAssertEqual(store.comments.first?.text, "still here")
    }

    @MainActor
    func testFetchingADifferentSliceStartsWithNoComments() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "about slice one")

        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertEqual(store.pendingCommentCount, 0)
    }

    @MainActor
    func testClearDropsPendingComments() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "x")

        store.clear()

        XCTAssertEqual(store.pendingCommentCount, 0)
        XCTAssertEqual(store.lastDroppedCommentCount, 0)
    }

    // MARK: - Sending comments

    @MainActor
    func testSendCommentsClearsThemOnSuccess() async throws {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "clamp this")

        let count = try await store.sendComments(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(count, 1)
        XCTAssertEqual(store.pendingCommentCount, 0)
        XCTAssertEqual(client.sentPrompts.count, 1)
        XCTAssertEqual(client.sentPrompts[0].sliceRef, "slice-1")
        XCTAssertTrue(client.sentPrompts[0].text.contains("clamp this"))
        XCTAssertTrue(client.sentPrompts[0].text.contains("nat/example"))
    }

    @MainActor
    func testSendCommentsKeepsThemOnFailure() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.sendError = DiffTestError()
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "clamp this")

        do {
            _ = try await store.sendComments(projectID: "proj-1", sliceRef: "slice-1")
            XCTFail("expected the send to throw")
        } catch {
            // expected
        }

        XCTAssertEqual(store.pendingCommentCount, 1, "a failed send should leave every comment pending")
    }

    @MainActor
    func testSendCommentsWithNothingPendingDoesNotCallTheClient() async throws {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        let count = try await store.sendComments(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(count, 0)
        XCTAssertEqual(client.sentPrompts.count, 0)
    }

    // MARK: - Approving

    @MainActor
    func testApprovePassesThroughToTheClient() async throws {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.approveResult = .success("https://github.test/craig/nat/pull/42")
        let store = DiffStore(client: client)

        let url = try await store.approve(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(url, "https://github.test/craig/nat/pull/42")
        XCTAssertEqual(client.approveCalls.count, 1)
        XCTAssertEqual(client.approveCalls[0].sliceRef, "slice-1")
    }

    @MainActor
    func testApprovePropagatesFailure() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.approveResult = .failure(DiffTestError())
        let store = DiffStore(client: client)

        do {
            _ = try await store.approve(projectID: "proj-1", sliceRef: "slice-1")
            XCTFail("expected approve to throw")
        } catch {
            // expected
        }
    }
}
