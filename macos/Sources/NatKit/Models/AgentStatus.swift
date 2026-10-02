import Foundation

/// The live activity state of an agent (from the tmux API).
public enum AgentActivityState: String, Codable, Equatable, Sendable {
    case working
    case waiting
    case unknown

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let rawValue = try container.decode(String.self)
        self = AgentActivityState(rawValue: rawValue) ?? .unknown
    }
}

/// Status of a running agent: its slice, session name, and current activity.
public struct AgentStatus: Codable, Equatable, Sendable, Identifiable {
    public let sliceID: String
    public let session: String
    public let activity: AgentActivityState
    /// Off the agent's own statusline (`nat status --json`): each is absent
    /// — never zero — until the agent's first payload lands.
    public let model: String?
    public let effort: String?
    public let contextPercent: Double?
    /// The tokens in the agent's context window — what `contextPercent` is
    /// a share of.
    public let contextTokens: Int?

    public var id: String { sliceID }

    enum CodingKeys: String, CodingKey {
        case sliceID = "slice_id"
        case session
        case activity
        case model
        case effort
        case contextPercent = "context_percent"
        case contextTokens = "context_tokens"
    }

    public init(
        sliceID: String, session: String, activity: AgentActivityState,
        model: String? = nil, effort: String? = nil, contextPercent: Double? = nil, contextTokens: Int? = nil
    ) {
        self.sliceID = sliceID
        self.session = session
        self.activity = activity
        self.model = model
        self.effort = effort
        self.contextPercent = contextPercent
        self.contextTokens = contextTokens
    }
}
