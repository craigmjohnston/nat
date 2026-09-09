import Foundation
import SwiftUI

/// Manages live agent presence by polling the tmux server periodically.
///
/// This store maintains a map of running agents keyed by slice ID, polling every 2 seconds.
/// The poll loop stops itself when no agents are found and is re-armed by calling `kick()`.
/// Failed readings keep the previous state (following the TUI convention).
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

    private let client: NatClientProtocol
    private let now: () -> Date
    private var pollTask: Task<Void, Never>?
    private var isPolling = false

    public init(client: NatClientProtocol = NatClient(), now: @escaping () -> Date = { Date() }) {
        self.client = client
        self.now = now
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

                    // Update the map keyed by slice ID
                    var newAgents: [String: AgentStatus] = [:]
                    for status in statuses {
                        newAgents[status.sliceID] = status
                    }
                    self.agents = newAgents
                    self.firstSeen = Self.mergeFirstSeen(
                        existing: self.firstSeen, sliceIDs: newAgents.keys, now: self.now()
                    )

                    // If no agents, stop polling
                    if statuses.isEmpty {
                        self.isPolling = false
                        break
                    }

                    // Sleep for 2 seconds before next poll
                    try await Task.sleep(nanoseconds: 2 * 1_000_000_000)
                } catch is CancellationError {
                    break
                } catch {
                    // Failed reading: keep previous state and log error
                    NSLog("ActivityStore: failed to read agent status: %@", error.localizedDescription)

                    // Sleep briefly before retrying
                    try? await Task.sleep(nanoseconds: 2 * 1_000_000_000)
                }
            }
        }
    }
}
