import Foundation

/// The one word the gnat design draws a slice by — its sidebar dot, its
/// Active membership and which navigator section opens first. Read off the
/// slice's `WorkflowStage` (Notion's status, never re-derived) with the live
/// agent refining only what a stage leaves open: whether a slice being worked
/// is waiting on the user.
public enum SliceDisplayState: String, CaseIterable, Equatable, Sendable {
    case todo, working, waiting, review, pr, blocked, done

    /// The design's `needsYou`: an agent waiting, a branch to review, a pull
    /// request open. What the sidebar sorts Active by and counts in hot.
    public var needsYou: Bool {
        switch self {
        case .waiting, .review, .pr: return true
        case .todo, .working, .blocked, .done: return false
        }
    }

    /// The design's `launched`: anything that has had an agent on it.
    public var isLaunched: Bool {
        self != .todo && self != .blocked
    }

    /// The state in words, for where a dot alone says too little — a
    /// dependency's hover detail.
    public var word: String {
        switch self {
        case .todo: return "To do"
        case .working: return "Working"
        case .waiting: return "Waiting for you"
        case .review: return "In review"
        case .pr: return "PR open"
        case .blocked: return "Blocked"
        case .done: return "Done"
        }
    }

    /// What the Active fold lists — `inFlightSliceIDs`'s own rule, so the
    /// fold and the reaper's sweep can never disagree.
    public var isInFlight: Bool {
        switch self {
        case .working, .waiting, .review, .pr: return true
        case .todo, .blocked, .done: return false
        }
    }
}

/// A slice's display state. `agent` is the live map's reading, which turns a
/// working slice (a resumed one too) into a waiting one and nothing else: a session
/// outlives hand-back and approve, so it never moves a slice's stage.
public func displayState(for slice: Slice, agent: AgentActivity?) -> SliceDisplayState {
    switch stage(for: slice, agent: agent) {
    case .todo: return slice.blocked ? .blocked : .todo
    case .working: return agent == .waiting ? .waiting : .working
    case .review: return .review
    case .pr: return .pr
    case .done: return .done
    }
}

/// One slice row of the Projects tree.
public struct SidebarSliceRow: Equatable, Identifiable, Sendable {
    public let sliceID: String
    public let projectID: String
    public let title: String
    public let state: SliceDisplayState
    /// Whether an agent is live on it — what makes a working dot pulse.
    public let live: Bool
    /// Its pull request's failing checks and conflict, where it reads either.
    public let marks: PRMarks

    public var id: String { sliceID }

    public init(
        sliceID: String, projectID: String, title: String, state: SliceDisplayState, live: Bool,
        marks: PRMarks = .none
    ) {
        self.sliceID = sliceID
        self.projectID = projectID
        self.title = title
        self.state = state
        self.live = live
        self.marks = marks
    }
}

/// A milestone row and the slices filed under it, in plan order.
public struct SidebarMilestone: Equatable, Identifiable, Sendable {
    public let name: String
    public let done: Int
    public let total: Int
    public let slices: [SidebarSliceRow]

    public var id: String { name }

    /// Every slice filed under it done — and at least one, since an empty
    /// milestone is a plan still to be filled, not a finished one.
    public var isComplete: Bool { total > 0 && done == total }

    /// Whether its folder starts open, where the user has not folded it:
    /// partly done, or holding the selected slice. One with nothing done
    /// starts folded — its work in progress is already on show in Active.
    public func opensByDefault(selecting sliceID: String?) -> Bool {
        (done > 0 && done < total) || sliceID.map { id in slices.contains { $0.sliceID == id } } ?? false
    }

    public init(name: String, done: Int, total: Int, slices: [SidebarSliceRow]) {
        self.name = name
        self.done = done
        self.total = total
        self.slices = slices
    }
}

/// What kind of project row a project is.
public enum SidebarProjectKind: Equatable, Sendable {
    case project
    /// The reserved scratch project ad hoc sessions without a project run in.
    case scratch
    /// A project not yet made: the starter card's row.
    case untitled
}

