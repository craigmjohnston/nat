import Foundation

/// The wire shape of `nat pr-view --project <id> --json <slice>`: one pull
/// request in full, gh's own fields kept as gh writes them (mirrors
/// `internal/cli/prview.go`'s `prDoc`) — GitHub's vocabulary rather than a
/// word this app invented, exactly as `internal/gh.PR` itself keeps it.
///
/// `PRPresentation.swift` is what turns these raw fields into what the PR tab
/// draws (a state chip, a check's outcome, a review's tone, a merge
/// verdict) — this type carries nothing already interpreted.
public struct PRDetail: Codable, Equatable, Sendable {
    public let number: Int
    public let title: String
    public let body: String
    public let state: String
    public let isDraft: Bool
    public let author: String
    public let baseRefName: String
    public let headRefName: String
    public let url: String
    public let checks: [PRCheck]
    public let reviews: [PRReview]
    public let comments: [PRCommentEntry]
    public let reviewDecision: String
    public let mergeable: String
    public let mergeStateStatus: String

    /// The change's own tally — additions, deletions, the files touched and
    /// how many commits are on the branch. Optional because they are not
    /// (yet) part of every `nat pr-view --json` a caller might be reading:
    /// a `nat` older than the Go side's own addition of these fields simply
    /// omits the keys, which decodes as `nil` here rather than failing the
    /// whole read — the CHANGES section falls back to naming the two
    /// branches alone rather than inventing a number nobody sent.
    public let additions: Int?
    public let deletions: Int?
    public let changedFiles: Int?
    public let commits: Int?

    enum CodingKeys: String, CodingKey {
        case number, title, body, state
        case isDraft = "is_draft"
        case author
        case baseRefName = "base_ref_name"
        case headRefName = "head_ref_name"
        case url, checks, reviews, comments
        case reviewDecision = "review_decision"
        case mergeable
        case mergeStateStatus = "merge_state_status"
        case additions, deletions
        case changedFiles = "changed_files"
        case commits
    }

    public init(
        number: Int,
        title: String,
        body: String,
        state: String,
        isDraft: Bool,
        author: String,
        baseRefName: String,
        headRefName: String,
        url: String,
        checks: [PRCheck] = [],
        reviews: [PRReview] = [],
        comments: [PRCommentEntry] = [],
        reviewDecision: String,
        mergeable: String,
        mergeStateStatus: String,
        additions: Int? = nil,
        deletions: Int? = nil,
        changedFiles: Int? = nil,
        commits: Int? = nil
    ) {
        self.number = number
        self.title = title
        self.body = body
        self.state = state
        self.isDraft = isDraft
        self.author = author
        self.baseRefName = baseRefName
        self.headRefName = headRefName
        self.url = url
        self.checks = checks
        self.reviews = reviews
        self.comments = comments
        self.reviewDecision = reviewDecision
        self.mergeable = mergeable
        self.mergeStateStatus = mergeStateStatus
        self.additions = additions
        self.deletions = deletions
        self.changedFiles = changedFiles
        self.commits = commits
    }
}

/// The two lifecycle states a reader acts on — GitHub's own words, exactly as
/// `internal/gh.PRStateMerged`/`PRStateClosed` name them. OPEN is not among
/// them on purpose: it is what a pull request is when it is neither of these.
public enum PRLifecycleState {
    public static let merged = "MERGED"
    public static let closed = "CLOSED"
}

/// One entry of the status check rollup: what it is called, where it stands
/// and where the run itself can be read.
public struct PRCheck: Codable, Equatable, Sendable {
    public let name: String
    public let state: String
    public let link: String

    public init(name: String, state: String, link: String) {
        self.name = name
        self.state = state
        self.link = link
    }
}

/// One review left on the pull request: who submitted it, what they submitted
/// it as, what they wrote with it (empty for the many reviews that are a
/// verdict and no words), and when.
///
/// `submittedAt` decodes to `nil` for a review nobody has submitted yet — the
/// Go side's `time.Time` zero value round-trips through JSON as
/// `"0001-01-01T00:00:00Z"` rather than being omitted (Go's `omitempty` has no
/// effect on a struct field), so that exact marker is read back as "no time at
/// all" here, which is what `convoTone`'s filter rule needs: a review with no
/// `submitted_at` is dropped from the conversation.
public struct PRReview: Codable, Equatable, Sendable {
    public let author: String
    public let state: String
    public let body: String
    public let submittedAt: Date?

    enum CodingKeys: String, CodingKey {
        case author, state, body
        case submittedAt = "submitted_at"
    }

    public init(author: String, state: String, body: String, submittedAt: Date?) {
        self.author = author
        self.state = state
        self.body = body
        self.submittedAt = submittedAt
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        author = try container.decode(String.self, forKey: .author)
        state = try container.decode(String.self, forKey: .state)
        body = try container.decode(String.self, forKey: .body)
        let raw = try container.decodeIfPresent(String.self, forKey: .submittedAt)
        submittedAt = raw.flatMap(PRDetail.parseGoTime)
    }
}

/// One comment on the pull request itself — the conversation, not comments
/// left on lines of the diff, which `gh pr view` does not carry.
public struct PRCommentEntry: Codable, Equatable, Sendable {
    public let author: String
    public let body: String
    public let createdAt: Date
    public let url: String

    enum CodingKeys: String, CodingKey {
        case author, body
        case createdAt = "created_at"
        case url
    }

    public init(author: String, body: String, createdAt: Date, url: String) {
        self.author = author
        self.body = body
        self.createdAt = createdAt
        self.url = url
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        author = try container.decode(String.self, forKey: .author)
        body = try container.decode(String.self, forKey: .body)
        url = try container.decode(String.self, forKey: .url)
        let raw = try container.decode(String.self, forKey: .createdAt)
        guard let date = PRDetail.parseGoTime(raw) else {
            throw DecodingError.dataCorruptedError(
                forKey: .createdAt, in: container, debugDescription: "unreadable created_at: \(raw)")
        }
        createdAt = date
    }
}

// MARK: - Go's time.Time, read back

extension PRDetail {
    /// What Go's `encoding/json` writes for a zero `time.Time` — not omitted
    /// despite `omitempty`, since that tag has no effect on struct fields.
    static let zeroGoTime = "0001-01-01T00:00:00Z"

    /// Parses a Go-written RFC3339 timestamp, reading the zero-time marker
    /// back as "no time at all" rather than a real (if absurd) date.
    static func parseGoTime(_ raw: String) -> Date? {
        guard raw != zeroGoTime, !raw.isEmpty else { return nil }
        if let date = iso8601Fractional.date(from: raw) { return date }
        return iso8601Plain.date(from: raw)
    }

    // `ISO8601DateFormatter` is not `Sendable`, but Foundation's formatters
    // are documented thread-safe for concurrent reads once configured and
    // never mutated again, which is the whole of how these two are used —
    // hence the explicit opt-out rather than paying for a fresh formatter
    // (and its locale/calendar setup) on every parse.
    nonisolated(unsafe) private static let iso8601Fractional: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    nonisolated(unsafe) private static let iso8601Plain: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()
}
