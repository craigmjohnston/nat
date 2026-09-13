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

    /// How thick a placeholder line is drawn for type at `size`: roughly the
    /// cap height of it, so the block reads as the ink of a line rather than
    /// as the line's whole box — which, at the leading a text row actually
    /// takes, would be a slab.
    ///
    /// Derived rather than picked per call site: every placeholder line in
    /// the app stands in for a run of type whose size is already a number
    /// the design system holds (`Typo`), and a thickness chosen beside each
    /// one is how the skeletons came to be drawn at heights the views they
    /// replace never used.
    public static func lineThickness(forTextOf size: CGFloat) -> CGFloat {
        (size * capHeightRatio).rounded()
    }

    /// What share of its point size a line of type inks — near enough the
    /// cap height of the faces the app is set in.
    static let capHeightRatio: CGFloat = 0.62
}
