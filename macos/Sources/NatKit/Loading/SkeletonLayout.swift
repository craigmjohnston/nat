import CoreGraphics

/// The two pieces of arithmetic the pane skeletons lay their blocks out
/// with, kept here beside `Skeleton`'s own so they can be read — and tested
/// — without mounting anything.
public enum SkeletonLayout {
    /// The narrowest a placeholder line is ever drawn: a pane dragged very
    /// narrow still shows a block rather than a sliver of one.
    public static let minimumLineWidth: CGFloat = 24

    /// One line's width: its fraction of the room its parent has left it,
    /// floored so a narrow pane still shows something and capped at that
    /// room, so a block never runs out past the card, box or rail it is
    /// drawn in — which would push the layout wider than the pane and give
    /// the very reflow a skeleton is there to prevent.
    ///
    /// A parent that has left it nothing at all (the first pass of a layout,
    /// where a `GeometryReader` reports zero) draws nothing rather than the
    /// floor, since a block wider than its parent is exactly what the cap is
    /// for.
    public static func lineWidth(_ fraction: Double, in available: CGFloat) -> CGFloat {
        guard available > 0 else { return 0 }
        return min(available, max(minimumLineWidth, available * CGFloat(fraction)))
    }

    /// How tall a run of `count` lines comes to at that height and spacing —
    /// what the paragraph's own frame is set to, since a `GeometryReader`
    /// left to itself is greedy in both axes and would take the whole pane.
    public static func paragraphHeight(_ count: Int, height: CGFloat, spacing: CGFloat) -> CGFloat {
        guard count > 0 else { return 0 }
        return CGFloat(count) * height + CGFloat(count - 1) * spacing
    }
}
