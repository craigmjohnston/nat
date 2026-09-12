import Foundation

/// The one thing a project's dot says: what, of everything in flight on it,
/// is the most worth the user's eye. Named for meaning rather than a colour,
/// like `ActiveTintRole` — the view is what maps a role onto a
/// `DesignTokens` colour, and the rail's own vocabulary is the same one:
/// waiting yellow, review green, working accent, nothing-happening muted.
public enum ProjectAttentionRole: Equatable, Sendable {
    /// An agent has stopped and wants an answer — the planning agent
    /// included. Nothing outranks it: it is the only state where work has
    /// actually halted on the user.
    case waiting
    /// Nothing is waiting, but there is something to read: a branch handed
    /// back, or a pull request GitHub will take.
    case review
    /// Agents are working and nothing wants the user. The one state that
    /// pulses — a moving dot reads as "busy", a still coloured one as
    /// "something waits on you".
    case working
    /// No live agent, nothing to review. A neutral dot rather than no dot,
    /// so a tab does not change width as its project goes quiet.
    case idle
}

/// A project's whole attention reading: how many things want the user now,
/// and which state the dot should say they are in. One value for both, so
/// the pill and the dot can never disagree with each other — or with the
/// rail, which draws the same roles.
public struct ProjectAttention: Equatable, Sendable {
    /// How many things need the user now. See `projectAttention` for what
    /// counts.
    public let count: Int
    public let role: ProjectAttentionRole

    public init(count: Int, role: ProjectAttentionRole) {
        self.count = count
        self.role = role
    }

    /// The pill's number, nil-suppressed at zero: a project with nothing
    /// waiting draws no badge at all rather than a "0".
    public var badge: Int? {
        count > 0 ? count : nil
    }

    /// Only the working dot pulses.
    public var pulses: Bool {
        role == .working
    }

    /// A project nothing has been read of.
    public static let none = ProjectAttention(count: 0, role: .idle)
}

/// Reads a project's attention off the same three facts the rail is built
/// from: the plan's slices, the live tmux map, and the PR-readiness reading.
/// Pure, on the `buildRailModel` pattern — inputs in, a value out, and no
/// store reached for here.
///
/// `liveAgents` may be the whole activity map; only the entries naming a
/// slice of this project that the rail's ACTIVE section would draw are read
/// (`inFlightSliceIDs`), since the map is one reading across every project
/// the app has open and a session can outlive its slice. The planning agent
/// has no slice to be keyed by and is passed separately, nil where the map
/// attributes none to this project.
///
/// What counts towards the pill is a thing needing the user *now*: an agent
/// waiting for input, a slice handed back for review, and a pull request
/// ready to merge. Counted per slice rather than per fact, so a handed-back
/// slice whose agent is also waiting is one thing to attend to and not two;
/// a waiting planning agent is one more, being nobody's slice.
///
/// A pull request merely awaiting its review is not counted and not drawn:
/// the slice it belongs to is already counted while it is handed back, and
/// once approved what the review owes is somebody else's turn.
public func projectAttention(
    slices: [Slice],
    liveAgents: [String: AgentActivity],
    planningAgent: AgentActivity? = nil,
    prReadiness: [String: String] = [:]
) -> ProjectAttention {
    // Only a slice the ACTIVE section would draw may contribute an agent:
    // a tmux session can outlive the slice it was launched on — an idle
    // Claude Code left in the pane of a Done slice whose pull request has
    // merged — and the rail refuses exactly that. One rule for both, so the
    // dot and the section can never disagree about what is in flight.
    let inFlight = inFlightSliceIDs(slices: slices, openPRSliceIDs: Set(prReadiness.keys))
    let agents = liveAgents.filter { inFlight.contains($0.key) }
    let waiting = agents.filter { $0.value == .waiting }.keys
    let planningWaiting = planningAgent == .waiting

    // The slices with something for the user to do about them.
    var pending = Set(
        slices
            .filter { $0.handedBack || prReadiness[$0.id] == PRStatusSlice.readyToMerge }
            .map(\.id)
    )
    pending.formUnion(waiting)

    let count = pending.count + (planningWaiting ? 1 : 0)

    let role: ProjectAttentionRole
    if planningWaiting || !waiting.isEmpty {
        role = .waiting
    } else if !pending.isEmpty {
        role = .review
    } else if !agents.isEmpty || planningAgent != nil {
        role = .working
    } else {
        role = .idle
    }

    return ProjectAttention(count: count, role: role)
}
