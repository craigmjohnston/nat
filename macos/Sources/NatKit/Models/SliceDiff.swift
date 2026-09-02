import Foundation

/// The wire shape of `nat slice-diff <slice> --project <id> --json`: the
/// unified diff of a handed-back branch, already split into files by the Go
/// side (`git.ParseFiles`). Each file's `lines` are its section of the diff
/// exactly as git wrote it — headers, hunk headers, +/-/context lines — since
/// that is the shape `DiffModel` walks to build render-ready rows.
public struct SliceDiff: Codable, Equatable, Sendable {
    public let base: String
    public let branch: String
    public let files: [SliceDiffFile]

    enum CodingKeys: String, CodingKey {
        case base
        case branch
        case files
    }

    public init(base: String, branch: String, files: [SliceDiffFile]) {
        self.base = base
        self.branch = branch
        self.files = files
    }
}

/// One file's section of a unified diff, as `nat slice-diff --json` reports it.
///
/// `oldPath` names where the file came from. It is set to the same value as
/// `path` for a file that is not a rename — created and deleted files included,
/// since git names both sides of its own header even where one side is
/// `/dev/null` — and only differs from `path` on an actual rename, which is
/// what `isRenamed` reads.  The Go side omits the key entirely when it would be
/// empty, which in practice never happens for a real diff; decoding treats a
/// missing key the same as an empty string rather than failing.
public struct SliceDiffFile: Codable, Equatable, Sendable {
    public let path: String
    public let oldPath: String
    public let adds: Int
    public let dels: Int
    public let described: Bool
    public let lines: [String]

    /// Whether the change moved the file, which is the one case where naming
    /// the old path as well says something (mirrors `git.File.Renamed()`).
    public var isRenamed: Bool {
        !oldPath.isEmpty && oldPath != path
    }

    enum CodingKeys: String, CodingKey {
        case path
        case oldPath = "old_path"
        case adds
        case dels
        case described
        case lines
    }

    public init(
        path: String,
        oldPath: String = "",
        adds: Int,
        dels: Int,
        described: Bool,
        lines: [String]
    ) {
        self.path = path
        self.oldPath = oldPath
        self.adds = adds
        self.dels = dels
        self.described = described
        self.lines = lines
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        path = try container.decode(String.self, forKey: .path)
        oldPath = try container.decodeIfPresent(String.self, forKey: .oldPath) ?? ""
        adds = try container.decode(Int.self, forKey: .adds)
        dels = try container.decode(Int.self, forKey: .dels)
        described = try container.decode(Bool.self, forKey: .described)
        lines = try container.decode([String].self, forKey: .lines)
    }
}
