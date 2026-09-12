import Foundation

/// What a milestone folder's right-click menu offers, worked out from the
/// plan rather than from the row: moving one is a question about its
/// neighbours and deleting one a question about the slices filed under it,
/// and neither is anything a folder row knows on its own.
///
/// The rules are the CLI's own, said here so the menu is enabled under
/// exactly the conditions `nat` would accept — a greyed item is a refusal
/// the user never has to read.
public struct MilestoneMenuActions: Equatable, Sendable {
    /// The milestone this one would be moved directly before — "Move Up",
    /// which is `milestone-move --before <that name>`. Nil for the first
    /// milestone in the plan, which has nothing above it to go before.
    public let moveBefore: String?
    /// The milestone this one would be moved directly after — "Move Down",
    /// which is `milestone-move --after <that name>`. Nil for the last.
    public let moveAfter: String?
    /// Whether "Delete" is offered. `milestone-remove` refuses a milestone
    /// with any slice still filed under it, naming them, so the menu offers
    /// it only on an empty one.
    public let canDelete: Bool

    public init(moveBefore: String?, moveAfter: String?, canDelete: Bool) {
        self.moveBefore = moveBefore
        self.moveAfter = moveAfter
        self.canDelete = canDelete
    }
}

public enum MilestoneMenuRules {
    /// The menu's answers for one milestone, named as the plan names it.
    ///
    /// `milestones` is taken in whatever order it arrives and sorted here on
    /// `order`, which is the plan's own: the rail draws its folders in that
    /// order and a menu that disagreed with the tree it was opened on would
    /// move a milestone somewhere other than where the arrow pointed.
    ///
    /// A milestone the plan does not hold — a row drawn from a reading the
    /// plan has since moved past — offers no move at all, since there is
    /// nowhere in the plan to place it relative to.
    public static func actions(
        for milestoneName: String,
        in milestones: [Milestone],
        sliceCount: Int
    ) -> MilestoneMenuActions {
        let ordered = milestones.sorted { $0.order < $1.order }
        guard let index = ordered.firstIndex(where: { $0.name == milestoneName }) else {
            return MilestoneMenuActions(moveBefore: nil, moveAfter: nil, canDelete: sliceCount == 0)
        }
        return MilestoneMenuActions(
            moveBefore: index > 0 ? ordered[index - 1].name : nil,
            moveAfter: index < ordered.count - 1 ? ordered[index + 1].name : nil,
            canDelete: sliceCount == 0
        )
    }
}