/// Where a project's plan read has got to, for the row's own note.
public enum SidebarPlanStatus: Equatable, Sendable {
    case loading
    case failed(String)
    /// A plan on screen whose last refresh failed — kept, and said so.
    case stale(String)
    case loaded
    case empty
    /// An Untitled row has no plan to read.
    case none
}

/// One project of the Projects fold.
public struct SidebarProject: Equatable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let kind: SidebarProjectKind
    public let status: SidebarPlanStatus
    /// The milestones still holding work, in plan order — a finished slice
    /// stays under its milestone here until the whole milestone is done.
    public let milestones: [SidebarMilestone]
    /// The milestones every slice of which is done, in plan order: what the
    /// project's Done folder holds, drawn once there is at least one.
    public let doneMilestones: [SidebarMilestone]
    /// How many of its rows need the user — the collapsed row's activity pip, on its folder's shoulder.
    public let needsYou: Int
    /// The slices of the scratch project's unfiled milestone (`Milestone.unfiled`):
    /// drawn loose at the head of the tree, above every milestone, with no
    /// folder of their own. Empty for every other project.
    public let loose: [SidebarSliceRow]
    /// A source project's fold — its plugin and tree; nil for every other
    /// project, and for a source project whose plan has not landed yet.
    public let source: SidebarSource?
    /// The project's colour — its badge's; nil for one nat has not coloured
    /// yet and for an Untitled row, which has no config entry.
    public let color: ProjectColor?
    /// The project's short tag — its badge's word (`sidebarTags`); empty for
    /// an Untitled row, which draws no badge.
    public let tag: String

    public init(
        id: String, name: String, kind: SidebarProjectKind, status: SidebarPlanStatus,
        milestones: [SidebarMilestone], doneMilestones: [SidebarMilestone] = [], needsYou: Int,
        loose: [SidebarSliceRow] = [], source: SidebarSource? = nil, color: ProjectColor? = nil, tag: String = ""
    ) {
        self.id = id
        self.name = name
        self.kind = kind
        self.status = status
        self.milestones = milestones
        self.doneMilestones = doneMilestones
        self.needsYou = needsYou
        self.loose = loose
        self.source = source
        self.color = kind == .untitled ? nil : color
        self.tag = kind == .untitled ? "" : tag
    }

    /// Whether the project files a slice, for the default-open rule.
    public func contains(sliceID: String) -> Bool {
        loose.contains { $0.sliceID == sliceID }
            || (milestones + doneMilestones).contains { $0.slices.contains { $0.sliceID == sliceID } }
    }

    /// Whether the Done folder holds a slice — what opens it by default.
    public func doneContains(sliceID: String) -> Bool {
        doneMilestones.contains { $0.slices.contains { $0.sliceID == sliceID } }
    }

    /// The project as View ▸ Hide Done Items draws it: no Done folder, and
    /// no done slice under a milestone still holding work. A milestone keeps
    /// its own count — it is still that far through. A source project's
    /// containers drop their done tasks the same way.
    public func hidingDone() -> SidebarProject {
        SidebarProject(
            id: id, name: name, kind: kind, status: status,
            milestones: milestones.map { milestone in
                SidebarMilestone(
                    name: milestone.name, done: milestone.done, total: milestone.total,
                    slices: milestone.slices.filter { $0.state != .done })
            },
            doneMilestones: [], needsYou: needsYou, loose: loose.filter { $0.state != .done },
            source: source?.hidingDone(), color: color, tag: tag)
    }
}

// MARK: - Task sources

/// What a source draws itself with: an SF Symbol, and the plugin's own SVG
/// where it gave one (drawn as a template, the symbol its fallback).
public struct SourceIcon: Equatable, Sendable {
    public let symbol: String
    public let svg: String?

    /// The glyph a source that named none is drawn with.
    public static let fallbackSymbol = "puzzlepiece.extension"

    public init(symbol: String, svg: String? = nil) {
        self.symbol = symbol.isEmpty ? Self.fallbackSymbol : symbol
        self.svg = svg
    }
}

