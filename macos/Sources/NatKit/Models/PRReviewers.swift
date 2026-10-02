import Foundation

/// The wire shape of `nat pr-reviewers --json`: who is asked to review a
/// slice's pull request (read back from GitHub after any edit), who else
/// could be — the repository's collaborators bar the author and those
/// already asked — and, when that listing failed, why. A failed listing is
/// no news about who could review (`candidatesError`), never "nobody".
public struct PRReviewers: Codable, Equatable, Sendable {
    public let pr: String
    public let requested: [String]
    public let candidates: [String]
    public let candidatesError: String?

    enum CodingKeys: String, CodingKey {
        case pr, requested, candidates
        case candidatesError = "candidates_error"
    }

    public init(pr: String, requested: [String], candidates: [String], candidatesError: String? = nil) {
        self.pr = pr
        self.requested = requested
        self.candidates = candidates
        self.candidatesError = candidatesError
    }
}
