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
/// review" and "ready to merge" are a pull request positively read as open,
/// and "unread" is everything else at once — no longer open, or a repository
/// whose listing could not be taken, which the reading deliberately does not
/// tell apart.
public struct PRStatusSlice: Codable, Equatable, Sendable {
    public let sliceID: String
    public let name: String
    public let pr: String
    public let readiness: String

    enum CodingKeys: String, CodingKey {
        case sliceID = "slice_id"
        case name, pr, readiness
    }

    public init(sliceID: String, name: String, pr: String, readiness: String) {
        self.sliceID = sliceID
        self.name = name
        self.pr = pr
        self.readiness = readiness
    }

    /// Whether the reading positively saw this pull request open — the one
    /// fact rail membership rides on.
    public var isOpen: Bool {
        readiness == "awaiting review" || readiness == "ready to merge"
    }
}
