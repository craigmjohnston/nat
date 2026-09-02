import Foundation
import SwiftUI

/// Manages detailed slice information with in-memory caching.
///
/// This store fetches slice details on demand, caches them by slice ID, and invalidates
/// the cache when the plan is refreshed (indicating the slice page may have changed).
@MainActor
@Observable
public final class SliceDetailStore {
    /// Loading state for the current fetch.
    public private(set) var loadState: LoadState = .idle

    private let projectID: String
    private let client: NatClientProtocol
    private var cache: [String: SliceDetail] = [:]
    private var isFetching = false

    public init(projectID: String, client: NatClientProtocol = NatClient()) {
        self.projectID = projectID
        self.client = client
    }

    /// Fetch or return cached slice details.
    public func fetch(sliceRef: String) async {
        guard !isFetching else { return }

        // Check cache first
        if cache[sliceRef] != nil {
            loadState = .loaded(ProjectInfo(
                project: Project(id: "", name: "", conventions: ""),
                milestones: [],
                slices: []
            ))
            return
        }

        isFetching = true
        defer { isFetching = false }

        loadState = .loading

        do {
            let detail = try await client.sliceShow(projectID: projectID, sliceRef: sliceRef)
            cache[sliceRef] = detail
            // Store in the load state as a marker of success
            // (we'll access via getDetail below)
            loadState = .loaded(ProjectInfo(
                project: Project(id: detail.id, name: detail.name, conventions: ""),
                milestones: [],
                slices: []
            ))
        } catch {
            // Keep previous state on error
            let previousInfo = loadState.projectInfo
            loadState = .failed(error.localizedDescription, previous: previousInfo)
        }
    }

    /// Get the most recently fetched detail, or cached if available.
    public func getDetail(sliceRef: String) -> SliceDetail? {
        return cache[sliceRef]
    }

    /// Invalidate the cache (called after plan refresh).
    public func invalidateCache() {
        cache.removeAll()
    }

    /// Clear all state.
    public func clear() {
        cache.removeAll()
        loadState = .idle
        isFetching = false
    }
}
