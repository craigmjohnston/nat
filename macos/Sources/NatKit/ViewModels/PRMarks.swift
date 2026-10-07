import Foundation

/// A branch that conflicts with the one it merges into — what the conflict
/// mark draws and names. Nothing in it is about a pull request: a handed-back
/// branch with none that conflicts is the same mark.
public struct BranchConflict: Equatable, Sendable {
    /// The branch it conflicts with, where the reading named one.
    public let base: String?

    public init(base: String?) {
        let trimmed = base?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.base = trimmed?.isEmpty == false ? trimmed : nil
    }

    /// The mark's tooltip.
    public var help: String {
        base.map { "Conflicts with \($0)" } ?? "Merge conflicts"
    }
}

/// What a sidebar row carries about its pull request: the checks it was last
/// read failing, whether it conflicts, and whether its checks all passed or
/// are still running.
/// Failing and conflict can be set at once; `.none` draws nothing.
public struct PRMarks: Equatable, Sendable {
    /// The failing checks by name — nil where the checks are not failing, and
    /// empty for a failure the reading named no check of.
    public let failingChecks: [String]?
    public let conflict: BranchConflict?
    /// Whether the checks were last read passing. `PRReading.marks` sets it
    /// from the verdict alone; `prMarks(_:for:)` keeps it only where the
    /// green tick can be trusted.
    public let checksPassing: Bool
    /// Whether the checks were last read still running — set and kept by the
    /// same rule as `checksPassing`, drawn in the same slot.
    public let checksRunning: Bool

    public init(
        failingChecks: [String]? = nil, conflict: BranchConflict? = nil, checksPassing: Bool = false,
        checksRunning: Bool = false
    ) {
        self.failingChecks = failingChecks
        self.conflict = conflict
        self.checksPassing = checksPassing
        self.checksRunning = checksRunning
    }

    public static let none = PRMarks()

    public var isEmpty: Bool { failingChecks == nil && conflict == nil && !checksPassing && !checksRunning }

    /// The success mark's tooltip, where there is one.
    public var passingHelp: String? {
        checksPassing ? "Checks passing" : nil
    }

    /// The running mark: GitHub's "in progress" as the check rows' filled
    /// circles draw it — an ellipsis — in a neutral ink, never a warning.
    /// The PR header draws its outline form.
    public static let runningSymbol = "ellipsis.circle.fill"
    public static let runningOutlineSymbol = "ellipsis.circle"

    /// The running mark's tooltip, where there is one.
    public var runningHelp: String? {
        checksRunning ? "Checks running" : nil
    }

    /// The danger mark's tooltip, where there is one.
    public var checksHelp: String? {
        failingChecks.map { $0.isEmpty ? "Checks failing" : "Checks failing: \($0.joined(separator: ", "))" }
    }
}

/// The glyph and ink a check row leads with, by outcome — the sidebar's own
/// marks for passing, failing and running, and a slashed circle for a check
/// that never ran, whose row is drawn faded and struck through
/// (`isSkipped`).
public struct CheckRowMark: Equatable, Sendable {
    public let symbol: String
    public let role: InkRole

    public init(_ outcome: CheckOutcome) {
        switch outcome {
        case .passing: (symbol, role) = ("checkmark.circle.fill", .success)
        case .failing: (symbol, role) = ("xmark.circle.fill", .danger)
        case .pending: (symbol, role) = (PRMarks.runningSymbol, .secondary)
        case .skipped: (symbol, role) = ("slash.circle", .tertiary)
        }
    }
}

/// One project's `nat pr-status` reading as the app keeps it — the last that
/// arrived, replaced only by a newer one.
public struct PRReading: Equatable, Sendable {
    public let doc: PRStatusDoc

    public init(_ doc: PRStatusDoc) {
        self.doc = doc
    }

    public static let empty = PRReading(PRStatusDoc(slices: []))

    /// The readiness words of every pull request positively read as open, by
    /// slice id. A slice absent here has no open pull request as far as
    /// anything has read, which for a Done slice is what keeps a project's
    /// whole finished history out of the Active section.
    public var readiness: [String: String] {
        doc.slices.reduce(into: [:]) { map, slice in
            if slice.isOpen { map[slice.sliceID] = slice.readiness }
        }
    }

    /// The names of the checks each pull request read "checks failing" has
    /// failed, by slice id.
    public var failingChecks: [String: [String]] {
        doc.slices.reduce(into: [:]) { map, slice in
            if slice.readiness == PRStatusSlice.checksFailing {
                map[slice.sliceID] = (slice.checks?.failing ?? []).map(\.name)
            }
        }
    }