/// One container row of a source fold: the plugin's own row, with the plan's
/// tasks filed under it nested beneath.
public struct SidebarContainer: Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let externalURL: String?
    public let badges: [SourceBadge]
    /// Drawn only under the pointer.
    public let meta: String?
    public let menu: [SourceAction]
    public let tasks: [SidebarSliceRow]
    /// How many of its tasks need the user.
    public let needsYou: Int

    public init(
        id: String, title: String, externalURL: String? = nil, badges: [SourceBadge] = [], meta: String? = nil,
        menu: [SourceAction] = [], tasks: [SidebarSliceRow], needsYou: Int
    ) {
        self.id = id
        self.title = title
        self.externalURL = externalURL
        self.badges = badges
        self.meta = meta
        self.menu = menu
        self.tasks = tasks
        self.needsYou = needsYou
    }

    func hidingDone() -> SidebarContainer {
        SidebarContainer(
            id: id, title: title, externalURL: externalURL, badges: badges, meta: meta, menu: menu,
            tasks: tasks.filter { $0.state != .done }, needsYou: needsYou)
    }
}

/// One group of a source fold: a header with the plugin's count, then its
/// child groups (one level) or its containers.
public struct SidebarSourceGroup: Equatable, Identifiable, Sendable {
    public let id: String
    public let label: String
    public let count: Int?
    /// Folded until asked for, and listed only once `info --expand` names it.
    public let lazy: Bool
    public let menu: [SourceAction]
    public let children: [SidebarSourceGroup]
    public let containers: [SidebarContainer]

    public init(
        id: String, label: String, count: Int? = nil, lazy: Bool = false, menu: [SourceAction] = [],
        children: [SidebarSourceGroup] = [], containers: [SidebarContainer] = []
    ) {
        self.id = id
        self.label = label
        self.count = count
        self.lazy = lazy
        self.menu = menu
        self.children = children
        self.containers = containers
    }

    func hidingDone() -> SidebarSourceGroup {
        SidebarSourceGroup(
            id: id, label: label, count: count, lazy: lazy, menu: menu,
            children: children.map { $0.hidingDone() }, containers: containers.map { $0.hidingDone() })
    }
}

/// A source project's fold: who the plugin is, its header menu, its tree.
public struct SidebarSource: Equatable, Sendable {
    public let name: String
    /// The plugin's own title, else its name.
    public let title: String
    /// The short tag Active rows and the titlebar carry for its tasks.
    public let tag: String
    public let icon: SourceIcon
    public let containerNoun: String
    public let taskNoun: String
    public let menu: [SourceAction]
    public let groups: [SidebarSourceGroup]
    /// The plugin's failed read, drawn as the fold's note.
    public let error: String?

    public init(
        name: String, title: String, tag: String, icon: SourceIcon, containerNoun: String, taskNoun: String,
        menu: [SourceAction] = [], groups: [SidebarSourceGroup], error: String? = nil
    ) {
        self.name = name
        self.title = title
        self.tag = tag
        self.icon = icon
        self.containerNoun = containerNoun
        self.taskNoun = taskNoun
        self.menu = menu
        self.groups = groups
        self.error = error
    }

    func hidingDone() -> SidebarSource {
        SidebarSource(
            name: name, title: title, tag: tag, icon: icon, containerNoun: containerNoun, taskNoun: taskNoun,
            menu: menu, groups: groups.map { $0.hidingDone() }, error: error)
    }

    /// The container with `id`, wherever it sits in the tree.
    public func container(withID id: String) -> SidebarContainer? {
        for (_, containers) in containerGroups {
            if let found = containers.first(where: { $0.id == id }) { return found }
        }
        return nil
    }

    /// The filter action of the header's menu (`group` nil) or of a group's,
    /// as the tree has it now — what an open filter editor redraws from, so
    /// a field still loading fills in when the tree is read again.
    public func filterAction(group: String?) -> SourceAction? {
        let menu: [SourceAction]
        if let group {
            menu = (groups + groups.flatMap(\.children)).first { $0.id == group }?.menu ?? []
        } else {
            menu = self.menu
        }
        return menu.first { $0.input == .filter }
    }

