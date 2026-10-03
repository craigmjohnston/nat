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
            checks: PRStatusChecks(verdict: "failing", failing: [PRStatusCheck(name: "test", url: failingRunURL)])
        ),
    ])

    /// The plan with the approved slice under a fix — a Relaunched or a Sent
    /// back after its approval, as nat reads `fixing` off the record.
    public static var fixingProjectInfo: ProjectInfo {
        ProjectInfo(
            project: projectInfo.project,
            milestones: projectInfo.milestones,
            slices: projectInfo.slices.map { $0.id == approveSliceID ? fixing($0) : $0 })
    }

    /// A slice as nat reports it once a fix is under way on it.
    public static func fixing(_ s: Slice) -> Slice {
        Slice(
            id: s.id, name: s.name, status: s.status, milestoneID: s.milestoneID, assignee: s.assignee, pr: s.pr,
            url: s.url, branch: s.branch, repo: s.repo, dependsOn: s.dependsOn, blocked: s.blocked,
            handedBack: s.handedBack, fixing: true, state: s.state)
    }

    /// The failure as the record names it, one bullet per check.
    public static let failedChecksNote = "The pull request's checks failed:\n\n- test: \(failingRunURL)"

    /// The approved slice's task log, its last recorded event `last` — then
    /// the approve `slice-show` adds from the slice's own properties.
    public static func approvedSliceDetail(last: TaskLogEvent) -> SliceDetail {
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
            .sentBack, note: "The pull request's checks failed, and the agent was told:\n\n- test: \(failingRunURL)"))]) { _, new in new }
    }

    /// A live fix agent on the approved slice.
    public static var fixAgentStatuses: [AgentStatus] {
        agentStatuses + [
            AgentStatus(sliceID: approveSliceID, session: TmuxSession.name(forSlicePageID: approveSliceID), activity: .working),
        ]
    }
}
