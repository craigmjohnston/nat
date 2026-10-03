import Foundation
import SwiftUI

/// The state of one container's `container-show` read, keyed by container id
/// in `ContainerStore`'s cache. A failed re-read keeps what was read before,
/// as the pull request screen does: a stale reading of the right container
/// is still about the right container.
public enum ContainerLoadState: Equatable, Sendable {
    case idle
    case loading(stale: ContainerShow?)
    case loaded(ContainerShow)
    case failed(String, previous: ContainerShow?)

    public var show: ContainerShow? {
        switch self {
        case .idle: return nil
        case .loading(let stale): return stale
        case .loaded(let show): return show
        case .failed(_, let previous): return previous
        }
    }

    public var errorMessage: String? {
        if case .failed(let message, _) = self { return message }
        return nil
    }
}

/// Every container's detail for one source project, cached by id so
/// reselecting one already read draws at once while a fresh read runs
/// behind it — `SliceDetailStore`'s shape.
@MainActor
@Observable
public final class ContainerStore {
    private let projectID: String
    private let client: NatClientProtocol
    private var cache: [String: ContainerLoadState] = [:]
    private var fetching: Set<String> = []

    public init(projectID: String, client: NatClientProtocol = NatClient()) {
        self.projectID = projectID
        self.client = client
    }

    /// The state for one container — `.idle` for one never read.
    public func state(for containerID: String) -> ContainerLoadState {
        cache[containerID] ?? .idle
    }

    /// Read one container afresh, whatever is cached shown meanwhile. A
    /// read already in flight for it is left alone.
    public func fetch(containerID: String) async {
        guard !fetching.contains(containerID) else { return }
        fetching.insert(containerID)
        defer { fetching.remove(containerID) }

        let stale = cache[containerID]?.show
        cache[containerID] = .loading(stale: stale)
        do {
            cache[containerID] = .loaded(try await client.containerShow(projectID: projectID, containerID: containerID))
        } catch {
            cache[containerID] = .failed(error.localizedDescription, previous: stale)
        }
    }

    /// Drops every cached reading but the one on screen, after a plan
    /// refresh — see `SliceDetailStore.invalidateCache(keeping:)`.
    public func invalidateCache(keeping containerID: String? = nil) {
        guard let containerID, let kept = cache[containerID] else {
            cache.removeAll()
            return
        }
        cache = [containerID: kept]
    }
}
