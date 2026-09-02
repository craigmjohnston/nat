import Foundation
import SwiftUI

/// The pull request tab's own vocabulary, ported from the Go TUI's
/// `internal/tui/prview.go`, `prchecks.go`, `prconvo.go` and `prmerge.go` (and
/// `internal/actions/mergerefusal.go`, the headless commands' copy of the same
/// rules) rather than called into from here: this is a SwiftUI app with no Go
/// runtime, so the same *decisions* — which four words a state chip draws,
/// which four outcomes a check or a merge verdict amounts to, which reviews
/// belong in the conversation at all — are re-typed here and unit-tested
/// against the same cases the Go tests pin. A change to either's wording
/// belongs in both.

// MARK: - PR state chip

/// Where a pull request stands, in GitHub's own four words — merged, closed,
/// draft, open — and the tint that word is drawn in.
public struct PRStateChip: Equatable, Sendable {
    public let label: String
    public let tint: Color
}

/// `prStateChip` mirrors `internal/tui/prview.go`'s `prStateChip`: merged and
/// closed are tested before draft, since a pull request that has since merged
/// or been closed is no longer a draft whatever the flag still says, and a
/// state this build does not recognise reads as open — the word for a pull
/// request that is neither merged nor closed.
public func prStateChip(state: String, isDraft: Bool) -> PRStateChip {
    if state == PRLifecycleState.merged {
        return PRStateChip(label: "merged", tint: DesignTokens.accent)
    }
    if state == PRLifecycleState.closed {
        return PRStateChip(label: "closed", tint: DesignTokens.systemRed)
    }
    if isDraft {
        return PRStateChip(label: "draft", tint: DesignTokens.labelSecondary)
    }
    return PRStateChip(label: "open", tint: DesignTokens.systemGreen)
}

// MARK: - Check / verdict outcome

/// What a status check — or a merge verdict, which is read exactly the same
/// way — amounts to for a reader: it stands, it does not, it is not settled
/// yet, or it was never asked. Shared between the checks section and the
/// merge box, mirroring the Go TUI's own `checkOutcome`.
public enum CheckOutcome: Equatable, Sendable {
    case passing
    case failing
    case pending
    case skipped

    /// What the rollup line calls a count of this outcome.
    public var word: String {
        switch self {
        case .passing: return "passing"
        case .failing: return "failing"
        case .pending: return "pending"
        case .skipped: return "skipped"
        }
    }

    /// The SF Symbol a row of this outcome opens with.
    public var markSymbolName: String {
        switch self {
        case .passing: return "checkmark.circle.fill"
        case .failing: return "xmark.circle.fill"
        case .pending: return "circle.lefthalf.filled"
        case .skipped: return "minus.circle"
        }
    }

    /// The colour this outcome is drawn in.
    public var tint: Color {
        switch self {
        case .passing: return DesignTokens.systemGreen
        case .failing: return DesignTokens.systemRed
        case .pending: return DesignTokens.systemOrange
        case .skipped: return DesignTokens.systemGray
        }
    }
}

/// The order a rollup line reads its counts in, worst first: what is wrong,
/// what is not settled yet, what stands, what never ran.
private let checkOutcomeOrder: [CheckOutcome] = [.failing, .pending, .passing, .skipped]

/// The GitHub states that are a machine done and either happy or not.
/// Everything else — `QUEUED`, `IN_PROGRESS`, an unrecognised word GitHub adds
/// next — is a check still going, mirroring the Go TUI's own `checkStates`
/// map and its fallback: a state this build does not know is PENDING, never
/// PASSING, since calling it a pass would have the rollup say the work is
/// ready when nothing said so.
private let checkStateOutcomes: [String: CheckOutcome] = [
    "SUCCESS": .passing,
    "FAILURE": .failing,
    "ERROR": .failing,
    "TIMED_OUT": .failing,
    "STARTUP_FAILURE": .failing,
    "ACTION_REQUIRED": .failing,
    "SKIPPED": .skipped,
    "NEUTRAL": .skipped,
    "CANCELLED": .skipped,
    "STALE": .skipped,
]

/// Where one check's own state puts it.
public func checkOutcome(state: String) -> CheckOutcome {
    checkStateOutcomes[state.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()] ?? .pending
}

/// Reads a word GitHub might not have written, in the case and spacing the
/// rest of the interface is written in.
private func humanizedWord(_ raw: String) -> String {
    raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased().replacingOccurrences(of: "_", with: " ")
}

