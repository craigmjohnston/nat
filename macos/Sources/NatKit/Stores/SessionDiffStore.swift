import Foundation

/// An ad hoc session's Diff tab: fetching `nat session-diff` on demand and
/// the screen's own viewed/collapsed marks over it — `DiffStore`'s own
/// shape, cut down to what a session's diff actually is. A session has no
/// hand-back to approve and no review to leave comments into (there is no
/// pull request this diff is itself the review of), so neither of
/// `DiffStore`'s own machinery for those has anything to attach to here.
@MainActor
@Observable
public final class SessionDiffStore {
    public private(set) var loadState: DiffLoadState = .idle
    public private(set) var isRefreshing = false
    public private(set) var viewedFiles: Set<String> = []
    public private(set) var collapsedFiles: Set<String> = []

    private let client: NatClientProtocol
    private var isFetching = false
    private var sessionID: String?

    public init(client: NatClientProtocol = NatClient()) {
        self.client = client
    }

    /// Fetch a session's diff, unless one is already loaded (or loading) for
    /// that same session — mirrors `DiffStore.fetch(projectID:sliceRef:)`.
    public func fetch(projectID: String, sessionID: String) async {
        guard !isFetching else { return }
        if self.sessionID == sessionID, case .loaded = loadState {
            return
        }
        if let previous = self.sessionID, previous != sessionID {
            viewedFiles = []
            collapsedFiles = []
        }
        self.sessionID = sessionID
        await load(projectID: projectID, sessionID: sessionID)
    }

    /// Re-read the current session's diff — the Diff tab's own refresh,
    /// following the branch the way the slice Diff tab's does.
    public func refresh(projectID: String) async {
        guard !isFetching, let sessionID else { return }
        await load(projectID: projectID, sessionID: sessionID, isRefresh: true)
    }

    public func isViewed(_ path: String) -> Bool { viewedFiles.contains(path) }
    public func isCollapsed(_ path: String) -> Bool { collapsedFiles.contains(path) }

    public func toggleViewed(_ path: String) {
        if viewedFiles.contains(path) {
            viewedFiles.remove(path)
        } else {
            viewedFiles.insert(path)
            collapsedFiles.insert(path)
        }
    }

    public func toggleCollapsed(_ path: String) {
        if collapsedFiles.contains(path) {
            collapsedFiles.remove(path)
        } else {
            collapsedFiles.insert(path)
        }
    }

    public func clear() {
        loadState = .idle
        viewedFiles = []
        collapsedFiles = []
        sessionID = nil
    }

    private func load(projectID: String, sessionID: String, isRefresh: Bool = false) async {
        isFetching = true
        if isRefresh {
            isRefreshing = true
        } else if case .loaded = loadState {
            // Keep whatever is on screen; only the busy mark says a read is
            // running.
            isRefreshing = true
        } else {
            loadState = .loading
        }
        do {
            let diff = try await client.sessionDiff(projectID: projectID, sessionID: sessionID, branch: nil)
            loadState = .loaded(buildDiffModel(from: diff))
        } catch {
            loadState = .failed(error.localizedDescription, previous: loadState.diff)
        }
        isRefreshing = false
        isFetching = false
    }
}
