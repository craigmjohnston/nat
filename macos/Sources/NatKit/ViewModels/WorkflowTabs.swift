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

    /// The tab to land on — the slice's `WorkflowStage`'s tab.
    public let defaultTab: WorkflowTab

    /// A count drawn beside a stage's label, for the stages that have one.
    public let badges: [WorkflowTab: Int]

    /// Stages whose completion is a fact of their own rather than of the
    /// stages after them — `isComplete`'s default reading, which they replace.
    public let completion: [WorkflowTab: Bool]

    public init(
        tabs: [WorkflowTab], reachable: Set<WorkflowTab>, defaultTab: WorkflowTab,
        badges: [WorkflowTab: Int] = [:], completion: [WorkflowTab: Bool] = [:]
    ) {
        self.tabs = tabs
        self.reachable = reachable
        self.defaultTab = defaultTab
        self.badges = badges
        self.completion = completion
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
        if let own = completion[tab] { return own }
        guard let tabIndex = tabs.firstIndex(of: tab) else { return false }
        return tabs[(tabIndex + 1)...].contains { reachable.contains($0) }
    }

    /// Whether the separator drawn before `tab` — the one joining it to the
    /// stage on its left — is lit. A separator is the step from one stage to
    /// the next, so it lights only where both of its ends are reachable:
    /// either end still locked and the step is not one that can be taken.
    /// The first stage has nothing on its left and so no separator at all,
    /// which reads as unlit, as does a tab this state says nothing about.
    public func isSeparatorLit(before tab: WorkflowTab) -> Bool {
        guard let tabIndex = tabs.firstIndex(of: tab), tabIndex > 0 else { return false }
        return isReachable(tabs[tabIndex - 1]) && isReachable(tab)
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
/// The default tab is the slice's `WorkflowStage`'s own (`stage(for:)`), not a
/// precedence over raw facts: a live session outlives hand-back and approve,
/// so "a session exists" says nothing about where the slice stands. Only the
/// landing tab comes from the stage; reachability keeps its fact-based rules.
public func buildWorkflowTabState(
    for slice: Slice,
    hasLiveAgent: Bool,
    fixLaunched: Bool = false
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

    let workflowStage = stage(for: slice, agent: nil, fixLaunched: fixLaunched)
    let defaultTab = workflowStage.tab(for: slice)

    return WorkflowTabState(tabs: allTabs, reachable: reachable, defaultTab: defaultTab)
}

/// The workflow tab state for an ad hoc session's pane: Agent, Diff and PR,
/// no Brief and no Brief stage in the stepper — a session has no brief to
/// write one for. All three are always reachable: a session always has a
/// worktree and a branch to diff and a first pull request (or none yet) to
/// name, unlike a slice's own stages, which unlock one at a time as the work
/// progresses. Agent is the default: it is where a session is watched while
/// its agent works, the same reason a slice with a live agent defaults there.
///
/// The PR stage reads the session's own pull requests rather than the stages
/// after it: its badge is how many are still open, and it is complete only
/// once every one has merged. The Diff stage keeps the default reading.
public func buildSessionTabState(prs: [SessionPR] = []) -> WorkflowTabState {
    let tabs: [WorkflowTab] = [.agent, .diff, .pr]
    let stage = SessionPRStage(prs: prs)
    return WorkflowTabState(
        tabs: tabs, reachable: Set(tabs), defaultTab: .agent,
        badges: stage.openCount > 0 ? [.pr: stage.openCount] : [:],
        completion: [.pr: stage.isComplete]
    )
}
