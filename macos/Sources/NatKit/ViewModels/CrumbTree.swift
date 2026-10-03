import Foundation

/// The titlebar breadcrumb's tree picker — projects, then a project's
/// milestones, then a milestone's slices, a column apiece — as far as it has
/// been opened. A project crumb opens it on that project, a milestone crumb on
/// that project and milestone.
///
/// A source project's containers stand in for milestones — every container
/// its fold lists, once each, in the fold's order, its groups (segments)
/// being no level of their own — then a container's tasks.
public struct CrumbTree: Equatable, Sendable {
    /// One row of the middle column: a project's unfiled slices sit loose
    /// above its milestones, as the sidebar draws them; a source project's
    /// containers stand in for milestones.
    public enum Entry: Equatable, Identifiable, Sendable {
        case slice(SidebarSliceRow)
        case milestone(SidebarMilestone)
        case container(SidebarContainer)

        public var id: String {
            switch self {
            case .slice(let row): return "s:\(row.sliceID)"
            case .milestone(let milestone): return "m:\(milestone.name)"
            case .container(let container): return "c:\(container.id)"
            }
        }
    }

    /// Every project, the source projects after the rest, the scratch one
    /// last.
    public let projects: [SidebarProject]
    public var projectID: String
    /// The milestone opened, by name; nil with only the project open.
    public var milestone: String?
    /// A source project's container opened, by id.
    public var container: String?

    public init(model: SidebarModel, projectID: String, milestone: String? = nil, container: String? = nil) {
        projects = model.projects.filter { $0.kind != .untitled } + model.sources + (model.scratch.map { [$0] } ?? [])
        self.projectID = projectID
        self.milestone = milestone
        self.container = container
    }

    private var project: SidebarProject? { projects.first { $0.id == projectID } }

    /// The middle column: the open project's loose slices, its milestones
    /// still holding work, then its finished ones — or a source project's
    /// containers, each once though several groups list it.
    public var entries: [Entry] {
        guard let project else { return [] }
        if let source = project.source {
            var seen = Set<String>()
            return source.containerGroups.flatMap(\.containers)
                .filter { seen.insert($0.id).inserted }
                .map(Entry.container)
        }
        return project.loose.map(Entry.slice)
            + (project.milestones + project.doneMilestones).map(Entry.milestone)
    }

    /// The column of slices: the open milestone's, or the open container's
    /// tasks — nil with neither open, when there is no such column at all.
    public var slices: [SidebarSliceRow]? {
        if let source = project?.source {
            guard let container else { return nil }
            return source.container(withID: container)?.tasks ?? []
        }
        guard let milestone, let project else { return nil }
        return (project.milestones + project.doneMilestones).first { $0.name == milestone }?.slices ?? []
    }

    /// Opens a project, closing whatever milestone or container was open.
    public mutating func open(project id: String) {
        guard id != projectID else { return }
        projectID = id
        milestone = nil
        container = nil
    }
}
