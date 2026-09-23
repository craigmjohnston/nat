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

/// Where a project's plan lives: a Notion database, or a file of nat's own with
/// no workspace behind it.
///
/// Only the word `local` says the second; anything else — an absent key, the
/// empty string, a backend a later nat invented — reads as Notion, which is
/// what every project meant before there was a choice. It is decoded from a
/// string rather than as an enum so an unknown one can never fail the read: a
/// config that will not parse is an app showing onboarding to somebody who has
/// already onboarded.
public enum PlanBackend: String, Equatable, Sendable {
    case notion
    case local

    /// The backend a config file's (or `config-show`'s) word for it means.
    public init(word: String?) {
        self = word == PlanBackend.local.rawValue ? .local : .notion
    }
}

/// Configuration for a single tracked project.
public struct ProjectConfig: Codable, Equatable, Sendable {
    public let name: String
    /// The Notion data source the plan is kept in. A project of nat's own has
    /// none, so the key is absent from its entry and this is nil.
    public let slicesDSID: String?
    public let workingDir: String
    /// Where the plan lives; Notion wherever the entry says nothing.
    public let backend: PlanBackend
    /// The directory a local project's plan file is kept in, where its entry
    /// names one; nil is nat's own data directory.
    public let planDir: String?

    enum CodingKeys: String, CodingKey {
        case name
        case slicesDSID = "slices_ds_id"
        case workingDir = "working_dir"
        case backend
        case planDir = "plan_dir"
    }

    public init(
        name: String,
        slicesDSID: String? = nil,
        workingDir: String,
        backend: PlanBackend = .notion,
        planDir: String? = nil
    ) {
        self.name = name
        self.slicesDSID = slicesDSID
        self.workingDir = workingDir
        self.backend = backend
        self.planDir = planDir
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        slicesDSID = try c.decodeIfPresent(String.self, forKey: .slicesDSID)
        workingDir = try c.decode(String.self, forKey: .workingDir)
        // A backend of the wrong type is no more a reason to lose the config
        // than one this build does not know: both read as Notion.
        backend = PlanBackend(word: try? c.decodeIfPresent(String.self, forKey: .backend))
        planDir = try c.decodeIfPresent(String.self, forKey: .planDir)
    }

    /// Written the way nat writes it: the backend and plan directory only
    /// where they mean something, so an entry for a Notion project round-trips
    /// unchanged.
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(name, forKey: .name)
        try c.encodeIfPresent(slicesDSID, forKey: .slicesDSID)
        try c.encode(workingDir, forKey: .workingDir)
        if backend == .local { try c.encode(backend.rawValue, forKey: .backend) }
        try c.encodeIfPresent(planDir, forKey: .planDir)
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
