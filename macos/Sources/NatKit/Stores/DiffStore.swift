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

    private let client: NatClientProtocol
    private var isFetching = false
    private var projectID: String?
    private var sliceRef: String?

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
        self.projectID = projectID
        self.sliceRef = sliceRef
        await load()
    }

    /// Re-read the current slice's branch. A no-op with nothing fetched yet.
    public func refresh() async {
        guard !isFetching, projectID != nil, sliceRef != nil else { return }
        await load()
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
    }

    // MARK: - Private

    private func load() async {
        guard let projectID, let sliceRef else { return }
        isFetching = true
        defer { isFetching = false }

        loadState = .loading
        // A re-read's marks are the previous diff's, not the one about to
        // replace it — cleared here regardless of how the read turns out.
        viewedFiles = []
        collapsedFiles = []

        do {
            let diff = try await client.sliceDiff(projectID: projectID, sliceRef: sliceRef)
            loadState = .loaded(buildDiffModel(from: diff))
        } catch {
            loadState = .failed(error.localizedDescription)
        }
    }
}
