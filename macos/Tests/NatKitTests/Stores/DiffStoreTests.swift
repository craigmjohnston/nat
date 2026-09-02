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
    /// The `commit` every `sliceDiff` call was given, in order — nil for a
    /// whole-branch read, a sha for a per-commit one.
    private(set) var commitCalls: [String?] = []

    /// What one commit's own diff decodes to, by sha — `sliceDiff(commit:)`
    /// looks a requested sha up here rather than in `response`, since a
    /// commit's diff is a different shape of change than the branch's own.
    var commitDiffs: [String: SliceDiff] = [:]
    var commitDiffError: Error?

    /// What `sliceCommits` answers with.
    var commitsResult: Result<SliceCommitsDoc, Error> = .success(
        SliceCommitsDoc(base: "main", branch: "nat/example", commits: [])
    )
    private(set) var commitsCallCount = 0

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

    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        callCount += 1
        lastSliceRef = sliceRef
        commitCalls.append(commit)
        if let commit {
            if let commitDiffError {
                throw commitDiffError
            }
            guard let diff = commitDiffs[commit] else { throw DiffTestError() }
            return diff
        }
        switch response {
        case .success(let diff):
            return diff
        case .failure:
            throw DiffTestError()
        }
    }

    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        commitsCallCount += 1
        return try commitsResult.get()
    }

    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        throw DiffTestError()
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

    func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        throw DiffTestError()
    }

    func prMerge(projectID: String, sliceRef: String) async throws {
        throw DiffTestError()
    }

    func prComment(projectID: String, sliceRef: String, body: String) async throws {
        throw DiffTestError()
    }

    func workshopLaunch(projectID: String, model: String?, effort: String?) async throws -> WorkshopLaunchResult {
        throw DiffTestError()
    }

    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        throw DiffTestError()
    }

    func configShow() async throws -> ConfigDoc { throw DiffTestError() }
    func configSet(key: String, value: String) async throws { throw DiffTestError() }
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

    // MARK: - Commits: listing

    private func makeCommits(_ shas: [String] = ["sha1", "sha2"]) -> SliceCommitsDoc {
        SliceCommitsDoc(
            base: "main", branch: "nat/example",
            commits: shas.map { SliceCommit(sha: $0, subject: "commit \($0)", author: "craig", date: Date(timeIntervalSince1970: 0)) }
        )
    }

    @MainActor
    func testInitialStateHasNoCommits() {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        XCTAssertTrue(store.commits.isEmpty)
        XCTAssertEqual(store.commitsLoadState, .idle)
        XCTAssertNil(store.selectedCommit)
        XCTAssertTrue(store.commentsEditable)
    }

    @MainActor
    func testFetchCommitsLoadsTheList() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitsResult = .success(makeCommits())
        let store = DiffStore(client: client)

        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.commitsCallCount, 1)
        XCTAssertEqual(store.commits.map(\.sha), ["sha1", "sha2"])
        XCTAssertEqual(store.commitsLoadState, .loaded(store.commits))
    }

    @MainActor
    func testFetchCommitsDoesNotRefetchOnceLoaded() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitsResult = .success(makeCommits())
        let store = DiffStore(client: client)

        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(client.commitsCallCount, 1)
    }

    @MainActor
    func testFetchCommitsForcedRefetches() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitsResult = .success(makeCommits())
        let store = DiffStore(client: client)

        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1", force: true)

        XCTAssertEqual(client.commitsCallCount, 2)
    }

    @MainActor
    func testFetchCommitsFailureIsRecorded() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitsResult = .failure(DiffTestError())
        let store = DiffStore(client: client)

        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")

        XCTAssertEqual(store.commitsLoadState, .failed(DiffTestError().localizedDescription))
    }

    @MainActor
    func testANewSliceClearsTheCommitsList() async {
        let client = MockDiffClient(response: .success(makeDiff(file: "a.go")))
        client.commitsResult = .success(makeCommits())
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")
        XCTAssertFalse(store.commits.isEmpty)

        client.setResponse(.success(makeDiff(file: "b.go")))
        await store.fetch(projectID: "proj-1", sliceRef: "slice-2")

        XCTAssertTrue(store.commits.isEmpty)
        XCTAssertEqual(store.commitsLoadState, .idle)
    }

    // MARK: - Commits: selecting one to view

    @MainActor
    func testSelectingACommitFetchesAndShowsItsOwnDiff() async throws {
        let client = MockDiffClient(response: .success(makeDiff(file: "a.go")))
        let commitDiff = SliceDiff(
            base: "sha1^", branch: "sha1",
            files: [SliceDiffFile(path: "only-in-commit.go", adds: 1, dels: 0, described: false, lines: ["+x"])]
        )
        client.commitDiffs = ["sha1": commitDiff]
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        await store.selectCommit("sha1")

        XCTAssertEqual(store.selectedCommit, "sha1")
        XCTAssertEqual(store.loadState.diff?.files.first?.path, "only-in-commit.go")
        XCTAssertEqual(client.commitCalls, [nil, "sha1"])
        XCTAssertFalse(store.commentsEditable)
    }

    @MainActor
    func testSelectingTheSameCommitAgainDoesNotRefetch() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitDiffs = ["sha1": makeDiff(file: "b.go")]
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.selectCommit("sha1")
        let callsAfterFirstSelect = client.commitCalls.count

        await store.selectCommit("sha1")

        XCTAssertEqual(client.commitCalls.count, callsAfterFirstSelect)
    }

    @MainActor
    func testSelectingACommitTwiceReusesTheCache() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitDiffs = ["sha1": makeDiff(file: "b.go")]
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.selectCommit("sha1")
        await store.selectCommit(nil)

        await store.selectCommit("sha1")

        // Fetched once for the commit on the first visit — not again for the
        // second, since it is still cached.
        XCTAssertEqual(client.commitCalls.filter { $0 == "sha1" }.count, 1)
    }

    @MainActor
    func testSelectingNilReturnsToTheBranchDiffWithoutRefetching() async {
        let client = MockDiffClient(response: .success(makeDiff(file: "a.go")))
        client.commitDiffs = ["sha1": makeDiff(file: "b.go")]
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.selectCommit("sha1")
        let callCountAfterSelect = client.callCount

        await store.selectCommit(nil)

        XCTAssertEqual(store.selectedCommit, nil)
        XCTAssertEqual(store.loadState.diff?.files.first?.path, "a.go")
        XCTAssertEqual(client.callCount, callCountAfterSelect, "returning to All commits should use the diff already held")
        XCTAssertTrue(store.commentsEditable)
    }

    @MainActor
    func testSelectingACommitThatFailsRecordsTheFailure() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitDiffError = DiffTestError()
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        await store.selectCommit("sha1")

        XCTAssertNil(store.loadState.diff)
        XCTAssertNotNil(store.loadState.errorMessage)
    }

    @MainActor
    func testSwitchingCommitsKeepsPendingComments() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitDiffs = ["sha1": makeDiff(file: "b.go")]
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        let rowID = store.loadState.diff!.files[0].rows[0].id
        store.setComment(path: "a.go", anchorRowIDs: [rowID], text: "still relevant")

        await store.selectCommit("sha1")

        XCTAssertEqual(store.pendingCommentCount, 1, "switching to a single commit should not drop pending comments")
        XCTAssertEqual(store.comments.first?.text, "still relevant")
    }

    // MARK: - Refresh drops the per-commit cache and re-reads the selection

    @MainActor
    func testRefreshDropsThePerCommitCacheAndRereadsIt() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitDiffs = ["sha1": makeDiff(file: "b.go")]
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")
        await store.selectCommit("sha1")
        let commitCallsBeforeRefresh = client.commitCalls.filter { $0 == "sha1" }.count

        await store.refresh()

        XCTAssertEqual(
            client.commitCalls.filter { $0 == "sha1" }.count, commitCallsBeforeRefresh + 1,
            "a refresh drops the cached commit diff, so the still-selected commit is read again"
        )
        XCTAssertEqual(store.selectedCommit, "sha1", "refresh should not snap the view back to All commits")
    }

    @MainActor
    func testRefreshWithNoCommitSelectedDoesNotFetchAnyCommitDiff() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        await store.refresh()

        XCTAssertTrue(client.commitCalls.allSatisfy { $0 == nil })
    }

    @MainActor
    func testRefreshRefetchesCommitsOnlyIfAlreadyLoaded() async {
        let client = MockDiffClient(response: .success(makeDiff()))
        client.commitsResult = .success(makeCommits())
        let store = DiffStore(client: client)
        await store.fetch(projectID: "proj-1", sliceRef: "slice-1")

        await store.refresh()
        XCTAssertEqual(client.commitsCallCount, 0, "commits never asked for should not be fetched by a refresh")

        await store.fetchCommits(projectID: "proj-1", sliceRef: "slice-1")
        await store.refresh()
        XCTAssertEqual(client.commitsCallCount, 2, "once loaded, a refresh should read the commit list fresh too")
    }
}
