import Foundation

/// What `AppModel` keeps of its workshops between launches: every tab's
/// unlaunched workshop (its pinned row, the brief being typed, an attached
/// plan document) and the brief a launched one was sent, and the Untitled
/// tabs themselves with their workspace ids — so a planning agent left
/// running is found again under the id it was launched by.
///
/// View state, not plan data: nothing here is anything nat knows about.
public struct WorkshopSnapshot: Codable, Equatable, Sendable {
    /// The shape this build writes. A file of any other is read as nothing:
    /// it was written by a build whose fields meant something else.
    public static let currentVersion = 1

    /// One open Untitled tab, in strip order.
    public struct UntitledTab: Codable, Equatable, Sendable {
        public var id: String
        public var workspaceID: String

        public init(id: String, workspaceID: String) {
            self.id = id
            self.workspaceID = workspaceID
        }
    }

    /// One tab's workshop — a project's, or an Untitled tab's.
    public struct Workshop: Codable, Equatable, Sendable {
        public var pinned: Bool
        public var draft: String?
        public var planFile: PlanFile?
        public var request: String?
        /// True once the session's plan has been accepted: an agent found
        /// gone after that has nothing left unsaved, and its workshop goes.
        /// Nil rather than false, so a file written before it reads the same.
        public var accepted: Bool?

        public init(
            pinned: Bool = false, draft: String? = nil, planFile: PlanFile? = nil, request: String? = nil,
            accepted: Bool? = nil
        ) {
            self.pinned = pinned
            self.draft = draft
            self.planFile = planFile
            self.request = request
            self.accepted = accepted
        }
    }

    public var version: Int
    public var untitledTabs: [UntitledTab]
    /// By tab ID.
    public var workshops: [String: Workshop]

    public init(untitledTabs: [UntitledTab] = [], workshops: [String: Workshop] = [:]) {
        self.version = Self.currentVersion
        self.untitledTabs = untitledTabs
        self.workshops = workshops
    }
}

/// Where `AppModel` keeps its `WorkshopSnapshot` between launches — the
/// shape `PlanCaching` and `UsageCaching` have, but synchronous: the last
/// write is made as the app quits, and an `async` one would still be in
/// flight when the process ends.
public protocol WorkshopCaching: Sendable {
    /// The snapshot last written, or nil where there is none — every first
    /// launch, and equally a file that is truncated, hand-edited or of
    /// another build's shape. Never fatal: it restores nothing.
    func read() -> WorkshopSnapshot?

    /// Records the workshops as they stand. A write that fails costs the
    /// next launch its drafts and nothing else, so it is never reported.
    func write(_ snapshot: WorkshopSnapshot)
}

/// `WorkshopCaching` against the app's own Application Support directory:
/// one JSON file, beside `DiskUsageCache`'s.
public struct DiskWorkshopCache: WorkshopCaching {
    /// `<Application Support>/<bundle ID>/workshops.json`.
    public let fileURL: URL

    public init(fileURL: URL) {
        self.fileURL = fileURL
    }

    /// The file at its default location, falling back to the temporary
    /// directory the way `DiskUsageCache.init()` does.
    public init() {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
        self.fileURL = base.appendingPathComponent(DiskPlanCache.bundleID, isDirectory: true)
            .appendingPathComponent("workshops.json", isDirectory: false)
    }

    public func read() -> WorkshopSnapshot? {
        guard let data = try? Data(contentsOf: fileURL),
              let snapshot = try? JSONDecoder().decode(WorkshopSnapshot.self, from: data),
              snapshot.version == WorkshopSnapshot.currentVersion else { return nil }
        return snapshot
    }

    public func write(_ snapshot: WorkshopSnapshot) {
        guard let data = try? JSONEncoder().encode(snapshot) else { return }
        try? FileManager.default.createDirectory(
            at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true
        )
        try? data.write(to: fileURL, options: .atomic)
    }
}

/// `WorkshopCaching` held in memory — what an `AppModel` built without being
/// told otherwise gets, so a test or a gallery story never touches the real
/// Application Support file.
public final class InMemoryWorkshopCache: WorkshopCaching, @unchecked Sendable {
    private let lock = NSLock()
    private var stored: WorkshopSnapshot?
    private var count = 0

    public init(_ snapshot: WorkshopSnapshot? = nil) {
        stored = snapshot
    }

    /// How many writes have been made — for a test of the debounce.
    public var writes: Int {
        lock.withLock { count }
    }

    public func read() -> WorkshopSnapshot? {
        lock.withLock { stored }
    }

    public func write(_ snapshot: WorkshopSnapshot) {
        lock.withLock {
            stored = snapshot
            count += 1
        }
    }
}
