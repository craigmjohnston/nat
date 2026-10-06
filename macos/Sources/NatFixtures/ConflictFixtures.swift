import Foundation
import NatKit

// MARK: - Merge conflicts, and pull request marks across projects

extension Fixtures {
    /// `nat pr-status --json` with the approved slice's pull request
    /// conflicting with main, its checks green.
    public static let prStatusConflicting = PRStatusDoc(slices: [
        PRStatusSlice(
            sliceID: approveSliceID, name: "Approve opens the pull request", pr: prURL,
            readiness: PRStatusSlice.awaitingReview, checks: PRStatusChecks(verdict: "passing"),
            conflicting: true, base: "main"),
    ])

    /// `nat pr-status --json` with the handed-back slice — in review, no pull
    /// request yet — tested conflicting with origin/main.
    public static let prStatusBranchConflicting = PRStatusDoc(slices: [], branches: [
        PRStatusBranch(
            sliceID: mergeBoxSliceID, name: "Draw the merge box on the PR tab", branch: diffBranch, base: "origin/main",
            conflicting: true),
    ])

    /// The same pull request mergeable, its checks all passed.
    public static let prStatusChecksPassing = PRStatusDoc(slices: [
        PRStatusSlice(
            sliceID: approveSliceID, name: "Approve opens the pull request", pr: prURL,
            readiness: PRStatusSlice.readyToMerge, checks: PRStatusChecks(verdict: PRStatusSlice.checksPassing),
            base: "main"),
    ])

    /// The same pull request red and conflicting at once.
    public static let prStatusChecksFailingAndConflicting = PRStatusDoc(slices: [
        PRStatusSlice(
            sliceID: approveSliceID, name: "Approve opens the pull request", pr: prURL,
            readiness: PRStatusSlice.checksFailing,
            checks: PRStatusChecks(verdict: "failing", failing: [PRStatusCheck(name: "CI / test", url: failingRunURL)]),
            conflicting: true, base: "main"),
    ])

    /// The red pull request (`prFailingChecks`) conflicting with main too.
    public static var prFailingChecksConflicting: PRDetail {
        let red = prFailingChecks
        return PRDetail(
            number: red.number, title: red.title, body: red.body, state: red.state, isDraft: red.isDraft,
            author: red.author, baseRefName: red.baseRefName, headRefName: red.headRefName, url: red.url,
            checks: red.checks, reviews: red.reviews, comments: red.comments, reviewDecision: red.reviewDecision,
            mergeable: "CONFLICTING", mergeStateStatus: "DIRTY", additions: red.additions,
            deletions: red.deletions, changedFiles: red.changedFiles, commits: red.commits)
    }

    /// The second project's slice whose pull request reads checks failing.
    public static let secondRedSliceID = "f1x75333-0000-4000-8000-000000000005"
    /// The second project's slice whose pull request conflicts.
    public static let secondConflictingSliceID = "f1x75333-0000-4000-8000-000000000006"

    /// The second project ("gnat") with two approved slices — pull requests
    /// open, nobody on them — beside its own plan.
    public static var secondProjectInfoWithPRs: ProjectInfo {
        func approved(_ id: String, _ name: String, _ number: Int) -> Slice {
            Slice(
                id: id, name: name, status: "In progress", milestoneID: "Detail overhaul",
                assignee: "Craig Johnston", pr: "https://github.com/craigmjohnston/gnat/pull/\(number)", url: "",
                branch: "slice/\(number)", blocked: false, handedBack: false)
        }
        return ProjectInfo(
            project: secondProjectInfo.project, milestones: secondProjectInfo.milestones,
            slices: secondProjectInfo.slices + [
                approved(secondRedSliceID, "Cache series artwork on disk", 41),
                approved(secondConflictingSliceID, "Page the episode list", 42),
            ])
    }

    /// `nat pr-status --json` for the second project: one pull request red,
    /// one conflicting with main, and its Done slice's long since merged.
    public static let secondProjectPRStatus = PRStatusDoc(slices: [
        PRStatusSlice(
            sliceID: secondRedSliceID, name: "Cache series artwork on disk",
            pr: "https://github.com/craigmjohnston/gnat/pull/41", readiness: PRStatusSlice.checksFailing,
            checks: PRStatusChecks(verdict: "failing", failing: [
                PRStatusCheck(name: "CI / build", url: "https://github.com/craigmjohnston/gnat/actions/runs/7/job/1"),
            ]),
            base: "main"),
        PRStatusSlice(
            sliceID: secondConflictingSliceID, name: "Page the episode list",
            pr: "https://github.com/craigmjohnston/gnat/pull/42", readiness: PRStatusSlice.awaitingReview,
            checks: PRStatusChecks(verdict: "passing"), conflicting: true, base: "main"),
        PRStatusSlice(
            sliceID: "f1x75333-0000-4000-8000-000000000004", name: "Fetch series, episodes and files",
            pr: "https://github.com/craigmjohnston/gnat/pull/38", readiness: "unread"),
    ])
}
