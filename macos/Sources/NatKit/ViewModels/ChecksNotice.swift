import Foundation

/// What the pane says about a pull request whose checks are failing: which
/// checks, and what is to be done — send it back to the agent, or nothing,
/// the failure already sent to it.
public struct ChecksNotice: Equatable, Sendable {
    public enum Action: Equatable, Sendable {
        /// No agent is live, and one can be launched: the action bar's Send
        /// back to agent, prefilled with the failure, resumes the slice and
        /// launches one.
        case sendBack
        /// The failure was sent to the live agent (the `Sent back` nat files
        /// with its nudge is the latest thing on the record): nothing to press.
        case sentToAgent
        /// Neither: an agent is live but the record does not say the failure
        /// was sent to it, or no launch is open.
        case none
    }

    public let checks: [String]
    public let action: Action

    public init(checks: [String], action: Action) {
        self.checks = checks
        self.action = action
    }

    /// The notice's words: the failing checks by name, and — once they
    /// have gone to the agent — that it has them to fix.
    public var text: String {
        let named = checks.isEmpty ? "Checks failing" : "Checks failing: \(checks.joined(separator: ", "))"
        switch action {
        case .sentToAgent: return "\(named) — sent to the agent to fix."
        case .sendBack, .none: return "\(named)."
        }
    }
}

/// The checks notice for a slice, or nil where there is none to draw: only a
/// slice at the PR stage whose pull request was last read
/// failing its checks (`ReviewStatsStore.failingChecks`) — a pending, green or
/// never-taken reading draws nothing. `events` is `slice-show`'s task log,
/// whose recorded events come before the `approved`/`merged` it adds from the
/// slice's properties; the latest recorded one says whether a live agent was
/// the one the nudge reached.
public func checksNotice(
    slice: Slice, failing: [String]?, hasLiveAgent: Bool, events: [TaskLogEvent]?
) -> ChecksNotice? {
    guard let failing else { return nil }
    guard atPullRequest(slice) else { return nil }
    guard hasLiveAgent else {
        let action: ChecksNotice.Action = LaunchPlan(for: slice, hasLiveAgent: false).canLaunch ? .sendBack : .none
        return ChecksNotice(checks: failing, action: action)
    }
    let closing: Set<TaskLogEvent.Kind> = [.approved, .merged]
    let latest = events?.last { !closing.contains($0.kind) }
    return ChecksNotice(checks: failing, action: latest?.kind == .sentBack ? .sentToAgent : .none)
}
