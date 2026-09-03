import Foundation
import SwiftUI

/// The state of loading a slice's pull request.
///
/// Unlike `LoadState` (which keeps the previous project on a failed refresh),
/// a failed read here drops whatever pull request it replaced — the same
/// rule `DiffLoadState` follows and for the same reason: a pull request is
/// one reading of GitHub at one moment, and leaving the last one on screen
/// under a failure would be showing the wrong state (mirrors the Go TUI's
/// `PRView.Fail`).
public enum PRLoadState: Equatable, Sendable {
    case idle
    case loading
    case loaded(PRDetail)
    case failed(String)

    public var pr: PRDetail? {
        if case .loaded(let pr) = self { return pr }
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

/// Manages a slice's pull request: fetching it on demand, polling it while
/// the tab showing it is open and there is still something to watch, and
/// merging it.
///
/// Nothing here is written to Notion — the slice was marked Done as its pull
/// request was opened, and everything this store does is either a reading of
/// GitHub or the merge itself.
@MainActor
@Observable
public final class PRStore {
    public private(set) var loadState: PRLoadState = .idle

    private let client: NatClientProtocol
    private let pollIntervalNanoseconds: UInt64
    private var isFetching = false
    private var isPolling = false
    private var pollTask: Task<Void, Never>?
    private var projectID: String?
    private var sliceRef: String?

    /// Pull requests already read this session, by slice ref — what makes
    /// switching back to a slice already viewed show its pull request
    /// instantly rather than behind a spinner again. Evicted for a slice
    /// whenever a read of it fails, so a later switch back to it does not
    /// serve a reading already known to be stale.
    private var prCache: [String: PRDetail] = [:]

    /// - Parameter pollIntervalNanoseconds: how long the poll loop sleeps
    ///   between readings — 15 seconds in the app, overridable so a test does
    ///   not have to wait 15 real seconds to see it fire twice.
    public init(client: NatClientProtocol = NatClient(), pollIntervalNanoseconds: UInt64 = 15 * 1_000_000_000) {
        self.client = client
        self.pollIntervalNanoseconds = pollIntervalNanoseconds
    }

    /// Fetch the pull request for a slice, unless it is already loaded (or
    /// loading) for that same slice. Switching to a different slice's pull
    /// request stops any poll left running for the one before — a poll that
    /// kept going would end up reading the new slice under the old one's
    /// name. A slice whose pull request was already read this session shows
    /// that reading instantly rather than behind a spinner again; the poll
    /// this store already runs (started by the caller right after `fetch`
    /// returns, same as any other first show) is what keeps it fresh from
    /// here, so this does not also re-read it.
    public func fetch(projectID: String, sliceRef: String) async {
        guard !isFetching else { return }
        if self.projectID == projectID, self.sliceRef == sliceRef, loadState.pr != nil {
            return
        }
        if let previous = self.sliceRef, previous != sliceRef {
            stopPolling()
        }
        self.projectID = projectID
        self.sliceRef = sliceRef

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

    /// Re-read the current slice's pull request. A no-op with nothing
    /// fetched yet.
    public func refresh() async {
        guard !isFetching, projectID != nil, sliceRef != nil else { return }
        await load()
    }

    /// Merge the pull request on show, then read it again so the screen says
    /// merged rather than going on showing the question it just answered.
    /// A refusal from gh propagates rather than being read again — the pull
    /// request is still there and still open, exactly as it was.
    public func merge() async throws {
        guard let projectID, let sliceRef else { return }
        try await client.prMerge(projectID: projectID, sliceRef: sliceRef)
        await refresh()
    }

    /// Leave a top-level comment on the pull request on show, then read it
    /// again so it appears in the conversation timeline — both PR-tab
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
        await refresh()
    }

    /// Drop everything, as if nothing had ever been fetched.
    public func clear() {
        stopPolling()
        loadState = .idle
        projectID = nil
        sliceRef = nil
        prCache = [:]
    }

    // MARK: - Polling

    /// Whether polling is still worth doing: an open pull request with a
    /// check still pending. Merged, closed, or nothing left pending stops
    /// it — a poll over a settled conversation would only ever read the same
    /// answer again, the same rule the Go TUI's own `prSettled` follows for
    /// the board's background reading.
    public var shouldPoll: Bool {
        guard let pr = loadState.pr else { return false }
        guard pr.state != PRLifecycleState.merged, pr.state != PRLifecycleState.closed else { return false }
        return checkRollup(pr.checks).outcome == .pending
    }

    /// Starts the poll loop if it is not already running and there is still
    /// something worth polling for. The view calls this while its PR tab is
    /// visible (and again after a load lands, since that is when
    /// `shouldPoll` might have just turned true or false) and `stopPolling`
    /// when the tab is hidden.
    public func startPolling() {
        guard !isPolling, shouldPoll else { return }
        isPolling = true
        pollTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: pollIntervalNanoseconds)
                guard !Task.isCancelled else { break }
                await self.refresh()
                guard self.shouldPoll else { break }
            }
            self.isPolling = false
        }
    }

    /// Stops the poll loop, if one is running.
    public func stopPolling() {
        pollTask?.cancel()
        pollTask = nil
        isPolling = false
    }

    // MARK: - Private

    private func load() async {
        guard let projectID, let sliceRef else { return }
        isFetching = true
        defer { isFetching = false }
        // A background re-read (merge, a posted comment, or an explicit
        // refresh) never blanks the screen first — only a read with nothing
        // already on it to show blocks behind a spinner.
        if loadState.pr == nil {
            loadState = .loading
        }
        do {
            let pr = try await client.prView(projectID: projectID, sliceRef: sliceRef)
            prCache[sliceRef] = pr
            loadState = .loaded(pr)
        } catch {
            prCache.removeValue(forKey: sliceRef)
            loadState = .failed(error.localizedDescription)
        }
    }
}
