import Foundation
import SwiftUI

/// Manages live agent presence by polling the tmux server periodically.
///
/// This store maintains a map of running agents keyed by slice ID, polling every 2 seconds. Each
/// agent's status carries its model, effort and context percent, so the
/// status bar's live readout rides this same poll.
/// The poll loop stops itself when no agents are found and is re-armed by calling `kick()`.
/// Failed readings keep the previous state (following the TUI convention).
///
/// Over the readings sits one overlay, display state only: an action whose
/// effect on the pip is known — a send to an agent waiting on the user — is
/// drawn at once (`expectWorking`), and readings put it right later only where
/// reality disagrees. Every reader of an agent's activity goes through
/// `activity(for:)`/`status(for:)`/`displayedAgents`, never `agents`' own
/// `activity`, so the sidebar, titlebar, dock badge and attention agree.
@MainActor
@Observable
public final class ActivityStore {
    /// Map of slice ID to agent status for all running agents.
    public private(set) var agents: [String: AgentStatus] = [:]

    /// When each running agent was first seen by the poll, keyed like
    /// `agents` — the rail's elapsed time is measured from this. An agent
    /// keeps its stamp across polls and loses it when it goes, so a relaunch
    /// starts the clock again.
    public private(set) var firstSeen: [String: Date] = [:]

    /// Whether a reading has landed this run — any that completed, an empty
    /// one included; a failed one is no reading. Until then `agents` being
    /// empty says nothing about what is running.
    public private(set) var hasRead = false

    /// Called right after every reading that lands: what `AppModel` reads
    /// planning agents coming and going off — the workshops it draws
    /// provisionally from the last run until the first reading, and those
    /// whose agent has ended since the last.
    @ObservationIgnored public var onReading: (() -> Void)?

    /// When each agent was last expected to be working again, keyed like
    /// `agents`: set by an action that sends to it, before its `nat` call
    /// (`expectWorking`), and settled by every reading that lands — dropped
    /// once a reading agrees or the agent is gone, kept while one still reads
    /// waiting until `expectationTTL` has passed. Never written to nat.
    public private(set) var expectations: [String: Date] = [:]

    /// How long an expectation outlives readings that still say waiting: long
    /// enough for nat's marker to move and a poll to read it, short enough
    /// that an Enter which answered nothing goes back to waiting soon.
    public nonisolated static let defaultExpectationTTL: TimeInterval = 15

    private let client: NatClientProtocol
    private let now: () -> Date
    private let expectationTTL: TimeInterval
    private var pollTask: Task<Void, Never>?
    private var isPolling = false

    public init(
        client: NatClientProtocol = NatClient(), now: @escaping () -> Date = { Date() },
        expectationTTL: TimeInterval = ActivityStore.defaultExpectationTTL
    ) {
        self.client = client
        self.now = now
        self.expectationTTL = expectationTTL
    }

    // MARK: - Expectations

    /// Draw `key`'s agent as working from now, whatever its reading says,
    /// until a reading settles it.
    public func expectWorking(_ key: String) {
        expectations[key] = now()
    }

    /// Drop `key`'s expectation at once — the action that set it failed.
    public func withdraw(_ key: String) {
        expectations[key] = nil
    }

    /// `key`'s agent's activity as drawn: its reading, except that one read as
    /// waiting with an unexpired expectation reads working. Nil with no
    /// reading, whatever the expectation.
    public func activity(for key: String) -> AgentActivityState? {
        status(for: key)?.activity
    }

    /// `key`'s reading with `activity(for:)`'s overlay applied — for readers
    /// that take the whole status (model, effort, context) along with it.
    public func status(for key: String) -> AgentStatus? {
        guard let status = agents[key] else { return nil }
        guard status.activity == .waiting, let expected = expectations[key],
              now().timeIntervalSince(expected) < expectationTTL else { return status }
        return AgentStatus(
            sliceID: status.sliceID, session: status.session, activity: .working,
            model: status.model, effort: status.effort,
            contextPercent: status.contextPercent, contextTokens: status.contextTokens)
    }

