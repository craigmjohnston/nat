import Foundation

/// The navigator's stacked foldouts, in the order the design stacks them —
/// Visual changes, which the design does not draw, between Changes and PR.
/// The brief is no section of its own: it is the Thread's first item, what
/// the slice's story starts from.
public enum NavigatorSection: String, CaseIterable, Equatable, Hashable, Sendable {
    case thread, changes, visuals, pr

    public var label: String {
        switch self {
        case .thread: return "Task"
        case .changes: return "Changes"
        case .visuals: return "Visual changes"
        case .pr: return "PR"
        }
    }
}

/// What the main pane shows: the agent's terminal, the diff, the images the
/// agent handed in, the pull request's description and conversation, or
/// nothing.
public enum MainPaneMode: Equatable, Sendable {
    case terminal
    case diff
    case visuals
    case pr
    /// "The terminal opens here on launch."
    case empty
}

/// What the navigator has open and what the main pane shows, together — the
/// two a header click changes at once.
public struct NavigatorFocus: Equatable, Sendable {
    public var open: Set<NavigatorSection>
    public var main: MainPaneMode

    public init(open: Set<NavigatorSection>, main: MainPaneMode) {
        self.open = open
        self.main = main
    }

    /// The chevron: fold or unfold the section, and nothing else — the main
    /// pane stays on whatever it was showing.
    public func togglingFold(_ section: NavigatorSection) -> NavigatorFocus {
        var next = self
        if open.contains(section) { next.open.remove(section) } else { next.open.insert(section) }
        return next
    }

    /// The rest of the header: a section with a main-pane view of its own
    /// (`shows`) is opened and its view put up — or, when it is already open
    /// and already up, folded, so a second click undoes the first. A section
    /// with none just folds or unfolds, as its chevron would.
    public func clickingHead(_ section: NavigatorSection, shows: MainPaneMode?) -> NavigatorFocus {
        guard let shows, !(open.contains(section) && main == shows) else { return togglingFold(section) }
        var next = self
        next.open.insert(section)
        next.main = shows
        return next
    }

    /// A main-pane tab (or a View menu item): open the section and put its
    /// view up, never folding one whose view is already up. A section with
    /// no view of its own changes nothing.
    public func showing(_ section: NavigatorSection, shows: MainPaneMode?) -> NavigatorFocus {
        guard let shows else { return self }
        var next = self
        next.open.insert(section)
        next.main = shows
        return next
    }
}

/// The main pane's tabs, in its titlebar: each stands for a navigator
/// section and the view that section's header puts up.
public enum MainPaneTab: CaseIterable, Equatable, Sendable {
    case terminal, changes, visuals, pr

    public var label: String {
        switch self {
        case .terminal: return "Terminal"
        case .changes: return "Changes"
        case .visuals: return "Visual changes"
        case .pr: return "PR"
        }
    }

    /// The section the tab opens.
    public var section: NavigatorSection {
        switch self {
        case .terminal: return .thread
        case .changes: return .changes
        case .visuals: return .visuals
        case .pr: return .pr
        }
    }

    /// The main-pane view the tab puts up.
    public var mode: MainPaneMode {
        switch self {
        case .terminal: return .terminal
        case .changes: return .diff
        case .visuals: return .visuals
        case .pr: return .pr
        }
    }

    /// A session's tabs: every section of a session has a view of its own,
    /// PR only once the session has opened one.
    public static func forSession(hasPRs: Bool) -> [MainPaneTab] {
        hasPRs ? [.terminal, .changes, .pr] : [.terminal, .changes]
    }
}

/// The navigator's reading of one slice: which sections are live, which open
/// first, which header actions it offers, and what the main pane lands on.
/// The design's `phaseOf`/`launched`/`handed`/`hasPR`, over the slice's real
/// facts — and, where the design assumed a fact nat does not have, over the
/// fact nat does: Changes reads a branch, and only a recorded one can be read
/// (`nat slice-diff` refuses a slice with none), so it is live once a branch
/// is recorded rather than the moment an agent launches.
public struct NavigatorModel: Equatable, Sendable {
    public let state: SliceDisplayState
    public let hasPR: Bool
    public let hasBranch: Bool
    public let hasLiveAgent: Bool
    /// The Thread header's Launch: `LaunchPlan`'s own answer, so the header,
    /// the slice menu and the CLI never disagree.
    public let canLaunch: Bool
    /// Whether the slice's agent has handed in any images — the Visual
    /// changes section exists only then.
    public let hasVisuals: Bool

    public init(slice: Slice, agent: AgentActivity?, fixLaunched: Bool, hasVisuals: Bool = false) {
        self.state = displayState(for: slice, agent: agent, fixLaunched: fixLaunched)
        self.hasPR = !slice.pr.isEmpty
        self.hasBranch = slice.handedBack || !(slice.branch ?? "").isEmpty
        self.hasLiveAgent = agent != nil
        self.canLaunch = LaunchPlan(for: slice, hasLiveAgent: agent != nil).canLaunch
        self.hasVisuals = hasVisuals
    }

