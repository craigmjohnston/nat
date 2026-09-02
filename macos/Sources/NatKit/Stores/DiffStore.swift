import Foundation
import SwiftUI

/// The state of loading a slice's diff.
///
/// Unlike `LoadState` (which keeps the previous project on a failed refresh so
/// the board never blanks under a poll hiccup), a failed read here drops
/// whatever diff it replaced: a diff is of one branch at one moment, and
/// leaving the last one on screen under a failure would be showing the wrong
/// change (the same rule the Go TUI's `Diff` screen follows).
public enum DiffLoadState: Equatable, Sendable {
    case idle
    case loading
    case loaded(DiffModel)
    case failed(String)

    public var diff: DiffModel? {
        if case .loaded(let diff) = self { return diff }
        return nil
    }

    public var errorMessage: String? {
        if case .failed(let message) = self { return message }
        return nil
    }

    public var isLoading: Bool {
        if case .loading = self { return true }
        return false
    }
}

/// The state of loading a slice's branch's own commits — the "All commits"
/// dropdown's own list, fetched once the branch diff itself has loaded and
/// kept until the slice changes or a refresh asks for it again.
public enum CommitsLoadState: Equatable, Sendable {
    case idle
    case loading
    case loaded([SliceCommit])
    case failed(String)
}

/// Manages a slice's diff: fetching it on demand, and the screen's own state
/// over it — which files have been marked viewed, and which are collapsed.
///
/// Both of those are the screen's, not the diff's: they are never written
/// anywhere, and a re-read of the branch clears them exactly as the Go TUI's
/// fold and viewed-mark do, since a fold says the user has seen what was
/// there and that is exactly what a fresh read may have changed.
@MainActor
@Observable
public final class DiffStore {
    public private(set) var loadState: DiffLoadState = .idle
    public private(set) var viewedFiles: Set<String> = []
    public private(set) var collapsedFiles: Set<String> = []

    /// The pending review left on this slice's branch — ephemeral, never
    /// written to Notion or GitHub, cleared only once `sendComments` has
    /// actually reached the agent's pane. Ordered the way the diff draws
    /// them: by file in the order the change touches them, then by the
    /// comment's own position within the file (mirrors the Go TUI's
    /// `Comments()`), and by path alone once there is no diff left to order
    /// them by at all — a comment outlives a read that failed after it was
    /// left.
    public private(set) var comments: [PendingComment] = []

    /// How many pending comments the last successful read of the branch
    /// dropped, because their anchored lines no longer exist as a whole run —
    /// the view surfaces this so the user is told, since the comments
    /// themselves are gone without a trace once dropped.
    public private(set) var lastDroppedCommentCount = 0

    /// The branch's own commits since its merge base — the "All commits"
    /// dropdown's list, fetched alongside the branch diff itself.
    public private(set) var commits: [SliceCommit] = []
    public private(set) var commitsLoadState: CommitsLoadState = .idle

    /// Which diff `loadState` is currently showing: nil for the branch as a
    /// whole ("All commits"), or one commit's own sha.
    public private(set) var selectedCommit: String?

    /// Whether a new comment can be created, and whatever is already pending
    /// can be sent or the branch approved — true only while showing the
    /// branch as a whole. A comment or an approval is about the branch's own
    /// diff (what `reanchorComments` matches a re-read against), and one
    /// commit's own diff is not that, however much it overlaps it.
    public var commentsEditable: Bool { selectedCommit == nil }

    private let client: NatClientProtocol
    private var isFetching = false
    private var projectID: String?
    private var sliceRef: String?

    /// The branch-wide diff, kept regardless of which commit (if any) is
    /// selected: comments always anchor to it, and reanchoring on a refresh
    /// always matches against it rather than whatever is on screen.
    private var branchDiff: DiffModel?

    /// Per-commit diffs already fetched, by sha — dropped on every refresh,
    /// since a fresh read of the branch is the moment a stale per-commit diff
    /// stops being trustworthy.
    private var commitDiffCache: [String: DiffModel] = [:]

    public init(client: NatClientProtocol = NatClient()) {
        self.client = client
    }

