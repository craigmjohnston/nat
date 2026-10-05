import Foundation

/// A badge on something the user has not seen as it now is: `new` where it
/// was not there when they last looked, `updated` where it was and has
/// changed since. One rule for every section that wears one — a Changes
/// file, a Visual changes image, the PR section (`SeenMemory`).
public enum SeenBadge: Equatable, Sendable {
    case new, updated
}

/// The sections whose contents the user is told are new or updated, each
/// remembered apart under the one store.
public enum SeenSection: String, CaseIterable, Sendable {
    /// Changes: a file of the branch's diff, by path, at a fingerprint of
    /// its rows (`DiffFileModel.seenFingerprint`).
    case changes
    /// Visual changes: an image, by name, at its `VisualChange.identity`.
    case visuals
    /// PR: the pull request, as one item, at its head commit.
    case pr
}

/// What the user last saw of each slice's sections — what puts New and
/// Updated badges on, and takes them off again. Each section of a slice is
/// remembered as a snapshot, item → fingerprint, of what was there when the
/// user last looked:
///
/// - never looked at (no snapshot): nothing is badged, and the first look
///   (`baseline`) records everything as seen — a slice opened for the first
///   time has nothing "new" about it;
/// - an item the snapshot does not hold is New, one it holds at another
///   fingerprint is Updated (`seenBadge`);
/// - looking at an item (`markSeen`) records it as it is now, taking its
///   badge off.
///
/// Held in `UserDefaults`, as `MirrorNudgeMemory` is and for its reason: it is
/// this Mac's own view state and no part of the plan, and it survives a
/// relaunch. `init(defaults:)` is the app's; `inMemory()` is what a model built
/// without being told otherwise gets, so a test or a gallery story never
/// writes to the defaults of the machine it happens to run on.
public struct SeenMemory: Sendable {
    /// The `UserDefaults` key the snapshots are stored under: one dictionary
    /// per project, slice and section.
    public static let storageKey = "seenSnapshots"

    private let read: @Sendable () -> [String: [String: String]]
    private let write: @Sendable ([String: [String: String]]) -> Void

    public init(defaults: UserDefaults = .standard) {
        // `UserDefaults` is documented thread-safe; it is only not declared so.
        nonisolated(unsafe) let defaults = defaults
        read = { (defaults.dictionary(forKey: Self.storageKey) as? [String: [String: String]]) ?? [:] }
        write = { defaults.set($0, forKey: Self.storageKey) }
    }

    /// A memory that lives only as long as the value does.
    public static func inMemory() -> SeenMemory {
        let box = LockedSnapshots()
        return SeenMemory(read: { box.get() }, write: { box.set($0) })
    }

    private init(
        read: @escaping @Sendable () -> [String: [String: String]],
        write: @escaping @Sendable ([String: [String: String]]) -> Void
    ) {
        self.read = read
        self.write = write
    }

    /// What the user last saw of a slice's section, item → fingerprint; nil
    /// where they have never looked at it.
    public func snapshot(projectID: String, sliceID: String, _ section: SeenSection) -> [String: String]? {
        read()[Self.key(projectID: projectID, sliceID: sliceID, section)]
    }

    /// The first look at a slice's section: every item recorded as seen as it
    /// is — and nothing written where the section was looked at before.
    public func baseline(projectID: String, sliceID: String, _ section: SeenSection, _ items: [String: String]) {
        let key = Self.key(projectID: projectID, sliceID: sliceID, section)
        var all = read()
        guard all[key] == nil else { return }
        all[key] = items
        write(all)
    }

    /// The user has seen an item as it now is. Writes nothing where that is
    /// already what the snapshot holds — or where the section has never been
    /// looked at, whose first look is `baseline`'s.
    public func markSeen(projectID: String, sliceID: String, _ section: SeenSection, item: String, fingerprint: String) {
        let key = Self.key(projectID: projectID, sliceID: sliceID, section)
        var all = read()
        guard var snapshot = all[key], snapshot[item] != fingerprint else { return }
        snapshot[item] = fingerprint
        all[key] = snapshot
        write(all)
    }

    /// Forget what a section's snapshot holds of anything no longer there,
    /// so the memory keeps no more than a slice's current items — one that
    /// comes back later is New again. Writes nothing where nothing is dropped.
    public func retain(projectID: String, sliceID: String, _ section: SeenSection, items: Set<String>) {
        let key = Self.key(projectID: projectID, sliceID: sliceID, section)
        var all = read()
        guard let snapshot = all[key] else { return }
        let kept = snapshot.filter { items.contains($0.key) }
        guard kept.count != snapshot.count else { return }
        all[key] = kept
        write(all)
    }

    /// An item's badge against what the user last saw of its section.
    public func badge(
        projectID: String, sliceID: String, _ section: SeenSection, item: String, fingerprint: String
    ) -> SeenBadge? {
        seenBadge(snapshot: snapshot(projectID: projectID, sliceID: sliceID, section), item: item, fingerprint: fingerprint)
    }

    private static func key(projectID: String, sliceID: String, _ section: SeenSection) -> String {
        "\(projectID)\u{1F}\(sliceID)\u{1F}\(section.rawValue)"
    }
}

/// An item's badge against a section's snapshot: none where the section was
/// never looked at, New where the snapshot does not hold the item, Updated
/// where it holds it at another fingerprint, none where it is as seen.
public func seenBadge(snapshot: [String: String]?, item: String, fingerprint: String) -> SeenBadge? {
    guard let snapshot else { return nil }
    guard let seen = snapshot[item] else { return .new }
    return seen == fingerprint ? nil : .updated
}

private final class LockedSnapshots: @unchecked Sendable {
    private let lock = NSLock()
    private var snapshots: [String: [String: String]] = [:]

    func get() -> [String: [String: String]] { lock.withLock { snapshots } }
    func set(_ new: [String: [String: String]]) { lock.withLock { snapshots = new } }
}
