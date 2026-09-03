import Foundation
import SwiftUI

/// The state of one slice's detail read, keyed by slice ref in
/// `SliceDetailStore`'s own cache.
///
/// Unlike `DiffLoadState`/`PRLoadState` (which drop what they were showing
/// on a failed read, since a diff or a pull request is one external
/// reading at one moment), a slice's brief is Notion content the same way
/// the plan itself is — `.failed` here keeps whatever was read before, the
/// same rule `LoadState` follows for the plan, so a transient read error
/// never blanks a brief that was showing a moment ago.
public enum SliceDetailLoadState: Equatable, Sendable {
    case idle
    case loading(stale: SliceDetail?)
    case loaded(SliceDetail)
    case failed(String, previous: SliceDetail?)

    public var detail: SliceDetail? {
        switch self {
        case .idle:
            return nil
        case .loading(let stale):
            return stale
        case .loaded(let detail):
            return detail
        case .failed(_, let previous):
            return previous
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

/// Manages every slice's detail for one project, cached by slice ref so
/// re-selecting a slice already read this session renders instantly instead
/// of behind a spinner.
///
/// A `fetch` always reads fresh — this is the store `BriefTabView` reads
/// through, and a slice's brief, dependencies and branch are exactly the
/// kind of thing that can have changed since it was last shown — but it
/// shows whatever is cached immediately while that read is in flight, and
/// only replaces it once the read actually lands: the cache is what makes
/// re-selecting a slice instant, the background read is what keeps it
/// honest.
@MainActor
@Observable
public final class SliceDetailStore {
    private let projectID: String
    private let client: NatClientProtocol
    private var cache: [String: SliceDetailLoadState] = [:]
    private var fetchingRefs: Set<String> = []

    public init(projectID: String, client: NatClientProtocol = NatClient()) {
        self.projectID = projectID
        self.client = client
    }

    /// The current state for one slice — `.idle` for a slice never fetched
    /// (or dropped by `invalidateCache`).
    public func state(for sliceRef: String) -> SliceDetailLoadState {
        cache[sliceRef] ?? .idle
    }

    /// Read one slice's detail. Whatever is already cached for it (a
    /// previous `.loaded` or `.failed` reading) is left in `state(for:)`
    /// throughout — the caller sees it continuously, with no `.idle`/blank
    /// gap — while the read itself runs in the background. A fetch already
    /// in flight for this slice is left alone rather than started twice.
    public func fetch(sliceRef: String) async {
        guard !fetchingRefs.contains(sliceRef) else { return }
        fetchingRefs.insert(sliceRef)
        defer { fetchingRefs.remove(sliceRef) }

        let stale = cache[sliceRef]?.detail
        cache[sliceRef] = .loading(stale: stale)

        do {
            let detail = try await client.sliceShow(projectID: projectID, sliceRef: sliceRef)
            cache[sliceRef] = .loaded(detail)
        } catch {
            cache[sliceRef] = .failed(error.localizedDescription, previous: stale)
        }
    }

    /// Drops every cached reading except (optionally) one — called after a
    /// plan refresh (nudge or poll), since any slice's page may have changed
    /// underneath a reading of it taken before the refresh landed.
    /// `keeping` is the slice currently on screen, if any: it is left alone
    /// rather than dropped, because nothing would re-fetch it until the user
    /// actually navigates away and back, and dropping it here would blank a
    /// brief the user is looking at right now for no reason — every other
    /// cached slice is one nothing is currently reading, so clearing it costs
    /// nothing and it re-reads fresh next time it is actually selected.
    public func invalidateCache(keeping sliceRef: String? = nil) {
        guard let sliceRef, let kept = cache[sliceRef] else {
            cache.removeAll()
            return
        }
        cache = [sliceRef: kept]
    }

    /// Clear all state, as if nothing had ever been fetched.
    public func clear() {
        cache.removeAll()
        fetchingRefs.removeAll()
    }
}