    /// Every group that lists containers, depth-first, a child group named
    /// under its parent ("Ready · Mine") — the breadcrumb picker's column.
    public var containerGroups: [(group: SidebarSourceGroup, containers: [SidebarContainer])] {
        var result: [(SidebarSourceGroup, [SidebarContainer])] = []
        for group in groups {
            result.append((group, group.containers))
            for child in group.children {
                let named = SidebarSourceGroup(
                    id: child.id, label: "\(group.label) \u{00B7} \(child.label)", count: child.count,
                    lazy: child.lazy, menu: child.menu, containers: child.containers)
                result.append((named, child.containers))
            }
        }
        return result
    }
}

/// A source project's fold, built from its `info` reading: each container's
/// tasks are the plan's slices filed under it (`milestoneID` is the
/// container's id), so a container listed in two groups nests the same tasks
/// in both.
func buildSidebarSource(_ info: SourceInfo, rows: [SidebarSliceRow], plan: ProjectInfo) -> SidebarSource {
    func container(_ row: SourceContainer) -> SidebarContainer {
        let ids = Set(plan.slices.filter { $0.milestoneID == row.id }.map(\.id))
        let filed = rows.filter { ids.contains($0.sliceID) }
        // As a milestone orders them: what can be started first, then the
        // blocked, then the done.
        let tasks = filed.filter { $0.state != .blocked && $0.state != .done }
            + filed.filter { $0.state == .blocked } + filed.filter { $0.state == .done }
        return SidebarContainer(
            id: row.id, title: row.title, externalURL: row.externalURL, badges: row.badges, meta: row.meta,
            menu: row.menu, tasks: tasks, needsYou: tasks.filter(\.state.needsYou).count)
    }
    func group(_ group: SourceGroup) -> SidebarSourceGroup {
        SidebarSourceGroup(
            id: group.id, label: group.label, count: group.count, lazy: group.lazy, menu: group.menu,
            children: group.children.map { child in
                // One level of sub-groups: a grandchild's containers are its
                // parent's.
                SidebarSourceGroup(
                    id: child.id, label: child.label, count: child.count, lazy: child.lazy, menu: child.menu,
                    containers: (child.containers + child.children.flatMap(\.containers)).map(container))
            },
            containers: group.containers.map(container))
    }
    return SidebarSource(
        name: info.name, title: info.title.isEmpty ? info.name : info.title,
        tag: info.tag, icon: SourceIcon(symbol: info.iconSymbol, svg: info.iconSVG),
        containerNoun: info.containerNoun.isEmpty ? "container" : info.containerNoun,
        taskNoun: info.taskNoun.isEmpty ? "task" : info.taskNoun,
        menu: info.menu, groups: info.groups.map(group), error: info.error)
}

/// The `UserDefaults` key View ▸ Show/Hide Done Items writes.
public let showsDoneItemsKey = "showsDoneItems"

/// What an Active row selects.
public enum SidebarActiveKind: Equatable, Sendable {
    case slice
    case session
    case workshop
}

/// One row of the Active fold: `Project / title` and its dot.
public struct SidebarActiveRow: Equatable, Identifiable, Sendable {
    public let kind: SidebarActiveKind
    /// The slice's or session's own ID; the project's for a workshop row.
    public let targetID: String
    public let projectID: String
    public let projectName: String
    /// The project's short tag — see `projectTags`.
    public let projectTag: String
    public let title: String
    public let state: SliceDisplayState
    public let live: Bool
    /// A slice's pull request's failing checks and conflict, where it was
    /// last read with either — `.none` for every other row.
    public let marks: PRMarks
    /// A workshop row whose workshop has a proposal up — the row's Plan ready
    /// badge. False for every slice and session row.
    public let planReady: Bool
    /// A workshop row drawn from the last run's request while the activity
    /// poll's first reading is still to land — drawn as a launching row,
    /// captioned `reconnectingLabel`. False for every other row.
    public let reconnecting: Bool
    /// The row's project's colour — its badge's; nil where it has none yet.
    public let color: ProjectColor?
    /// A source task's card: Active draws the task nested under it
    /// (`SidebarModel.activeEntries`), and the titlebar names the task by
    /// the card's badge in the project badge's place — a source project
    /// takes no badge of its own (`projectTag` empty). Nil for every other
    /// row, and for a task whose card neither the tree nor the plan names.
    public let card: SidebarActiveCard?

    public var id: String { "\(kind):\(targetID)" }

