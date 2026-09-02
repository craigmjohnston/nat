import Foundation

/// Information about a nat project: its metadata, milestones, and slices.
public struct ProjectInfo: Codable, Equatable {
    public let project: Project
    public let milestones: [Milestone]
    public let slices: [Slice]

    enum CodingKeys: String, CodingKey {
        case project
        case milestones
        case slices
    }

    public init(project: Project, milestones: [Milestone], slices: [Slice]) {
        self.project = project
        self.milestones = milestones
        self.slices = slices
    }
}

/// A project's metadata: ID, name, and conventions.
public struct Project: Codable, Equatable {
    public let id: String
    public let name: String
    public let conventions: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case conventions
    }

    public init(id: String, name: String, conventions: String) {
        self.id = id
        self.name = name
        self.conventions = conventions
    }
}

/// A milestone in the project plan.
public struct Milestone: Codable, Equatable, Identifiable {
    public let id: String
    public let name: String
    public let order: Double
    public let status: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case order
        case status
    }

    public init(id: String, name: String, order: Double, status: String) {
        self.id = id
        self.name = name
        self.order = order
        self.status = status
    }
}

/// A slice of work in the project.
public struct Slice: Codable, Equatable, Identifiable {
    public let id: String
    public let name: String
    public let status: String
    public let milestoneID: String
    public let assignee: String
    public let pr: String
    public let url: String
    public let branch: String?
    public let repo: String?
    public let dependsOn: [String]?
    public let blocked: Bool
    public let handedBack: Bool
    public let state: SliceState?

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case status
        case milestoneID = "milestone_id"
        case assignee
        case pr
        case url
        case branch
        case repo
        case dependsOn = "depends_on"
        case blocked
        case handedBack = "handed_back"
        case state
    }

    public init(
        id: String,
        name: String,
        status: String,
        milestoneID: String,
        assignee: String,
        pr: String,
        url: String,
        branch: String? = nil,
        repo: String? = nil,
        dependsOn: [String]? = nil,
        blocked: Bool,
        handedBack: Bool,
        state: SliceState? = nil
    ) {
        self.id = id
        self.name = name
        self.status = status
        self.milestoneID = milestoneID
        self.assignee = assignee
        self.pr = pr
        self.url = url
        self.branch = branch
        self.repo = repo
        self.dependsOn = dependsOn
        self.blocked = blocked
        self.handedBack = handedBack
        self.state = state
    }
}

/// Paths to nat's configuration and runtime files.
public struct NatPaths: Codable, Equatable {
    public let config: String
    public let logDir: String
    public let nudge: String

    enum CodingKeys: String, CodingKey {
        case config
        case logDir = "log_dir"
        case nudge
    }

    public init(config: String, logDir: String, nudge: String) {
        self.config = config
        self.logDir = logDir
        self.nudge = nudge
    }
}

/// The local configuration for nat: projects, agent settings, UI preferences.
public struct NatProjectConfig: Codable, Equatable {
    public let projects: [String: ProjectConfig]
    public let agentSplitPercent: Int?
    public let pollSeconds: Int?
    public let workshopAgent: AgentModel?
    public let sliceAgent: AgentModel?

    enum CodingKeys: String, CodingKey {
        case projects
        case agentSplitPercent = "agent_split_percent"
        case pollSeconds = "poll_seconds"
        case workshopAgent = "workshop_agent"
        case sliceAgent = "slice_agent"
    }

    public init(
        projects: [String: ProjectConfig],
        agentSplitPercent: Int? = nil,
        pollSeconds: Int? = nil,
        workshopAgent: AgentModel? = nil,
        sliceAgent: AgentModel? = nil
    ) {
        self.projects = projects
        self.agentSplitPercent = agentSplitPercent
        self.pollSeconds = pollSeconds
        self.workshopAgent = workshopAgent
        self.sliceAgent = sliceAgent
    }
}

/// Configuration for a single tracked project.
public struct ProjectConfig: Codable, Equatable {
    public let name: String
    public let slicesDSID: String
    public let workingDir: String

    enum CodingKeys: String, CodingKey {
        case name
        case slicesDSID = "slices_ds_id"
        case workingDir = "working_dir"
    }

    public init(name: String, slicesDSID: String, workingDir: String) {
        self.name = name
        self.slicesDSID = slicesDSID
        self.workingDir = workingDir
    }
}

/// Configuration for an agent (model and effort level).
public struct AgentModel: Codable, Equatable {
    public let model: String?
    public let effort: String?

    enum CodingKeys: String, CodingKey {
        case model
        case effort
    }

    public init(model: String? = nil, effort: String? = nil) {
        self.model = model
        self.effort = effort
    }

    public var isEmpty: Bool {
        model == nil && effort == nil
    }
}