    /// Fetch the diff for a slice, unless it is already loaded (or loading)
    /// for that same slice.
    public func fetch(projectID: String, sliceRef: String) async {
        guard !isFetching else { return }
        if self.projectID == projectID, self.sliceRef == sliceRef, loadState.diff != nil {
            return
        }
        // Another slice's branch starts with no comments on it — the pending
        // review left on the one before is about lines this slice never had —
        // and with no commits of its own read yet either.
        if let previous = self.sliceRef, previous != sliceRef {
            comments = []
            lastDroppedCommentCount = 0
            commits = []
            commitsLoadState = .idle
            commitDiffCache = [:]
            selectedCommit = nil
            branchDiff = nil
        }
        self.projectID = projectID
        self.sliceRef = sliceRef
        await load()
    }

    /// Re-read the current slice's branch — and, if a specific commit is
    /// selected, that commit's own diff too, so a refresh leaves the user
    /// looking at fresh content of whichever they were reading rather than
    /// snapping back to "All commits". The per-commit cache is dropped first:
    /// a refresh is exactly the moment a cached commit diff stops being
    /// trustworthy. A no-op with nothing fetched yet.
    public func refresh() async {
        guard !isFetching, let projectID, let sliceRef else { return }
        commitDiffCache = [:]
        await load()
        if let commit = selectedCommit {
            await loadCommitDiff(commit, projectID: projectID, sliceRef: sliceRef)
        }
        if case .idle = commitsLoadState {
            // Never asked for; a refresh does not go looking for them either.
        } else {
            await fetchCommits(projectID: projectID, sliceRef: sliceRef, force: true)
        }
    }

    /// Switch which diff is shown: nil for the branch as a whole ("All
    /// commits", already held), or one commit's own sha — fetched once and
    /// cached by sha afterwards. A no-op if it is already what is shown.
    public func selectCommit(_ sha: String?) async {
        guard sha != selectedCommit else { return }
        guard let projectID, let sliceRef else { return }
        selectedCommit = sha

        guard let sha else {
            if let branchDiff {
                loadState = .loaded(branchDiff)
            }
            return
        }
        if let cached = commitDiffCache[sha] {
            loadState = .loaded(cached)
            return
        }
        await loadCommitDiff(sha, projectID: projectID, sliceRef: sliceRef)
    }

    /// Fetch the branch's own commits since its merge base, unless already
    /// loaded (or loading) — `force` bypasses that, for a refresh that wants
    /// them read fresh.
    public func fetchCommits(projectID: String, sliceRef: String, force: Bool = false) async {
        if case .loading = commitsLoadState { return }
        if !force, case .loaded = commitsLoadState { return }
        commitsLoadState = .loading
        do {
            let doc = try await client.sliceCommits(projectID: projectID, sliceRef: sliceRef)
            commits = doc.commits
            commitsLoadState = .loaded(doc.commits)
        } catch {
            commitsLoadState = .failed(error.localizedDescription)
        }
    }

    /// Whether a file (by path) has been marked viewed.
    public func isViewed(_ path: String) -> Bool {
        viewedFiles.contains(path)
    }

    /// Whether a file (by path) is collapsed to its header row alone.
    public func isCollapsed(_ path: String) -> Bool {
        collapsedFiles.contains(path)
    }

    /// Toggle a file's viewed mark. Marking a file viewed collapses it too,
    /// GitHub-fashion; un-marking it leaves its fold as it was, since the user
    /// asking to look again is not the same as asking to expand it.
    public func toggleViewed(_ path: String) {
        if viewedFiles.contains(path) {
            viewedFiles.remove(path)
        } else {
            viewedFiles.insert(path)
            collapsedFiles.insert(path)
        }
    }

    /// Toggle a file's own fold, independent of whether it is viewed.
    public func toggleCollapsed(_ path: String) {
        if collapsedFiles.contains(path) {
            collapsedFiles.remove(path)
        } else {
            collapsedFiles.insert(path)
        }
    }

