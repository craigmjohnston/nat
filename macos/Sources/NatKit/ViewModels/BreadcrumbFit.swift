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
/// 2. the milestone (or container) ellipsizes, down to 50% of it shown, the
///    name held at 80%;
/// 3. past that, the breadcrumb goes and the selection is drawn as its Active
///    row draws it — project badge, state dot, name — which alone ellipsizes.
///
/// The crumb naming the project is its badge, already as short as it gets,
/// so it never gives way on its own. At each stage the selection's name
/// takes all the room it can, up to its whole width, before anything else
/// gives way.
public struct BreadcrumbFit: Equatable, Sendable {
    public enum Stage: Equatable, Sendable {
        /// Every crumb in full; only the name may be shortened.
        case full
        /// The milestone crumb shortened as well.
        case parentShortened
        /// No breadcrumb: the Active row's badge, dot and name alone.
        case minimal
    }

    /// The least of the name shown before the milestone gives way.
    public static let titleFloor = 0.8
    /// The least of the milestone shown before the breadcrumb goes.
    public static let parentFloor = 0.5

    public let stage: Stage
    /// The most the selection's crumb may take, glyphs and all; nil for no
    /// limit (the minimal stage, which shortens on its own).
    public let titleWidth: Double?
    /// The most the milestone crumb may take; nil for its whole width.
    public let parentWidth: Double?

    /// - Parameters:
    ///   - available: the breadcrumb's room.
    ///   - spacing: the gap between crumbs.
    ///   - project: the crumb naming the project, badge and slash — the
    ///     project crumb, or a workshop's or session's project crumb; nil
    ///     with none.
    ///   - parent: the milestone or container crumb; nil with none.
    ///   - title: the selection's crumb.
    public init(available: Double, spacing: Double, project: Double?, parent: CrumbWidth?, title: CrumbWidth) {
        func row(parent: Double?, title: Double) -> Double {
            let parts = [project, parent, title].compactMap { $0 }
            return parts.reduce(0, +) + spacing * Double(max(0, parts.count - 1))
        }
        let titleFloor = title.fixed + title.text * Self.titleFloor

        // 1: everything whole but the name, shortened to its floor at most.
        if row(parent: parent?.group, title: titleFloor) <= available {
            let rest = row(parent: parent?.group, title: 0)
            self.init(stage: .full, titleWidth: min(title.group, available - rest), parentWidth: nil)
            return
        }

        // 2: the milestone shortened, down to its floor, the name at its.
        if let parent {
            let parentRoom = available - row(parent: 0, title: titleFloor)
            if parentRoom >= parent.fixed + parent.text * Self.parentFloor {
                self.init(stage: .parentShortened, titleWidth: titleFloor, parentWidth: parentRoom)
                return
            }
        }

        // 3: the Active row's line alone.
        self.init(stage: .minimal, titleWidth: nil, parentWidth: nil)
    }

    init(stage: Stage, titleWidth: Double?, parentWidth: Double?) {
        self.stage = stage
        self.titleWidth = titleWidth
        self.parentWidth = parentWidth
    }
}
