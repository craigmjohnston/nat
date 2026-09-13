import Foundation

/// How long a finished slice's session is left alone after the slice is
/// clicked away from: long enough that clicking through the rail never kills
/// the session the user is about to come back to, short enough that a day's
/// finished work is not still on the tmux server at bedtime.
public let agentReapGrace: TimeInterval = 5 * 60

/// Which live agent sessions are finished with — the slices this project has
/// done, whose sessions are only holding a pane on the tmux server. Killing
/// one is `nat agent-kill`; deciding which is this, kept pure so the rule can
/// be read (and tested) without a tmux anywhere near it.
///
/// Four things have to be true of a slice before its session is reaped:
///
/// - its work is actually done — `sliceWorkDone`, so a Done slice whose pull
///   request is still open is left alone, which is exactly the slice a fix
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
