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
    /// The branch a launch cuts the slice's worktree from, as nat resolves
    /// it in the slice's repo; absent with no repo to ask.
    public let base: String?
    public let pr: String?
    public let dependsOn: [String]?
    public let blocked: Bool
    public let handedBack: Bool
    public let state: String?
    public let brief: String
    /// The follow-ups the slice's agent handed in that still await the
    /// user's decision — `nat slice-show`'s `followUps`, absent (decoded as
    /// empty) when there are none.
    public let followUps: [FollowUp]

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case url
        case status
        case milestone
        case assignee
        case branch
        case repo
        case base
        case pr
        case dependsOn = "depends_on"
        case blocked
        case handedBack = "handed_back"
        case state
        case brief
        case followUps
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        url = try c.decode(String.self, forKey: .url)
        status = try c.decode(String.self, forKey: .status)
        milestone = try c.decode(String.self, forKey: .milestone)
        assignee = try c.decode(String.self, forKey: .assignee)
        branch = try c.decodeIfPresent(String.self, forKey: .branch)
        repo = try c.decodeIfPresent(String.self, forKey: .repo)
        base = try c.decodeIfPresent(String.self, forKey: .base)
        pr = try c.decodeIfPresent(String.self, forKey: .pr)
        dependsOn = try c.decodeIfPresent([String].self, forKey: .dependsOn)
        blocked = try c.decode(Bool.self, forKey: .blocked)
        handedBack = try c.decode(Bool.self, forKey: .handedBack)
        state = try c.decodeIfPresent(String.self, forKey: .state)
        brief = try c.decode(String.self, forKey: .brief)
        followUps = try c.decodeIfPresent([FollowUp].self, forKey: .followUps) ?? []
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
        base: String? = nil,
        pr: String? = nil,
        dependsOn: [String]? = nil,
        blocked: Bool,
        handedBack: Bool,
        state: String? = nil,
        brief: String,
        followUps: [FollowUp] = []
    ) {
        self.id = id
        self.name = name
        self.url = url
        self.status = status
        self.milestone = milestone
        self.assignee = assignee
        self.branch = branch
        self.repo = repo
        self.base = base
        self.pr = pr
        self.dependsOn = dependsOn
        self.blocked = blocked
        self.handedBack = handedBack
        self.state = state
        self.brief = brief
        self.followUps = followUps
    }
}

/// One follow-up an agent proposed before handing back, as `slice-show`
/// reads it: its 1-based index in the proposal — what `slice-triage` names
/// it by — its title and its brief.
public struct FollowUp: Codable, Equatable, Hashable, Sendable, Identifiable {
    public let index: Int
    public let title: String
    public let brief: String

    public var id: Int { index }

    public init(index: Int, title: String, brief: String) {
        self.index = index
        self.title = title
        self.brief = brief
    }
}

/// What `nat slice-triage --json` reports: the slices queued, and the
/// titles folded in and dropped.
public struct TriageResult: Codable, Equatable, Sendable {
    public struct Queued: Codable, Equatable, Sendable {
        public let title: String
        public let id: String
        public let url: String

        public init(title: String, id: String, url: String) {
            self.title = title
            self.id = id
            self.url = url
        }
    }

    public let queued: [Queued]
    public let folded: [String]
    public let dropped: [String]

    public init(queued: [Queued], folded: [String], dropped: [String]) {
        self.queued = queued
        self.folded = folded
        self.dropped = dropped
    }
}
