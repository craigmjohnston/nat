import Foundation

/// The slice `nat slice-edit --json` reports having edited: the page it wrote
/// to and the brief that landed on it, so the caller can see exactly what was
/// saved rather than trust the write went through (mirrors
/// `internal/cli/sliceedit.go`'s `sliceEditedJSON`).
public struct SliceEditResult: Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let url: String
    public let brief: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case url
        case brief
    }

    public init(id: String, name: String, url: String = "", brief: String) {
        self.id = id
        self.name = name
        self.url = url
        self.brief = brief
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        url = try container.decodeIfPresent(String.self, forKey: .url) ?? ""
        brief = try container.decode(String.self, forKey: .brief)
    }
}