    /// The SF Symbol drawn in the state dot's place — a workshop row's wand;
    /// nil for every row that draws its dot.
    public var symbol: String? { kind == .workshop ? workshopSymbol : nil }

    public init(
        kind: SidebarActiveKind, targetID: String, projectID: String, projectName: String,
        projectTag: String? = nil, title: String, state: SliceDisplayState, live: Bool,
        marks: PRMarks = .none, planReady: Bool = false, reconnecting: Bool = false, color: ProjectColor? = nil,
        card: SidebarActiveCard? = nil
    ) {
        self.kind = kind
        self.targetID = targetID
        self.projectID = projectID
        self.projectName = projectName
        self.projectTag = projectTag ?? projectTags([(projectID, projectName)])[projectID] ?? ""
        self.title = title
        self.state = state
        self.live = live
        self.marks = marks
        self.planReady = kind == .workshop && planReady
        self.reconnecting = kind == .workshop && reconnecting
        self.color = color
        self.card = kind == .slice ? card : nil
    }
}

/// A source task's card as Active draws it: the top-level row its active
/// tasks nest under, named by its badge — its first (`badge`, a Shortcut
/// card's project), led by the source's `icon` — and its title.
public struct SidebarActiveCard: Equatable, Sendable {
    /// The container's id.
    public let id: String
    public let projectID: String
    public let title: String
    /// Nil for a card with none, which draws no badge and no slash.
    public let badge: SourceBadge?
    public let icon: SourceIcon

    public init(id: String, projectID: String, title: String, badge: SourceBadge?, icon: SourceIcon) {
        self.id = id
        self.projectID = projectID
        self.title = title
        self.badge = badge
        self.icon = icon
    }
}

/// One top-level item of the Active fold: a row as it is, or a source card
/// with its active tasks' rows nested under it.
public enum SidebarActiveEntry: Equatable, Identifiable, Sendable {
    case row(SidebarActiveRow)
    case card(SidebarActiveCard, rows: [SidebarActiveRow])

    public var id: String {
        switch self {
        case .row(let row): row.id
        case .card(let card, _): "card:\(card.projectID):\(card.id)"
        }
    }

    /// The rows it draws, in order.
    public var rows: [SidebarActiveRow] {
        switch self {
        case .row(let row): [row]
        case .card(_, let rows): rows
        }
    }
}

/// Everything one project contributes to the sidebar, as `AppModel` holds it.
public struct SidebarProjectInput: Sendable {
    public let id: String
    public let name: String
    public let kind: SidebarProjectKind
    public let plan: ProjectInfo?
    public let isLoading: Bool
    public let errorMessage: String?
    /// Whether config names it a source project — what files it under its
    /// own fold before its plan (and the `source` in it) has landed.
    public let isSource: Bool
    /// The colour its config entry holds, nil where it holds none.
    public let color: ProjectColor?

    public init(
        id: String, name: String, kind: SidebarProjectKind = .project, plan: ProjectInfo?,
        isLoading: Bool = false, errorMessage: String? = nil, isSource: Bool = false, color: ProjectColor? = nil
    ) {
        self.id = id
        self.name = name
        self.kind = kind
        self.plan = plan
        self.isLoading = isLoading
        self.errorMessage = errorMessage
        self.isSource = isSource
        self.color = color
    }

    /// A source project: config says so, or its plan carries a `source`.
    public var isSourceProject: Bool { kind == .project && (isSource || plan?.source != nil) }
}

/// The sidebar: the Active fold across every project, the Projects tree,
/// each source project's own fold, and the Scratch fold under them.
public struct SidebarModel: Equatable, Sendable {
    public let active: [SidebarActiveRow]
    /// Every project but the scratch one and the source projects.
    public let projects: [SidebarProject]
    /// The source projects, each drawn as a top-level fold of its own
    /// between Projects and Scratch.
    public let sources: [SidebarProject]
    /// The reserved scratch project, drawn as a fold of its own rather than
    /// a row of Projects — nil when there is none open.
    public let scratch: SidebarProject?

