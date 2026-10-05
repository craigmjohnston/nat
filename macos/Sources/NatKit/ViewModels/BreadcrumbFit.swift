import Foundation

/// One crumb's width as drawn whole: `group` is everything the crumb takes in
/// the row (glyph, words, the slash after it), `text` the words alone — the
/// part an ellipsis shortens.
public struct CrumbWidth: Equatable, Sendable {
    public let group: Double
    public let text: Double

    public init(group: Double, text: Double) {
        self.group = group
        self.text = text
    }

    /// The part that never shortens: glyph, gaps, slash.
    var fixed: Double { max(0, group - text) }
}

/// How the titlebar's breadcrumb gives way as its room runs out. The
/// selection's name comes first; the rest degrade, in this order, while that
/// is still feasible:
///
/// 1. the selection's name ellipsizes, down to 80% of it shown;
/// 2. the crumb naming the project turns into the project's tag;
/// 3. the milestone (or container) ellipsizes, down to 50% of it shown, the
///    name held at 80%;
/// 4. past that, the breadcrumb goes and the selection is drawn as its Active
///    row draws it — state dot, project tag, name — which alone ellipsizes.
///
/// At each stage the selection's name takes all the room it can, up to its
/// whole width, before anything else gives way.
public struct BreadcrumbFit: Equatable, Sendable {
    public enum Stage: Equatable, Sendable {
        /// Every crumb in full; only the name may be shortened.
        case full
        /// The project crumb drawn as its tag.
        case projectTag
        /// The milestone crumb shortened as well.
        case parentShortened
        /// No breadcrumb: the Active row's dot, tag and name alone.
        case minimal
    }

    /// The least of the name shown before the project crumb gives way.
    public static let titleFloor = 0.8
    /// The least of the milestone shown before the breadcrumb goes.
    public static let parentFloor = 0.5

    public let stage: Stage
    /// The most the selection's crumb may take, glyphs and all; nil for no
    /// limit (the minimal stage, which shortens on its own).
    public let titleWidth: Double?
    /// The most the milestone crumb may take; nil for its whole width.
    public let parentWidth: Double?
    /// Whether the project crumb is drawn as the project's tag — from the
    /// second stage on, where the project has one.
    public let projectAsTag: Bool

    /// - Parameters:
    ///   - available: the breadcrumb's room.
    ///   - spacing: the gap between crumbs.
    ///   - project: the crumb naming the project, whole — the project crumb,
    ///     or a workshop's or session's project-name crumb; nil with none.
    ///   - projectTag: that crumb drawn as the project's tag; nil where the
    ///     project has no tag.
    ///   - parent: the milestone or container crumb; nil with none.
    ///   - title: the selection's crumb.
    public init(
        available: Double, spacing: Double, project: CrumbWidth?, projectTag: CrumbWidth?, parent: CrumbWidth?,
        title: CrumbWidth
    ) {
        func row(_ project: CrumbWidth?, parent: Double?, title: Double) -> Double {
            let parts = [project?.group, parent, title].compactMap { $0 }
            return parts.reduce(0, +) + spacing * Double(max(0, parts.count - 1))
        }
        let titleFloor = title.fixed + title.text * Self.titleFloor

        // 1, then 2: everything whole but the name, shortened to its floor
        // at most — first with the project's name, then with its tag.
        var projects: [(CrumbWidth?, Stage)] = [(project, .full)]
        if project != nil, let projectTag { projects.append((projectTag, .projectTag)) }
        for (shown, stage) in projects {
            let rest = row(shown, parent: parent?.group, title: 0)
            if row(shown, parent: parent?.group, title: titleFloor) <= available {
                self.init(
                    stage: stage, titleWidth: min(title.group, available - rest), parentWidth: nil,
                    projectAsTag: stage == .projectTag)
                return
            }
        }

        // 3: the milestone shortened, down to its floor, the name at its.
        if let parent {
            let asTag = project != nil && projectTag != nil
            let parentRoom = available - row(asTag ? projectTag : project, parent: 0, title: titleFloor)
            if parentRoom >= parent.fixed + parent.text * Self.parentFloor {
                self.init(stage: .parentShortened, titleWidth: titleFloor, parentWidth: parentRoom, projectAsTag: asTag)
                return
            }
        }

        // 4: the Active row's line alone.
        self.init(stage: .minimal, titleWidth: nil, parentWidth: nil, projectAsTag: false)
    }

    init(stage: Stage, titleWidth: Double?, parentWidth: Double?, projectAsTag: Bool) {
        self.stage = stage
        self.titleWidth = titleWidth
        self.parentWidth = parentWidth
        self.projectAsTag = projectAsTag
    }
}
