import Foundation

/// Which piece of the diff is at the top of the column, kept so a width change
/// (a divider drag, a window resize) can put the same code back at the top
/// after the rows re-wrap. Rows report their frames in the scroll view's own
/// coordinate space; the tracker keeps the topmost one still on screen.
///
/// Updates are ignored while a restore is pending: the reports that follow a
/// width change describe the re-wrapped layout at the *old* offset — different
/// code — and would overwrite the very anchor being restored.
public struct DiffScrollAnchor: Equatable, Sendable {
    /// A row's identity across files: the row id alone repeats between files.
    public struct Key: Hashable, Sendable {
        public let path: String
        public let rowID: String

        public init(path: String, rowID: String) {
            self.path = path
            self.rowID = rowID
        }

        /// The scroll-target id a row is tagged with. The NUL keeps it from
        /// ever equalling a bare file path, which is the file boxes' own id.
        public var scrollID: String { "\(path)\u{0}\(rowID)" }
    }

    public private(set) var key: Key?
    public private(set) var isRestoring = false
    /// Whether the content is taller than the viewport; a diff that fits has
    /// nowhere to scroll, so restoring one could only jump it wrongly.
    public var canScroll = false

    public init() {}

    /// Takes the visible rows' frames (`minY`/`maxY` in viewport coordinates,
    /// 0 being the top edge) and keeps the topmost one that reaches the
    /// viewport. A row wholly above the top is not on screen.
    public mutating func update(rows: [(key: Key, minY: Double, maxY: Double)]) {
        guard !isRestoring else { return }
        key = rows.filter { $0.maxY > 0 }.min { $0.minY < $1.minY }?.key
    }

    /// A width change: freezes the anchor and returns what to scroll back to,
    /// or nil when there is nothing to restore (no anchor, or nothing to
    /// scroll), in which case nothing is frozen.
    public mutating func beginRestore() -> Key? {
        guard canScroll, let key else { return nil }
        isRestoring = true
        return key
    }

    /// The scroll has been put back; row reports count again.
    public mutating func endRestore() {
        isRestoring = false
    }
}
