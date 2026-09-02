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
}

/// Determines the workflow tab state for a slice.
///
/// Rules:
/// - Brief is always reachable.
/// - Agent is reachable if the slice has a live agent OR status is in progress.
/// - Diff is reachable if the slice is handed_back (has a branch).
/// - PR is reachable if the slice has a non-empty PR URL.
///
/// Default tab precedence (furthest reachable): live agent → Agent; handed back → Diff; PR recorded → PR; else Brief.
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

    // Diff is reachable if handed back (has branch)
    if slice.handedBack {
        reachable.insert(.diff)
    }

    // PR is reachable if PR URL is non-empty
    if !slice.pr.isEmpty {
        reachable.insert(.pr)
    }

    // Determine default tab: precedent is live agent > handed back (Diff) > Agent (reachable) > PR > Brief
    let defaultTab: WorkflowTab
    if hasLiveAgent {
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