/// How a check's own state is drawn beside its name: GitHub's word as GitHub
/// writes it, lower-cased and despaced — or "unknown" for a check GitHub
/// reported no state for at all.
public func checkStateWord(_ state: String) -> String {
    let trimmed = state.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? "unknown" : humanizedWord(trimmed)
}

/// The worst-first summary line a rollup of checks (or merge verdicts) draws
/// — "2 failing · 1 pending · 5 passing" — and the outcome it is coloured by:
/// the worst any of them is in, which is GitHub's own banner regardless of how
/// many stand.
public struct Rollup: Equatable, Sendable {
    public let summary: String
    public let outcome: CheckOutcome
}

/// `checkRollup` mirrors the Go TUI's `checkSummary`/`checkRollup` as one
/// value: every outcome any check is in, counted, worst first, and the worst
/// of them. A rollup of no checks at all is never reachable through the
/// checks section (which draws one quiet line instead), so it answers
/// PASSING here too — the outcome nothing is wrong in.
public func checkRollup(_ checks: [PRCheck]) -> Rollup {
    var counts: [CheckOutcome: Int] = [:]
    for check in checks {
        counts[checkOutcome(state: check.state), default: 0] += 1
    }
    var parts: [String] = []
    for outcome in checkOutcomeOrder {
        if let count = counts[outcome], count > 0 {
            parts.append("\(count) \(outcome.word)")
        }
    }
    let worst = checkOutcomeOrder.first { (counts[$0] ?? 0) > 0 } ?? .passing
    return Rollup(summary: parts.joined(separator: " · "), outcome: worst)
}

// MARK: - Conversation

/// What an entry of the conversation amounts to at a glance: a verdict for
/// the work, one against it, one taken back, or words with no verdict in
/// them at all. Mirrors the Go TUI's `convoTone`.
public enum ConvoTone: Equatable, Sendable {
    case neutral
    case approved
    case rejected
    case dismissed

    public var markSymbolName: String {
        switch self {
        case .neutral: return "circle.fill"
        case .approved: return "checkmark.circle.fill"
        case .rejected: return "xmark.circle.fill"
        case .dismissed: return "minus.circle"
        }
    }

    public var tint: Color {
        switch self {
        case .neutral: return DesignTokens.labelTertiary
        case .approved: return DesignTokens.systemGreen
        case .rejected: return DesignTokens.systemRed
        case .dismissed: return DesignTokens.systemGray
        }
    }
}

/// `convoTone` mirrors the Go TUI's own reading of a review's state: approved,
/// changes-requested and dismissed take check marks, and every other word —
/// including one this build does not know — is neutral, the same as a plain
/// comment. It is never asked about a review this app has already dropped
/// (`reviewEntry` below drops those first).
public func convoTone(reviewState: String) -> ConvoTone {
    switch reviewState.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() {
    case "APPROVED": return .approved
    case "CHANGES_REQUESTED": return .rejected
    case "DISMISSED": return .dismissed
    default: return .neutral
    }
}

/// One thing said on the pull request, whichever of the two ways GitHub
/// records it: who said it, what they did in saying it, when, and the
/// markdown they wrote (empty for the many reviews that are a verdict and no
/// words). `isReview` tells the two kinds apart for the heading's count
/// alone.
public struct ConvoEntry: Equatable, Sendable {
    public let author: String
    public let verb: String
    public let at: Date
    public let body: String
    public let tone: ConvoTone
    public let isReview: Bool
}

/// Whoever GitHub names. A comment or review left by an account since deleted
/// has no login at all, and is still something that was said.
public func convoAuthor(_ login: String) -> String {
    let trimmed = login.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? "someone" : trimmed
}

/// A review as the timeline draws it, and whether it belongs on the timeline
/// at all.
///
/// Two kinds do not, mirroring the Go TUI's `reviewEntry`: a review with no
/// `submittedAt` — never submitted, or the wrapper GitHub puts around
/// per-line diff comments, which is a COMMENTED review with no words at all —
/// and, for the same reason, any other COMMENTED review with an empty body.
/// Every other wordless review is a verdict, which is the whole of what it
/// had to say.
func reviewEntry(_ review: PRReview) -> ConvoEntry? {
    guard let submittedAt = review.submittedAt else { return nil }
    let state = review.state.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
    let body = review.body.trimmingCharacters(in: .whitespacesAndNewlines)

    let verb: String
    switch state {
    case "APPROVED":
        verb = "approved"
    case "CHANGES_REQUESTED":
        verb = "requested changes"
    case "DISMISSED":
        verb = "review dismissed"
    case "COMMENTED":
        guard !body.isEmpty else { return nil }
        verb = "reviewed"
    default:
        // A word GitHub has added since. Still a review somebody submitted, so
        // it is drawn in GitHub's own word rather than dropped.
        verb = "reviewed (\(humanizedWord(state)))"
    }
    return ConvoEntry(
        author: convoAuthor(review.author), verb: verb, at: submittedAt, body: body,
        tone: convoTone(reviewState: state), isReview: true)
}

