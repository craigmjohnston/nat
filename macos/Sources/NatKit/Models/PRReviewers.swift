import Foundation

/// The wire shape of `nat pr-reviewers --json`. A read says who is asked to
/// review a slice's pull request, who else could be — the repository's
/// collaborators bar the author and those already asked — and, when that
/// listing failed, why: a failed listing is no news about who could review
/// (`candidatesError`), never "nobody". An edit says only what it did —
/// `added` and `removed` — and reads nothing back, so its `requested` and
/// `candidates` are empty: the next reading of the pull request says who is
/// asked now.
public struct PRReviewers: Codable, Equatable, Sendable {
    public let pr: String
    public let requested: [String]
    public let candidates: [String]
    public let candidatesError: String?
    public let added: [String]
    public let removed: [String]

    enum CodingKeys: String, CodingKey {
        case pr, requested, candidates, added, removed
        case candidatesError = "candidates_error"
    }

    public init(
        pr: String, requested: [String] = [], candidates: [String] = [], candidatesError: String? = nil,
        added: [String] = [], removed: [String] = []
    ) {
        self.pr = pr
        self.requested = requested
        self.candidates = candidates
        self.candidatesError = candidatesError
        self.added = added
        self.removed = removed
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        pr = try c.decode(String.self, forKey: .pr)
        requested = try c.decodeIfPresent([String].self, forKey: .requested) ?? []
        candidates = try c.decodeIfPresent([String].self, forKey: .candidates) ?? []
        candidatesError = try c.decodeIfPresent(String.self, forKey: .candidatesError)
        added = try c.decodeIfPresent([String].self, forKey: .added) ?? []
        removed = try c.decodeIfPresent([String].self, forKey: .removed) ?? []
    }
}
