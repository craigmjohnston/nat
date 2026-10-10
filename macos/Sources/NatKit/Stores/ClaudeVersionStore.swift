import Foundation

/// Where a Claude Code update the user asked for stands — what the update
/// sheet draws.
public enum ClaudeUpdateState: Equatable, Sendable {
    /// The window is up and nothing has run: the user has yet to press
    /// Update, and may close it instead.
    case confirming
    /// `nat claude-update` is running.
    case running
    /// It finished. What the updater printed is not kept: the window shows
    /// the installed version as read again.
    case finished
    /// It failed; `message` is nat's refusal, carrying the updater's words —
    /// the window's folded details.
    case failed(message: String)
}

/// gnat's Claude Code update notice: the app says once, in the status bar,
/// that a newer Claude Code exists, and on a click opens a window saying what
/// an update would do, which runs it only on Update. Reads `nat
/// claude-version` at launch and every `refreshIntervalSeconds` after (nat
/// keeps the release feed's answer an hour itself), and runs `nat
/// claude-update` when Update is pressed.
@MainActor
@Observable
public final class ClaudeVersionStore {
    /// The last reading; a read that fails leaves the one before standing.
    public private(set) var version: ClaudeVersion?
    /// The update the user asked for, nil while none has been — the sheet is
    /// up exactly while this is set.
    public private(set) var update: ClaudeUpdateState?

    private let client: NatClientProtocol
    private let refreshIntervalSeconds: UInt64
    private var refreshTask: Task<Void, Never>?

    public init(client: NatClientProtocol = NatClient(), refreshIntervalSeconds: UInt64 = 60 * 60) {
        self.client = client
        self.refreshIntervalSeconds = refreshIntervalSeconds
    }

    /// The status bar's notice, nil where there is nothing newer.
    public var notice: String? { version?.notice }

    /// Reads once, then arms the hourly timer. Called once, at app start.
    public func start() async {
        await refresh()
        startTimer()
    }

    public func refresh() async {
        guard let fresh = try? await client.claudeVersion() else { return }
        version = fresh
    }

    /// Opens the update window, running nothing — the notice's click. A
    /// window already up is left as it stands.
    public func confirmUpdate() {
        guard update == nil else { return }
        update = .confirming
    }

    /// Runs the update through nat — the window's Update — the window showing
    /// it under way and then its outcome; a success reads the version again,
    /// so the window shows the new one and the notice goes. One at a time:
    /// asked again while one runs, nothing happens.
    public func runUpdate() async {
        guard update != .running else { return }
        update = .running
        do {
            _ = try await client.claudeUpdate()
            await refresh()
            update = .finished
        } catch {
            update = .failed(message: error.localizedDescription)
        }
    }

    /// Takes the sheet down — not while the update is still running.
    public func dismissUpdate() {
        guard update != .running else { return }
        update = nil
    }

    public func stop() {
        refreshTask?.cancel()
        refreshTask = nil
    }

    private func startTimer() {
        refreshTask?.cancel()
        refreshTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: refreshIntervalSeconds * 1_000_000_000)
                if Task.isCancelled { break }
                await refresh()
            }
        }
    }
}
