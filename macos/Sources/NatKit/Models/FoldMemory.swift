import Foundation

/// The sidebar's folds — which projects, milestones, folders and sections
/// are open or folded — so the tree comes back as it was left. Held in
/// `UserDefaults`, as `ClosedTabMemory` is and for its reason: it is this
/// Mac's own view state and no part of the plan. One dictionary, fold key →
/// folded, since every key already names its project (`SidebarFolds`).
///
/// `init(defaults:)` is the app's; `inMemory()` is what a model built without
/// being told otherwise gets, so a test or a gallery story never writes to the
/// defaults of the machine it happens to run on.
public struct FoldMemory: Sendable {
    /// The `UserDefaults` key the folds are stored under.
    public static let storageKey = "sidebarFolds"

    private let read: @Sendable () -> [String: Bool]
    private let write: @Sendable ([String: Bool]) -> Void

    public init(defaults: UserDefaults = .standard) {
        // `UserDefaults` is documented thread-safe; it is only not declared so.
        nonisolated(unsafe) let defaults = defaults
        read = { (defaults.dictionary(forKey: Self.storageKey) as? [String: Bool]) ?? [:] }
        write = { defaults.set($0, forKey: Self.storageKey) }
    }

    /// A memory that lives only as long as the value does.
    public static func inMemory() -> FoldMemory {
        let box = LockedFolds()
        return FoldMemory(read: { box.get() }, write: { box.set($0) })
    }

    private init(read: @escaping @Sendable () -> [String: Bool], write: @escaping @Sendable ([String: Bool]) -> Void) {
        self.read = read
        self.write = write
    }

    /// Every fold recorded, by key: true folded, false open.
    public var folds: [String: Bool] { read() }

    /// Record the folds, replacing what was recorded.
    public func save(_ folds: [String: Bool]) {
        write(folds)
    }
}

private final class LockedFolds: @unchecked Sendable {
    private let lock = NSLock()
    private var folds: [String: Bool] = [:]

    func get() -> [String: Bool] { lock.withLock { folds } }
    func set(_ new: [String: Bool]) { lock.withLock { folds = new } }
}