    /// Active as the sidebar draws it: `active` in order, each source task
    /// nested under its card, the card standing where its first task would.
    public var activeEntries: [SidebarActiveEntry] {
        var entries: [SidebarActiveEntry] = []
        var cardAt: [String: Int] = [:]
        for row in active {
            guard let card = row.card else {
                entries.append(.row(row))
                continue
            }
            let key = "\(card.projectID):\(card.id)"
            if let index = cardAt[key], case .card(let held, let rows) = entries[index] {
                entries[index] = .card(held, rows: rows + [row])
            } else {
                cardAt[key] = entries.count
                entries.append(.card(card, rows: [row]))
            }
        }
        return entries
    }

    /// The Active heading's hot count.
    public var needsYouCount: Int {
        active.filter(\.state.needsYou).count
    }

    public init(
        active: [SidebarActiveRow], projects: [SidebarProject], sources: [SidebarProject] = [],
        scratch: SidebarProject? = nil
    ) {
        self.active = active
        self.projects = projects
        self.sources = sources
        self.scratch = scratch
    }

    /// The source project with `id`, if it is one.
    public func source(projectID id: String) -> SidebarProject? {
        sources.first { $0.id == id }
    }
}

/// Every project's tag as Active rows and the titlebar carry it: a source
/// project's is its plugin's own `tag` where it has one, every other
/// project's `projectTags`'.
public func sidebarTags(_ projects: [SidebarProjectInput]) -> [String: String] {
    var tags = projectTags(projects.map { (id: $0.id, name: $0.name) })
    for project in projects {
        if let tag = project.plan?.source?.tag, !tag.isEmpty { tags[project.id] = tag }
    }
    return tags
}

/// Each project's short tag for the Active rows: the first three letters of
/// its name, capitalised. Where two or more projects would share a tag, each
/// of them keeps the first two letters and takes a number in the order they
/// are listed instead — "notion" and "nothing" are NO1 and NO2.
public func projectTags(_ projects: [(id: String, name: String)]) -> [String: String] {
    func letters(_ name: String) -> String {
        String(name.filter { $0.isLetter || $0.isNumber }.uppercased())
    }
    let plain = projects.map { (id: $0.id, tag: String(letters($0.name).prefix(3))) }
    var counts: [String: Int] = [:]
    for entry in plain { counts[entry.tag, default: 0] += 1 }
    var seen: [String: Int] = [:]
    var tags: [String: String] = [:]
    for (entry, project) in zip(plain, projects) {
        guard counts[entry.tag, default: 0] > 1 else {
            tags[entry.id] = entry.tag
            continue
        }
        seen[entry.tag, default: 0] += 1
        tags[entry.id] = String(letters(project.name).prefix(2)) + String(seen[entry.tag]!)
    }
    return tags
}

/// The title an ad hoc session's row carries, and a planning agent's.
public let sessionRowTitle = "Ad hoc session"
public let workshopRowTitle = "Workshop"

/// The SF Symbol a workshop row and crumb carry in a state dot's place — the
/// starter card's own glyph, so the row reads as what that opened.
public let workshopSymbol = "wand.and.stars"

/// The badge a workshop row wears while its proposal is up.
public let planReadyLabel = "Plan ready"

