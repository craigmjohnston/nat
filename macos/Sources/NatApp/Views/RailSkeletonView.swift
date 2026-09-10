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
                switch row.kind {
                case .heading:
                    headingRow(row)
                case .folder, .slice:
                    treeRow(row)
                }
            }
        }
        // The blocks say nothing on their own, and a screen reader landing on
        // nine of them should hear what they stand for.
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(RailSkeleton.accessibilityLabel)
    }

    /// A section heading's two blocks — the icon in the shared slot and the
    /// all-caps label beside it. Its height comes from hidden runs of the
    /// very icon and type `sectionHeading` sets it in rather than from a
    /// number copied off it, so the two cannot drift apart.
    private func headingRow(_ row: RailSkeletonRow) -> some View {
        HStack(spacing: RailSlot.spacing) {
            headingIcon
                .frame(width: RailSlot.slot)
                .overlay { SkeletonBlock(width: RailSlot.slot, height: 9, cornerRadius: 2) }

            titleBlock(row, height: 8, cornerRadius: 2)

            Spacer(minLength: 0)

            // Zero-width — it takes no room and still gives the row the
            // height a heading's line of type comes to. Fixed first, or a
            // width of nothing would wrap it a letter to the line and make
            // the row four times the height it should be.
            headingLabel.fixedSize().frame(width: 0)
        }
        .padding(.leading, RailSlot.leading)
        .padding(.trailing, RailSlot.trailing)
        .padding(.bottom, 5)
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

    /// Hidden stand-ins sizing the heading row: they draw nothing and are
    /// there for the height alone.
    private var headingIcon: some View {
        Image(systemName: "list.bullet")
            .font(.system(size: Typo.caption, weight: .semibold))
            .hidden()
    }

    private var headingLabel: some View {
        Text("TODO")
            .font(.system(size: Typo.caption, weight: .semibold))
            .hidden()
    }
}

#Preview {
    RailSkeletonView()
        .frame(width: 260, height: 400)
        .background(DesignTokens.windowBg)
}
