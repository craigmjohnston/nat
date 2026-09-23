import Foundation

/// The active project's ad hoc sessions — `nat session-list`, read on the
/// same cadence `ReviewStatsStore`'s PR-readiness reading rides (every plan
/// reload and the poll's own tick), through `AppModel.updateReviewStats`.
///
/// A failed read is quiet, mirroring `ReviewStatsStore`: the rail simply
/// keeps whatever it last held, and the next reload tries again on its own.
@MainActor
@Observable
public final class SessionStore {
    /// The active project's sessions, as `session-list` last read them.
    public private(set) var sessions: [Session] = []

    private let client: NatClientProtocol

    public init(client: NatClientProtocol = NatClient()) {
        self.client = client
    }

    /// Bring `sessions` in line with a fresh `session-list` read. A read
    /// that fails leaves the last one standing.
    public func update(projectID: String) async {
        guard let fresh = try? await client.sessionList(projectID: projectID) else { return }
        sessions = fresh
    }

    /// Clear everything, as if nothing had ever been fetched.
    public func clear() {
        sessions = []
    }
}
