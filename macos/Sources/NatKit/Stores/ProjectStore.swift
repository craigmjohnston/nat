import Foundation
import SwiftUI

/// A protocol for providing nat client functionality (allows injection for testing).
public protocol NatClientProtocol: Sendable {
    func info(projectID: String) async throws -> ProjectInfo
    func status() async throws -> [AgentStatus]
    func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail
    func sliceDiff(projectID: String, sliceRef: String) async throws -> SliceDiff
    func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult
    func agentInterrupt(projectID: String, sliceRef: String) async throws -> Void
    func agentSend(projectID: String, sliceRef: String, text: String) async throws -> Void
    func sliceApprove(projectID: String, sliceRef: String) async throws -> String
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
    private var isLoadInFlight = false

    public init(projectID: String, client: NatClientProtocol = NatClient()) {
        self.projectID = projectID
        self.client = client
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

        state = .loading

        do {
            let info = try await client.info(projectID: projectID)
            state = .loaded(info)
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