    /// Clear all state, as if nothing had ever been fetched.
    public func clear() {
        loadState = .idle
        viewedFiles = []
        collapsedFiles = []
        projectID = nil
        sliceRef = nil
        comments = []
        lastDroppedCommentCount = 0
        commits = []
        commitsLoadState = .idle
        selectedCommit = nil
        commitDiffCache = [:]
        branchDiff = nil
    }

    // MARK: - Comments

    /// Add or replace the pending comment on a run of one file's rows — or,
    /// with text that trims to nothing, take it back, exactly as the Go
    /// TUI's `SetComment` does: an emptied comment box is how one is
    /// removed. Commenting again on exactly the same run edits what was
    /// already there rather than adding a second comment beside it.
    @discardableResult
    public func setComment(path: String, anchorRowIDs: [String], text: String) -> PendingComment? {
        guard !path.isEmpty, !anchorRowIDs.isEmpty else { return nil }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let existingIndex = comments.firstIndex { $0.path == path && $0.anchorRowIDs == anchorRowIDs }

        if trimmed.isEmpty {
            if let existingIndex {
                comments.remove(at: existingIndex)
            }
            return nil
        }

        if let existingIndex {
            comments[existingIndex].text = trimmed
            return comments[existingIndex]
        }

        let comment = PendingComment(path: path, anchorRowIDs: anchorRowIDs, text: trimmed)
        comments.append(comment)
        sortComments()
        return comment
    }

    /// The pending comment already left on exactly this run of rows, if any —
    /// what a reopened comment box prefills with.
    public func comment(path: String, anchorRowIDs: [String]) -> PendingComment? {
        comments.first { $0.path == path && $0.anchorRowIDs == anchorRowIDs }
    }

    /// Take back one pending comment outright, by identity — the trash icon
    /// on a comment card.
    public func deleteComment(id: PendingComment.ID) {
        comments.removeAll { $0.id == id }
    }

    /// How many comments are waiting to be sent.
    public var pendingCommentCount: Int { comments.count }

    /// Send every pending comment to the agent working this slice's branch,
    /// as one prompt (mirrors `internal/tui/diffcomment.go`'s
    /// `sendCommentsFlow`). They are cleared only once the send has actually
    /// reached the pane: a send that failed leaves every one of them
    /// pending, because they are held nowhere else and retyping a review is
    /// not a thing to ask of anybody.
    @discardableResult
    public func sendComments(projectID: String, sliceRef: String) async throws -> Int {
        guard let diff = loadState.diff, !comments.isEmpty else { return 0 }
        let prompt = commentsPrompt(comments, diff: diff)
        let count = comments.count
        try await client.agentSend(projectID: projectID, sliceRef: sliceRef, text: prompt)
        comments = []
        return count
    }

    /// Open a pull request for this slice's handed-back branch and record it,
    /// exactly what the board's own approve key does — the one action here
    /// that reaches outside the diff itself.
    public func approve(projectID: String, sliceRef: String) async throws -> String {
        try await client.sliceApprove(projectID: projectID, sliceRef: sliceRef)
    }

    // MARK: - Private

    /// Reads the branch-wide diff — comments always anchor to it, and
    /// reanchoring on a refresh always matches against it — and, only while
    /// "All commits" is what is actually shown, updates `loadState` with it
    /// too. While a specific commit is selected, this still runs (so a
    /// refresh keeps comments in step with the branch even though something
    /// else is on screen), but leaves `loadState` alone; `refresh()` is what
    /// re-reads the selected commit's own diff afterwards.
    private func load() async {
        guard let projectID, let sliceRef else { return }
        isFetching = true
        defer { isFetching = false }

        let showingBranch = selectedCommit == nil
        if showingBranch {
            loadState = .loading
        }
        // A re-read's marks are the previous diff's, not the one about to
        // replace it — cleared here regardless of how the read turns out.
        viewedFiles = []
        collapsedFiles = []
        lastDroppedCommentCount = 0

        do {
            let diff = try await client.sliceDiff(projectID: projectID, sliceRef: sliceRef, commit: nil)
            let model = buildDiffModel(from: diff)
            branchDiff = model
            reanchorComments(to: model)
            if showingBranch {
                loadState = .loaded(model)
            }
        } catch {
            // A read that fails takes the diff it replaced with it, but not
            // the comments left on it: they are still there to send once a
            // read succeeds again, ordered by path alone with no diff left
            // to order them by (mirrors the Go TUI's own rule).
            branchDiff = nil
            comments.sort { $0.path < $1.path }
            if showingBranch {
                loadState = .failed(error.localizedDescription)
            }
        }
    }

