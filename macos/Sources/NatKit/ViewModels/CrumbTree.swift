import Foundation

/// The titlebar breadcrumb's tree picker — projects, then a project's
/// milestones, then a milestone's slices, a column apiece — as far as it has
/// been opened. A project crumb opens it on that project, a milestone crumb on
/// that project and milestone.
public struct CrumbTree: Equatable, Sendable {
    /// One row of the middle column: a project's unfiled slices sit loose
    /// above its milestones, as the sidebar draws them.
    public enum Entry: Equatable, Identifiable, Sendable {
        case slice(SidebarSliceRow)
        case milestone(SidebarMilestone)

        public var id: String {
            switch self {
            case .slice(let row): return "s:\(row.sliceID)"
            case .milestone(let milestone): return "m:\(milestone.name)"
            }
        }
    }

    /// Every project, the scratch one last.
    public let projects: [SidebarProject]
    public var projectID: String
    /// The milestone opened, by name; nil with only the project open.
    public var milestone: String?

    public init(model: SidebarModel, projectID: String, milestone: String? = nil) {
        projects = model.projects.filter { $0.kind != .untitled } + (model.scratch.map { [$0] } ?? [])
        self.projectID = projectID
        self.milestone = milestone
    }

    private var project: SidebarProject? { projects.first { $0.id == projectID } }

    /// The middle column: the open project's loose slices, its milestones
    /// still holding work, then its finished ones.
    public var entries: [Entry] {
        guard let project else { return [] }
        return project.loose.map(Entry.slice)
            + (project.milestones + project.doneMilestones).map(Entry.milestone)
    }

    /// The third column — the open milestone's slices — or nil with no
    /// milestone open, when there is no third column at all.
    public var slices: [SidebarSliceRow]? {
        guard let milestone, let project else { return nil }
        return (project.milestones + project.doneMilestones).first { $0.name == milestone }?.slices ?? []
    }

    /// Opens a project, closing whatever milestone was open.
    public mutating func open(project id: String) {
        guard id != projectID else { return }
        projectID = id
        milestone = nil
    }
}
