import Foundation

/// One rate-limit window off Claude Code's own statusline: how much of it is
/// used and when it resets, both computed server-side — `nat usage --json`'s
/// `five_hour`/`seven_day` fields.
///
/// `resetsAt` is decoded from the wire's unix-epoch seconds rather than an
/// ISO8601 string, so this carries its own `Codable` conformance rather than
/// leaning on whatever date strategy the caller's `JSONDecoder` happens to be
/// configured with.
public struct UsageRateLimit: Codable, Equatable, Sendable {
    public let usedPercentage: Double
    public let resetsAt: Date

    private enum CodingKeys: String, CodingKey {
        case usedPercentage = "used_percentage"
        case resetsAt = "resets_at"
    }

    public init(usedPercentage: Double, resetsAt: Date) {
        self.usedPercentage = usedPercentage
        self.resetsAt = resetsAt
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        usedPercentage = try container.decode(Double.self, forKey: .usedPercentage)
        let epochSeconds = try container.decode(Double.self, forKey: .resetsAt)
        resetsAt = Date(timeIntervalSince1970: epochSeconds)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(usedPercentage, forKey: .usedPercentage)
        try container.encode(resetsAt.timeIntervalSince1970, forKey: .resetsAt)
    }
}

/// What one usage probe read: the two rate-limit windows a Pro/Max
/// subscriber's statusline can carry, each independently absent — never
/// zero, only unknown — when the account has no such window at all, or the
/// probe never landed one.
public struct UsageReading: Codable, Equatable, Sendable {
    public let fiveHour: UsageRateLimit?
    public let sevenDay: UsageRateLimit?

    private enum CodingKeys: String, CodingKey {
        case fiveHour = "five_hour"
        case sevenDay = "seven_day"
    }

    public init(fiveHour: UsageRateLimit? = nil, sevenDay: UsageRateLimit? = nil) {
        self.fiveHour = fiveHour
        self.sevenDay = sevenDay
    }

    /// Neither window read — a probe that has not run yet, one that failed
    /// or timed out, or an account with no rate-limit state to report at
    /// all. The readout draws nothing for it.
    public static let empty = UsageReading()

    public var isEmpty: Bool { fiveHour == nil && sevenDay == nil }
}
