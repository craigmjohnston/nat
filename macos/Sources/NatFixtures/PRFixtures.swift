import Foundation
import NatKit

extension Fixtures {
    /// The pull request every PR fixture is of — the URL the approved slice
    /// records, so a fixture board and a fixture PR tab name the same one.
    public static let prURL = "https://github.com/craigmjohnston/notion-agent-tracker/pull/214"

    static let prBody = """
    Draw the merge box on the PR tab: the three verdicts a merge is weighed on
    — the review, the checks and the branch itself — under the conversation and
    over the merge button, so the two lines that could disagree sit next to
    each other.

    - `mergeVerdicts` reads all three off `PRDetail` alone
    - the heading is the worst of them, so green across the board is the yes
    - `mergeRefusal` is read off the very verdicts the box draws
    """

    static let passingChecks: [PRCheck] = [
        PRCheck(name: "test", state: "SUCCESS", link: prURL + "/checks?check_run_id=1"),
        PRCheck(name: "lint", state: "SUCCESS", link: prURL + "/checks?check_run_id=2"),
        PRCheck(name: "macOS App CI / test", state: "SUCCESS", link: prURL + "/checks?check_run_id=3"),
        PRCheck(name: "codeql", state: "SKIPPED", link: prURL + "/checks?check_run_id=4"),
    ]

    static let failingChecks: [PRCheck] = [
        PRCheck(name: "test", state: "FAILURE", link: prURL + "/checks?check_run_id=1"),
        PRCheck(name: "lint", state: "SUCCESS", link: prURL + "/checks?check_run_id=2"),
        PRCheck(name: "macOS App CI / test", state: "IN_PROGRESS", link: prURL + "/checks?check_run_id=3"),
        PRCheck(name: "codeql", state: "SKIPPED", link: prURL + "/checks?check_run_id=4"),
    ]

    static let approvingReviews: [PRReview] = [
        PRReview(
            author: "craigmjohnston",
            state: "APPROVED",
            body: "Reads well. The refusal coming off the same verdicts is the part I wanted.",
            submittedAt: minutesAgo(35)
        ),
    ]

    static let rejectingReviews: [PRReview] = [
        PRReview(
            author: "craigmjohnston",
            state: "CHANGES_REQUESTED",
            body: "`mergeableVerdict` reads UNKNOWN as a pass — that has to be pending.",
            submittedAt: minutesAgo(28)
        ),
        PRReview(author: "octocat", state: "COMMENTED", body: "", submittedAt: minutesAgo(20)),
    ]

    static let prComments: [PRCommentEntry] = [
        PRCommentEntry(
            author: "craigmjohnston",
            body: "Pushed the rollup fix — `checkRollup` is now the one reading both sections draw from.",
            createdAt: minutesAgo(180),
            url: prURL + "#issuecomment-1"
        ),
        PRCommentEntry(
            author: "octocat",
            body: "Nice. Does the heading follow the worst verdict or the first failing one?",
            createdAt: minutesAgo(120),
            url: prURL + "#issuecomment-2"
        ),
    ]

    /// Everything green: approved, every check passing or skipped, no
    /// conflicts — the merge box's own yes.
    public static let prGreen = PRDetail(
        number: 214,
        title: "Draw the merge box on the PR tab",
        body: prBody,
        state: "OPEN",
        isDraft: false,
        author: "craigmjohnston",
        baseRefName: "main",
        headRefName: diffBranch,
        url: prURL,
        checks: passingChecks,
        reviews: approvingReviews,
        comments: prComments,
        reviewDecision: "APPROVED",
        mergeable: "MERGEABLE",
        mergeStateStatus: "CLEAN",
        additions: diffAdds,
        deletions: diffDels,
        changedFiles: sliceDiff.files.count,
        commits: commits.count
    )

    /// A check has gone red and another is still running, and the review has
    /// asked for changes — the merge box's own no, with a refusal to show.
    public static let prFailingChecks = PRDetail(
        number: 214,
        title: "Draw the merge box on the PR tab",
        body: prBody,
        state: "OPEN",
        isDraft: false,
        author: "craigmjohnston",
        baseRefName: "main",
        headRefName: diffBranch,
        url: prURL,
        checks: failingChecks,
        reviews: rejectingReviews,
        comments: prComments,
        reviewDecision: "CHANGES_REQUESTED",
        mergeable: "MERGEABLE",
        mergeStateStatus: "BLOCKED",
        additions: diffAdds,
        deletions: diffDels,
        changedFiles: sliceDiff.files.count,
        commits: commits.count
    )

    /// The branch conflicts with its base: the review has not been left and
    /// the checks have not been run, so the mergeability is the one verdict
    /// that has anything to say.
    public static let prConflicting = PRDetail(
        number: 214,
        title: "Draw the merge box on the PR tab",
        body: prBody,
        state: "OPEN",
        isDraft: false,
        author: "craigmjohnston",
        baseRefName: "main",
        headRefName: diffBranch,
        url: prURL,
        checks: [],
        reviews: [],
        comments: [],
        reviewDecision: "REVIEW_REQUIRED",
        mergeable: "CONFLICTING",
        mergeStateStatus: "DIRTY",
        additions: diffAdds,
        deletions: diffDels,
        changedFiles: sliceDiff.files.count,
        commits: commits.count
    )

    /// A draft nobody has said anything on and nothing has run on — the two
    /// quiet lines a PR tab draws in place of a conversation and a rollup.
    public static let prDraft = PRDetail(
        number: 215,
        title: "WIP: run the gallery from the fixtures",
        body: "",
        state: "OPEN",
        isDraft: true,
        author: "craigmjohnston",
        baseRefName: "main",
        headRefName: "slice/run-the-gallery-from-the-fixtures",
        url: "https://github.com/craigmjohnston/notion-agent-tracker/pull/215",
        checks: [],
        reviews: [],
        comments: [],
        reviewDecision: "",
        mergeable: "UNKNOWN",
        mergeStateStatus: "UNKNOWN",
        additions: 0,
        deletions: 0,
        changedFiles: 0,
        commits: 1
    )

    /// Already in: the merge box replaced by the ending rather than three
    /// verdicts about a branch that has landed.
    public static let prMerged = PRDetail(
        number: 213,
        title: "Read the plan through nat info",
        body: prBody,
        state: "MERGED",
        isDraft: false,
        author: "craigmjohnston",
        baseRefName: "main",
        headRefName: "slice/read-the-plan-through-nat-info",
        url: "https://github.com/craigmjohnston/notion-agent-tracker/pull/213",
        checks: passingChecks,
        reviews: approvingReviews,
        comments: prComments,
        reviewDecision: "APPROVED",
        mergeable: "MERGEABLE",
        mergeStateStatus: "CLEAN",
        additions: 402,
        deletions: 96,
        changedFiles: 11,
        commits: 6
    )

    // MARK: - Load states

    public static let prStateIdle: PRLoadState = .idle
    public static let prStateLoading: PRLoadState = .loading
    public static let prStateLoaded: PRLoadState = .loaded(prGreen)
    /// A read that failed with nothing behind it.
    public static let prStateFailed: PRLoadState = .failed(prErrorMessage, previous: nil)
    /// A read that failed over a reading already on screen.
    public static let prStateStale: PRLoadState = .failed(prErrorMessage, previous: prGreen)

    public static let prErrorMessage =
        "nat pr-view: gh: no pull requests found for branch \"\(diffBranch)\""
}