    /// Every running agent through `status(for:)`, keyed like `agents`.
    public var displayedAgents: [String: AgentStatus] {
        Dictionary(uniqueKeysWithValues: agents.keys.compactMap { key in status(for: key).map { (key, $0) } })
    }

    /// The expectations one reading leaves standing: one whose agent now
    /// reads working is dropped (reality agrees), one whose agent is gone is
    /// dropped, and one still read as waiting is kept until older than `ttl`.
    /// Pure, so the rule is testable without the loop.
    nonisolated static func settle(
        expectations: [String: Date], agents: [String: AgentStatus], now: Date, ttl: TimeInterval
    ) -> [String: Date] {
        expectations.filter { key, expected in
            agents[key]?.activity == .waiting && now.timeIntervalSince(expected) < ttl
        }
    }

    /// `firstSeen` brought in line with one poll's reading: an agent already
    /// stamped keeps its stamp, a new one is stamped `now`, and one no longer
    /// running is dropped. Pure, so the rule is testable without the loop.
    nonisolated static func mergeFirstSeen(
        existing: [String: Date],
        sliceIDs: some Sequence<String>,
        now: Date
    ) -> [String: Date] {
        sliceIDs.reduce(into: [:]) { merged, sliceID in
            merged[sliceID] = existing[sliceID] ?? now
        }
    }

    /// Re-arm the poll loop if it has stopped.
    public func kick() {
        guard !isPolling else { return }
        startPolling()
    }

    /// Read now rather than at the next tick. A nudge is as often an agent
    /// marking its own pane (`nat agent-waiting`/`agent-working`) as a plan
    /// write, and the needs-attention state should follow at once: a live loop
    /// is restarted, its reading taken now, and a stopped one is started.
    public func reread() {
        stop()
        startPolling()
    }

    /// Stop the poll loop and clean up.
    public func stop() {
        pollTask?.cancel()
        pollTask = nil
        isPolling = false
    }

    // MARK: - Private

    private func startPolling() {
        isPolling = true
        pollTask = Task {
            while !Task.isCancelled {
                do {
                    let statuses = try await client.status()
                    // A loop cancelled mid-read (reread() replaced it) leaves
                    // the store to the one that replaced it.
                    if Task.isCancelled { break }

                    // Update the map keyed by slice ID
                    var newAgents: [String: AgentStatus] = [:]
                    for status in statuses {
                        newAgents[status.sliceID] = status
                    }
                    self.agents = newAgents
                    self.firstSeen = Self.mergeFirstSeen(
                        existing: self.firstSeen, sliceIDs: newAgents.keys, now: self.now()
                    )
                    let settled = Self.settle(
                        expectations: self.expectations, agents: newAgents, now: self.now(), ttl: self.expectationTTL)
                    if settled != self.expectations { self.expectations = settled }
                    self.hasRead = true
                    self.onReading?()

                    // If no agents, stop polling. Said out loud, because an
                    // empty reading ends the loop until the next kick(): a
                    // reading that is wrongly empty — a tmux whose output the
                    // client mangled once hid every live agent this way — is
                    // otherwise indistinguishable from a quiet board.
                    if statuses.isEmpty {
                        NSLog("ActivityStore: no agents reported; polling stops until the next kick")
                        self.isPolling = false
                        break
                    }

                    // Sleep for 2 seconds before next poll
                    try await Task.sleep(nanoseconds: 2 * 1_000_000_000)
                } catch is CancellationError {
                    break
                } catch {
                    if Task.isCancelled { break }
                    // Failed reading: keep previous state and log error
                    NSLog("ActivityStore: failed to read agent status: %@", error.localizedDescription)

                    // With no agent known of, there is nothing the retry is
                    // for — the loop stops the way an empty reading stops it,
                    // and kick() re-arms it. A client that only ever fails
                    // would otherwise poll (and log) every two seconds
                    // forever.
                    if self.agents.isEmpty {
                        self.isPolling = false
                        break
                    }

                    // Sleep briefly before retrying
                    try? await Task.sleep(nanoseconds: 2 * 1_000_000_000)
                }
            }
        }
    }
}
