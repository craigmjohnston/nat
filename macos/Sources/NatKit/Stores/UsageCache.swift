import Foundation

/// Where `UsageStore` keeps the last reading between launches, so a launch
/// shows last-known usage immediately while the fresh probe is still in
/// flight — the same cache-then-refresh shape `PlanCaching` gives a
/// project's plan.
///
/// One reading, not one per project: usage is a property of the logged-in
/// Claude account, which every project shares.
public protocol UsageCaching: Sendable {
    /// The reading last written, or nil where there is none — every first
    /// launch, and equally a file that has been truncated, hand-edited or
    /// written by a build whose `UsageReading` was a different shape.
    func read() async -> UsageReading?

    /// Records a reading that has just landed. Fire-and-forget by contract:
    /// a write that fails costs only the next launch's head start.
    func write(_ reading: UsageReading) async
}

/// `UsageCaching` against the app's own Application Support directory: one
/// JSON file, beside `DiskPlanCache`'s own `plans` directory.
public struct DiskUsageCache: UsageCaching {
    /// `<Application Support>/<bundle ID>/usage.json`.
    public let fileURL: URL

    /// The default location, or nil in the impossible case of a home
    /// directory with no Application Support to speak of — see
    /// `DiskUsageCache.init()`.
    public static var defaultFileURL: URL? {
        guard let base = FileManager.default.urls(
            for: .applicationSupportDirectory, in: .userDomainMask
        ).first else { return nil }
        return base.appendingPathComponent(DiskPlanCache.bundleID, isDirectory: true)
            .appendingPathComponent("usage.json", isDirectory: false)
    }

    public init(fileURL: URL) {
        self.fileURL = fileURL
    }

    /// The cache at its default location, falling back to the temporary
    /// directory the same way `DiskPlanCache.init()` does: what is at stake
    /// is one launch's head start, not correctness.
    public init() {
        self.fileURL = Self.defaultFileURL
            ?? URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
                .appendingPathComponent(DiskPlanCache.bundleID, isDirectory: true)
                .appendingPathComponent("usage.json", isDirectory: false)
    }

    public func read() async -> UsageReading? {
        guard let data = try? Data(contentsOf: fileURL) else { return nil }
        return try? JSONDecoder().decode(UsageReading.self, from: data)
    }

    public func write(_ reading: UsageReading) async {
        guard let data = try? JSONEncoder().encode(reading) else { return }
        try? FileManager.default.createDirectory(
            at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true
        )
        try? data.write(to: fileURL, options: .atomic)
    }
}
