import Foundation

/// Information about a nat project: its metadata, milestones, and slices.
public struct ProjectInfo: Codable, Equatable, Sendable {
    public let project: Project
    public let milestones: [Milestone]
    public let slices: [Slice]
    /// A source project's plugin and sidebar tree (`info --json`'s `source`);
    /// nil for every other project. Its containers are `milestones` too.
    public let source: SourceInfo?

    enum CodingKeys: String, CodingKey {
        case project
        case milestones
        case slices
        case source
    }

    public init(project: Project, milestones: [Milestone], slices: [Slice], source: SourceInfo? = nil) {
        self.project = project
        self.milestones = milestones
        self.slices = slices
        self.source = source
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        project = try c.decode(Project.self, forKey: .project)
        milestones = try c.decode([Milestone].self, forKey: .milestones)
        slices = try c.decode([Slice].self, forKey: .slices)
        source = try c.decodeIfPresent(SourceInfo.self, forKey: .source)
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
    /// The scratch project's reserved milestone, which files the slices
    /// added there with none — `nat info`'s `unfiled`. The sidebar draws its
    /// slices loose at the head of the Scratch fold, never as a folder.
    public let unfiled: Bool

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case order
        case status
        case unfiled
    }

    public init(id: String, name: String, order: Double, status: String, unfiled: Bool = false) {
        self.id = id
        self.name = name
        self.order = order
        self.status = status
        self.unfiled = unfiled
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        order = try container.decode(Double.self, forKey: .order)
        status = try container.decode(String.self, forKey: .status)
        unfiled = try container.decodeIfPresent(Bool.self, forKey: .unfiled) ?? false
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
    /// The slice was resumed after its hand-back: In progress, a PR
    /// recorded and its `Branch` cleared (`nat slice-resume`), on a project
    /// whose shape has a Branch column — nat's own reading, never re-derived
    /// here, since a project with no Branch column holds a PR and no branch
    /// legitimately. Left by the agent's next hand-back. False where nat does
    /// not say.
    public let resumed: Bool
    /// Taken back after a hand-back: In progress, its Branch cleared on a
    /// project with a Branch column, and a Handed back on its task log —
    /// nat's `taken_back`. True for a resumed slice too, and for a review
    /// sent back with no PR, which `resumed` (PR recorded) does not cover:
    /// what keeps its Changes and Visual changes on screen while the work is
    /// redone. Never a stage of its own. False where nat does not say.
    public let takenBack: Bool
    /// The checks a taken-back slice's agent was last given failing — a
    /// Checks failed, or CI's Sent back — and has not handed back a fix for:
    /// nat's `fixing_checks`. What keeps a resumed slice marked failing while
    /// its fix's checks run. Nil where there are none, or nat does not say.
    public let fixingChecks: [String]?
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
        case resumed
        case takenBack = "taken_back"
        case fixingChecks = "fixing_checks"
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
        resumed: Bool = false,
        takenBack: Bool = false,
        fixingChecks: [String]? = nil,
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
        self.resumed = resumed
        self.takenBack = takenBack
        self.fixingChecks = fixingChecks
        self.state = state
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        status = try c.decode(String.self, forKey: .status)
        milestoneID = try c.decode(String.self, forKey: .milestoneID)
        assignee = try c.decode(String.self, forKey: .assignee)
        pr = try c.decode(String.self, forKey: .pr)
        url = try c.decode(String.self, forKey: .url)
        branch = try c.decodeIfPresent(String.self, forKey: .branch)
        repo = try c.decodeIfPresent(String.self, forKey: .repo)
        dependsOn = try c.decodeIfPresent([String].self, forKey: .dependsOn)
        blocked = try c.decode(Bool.self, forKey: .blocked)
        handedBack = try c.decode(Bool.self, forKey: .handedBack)
        resumed = try c.decodeIfPresent(Bool.self, forKey: .resumed) ?? false
        takenBack = try c.decodeIfPresent(Bool.self, forKey: .takenBack) ?? false
        fixingChecks = try c.decodeIfPresent([String].self, forKey: .fixingChecks)
        state = try c.decodeIfPresent(SliceState.self, forKey: .state)
    }
}

/// Paths to nat's configuration and runtime files.
public struct NatPaths: Codable, Equatable, Sendable {
    public let config: String
    public let logDir: String
    public let nudge: String
    /// A project's plan file, where `--project` named one that has a file.
    public let plan: String?

    enum CodingKeys: String, CodingKey {
        case config
        case logDir = "log_dir"
        case nudge
        case plan
    }