    /// Fetches one commit's own diff, caching it by sha — used by
    /// `selectCommit` for a sha not already cached, and by `refresh()` to
    /// re-read the selected commit fresh after the cache has been dropped.
    private func loadCommitDiff(_ sha: String, projectID: String, sliceRef: String) async {
        loadState = .loading
        do {
            let diff = try await client.sliceDiff(projectID: projectID, sliceRef: sliceRef, commit: sha)
            let model = buildDiffModel(from: diff)
            commitDiffCache[sha] = model
            loadState = .loaded(model)
        } catch {
            loadState = .failed(error.localizedDescription)
        }
    }

    /// Carries every pending comment onto the rows it was left on wherever
    /// they have got to in a freshly read diff, and drops — counting —
    /// those whose rows can no longer all be found as one contiguous run in
    /// the same file: `internal/tui/diffcomment.go`'s
    /// `TestDiffCommentsSurviveARereadThatMovedThem`/
    /// `TestDiffCommentsDroppedWhenTheirFileGoes` is the rule this ports.
    private func reanchorComments(to model: DiffModel) {
        var positions: [String: [String: Int]] = [:]
        for file in model.files {
            var byID: [String: Int] = [:]
            for (index, row) in file.rows.enumerated() {
                byID[row.id] = index
            }
            positions[file.path] = byID
        }

        var kept: [PendingComment] = []
        var dropped = 0
        for comment in comments {
            if let byID = positions[comment.path], resolvedPositions(comment.anchorRowIDs, in: byID) != nil {
                kept.append(comment)
            } else {
                dropped += 1
            }
        }
        comments = kept
        lastDroppedCommentCount = dropped
        sortComments(diff: model)
    }

    /// The row positions a comment's anchor IDs resolve to in a file's rows,
    /// only where every one of them is still there and they still form one
    /// contiguous run — a comment whose lines have been split apart, or
    /// which no longer all exist, cannot be re-homed without guessing.
    private func resolvedPositions(_ anchorRowIDs: [String], in byID: [String: Int]) -> [Int]? {
        var found: [Int] = []
        for id in anchorRowIDs {
            guard let index = byID[id] else { return nil }
            found.append(index)
        }
        let sorted = found.sorted()
        for i in 1..<sorted.count where sorted[i] != sorted[i - 1] + 1 {
            return nil
        }
        return sorted
    }

    /// Orders the pending comments the way the diff draws them: by file in
    /// the order the change touches them, then by where the comment's first
    /// anchored row sits in that file. With no diff to order them by (a
    /// failed read), callers fall back to sorting by path alone.
    private func sortComments(diff: DiffModel? = nil) {
        guard let diff = diff ?? loadState.diff else {
            comments.sort { $0.path < $1.path }
            return
        }

        var fileOrder: [String: Int] = [:]
        var rowOrder: [String: [String: Int]] = [:]
        for (fileIndex, file) in diff.files.enumerated() {
            if fileOrder[file.path] == nil {
                fileOrder[file.path] = fileIndex
            }
            var byID: [String: Int] = [:]
            for (rowIndex, row) in file.rows.enumerated() {
                byID[row.id] = rowIndex
            }
            rowOrder[file.path] = byID
        }

        func rank(_ comment: PendingComment) -> (Int, Int) {
            let fileRank = fileOrder[comment.path] ?? Int.max
            let rowRank = comment.anchorRowIDs.first.flatMap { rowOrder[comment.path]?[$0] } ?? Int.max
            return (fileRank, rowRank)
        }

        comments.sort { a, b in
            let (fa, ra) = rank(a)
            let (fb, rb) = rank(b)
            if fa != fb { return fa < fb }
            if ra != rb { return ra < rb }
            return a.path < b.path
        }
    }
}
