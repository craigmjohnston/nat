import Foundation

/// What `nat slice-checks-rerun` re-runs: every Actions run whole, each run's
/// failed jobs, or named checks' jobs.
public enum ChecksRerunMode: Equatable, Sendable {
    case all
    case failed
    case checks([String])
}

/// The wire shape of `nat slice-checks-rerun --json` and `nat
/// slice-checks-cancel --json` (mirrors `internal/cli/slicechecksrerun.go`'s
/// `checksActionDoc`): the checks cancelled — those asked for and the
/// siblings stopped with them — the checks re-run, and the checks skipped
/// for having no Actions run behind them. `rerun` is absent from a cancel.
public struct ChecksActionResult: Codable, Equatable, Sendable {
    public let cancelled: [String]
    public let rerun: [String]
    public let skipped: [String]

    enum CodingKeys: String, CodingKey {
        case cancelled, rerun, skipped
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        cancelled = try c.decodeIfPresent([String].self, forKey: .cancelled) ?? []
        rerun = try c.decodeIfPresent([String].self, forKey: .rerun) ?? []
        skipped = try c.decodeIfPresent([String].self, forKey: .skipped) ?? []
    }

    public init(cancelled: [String] = [], rerun: [String] = [], skipped: [String] = []) {
        self.cancelled = cancelled
        self.rerun = rerun
        self.skipped = skipped
    }
}