    public init(config: String, logDir: String, nudge: String, plan: String? = nil) {
        self.config = config
        self.logDir = logDir
        self.nudge = nudge
        self.plan = plan
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
    /// The reserved local project ad hoc work lives in — one of `projects`,
    /// which the strip pins first and draws icon-only. Nil until `nat
    /// scratch-open` has run.
    public let scratchProject: String?

    enum CodingKeys: String, CodingKey {
        case projects
        case agentSplitPercent = "agent_split_percent"
        case pollSeconds = "poll_seconds"
        case workshopAgent = "workshop_agent"
        case sliceAgent = "slice_agent"
        case assigneeUserName = "assignee_user_name"
        case scratchProject = "scratch_project"
    }

    public init(
        projects: [String: ProjectConfig],
        agentSplitPercent: Int? = nil,
        pollSeconds: Int? = nil,
        workshopAgent: AgentModel? = nil,
        sliceAgent: AgentModel? = nil,
        assigneeUserName: String? = nil,
        scratchProject: String? = nil
    ) {
        self.scratchProject = scratchProject
        self.projects = projects
        self.agentSplitPercent = agentSplitPercent
        self.pollSeconds = pollSeconds
        self.workshopAgent = workshopAgent
        self.sliceAgent = sliceAgent
        self.assigneeUserName = assigneeUserName
    }

    /// Whether the project `id` names takes a colour — nat's
    /// `Config.Colorable`: every one but the scratch project and a source
    /// project, which nat never colours.
    public func takesColor(_ id: String) -> Bool {
        guard let entry = projects[id] else { return false }
        return entry.backend != .source && id != scratchProject
    }
}

/// Where a project's plan lives: a Notion database, or a file of nat's own with
/// no workspace behind it.
///
/// Only the words `local` and `source` (a local plan whose milestones are a
/// task-source plugin's containers) say otherwise; anything else — an absent
/// key, the empty string, a backend a later nat invented — reads as Notion,
/// which is what every project meant before there was a choice. It is decoded
/// from a string rather than as an enum so an unknown one can never fail the
/// read: a config that will not parse is an app showing onboarding to somebody
/// who has already onboarded.
public enum PlanBackend: String, Equatable, Sendable {
    case notion
    case local
    case source

    /// The backend a config file's (or `config-show`'s) word for it means.
    public init(word: String?) {
        switch word {
        case PlanBackend.local.rawValue: self = .local
        case PlanBackend.source.rawValue: self = .source
        default: self = .notion
        }
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
    /// The task-source plugin a `source` project's containers come from, by
    /// name; nil for every other project.
    public let source: String?
    /// The project's run commands — none where its entry names none.
    public let runs: [RunCommand]
    /// The project's colour; nil until nat has given it one (and for a word
    /// this build does not know).
    public let color: ProjectColor?

    enum CodingKeys: String, CodingKey {
        case name
        case slicesDSID = "slices_ds_id"
        case workingDir = "working_dir"
        case backend
        case planDir = "plan_dir"
        case source
        case runs
        case color
    }

    public init(
        name: String,
        slicesDSID: String? = nil,
        workingDir: String,
        backend: PlanBackend = .notion,
        planDir: String? = nil,
        source: String? = nil,
        runs: [RunCommand] = [],
        color: ProjectColor? = nil
    ) {
        self.name = name
        self.slicesDSID = slicesDSID
        self.workingDir = workingDir
        self.backend = backend
        self.planDir = planDir
        self.source = source
        self.runs = runs
        self.color = color
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        // A source project's entry carries no name — it is named by its
        // plugin (`AppModel.tabName`) — so an absent one is empty.
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        slicesDSID = try c.decodeIfPresent(String.self, forKey: .slicesDSID)
        workingDir = try c.decode(String.self, forKey: .workingDir)
        // A backend of the wrong type is no more a reason to lose the config
        // than one this build does not know: both read as Notion.
        backend = PlanBackend(word: try? c.decodeIfPresent(String.self, forKey: .backend))
        planDir = try c.decodeIfPresent(String.self, forKey: .planDir)
        source = try c.decodeIfPresent(String.self, forKey: .source)
        runs = try c.decodeIfPresent([RunCommand].self, forKey: .runs) ?? []
        color = ProjectColor(word: try? c.decodeIfPresent(String.self, forKey: .color))
    }

    /// Written the way nat writes it: the backend, plan directory, source,
    /// runs and colour only where they mean something, so an entry for a
    /// Notion project round-trips unchanged.
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(name, forKey: .name)
        try c.encodeIfPresent(slicesDSID, forKey: .slicesDSID)
        try c.encode(workingDir, forKey: .workingDir)
        if backend != .notion { try c.encode(backend.rawValue, forKey: .backend) }
        try c.encodeIfPresent(planDir, forKey: .planDir)
        try c.encodeIfPresent(source, forKey: .source)
        if !runs.isEmpty { try c.encode(runs, forKey: .runs) }
        try c.encodeIfPresent(color?.rawValue, forKey: .color)
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
