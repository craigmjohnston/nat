import Foundation

/// The app's one read of GitHub: `nat pr-status` naming every open project at
/// once — one GraphQL document, a point of GitHub's shared hourly budget,
/// whatever the number of projects or pull requests — and, with `--detail`,
/// the pull request on a visible PR tab in full. What it reads is handed on
/// (`deliver`): each project's part to `PRStatusStore` (the marks, the Active
/// rail, the dock badge), the detail to the PR tab, and the session rows a
/// fresh `session-list`, which reads the pull requests this reading kept.
///
/// It reads on a tick (`tick`, the poll's `poll_seconds`) and on a **settle
/// read** after an action changed something on GitHub (`scheduleSettle`):
/// approve, merge, comment, reviewers, re-run or cancel checks, the manual
/// refresh. The settle read waits `settleDelay` (5 seconds) first — `gh pr
/// create` returns once the pull request exists, but GitHub reads its
/// mergeability as UNKNOWN for a few seconds after and starts the checks later
/// still — and every action inside that window folds into the same pending
/// read; the tick restarts from it. A nudge never reaches here: a plan read is
/// not news about GitHub.
///
/// Never two reads in flight: a tick that finds one running leaves it to
/// finish and reads nothing itself; a settle read waits for it, then reads.
/// A read that fails changes nothing — the last reading stands everywhere.
@MainActor
@Observable
public final class GitHubReadingStore {
    /// GitHub's budget as the last reading left it — kept for the throttle
    /// and the status bar; nothing draws it yet.
    public private(set) var rateLimit: GitHubRateLimit?

    /// What one reading asks: every project to name, and the pull request to
    /// read in full, if any.
    public struct Request: Equatable, Sendable {
        public let projectIDs: [String]
        public let detail: String?

        public init(projectIDs: [String], detail: String? = nil) {
            self.projectIDs = projectIDs
            self.detail = detail
        }
    }

    private let client: NatClientProtocol
    private let request: @MainActor () -> Request?
    private let deliver: @MainActor (GitHubReading) async -> Void
    private let tick: Duration?
    private let settleDelay: Duration
    private let sleep: @Sendable (Duration) async -> Void

    private var inFlight: Task<Void, Never>?
    private var tickTask: Task<Void, Never>?
    private var settleTask: Task<Void, Never>?

    /// - Parameters:
    ///   - request: what a reading asks, read as it starts — nil, or no
    ///     project, reads nothing
    ///   - deliver: hands a reading that landed to the stores it feeds
    ///   - tick: the interval between readings; nil, as in tests, for no
    ///     tick at all
    ///   - settleDelay: how long a settle read waits after the action
    ///   - sleep: how a wait is waited — `Task.sleep` in the app, a gate in
    ///     tests
    public init(
        client: NatClientProtocol,
        request: @escaping @MainActor () -> Request?,
        deliver: @escaping @MainActor (GitHubReading) async -> Void,
        tick: Duration? = nil,
        settleDelay: Duration = .seconds(5),
        sleep: @escaping @Sendable (Duration) async -> Void = { try? await Task.sleep(for: $0) }
    ) {
        self.client = client
        self.request = request
        self.deliver = deliver
        self.tick = tick
        self.settleDelay = settleDelay
        self.sleep = sleep
    }

    /// Whether a settle read is waiting out its delay.
    public var isSettlePending: Bool { settleTask != nil }

    /// Starts the tick.
    public func start() {
        restartTick()
    }

    /// Whether a reading is running.
    public var isReading: Bool { inFlight != nil }

    /// Takes a reading now, unless one is already running — that one is left
    /// to finish, and is the reading.
    public func read() async {
        guard inFlight == nil else { return }
        await begin()?.value
    }

    /// Starts a reading without waiting on it — the launch's first, off the
    /// startup path — unless one is already running.
    public func readSoon() {
        guard inFlight == nil else { return }
        begin()
    }

    /// Returns once no reading is running and no settle read is pending —
    /// for a fixture or a test that wants the board as the readings left it.
    public func idle() async {
        while true {
            if let running = inFlight {
                await running.value
            } else if let pending = settleTask {
                await pending.value
            } else {
                return
            }
        }
    }

    /// Schedules the settle read `settleDelay` out, folding into one already
    /// pending. The tick stops until it has read, and restarts from it.
    public func scheduleSettle() {
        guard settleTask == nil else { return }
        tickTask?.cancel()
        tickTask = nil
        let delay = settleDelay
        let sleep = self.sleep
        settleTask = Task { [weak self] in
            await sleep(delay)
            guard !Task.isCancelled, let self else { return }
            while let running = self.inFlight { await running.value }
            self.settleTask = nil
            await self.begin()?.value
            self.restartTick()
        }
    }

    /// Stops the tick and any pending settle read — the app model going away.
    public func stop() {
        tickTask?.cancel()
        tickTask = nil
        settleTask?.cancel()
        settleTask = nil
    }

    /// Starts one reading of what `request` asks, marked in flight until it
    /// has been handed on; nil where there is nothing to ask.
    @discardableResult
    private func begin() -> Task<Void, Never>? {
        guard let request = request(), !request.projectIDs.isEmpty else { return nil }
        let client = self.client
        let task = Task { @MainActor [weak self] in
            defer { self?.inFlight = nil }
            guard let reading = try? await client.prStatus(projectIDs: request.projectIDs, detail: request.detail),
                  let self else { return }
            if let rateLimit = reading.rateLimit { self.rateLimit = rateLimit }
            await self.deliver(reading)
        }
        inFlight = task
        return task
    }

    private func restartTick() {
        tickTask?.cancel()
        tickTask = nil
        // A settle read scheduled while this one ran restarts the tick itself.
        guard let tick, settleTask == nil else { return }
        let sleep = self.sleep
        tickTask = Task { [weak self] in
            while !Task.isCancelled {
                await sleep(tick)
                guard !Task.isCancelled, let self else { return }
                await self.read()
            }
        }
    }
}
