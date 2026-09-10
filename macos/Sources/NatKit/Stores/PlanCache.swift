import Foundation

/// The last plan each project was successfully read as, kept on disk so a
/// launch draws the board from it while the fresh read is still in flight —
/// the "start from what we had" half of loading calmly, the other half being
/// `LoadState`'s own refusal to blank what it is showing.
///
/// Plan data and nothing else: a `ProjectInfo` is what `nat info` printed,
/// which is the project's name, its conventions and its slices. Nothing here
/// ever sees the Notion token or anything else out of the keychain, and
/// nothing that does may be added.
public protocol PlanCaching: Sendable {
    /// The plan last written for this project, or nil where there is none —
    /// which is every first launch, and equally a file that has been
    /// truncated, hand-edited or written by a build whose `ProjectInfo` was
    /// a different shape. All of those degrade to the cold load that was the
    /// only load before there was a cache.
    func read(projectID: String) async -> ProjectInfo?

    /// Records a plan that has just landed. Fire-and-forget by contract: a
    /// write that fails costs the next launch its head start and nothing
    /// else, so it is never reported to the caller.
    func write(_ info: ProjectInfo, projectID: String) async
}

/// `PlanCaching` against the app's own Application Support directory: one
/// JSON file per project, named by its page ID.
///
/// Its methods are `nonisolated async`, so the file system work runs off the
/// main actor even though every caller is on it — the store reads this before
/// its first paint and writes it after every read that lands, and neither is
/// worth a frame.
public struct DiskPlanCache: PlanCaching {
    /// The bundle identifier the app ships under, appended by hand rather
    /// than left to the file manager: an unsandboxed process is handed the
    /// bare `~/Library/Application Support`, so the bundled app and the bare
    /// executable `make run` starts would otherwise disagree about where the
    /// cache lives.
    public static let bundleID = "com.craigmjohnston.nat.NatApp"

    /// The directory the files sit in — `<Application Support>/<bundle
    /// ID>/plans`.
    public let directory: URL

    /// The default location, or nil in the impossible case of a home
    /// directory with no Application Support to speak of — see
    /// `DiskPlanCache.init()`.
    public static var defaultDirectory: URL? {
        guard let base = FileManager.default.urls(
            for: .applicationSupportDirectory, in: .userDomainMask
        ).first else { return nil }
        return base.appendingPathComponent(bundleID, isDirectory: true)
            .appendingPathComponent("plans", isDirectory: true)
    }

    public init(directory: URL) {
        self.directory = directory
    }

    /// The cache at its default location. A machine that will not name one
    /// falls back to the temporary directory rather than refusing to have a
    /// cache: what is at stake is one launch's head start, and a cache that
    /// does not survive a reboot still serves every launch between them.
    public init() {
        self.directory = Self.defaultDirectory
            ?? URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
                .appendingPathComponent(Self.bundleID, isDirectory: true)
                .appendingPathComponent("plans", isDirectory: true)
    }

    /// Where one project's plan is filed. The ID is slugged rather than
    /// trusted: it comes out of a config file a person can edit, and a `/` in
    /// it would otherwise name a directory nobody meant.
    public func fileURL(projectID: String) -> URL {
        directory.appendingPathComponent(Self.fileSlug(projectID) + ".json", isDirectory: false)
    }

    /// Every run of anything but a letter, a digit, a hyphen or an underscore
    /// collapsed to one hyphen — the same shape `worktree.pathSlug` makes on
    /// the Go side, and enough to keep a page ID (hex and dashes) as it is.
    static func fileSlug(_ id: String) -> String {
        var out = ""
        var pendingHyphen = false
        for ch in id {
            if ch.isLetter || ch.isNumber || ch == "-" || ch == "_" {
                if pendingHyphen { out.append("-"); pendingHyphen = false }
                out.append(ch)
            } else if !out.isEmpty {
                pendingHyphen = true
            }
        }
        return out.isEmpty ? "project" : out
    }

    public func read(projectID: String) async -> ProjectInfo? {
        guard let data = try? Data(contentsOf: fileURL(projectID: projectID)) else { return nil }
        return try? JSONDecoder().decode(ProjectInfo.self, from: data)
    }

    public func write(_ info: ProjectInfo, projectID: String) async {
        guard let data = try? JSONEncoder().encode(info) else { return }
        try? FileManager.default.createDirectory(
            at: directory, withIntermediateDirectories: true
        )
        // Atomic, so a launch reading this file while a refresh writes it
        // sees one whole plan or the other rather than half of each.
        try? data.write(to: fileURL(projectID: projectID), options: .atomic)
    }
}
