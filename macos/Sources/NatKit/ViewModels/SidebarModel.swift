import Foundation

/// The one word the gnat design draws a slice by — its sidebar dot, its
/// Active membership and which navigator section opens first. Read off the
/// slice's `WorkflowStage` (Notion's status, never re-derived) with the live
/// agent refining only what a stage leaves open: whether a slice being worked
/// is waiting on the user.
public enum SliceDisplayState: String, CaseIterable, Equatable, Sendable {
    case todo, working, waiting, review, pr, fixing, blocked, done

    /// The design's `needsYou`: an agent waiting, a branch to review, a pull
    /// request open. What the sidebar sorts Active by and counts in hot.
    public var needsYou: Bool {
        switch self {
        case .waiting, .review, .pr: return true
        case .todo, .working, .fixing, .blocked, .done: return false
        }
    }

    /// The design's `launched`: anything that has had an agent on it.
    public var isLaunched: Bool {
        self != .todo && self != .blocked
    }

    /// What the Active fold lists — `inFlightSliceIDs`'s own rule, so the
    /// fold and the reaper's sweep can never disagree.
    public var isInFlight: Bool {
        switch self {
        case .working, .waiting, .review, .pr, .fixing: return true
        case .todo, .blocked, .done: return false
        }
    }
}

/// A slice's display state. `agent` is the live map's reading, which turns a
/// working (or fixing) slice into a waiting one and nothing else: a session
/// outlives hand-back and approve, so it never moves a slice's stage.
public func displayState(for slice: Slice, agent: AgentActivity?, fixLaunched: Bool) -> SliceDisplayState {
    switch stage(for: slice, agent: agent, fixLaunched: fixLaunched) {
    case .todo: return slice.blocked ? .blocked : .todo
    case .working: return agent == .waiting ? .waiting : .working
    case .fixing: return agent == .waiting ? .waiting : .fixing
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

    public var id: String { sliceID }

    public init(sliceID: String, projectID: String, title: String, state: SliceDisplayState, live: Bool) {
        self.sliceID = sliceID
        self.projectID = projectID
        self.title = title
        self.state = state
        self.live = live
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
    /// How many of its rows need the user — the collapsed row's hot dot.
    public let needsYou: Int

    public init(
        id: String, name: String, kind: SidebarProjectKind, status: SidebarPlanStatus,
        milestones: [SidebarMilestone], doneMilestones: [SidebarMilestone] = [], needsYou: Int
    ) {
        self.id = id
        self.name = name
        self.kind = kind
        self.status = status
        self.milestones = milestones
        self.doneMilestones = doneMilestones
        self.needsYou = needsYou
    }

    /// Whether the project files a slice, for the default-open rule.
    public func contains(sliceID: String) -> Bool {
        (milestones + doneMilestones).contains { $0.slices.contains { $0.sliceID == sliceID } }
    }

    /// Whether the Done folder holds a slice — what opens it by default.
    public func doneContains(sliceID: String) -> Bool {
        doneMilestones.contains { $0.slices.contains { $0.sliceID == sliceID } }
    }
}

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

    public var id: String { "\(kind):\(targetID)" }

    public init(
        kind: SidebarActiveKind, targetID: String, projectID: String, projectName: String,
        projectTag: String? = nil, title: String, state: SliceDisplayState, live: Bool
    ) {
        self.kind = kind
        self.targetID = targetID
        self.projectID = projectID
        self.projectName = projectName
        self.projectTag = projectTag ?? projectTags([(projectID, projectName)])[projectID] ?? ""
        self.title = title
        self.state = state
        self.live = live
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

    public init(
        id: String, name: String, kind: SidebarProjectKind = .project, plan: ProjectInfo?,
        isLoading: Bool = false, errorMessage: String? = nil
    ) {
        self.id = id
        self.name = name
        self.kind = kind
        self.plan = plan
        self.isLoading = isLoading
        self.errorMessage = errorMessage
    }
}

/// The sidebar: the Active fold across every project, the Projects tree, and
/// the Scratch fold under it.
public struct SidebarModel: Equatable, Sendable {
    public let active: [SidebarActiveRow]
    /// Every project but the scratch one.
    public let projects: [SidebarProject]
    /// The reserved scratch project, drawn as a fold of its own rather than
    /// a row of Projects — nil when there is none open.
    public let scratch: SidebarProject?

    /// The Active heading's hot count.
    public var needsYouCount: Int {
        active.filter(\.state.needsYou).count
    }

    public init(active: [SidebarActiveRow], projects: [SidebarProject], scratch: SidebarProject? = nil) {
        self.active = active
        self.projects = projects
        self.scratch = scratch
    }
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
public let workshopRowTitle = "Workshop the plan"

/// Builds the sidebar.
///
/// `sessions` are the ad hoc sessions of `sessionsProjectID` alone — the one
/// project `SessionStore` reads — and `planningAgents` the live planning
/// agent of each project that has one, keyed by project ID. Active is sorted
/// needs-you first and otherwise left in project, then plan, order.
public func buildSidebarModel(
    projects: [SidebarProjectInput],
    liveAgents: [String: AgentActivity],
    sessions: [Session] = [],
    sessionsProjectID: String? = nil,
    planningAgents: [String: AgentActivity] = [:],
    fixLaunched: Set<String> = []
) -> SidebarModel {
    var active: [SidebarActiveRow] = []
    var built: [SidebarProject] = []
    let tags = projectTags(projects.map { (id: $0.id, name: $0.name) })

    for project in projects {
        var needsYou = 0

        if let planner = planningAgents[project.id] {
            let state: SliceDisplayState = planner == .waiting ? .waiting : .working
            if state.needsYou { needsYou += 1 }
            active.append(SidebarActiveRow(
                kind: .workshop, targetID: project.id, projectID: project.id, projectName: project.name, projectTag: tags[project.id],
                title: workshopRowTitle, state: state, live: true))
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
                    kind: .session, targetID: session.id, projectID: project.id, projectName: project.name, projectTag: tags[project.id],
                    title: sessionRowTitle, state: state, live: liveAgents[session.tag] != nil))
            }
        }

        var milestones: [SidebarMilestone] = []
        var doneMilestones: [SidebarMilestone] = []
        if let plan = project.plan {
            let rows = plan.slices.map { slice -> SidebarSliceRow in
                let agent = liveAgents[slice.id]
                return SidebarSliceRow(
                    sliceID: slice.id, projectID: project.id, title: slice.name,
                    state: displayState(for: slice, agent: agent, fixLaunched: fixLaunched.contains(slice.id)),
                    live: agent != nil)
            }
            for row in rows where row.state.isInFlight {
                if row.state.needsYou { needsYou += 1 }
                active.append(SidebarActiveRow(
                    kind: .slice, targetID: row.sliceID, projectID: project.id, projectName: project.name, projectTag: tags[project.id],
                    title: row.title, state: row.state, live: row.live))
            }

            var filed = Set<String>()
            for milestone in plan.milestones.sorted(by: { $0.order < $1.order }) {
                let ids = Set(plan.slices.filter { $0.milestoneID == milestone.id }.map(\.id))
                // Plan order, with the blocked ones moved below the rest:
                // what can be started reads first.
                let filedRows = rows.filter { ids.contains($0.sliceID) }
                let slices = filedRows.filter { $0.state != .blocked } + filedRows.filter { $0.state == .blocked }
                filed.formUnion(ids)
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
            needsYou: needsYou))
    }

    // Stable: needs-you first, the rest of the order kept.
    let sorted = active.enumerated().sorted { lhs, rhs in
        if lhs.element.state.needsYou != rhs.element.state.needsYou { return lhs.element.state.needsYou }
        return lhs.offset < rhs.offset
    }.map(\.element)

    return SidebarModel(
        active: sorted, projects: built.filter { $0.kind != .scratch },
        scratch: built.first { $0.kind == .scratch })
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
