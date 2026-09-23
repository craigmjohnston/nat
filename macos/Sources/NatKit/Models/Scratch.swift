import Foundation

/// `nat scratch-open --json`'s own reading (mirrors `internal/cli/scratch.go`'s
/// `scratchOpenJSON`): the reserved scratch project's ID, and whether this call
/// is the one that made it.
public struct ScratchOpenResult: Codable, Equatable, Sendable {
    public let id: String
    public let created: Bool

    public init(id: String, created: Bool) {
        self.id = id
        self.created = created
    }
}

/// `nat done-clear --json`'s own reading (mirrors `internal/cli/scratch.go`'s
/// `doneClearJSON`): what was removed, slices and milestones by name and
/// sessions by ID.
public struct DoneClearResult: Codable, Equatable, Sendable {
    public let slices: [String]
    public let sessions: [String]
    public let milestones: [String]

    public init(slices: [String] = [], sessions: [String] = [], milestones: [String] = []) {
        self.slices = slices
        self.sessions = sessions
        self.milestones = milestones
    }
}
