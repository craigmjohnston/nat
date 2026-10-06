import Foundation
import SwiftUI

/// The state of loading a slice's pull request.
///
/// A failed read keeps whatever pull request it replaced — the same rule
/// `DiffLoadState` follows and for the same reason: a read that failed is a
/// reading that did not happen rather than a pull request that went away. Blanking the pane over it would throw the user
/// out of a conversation they were reading. What it carries is stale and is
/// said to be — the view draws the error over the reading it kept — and only
/// a read with nothing ever behind it (`previous == nil`) has an empty pane
/// to show.
public enum PRLoadState: Equatable, Sendable {
    case idle
    case loading
    case loaded(PRDetail)
    case failed(String, previous: PRDetail?)

    public var pr: PRDetail? {
        switch self {
        case .loaded(let pr):
            return pr
        case .failed(_, let previous):
            return previous
        case .idle, .loading:
            return nil
        }
    }

    public var errorMessage: String? {
        if case .failed(let message, _) = self { return message }
        return nil
    }

    public var isLoading: Bool {
        if case .loading = self { return true }
        return false
    }
}

/// Which re-run or cancel control a call is under way from: the heading's
/// two, or one check row's.
public enum ChecksActionSource: Hashable, Sendable {
    case rerunAll
    case cancelAll
    case rerun(String)
    case cancel(String)
}

/// What the PR section says after a re-run or cancel: what nat reports doing,
/// or its refusal.
public struct ChecksActionNotice: Equatable, Sendable {
    public let text: String
    public let isError: Bool

    public init(text: String, isError: Bool) {
        self.text = text
        self.isError = isError
    }
}

/// Manages a slice's pull request: fetching it on demand, taking each fresh
/// reading of it the batched GitHub reading brings while the tab showing it
/// is visible (`applyDetail`, `GitHubReadingStore`'s `--detail`), and the
/// actions taken on it — merge, comment, reviewers, re-run and cancel — each
/// of which asks for a settle read (`settle`) rather than reading it again
/// itself.
///
/// Nothing here is written to Notion — the slice was marked Done as its pull
/// request was opened, and everything this store does is either a reading of
/// GitHub or the merge itself.
@MainActor
@Observable
public final class PRStore {
    public private(set) var loadState: PRLoadState = .idle

    /// Whether a read is in flight over a pull request already on screen —
    /// a Retry. The view draws it as a busy mark in a slot it reserves either
    /// way, so a reading never moves a row; `loadState`'s own `.loading` is
    /// the other case, the one with nothing to keep.
    public private(set) var isRefreshing = false

    /// The re-run or cancel control a call is under way from, nil with none —
    /// while set, every such control is disabled and this one spins.
    public private(set) var checksActionSource: ChecksActionSource?
    /// What the last re-run or cancel did, or why nat refused it — cleared
    /// when another slice's pull request is fetched.
    public private(set) var checksNotice: ChecksActionNotice?

    private let client: NatClientProtocol
    /// Asks for the batched reading's settle read, after an action changed
    /// the pull request on GitHub.
    private let settle: @MainActor () -> Void
    private var isFetching = false
    /// Whether the tab showing the pull request is on screen — what makes
    /// its pull request the batched reading's detail.
    public private(set) var isVisible = false
    private var projectID: String?
    private var sliceRef: String?
    /// The ad hoc session `sliceRef` is a pull request URL of, when it is one
    /// rather than a slice — what `load()` reads it through.
    private var sessionID: String?

    /// Pull requests already read this session, by slice ref — what makes
    /// switching back to a slice already viewed show its pull request
    /// instantly rather than behind a spinner again. Evicted for a slice
    /// whenever a read of it fails, so a later switch back to it does not
    /// serve a reading already known to be stale.
    private var prCache: [String: PRDetail] = [:]

