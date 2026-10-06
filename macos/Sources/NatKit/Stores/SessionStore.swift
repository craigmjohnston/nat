import Foundation

/// The active project's ad hoc sessions — `nat session-list`, read when a
/// project is activated, after a session is launched, ended or discarded,
/// and after every GitHub reading lands (`AppModel.deliver`): the listing
/// asks GitHub nothing itself, each row's pull requests being the ones that
/// reading kept on disk.
///
/// A failed read is quiet, mirroring `ReviewStatsStore`: the rail simply
/// keeps whatever it last held, and the next reading tries again on its own.
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
