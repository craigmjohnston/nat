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

/// One section of a properties rail: the heading it is drawn under, which is
/// a fixed label in either rail and so is drawn as itself rather than stood
/// in for, and the widths of the rows being read into it, which are not.
public struct SkeletonRailSectionShape: Equatable, Sendable {
    public let title: String
    public let rows: SkeletonLines

    public init(title: String, rows: SkeletonLines) {
        self.title = title
        self.rows = rows
    }
}

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

    /// The properties rail's sections, each its own heading — the same four
    /// `BriefTabView` draws whatever the slice turns out to be — and the
    /// width of the value being read into it.
    public static let sidebarSections: [SkeletonRailSectionShape] = [
        SkeletonRailSectionShape(title: "STATUS", rows: [0.52]),
        SkeletonRailSectionShape(title: "MILESTONE", rows: [0.74]),
        SkeletonRailSectionShape(title: "BRANCH", rows: [0.86]),
        SkeletonRailSectionShape(title: "DEPENDS ON", rows: [0.63])
    ]

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

    /// How many digits the gutter is drawn at — what `DiffModel.numberWidth`
    /// comes to for a file of a few hundred lines, which is the ordinary
    /// case. It is the one number here a reading actually replaces: the
    /// gutter is as wide as the longest line number in the diff, so a branch
    /// touching a very long file widens it as it lands.
    public static let numberWidth = 3

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

    /// The rail's sections, each its own heading and the widths of the rows
    /// being read into it. `CHECKS` grows a count beside it once the checks
    /// are in — the word itself is there either way, and it is leading, so
    /// nothing under it moves when the count arrives.
    public static let sidebarSections: [SkeletonRailSectionShape] = [
        SkeletonRailSectionShape(title: "CHECKS", rows: [0.88, 0.72, 0.80]),
        SkeletonRailSectionShape(title: "REVIEW", rows: [0.66, 0.52]),
        SkeletonRailSectionShape(title: "CHANGES", rows: [0.58, 0.45])
    ]

    public static let accessibilityLabel = "Reading the pull request…"
}

/// The Agent stage while a launch is in flight and no session has appeared
/// yet: a few lines of terminal output being read in, on the terminal's own
/// surface.
public enum AgentSkeleton {
    public static let lines: SkeletonLines = [0.42, 0.68, 0.55, 0.31, 0.74, 0.48]

    public static let accessibilityLabel = "Starting the agent…"
}
