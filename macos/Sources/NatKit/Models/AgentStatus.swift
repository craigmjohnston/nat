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

    public var id: String { sliceID }

    enum CodingKeys: String, CodingKey {
        case sliceID = "slice_id"
        case session
        case activity
    }

    public init(sliceID: String, session: String, activity: AgentActivityState) {
        self.sliceID = sliceID
        self.session = session
        self.activity = activity
    }
}
