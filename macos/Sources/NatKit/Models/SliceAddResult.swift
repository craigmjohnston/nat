import Foundation

/// The slice `nat slice-add --json` reports having created: what `info`
/// reports about a slice, plus the milestone's name and the resolved working
/// directory, which are what the person who just filed it wants confirmed.
public struct SliceAddResult: Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let status: String
    public let milestoneID: String
    public let milestoneName: String
    public let repo: String
    public let url: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case status
        case milestoneID = "milestone_id"
        case milestoneName = "milestone_name"
        case repo
        case url
    }

    public init(
        id: String,
        name: String,
        status: String,
        milestoneID: String,
        milestoneName: String,
        repo: String,
        url: String
    ) {
        self.id = id
        self.name = name
        self.status = status
        self.milestoneID = milestoneID
        self.milestoneName = milestoneName
        self.repo = repo
        self.url = url
    }
}
