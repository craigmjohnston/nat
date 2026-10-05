import Foundation

/// Every open project's `nat pr-status` reading — readiness, failing checks,
/// conflicts — held per project, app-wide, so a pull request's marks are drawn
/// in every project whether or not it is the one open, and survive switching
/// away from it and back.
///
/// A project's reading is replaced only by a newer reading of that project: a
/// slice's mark goes when one arrives that no longer says it. A reading that
/// fails leaves the last one standing — stale news about an open pull request
/// beats no news, and the next plan read retries on its own. Each reading that
/// lands is written to the project's read cache (`PlanCaching`), and `restore`
/// reads it back at launch, so the marks are there before the first fresh
/// reading.
@MainActor
@Observable
public final class PRStatusStore {
    /// The last reading of each project, by project ID — absent for one never
    /// read.
    public private(set) var readings: [String: PRReading] = [:]

    private let client: NatClientProtocol
    private let cache: PlanCaching

    public init(client: NatClientProtocol = NatClient(), cache: PlanCaching = DiskPlanCache()) {
        self.client = client
        self.cache = cache
    }

    /// One project's reading — empty for one never read.
    public func reading(projectID: String) -> PRReading {
        readings[projectID] ?? .empty
    }

    /// Every project's marks, by slice id — slice ids are page IDs, unique
    /// across projects, so one map serves the sidebar's rows of all of them.
    public var marks: [String: PRMarks] {
        readings.values.reduce(into: [:]) { all, reading in
            all.merge(reading.marks) { _, new in new }
        }
    }

    /// Takes a fresh reading of one project and replaces its last one with
    /// it, recording it in the cache; a reading that fails changes nothing.
    public func update(projectID: String) async {
        guard let doc = try? await client.prStatus(projectID: projectID) else { return }
        readings[projectID] = PRReading(doc)
        await cache.writePRStatus(doc, projectID: projectID)
    }

    /// Puts a project's cached reading up, where nothing fresher has landed
    /// for it — at launch, before the first fresh reading.
    public func restore(projectID: String) async {
        guard readings[projectID] == nil, let doc = await cache.readPRStatus(projectID: projectID) else { return }
        // A fresh reading that landed while the cache was read wins.
        guard readings[projectID] == nil else { return }
        readings[projectID] = PRReading(doc)
    }

    /// Forgets a project whose tab has gone.
    public func forget(projectID: String) {
        readings.removeValue(forKey: projectID)
    }
}
