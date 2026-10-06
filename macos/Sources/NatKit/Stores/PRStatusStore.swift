import Foundation

/// Every open project's `nat pr-status` reading — readiness, failing checks,
/// conflicts — held per project, app-wide, so a pull request's marks are drawn
/// in every project whether or not it is the one open, and survive switching
/// away from it and back.
///
/// Nothing here reads GitHub: `GitHubReadingStore` takes the one batched
/// reading of every open project and hands each project's part of it here
/// (`apply`). A project's reading is replaced only by a newer reading of that
/// project: a slice's mark goes when one arrives that no longer says it. A
/// reading that fails never arrives, so the last one stands — stale news about
/// an open pull request beats no news. Each reading that lands is written to
/// the project's read cache (`PlanCaching`), and `restore` reads it back at
/// launch, so the marks are there before the first fresh reading.
@MainActor
@Observable
public final class PRStatusStore {
    /// The last reading of each project, by project ID — absent for one never
    /// read.
    public private(set) var readings: [String: PRReading] = [:]

    private let cache: PlanCaching

    public init(cache: PlanCaching = DiskPlanCache()) {
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

    /// Replaces a project's last reading with a fresh one, recording it in
    /// the cache; one equal to the last publishes nothing. The fresh one
    /// holds the last one's failures over checks now running
    /// (`PRReading.heldFailingChecks`) — never cached, so a launch starts
    /// with none held.
    public func apply(_ doc: PRStatusDoc, projectID: String) async {
        let reading = PRReading(doc, after: readings[projectID])
        guard readings[projectID] != reading else { return }
        readings[projectID] = reading
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
