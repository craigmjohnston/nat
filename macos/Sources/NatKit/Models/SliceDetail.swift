import Foundation

/// Full details of a slice, including its brief and blocking state.
public struct SliceDetail: Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let url: String
    public let status: String
    public let milestone: String
    public let assignee: String
    public let branch: String?
    public let repo: String?
    public let pr: String?
    public let dependsOn: [String]?
    public let blocked: Bool
    public let handedBack: Bool
    public let state: String?
    public let brief: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case url
        case status
        case milestone
        case assignee
        case branch
        case repo
        case pr
        case dependsOn = "depends_on"
        case blocked
        case handedBack = "handed_back"
        case state
        case brief
    }

    public init(
        id: String,
        name: String,
        url: String,
        status: String,
        milestone: String,
        assignee: String,
        branch: String? = nil,
        repo: String? = nil,
        pr: String? = nil,
        dependsOn: [String]? = nil,
        blocked: Bool,
        handedBack: Bool,
        state: String? = nil,
        brief: String
    ) {
        self.id = id
        self.name = name
        self.url = url
        self.status = status
        self.milestone = milestone
        self.assignee = assignee
        self.branch = branch
        self.repo = repo
        self.pr = pr
        self.dependsOn = dependsOn
        self.blocked = blocked
        self.handedBack = handedBack
        self.state = state
        self.brief = brief
    }
}