/// Builds the sidebar.
///
/// `sessions` are the ad hoc sessions of `sessionsProjectID` alone — the one
/// project `SessionStore` reads — and `planningAgents` the live planning
/// agent of each project that has one, keyed by project ID. `pinnedWorkshops`
/// are the projects whose workshop was opened and not yet launched or
/// dismissed, each a workshop row of its own with nothing running —
/// `launchingWorkshop` the one whose launch is in flight, and
/// `reconnectingWorkshops` the ones restored as running that the activity
/// poll has yet to read (`AppModel.reconnectingWorkshops`). A live agent wins
/// over all three. Active is sorted needs-you first and otherwise left in project,
/// then plan, order. `prMarks` is every project's pull request marks by slice
/// id (`PRStatusStore.marks`), gated by `prMarks(_:for:)`: drawn whole on a
/// slice's Active row, and on its tree row only the conflict — the checks'
/// marks are Active's alone. `proposedWorkshops` are the tabs
/// whose workshop has a proposal up (`AppModel.proposals`' keys — a project's
/// id, an Untitled tab's own): their workshop rows say Plan ready.
public func buildSidebarModel(
    projects: [SidebarProjectInput],
    liveAgents: [String: AgentActivity],
    sessions: [Session] = [],
    sessionsProjectID: String? = nil,
    planningAgents: [String: AgentActivity] = [:],
    pinnedWorkshops: Set<String> = [],
    launchingWorkshop: String? = nil,
    reconnectingWorkshops: Set<String> = [],
    prMarks: [String: PRMarks] = [:],
    proposedWorkshops: Set<String> = []
) -> SidebarModel {
    var active: [SidebarActiveRow] = []
    var built: [SidebarProject] = []
    let tags = sidebarTags(projects)

    for project in projects {
        var needsYou = 0
        // A source project takes no badge: its rows carry no tag, a task's
        // its card's badge instead.
        let tag = project.isSourceProject ? "" : tags[project.id] ?? ""

        if let planner = planningAgents[project.id] {
            let state: SliceDisplayState = planner == .waiting ? .waiting : .working
            if state.needsYou { needsYou += 1 }
            active.append(SidebarActiveRow(
                kind: .workshop, targetID: project.id, projectID: project.id, projectName: project.name, projectTag: tag,
                title: workshopRowTitle, state: state, live: true,
                planReady: proposedWorkshops.contains(project.id), color: project.color))
        } else if reconnectingWorkshops.contains(project.id) {
            // Running when the app last quit, and not yet read again: drawn
            // as a launch is, until the first reading confirms or ends it.
            active.append(SidebarActiveRow(
                kind: .workshop, targetID: project.id, projectID: project.id, projectName: project.name, projectTag: tag,
                title: workshopRowTitle, state: .working, live: false,
                planReady: proposedWorkshops.contains(project.id), reconnecting: true, color: project.color))
        } else if pinnedWorkshops.contains(project.id) || launchingWorkshop == project.id {
            // Opened and not yet running: a draft being written, or a launch
            // on its way — the row holds the workshop's place until then.
            active.append(SidebarActiveRow(
                kind: .workshop, targetID: project.id, projectID: project.id, projectName: project.name, projectTag: tag,
                title: workshopRowTitle, state: launchingWorkshop == project.id ? .working : .todo, live: false,
                planReady: proposedWorkshops.contains(project.id), color: project.color))
        }

        if project.id == sessionsProjectID {
            for session in sessions.sorted(by: { $0.startedAt > $1.startedAt }) {
                let state: SliceDisplayState
                if let agent = liveAgents[session.tag] {
                    state = agent == .waiting ? .waiting : .working
                } else if sessionNeedsReview(session, liveAgents: liveAgents) {
                    state = .review
                } else {
                    continue
                }
                if state.needsYou { needsYou += 1 }
                active.append(SidebarActiveRow(
                    kind: .session, targetID: session.id, projectID: project.id, projectName: project.name, projectTag: tag,
                    title: sessionRowTitle, state: state, live: liveAgents[session.tag] != nil, color: project.color))
            }
        }

        var milestones: [SidebarMilestone] = []
        var doneMilestones: [SidebarMilestone] = []
        var loose: [SidebarSliceRow] = []
        if let plan = project.plan {
            // A pull request read failing its checks or conflicting is marked
            // at the PR stage — never a Done or pre-PR slice, and a resumed
            // one only its failing checks. Passing checks, more narrowly
            // (`prMarks`). The Active row carries all of it; the tree row
            // the conflict alone, the checks' slot left to Active.
            var activeMarks: [String: PRMarks] = [:]
            let rows = plan.slices.map { slice -> SidebarSliceRow in
                let agent = liveAgents[slice.id]
                let marks = NatKit.prMarks(prMarks[slice.id] ?? .none, for: slice)
                activeMarks[slice.id] = marks
                return SidebarSliceRow(
                    sliceID: slice.id, projectID: project.id, title: slice.name,
                    state: displayState(for: slice, agent: agent),
                    live: agent != nil,
                    marks: PRMarks(conflict: marks.conflict))
            }
            let filedUnder = Dictionary(plan.slices.map { ($0.id, $0.milestoneID) }, uniquingKeysWith: { first, _ in first })
            for row in rows where row.state.isInFlight {
                if row.state.needsYou { needsYou += 1 }
                active.append(SidebarActiveRow(
                    kind: .slice, targetID: row.sliceID, projectID: project.id, projectName: project.name, projectTag: tag,
                    title: row.title, state: row.state, live: row.live, marks: activeMarks[row.sliceID] ?? .none,
                    color: project.color,
                    card: activeCard(filedUnder[row.sliceID] ?? "", projectID: project.id, plan: plan)))
            }

            // A source project's tasks are drawn under the plugin's own tree,
            // not as milestones.
            if let info = plan.source {
                built.append(SidebarProject(
                    id: project.id, name: project.name, kind: project.kind, status: planStatus(project),
                    milestones: [], needsYou: needsYou, source: buildSidebarSource(info, rows: rows, plan: plan),
                    color: project.color, tag: ""))
                continue
            }

            var filed = Set<String>()
            for milestone in plan.milestones.sorted(by: { $0.order < $1.order }) {
                let ids = Set(plan.slices.filter { $0.milestoneID == milestone.id }.map(\.id))
                // Plan order, with the blocked ones moved below the rest and
                // the done ones below those: what can be started reads first.
                let filedRows = rows.filter { ids.contains($0.sliceID) }
                let slices = filedRows.filter { $0.state != .blocked && $0.state != .done }
                    + filedRows.filter { $0.state == .blocked } + filedRows.filter { $0.state == .done }
                filed.formUnion(ids)
                if milestone.unfiled {
                    loose = slices
                    continue
                }
                let row = SidebarMilestone(
                    name: milestone.name,
                    done: slices.filter { $0.state == .done }.count,
                    total: slices.count,
                    slices: slices)
                if row.isComplete {
                    doneMilestones.append(row)
                } else {
                    milestones.append(row)
                }
            }
            // A slice filed under no milestone the plan names still has a
            // place to be drawn rather than being dropped from the tree.
            let orphans = rows.filter { !filed.contains($0.sliceID) }
            if !orphans.isEmpty {
                milestones.append(SidebarMilestone(
                    name: "", done: orphans.filter { $0.state == .done }.count,
                    total: orphans.count, slices: orphans))
            }
        }

        built.append(SidebarProject(
            id: project.id, name: project.name, kind: project.kind,
            status: planStatus(project), milestones: milestones, doneMilestones: doneMilestones,
            needsYou: needsYou, loose: loose, color: project.color, tag: tags[project.id] ?? ""))
    }

    // Stable: needs-you first, the rest of the order kept.
    let sorted = active.enumerated().sorted { lhs, rhs in
        if lhs.element.state.needsYou != rhs.element.state.needsYou { return lhs.element.state.needsYou }
        return lhs.offset < rhs.offset
    }.map(\.element)

    let sourceIDs = Set(projects.filter(\.isSourceProject).map(\.id))
    return SidebarModel(
        active: sorted, projects: built.filter { $0.kind != .scratch && !sourceIDs.contains($0.id) },
        sources: built.filter { sourceIDs.contains($0.id) },
        scratch: built.first { $0.kind == .scratch })
}

/// A source task's card as Active draws it — titled as the plugin's tree
/// titles it, else as nat's cache does — nil outside a source project and
/// for a card neither names.
func activeCard(_ id: String, projectID: String, plan: ProjectInfo) -> SidebarActiveCard? {
    guard let info = plan.source, !id.isEmpty else { return nil }
    let container = info.container(withID: id)
    guard let title = container?.title ?? plan.milestones.first(where: { $0.id == id })?.name else { return nil }
    return SidebarActiveCard(id: id, projectID: projectID, title: title, badge: container?.badges.first, icon: info.icon)
}

private func planStatus(_ project: SidebarProjectInput) -> SidebarPlanStatus {
    if project.kind == .untitled { return .none }
    guard let plan = project.plan else {
        if let message = project.errorMessage, !project.isLoading { return .failed(message) }
        return .loading
    }
    if let message = project.errorMessage { return .stale(message) }
    return plan.slices.isEmpty ? .empty : .loaded
}