    /// What the user last saw of each slice's pull request — its head commit
    /// (`SeenMemory`, `.pr`), what the section's Updated badge reads.
    private let seen: SeenMemory
    /// Bumped on every seen mark written, so a view reading `badge` redraws:
    /// the memory itself is not observable.
    private var seenTick = 0
    /// The one item the PR section's snapshot holds.
    public nonisolated static let seenHead = "head"

    /// - Parameter settle: asks for the batched reading's settle read — the
    ///   app model's `GitHubReadingStore.scheduleSettle`; nothing by default.
    public init(
        client: NatClientProtocol = NatClient(), seen: SeenMemory = .inMemory(),
        settle: @escaping @MainActor () -> Void = {}
    ) {
        self.client = client
        self.seen = seen
        self.settle = settle
    }

    /// Fetch the pull request for a slice, unless it is already loaded (or
    /// loading) for that same slice. A slice whose pull request was already
    /// read this session shows that reading instantly rather than behind a
    /// spinner again, and is not read again here: the batched reading's
    /// detail keeps it fresh while its tab is visible, so `pr-view` runs only
    /// on a tab's first open.
    ///
    /// With `sessionID`, `sliceRef` is instead the URL of one of that ad hoc
    /// session's pull requests, read with `nat pr-view --session` — the
    /// session has no slice to name it by. A URL never collides with a
    /// slice's ID in the cache, so the two share one store.
    public func fetch(projectID: String, sliceRef: String, sessionID: String? = nil) async {
        guard !isFetching else { return }
        if self.projectID == projectID, self.sliceRef == sliceRef, self.sessionID == sessionID, case .loaded = loadState {
            return
        }
        if let previous = self.sliceRef, previous != sliceRef {
            checksNotice = nil
        }
        self.projectID = projectID
        self.sliceRef = sliceRef
        self.sessionID = sessionID

        if let cached = prCache[sliceRef] {
            loadState = .loaded(cached)
            return
        }

        // Nothing cached for this slice: clear whatever the previous slice
        // left in `loadState` before reading, so `load()` blanks the screen
        // rather than showing the wrong slice's pull request under this
        // one's name for the moment the read is in flight.
        loadState = .idle
        await load()
    }

    /// Re-read the current slice's pull request with `pr-view` — the Retry
    /// after a first open that failed. A no-op with nothing fetched yet.
    public func refresh() async {
        guard !isFetching, projectID != nil, sliceRef != nil else { return }
        await load()
    }

    /// Merge the pull request on show, then ask for the settle read that
    /// brings it back merged. A refusal from gh propagates and asks for
    /// nothing — the pull request is still there and still open, exactly as
    /// it was.
    public func merge() async throws {
        guard let projectID, let sliceRef else { return }
        try await client.prMerge(projectID: projectID, sliceRef: sliceRef)
        settle()
    }

    /// Leave a top-level comment on the pull request on show, then ask for
    /// the settle read that brings it into the conversation — both PR-tab
    /// composers (the one pinned at the tab's own foot and the compact one
    /// at the conversation's foot) call this; neither writes anywhere but
    /// through `nat pr-comment`, so a failed send leaves nothing posted and
    /// propagates the error for the view to show inline. Blank text is a
    /// no-op, since a comment with nothing said is not one to send.
    public func comment(text: String) async throws {
        guard let projectID, let sliceRef else { return }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        try await client.prComment(projectID: projectID, sliceRef: sliceRef, body: trimmed)
        await reread()
    }

    /// Replace the description of the pull request on show (`nat pr-edit`),
    /// then read it again at once (`pr-view`), so the description the editor
    /// gives way to is the one just saved rather than the one a settle read
    /// would bring five seconds later. A refusal from gh propagates for the
    /// editor to show, nothing read; blank text is a no-op.
    public func editDescription(_ text: String) async throws {
        guard let projectID, let sliceRef else { return }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        try await client.prEdit(projectID: projectID, sliceRef: sliceRef, body: trimmed)
        await load()
    }

