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


/// What a thing waiting on the user is waiting for, in order of urgency —
/// the order the dock menu lists them in, and the one a slice with two such
/// facts is counted under (the first), since it is one thing to attend to.
public enum AttentionKind: Int, CaseIterable, Comparable, Sendable {
    /// An agent has stopped and wants an answer — a slice's, a session's or
    /// the planning agent's.
    case waiting
    /// A branch handed back, the user's turn to read the diff — or an ad hoc
    /// session gone with a pull request still open.
    case review
    /// A pull request's checks read failing with no live agent on its slice.
    case checksFailed
    /// A pull request conflicting with its base with no live agent on its
    /// slice.
    case conflict
    /// A pull request whose checks passed, mergeable, no agent working on it.
    case readyToMerge

    public static func < (lhs: Self, rhs: Self) -> Bool { lhs.rawValue < rhs.rawValue }

    /// The dock menu's heading for the kind.
    public var heading: String {
        switch self {
        case .waiting: return "Waiting for input"
        case .review: return "Handed back for review"
        case .checksFailed: return "Checks failed"
        case .conflict: return "Conflicts"
        case .readyToMerge: return "Ready to merge"
        }
    }
}

/// What an attention item is about — what choosing it selects.
public enum AttentionSubject: Hashable, Sendable {
    case slice(String)
    /// An ad hoc session, by its id.
    case session(String)
    /// The project's planning agent, which is nobody's slice.
    case workshop
}

/// One thing waiting on the user: one per slice (or session, or planning
/// agent), never one per fact.
public struct AttentionItem: Hashable, Sendable {
    public let kind: AttentionKind
    public let subject: AttentionSubject
    /// What the item is called — the slice's name, "Workshop", or a
    /// session's "Ad hoc session" and its label.
    public let name: String
    public let projectID: String

    public init(kind: AttentionKind, subject: AttentionSubject, name: String, projectID: String) {
        self.kind = kind
        self.subject = subject
        self.name = name
        self.projectID = projectID
    }

    /// What makes an item the same item across two readings: where it is and
    /// what it waits for. A slice moving from one kind to another is a new
    /// arrival — something new wants the user — and a renamed one is not.
    public struct Identity: Hashable, Sendable {
        public let projectID: String
        public let subject: AttentionSubject
        public let kind: AttentionKind
    }

    public var identity: Identity { Identity(projectID: projectID, subject: subject, kind: kind) }
}

/// Everything in a project waiting on the user, read off the same facts the
/// rail is built from: the plan's slices, the live tmux map, the project's
/// `pr-status` reading, the planning agent and the active project's ad hoc
/// sessions. Pure, on the `buildRailModel` pattern. Ordered by kind, then by
/// plan order, then sessions, then the planning agent.
///
/// `liveAgents` may be the whole activity map; only the entries naming a
/// slice of this project that the rail's ACTIVE section would draw are read
/// (`inFlightSliceIDs`), since the map is one reading across every project
/// the app has open and a session can outlive its slice. The planning agent
/// has no slice to be keyed by and is passed separately, nil where the map
/// attributes none to this project.
///
/// One item per slice, under its most urgent fact:
/// - **waiting** — its agent waiting for input;
/// - **review** — handed back, no pull request yet (stage `review`);
/// - **checksFailed** — at the PR stage, read failing, and **no live agent**:
///   with one, `pr-status` has already sent the failure to it in the same
///   reading (`actions.NoticeFailingChecks`), so it is never the user's —
///   not even for the one plan read before `resumed` lands;
/// - **conflict** — at the PR stage, conflicting, and no live agent (the
///   conflict notice names the live agent rather than the user);
/// - **readyToMerge** — the sidebar's own passing-checks gate
///   (`prMarks(_:for:agent:)`): the PR stage, checks read passing, neither
///   conflicting nor failing, no agent working.
///
/// A resumed slice is working, so it counts for nothing but a waiting agent;
/// a Done slice for nothing at all. A pull request merely awaiting its review
/// with checks pending is not counted: nothing in it is the user's yet.
///
/// Sessions are read as `RailModel`'s session rows are: a waiting agent is
/// waiting, a gone one with a pull request still open review
/// (`sessionNeedsReview`).
public func attentionItems(
    projectID: String = "",
    slices: [Slice],
    liveAgents: [String: AgentActivity],
    planningAgent: AgentActivity? = nil,
    prReading: PRReading = .empty,
    sessions: [Session] = []
) -> [AttentionItem] {
    // Only a slice the ACTIVE section would draw may contribute an agent:
    // a tmux session can outlive the slice it was launched on — an idle
    // Claude Code left in the pane of a Done slice whose pull request has
    // merged — and the rail refuses exactly that. One rule for both, so the
    // count and the section can never disagree about what is in flight.
    let inFlight = inFlightSliceIDs(slices: slices)
    let marks = prReading.marks

    var items: [AttentionItem] = []
    func add(_ kind: AttentionKind?, _ subject: AttentionSubject, _ name: String) {
        if let kind { items.append(AttentionItem(kind: kind, subject: subject, name: name, projectID: projectID)) }
    }

    for slice in slices {
        let agent = inFlight.contains(slice.id) ? liveAgents[slice.id] : nil
        add(attentionKind(slice, agent: agent, marks: marks[slice.id] ?? .none), .slice(slice.id), slice.name)
    }
    for session in sessions {
        let kind: AttentionKind? = liveAgents[session.tag] == .waiting ? .waiting
            : sessionNeedsReview(session, liveAgents: liveAgents) ? .review : nil
        add(kind, .session(session.id), "Ad hoc session \(session.label)")
    }
    if planningAgent == .waiting { add(.waiting, .workshop, "Workshop") }

    // A stable sort: plan order kept within each kind.
    return items.enumerated()
        .sorted { ($0.element.kind, $0.offset) < ($1.element.kind, $1.offset) }
        .map(\.element)
}

