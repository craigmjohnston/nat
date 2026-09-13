import Foundation

/// How long a finished slice's session is left alone after the slice is
/// clicked away from: long enough that clicking through the rail never kills
/// the session the user is about to come back to, short enough that a day's
/// finished work is not still on the tmux server at bedtime.
public let agentReapGrace: TimeInterval = 5 * 60

/// Which live agent sessions might be finished with — the slices this project
/// has done, whose sessions are only holding a pane on the tmux server.
/// Killing one is `nat agent-kill`; deciding which is this, kept pure so the
/// rule can be read (and tested) without a tmux anywhere near it.
///
/// This is the cheap half of the decision and not the whole of it: a
/// candidate whose slice records a pull request is reaped only once that pull
/// request has been positively read as landed — `prIsSettled`, off a reading
/// of GitHub itself. Every reading here is of the plan, which says a slice is
/// Done, and Notion's Done is not proof the work is on main.
///
/// Four things have to be true of a slice before its session is reaped:
///
/// - its work is done as far as the plan and the last PR-readiness reading
///   are concerned — `sliceWorkDone`, so a Done slice whose pull request was
///   read as still open is left alone, which is exactly the slice a fix
///   session runs on (see the domain rule on `l`);
/// - it is not the slice on screen, since a session being looked at is not a
///   dangling one;
/// - it was clicked away from at least `grace` ago. `leftAt` is when each
///   slice last lost the selection this run; a slice with no stamp was never
///   selected, which is every session left over from a previous run of the
///   app — those are the dangling ones, and they go on the first sweep;
/// - its agent is not mid-turn. A finished slice with an agent still working
///   is somebody working in that session deliberately, and a kill would take
///   the turn with it; the next sweep after it settles is soon enough.
///
/// The answer is sorted so a sweep kills in the same order twice, which is
/// what lets a test say what it asked for.
public func agentSessionsToReap(
    agents: [String: AgentStatus],
    slices: [Slice],
    openPRSliceIDs: Set<String>,
    selectedSliceID: String?,
    leftAt: [String: Date],
    now: Date,
    grace: TimeInterval = agentReapGrace
) -> [String] {
    slices
        .filter { slice in
            guard let agent = agents[slice.id] else { return false }
            guard sliceWorkDone(slice, openPRSliceIDs: openPRSliceIDs) else { return false }
            guard slice.id != selectedSliceID else { return false }
            guard agent.activity != .working else { return false }
            return now.timeIntervalSince(leftAt[slice.id] ?? .distantPast) >= grace
        }
        .map(\.id)
        .sorted()
}

/// Whether a pull request has stopped being something anybody is waiting on:
/// merged, or closed without merging — GitHub's own two endings, which is
/// what `PRLifecycleState` names.
///
/// This is the reading a kill needs and `sliceWorkDone` cannot give. The
/// PR-readiness listing the rest of the app rides says only whether a pull
/// request was positively seen open, and deliberately does not tell "no
/// longer open" apart from "the repository could not be asked" — right for a
/// chip that goes undrawn, wrong for a session that gets killed. So a
/// candidate carrying a pull request is read in full first, and anything but
/// these two endings — still open, a draft, a read that failed at all —
/// leaves its session exactly where it is.
public func prIsSettled(_ pr: PRDetail) -> Bool {
    pr.state == PRLifecycleState.merged || pr.state == PRLifecycleState.closed
}