    /// The checks the PR section lists for a slice's pull request: the
    /// reading's — the one the heading and the sidebar marks come from —
    /// wherever it read the slice's checks, each carrying what only the
    /// detail (`pr-view`) knows of the check by that name (`rerunnable`,
    /// `run`); else the detail's own (`detail`), as for an ad hoc session's
    /// pull request, which the reading lists under no slice, or before any
    /// reading has landed.
    public func checkRows(sliceID: String, detail: [PRCheck]) -> [PRCheck] {
        guard let read = doc.slices.first(where: { $0.sliceID == sliceID })?.checks?.checks else { return detail }
        let known = Dictionary(detail.map { ($0.name, $0) }, uniquingKeysWith: { first, _ in first })
        return read.map { check in
            let old = known[check.name]
            return PRCheck(
                name: check.name, state: check.state, link: check.url.isEmpty ? old?.link ?? "" : check.url,
                rerunnable: old?.rerunnable ?? false, run: old?.run)
        }
    }

    /// Every pull request GitHub said conflicts, by slice id.
    public var conflicts: [String: BranchConflict] {
        doc.slices.reduce(into: [:]) { map, slice in
            if slice.conflicting { map[slice.sliceID] = BranchConflict(base: slice.base) }
        }
    }

    /// Every hand-back with no pull request nat tested conflicting with its
    /// base, by slice id. A branch nat could not test is not in the reading
    /// at all, so it is never here.
    public var branchConflicts: [String: BranchConflict] {
        doc.branches.reduce(into: [:]) { map, branch in
            if branch.conflicting { map[branch.sliceID] = BranchConflict(base: branch.base) }
        }
    }

    /// Every pull request whose checks were read passing, by slice id.
    public var passingChecks: Set<String> {
        Set(doc.slices.filter { $0.checks?.verdict == PRStatusSlice.checksPassing }.map(\.sliceID))
    }

    /// Every pull request whose checks were read still running, by slice id.
    public var runningChecks: Set<String> {
        Set(doc.slices.filter { $0.checks?.verdict == PRStatusSlice.checksPending }.map(\.sliceID))
    }

    /// Each slice's marks, by slice id — only slices with one. A slice's
    /// conflict is its pull request's, else its handed-back branch's: a slice
    /// has one or the other, never both.
    public var marks: [String: PRMarks] {
        let failing = failingChecks
        let conflicts = conflicts.merging(branchConflicts) { pr, _ in pr }
        let passing = passingChecks
        let running = runningChecks
        var out: [String: PRMarks] = [:]
        for id in Set(failing.keys).union(conflicts.keys).union(passing).union(running) {
            out[id] = PRMarks(
                failingChecks: failing[id], conflict: conflicts[id], checksPassing: passing.contains(id),
                checksRunning: running.contains(id))
        }
        return out
    }
}

/// The conflict the PR section draws for the pull request at `prURL`: the
/// `pr-status` reading's, unless a loaded `pr-view` of that same pull request
/// is in hand — a `PRDetail` is the fresher reading, so it decides,
/// conflicting by the merge box's own rule (`mergeableVerdict`), else not. A
/// detail of some other pull request (the store's last, before the slice's
/// own is read) says nothing about this one.
public func conflict(reading: BranchConflict?, detail: PRDetail?, prURL: String) -> BranchConflict? {
    guard let detail, detail.number == pullRequestNumber(prURL) else { return reading }
    let verdict = mergeableVerdict(
        mergeable: detail.mergeable, mergeStateStatus: detail.mergeStateStatus, baseRefName: detail.baseRefName)
    return verdict.outcome == .failing ? BranchConflict(base: detail.baseRefName) : nil
}

/// What the PR section says about a pull request that conflicts with its base
/// — or the Changes section about a handed-back branch with no pull request
/// that does: which base, and what is to be done — send it back to the agent,
/// or ask the live one.
public struct ConflictNotice: Equatable, Sendable {
    public enum Action: Equatable, Sendable {
        /// No agent is live, and one can be launched: the action bar's
        /// Resolve conflicts (`BarFix`) resumes the slice and launches one.
        case sendBack
        /// An agent is live on the slice: it is the one to resolve them —
        /// the bar's Resolve conflicts tells it.
        case liveAgent
        /// Neither: no agent, and no launch open.
        case none
    }

