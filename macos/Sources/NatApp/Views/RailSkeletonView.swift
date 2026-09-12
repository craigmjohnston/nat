import SwiftUI
import NatKit

/// The rail's first load, drawn as the plan it is about to be rather than as
/// a spinner in an empty column: `RailSkeleton`'s rows built out of
/// `SkeletonBlock`s at the rail's own geometry — the same `RailSlot` leading
/// axis, the same indent per level, the same row height — so the arriving
/// plan lands on a layout that is already correct and nothing shifts under
/// it.
///
/// Only a cold load gets this: a project opening from its cached plan has
/// real rows to draw from the first frame, and a refresh keeps the plan it
/// already has. `QuietLoadingView` is still what a wait with no shape to it
/// gets — a diff, a pull request — since a skeleton can only be drawn where
/// the content's shape is known in advance.
struct RailSkeletonView: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(RailSkeleton.rows.enumerated()), id: \.offset) { _, row in
                treeRow(row)
            }
        }
        // The blocks say nothing on their own, and a screen reader landing on
        // eight of them should hear what they stand for.
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(RailSkeleton.accessibilityLabel)
    }

    /// A folder or slice row: its glyph block in the slot at the row's own
    /// indent, its title block beside it, in the frame every tree row in the
    /// rail is drawn at.
    private func treeRow(_ row: RailSkeletonRow) -> some View {
        HStack(spacing: RailSlot.spacing) {
            SkeletonBlock(
                width: RailSlot.slot,
                height: row.kind == .folder ? 10.5 : 12,
                cornerRadius: 2
            )

            titleBlock(row, height: 9)

            Spacer(minLength: 0)
        }
        .frame(height: RailSlot.rowHeight)
        .padding(.leading, RailSlot.leading + CGFloat(row.depth) * RailSlot.indent)
        .padding(.trailing, RailSlot.trailing)
    }

    /// The title block, at its share of the rail: measured off the rail's own
    /// width rather than a `GeometryReader`, so a row keeps the natural
    /// height its content gives it. Floored so a rail dragged narrow still
    /// shows a block, and capped at the room left beside the glyph so one
    /// never runs out past the trailing edge.
    private func titleBlock(_ row: RailSkeletonRow, height: CGFloat, cornerRadius: CGFloat = 3) -> some View {
        SkeletonBlock(height: height, cornerRadius: cornerRadius)
            .containerRelativeFrame(.horizontal, alignment: .leading) { width, _ in
                let inset = RailSlot.leading + CGFloat(row.depth) * RailSlot.indent
                    + RailSlot.slot + RailSlot.spacing + RailSlot.trailing
                return min(max(24, width - inset), max(24, width * CGFloat(row.titleWidth)))
            }
    }

}

#Preview {
    RailSkeletonView()
        .frame(width: 260, height: 400)
        .surface(.window)
}
