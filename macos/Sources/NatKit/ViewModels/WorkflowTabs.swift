import Foundation

/// A tab in the workflow strip (Brief, Agent, Diff, PR).
public enum WorkflowTab: String, CaseIterable, Equatable {
    case brief = "Brief"
    case agent = "Agent"
    case diff = "Diff"
    case pr = "PR"

    public var symbolName: String {
        switch self {
        case .brief:
            return "doc.text"
        case .agent:
            return "chevron.left.forwardslash.chevron.right"
        case .diff:
            return "plus.forwardslash.minus"
        case .pr:
            return "arrow.branch"
        }
    }
}

/// Determines which tabs are reachable for a given slice and its state.
public struct WorkflowTabState: Equatable {
    /// All tabs, in order.
    public let tabs: [WorkflowTab]

    /// Tabs that can be navigated to.
    public let reachable: Set<WorkflowTab>

    /// The default tab to show (the furthest reachable, with precedent: Agent > Diff > PR > Brief).
    public let defaultTab: WorkflowTab

    public init(tabs: [WorkflowTab], reachable: Set<WorkflowTab>, defaultTab: WorkflowTab) {
        self.tabs = tabs
        self.reachable = reachable
        self.defaultTab = defaultTab
    }

    /// Whether a tab is reachable.
    public func isReachable(_ tab: WorkflowTab) -> Bool {
        reachable.contains(tab)
    }

    /// Whether a tab's stage is behind the slice's own progress — what the
    /// strip draws a checkmark for. A stage is behind when any stage past it
    /// has unlocked, which is a fact about the slice alone. Not measured off
    /// `defaultTab`: that answers where to look now, and a live agent pulls
    /// it back to Agent on a slice whose Diff is already unlocked — which
    /// must not untick the stages the slice has been through.
    public func isComplete(_ tab: WorkflowTab) -> Bool {
        guard let tabIndex = tabs.firstIndex(of: tab) else { return false }
        return tabs[(tabIndex + 1)...].contains { reachable.contains($0) }
    }
}

/// Determines the workflow tab state for a slice.
///
/// Rules:
/// - Brief is always reachable.
/// - Agent is reachable if the slice has a live agent OR status is in progress.
/// - Diff is reachable if the slice has a branch recorded — handed back, or
///   Done with its pull request still in review: approving opened the pull
///   request, it did not end the review, and the branch reads until it lands.
/// - PR is reachable if the slice has a non-empty PR URL.
///
/// Default tab precedence: a Done slice with a pull request lands on PR — its
/// state IS the pull request, and a lingering agent session must not steal
/// the landing, since a session can outlive the slice it was launched on.
/// After that, furthest reachable: live agent → Agent; handed back → Diff;
/// in progress → Agent; PR recorded → PR; else Brief.
public func buildWorkflowTabState(
    for slice: Slice,
    hasLiveAgent: Bool
) -> WorkflowTabState {
    let allTabs = WorkflowTab.allCases

    // Brief is always reachable
    var reachable: Set<WorkflowTab> = [.brief]

    // Agent is reachable if live agent or in progress
    if hasLiveAgent || slice.status == "In progress" {
        reachable.insert(.agent)
    }

    // Diff is reachable wherever a branch is recorded to read
    if slice.handedBack || !(slice.branch ?? "").isEmpty {
        reachable.insert(.diff)
    }

    // PR is reachable if PR URL is non-empty
    if !slice.pr.isEmpty {
        reachable.insert(.pr)
    }

    let defaultTab: WorkflowTab
    if slice.status == "Done", !slice.pr.isEmpty {
        defaultTab = .pr
    } else if hasLiveAgent {
        defaultTab = .agent
    } else if slice.handedBack {
        // Handed back means there's a branch awaiting review
        defaultTab = .diff
    } else if reachable.contains(.agent) {
        // In progress but no live agent yet
        defaultTab = .agent
    } else if !slice.pr.isEmpty {
        defaultTab = .pr
    } else {
        defaultTab = .brief
    }

    return WorkflowTabState(tabs: allTabs, reachable: reachable, defaultTab: defaultTab)
}
