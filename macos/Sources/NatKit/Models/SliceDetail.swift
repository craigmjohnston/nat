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
    /// The images the slice's agent last handed in of what it changed —
    /// `nat slice-show`'s `visuals`, absent (decoded as empty) when none were.
    public let visuals: [VisualChange]
    /// What has happened to the slice, in order — `nat slice-show`'s
    /// `events`: each hand-back, send-back, release, relaunch and proposal of
    /// follow-ups its page records, then the approve and merge its properties
    /// say. Nil from a nat too old to report them, which the Task log reads
    /// as "fall back to what the properties alone say".
    public let events: [TaskLogEvent]?
    /// The container a task in a source project is filed under —
    /// `slice-show`'s `container`; nil for every other project.
    public let container: SliceContainer?

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
        case visuals
        case events
        case container
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
        visuals = try c.decodeIfPresent([VisualChange].self, forKey: .visuals) ?? []
        // A kind this build does not know is left out rather than failing
        // the whole read — a newer nat may record more than this app draws.
        events = try c.decodeIfPresent([LossyTaskLogEvent].self, forKey: .events)?.compactMap(\.event)
        container = try c.decodeIfPresent(SliceContainer.self, forKey: .container)
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
        followUps: [FollowUp] = [],
        visuals: [VisualChange] = [],
        events: [TaskLogEvent]? = nil,
        container: SliceContainer? = nil
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
        self.visuals = visuals
        self.events = events
        self.container = container
    }
}

/// One thing that happened to a slice, as `nat slice-show`'s `events` reads
/// it off the slice's page and properties.
public struct TaskLogEvent: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case handedBack = "handed_back"
        case sentBack = "sent_back"
        case released
        case relaunched
        case blocked
        case summary
        case followUps = "follow_ups"
        /// A note left on the brief with `nat slice-note`.
        case note
        case approved
        case merged
    }

    public let kind: Kind
    /// The section's own text: a hand-back's note, the comments sent back,
    /// a blocked or closing summary, a note's text without its provenance.
    public let note: String?
    /// Who released it, or who a note came from — a slice by name and
    /// milestone, or a person.
    public let by: String?
    /// The slice a note came from, by name and milestone, where its
    /// provenance names one — `fromSlice`. Nat does not resolve it to an ID;
    /// the Thread matches it against the plan it already holds.
    public let fromSlice: NoteSource?
    /// When it happened, off the stamp its section opens with — `at`. Nil
    /// for a section written before sections were stamped, and for an
    /// approve or merge, which nat has no time for.
    public let at: Date?
    /// The pull request an approve opened.
    public let pr: String?
    /// A proposal's follow-ups, each with what was decided about it.
    public let followUps: [TaskFollowUp]

    public init(
        _ kind: Kind, note: String? = nil, by: String? = nil, fromSlice: NoteSource? = nil, at: Date? = nil,
        pr: String? = nil, followUps: [TaskFollowUp] = []
    ) {
        self.kind = kind
        self.note = note
        self.by = by
        self.fromSlice = fromSlice
        self.at = at
        self.pr = pr
        self.followUps = followUps
    }

    enum CodingKeys: String, CodingKey { case kind, note, by, fromSlice, at, pr, followUps }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        kind = try c.decode(Kind.self, forKey: .kind)
        note = try c.decodeIfPresent(String.self, forKey: .note)
        by = try c.decodeIfPresent(String.self, forKey: .by)
        fromSlice = try c.decodeIfPresent(NoteSource.self, forKey: .fromSlice)
        // A time that will not parse is no time, not a failed read.
        at = (try c.decodeIfPresent(String.self, forKey: .at)).flatMap(PRDetail.parseGoTime)
        pr = try c.decodeIfPresent(String.self, forKey: .pr)
        followUps = try c.decodeIfPresent([TaskFollowUp].self, forKey: .followUps) ?? []
    }
}

/// A slice as a note's provenance names it — `slice-show`'s `fromSlice`: its
/// name, and its milestone's name, empty where it is filed under none.
public struct NoteSource: Codable, Equatable, Sendable {
    public let name: String
    public let milestone: String

    public init(name: String, milestone: String = "") {
        self.name = name
        self.milestone = milestone
    }

    enum CodingKeys: String, CodingKey { case name, milestone }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        milestone = try c.decodeIfPresent(String.self, forKey: .milestone) ?? ""
    }
}

/// One follow-up of a proposal in the Task log: what it was, and what the
/// user made of it — nil while it still awaits a decision.
public struct TaskFollowUp: Codable, Equatable, Sendable {
    public enum Decision: String, Codable, Sendable {
        case queued, folded, dropped
    }

    public let index: Int
    public let title: String
    public let brief: String
    public let decision: Decision?
    /// Where a queued one's slice is.
    public let link: String?

    public init(index: Int, title: String, brief: String = "", decision: Decision? = nil, link: String? = nil) {
        self.index = index
        self.title = title
        self.brief = brief
        self.decision = decision
        self.link = link
    }

    enum CodingKeys: String, CodingKey { case index, title, brief, decision, link }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        index = try c.decode(Int.self, forKey: .index)
        title = try c.decode(String.self, forKey: .title)
        brief = try c.decodeIfPresent(String.self, forKey: .brief) ?? ""
        // An empty or unknown decision is one still to be made.
        decision = (try c.decodeIfPresent(String.self, forKey: .decision)).flatMap(Decision.init(rawValue:))
        link = try c.decodeIfPresent(String.self, forKey: .link)
    }
}

/// An event that decodes to nil rather than failing where its kind is one
/// this build does not know.
private struct LossyTaskLogEvent: Decodable {
    let event: TaskLogEvent?

    init(from decoder: Decoder) throws {
        event = try? TaskLogEvent(from: decoder)
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

/// One image an agent handed in of what its slice changed, as `slice-show`
/// reads it: its 1-based index in the hand-in, what it shows, and where it is —
/// an absolute path, or a URI as the agent gave it.
public struct VisualChange: Codable, Equatable, Hashable, Sendable, Identifiable {
    public let index: Int
    public let name: String
    public let uri: String

    public var id: Int { index }

    public init(index: Int, name: String, uri: String) {
        self.index = index
        self.name = name
        self.uri = uri
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
