import Foundation

/// `nat pr-status --json`: the board's PR-readiness reading taken headlessly —
/// one entry per slice whose pull request anything might still be waiting on,
/// in the plan's own order.
public struct PRStatusDoc: Codable, Equatable, Sendable {
    public let slices: [PRStatusSlice]

    public init(slices: [PRStatusSlice]) {
        self.slices = slices
    }
}

/// One slice's reading, in `domain.PRReadiness`'s own words: "awaiting
/// review", "ready to merge" and "checks failing" are a pull request
/// positively read as open,
/// and "unread" is everything else at once — no longer open, or a repository
/// whose listing could not be taken, which the reading deliberately does not
/// tell apart.
public struct PRStatusSlice: Codable, Equatable, Sendable {
    public let sliceID: String
    public let name: String
    public let pr: String
    public let readiness: String
    /// How the pull request's checks stand — present for every open pull
    /// request the listing read, nil otherwise.
    public let checks: PRStatusChecks?
    /// GitHub positively said the branch conflicts with its base — false for
    /// a mergeable branch, one whose mergeability is still being worked out,
    /// a pull request the reading did not reach, and an older `nat` that sent
    /// no such key.
    public let conflicting: Bool
    /// The branch the pull request merges into, where the reading had it.
    public let base: String?

    enum CodingKeys: String, CodingKey {
        case sliceID = "slice_id"
        case name, pr, readiness, checks, conflicting, base
    }

    public init(
        sliceID: String, name: String, pr: String, readiness: String, checks: PRStatusChecks? = nil,
        conflicting: Bool = false, base: String? = nil
    ) {
        self.sliceID = sliceID
        self.name = name
        self.pr = pr
        self.readiness = readiness
        self.checks = checks
        self.conflicting = conflicting
        self.base = base
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        sliceID = try c.decode(String.self, forKey: .sliceID)
        name = try c.decode(String.self, forKey: .name)
        pr = try c.decode(String.self, forKey: .pr)
        readiness = try c.decode(String.self, forKey: .readiness)
        checks = try c.decodeIfPresent(PRStatusChecks.self, forKey: .checks)
        conflicting = try c.decodeIfPresent(Bool.self, forKey: .conflicting) ?? false
        base = try c.decodeIfPresent(String.self, forKey: .base)
    }

    /// `domain.PRReadiness`'s affirmative words, said once here rather
    /// than spelled out wherever a reading is compared against one.
    public static let awaitingReview = "awaiting review"
    public static let readyToMerge = "ready to merge"
    public static let checksFailing = "checks failing"

    /// Whether the reading positively saw this pull request open — the one
    /// fact rail membership rides on.
    public var isOpen: Bool {
        readiness == Self.awaitingReview || readiness == Self.readyToMerge
            || readiness == Self.checksFailing
    }
}

/// One open pull request's checks, as `nat pr-status` reads them: the
/// verdict — "passing", "failing", "pending" or "none" — and every check
/// that failed, by name and run URL.
public struct PRStatusChecks: Codable, Equatable, Sendable {
    public let verdict: String
    public let failing: [PRStatusCheck]

    public init(verdict: String, failing: [PRStatusCheck] = []) {
        self.verdict = verdict
        self.failing = failing
    }

    enum CodingKeys: String, CodingKey { case verdict, failing }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        verdict = try c.decode(String.self, forKey: .verdict)
        failing = try c.decodeIfPresent([PRStatusCheck].self, forKey: .failing) ?? []
    }
}

/// One failed check: its name and where its run can be read.
public struct PRStatusCheck: Codable, Equatable, Sendable {
    public let name: String
    public let url: String

    public init(name: String, url: String) {
        self.name = name
        self.url = url
    }
}