    /// Where the slice stands, as the section that should be open first.
    public var phase: NavigatorSection {
        switch state {
        case .todo, .blocked, .working, .waiting, .fixing: return .thread
        case .review: return .changes
        case .pr: return .pr
        // With no pull request, the Thread is where a finished slice says
        // how it ended — merged, or closed with the agent's summary.
        case .done: return hasPR ? .pr : .thread
        }
    }

    public var defaultOpen: Set<NavigatorSection> { [phase] }

    /// Whether a section's header can be opened at all. The Thread always
    /// can: it opens on the brief.
    public func isLive(_ section: NavigatorSection) -> Bool {
        switch section {
        case .thread: return true
        case .changes: return hasBranch
        case .visuals: return hasVisuals
        case .pr: return hasPR
        }
    }

    /// The main-pane view a section's header puts up, when it has one.
    public func mainMode(for section: NavigatorSection) -> MainPaneMode? {
        switch section {
        case .thread: return agentAvailable ? .terminal : nil
        case .changes: return diffAvailable ? .diff : nil
        case .visuals: return hasVisuals ? .visuals : nil
        case .pr: return hasPR ? .pr : nil
        }
    }

    /// The design's main-pane default: the terminal while the Thread is the
    /// phase of a launched slice, the pull request while that is, the diff
    /// once anything has been handed back, else the note.
    public var defaultMain: MainPaneMode {
        if phase == .thread && agentAvailable && state != .done { return .terminal }
        if phase == .pr { return .pr }
        return hasBranch ? .diff : .empty
    }

    /// The main pane's tabs: one per section the navigator draws whose
    /// header would put its view up — so no Terminal before an agent.
    public var tabs: [MainPaneTab] {
        MainPaneTab.allCases.filter { isLive($0.section) && mainMode(for: $0.section) == $0.mode }
    }

    /// Whether the Agent half of the switch can be picked.
    public var agentAvailable: Bool { state.isLaunched || hasLiveAgent }

    /// Whether the Diff half can.
    public var diffAvailable: Bool { hasBranch }

    /// Whether Launch is the primary action — a Todo slice — rather than a
    /// relaunch or a fix session offered on one already under way.
    public var launchIsPrimary: Bool { state == .todo }

    /// Whether the Thread header offers Launch: a slice not yet launched (a
    /// blocked one drawn disabled, as the design draws it), and one being
    /// worked whose agent is gone — a relaunch. A slice handed back, in
    /// review or done carries no Launch here, as in the design; the slice's
    /// menu still offers whatever `LaunchPlan` allows.
    public var showsLaunch: Bool {
        guard !hasLiveAgent else { return false }
        switch state {
        case .todo, .blocked, .working, .fixing: return true
        case .waiting, .review, .pr, .done: return false
        }
    }

    /// Whether Changes carries Send and Approve: only a hand-back awaiting
    /// review has anything to approve.
    public var showsReviewActions: Bool { state == .review }

    /// Whether Visual changes carries Send: comments go to the agent, so
    /// only while there is one to receive them.
    public var showsVisualActions: Bool { hasVisuals && hasLiveAgent }

    /// Whether the PR header carries Merge: an open pull request on a slice
    /// not yet Done.
    public var showsMerge: Bool { hasPR && state != .done }
}

/// How a Thread card's meta line is toned.
public enum ThreadTone: Equatable, Sendable {
    case muted, accent, hot
}

/// What a Thread card records — what its icon is drawn from.
public enum ThreadEventKind: Equatable, Sendable {
    /// An agent launched on the slice, or a session started.
    case launched
    /// The live agent, working or waiting — or a session's, ended.
    case agent
    case handedBack
    case approved
    case merged
    /// Closed straight to Done with no branch.
    case closed
}

/// One labelled value in a Thread card's foot — `model opus`, `branch
/// slice/x` — drawn as the brief's own facts are.
public struct ThreadFact: Equatable, Sendable {
    public let key: String
    public let value: String

    public init(_ key: String, _ value: String) {
        self.key = key
        self.value = value
    }
}

/// One card of the Thread log.
public struct ThreadEvent: Equatable, Sendable {
    public let kind: ThreadEventKind
    public let who: String
    public let meta: String?
    public let tone: ThreadTone
    public let body: String?
    public let facts: [ThreadFact]

    public init(
        _ kind: ThreadEventKind, who: String, meta: String? = nil, tone: ThreadTone = .muted,
        body: String? = nil, facts: [ThreadFact] = []
    ) {
        self.kind = kind
        self.who = who
        self.meta = meta
        self.tone = tone
        self.body = body
        self.facts = facts
    }
}

