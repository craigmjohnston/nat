import Foundation

/// One batched GitHub reading — `nat pr-status --project …` naming every
/// project at once, one GraphQL document behind it — as the app takes it:
/// each project's reading by ID, the budget GitHub said is left, and the pull
/// request asked for in full (`--detail`), where one was.
public struct GitHubReading: Equatable, Sendable {
    public let projects: [String: PRStatusDoc]
    public let rateLimit: GitHubRateLimit?
    public let detail: PRDetail?

    public init(projects: [String: PRStatusDoc], rateLimit: GitHubRateLimit? = nil, detail: PRDetail? = nil) {
        self.projects = projects
        self.rateLimit = rateLimit
        self.detail = detail
    }

    /// Decodes nat's answer for `projectIDs`: a run naming one project prints
    /// that project's reading with the rate limit and detail beside it, one
    /// naming several keys each by ID under `projects` and carries those two
    /// once.
    public static func decode(_ data: Data, projectIDs: [String]) throws -> GitHubReading {
        let decoder = JSONDecoder()
        let top = try decoder.decode(Top.self, from: data)
        if projectIDs.count == 1, let only = projectIDs.first {
            return GitHubReading(
                projects: [only: try decoder.decode(PRStatusDoc.self, from: data)],
                rateLimit: top.rateLimit, detail: top.detail)
        }
        return GitHubReading(projects: top.projects ?? [:], rateLimit: top.rateLimit, detail: top.detail)
    }

    private struct Top: Decodable {
        let projects: [String: PRStatusDoc]?
        let rateLimit: GitHubRateLimit?
        let detail: PRDetail?

        enum CodingKeys: String, CodingKey {
            case projects, detail
            case rateLimit = "rate_limit"
        }
    }
}

/// GitHub's GraphQL budget as the reading's document left it: the hour's
/// points, what is left of them, and when the hour resets. Kept for the
/// throttle and the status bar; nothing draws it yet.
public struct GitHubRateLimit: Codable, Equatable, Sendable {
    public let limit: Int
    public let remaining: Int
    public let resetAt: Date

    enum CodingKeys: String, CodingKey {
        case limit, remaining
        case resetAt = "reset_at"
    }

    public init(limit: Int, remaining: Int, resetAt: Date) {
        self.limit = limit
        self.remaining = remaining
        self.resetAt = resetAt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        limit = try c.decode(Int.self, forKey: .limit)
        remaining = try c.decode(Int.self, forKey: .remaining)
        let raw = try c.decode(String.self, forKey: .resetAt)
        guard let at = PRDetail.parseGoTime(raw) else {
            throw DecodingError.dataCorruptedError(
                forKey: .resetAt, in: c, debugDescription: "unreadable reset_at: \(raw)")
        }
        resetAt = at
    }
}
