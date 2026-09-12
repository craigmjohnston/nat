import Foundation
import SwiftUI

/// A protocol for providing nat client functionality (allows injection for testing).
public protocol NatClientProtocol: Sendable {
    func info(projectID: String) async throws -> ProjectInfo
    func status() async throws -> [AgentStatus]
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail
    func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff
    func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc
    func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult
    func agentSend(projectID: String, sliceRef: String, text: String) async throws -> Void
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String
    func prView(projectID: String, sliceRef: String) async throws -> PRDetail
    func prStatus(projectID: String) async throws -> PRStatusDoc
    func prMerge(projectID: String, sliceRef: String) async throws -> Void
    func prComment(projectID: String, sliceRef: String, body: String) async throws -> Void
    func workshopLaunch(projectID: String, model: String?, effort: String?, request: String?) async throws -> WorkshopLaunchResult
    func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult
    func configShow() async throws -> ConfigDoc
    func configSet(key: String, value: String) async throws -> Void
}

extension NatClientProtocol {
    /// The whole-branch diff, without naming a commit — `sliceDiff(projectID:sliceRef:commit:)`
    /// with `commit: nil`, kept as the two-argument spelling every caller
    /// asking for "the diff" (rather than one commit of it) already uses.
    public func sliceDiff(projectID: String, sliceRef: String) async throws -> SliceDiff {
        try await sliceDiff(projectID: projectID, sliceRef: sliceRef, commit: nil)
    }
}

// Make NatClient conform to the protocol
extension NatClient: NatClientProtocol {}

/// Manages loading and refreshing project information.
///
/// This store coordinates loading project data from the nat CLI. It handles concurrent load
/// coalescing, error states with fallback to the previous successful load, and refresh
/// operations that keep showing the previous data while reloading.
@MainActor
@Observable
public final class ProjectStore {
    public private(set) var projectID: String
    public private(set) var state: LoadState = .idle
    private let client: NatClientProtocol
    private let cache: PlanCaching
    private var isLoadInFlight = false

    /// Whether the cache has already been asked about this project. It is
    /// asked once, before the first read lands: after that the plan in hand
    /// is fresher than anything on disk, so a later load has nothing to seed
    /// from and every refresh that fails keeps what it has anyway.
    private var cacheConsulted = false

    public init(
        projectID: String,
        client: NatClientProtocol = NatClient(),
        cache: PlanCaching = DiskPlanCache()
    ) {
        self.projectID = projectID
        self.client = client
        self.cache = cache
    }

    /// Load project information.
    ///
    /// If a load is already in flight, this call is ignored (concurrent load coalescing).
    /// On error, the previous successful load (if any) is kept in the state.
    public func load() async {
        guard !isLoadInFlight else {
            return
        }

        isLoadInFlight = true
        defer { isLoadInFlight = false }

        // Only a first load shows as loading, and only where there is
        // nothing to show in the meantime: the last plan this project was
        // read as is on disk, and seeding it here is what puts the board on
        // screen at once. From there it is the ordinary refresh — a plan
        // already in hand stays up until the fresh one lands, so the board
        // never blanks under a poll, a nudge, or a launch.
        if state.projectInfo == nil, !cacheConsulted {
            cacheConsulted = true
            if let cached = await cache.read(projectID: projectID) {
                state = .loaded(cached)
            }
        }
        if state.projectInfo == nil {
            state = .loading
        }

        do {
            let info = try await client.info(projectID: projectID)
            state = .loaded(info)
            // Every read that lands is what the next launch starts from.
            await cache.write(info, projectID: projectID)
        } catch {
            let previousInfo = state.projectInfo
            state = .failed(error.localizedDescription, previous: previousInfo)
        }
    }

    /// Refresh project information, keeping the previous data visible while reloading.
    ///
    /// This is the same as load() but more explicitly named for refresh operations.
    /// If a load is already in flight, this call is ignored.
    public func refresh() async {
        await load()
    }
}