    /// After a write to the pull request: the settle read while it is open,
    /// and a `pr-view` of its own once it is merged or closed — the batched
    /// reading reads a settled pull request no more (`shouldRead`), so a
    /// comment left on one would otherwise never be seen.
    private func reread() async {
        if shouldRead {
            settle()
        } else {
            await load()
        }
    }

    /// Who is asked to review the pull request on show and who else could
    /// be — `nat pr-reviewers`, a read for the reviewer picker. Nil with
    /// nothing fetched, or for an ad hoc session's pull request, which has
    /// no slice to name it by.
    public func reviewers() async throws -> PRReviewers? {
        guard let projectID, let sliceRef, sessionID == nil else { return nil }
        return try await client.prReviewers(projectID: projectID, sliceRef: sliceRef, add: [], remove: [])
    }

    /// Ask (`add`) or stop asking (`remove`) for reviews on the pull request
    /// on show — nat answers with what the edit did, reading nothing back —
    /// then ask for the settle read that brings its requested reviewers. A
    /// refusal from gh propagates for the view to show, nothing asked.
    @discardableResult
    public func editReviewers(add: [String] = [], remove: [String] = []) async throws -> PRReviewers? {
        guard let projectID, let sliceRef, sessionID == nil else { return nil }
        let answer = try await client.prReviewers(projectID: projectID, sliceRef: sliceRef, add: add, remove: remove)
        settle()
        return answer
    }

    /// Re-run the pull request's checks (`nat slice-checks-rerun`) from the
    /// control `source`, say what nat cancelled and re-ran — or why it
    /// refused — and ask for the settle read. One call at a time.
    public func rerunChecks(_ mode: ChecksRerunMode, from source: ChecksActionSource) async {
        await checksAction(from: source) { client, projectID, sliceRef in
            try await client.sliceChecksRerun(projectID: projectID, sliceRef: sliceRef, mode: mode)
        }
    }

    /// Cancel the runs still going behind the pull request's checks — all, or
    /// the named checks' — (`nat slice-checks-cancel`), as `rerunChecks` does.
    public func cancelChecks(_ checks: [String], from source: ChecksActionSource) async {
        await checksAction(from: source) { client, projectID, sliceRef in
            try await client.sliceChecksCancel(projectID: projectID, sliceRef: sliceRef, checks: checks)
        }
    }

    private func checksAction(
        from source: ChecksActionSource,
        _ call: (NatClientProtocol, String, String) async throws -> ChecksActionResult
    ) async {
        guard checksActionSource == nil, let projectID, let sliceRef, sessionID == nil else { return }
        checksActionSource = source
        checksNotice = nil
        do {
            let result = try await call(client, projectID, sliceRef)
            checksNotice = ChecksActionNotice(text: checksActionNotice(result), isError: false)
        } catch {
            checksNotice = ChecksActionNotice(text: SliceActionTracker.message(for: error), isError: true)
        }
        checksActionSource = nil
        settle()
    }

    /// Drop everything, as if nothing had ever been fetched.
    public func clear() {
        isVisible = false
        checksNotice = nil
        loadState = .idle
        projectID = nil
        sliceRef = nil
        sessionID = nil
        prCache = [:]
    }

    // MARK: - The batched reading's detail

    /// Whether the pull request is still worth reading: it is open. Checks
    /// starting late, review decisions, mergeability and a merge made on
    /// GitHub directly all keep arriving until it settles as merged or closed
    /// — an empty check list is no sign of a settled one, only of one GitHub
    /// has not started on yet.
    public var shouldRead: Bool {
        guard let pr = loadState.pr else { return false }
        return pr.state != PRLifecycleState.merged && pr.state != PRLifecycleState.closed
    }

    /// The pull request the batched reading reads in full for this store:
    /// the one on show, while its tab is visible and it is still open.
    public var detailURL: String? {
        guard isVisible, shouldRead, let url = loadState.pr?.url, !url.isEmpty else { return nil }
        return url
    }

