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

    enum CodingKeys: String, CodingKey {
        case agentSplitPercent = "agent_split_percent"
        case pollSeconds = "poll_seconds"
        case workshopAgent = "workshop_agent"
        case sliceAgent = "slice_agent"
        case projects
    }

    public init(
        agentSplitPercent: Int,
        pollSeconds: Int,
        workshopAgent: AgentModel,
        sliceAgent: AgentModel,
        projects: [String: ConfigDocProject]
    ) {
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
/// where its plan is kept.
public struct ConfigDocProject: Codable, Equatable, Sendable {
    public let name: String
    public let workingDir: String

    /// Where the project's plan lives — `"notion"` or `"local"`. `config-show`
    /// always says which, even though the config file itself leaves it
    /// unwritten for a Notion project; a listing read by somebody asking which
    /// is which must not answer with a blank. An older `nat` that does not
    /// print it at all reads as Notion, which is what every project was before
    /// there was a choice.
    public let backend: String

    /// The directory a local plan's file is kept in, and empty both for a
    /// Notion project and for a local one kept in nat's own data directory.
    public let planDir: String

    /// Whether the plan is kept on this machine rather than in a workspace.
    /// Anything that is not the local word is Notion, the empty string
    /// included: a backend this build does not know came from a later `nat`,
    /// and reading it as local would claim a file that is not there.
    public var isLocal: Bool { backend.lowercased() == "local" }

    enum CodingKeys: String, CodingKey {
        case name
        case workingDir = "working_dir"
        case backend
        case planDir = "plan_dir"
    }

    public init(name: String, workingDir: String, backend: String = "notion", planDir: String = "") {
        self.name = name
        self.workingDir = workingDir
        self.backend = backend
        self.planDir = planDir
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        name = try container.decode(String.self, forKey: .name)
        workingDir = try container.decodeIfPresent(String.self, forKey: .workingDir) ?? ""
        // Absent from anything an older nat printed, which is a project kept
        // in Notion because that is all there was.
        backend = try container.decodeIfPresent(String.self, forKey: .backend) ?? "notion"
        planDir = try container.decodeIfPresent(String.self, forKey: .planDir) ?? ""
    }
}
