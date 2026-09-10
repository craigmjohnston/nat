import Foundation

/// One row of the rail's skeleton: which of the rail's three row shapes it
/// stands in for, how deep in the tree it sits, and how much of the rail's
/// width its title block takes.
public struct RailSkeletonRow: Equatable, Sendable {
    public enum Kind: Hashable, Sendable {
        /// A section heading — TODO, ACTIVE — with its icon in the slot.
        case heading
        /// A milestone folder.
        case folder
        /// A slice under one.
        case slice
    }

    public let kind: Kind
    /// Tree depth, in `RailSlot.indent` steps: a heading and a folder sit at
    /// 0 and a slice under a folder at 1, exactly as the real rows do.
    public let depth: Int
    /// The title block's width as a fraction of the rail's own width.
    public let titleWidth: Double

    public init(kind: Kind, depth: Int, titleWidth: Double) {
        self.kind = kind
        self.depth = depth
        self.titleWidth = titleWidth
    }
}

/// The shape the rail draws while its first plan is still being read: a
/// heading, a couple of open milestone folders and the slices under them —
/// the shape nearly every plan lands in, so what arrives replaces it rather
/// than pushing it out of the way.
///
/// Fixed rather than random: the rail redraws for every hover and every
/// window resize, and widths rolled afresh each time would have the whole
/// column twitching.
public enum RailSkeleton {
    public static let rows: [RailSkeletonRow] = [
        RailSkeletonRow(kind: .heading, depth: 0, titleWidth: 0.22),
        RailSkeletonRow(kind: .folder, depth: 0, titleWidth: 0.58),
        RailSkeletonRow(kind: .slice, depth: 1, titleWidth: 0.62),
        RailSkeletonRow(kind: .slice, depth: 1, titleWidth: 0.44),
        RailSkeletonRow(kind: .slice, depth: 1, titleWidth: 0.55),
        RailSkeletonRow(kind: .folder, depth: 0, titleWidth: 0.47),
        RailSkeletonRow(kind: .slice, depth: 1, titleWidth: 0.58),
        RailSkeletonRow(kind: .slice, depth: 1, titleWidth: 0.40),
        RailSkeletonRow(kind: .folder, depth: 0, titleWidth: 0.52)
    ]

    /// What a screen reader is told while these are up, since the blocks
    /// themselves say nothing.
    public static let accessibilityLabel = "Loading the plan…"
}