    /// The view's PR tab came on screen (`true`) or went (`false`).
    public func setVisible(_ visible: Bool) {
        isVisible = visible
    }

    /// Takes the batched reading's detail where it is the pull request on
    /// show — what keeps an open PR tab fresh with no `pr-view` of its own.
    /// One for any other pull request is not this store's.
    public func applyDetail(_ pr: PRDetail) {
        guard let projectID, let sliceRef, let shown = loadState.pr,
              Self.sameURL(shown.url, pr.url) else { return }
        prCache[sliceRef] = pr
        loadState = .loaded(pr)
        if sessionID == nil, !pr.headRefOid.isEmpty {
            seen.baseline(projectID: projectID, sliceID: sliceRef, .pr, [Self.seenHead: pr.headRefOid])
            seenTick += 1
        }
    }

    /// Whether two pull request URLs name the same one, whatever the case or
    /// a trailing slash — nat normalises the URL it reads by, not the one it
    /// prints back.
    private static func sameURL(_ a: String, _ b: String) -> Bool {
        func normal(_ url: String) -> String {
            var s = url.lowercased()
            while s.hasSuffix("/") { s.removeLast() }
            return s
        }
        return normal(a) == normal(b)
    }

    // MARK: - Updated

    /// The PR section's badge for a slice: Updated where the pull request
    /// this store holds is that slice's and its head has moved since the user
    /// last opened the section — nil otherwise, and nil before any head was
    /// read.
    public func badge(sliceID: String) -> SeenBadge? {
        _ = seenTick
        guard let projectID, sessionID == nil, sliceRef == sliceID,
              let head = loadState.pr?.headRefOid, !head.isEmpty
        else { return nil }
        return seen.badge(projectID: projectID, sliceID: sliceID, .pr, item: Self.seenHead, fingerprint: head)
    }

    /// The user has opened a slice's PR section: its head as read is seen.
    public func markSeen(sliceID: String) {
        guard badge(sliceID: sliceID) != nil, let projectID, let head = loadState.pr?.headRefOid else { return }
        seen.markSeen(projectID: projectID, sliceID: sliceID, .pr, item: Self.seenHead, fingerprint: head)
        seenTick += 1
    }

    // MARK: - Private

    private func load() async {
        guard let projectID, let sliceRef else { return }
        isFetching = true
        defer { isFetching = false }
        // A background re-read (a poll, a merge, a posted comment, or an
        // explicit refresh) never blanks the screen first — only a read with
        // nothing already on it to show blocks behind a skeleton.
        let previous = loadState.pr
        if previous == nil {
            loadState = .loading
        } else {
            isRefreshing = true
        }
        defer { isRefreshing = false }
        do {
            let pr: PRDetail
            if let sessionID {
                pr = try await client.sessionPRView(projectID: projectID, sessionID: sessionID, prURL: sliceRef)
            } else {
                pr = try await client.prView(projectID: projectID, sliceRef: sliceRef)
            }
            prCache[sliceRef] = pr
            loadState = .loaded(pr)
            // The first reading of a slice's pull request is its first look:
            // nothing is Updated before the user has seen a head to compare
            // with. A session's pull request, and a head nat did not say,
            // take no part.
            if sessionID == nil, !pr.headRefOid.isEmpty {
                seen.baseline(projectID: projectID, sliceID: sliceRef, .pr, [Self.seenHead: pr.headRefOid])
                seenTick += 1
            }
        } catch {
            // The reading is kept and said to be stale rather than dropped:
            // see `PRLoadState`. The cache goes even so, so a later switch
            // back to this slice reads again rather than serving one already
            // known to be stale.
            prCache.removeValue(forKey: sliceRef)
            loadState = .failed(error.localizedDescription, previous: previous)
        }
    }
}