/// Everything said on the pull request in the order it was said: the comments
/// and the reviews as one timeline, oldest first — mirrors the Go TUI's
/// `conversation`.
///
/// The comments are gathered first and Swift's `sorted` is a stable sort, so
/// two entries stamped with the same instant keep that order rather than
/// swapping between one read and the next, with the comments first on a tie.
public func conversation(comments: [PRCommentEntry], reviews: [PRReview]) -> [ConvoEntry] {
    var entries: [ConvoEntry] = comments.map { comment in
        ConvoEntry(
            author: convoAuthor(comment.author), verb: "commented", at: comment.createdAt,
            body: comment.body.trimmingCharacters(in: .whitespacesAndNewlines), tone: .neutral, isReview: false)
    }
    for review in reviews {
        if let entry = reviewEntry(review) {
            entries.append(entry)
        }
    }
    return entries.sorted { $0.at < $1.at }
}

/// The count beside the conversation heading, in the two kinds GitHub keeps
/// them in — "2 comments · 1 review" — naming only the kind there is any of.
public func convoSummary(_ entries: [ConvoEntry]) -> String {
    let comments = entries.filter { !$0.isReview }.count
    let reviews = entries.filter(\.isReview).count
    var parts: [String] = []
    if comments > 0 {
        parts.append("\(comments) \(plural(comments, "comment", "comments"))")
    }
    if reviews > 0 {
        parts.append("\(reviews) \(plural(reviews, "review", "reviews"))")
    }
    return parts.joined(separator: " · ")
}

// MARK: - Merge box

/// One line the merge box is weighed on: what it is about, GitHub's answer put
/// into words, and what that answer amounts to. Mirrors the Go TUI's
/// `mergeVerdict` (and `internal/actions/mergerefusal.go`'s copy of the same
/// type for the headless commands).
public struct MergeVerdict: Equatable, Sendable {
    public let label: String
    public let word: String
    public let outcome: CheckOutcome
}

/// The branch a verdict names as the other side of the merge, named in words
/// rather than by a branch nobody can see where the pull request was read
/// with no base at all.
private func baseOf(_ baseRefName: String) -> String {
    let trimmed = baseRefName.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? "its base" : trimmed
}

/// Where the review stands: GitHub's three answers are the three a reader
/// acts on; an empty decision is a repository that requires no review at all
/// — the question never having been asked, never an approval — and anything
/// else GitHub says is a decision nobody here can read as settled.
public func reviewVerdict(reviewDecision: String) -> MergeVerdict {
    switch reviewDecision.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() {
    case "APPROVED":
        return MergeVerdict(label: "review", word: "approved", outcome: .passing)
    case "CHANGES_REQUESTED":
        return MergeVerdict(label: "review", word: "changes requested", outcome: .failing)
    case "REVIEW_REQUIRED":
        return MergeVerdict(label: "review", word: "review required", outcome: .pending)
    case "":
        return MergeVerdict(label: "review", word: "no review required", outcome: .skipped)
    default:
        return MergeVerdict(
            label: "review", word: humanizedWord(reviewDecision), outcome: .pending)
    }
}

/// The checks as one line, off the very rollup the checks section draws its
/// own heading from, so the two can never disagree about whether the
/// machines are happy. A pull request with none configured reports "no
/// checks" rather than reading as a pass.
public func checksVerdict(checks: [PRCheck]) -> MergeVerdict {
    guard !checks.isEmpty else {
        return MergeVerdict(label: "checks", word: "no checks", outcome: .skipped)
    }
    let rollup = checkRollup(checks)
    return MergeVerdict(label: "checks", word: rollup.summary, outcome: rollup.outcome)
}

/// Whether the branch can go in as it stands, read off both of GitHub's
/// fields rather than the mergeable flag alone: a branch that conflicts with
/// its base is named as conflicting (checked first, since either field can
/// say so), one merely behind its base is named as behind, and everything
/// else GitHub has not settled — an unknown mergeability, one read before
/// GitHub computed it, a word this build does not know — is a verdict still
/// to come.
public func mergeableVerdict(mergeable: String, mergeStateStatus: String, baseRefName: String) -> MergeVerdict {
    let base = baseOf(baseRefName)
    let mergeableWord = mergeable.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
    let stateWord = mergeStateStatus.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()

    if mergeableWord == "CONFLICTING" || stateWord == "DIRTY" {
        return MergeVerdict(label: "mergeable", word: "conflicting with \(base)", outcome: .failing)
    }
    if stateWord == "BEHIND" {
        return MergeVerdict(label: "mergeable", word: "behind \(base)", outcome: .pending)
    }
    if mergeableWord == "MERGEABLE" {
        return MergeVerdict(label: "mergeable", word: "no conflicts with \(base)", outcome: .passing)
    }
    return MergeVerdict(label: "mergeable", word: "mergeability unknown", outcome: .pending)
}

