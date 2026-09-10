import Foundation

/// The shapes the three content panes draw while their first read is still
/// in flight — the pane counterpart of `RailSkeleton`, and the same bargain:
/// placeholder blocks laid out where the content will be, so the arriving
/// brief, diff or pull request lands on a layout that is already correct and
/// nothing shifts under it.
///
/// Fixed rather than rolled, for the reason `RailSkeleton`'s widths are: a
/// pane redraws for every hover and every resize, and widths rolled afresh
/// each time would have the whole column twitching all through the load.
///
/// They are shapes rather than views so they can be read — and tested —
/// without mounting anything; `BriefSkeletonView`, `DiffSkeletonView` and
/// `PRSkeletonView` are what draw them at their pane's own geometry.

/// A run of placeholder lines, each as a fraction of the column it is drawn
/// in — one paragraph of prose, one comment's body, one sidebar section's
/// values.
public typealias SkeletonLines = [Double]

/// The Brief tab while a slice's detail is being read for the first time:
/// the brief card's prose, and the properties rail beside it — which is
/// drawn only once there is a detail to read it off, so a skeleton that
/// omitted it would have the reading column narrow the moment the brief
/// landed.
public enum BriefSkeleton {
    /// The brief itself: two paragraphs, the second shorter, ending on a
    /// part-line the way prose does.
    public static let paragraphs: [SkeletonLines] = [
        [0.96, 0.99, 0.92, 0.97, 0.58],
        [0.98, 0.94, 0.71]
    ]

    /// The properties rail's sections — `STATUS`, `MILESTONE`, `BRANCH`,
    /// `DEPENDS ON` — as the width of each one's value line. The heading
    /// above each is drawn at a width of its own by the view, since every
    /// section's heading is the same short all-caps run.
    public static let sidebarSections: SkeletonLines = [0.52, 0.74, 0.86, 0.63]

    /// What a screen reader is told while the blocks are up, since they say
    /// nothing themselves.
    public static let accessibilityLabel = "Loading the brief…"
}

/// One file box of the diff skeleton: how wide its path reads and the widths
/// of the code lines inside it.
public struct DiffSkeletonFile: Equatable, Sendable {
    public let pathWidth: Double
    public let rows: SkeletonLines

    public init(pathWidth: Double, rows: SkeletonLines) {
        self.pathWidth = pathWidth
        self.rows = rows
    }
}

/// The Diff tab while a branch is being read for the first time: a couple of
/// file boxes at the real box geometry, and the file list beside them.
public enum DiffSkeleton {
    public static let files: [DiffSkeletonFile] = [
        DiffSkeletonFile(pathWidth: 0.34, rows: [0.62, 0.44, 0.71, 0.38, 0.55, 0.67]),
        DiffSkeletonFile(pathWidth: 0.27, rows: [0.48, 0.66, 0.35, 0.59])
    ]

    /// The file list's own rows — one per file the branch touches, of which
    /// the boxes above are only the first few, so the sidebar reads as the
    /// longer list it always is.
    public static let sidebarRows: SkeletonLines = [0.78, 0.64, 0.83, 0.55, 0.72, 0.60]

    public static let accessibilityLabel = "Reading the diff…"
}

/// The PR tab while a pull request is being read for the first time: the
/// title and branch line, the description, a conversation, and the
/// checks/review/changes rail beside them.
public enum PRSkeleton {
    /// The title's own block, wide enough to read as a sentence.
    public static let titleWidth: Double = 0.62

    /// `head → base`, which is short whatever the branches are called.
    public static let branchLineWidth: Double = 0.38

    public static let descriptionLines: SkeletonLines = [0.95, 0.98, 0.88, 0.61]

    /// Two entries, each a byline the view sizes itself and the body lines
    /// under it.
    public static let conversationEntries: [SkeletonLines] = [
        [0.92, 0.74],
        [0.86, 0.95, 0.52]
    ]

    /// The rail's sections — checks, review, changes — as the width of each
    /// one's rows.
    public static let sidebarSections: [SkeletonLines] = [
        [0.88, 0.72, 0.80],
        [0.66],
        [0.58, 0.45]
    ]

    public static let accessibilityLabel = "Reading the pull request…"
}
