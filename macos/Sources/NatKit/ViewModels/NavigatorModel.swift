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
/// is recorded rather than the moment an agent is launched.
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

    public init(slice: Slice, agent: AgentActivity?, hasVisuals: Bool = false) {
        self.state = displayState(for: slice, agent: agent)
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

    /// Whether a launch would be a fix session: the slice sits at its pull
    /// request, approved, with no fix under way — "Launch fix agent".
    public var launchIsFix: Bool { state == .pr }

    /// The first section's label: "Task", whatever the slice's state — the
    /// one name the user knows it by, before launch and after.
    public var threadLabel: String { NavigatorSection.thread.label }

    /// Whether the Thread header offers Launch: a slice not yet launched (a
    /// blocked one drawn disabled, as the design draws it), one being worked
    /// or fixed whose agent is gone — a relaunch — and one approved and at
    /// its pull request, where it is a fix launch. A slice handed back, in
    /// review or done carries no Launch here, as in the design; the slice's
    /// menu still offers whatever `LaunchPlan` allows.
    public var showsLaunch: Bool {
        guard !hasLiveAgent else { return false }
        switch state {
        case .todo, .blocked, .working, .fixing, .pr: return true
        case .waiting, .review, .done: return false
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

    /// The PR header's status: Merged once the slice is Done with a pull
    /// request — Done is written only by the merge, so nothing past the
    /// slice's own status is read for it.
    public var prStatus: NavSectionStatus? {
        state == .done && hasPR ? .merged : nil
    }
}

/// A status a navigator section's header carries beside its label.
public enum NavSectionStatus: Equatable, Sendable {
    case merged

    public var label: String {
        switch self {
        case .merged: return "Merged"
        }
    }
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
    /// Sent back to its agent with review comments (`slice-rework`).
    case sentBack
    /// Released back to Todo, its session ended unfinished.
    case released
    /// Launched again on the work so far.
    case relaunched
    /// The pull request's checks failed with no agent live to be told.
    case checksFailed
    /// Handed in as blocked.
    case blocked
    /// Follow-ups the agent proposed: the count line.
    case followUps
    /// One proposed follow-up the user has decided, after its proposal.
    case followUp
    /// A note left on the brief, from another slice or a person.
    case note
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
    /// The slice the value names, where it names one on the plan — a note's
    /// `task` — which the card draws as the brief's own depends-on row
    /// (state dot, name, hover detail, a click selecting it) rather than as
    /// plain text. Nil for every other fact.
    public let sliceID: String?

    public init(_ key: String, _ value: String, sliceID: String? = nil) {
        self.key = key
        self.value = value
        self.sliceID = sliceID
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
    /// A follow-ups card whose proposal still awaits the user's decision —
    /// drawn as the triage card (Queue / Fold in / Drop, Apply) in its place
    /// in the log rather than as a record.
    public let awaitsTriage: Bool
    /// Something happening now (the live agent, working or waiting) or
    /// waiting on the user's decision (a proposal awaiting triage): drawn
    /// apart from the settled cards around it.
    public let isLive: Bool
    /// Whether the meta says what `who` did ("handed back", "left a note"),
    /// so the card reads the two as one line, `title`; false where it is
    /// a separate fact about the card, such as a comment's time.
    public let metaIsAction: Bool
    /// When it happened, where nat knows — drawn at the header's trailing
    /// end by `threadTimestamp`. Nil for a card nat records no time for. A
    /// `var` so a recorded event's card takes its event's time in one place.
    public var when: Date?

    public init(
        _ kind: ThreadEventKind, who: String, meta: String? = nil, tone: ThreadTone = .muted,
        body: String? = nil, facts: [ThreadFact] = [], awaitsTriage: Bool = false, isLive: Bool = false,
        metaIsAction: Bool = true, when: Date? = nil
    ) {
        self.kind = kind
        self.who = who
        self.meta = meta
        self.tone = tone
        self.body = body
        self.facts = facts
        self.awaitsTriage = awaitsTriage
        self.isLive = isLive
        self.metaIsAction = metaIsAction
        self.when = when
    }

    /// The card's first line where the meta is an action: who, then what
    /// they did, as one sentence — "Agent handed back".
    public var title: String {
        guard metaIsAction, let meta else { return who }
        return "\(who) \(meta)"
    }
}

/// When a Thread card happened, as its header says it: the time alone where
/// it was today, the day and month where it was earlier this year, and the
/// year too before that — each in the locale's own order and clock, from the
/// templates `jm` (23:14, or 11:14 PM), `d MMM` (3 Oct) and `d MMM y`
/// (3 Oct 2025). "Today" and "this year" are `calendar`'s, in its time zone,
/// so a test holds all three still.
public func threadTimestamp(
    _ date: Date, now: Date = Date(), calendar: Calendar = .current, locale: Locale = .current
) -> String {
    let template: String
    if calendar.isDate(date, inSameDayAs: now) {
        template = "jm"
    } else if calendar.component(.year, from: date) == calendar.component(.year, from: now) {
        template = "d MMM"
    } else {
        template = "d MMM y"
    }
    let formatter = DateFormatter()
    formatter.calendar = calendar
    formatter.timeZone = calendar.timeZone
    formatter.locale = locale
    formatter.setLocalizedDateFormatFromTemplate(template)
    return formatter.string(from: date)
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

/// The Task log, built only from what nat reports: the live agent's own
/// statusline reading (model, effort, context), the slice's recorded branch,
/// and `events` — `slice-show`'s ordered record of every hand-back, send-back,
/// release, relaunch, note and proposal of follow-ups on the slice's page, then
/// its approve and merge. The design's launch time, token count, files
/// touched, current tool and merged-by have no source in nat yet, and are
/// left out rather than made up.
///
/// With no `events` — the slice's detail not read yet, or a nat too old to
/// report them — the log falls back to what the slice's properties and its
/// last hand-back note alone say.
///
/// `plan` and `milestones` are the project's own, as loaded: what a note's
/// `fromSlice` is matched against to name the slice it came from as a task
/// on the plan (see `noteSourceSlice`). Without them every note's source is
/// plain text.
public func buildThreadEvents(
    slice: Slice, agent: AgentStatus?, brief: String?, events: [TaskLogEvent]? = nil,
    plan: [Slice] = [], milestones: [Milestone] = []
) -> [ThreadEvent] {
    let state = displayState(
        for: slice, agent: agent.map { AgentActivity($0.activity) })
    // A released slice is back to do, and its history is still its own. Notes
    // alone are not history: one left on a slice never launched is read in its
    // brief, and opens no log of launches that never happened.
    let history = (events ?? []).contains { $0.kind != .note }
    guard state.isLaunched || agent != nil || history else { return [] }

    let branch = (slice.branch ?? "").isEmpty ? nil : slice.branch
    let reading = agentFacts(agent)
    var log = [ThreadEvent(
        .launched, who: "Launched", facts: reading.model + (branch.map { [ThreadFact("branch", $0)] } ?? []))]
    let agentCard = agent.map { agent in
        let waiting = AgentActivity(agent.activity) == .waiting
        return ThreadEvent(
            .agent, who: "Agent",
            meta: waiting ? "on standby" : "working",
            tone: waiting ? .hot : .accent,
            facts: reading.context, isLive: true)
    }

    guard let events else {
        return log + legacyThreadEvents(slice: slice, state: state, branch: branch, agentCard: agentCard, brief: brief)
    }
    // What the page records, in the order it was written; then the agent as
    // it is now; then what the properties say came of it all.
    let closing: Set<TaskLogEvent.Kind> = [.approved, .merged]
    // Each card takes its event's time, but for one that has its own — a
    // decided follow-up's, the time it was decided.
    let cards = { (event: TaskLogEvent) -> [ThreadEvent] in
        threadEvents(event, plan: plan, milestones: milestones).map { card in
            var card = card
            if card.when == nil { card.when = event.at }
            return card
        }
    }
    log += events.filter { !closing.contains($0.kind) }.flatMap(cards)
    if let agentCard { log.append(agentCard) }
    log += events.filter { closing.contains($0.kind) }.flatMap(cards)
    return log
}

/// The one slice on the plan a note's source names: the same name, under a
/// milestone of the same name — a milestone the plan does not list read by
/// its id, as the brief's own facts read one — or, where the source names no
/// milestone, by name alone. Nil where none matches, or where more than one
/// does — a guess at which would be a wrong link as often as a right one.
public func noteSourceSlice(_ source: NoteSource, plan: [Slice], milestones: [Milestone]) -> Slice? {
    let names = Dictionary(milestones.map { ($0.id, $0.name) }, uniquingKeysWith: { first, _ in first })
    let matches = plan.filter { candidate in
        candidate.name == source.name
            && (source.milestone.isEmpty || (names[candidate.milestoneID] ?? candidate.milestoneID) == source.milestone)
    }
    return matches.count == 1 ? matches[0] : nil
}

/// The one slice on the plan a queued follow-up's `link` names — its URL,
/// or its ID (dashes and case aside), or a URL ending in that ID as a
/// Notion page URL does. Nil where none does.
public func followUpSlice(link: String?, plan: [Slice]) -> Slice? {
    guard let link, !link.isEmpty else { return nil }
    let compact = link.replacingOccurrences(of: "-", with: "").lowercased()
    return plan.first { candidate in
        let id = candidate.id.replacingOccurrences(of: "-", with: "").lowercased()
        return (!candidate.url.isEmpty && candidate.url == link) || (!id.isEmpty && compact.hasSuffix(id))
    }
}

/// One recorded event as its Task log cards: one card, except a proposal of
/// follow-ups, which is its count line then one card per decided follow-up —
/// headed by the decision ("Queued proposed follow-up"), its title then its
/// brief as the body, a `task` row for the slice a queued one became where
/// the plan holds it, and the time it was decided where nat read one.
private func threadEvents(_ event: TaskLogEvent, plan: [Slice], milestones: [Milestone]) -> [ThreadEvent] {
    [threadEvent(event, plan: plan, milestones: milestones)] + event.followUps.compactMap { followUp in
        followUp.decision.map { decision in
            let queued = decision == .queued ? followUpSlice(link: followUp.link, plan: plan) : nil
            // Two newlines, so markdown sets the title and brief as paragraphs.
            let body = followUp.brief.isEmpty ? followUp.title : "\(followUp.title)\n\n\(followUp.brief)"
            return ThreadEvent(
                .followUp, who: followUpDecisionHeading(decision), meta: "proposed follow-up",
                body: body,
                facts: queued.map { [ThreadFact("task", $0.name, sliceID: $0.id)] } ?? [],
                when: followUp.decidedAt)
        }
    }
}

/// One recorded event as its Task log card — a proposal of follow-ups as
/// its count line.
private func threadEvent(_ event: TaskLogEvent, plan: [Slice], milestones: [Milestone]) -> ThreadEvent {
    let note = event.note.flatMap { $0.isEmpty ? nil : $0 }
    switch event.kind {
    case .handedBack:
        return ThreadEvent(.handedBack, who: "Agent", meta: "handed back", body: note)
    case .sentBack:
        // One nat filed for a red pull request names where it came from in
        // `by` (CI), where a review's own comments name nobody: they are yours.
        if event.by.map({ !$0.isEmpty }) ?? false {
            // The sending is a line of the body, under the note as recorded.
            let sent = "Sent to the agent to fix."
            return ThreadEvent(
                .sentBack, who: "Checks failed", tone: .accent, body: note.map { "\($0)\n\n\(sent)" } ?? sent)
        }
        return ThreadEvent(.sentBack, who: "You", meta: "sent back with comments", tone: .accent, body: note)
    case .released:
        guard let by = event.by.flatMap({ $0.isEmpty ? nil : $0 }) else {
            return ThreadEvent(.released, who: "Released to Todo")
        }
        return ThreadEvent(.released, who: by, meta: "released to Todo")
    case .relaunched:
        return ThreadEvent(.relaunched, who: "Relaunched on the work so far")
    case .checksFailed:
        return ThreadEvent(.checksFailed, who: "Checks failed", tone: .hot, body: note)
    case .blocked:
        return ThreadEvent(.blocked, who: "Agent", meta: "blocked", tone: .hot, body: note)
    case .summary:
        return ThreadEvent(.closed, who: "Closed", body: note)
    case .followUps:
        let count = event.followUps.count
        let pending = event.followUps.contains { $0.decision == nil }
        return ThreadEvent(
            .followUps, who: "Agent", meta: "proposed \(count) follow-up\(count == 1 ? "" : "s")",
            tone: pending ? .hot : .muted, awaitsTriage: pending, isLive: pending)
    case .note:
        guard let by = event.by.flatMap({ $0.isEmpty ? nil : $0 }) else {
            return ThreadEvent(.note, who: "Note", body: note)
        }
        // A note from a task on the plan names it as a task row; from
        // anywhere else — a person, a workshop, a task since renamed or
        // removed — its provenance is the source, as nat wrote it.
        let source = event.fromSlice.flatMap { noteSourceSlice($0, plan: plan, milestones: milestones) }
        let fact = source.map { ThreadFact("task", $0.name, sliceID: $0.id) } ?? ThreadFact("source", by)
        return ThreadEvent(.note, who: "Another agent", meta: "left a note", body: note, facts: [fact])
    case .approved:
        let pr = event.pr ?? ""
        if let number = pullRequestNumber(pr) {
            return ThreadEvent(
                .approved, who: "You", meta: "approved",
                facts: [ThreadFact("pr", "#\(number)"), ThreadFact("into", "main")])
        }
        return ThreadEvent(.approved, who: "You", meta: "approved", facts: pr.isEmpty ? [] : [ThreadFact("pr", pr)])
    case .merged:
        return ThreadEvent(.merged, who: "Merged")
    }
}

/// Every key a Thread card's facts can carry — the brief's own, the launch
/// card's and each recorded card's — so the cards can give their key column
/// one width, that of the widest (`widestThreadFactKey`), and every value
/// starts at the same x. A source's own fact labels are its, and not listed.
public let threadFactKeys = [
    "milestone", "depends on", "model", "effort", "base", "branch", "context", "pr", "into", "task", "source",
]

/// The longest of `threadFactKeys` — the keys are set in mono, so the
/// longest is the widest.
public let widestThreadFactKey = threadFactKeys.max { $0.count < $1.count } ?? ""

/// What a triaged follow-up's log item says came of it, heading the item
/// as "<word> proposed follow-up" — a dropped one reads as dismissed.
public func followUpDecisionHeading(_ decision: TaskFollowUp.Decision) -> String {
    switch decision {
    case .queued: return "Queued"
    case .folded: return "Folded in"
    case .dropped: return "Dismissed"
    }
}

/// The log as the slice's properties alone tell it, for a reading with no
/// recorded events: one hand-back card carrying the last note, then the
/// approve and the ending.
private func legacyThreadEvents(
    slice: Slice, state: SliceDisplayState, branch: String?, agentCard: ThreadEvent?, brief: String?
) -> [ThreadEvent] {
    var events: [ThreadEvent] = []
    if let agentCard { events.append(agentCard) }

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
