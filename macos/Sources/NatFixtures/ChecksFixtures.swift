import Foundation
import NatKit

// MARK: - CI failure feedback

extension Fixtures {
    /// The approved slice's failed check, as `nat pr-status` names it.
    public static let failingRunURL = "https://github.com/craigmjohnston/notion-agent-tracker/actions/runs/9001/job/42"

    /// `nat pr-status --json` with the approved slice's pull request red: its
    /// `test` check failed.
    public static let prStatusChecksFailing = PRStatusDoc(slices: [
        PRStatusSlice(
            sliceID: approveSliceID,
            name: "Approve opens the pull request",
            pr: prURL,
            readiness: PRStatusSlice.checksFailing,
            checks: PRStatusChecks(verdict: "failing", failing: [PRStatusCheck(name: "CI / test", url: failingRunURL)])
        ),
    ])

    /// The plan with the approved slice resumed — sent back to its agent
    /// after its approval (`nat slice-resume`), its Branch cleared, as nat
    /// reads `resumed`.
    public static var resumedProjectInfo: ProjectInfo {
        ProjectInfo(
            project: projectInfo.project,
            milestones: projectInfo.milestones,
            slices: projectInfo.slices.map { $0.id == approveSliceID ? resumed($0) : $0 })
    }

    /// The plan with the approved slice resumed on the checks nudge, its fix
    /// not yet handed back: nat's `fixing_checks` names the failed check.
    public static var fixingChecksProjectInfo: ProjectInfo {
        ProjectInfo(
            project: projectInfo.project,
            milestones: projectInfo.milestones,
            slices: projectInfo.slices.map { $0.id == approveSliceID ? resumed($0, fixing: ["CI / test"]) : $0 })
    }

    /// A slice as nat reports it once resumed: its PR kept, its Branch
    /// cleared, not handed back — and the checks it was given to fix, where
    /// a CI failure since its hand-back names some.
    public static func resumed(_ s: Slice, fixing: [String]? = nil) -> Slice {
        Slice(
            id: s.id, name: s.name, status: s.status, milestoneID: s.milestoneID, assignee: s.assignee, pr: s.pr,
            url: s.url, branch: nil, repo: s.repo, dependsOn: s.dependsOn, blocked: s.blocked,
            handedBack: false, resumed: true, takenBack: fixing != nil, fixingChecks: fixing, state: s.state)
    }

    /// Why the approved slice was sent back, as the user put it.
    public static let resumedNote = "Checks are failing on the pull request: CI / test."

    /// The approved slice's task log once resumed: the hand-back, the Work
    /// resumed — then the approve `slice-show` adds from its properties.
    public static var resumedSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([approveSliceID: approvedSliceDetail(last: TaskLogEvent(
            .resumed, note: resumedNote, at: now.addingTimeInterval(-25 * 60)))]) { _, new in new }
    }

    /// The approved slice's task log after the resumed work came back: the
    /// Work resumed, then the hand-back that ended it — the slice at its pull
    /// request again.
    public static var resumedHandedBackSliceDetails: [String: SliceDetail] {
        let detail = approvedSliceDetail(last: TaskLogEvent(
            .resumed, note: resumedNote, at: now.addingTimeInterval(-25 * 60)))
        var events = detail.events ?? []
        events.insert(
            TaskLogEvent(
                .handedBack, note: "Fixed the flaky assertion in the approve test; CI is green on the branch.",
                at: now.addingTimeInterval(-6 * 60)),
            at: events.count - 1)
        return sliceDetails.merging([approveSliceID: SliceDetail(
            id: detail.id, name: detail.name, url: detail.url, status: detail.status, milestone: detail.milestone,
            assignee: detail.assignee, branch: detail.branch, repo: detail.repo, base: detail.base, pr: detail.pr,
            blocked: detail.blocked, handedBack: detail.handedBack, state: detail.state, brief: detail.brief,
            events: events)]) { _, new in new }
    }

    /// The failure as the record names it, one bullet per check.
    public static let failedChecksNote = "The pull request's checks failed:\n\n- test: \(failingRunURL)"

    /// The approved slice's task log, its last recorded event `last` — then
    /// the approve `slice-show` adds from the slice's own properties.
    public static func approvedSliceDetail(last: TaskLogEvent, visuals: [VisualChange] = []) -> SliceDetail {
        SliceDetail(
            id: approveSliceID,
            name: "Approve opens the pull request",
            url: "https://notion.so/\(approveSliceID)",
            status: "In progress",
            milestone: "M2: Review flow",
            assignee: "Craig Johnston",
            branch: "slice/approve-opens-the-pull-request",
            repo: "/Users/craig/Projects/notion-agent-tracker",
            base: "origin/main",
            pr: prURL,
            blocked: false,
            handedBack: false,
            state: "awaiting review",
            brief: "Approving a hand-back opens its pull request and records it on the slice.",
            visuals: visuals,
            events: [
                TaskLogEvent(.handedBack, note: "Approve opens the pull request through `nat slice-approve`."),
                last,
                TaskLogEvent(.approved, pr: prURL),
            ])
    }

    /// The red reading with no agent live: nat filed a Checks failed.
    public static var checksFailedSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([approveSliceID: approvedSliceDetail(last: TaskLogEvent(.checksFailed, note: failedChecksNote))]) { _, new in new }
    }

    /// The red reading with an agent live: nat sent it the failure and filed
    /// a Sent back.
    public static var checksNudgedSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([approveSliceID: approvedSliceDetail(last: TaskLogEvent(
            .sentBack, note: "- test: \(failingRunURL)",
            by: "CI"))]) { _, new in new }
    }

    /// A live agent on the approved slice — one left from its hand-back, or
    /// one back at work on it.
    public static var approvedAgentStatuses: [AgentStatus] {
        agentStatuses + [
            AgentStatus(sliceID: approveSliceID, session: TmuxSession.name(forSlicePageID: approveSliceID), activity: .working),
        ]
    }
}
