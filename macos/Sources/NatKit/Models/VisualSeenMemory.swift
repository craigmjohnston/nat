import Foundation

/// Which handed-in images the user has seen — what takes an image's New badge
/// away. An image is seen at its identity (`VisualChange.identity`: its name,
/// its image's URI and hash and its before's), so a later hand-in that
/// re-renders it is new again. Held in `UserDefaults`, as `MirrorNudgeMemory`
/// is and for its reason: it is this Mac's own view state and no part of the
/// plan, and it survives a relaunch.
///
/// `init(defaults:)` is the app's; `inMemory()` is what a model built without
/// being told otherwise gets, so a test or a gallery story never writes to the
/// defaults of the machine it happens to run on.
public struct VisualSeenMemory: Sendable {
    /// The `UserDefaults` key the seen marks are stored under.
    public static let storageKey = "visualsSeen"

    private let read: @Sendable () -> [String]
    private let write: @Sendable ([String]) -> Void

    public init(defaults: UserDefaults = .standard) {
        // `UserDefaults` is documented thread-safe; it is only not declared so.
        nonisolated(unsafe) let defaults = defaults
        read = { defaults.stringArray(forKey: Self.storageKey) ?? [] }
        write = { defaults.set($0, forKey: Self.storageKey) }
    }

    /// A memory that lives only as long as the value does.
    public static func inMemory() -> VisualSeenMemory {
        let box = LockedSeen()
        return VisualSeenMemory(read: { box.get() }, write: { box.set($0) })
    }

    private init(read: @escaping @Sendable () -> [String], write: @escaping @Sendable ([String]) -> Void) {
        self.read = read
        self.write = write
    }

    public func isSeen(projectID: String, sliceID: String, _ visual: VisualChange) -> Bool {
        read().contains(Self.mark(projectID: projectID, sliceID: sliceID, visual))
    }

    /// Mark an image seen. Marking one already seen writes nothing.
    public func markSeen(projectID: String, sliceID: String, _ visual: VisualChange) {
        let mark = Self.mark(projectID: projectID, sliceID: sliceID, visual)
        var marks = read()
        guard !marks.contains(mark) else { return }
        marks.append(mark)
        write(marks)
    }

    /// Forget a slice's marks on anything its hand-in no longer carries, so
    /// the memory holds no more than a slice's current images. Writes nothing
    /// where nothing is dropped.
    public func retain(projectID: String, sliceID: String, _ visuals: [VisualChange]) {
        let prefix = Self.prefix(projectID: projectID, sliceID: sliceID)
        let current = Set(visuals.map { Self.mark(projectID: projectID, sliceID: sliceID, $0) })
        let marks = read()
        let kept = marks.filter { !$0.hasPrefix(prefix) || current.contains($0) }
        if kept.count != marks.count { write(kept) }
    }

    private static func prefix(projectID: String, sliceID: String) -> String {
        "\(projectID)\u{1F}\(sliceID)\u{1F}"
    }

    private static func mark(projectID: String, sliceID: String, _ visual: VisualChange) -> String {
        prefix(projectID: projectID, sliceID: sliceID) + visual.identity
    }
}

private final class LockedSeen: @unchecked Sendable {
    private let lock = NSLock()
    private var marks: [String] = []

    func get() -> [String] { lock.withLock { marks } }
    func set(_ new: [String]) { lock.withLock { marks = new } }
}