    public let conflict: BranchConflict
    public let action: Action
    /// Whether the branch has a pull request. One that has is brought up to
    /// date by merging its base in, which keeps the history its review is
    /// of; one still under review in the app alone, with nothing published,
    /// is rebased onto it.
    public let hasPullRequest: Bool

    public init(conflict: BranchConflict, action: Action, hasPullRequest: Bool = true) {
        self.conflict = conflict
        self.action = action
        self.hasPullRequest = hasPullRequest
    }

    /// What the agent is to do about the conflict, in the words both the
    /// notice and Send back's prefill use.
    var remedy: String {
        let base = conflict.base ?? "its base"
        return hasPullRequest ? "merge \(base) in" : "rebase it on \(base)"
    }

    /// The notice's words.
    public var text: String {
        let named = "This branch conflicts with \(conflict.base ?? "its base")"
        switch action {
        case .sendBack: return "\(named) — send it back to the agent to \(remedy) and resolve them."
        case .liveAgent: return "\(named) — the live agent has it: ask it to \(remedy) and resolve them."
        case .none: return "\(named)."
        }
    }
}

/// The conflict notice for a slice, or nil where there is none to draw: only
/// a slice at the PR stage whose pull request reads conflicting — the same
/// stage gate the checks notice keeps.
public func conflictNotice(slice: Slice, conflict: BranchConflict?, hasLiveAgent: Bool) -> ConflictNotice? {
    guard let conflict, atPullRequest(slice) else { return nil }
    return ConflictNotice(conflict: conflict, action: conflictAction(slice, hasLiveAgent: hasLiveAgent))
}

/// The conflict notice the Changes section draws for a handed-back branch
/// with no pull request, or nil where there is none: only a slice in review
/// whose branch `pr-status` tested conflicting — never one it could not test.
public func branchConflictNotice(slice: Slice, conflict: BranchConflict?, hasLiveAgent: Bool) -> ConflictNotice? {
    guard let conflict, inReview(slice) else { return nil }
    return ConflictNotice(
        conflict: conflict, action: conflictAction(slice, hasLiveAgent: hasLiveAgent), hasPullRequest: false)
}

/// What a conflict notice points at: the live agent, Send back where a launch
/// is open, else nothing.
private func conflictAction(_ slice: Slice, hasLiveAgent: Bool) -> ConflictNotice.Action {
    if hasLiveAgent { return .liveAgent }
    return LaunchPlan(for: slice, hasLiveAgent: false).canLaunch ? .sendBack : .none
}

/// Whether a slice is handed back and under review with no pull request yet
/// — the one stage a handed-back branch's own conflict reading is drawn at.
public func inReview(_ slice: Slice) -> Bool {
    stage(for: slice, agent: nil) == .review
}

/// Whether a slice stands at its pull request — the PR stage — the one stage
/// a pull request's marks are drawn at. A resumed slice is working again, its
/// PR's reading about a commit the agent is about to replace.
public func atPullRequest(_ slice: Slice) -> Bool {
    switch stage(for: slice, agent: nil) {
    case .pr: return true
    case .todo, .working, .review, .done: return false
    }
}

/// The marks a slice's rows and PR heading draw, from its reading's `marks`:
/// a slice in review (`inReview`) its handed-back branch's conflict alone; a
/// resumed one — its work kicked back to the agent — its failing checks
/// alone: as read, or, while the checks run again, the failure its agent was
/// given and has not handed back a fix for (`Slice.fixingChecks`, off its
/// task log); none elsewhere off its pull request (`atPullRequest`).
/// At the PR stage, failing and conflict as read, and the passing tick and
/// the running mark where the pull request is neither conflicting nor read
/// failing — whatever its agent's activity reads, since an agent left idle
/// after its hand-back reads as working. The two share the checks' slot.
public func prMarks(_ marks: PRMarks, for slice: Slice) -> PRMarks {
    // In review, before any pull request: its branch's conflict alone.
    if inReview(slice) { return PRMarks(conflict: marks.conflict) }
    if slice.resumed {
        return PRMarks(failingChecks: marks.failingChecks ?? (marks.checksRunning ? slice.fixingChecks : nil))
    }
    guard atPullRequest(slice) else { return .none }
    let clear = marks.failingChecks == nil && marks.conflict == nil
    return PRMarks(
        failingChecks: marks.failingChecks, conflict: marks.conflict, checksPassing: clear && marks.checksPassing,
        checksRunning: clear && marks.checksRunning)
}
