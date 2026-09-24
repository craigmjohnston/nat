import Foundation

/// Which projects still owe the user the "Mirror this plan to Notion?" card:
/// armed when a plan is accepted into a new local project, and disarmed for
/// good by the card's ✕ or by the project mirroring. Held in `UserDefaults`
/// rather than in nat's config, for the reason the theme is: it is this Mac's
/// own view state and no part of the plan — and it survives a relaunch, which
/// is what "dismissing is remembered" means.
///
/// A project that was never armed — one opened from a folder, one the user
/// made some other way, every project that existed before this card did —
/// never shows it.
///
/// `init(defaults:)` is the app's; `inMemory()` is what a model built without
/// being told otherwise gets, so a test or a gallery story never writes to the
/// defaults of the machine it happens to run on.
public struct MirrorNudgeMemory: Sendable {
    /// The `UserDefaults` key the armed project IDs are stored under.
    public static let storageKey = "mirrorNudgePending"

    private let read: @Sendable () -> [String]
    private let write: @Sendable ([String]) -> Void

    public init(defaults: UserDefaults = .standard) {
        // `UserDefaults` is documented thread-safe; it is only not declared so.
        nonisolated(unsafe) let defaults = defaults
        read = { defaults.stringArray(forKey: Self.storageKey) ?? [] }
        write = { defaults.set($0, forKey: Self.storageKey) }
    }

    /// A memory that lives only as long as the value does.
    public static func inMemory() -> MirrorNudgeMemory {
        let box = LockedIDs()
        return MirrorNudgeMemory(read: { box.get() }, write: { box.set($0) })
    }

    private init(read: @escaping @Sendable () -> [String], write: @escaping @Sendable ([String]) -> Void) {
        self.read = read
        self.write = write
    }

    /// The projects still to be asked.
    public var pending: Set<String> { Set(read()) }

    public func isPending(_ projectID: String) -> Bool {
        pending.contains(projectID)
    }

    /// Owe a project the card. Arming one already armed is nothing.
    public func arm(_ projectID: String) {
        write(pending.union([projectID]).sorted())
    }

    /// Never ask about a project again — the ✕, or a mirror that took.
    public func disarm(_ projectID: String) {
        write(pending.subtracting([projectID]).sorted())
    }
}

private final class LockedIDs: @unchecked Sendable {
    private let lock = NSLock()
    private var ids: [String] = []

    func get() -> [String] { lock.withLock { ids } }
    func set(_ new: [String]) { lock.withLock { ids = new } }
}
