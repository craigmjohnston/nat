import Foundation

/// The wire shape of `nat slice-diff --commits --project <id> --json <slice>`:
/// the branch's own history since the merge base, without diffing any of it —
/// what the sidebar's "All commits" dropdown lists (mirrors
/// `internal/cli/slicecommits.go`'s `commitsDoc`).
public struct SliceCommitsDoc: Codable, Equatable, Sendable {
    public let base: String
    public let branch: String
    public let commits: [SliceCommit]

    public init(base: String, branch: String, commits: [SliceCommit]) {
        self.base = base
        self.branch = branch
        self.commits = commits
    }
}

/// One commit of a branch's history: a hash, a subject line, who wrote it and
/// when — the four fields `git log` is asked for and nothing more (mirrors
/// `internal/git.Commit`/`internal/cli/slicecommits.go`'s `commitJSON`).
public struct SliceCommit: Codable, Equatable, Sendable, Identifiable {
    public let sha: String
    public let subject: String
    public let author: String
    public let date: Date

    public var id: String { sha }

    enum CodingKeys: String, CodingKey {
        case sha, subject, author, date
    }

    public init(sha: String, subject: String, author: String, date: Date) {
        self.sha = sha
        self.subject = subject
        self.author = author
        self.date = date
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        sha = try container.decode(String.self, forKey: .sha)
        subject = try container.decode(String.self, forKey: .subject)
        author = try container.decode(String.self, forKey: .author)
        let raw = try container.decode(String.self, forKey: .date)
        guard let parsed = PRDetail.parseGoTime(raw) else {
            throw DecodingError.dataCorruptedError(forKey: .date, in: container, debugDescription: "unreadable date: \(raw)")
        }
        date = parsed
    }

    /// The commit's own first eight hex characters — long enough to name one
    /// unambiguously in a list, mirrors `internal/cli/slicecommits.go`'s
    /// `shortSHA`.
    public var shortSHA: String {
        sha.count > 8 ? String(sha.prefix(8)) : sha
    }
}
