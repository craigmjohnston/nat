import Foundation

/// Information about a nat project: its metadata, milestones, and slices.
public struct ProjectInfo: Codable, Equatable, Sendable {
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
public struct Project: Codable, Equatable, Sendable {
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
public struct Milestone: Codable, Equatable, Identifiable, Sendable {
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
public struct Slice: Codable, Equatable, Identifiable, Sendable {
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
public struct NatPaths: Codable, Equatable, Sendable {
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
public struct NatProjectConfig: Codable, Equatable, Sendable {
    public let projects: [String: ProjectConfig]
    public let agentSplitPercent: Int?
    public let pollSeconds: Int?
    public let workshopAgent: AgentModel?
    public let sliceAgent: AgentModel?
    /// The configured real Notion user's name — whose initials a pending
    /// comment's avatar is drawn with, since a comment left from nat is
    /// always this user's own.
    public let assigneeUserName: String?

    enum CodingKeys: String, CodingKey {
        case projects
        case agentSplitPercent = "agent_split_percent"
        case pollSeconds = "poll_seconds"
        case workshopAgent = "workshop_agent"
        case sliceAgent = "slice_agent"
        case assigneeUserName = "assignee_user_name"
    }

    public init(
        projects: [String: ProjectConfig],
        agentSplitPercent: Int? = nil,
        pollSeconds: Int? = nil,
        workshopAgent: AgentModel? = nil,
        sliceAgent: AgentModel? = nil,
        assigneeUserName: String? = nil
    ) {
        self.projects = projects
        self.agentSplitPercent = agentSplitPercent
        self.pollSeconds = pollSeconds
        self.workshopAgent = workshopAgent
        self.sliceAgent = sliceAgent
        self.assigneeUserName = assigneeUserName
    }
}

/// Configuration for a single tracked project, as the config file itself
/// writes it — this is the file `nat` keeps, read directly rather than through
/// a command, so what it tolerates is what the app can open.
public struct ProjectConfig: Codable, Equatable, Sendable {
    public let name: String

    /// The data source the project's plan lives in, and empty for a project
    /// whose plan is kept in a file of nat's own — there is no data source
    /// behind one, so the field may be absent from its entry entirely.
    public let slicesDSID: String
    public let workingDir: String

    /// Where the plan is kept — `"notion"` or `"local"`. The config file
    /// leaves it unwritten for a Notion project, since that is what every
    /// config written before there was a choice already means, so an absent
    /// one reads as Notion.
    public let backend: String

    /// The directory a local plan's file is kept in, and empty both for a
    /// Notion project and for a local one kept in nat's own data directory.
    public let planDir: String

    /// Whether the plan is kept on this machine rather than in a workspace.
    /// Anything that is not the local word is Notion, the empty string
    /// included: a backend this build does not know came from a later `nat`,
    /// and reading it as local would claim a plan file that is not there.
    public var isLocal: Bool { backend.lowercased() == "local" }

    enum CodingKeys: String, CodingKey {
        case name
        case slicesDSID = "slices_ds_id"
        case workingDir = "working_dir"
        case backend
        case planDir = "plan_dir"
    }

    public init(
        name: String,
        slicesDSID: String = "",
        workingDir: String = "",
        backend: String = "notion",
        planDir: String = ""
    ) {
        self.name = name
        self.slicesDSID = slicesDSID
        self.workingDir = workingDir
        self.backend = backend
        self.planDir = planDir
    }

    /// Every field but the name tolerates absence. A project kept in a file
    /// has no data source and may name no directory, and one hand-written into
    /// the config may name neither — a single missing key must not take the
    /// whole config down with it, since a config that will not parse is an app
    /// that shows onboarding to somebody who has already onboarded.
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        name = try container.decode(String.self, forKey: .name)
        slicesDSID = try container.decodeIfPresent(String.self, forKey: .slicesDSID) ?? ""
        workingDir = try container.decodeIfPresent(String.self, forKey: .workingDir) ?? ""
        backend = try container.decodeIfPresent(String.self, forKey: .backend) ?? "notion"
        planDir = try container.decodeIfPresent(String.self, forKey: .planDir) ?? ""
    }
}

/// Configuration for an agent (model and effort level).
public struct AgentModel: Codable, Equatable, Sendable {
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
