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

    private let client: NatClientProtocol
    private var pollTask: Task<Void, Never>?
    private var isPolling = false

    public init(client: NatClientProtocol = NatClient()) {
        self.client = client
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
