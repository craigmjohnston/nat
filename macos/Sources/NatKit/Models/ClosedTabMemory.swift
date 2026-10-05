import Foundation

/// The projects whose tabs the user closed, so a close outlasts the launch:
/// `AppModel.start()` leaves them out of the strip it builds from config, and
/// whatever brings one back into the strip forgets it. Held in `UserDefaults`
/// rather than in nat's config, as `MirrorNudgeMemory` is: it is this Mac's
/// own view of which tabs are open, and no part of the plan — the project
/// stays configured for every headless command and agent.
///
/// `init(defaults:)` is the app's; `inMemory()` is what a model built without
/// being told otherwise gets, so a test or a gallery story never writes to the
/// defaults of the machine it happens to run on.
public struct ClosedTabMemory: Sendable {
    /// The `UserDefaults` key the closed project IDs are stored under.
    public static let storageKey = "closedProjectTabs"

    private let read: @Sendable () -> [String]
    private let write: @Sendable ([String]) -> Void

    public init(defaults: UserDefaults = .standard) {
        // `UserDefaults` is documented thread-safe; it is only not declared so.
        nonisolated(unsafe) let defaults = defaults
        read = { defaults.stringArray(forKey: Self.storageKey) ?? [] }
        write = { defaults.set($0, forKey: Self.storageKey) }
    }

    /// A memory that lives only as long as the value does.
    public static func inMemory() -> ClosedTabMemory {
        let box = LockedIDs()
        return ClosedTabMemory(read: { box.get() }, write: { box.set($0) })
    }

    private init(read: @escaping @Sendable () -> [String], write: @escaping @Sendable ([String]) -> Void) {
        self.read = read
        self.write = write
    }

    /// The projects whose tabs are closed.
    public var closed: Set<String> { Set(read()) }

    /// Remember a project's tab as closed. Closing one already closed is nothing.
    public func close(_ projectID: String) {
        write(closed.union([projectID]).sorted())
    }

    /// Forget a project's close — its tab is in the strip again.
    public func reopen(_ projectID: String) {
        guard closed.contains(projectID) else { return }
        write(closed.subtracting([projectID]).sorted())
    }

    /// Keep only the closes of projects still in `configured`: one config no
    /// longer names has no tab to stay out of the strip.
    public func prune(keeping configured: Set<String>) {
        let kept = closed.intersection(configured)
        guard kept != closed else { return }
        write(kept.sorted())
    }
}
