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
/// read failing, whether it conflicts, and whether its checks all passed.
/// Failing and conflict can be set at once; `.none` draws nothing.
public struct PRMarks: Equatable, Sendable {
    /// The failing checks by name — nil where the checks are not failing, and
    /// empty for a failure the reading named no check of.
    public let failingChecks: [String]?
    public let conflict: BranchConflict?
    /// Whether the checks were last read passing. `PRReading.marks` sets it
    /// from the verdict alone; `prMarks(_:for:agent:)` keeps it only where
    /// the green tick can be trusted.
    public let checksPassing: Bool

    public init(failingChecks: [String]? = nil, conflict: BranchConflict? = nil, checksPassing: Bool = false) {
        self.failingChecks = failingChecks
        self.conflict = conflict
        self.checksPassing = checksPassing
    }

    public static let none = PRMarks()

    public var isEmpty: Bool { failingChecks == nil && conflict == nil && !checksPassing }

    /// The success mark's tooltip, where there is one.
    public var passingHelp: String? {
        checksPassing ? "Checks passing" : nil
    }

    /// The danger mark's tooltip, where there is one.
    public var checksHelp: String? {
        failingChecks.map { $0.isEmpty ? "Checks failing" : "Checks failing: \($0.joined(separator: ", "))" }
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

    /// Every pull request GitHub said conflicts, by slice id.
    public var conflicts: [String: BranchConflict] {
        doc.slices.reduce(into: [:]) { map, slice in
            if slice.conflicting { map[slice.sliceID] = BranchConflict(base: slice.base) }
        }
    }

    /// Every pull request whose checks were read passing, by slice id.
    public var passingChecks: Set<String> {
        Set(doc.slices.filter { $0.checks?.verdict == PRStatusSlice.checksPassing }.map(\.sliceID))
    }

    /// Each slice's marks, by slice id — only slices with one.
    public var marks: [String: PRMarks] {
        let failing = failingChecks
        let conflicts = conflicts
        let passing = passingChecks
        var out: [String: PRMarks] = [:]
        for id in Set(failing.keys).union(conflicts.keys).union(passing) {
            out[id] = PRMarks(failingChecks: failing[id], conflict: conflicts[id], checksPassing: passing.contains(id))
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

/// What the PR section says about a pull request that conflicts with its base:
/// which base, and what is to be done — send it back to the agent, or ask
/// the live one.
public struct ConflictNotice: Equatable, Sendable {
    public enum Action: Equatable, Sendable {
        /// No agent is live, and one can be launched: the action bar's Send
        /// back to agent resumes the slice and launches one.
        case sendBack
        /// An agent is live on the slice: it is the one to resolve them.
        case liveAgent
        /// Neither: no agent, and no launch open.
        case none
    }

    public let conflict: BranchConflict
    public let action: Action

    public init(conflict: BranchConflict, action: Action) {
        self.conflict = conflict
        self.action = action
    }

    /// The notice's words.
    public var text: String {
        let base = conflict.base ?? "its base"
        let named = "This branch conflicts with \(base)"
        switch action {
        case .sendBack: return "\(named) — send it back to the agent to merge \(base) in and resolve them."
        case .liveAgent: return "\(named) — the live agent has it: ask it to merge \(base) in and resolve them."
        case .none: return "\(named)."
        }
    }
}

/// The conflict notice for a slice, or nil where there is none to draw: only
/// a slice at the PR stage whose pull request reads conflicting — the same
/// stage gate the checks notice keeps.
public func conflictNotice(slice: Slice, conflict: BranchConflict?, hasLiveAgent: Bool) -> ConflictNotice? {
    guard let conflict, atPullRequest(slice) else { return nil }
    if hasLiveAgent { return ConflictNotice(conflict: conflict, action: .liveAgent) }
    let action: ConflictNotice.Action = LaunchPlan(for: slice, hasLiveAgent: false).canLaunch ? .sendBack : .none
    return ConflictNotice(conflict: conflict, action: action)
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
/// none off its pull request (`atPullRequest`); failing and conflict as read;
/// and the passing tick only where it can be trusted — at the PR stage
/// exactly, with no live agent working (an idle one left from hand-back is fine), and
/// the pull request neither conflicting nor read failing.
public func prMarks(_ marks: PRMarks, for slice: Slice, agent: AgentActivity?) -> PRMarks {
    guard atPullRequest(slice) else { return .none }
    let passing = marks.checksPassing && marks.failingChecks == nil && marks.conflict == nil
        && agent != .working
    return PRMarks(failingChecks: marks.failingChecks, conflict: marks.conflict, checksPassing: passing)
}
