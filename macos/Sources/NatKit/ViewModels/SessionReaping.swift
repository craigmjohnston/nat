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
/// It iterates the live sessions and looks each one's slice up in
/// `slicesByID`, every open project's plan merged into one map keyed by slice
/// ID — a slice ID names at most one project, so nothing is lost merging
/// them. A session's slice must be in that merge to be a candidate at all:
/// the slice's own project is the one its verification and kill are asked
/// on, and a session no open plan lists has no project gnat can name. Asked
/// on any other project, `nat slice-status` cannot find the slice, answers
/// `gone`, and the read that exists to keep a live agent would kill it — so
/// such a session (another project's, a closed tab's, one whose slice was
/// deleted) is never touched.
///
/// A session is a candidate when its slice is in an open plan with a status
/// that is not "In progress" — Done or Todo, Notion's own word disagreeing
/// with a session still running on it. In progress runs from claim to merge
/// (Done is written only by the merge), so anything else means no undone
/// work.
///
/// Two guards apply here rather than in the verification that follows,
/// since neither is a question `nat slice-status` could answer any better:
/// the slice on screen is never a dangling one, whatever its status says; and
/// a session whose visit hold is still running was looked at recently enough
/// that killing it now would surprise whoever just clicked away. The agent's
/// own activity is no guard: a pane reads as working until its agent says it
/// is waiting (`nat agent-waiting`), which one that has handed back never
/// does, so it would keep every finished session alive — the slice's status
/// is what says its work is over.
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
            guard agent.sliceID != selectedSliceID else { return false }
            if let until = heldUntil[agent.sliceID], until > now { return false }
            guard let slice = slicesByID[agent.sliceID] else { return false }
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
