import Foundation

/// One of a branch's pull requests, as `nat session-list`/`session-status
/// --json` report it — the same shape `internal/cli/sessionlist.go`'s
/// `headPRJSON` writes for both commands.
public struct SessionPR: Codable, Equatable, Sendable {
    public let number: Int
    public let title: String
    public let url: String
    public let state: String
    public let mergedAt: Date?

    enum CodingKeys: String, CodingKey {
        case number, title, url, state
        case mergedAt = "merged_at"
    }

    public init(number: Int, title: String, url: String, state: String, mergedAt: Date? = nil) {
        self.number = number
        self.title = title
        self.url = url
        self.state = state
        self.mergedAt = mergedAt
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        number = try container.decode(Int.self, forKey: .number)
        title = try container.decode(String.self, forKey: .title)
        url = try container.decode(String.self, forKey: .url)
        state = try container.decode(String.self, forKey: .state)
        let raw = try container.decodeIfPresent(String.self, forKey: .mergedAt)
        mergedAt = raw.flatMap(PRDetail.parseGoTime)
    }

    /// Whether this pull request is still open — the fact the rail and
    /// `projectAttention` both key an ad hoc session's state off.
    public var isOpen: Bool { state == "OPEN" }
}

/// One ad hoc session, as `nat session-list --project <id> --json` reports
/// it (mirrors `internal/cli/sessionlist.go`'s `sessionListJSON`). Its own
/// `tag` is the key its live activity sits under in `ActivityStore.agents`
/// — `agent.SessionTag(projectID, id)` on the Go side — carried here rather
/// than recomputed, since the JSON already names it.
public struct Session: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let tag: String
    public let live: Bool
    /// The tmux session name it runs in while live — empty once it is not.
    public let session: String
    public let startedAt: Date
    public let dir: String
    public let branch: String
    public let ended: Bool
    public let prs: [SessionPR]
    public let prsStale: Bool

    enum CodingKeys: String, CodingKey {
        case id, tag, live, session
        case startedAt = "started_at"
        case dir, branch, ended, prs
        case prsStale = "prs_stale"
    }

    public init(
        id: String, tag: String, live: Bool, session: String = "", startedAt: Date,
        dir: String, branch: String = "", ended: Bool = false,
        prs: [SessionPR] = [], prsStale: Bool = false
    ) {
        self.id = id
        self.tag = tag
        self.live = live
        self.session = session
        self.startedAt = startedAt
        self.dir = dir
        self.branch = branch
        self.ended = ended
        self.prs = prs
        self.prsStale = prsStale
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        tag = try container.decode(String.self, forKey: .tag)
        live = try container.decode(Bool.self, forKey: .live)
        session = try container.decodeIfPresent(String.self, forKey: .session) ?? ""
        let rawStarted = try container.decode(String.self, forKey: .startedAt)
        guard let started = PRDetail.parseGoTime(rawStarted) else {
            throw DecodingError.dataCorruptedError(
                forKey: .startedAt, in: container, debugDescription: "unreadable started_at: \(rawStarted)")
        }
        startedAt = started
        dir = try container.decode(String.self, forKey: .dir)
        branch = try container.decodeIfPresent(String.self, forKey: .branch) ?? ""
        ended = try container.decode(Bool.self, forKey: .ended)
        prs = try container.decodeIfPresent([SessionPR].self, forKey: .prs) ?? []
        prsStale = try container.decodeIfPresent(Bool.self, forKey: .prsStale) ?? false
    }

    /// The label the rail and pane draw for this session: its branch where
    /// it has cut one, else the directory it runs in — the two things
    /// `session-launch` can have named it by.
    public var label: String {
        if !branch.isEmpty { return branch }
        return (dir as NSString).lastPathComponent
    }

    /// Every pull request still open, in the order gh reported them.
    public var openPRs: [SessionPR] { prs.filter(\.isOpen) }
}

/// One branch a session has been on and the pull requests it has opened —
/// `nat session-status --json`'s own per-branch entry (mirrors
/// `internal/cli/sessionstatus.go`'s `branchStatusJSON`).
public struct SessionBranchStatus: Codable, Equatable, Sendable {
    public let branch: String
    public let stale: Bool
    public let prs: [SessionPR]

    enum CodingKeys: String, CodingKey {
        case branch, stale, prs
    }

    public init(branch: String, stale: Bool = false, prs: [SessionPR] = []) {
        self.branch = branch
        self.stale = stale
        self.prs = prs
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        branch = try container.decode(String.self, forKey: .branch)
        stale = try container.decodeIfPresent(Bool.self, forKey: .stale) ?? false
        prs = try container.decodeIfPresent([SessionPR].self, forKey: .prs) ?? []
    }
}

/// `nat session-status --project <id> --json <session>`'s whole reading
/// (mirrors `internal/cli/sessionstatus.go`'s `sessionStatusJSON`).
public struct SessionStatusDoc: Codable, Equatable, Sendable {
    public let id: String
    public let live: Bool
    public let ended: Bool
    public let dir: String
    public let branch: String
    public let branches: [SessionBranchStatus]

    enum CodingKeys: String, CodingKey {
        case id, live, ended, dir, branch, branches
    }

    public init(id: String, live: Bool, ended: Bool, dir: String, branch: String, branches: [SessionBranchStatus]) {
        self.id = id
        self.live = live
        self.ended = ended
        self.dir = dir
        self.branch = branch
        self.branches = branches
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        live = try container.decode(Bool.self, forKey: .live)
        ended = try container.decode(Bool.self, forKey: .ended)
        dir = try container.decode(String.self, forKey: .dir)
        branch = try container.decodeIfPresent(String.self, forKey: .branch) ?? ""
        branches = try container.decode([SessionBranchStatus].self, forKey: .branches)
    }

    /// The first pull request any of this session's branches has opened —
    /// the PR tab's own reading, since a session may have more than one and
    /// this slice draws only the first (`session-diff`/`session-status`'s
    /// own next slice picks among the rest).
    public var firstPR: SessionPR? {
        branches.first { !$0.prs.isEmpty }?.prs.first
    }
}

/// `nat session-launch --project <id> --json`'s own reading (mirrors
/// `internal/cli/sessionlaunch.go`'s `sessionLaunchJSON`).
public struct SessionLaunchResult: Codable, Equatable, Sendable {
    public let session: String
    public let tag: String
    public let id: String
    public let dir: String
    public let branch: String
    public let warning: String

    enum CodingKeys: String, CodingKey {
        case session, tag, id, dir, branch, warning
    }

    public init(session: String, tag: String, id: String, dir: String, branch: String, warning: String = "") {
        self.session = session
        self.tag = tag
        self.id = id
        self.dir = dir
        self.branch = branch
        self.warning = warning
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        session = try container.decode(String.self, forKey: .session)
        tag = try container.decode(String.self, forKey: .tag)
        id = try container.decode(String.self, forKey: .id)
        dir = try container.decode(String.self, forKey: .dir)
        branch = try container.decodeIfPresent(String.self, forKey: .branch) ?? ""
        warning = try container.decodeIfPresent(String.self, forKey: .warning) ?? ""
    }
}
