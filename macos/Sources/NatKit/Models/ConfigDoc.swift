import Foundation

/// The structured form of `nat config-show --json`: the fields the settings
/// scene edits and nothing else — the raw stored values, zero and empty
/// meaning unset exactly as the config file itself writes them, rather than
/// the resolved defaults a launch would swap in. This is what is on disk, not
/// what a launch would resolve it to.
public struct ConfigDoc: Codable, Equatable, Sendable {
    public let agentSplitPercent: Int
    public let pollSeconds: Int
    public let workshopAgent: AgentModel
    public let sliceAgent: AgentModel
    public let projects: [String: ConfigDocProject]
    /// The reserved scratch project's ID, so the app finds it without a second
    /// call. Nil until `nat scratch-open` has run, and from an older `nat`.
    public let scratchProject: String?

    enum CodingKeys: String, CodingKey {
        case agentSplitPercent = "agent_split_percent"
        case pollSeconds = "poll_seconds"
        case workshopAgent = "workshop_agent"
        case sliceAgent = "slice_agent"
        case projects
        case scratchProject = "scratch_project"
    }

    public init(
        agentSplitPercent: Int,
        pollSeconds: Int,
        workshopAgent: AgentModel,
        sliceAgent: AgentModel,
        projects: [String: ConfigDocProject],
        scratchProject: String? = nil
    ) {
        self.scratchProject = scratchProject
        self.agentSplitPercent = agentSplitPercent
        self.pollSeconds = pollSeconds
        self.workshopAgent = workshopAgent
        self.sliceAgent = sliceAgent
        self.projects = projects
    }
}

/// One tracked project's share of the config file, as `config-show` prints
/// it: its name, for labelling the settings field without a second lookup,
/// its working directory, the one field `config-set` can change on it, and
/// where its plan lives — said out loud for every project, the Notion ones
/// whose config entry leaves it unwritten included.
public struct ConfigDocProject: Codable, Equatable, Sendable {
    public let name: String
    public let workingDir: String
    /// Where the plan lives. An absent or unknown word reads as Notion, so an
    /// older `nat` that says nothing of it still decodes.
    public let backend: PlanBackend
    /// The directory a local project's plan file is kept in, when it chose one.
    public let planDir: String?

    enum CodingKeys: String, CodingKey {
        case name
        case workingDir = "working_dir"
        case backend
        case planDir = "plan_dir"
    }

    public init(name: String, workingDir: String, backend: PlanBackend = .notion, planDir: String? = nil) {
        self.name = name
        self.workingDir = workingDir
        self.backend = backend
        self.planDir = planDir
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        workingDir = try c.decode(String.self, forKey: .workingDir)
        backend = PlanBackend(word: try? c.decodeIfPresent(String.self, forKey: .backend))
        planDir = try c.decodeIfPresent(String.self, forKey: .planDir)
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(name, forKey: .name)
        try c.encode(workingDir, forKey: .workingDir)
        try c.encode(backend.rawValue, forKey: .backend)
        try c.encodeIfPresent(planDir, forKey: .planDir)
    }
}
