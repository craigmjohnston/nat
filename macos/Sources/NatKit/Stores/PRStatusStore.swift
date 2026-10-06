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
///
/// Given a `cadence`, the store also reads each project on its own loop,
/// whatever is selected or on screen: after every reading of a project —
/// whoever asked for it — it sleeps the cadence that reading calls for and
/// reads again. **Fast** while any open pull request's checks are still
/// running, or a live agent is on a slice with an open pull request (a push,
/// and the checks it starts, can land any moment); **slow** — the plan
/// poll's — otherwise. A project whose plan has no slice worth asking about
/// (`shouldRead`) is not read by the loop, as the plan poll skips it. Without
/// a cadence nothing polls: readings are taken only when asked for.
@MainActor
@Observable
public final class PRStatusStore {
    /// The last reading of each project, by project ID — absent for one never
    /// read.
    public private(set) var readings: [String: PRReading] = [:]

    /// The two intervals a project's own loop sleeps between readings.
    public struct Cadence: Equatable, Sendable {
        /// While checks run or an agent is live on an open pull request.
        public let fast: Duration
        /// Otherwise — the plan poll's own cadence.
        public let slow: Duration

        public init(fast: Duration, slow: Duration) {
            self.fast = fast
            self.slow = slow
        }
    }

    private let client: NatClientProtocol
    private let cache: PlanCaching
    private let cadence: Cadence?
    /// The slice ids with a live agent on them, read as each reading lands.
    private let liveSliceIDs: @MainActor () -> Set<String>
    /// Whether a project's plan has anything for `pr-status` to read — what
    /// the loop asks before each reading of its own.
    private let shouldRead: @MainActor (String) -> Bool
    /// The reading under way for each project — a second ask joins it.
    private var inFlight: [String: Task<Void, Never>] = [:]
    /// Each project's sleeping loop.
    private var pollTasks: [String: Task<Void, Never>] = [:]
    /// The interval each project's loop was last scheduled at.
    public private(set) var intervals: [String: Duration] = [:]
    /// Bumped by `forget`, so a reading that lands for a forgotten project
    /// neither stores itself nor starts a loop.
    private var generations: [String: Int] = [:]

    public init(
        client: NatClientProtocol = NatClient(), cache: PlanCaching = DiskPlanCache(), cadence: Cadence? = nil,
        liveSliceIDs: @escaping @MainActor () -> Set<String> = { [] },
        shouldRead: @escaping @MainActor (String) -> Bool = { _ in true }
    ) {
        self.client = client
        self.cache = cache
        self.cadence = cadence
        self.liveSliceIDs = liveSliceIDs
        self.shouldRead = shouldRead
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
    /// it, recording it in the cache; a reading that fails changes nothing,
    /// and one equal to the last publishes nothing. A reading already under
    /// way for the project is joined, never doubled. Once it lands (or
    /// fails), the project's loop is scheduled at the cadence it calls for.
    public func update(projectID: String) async {
        if let running = inFlight[projectID] {
            await running.value
            return
        }
        let generation = generations[projectID, default: 0]
        let task = Task { await read(projectID: projectID, generation: generation) }
        inFlight[projectID] = task
        await task.value
    }

    private func read(projectID: String, generation: Int) async {
        defer {
            if generations[projectID, default: 0] == generation {
                inFlight[projectID] = nil
                schedule(projectID: projectID)
            }
        }
        guard let doc = try? await client.prStatus(projectID: projectID),
            generations[projectID, default: 0] == generation
        else { return }
        let reading = PRReading(doc)
        guard readings[projectID] != reading else { return }
        readings[projectID] = reading
        await cache.writePRStatus(doc, projectID: projectID)
    }

    /// The interval a project's reading calls for: fast while any open pull
    /// request's checks are running or a live agent is on a slice with an
    /// open pull request, else slow.
    public func interval(for reading: PRReading, cadence: Cadence) -> Duration {
        let open = reading.readiness
        let running = reading.doc.slices.contains {
            open[$0.sliceID] != nil && $0.checks?.verdict == PRStatusSlice.checksPending
        }
        if running || !liveSliceIDs().isDisjoint(with: open.keys) { return cadence.fast }
        return cadence.slow
    }

    /// (Re)starts a project's loop at the interval its reading calls for —
    /// one sleeping task per project, the last one replaced.
    private func schedule(projectID: String) {
        guard let cadence else { return }
        pollTasks[projectID]?.cancel()
        let wait = interval(for: reading(projectID: projectID), cadence: cadence)
        intervals[projectID] = wait
        pollTasks[projectID] = Task { [weak self] in
            try? await Task.sleep(for: wait)
            guard !Task.isCancelled, let self else { return }
            guard self.shouldRead(projectID) else {
                // Nothing to read now; look again an interval on.
                self.schedule(projectID: projectID)
                return
            }
            await self.update(projectID: projectID)
        }
    }

    /// Puts a project's cached reading up, where nothing fresher has landed
    /// for it — at launch, before the first fresh reading.
    public func restore(projectID: String) async {
        guard readings[projectID] == nil, let doc = await cache.readPRStatus(projectID: projectID) else { return }
        // A fresh reading that landed while the cache was read wins.
        guard readings[projectID] == nil else { return }
        readings[projectID] = PRReading(doc)
    }

    /// Forgets a project whose tab has gone, stopping its loop; a reading of
    /// it still under way lands nowhere.
    public func forget(projectID: String) {
        readings.removeValue(forKey: projectID)
        pollTasks.removeValue(forKey: projectID)?.cancel()
        intervals.removeValue(forKey: projectID)
        inFlight.removeValue(forKey: projectID)
        generations[projectID, default: 0] += 1
    }

    /// Stops every project's loop — the app model going away.
    public func stop() {
        for task in pollTasks.values { task.cancel() }
        pollTasks = [:]
        intervals = [:]
    }
}
