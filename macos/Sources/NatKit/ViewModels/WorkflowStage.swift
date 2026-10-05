import Foundation

/// Where a slice stands in its workflow — the one source the pane's landing
/// tab and the rail's section both read, so the two can never disagree.
///
/// Transition table:
///
/// | Stage   | Entered by                    | Left by               | Tab                    |
/// |---------|-------------------------------|-----------------------|------------------------|
/// | todo    | plan, or release              | launch                | Brief                  |
/// | working | launch, send-back, or resume  | hand-back             | Agent                  |
/// | review  | hand-back                     | send-back, approve    | Diff                   |
/// | pr      | approve, or a hand-back after a resume | resume, merge | PR                    |
/// | done    | merge                         | never                 | PR, or Brief with no PR |
///
/// A live session never moves a slice backwards on its own: it outlives
/// hand-back and approve, so "a session exists" says nothing about the stage.
/// A resumed slice (`Slice.resumed`, nat's own reading: `nat slice-resume`
/// wrote a Resumed and cleared the Branch) is In progress and not handed
/// back, so it is `working` whatever its PR — the agent is at it again —
/// until the hand-back that follows re-records the branch.
public enum WorkflowStage: String, CaseIterable, Equatable, Sendable {
    case todo
    case working
    case review
    case pr
    case done

    /// The tab the pane lands on for this stage. A total switch with no
    /// default, so a new stage cannot be added without naming its tab.
    /// `hasPR` only matters to `done`, which has nothing to show on PR
    /// without one.
    public func tab(hasPR: Bool = true) -> WorkflowTab {
        switch self {
        case .todo: return .brief
        case .working: return .agent
        case .review: return .diff
        case .pr: return .pr
        case .done: return hasPR ? .pr : .brief
        }
    }
}

/// The stage of a slice, from the plan's own facts.
///
/// Notion's status is the only source of lifecycle truth, so Done is read off
/// it alone and never re-derived from a PR reading. In progress is told apart
/// by what the slice records: resumed (nat's flag, not re-derived here — a
/// project with no Branch column holds a PR and no branch and is not
/// resumed) is working again; else a PR means approved, a hand-back means
/// review, anything else is working — which is also what a sent-back slice
/// reads as, `slice-rework` having cleared its branch.
///
/// `agent` is taken so the callers pass what they hold, and deliberately never
/// read: a live session moves nothing.
public func stage(for slice: Slice, agent: AgentActivity?) -> WorkflowStage {
    _ = agent
    switch slice.status {
    case "Done":
        return .done
    case "In progress":
        if slice.resumed { return .working }
        if !slice.pr.isEmpty { return .pr }
        return slice.handedBack ? .review : .working
    default:
        return .todo
    }
}

extension WorkflowStage {
    /// The tab for this slice's stage — `tab(hasPR:)` with the slice's own PR.
    public func tab(for slice: Slice) -> WorkflowTab {
        tab(hasPR: !slice.pr.isEmpty)
    }
}
