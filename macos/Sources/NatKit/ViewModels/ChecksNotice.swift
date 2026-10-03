import Foundation

/// What the pane says about a pull request whose checks are failing: which
/// checks, and what is to be done — launch a fix agent, or nothing, the agent
/// already told.
public struct ChecksNotice: Equatable, Sendable {
    public enum Action: Equatable, Sendable {
        /// No agent is live, and one can be launched: the notice offers the
        /// fix launch the Thread's own Launch makes.
        case launchFix
        /// The live agent was sent the failure (the `Sent back` nat files
        /// with its nudge is the latest thing on the record): nothing to press.
        case agentTold
        /// Neither: an agent is live but the record does not say it was told,
        /// or no launch is open.
        case none
    }

    public let checks: [String]
    public let action: Action

    public init(checks: [String], action: Action) {
        self.checks = checks
        self.action = action
    }

    /// The notice's words: the failing checks by name, and — once the agent
    /// has been told — that it has.
    public var text: String {
        let named = checks.isEmpty ? "Checks are failing" : "Failing: \(checks.joined(separator: ", "))"
        switch action {
        case .agentTold: return "\(named) — the agent has been told."
        case .launchFix, .none: return "\(named)."
        }
    }
}

/// The checks notice for a slice, or nil where there is none to draw: only a
/// slice at the PR stage or under a fix whose pull request was last read
/// failing its checks (`ReviewStatsStore.failingChecks`) — a pending, green or
/// never-taken reading draws nothing. `events` is `slice-show`'s task log,
/// whose recorded events come before the `approved`/`merged` it adds from the
/// slice's properties; the latest recorded one says whether a live agent was
/// the one the nudge reached.
public func checksNotice(
    slice: Slice, failing: [String]?, hasLiveAgent: Bool, events: [TaskLogEvent]?
) -> ChecksNotice? {
    guard let failing else { return nil }
    switch stage(for: slice, agent: nil) {
    case .pr, .fixing: break
    case .todo, .working, .review, .done: return nil
    }
    guard hasLiveAgent else {
        let action: ChecksNotice.Action = LaunchPlan(for: slice, hasLiveAgent: false).canLaunch ? .launchFix : .none
        return ChecksNotice(checks: failing, action: action)
    }
    let closing: Set<TaskLogEvent.Kind> = [.approved, .merged]
    let latest = events?.last { !closing.contains($0.kind) }
    return ChecksNotice(checks: failing, action: latest?.kind == .sentBack ? .agentTold : .none)
}
