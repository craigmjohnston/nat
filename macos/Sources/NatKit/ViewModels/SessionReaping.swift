import Foundation

/// How long a slice's pane stays held after being visited — long enough that
/// clicking through the rail never kills the session the user is about to
/// come back to, short enough that a day's finished work is not still on the
/// tmux server at bedtime. Every re-visit resets it, and the slice currently
/// on screen is always held regardless of when it was last visited. Holds
/// exist only within the current app run — the app starts with none, which is
/// what makes a session left over from a previous run dangling rather than
/// merely idle.
public let agentVisitHold: TimeInterval = 5 * 60

/// Which live agent sessions might be worth ending, before the last word —
/// `nat slice-status`, read fresh per candidate — decides each one. Pure so
/// the rule can be read and tested without a tmux or a `nat` anywhere near
/// it.
///
/// It iterates the live sessions themselves rather than a plan's slices,
/// since the whole point is to catch a session belonging to no open plan at
/// all: a project closed, or never opened this run, whose session nonetheless
/// survives. `slicesByID` is every open project's plan merged into one map,
/// keyed by slice ID — a slice ID names at most one project, so nothing is
/// lost merging them, and a session's slice absent from the merge is exactly
/// the "no open plan holds it" case.
///
/// A session is a candidate when its slice is either:
/// - absent from every open project's plan (another, unopened or since-closed
///   project's session, or one whose slice has been deleted), or
/// - present, with a status that is not "In progress" — Done or Todo,
///   Notion's own word disagreeing with a session still running on it.
///
/// Three guards apply here rather than in the verification that follows,
/// since none of them is a question `nat slice-status` could answer any
/// better: an agent mid-turn is somebody working in that session
/// deliberately, and a kill would take the turn with it; the slice on screen
/// is never a dangling one, whatever its status says; and a session whose
/// visit hold is still running was looked at recently enough that killing it
/// now would surprise whoever just clicked away.
///
/// A session tagged as a planning agent rather than a slice
/// (`TmuxSession.isPlanTag`) is never a candidate: it belongs to no slice for
/// a status to disagree with, and nothing here reaps planning agents.
///
/// This is the cheap half of the decision, read off state the app already
/// holds — no tmux and no `nat` reached from here. The last word is `nat
/// slice-status`, asked once per candidate right before the kill (see
/// `AppModel.reapFinishedAgents()`), because a session's own claim is always
/// written before the session exists: a fresh page read taken after observing
/// the session can never show a phantom state, where this plan reading —
/// stale by exactly the length of a poll — might.
///
/// The answer is sorted so a sweep verifies and kills in the same order
/// twice, which is what lets a test say what it asked for.
public func agentSessionsToReap(
    agents: [AgentStatus],
    slicesByID: [String: Slice],
    selectedSliceID: String?,
    heldUntil: [String: Date],
    now: Date
) -> [String] {
    agents
        .filter { agent in
            guard !TmuxSession.isPlanTag(agent.sliceID) else { return false }
            guard agent.activity != .working else { return false }
            guard agent.sliceID != selectedSliceID else { return false }
            if let until = heldUntil[agent.sliceID], until > now { return false }
            guard let slice = slicesByID[agent.sliceID] else { return true }
            return slice.status != "In progress"
        }
        .map(\.sliceID)
        .sorted()
}

/// The last word on one candidate, from a fresh `nat slice-status`: whether
/// its session should actually end.
///
/// In progress is the one answer that keeps it — the fresh read disagrees
/// with the candidate rule's stale plan reading, which is exactly the race
/// this verification exists to close, and the session is doing precisely
/// what an in-progress slice's session should be doing. Done, Todo, or a page
/// read back trashed are each reasons to end it, and a page Notion has no
/// record of at all (`.gone`) is the same answer for the same reason: nothing
/// there is claiming the session any more. A read that failed answers false —
/// nothing is killed on no news, since the one question standing between a
/// session and a kill did not get an answer.
public func verifiedForReap(_ result: SliceStatusResult?) -> Bool {
    switch result {
    case .found(let status, let trashed):
        return trashed || status != "In progress"
    case .gone:
        return true
    case nil:
        return false
    }
}
