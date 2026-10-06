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

/// GitHub's GraphQL budget as the reading's document left it — the hour's
/// points, what is left of them, and when the hour resets — and nat's policy
/// for the next polling read (`internal/gh`'s budget): what it projects is
/// left at the reset, whether polling is stretched (`throttled`) or stopped
/// on a refusal until `pausedUntil`, the interval nat wants before the next
/// reading (`pollAfterSeconds`, which the read loop sleeps for) and the
/// points this reading spent (`cost`). The policy fields are absent from an
/// older nat, and read as nothing to say.
public struct GitHubRateLimit: Codable, Equatable, Sendable {
    public let limit: Int
    public let remaining: Int
    public let resetAt: Date
    public let projectedRemainingAtReset: Int?
    public let throttled: Bool
    public let pausedUntil: Date?
    public let pollAfterSeconds: Int?
    public let cost: Int

    enum CodingKeys: String, CodingKey {
        case limit, remaining, throttled, cost
        case resetAt = "reset_at"
        case projectedRemainingAtReset = "projected_remaining_at_reset"
        case pausedUntil = "paused_until"
        case pollAfterSeconds = "poll_after_seconds"
    }

    public init(
        limit: Int, remaining: Int, resetAt: Date, projectedRemainingAtReset: Int? = nil, throttled: Bool = false,
        pausedUntil: Date? = nil, pollAfterSeconds: Int? = nil, cost: Int = 0
    ) {
        self.limit = limit
        self.remaining = remaining
        self.resetAt = resetAt
        self.projectedRemainingAtReset = projectedRemainingAtReset
        self.throttled = throttled
        self.pausedUntil = pausedUntil
        self.pollAfterSeconds = pollAfterSeconds
        self.cost = cost
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        limit = try c.decode(Int.self, forKey: .limit)
        remaining = try c.decode(Int.self, forKey: .remaining)
        resetAt = try Self.time(c, .resetAt)
        projectedRemainingAtReset = try c.decodeIfPresent(Int.self, forKey: .projectedRemainingAtReset)
        throttled = try c.decodeIfPresent(Bool.self, forKey: .throttled) ?? false
        pausedUntil = c.contains(.pausedUntil) ? try Self.time(c, .pausedUntil) : nil
        pollAfterSeconds = try c.decodeIfPresent(Int.self, forKey: .pollAfterSeconds)
        cost = try c.decodeIfPresent(Int.self, forKey: .cost) ?? 0
    }

    /// A Go time under key, refused where it will not parse.
    private static func time(_ c: KeyedDecodingContainer<CodingKeys>, _ key: CodingKeys) throws -> Date {
        let raw = try c.decode(String.self, forKey: key)
        guard let at = PRDetail.parseGoTime(raw) else {
            throw DecodingError.dataCorruptedError(forKey: key, in: c, debugDescription: "unreadable \(key.stringValue): \(raw)")
        }
        return at
    }
}