/// The three lines the merge box is weighed on, in the order they are read
/// in: who has to say yes, what has to pass, and whether the branch goes in
/// as it stands.
public func mergeVerdicts(_ pr: PRDetail) -> [MergeVerdict] {
    [
        reviewVerdict(reviewDecision: pr.reviewDecision),
        checksVerdict(checks: pr.checks),
        mergeableVerdict(mergeable: pr.mergeable, mergeStateStatus: pr.mergeStateStatus, baseRefName: pr.baseRefName),
    ]
}

/// The worst of the verdicts, in the same worst-first order the checks
/// rollup reads in — one failing verdict is what the section is about
/// however many stand. Unreachable with an empty list through
/// `mergeVerdicts` (which always answers with three); asked anyway, it is the
/// outcome nothing is wrong in.
public func mergeRollup(_ verdicts: [MergeVerdict]) -> CheckOutcome {
    let present = Set(verdicts.map(\.outcome))
    return checkOutcomeOrder.first { present.contains($0) } ?? .passing
}

/// The merge box's own heading: green across the board means yes, one
/// verdict still to come means not yet, and a failing one means no.
public struct MergeHeading: Equatable, Sendable {
    public let words: String
    public let tint: Color
}

public func mergeHeading(_ rollup: CheckOutcome) -> MergeHeading {
    switch rollup {
    case .failing:
        return MergeHeading(words: "cannot merge", tint: rollup.tint)
    case .pending:
        return MergeHeading(words: "not ready to merge", tint: rollup.tint)
    default:
        return MergeHeading(words: "ready to merge", tint: rollup.tint)
    }
}

/// The merge box as a whole: still weighing up the three verdicts, or
/// replaced by the ending a merged or closed pull request has instead —
/// there is nothing left to weigh up, and three verdicts about a branch
/// already in (or already given up on) would read as a question still open.
public enum MergeBoxState: Equatable, Sendable {
    case verdicts(heading: MergeHeading, verdicts: [MergeVerdict])
    case ended(words: String, tint: Color)
}

public func mergeBoxState(for pr: PRDetail) -> MergeBoxState {
    if pr.state == PRLifecycleState.merged {
        return .ended(words: "merged into \(baseOf(pr.baseRefName))", tint: DesignTokens.accent)
    }
    if pr.state == PRLifecycleState.closed {
        return .ended(words: "closed without merging", tint: DesignTokens.systemRed)
    }
    let verdicts = mergeVerdicts(pr)
    return .verdicts(heading: mergeHeading(mergeRollup(verdicts)), verdicts: verdicts)
}

/// Why the merge button would be refused before gh is ever asked to attempt
/// it, in the same words the merge box itself shows — the first failing
/// verdict, of the review, the checks and the mergeability, read in that
/// order. Mirrors `internal/actions/mergerefusal.go`'s `MergeRefusal`, the
/// CLI's own copy of this rule: the button should not offer what the CLI (and
/// the Go TUI) would already refuse.
///
/// Only a failing verdict refuses. A verdict still to come — a review not
/// yet left, checks still running, a mergeability GitHub has not computed —
/// is not a no, and GitHub is the one to say whether it will take the merge
/// anyway.
public func mergeRefusal(_ pr: PRDetail) -> String? {
    for verdict in mergeVerdicts(pr) where verdict.outcome == .failing {
        return "\(verdict.label): \(verdict.word)"
    }
    return nil
}

// MARK: - Relative time

/// A gap in time in the TUI's own coarse words: "is this still how GitHub has
/// it", not how many seconds old it is. A clock that has gone backwards (or a
/// gap under a minute) reads as just now. Mirrors `internal/tui/refresh.go`'s
/// `ago`.
public func ago(_ interval: TimeInterval) -> String {
    switch interval {
    case ..<60:
        return "just now"
    case ..<3600:
        return "\(Int(interval / 60))m ago"
    case ..<86400:
        return "\(Int(interval / 3600))h ago"
    default:
        return "\(Int(interval / 86400))d ago"
    }
}
