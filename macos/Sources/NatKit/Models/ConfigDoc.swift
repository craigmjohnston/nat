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
/// and its working directory, the one field `config-set` can change on it.
public struct ConfigDocProject: Codable, Equatable, Sendable {
    public let name: String
    public let workingDir: String

    enum CodingKeys: String, CodingKey {
        case name
        case workingDir = "working_dir"
    }

    public init(name: String, workingDir: String) {
        self.name = name
        self.workingDir = workingDir
    }
}
