import Foundation

/// The app's one read of GitHub: `nat pr-status` naming every open project at
/// once — one GraphQL document, a point of GitHub's shared hourly budget,
/// whatever the number of projects or pull requests — and, with `--detail`,
/// the pull request on a visible PR tab in full. What it reads is handed on
/// (`deliver`): each project's part to `PRStatusStore` (the marks, the Active
/// rail, the dock badge), the detail to the PR tab, and the session rows a
/// fresh `session-list`, which reads the pull requests this reading kept.
///
/// It reads on a tick — sleeping for the `poll_after_seconds` the last
/// reading's budget asked for (nat's throttle and pause), else `tick`, the
/// poll's `poll_seconds` — and on a **settle read** after an action changed
/// something on GitHub (`scheduleSettle`), which runs `pr-status --settle`
/// and so goes through whatever the throttle or the pause say:
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
///
/// It also keeps gnat's own spend this session — the points its readings
/// cost (`cost`) and one per action that spent one (`actionRan`) — and when
/// the app launched: what Settings ▸ About's Diagnostics puts beside the
/// shared budget, which cannot say whose spend it is.
@MainActor
@Observable
public final class GitHubReadingStore {
    /// GitHub's budget as the last reading left it — the status bar's
    /// readout and the Diagnostics foldout.
    public private(set) var rateLimit: GitHubRateLimit?

    /// The points gnat has spent since launch: every reading's `cost`, and
    /// one per action (`actionRan`).
    public private(set) var sessionPoints = 0
    /// The readings gnat has taken since launch, settle reads included.
    public private(set) var sessionReadings = 0
    /// The actions gnat has run since launch that spend a point: approve,
    /// merge, comment, reviewers, re-run and cancel checks.
    public private(set) var sessionActions = 0
    /// When the app launched — the store is made once, at launch.
    public let launchedAt: Date

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
    /// Whether the pending settle read follows an action — and so runs past
    /// the throttle and the pause — rather than a plan loading.
    private var settleIsAction = false
    /// The interval the last reading's budget asked for, nil where it said
    /// none (nothing asked, or an older nat): the tick's own then.
    private var pollAfter: Duration?

    /// - Parameters:
    ///   - request: what a reading asks, read as it starts — nil, or no
    ///     project, reads nothing
    ///   - deliver: hands a reading that landed to the stores it feeds
    ///   - tick: the interval between readings; nil, as in tests, for no
    ///     tick at all
    ///   - settleDelay: how long a settle read waits after the action
    ///   - sleep: how a wait is waited — `Task.sleep` in the app, a gate in
    ///     tests
    ///   - now: the clock `launchedAt` is read off
    public init(
        client: NatClientProtocol,
        request: @escaping @MainActor () -> Request?,
        deliver: @escaping @MainActor (GitHubReading) async -> Void,
        tick: Duration? = nil,
        settleDelay: Duration = .seconds(5),
        sleep: @escaping @Sendable (Duration) async -> Void = { try? await Task.sleep(for: $0) },
        now: @Sendable () -> Date = { Date() }
    ) {
        self.launchedAt = now()
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

    /// Counts an action that spent a point of GitHub's budget, then asks for
    /// the settle read after it.
    public func actionRan() {
        sessionActions += 1
        sessionPoints += 1
        scheduleSettle()
    }

    /// Schedules the settle read `settleDelay` out, folding into one already
    /// pending. The tick stops until it has read, and restarts from it. One
    /// after an action (`afterAction`, the default) runs past nat's throttle
    /// and pause; one for a plan that loaded does not.
    public func scheduleSettle(afterAction: Bool = true) {
        settleIsAction = settleIsAction || afterAction
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
            let afterAction = self.settleIsAction
            self.settleIsAction = false
            await self.begin(settle: afterAction)?.value
            self.restartTick()
        }
    }

    /// Stops the tick and any pending settle read — the app model going away.
    public func stop() {
        tickTask?.cancel()
        tickTask = nil
        settleTask?.cancel()
        settleTask = nil
        settleIsAction = false
    }

    /// Starts one reading of what `request` asks, marked in flight until it
    /// has been handed on; nil where there is nothing to ask.
    @discardableResult
    private func begin(settle: Bool = false) -> Task<Void, Never>? {
        guard let request = request(), !request.projectIDs.isEmpty else { return nil }
        let client = self.client
        let task = Task { @MainActor [weak self] in
            defer { self?.inFlight = nil }
            guard let reading = try? await client.prStatus(
                projectIDs: request.projectIDs, detail: request.detail, settle: settle),
                  let self else { return }
            if let rateLimit = reading.rateLimit { self.rateLimit = rateLimit }
            self.pollAfter = reading.rateLimit?.pollAfterSeconds.map { Duration.seconds($0) }
            self.sessionReadings += 1
            self.sessionPoints += reading.rateLimit?.cost ?? 0
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
                await sleep(self?.pollAfter ?? tick)
                guard !Task.isCancelled, let self else { return }
                await self.read()
            }
        }
    }
}