/// The one kind a slice is counted under, or nil where it waits on nobody.
private func attentionKind(_ slice: Slice, agent: AgentActivity?, marks: PRMarks) -> AttentionKind? {
    if agent == .waiting { return .waiting }
    switch stage(for: slice, agent: nil) {
    case .review:
        return .review
    case .pr:
        // A live agent on the slice has the pull request's trouble already —
        // nat sent it the failure, the conflict notice names it — so none of
        // it is the user's.
        guard agent == nil else { break }
        let gated = prMarks(marks, for: slice, agent: agent)
        if gated.failingChecks != nil { return .checksFailed }
        if gated.conflict != nil { return .conflict }
        if gated.checksPassing { return .readyToMerge }
    case .todo, .working, .done:
        break
    }
    return nil
}

/// A project's attention: how many things wait on the user — exactly
/// `attentionItems`' count, so the pill and the dock can never disagree —
/// and which state its dot takes.
public func projectAttention(
    slices: [Slice],
    liveAgents: [String: AgentActivity],
    planningAgent: AgentActivity? = nil,
    prReading: PRReading = .empty,
    sessions: [Session] = []
) -> ProjectAttention {
    let items = attentionItems(
        slices: slices, liveAgents: liveAgents, planningAgent: planningAgent,
        prReading: prReading, sessions: sessions)

    let inFlight = inFlightSliceIDs(slices: slices)
    let sessionTags = Set(sessions.map(\.tag))
    let anyAgent = liveAgents.keys.contains { inFlight.contains($0) || sessionTags.contains($0) }

    let role: ProjectAttentionRole
    if items.contains(where: { $0.kind == .waiting }) {
        role = .waiting
    } else if !items.isEmpty {
        role = .review
    } else if anyAgent || planningAgent != nil {
        role = .working
    } else {
        role = .idle
    }
    return ProjectAttention(count: items.count, role: role)
}

/// What changed between two readings of everything waiting on the user.
public enum AttentionChange {
    /// The items in `new` that were not in `old`, by identity — so a reading
    /// that only reorders arrives nothing, and one item leaving as another
    /// comes still arrives the newcomer.
    public static func arrivals(from old: [AttentionItem], to new: [AttentionItem]) -> [AttentionItem] {
        let seen = Set(old.map(\.identity))
        return new.filter { !seen.contains($0.identity) }
    }
}

/// One of the dock menu's groups: its disabled heading, then one row per item.
public struct DockMenuSection: Equatable, Sendable {
    public let heading: String
    public let rows: [DockMenuRow]
}

/// One of the dock menu's items: its name, cut to `dockMenuNameLimit`, the
/// project's tag, and what choosing it selects.
public struct DockMenuRow: Equatable, Sendable {
    public let name: String
    /// The project's tag as the Active rows carry it — empty where it has
    /// none.
    public let tag: String
    public let item: AttentionItem

    /// The row's title: "<tag> · <name>", the name alone with no tag.
    public var title: String { tag.isEmpty ? name : "\(tag) · \(name)" }
}

/// The longest a dock menu row's name runs before it is cut with an
/// ellipsis — a slice's name can be a sentence, and the menu is as wide as
/// its widest row.
public let dockMenuNameLimit = 48

/// `name` cut to `limit` characters, ending in "…" where anything was cut.
func truncated(_ name: String, to limit: Int) -> String {
    guard name.count > limit else { return name }
    let cut = name.prefix(limit - 1).trimmingCharacters(in: .whitespaces)
    return cut + "…"
}

/// The dock menu's groups, by kind in order of urgency, each row its item's
/// name (cut to `dockMenuNameLimit`) and its project's tag as the Active rows
/// carry it (`tags`, by project id). No section for a kind nothing is
/// waiting on.
public func dockMenuSections(_ items: [AttentionItem], tags: [String: String]) -> [DockMenuSection] {
    AttentionKind.allCases.compactMap { kind in
        let rows = items.filter { $0.kind == kind }.map { item in
            DockMenuRow(
                name: truncated(item.name, to: dockMenuNameLimit), tag: tags[item.projectID] ?? "", item: item)
        }
        return rows.isEmpty ? nil : DockMenuSection(heading: kind.heading, rows: rows)
    }
}