/// A live agent's own statusline reading as facts: its model, its effort
/// and how much of its context it has used, each only once read.
public func agentFacts(_ agent: AgentStatus?) -> (model: [ThreadFact], context: [ThreadFact]) {
    guard let agent else { return ([], []) }
    var model: [ThreadFact] = []
    if let name = agent.model, !name.isEmpty { model.append(ThreadFact("model", name)) }
    if let effort = agent.effort, !effort.isEmpty { model.append(ThreadFact("effort", effort)) }
    let context = agent.contextPercent.map { [ThreadFact("context", "\(Int($0.rounded()))%")] } ?? []
    return (model, context)
}

/// The Thread log, built only from what nat reports: the live agent's own
/// statusline reading (model, effort, context), the slice's recorded branch,
/// the last hand-back note on its page, its pull request and its status. The
/// design's launch time, token count, files touched, current tool and
/// merged-by have no source in nat yet, and are left out rather than made up.
public func buildThreadEvents(slice: Slice, agent: AgentStatus?, brief: String?) -> [ThreadEvent] {
    let state = displayState(
        for: slice, agent: agent.map { AgentActivity($0.activity) }, fixLaunched: false)
    guard state.isLaunched || agent != nil else { return [] }

    var events: [ThreadEvent] = []
    let branch = (slice.branch ?? "").isEmpty ? nil : slice.branch
    let reading = agentFacts(agent)
    events.append(ThreadEvent(
        .launched, who: "Launched", facts: reading.model + (branch.map { [ThreadFact("branch", $0)] } ?? [])))

    if let agent {
        let waiting = AgentActivity(agent.activity) == .waiting
        events.append(ThreadEvent(
            .agent, who: "Agent",
            meta: waiting ? "waiting for you" : "working",
            tone: waiting ? .hot : .accent,
            facts: reading.context))
    }

    if slice.handedBack || !slice.pr.isEmpty || (state == .done && branch != nil) {
        events.append(ThreadEvent(
            .handedBack, who: "Agent", meta: "handed back",
            body: brief.flatMap(handBackNote)))
    }

    if let number = pullRequestNumber(slice.pr) {
        events.append(ThreadEvent(
            .approved, who: "You", meta: "approved", facts: [ThreadFact("pr", "#\(number)"), ThreadFact("into", "main")]))
    } else if !slice.pr.isEmpty {
        events.append(ThreadEvent(.approved, who: "You", meta: "approved", facts: [ThreadFact("pr", slice.pr)]))
    }

    if state == .done {
        if branch == nil && slice.pr.isEmpty {
            // Closed straight to Done with no branch — work that was never
            // code — so nothing was merged: the card says it closed, with
            // the summary the agent filed in place of a hand-back note.
            events.append(ThreadEvent(.closed, who: "Closed", body: brief.flatMap(summaryNote)))
        } else {
            events.append(ThreadEvent(.merged, who: "Merged"))
        }
    }
    return events
}

/// The pull request's number off its URL — `…/pull/40` reads 40.
public func pullRequestNumber(_ url: String) -> Int? {
    guard let range = url.range(of: "/pull/") else { return nil }
    let digits = url[range.upperBound...].prefix { $0.isNumber }
    return Int(digits)
}

/// The heading a hand-back note is filed under on the slice's page — the
/// store's own `handedBackHeading`.
public let handedBackHeading = "Handed back"

/// The heading a slice closed straight to Done files its note under.
public let summaryHeading = "Summary"

/// The last `Summary` section of a slice's page body — see `handBackNote`.
public func summaryNote(_ body: String) -> String? {
    lastSection(named: summaryHeading, in: body)
}

/// The last `Handed back` section of a slice's page body, as markdown: the
/// text under the heading up to the next heading of the same or a higher
/// level. A slice handed back twice has one per hand-back, and the last is
/// the one that describes the branch as it stands. Nil with none.
public func handBackNote(_ body: String) -> String? {
    lastSection(named: handedBackHeading, in: body)
}

/// The last section under a heading of this name: the text up to the next
/// heading of the same or a higher level.
func lastSection(named heading: String, in body: String) -> String? {
    var note: [Substring]?
    var level = 0
    var last: [Substring]?
    for line in body.split(separator: "\n", omittingEmptySubsequences: false) {
        let hashes = line.prefix { $0 == "#" }.count
        let isHeading = hashes > 0 && line.dropFirst(hashes).first == " "
        if isHeading {
            if note != nil && hashes <= level {
                last = note
                note = nil
            }
            let title = line.dropFirst(hashes).trimmingCharacters(in: .whitespaces)
            if title.caseInsensitiveCompare(heading) == .orderedSame {
                note = []
                level = hashes
                continue
            }
        }
        note?.append(line)
    }
    let text = (note ?? last)?.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
    return (text?.isEmpty ?? true) ? nil : text
}
