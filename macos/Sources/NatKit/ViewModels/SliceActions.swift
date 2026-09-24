import Foundation

/// The rule for a button that progresses a slice and can only sensibly be
/// pressed once — Launch Agent, Approve, Merge. Driven off the action's own
/// completion rather than off the state refresh that eventually follows it,
/// so the window between "the command returned" and "a poll caught up" never
/// leaves a live button to double-fire on.
///
/// Enabled only while the action is available at all, is not running, and has
/// not already succeeded. A failure re-arms it at once. A success re-arms it
/// only once the action has genuinely become available again: availability
/// has been seen to drop (the state that unlocked it went away) and then
/// return (it recurred) — a relaunch after the agent ended, say.
public struct OneShotAction: Equatable, Sendable {
    public private(set) var isRunning = false
    public private(set) var hasSucceeded = false

    /// Availability was seen false since the last success — the precondition
    /// for the next `true` to count as a recurrence rather than the stale
    /// reading the success itself has not yet outrun.
    private var sawUnavailable = false

    public init() {}

    public func isEnabled(available: Bool) -> Bool {
        available && !isRunning && !hasSucceeded
    }

    public mutating func begin() {
        isRunning = true
    }

    public mutating func succeed() {
        isRunning = false
        hasSucceeded = true
        sawUnavailable = false
    }

    public mutating func fail() {
        isRunning = false
        hasSucceeded = false
        sawUnavailable = false
    }

    /// Feed it the action's availability each time that changes. Only a
    /// success waiting to be outlived cares.
    public mutating func observe(available: Bool) {
        guard hasSucceeded, !isRunning else { return }
        if !available {
            sawUnavailable = true
        } else if sawUnavailable {
            hasSucceeded = false
            sawUnavailable = false
        }
    }
}

/// The slice-progressing actions that are one-shot.
public enum SliceActionKind: Hashable, Sendable, CaseIterable {
    case launch
    case approve
    case merge

    /// The pipeline stage the action moves the slice into, where it moves it
    /// to another one at all: launching hands the slice to the Agent stage,
    /// approving opens the pull request the PR stage reads. A merge ends the
    /// pipeline rather than advancing along it.
    public var advance: StageAdvance? {
        switch self {
        case .launch: return StageAdvance(from: .brief, to: .agent)
        case .approve: return StageAdvance(from: .diff, to: .pr)
        case .merge: return nil
        }
    }
}

/// A move from one pipeline stage to the next that has been shown before it
/// has landed.
public struct StageAdvance: Equatable {
    public let from: WorkflowTab
    public let to: WorkflowTab

    public init(from: WorkflowTab, to: WorkflowTab) {
        self.from = from
        self.to = to
    }
}

/// Holds every one-shot action's state per slice, outside any view: an
/// optimistic advance unmounts the tab the action was taken on, and the
/// action — and the error a failure leaves — must outlive it.
@MainActor
@Observable
public final class SliceActionTracker {
    private struct Key: Hashable {
        let sliceID: String
        let kind: SliceActionKind
    }

    private var actions: [Key: OneShotAction] = [:]
    private var errors: [Key: String] = [:]
    private var advances: [String: StageAdvance] = [:]

    public init() {}

    public func isEnabled(_ kind: SliceActionKind, sliceID: String, available: Bool) -> Bool {
        (actions[Key(sliceID: sliceID, kind: kind)] ?? OneShotAction()).isEnabled(available: available)
    }

    public func isRunning(_ kind: SliceActionKind, sliceID: String) -> Bool {
        actions[Key(sliceID: sliceID, kind: kind)]?.isRunning ?? false
    }

    public func error(_ kind: SliceActionKind, sliceID: String) -> String? {
        errors[Key(sliceID: sliceID, kind: kind)]
    }

    /// The stage move in flight on a slice, if any — what the pane draws a
    /// skeleton for.
    public func advance(for sliceID: String) -> StageAdvance? {
        advances[sliceID]
    }

    public func observe(_ kind: SliceActionKind, sliceID: String, available: Bool) {
        let key = Key(sliceID: sliceID, kind: kind)
        guard var action = actions[key] else { return }
        action.observe(available: available)
        actions[key] = action
    }

    /// Runs an action under the one-shot rule. `select` moves the pane to a
    /// stage: called with the destination the moment the action starts, and
    /// with the origin again if it fails, the error kept for the origin's
    /// tab to show. A call while the same action is already running, or has
    /// succeeded, does nothing.
    public func run(
        _ kind: SliceActionKind,
        sliceID: String,
        select: (WorkflowTab) -> Void,
        perform: () async throws -> Void
    ) async {
        let key = Key(sliceID: sliceID, kind: kind)
        var action = actions[key] ?? OneShotAction()
        guard !action.isRunning, !action.hasSucceeded else { return }
        action.begin()
        actions[key] = action
        errors[key] = nil
        if let advance = kind.advance {
            advances[sliceID] = advance
            select(advance.to)
        }

        do {
            try await perform()
            actions[key]?.succeed()
            advances[sliceID] = nil
        } catch {
            actions[key]?.fail()
            errors[key] = Self.message(for: error)
            if let advance = kind.advance {
                advances[sliceID] = nil
                select(advance.from)
            }
        }
    }

    /// The words a failure is shown in: a refused command's own message,
    /// otherwise the error's description.
    static func message(for error: Error) -> String {
        if let natError = error as? NatError, case .commandFailed(let message) = natError {
            return message
        }
        return error.localizedDescription
    }
}
