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

/// What a sidebar row carries about its pull request's trouble: the checks it
/// was last read failing, and whether it conflicts. Both can be set at once;
/// `.none` draws nothing.
public struct PRMarks: Equatable, Sendable {
    /// The failing checks by name — nil where the checks are not failing, and
    /// empty for a failure the reading named no check of.
    public let failingChecks: [String]?
    public let conflict: BranchConflict?

    public init(failingChecks: [String]? = nil, conflict: BranchConflict? = nil) {
        self.failingChecks = failingChecks
        self.conflict = conflict
    }

    public static let none = PRMarks()

    public var isEmpty: Bool { failingChecks == nil && conflict == nil }

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

    /// Each slice's marks, by slice id — only slices with one.
    public var marks: [String: PRMarks] {
        let failing = failingChecks
        let conflicts = conflicts
        var out: [String: PRMarks] = [:]
        for id in Set(failing.keys).union(conflicts.keys) {
            out[id] = PRMarks(failingChecks: failing[id], conflict: conflicts[id])
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
/// which base, and what is to be done — launch a fix agent, or ask the live
/// one.
public struct ConflictNotice: Equatable, Sendable {
    public enum Action: Equatable, Sendable {
        /// No agent is live, and one can be launched.
        case launchFix
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
        case .launchFix: return "\(named) — launch a fix agent to merge \(base) in and resolve them."
        case .liveAgent: return "\(named) — the live agent has it: ask it to merge \(base) in and resolve them."
        case .none: return "\(named)."
        }
    }
}

/// The conflict notice for a slice, or nil where there is none to draw: only
/// a slice at the PR stage or under a fix whose pull request reads
/// conflicting — the same stage gate the checks notice keeps.
public func conflictNotice(slice: Slice, conflict: BranchConflict?, hasLiveAgent: Bool) -> ConflictNotice? {
    guard let conflict, atPullRequest(slice) else { return nil }
    if hasLiveAgent { return ConflictNotice(conflict: conflict, action: .liveAgent) }
    let action: ConflictNotice.Action = LaunchPlan(for: slice, hasLiveAgent: false).canLaunch ? .launchFix : .none
    return ConflictNotice(conflict: conflict, action: action)
}

/// Whether a slice stands at its pull request — the PR stage, or under a fix
/// — the one stage a pull request's marks are drawn at.
public func atPullRequest(_ slice: Slice) -> Bool {
    switch stage(for: slice, agent: nil) {
    case .pr, .fixing: return true
    case .todo, .working, .review, .done: return false
    }
}
