import CoreGraphics

/// Where the Visual changes pane floats the comment box open at a point: in
/// the pane's own coordinates, kept `inset` in from every edge of a pane
/// `pane` in size, beside the pin it comments on and never over it.
///
/// Below the pin first, centred on it; above it where below does not fit;
/// where neither fits, the side with more room, clamped into the pane — and
/// if that clamp would put the box over the pin, the box goes beside the pin
/// instead, or failing that keeps to its side of the pin past the pane's
/// edge: a pin hidden under its own comment is worse than a box cut short.
/// A pin scrolled out of the pane is followed only as far as the pane's edge,
/// so the box stays on screen while its comment is typed.
public enum VisualEditorPlacement {
    /// The air between the pin and the box.
    public static let gap: CGFloat = 6

    /// The box's top leading corner for a pin whose frame is `pin`.
    public static func origin(pin: CGRect, boxSize: CGSize, pane: CGSize, inset: CGFloat) -> CGPoint {
        let bounds = CGRect(x: inset, y: inset, width: max(pane.width - 2 * inset, 0),
                            height: max(pane.height - 2 * inset, 0))
        let x = clamp(pin.midX - boxSize.width / 2, bounds.minX, bounds.maxX - boxSize.width)
        let below = pin.maxY + gap
        let above = pin.minY - gap - boxSize.height
        let roomBelow = bounds.maxY - below
        let roomAbove = above + boxSize.height - bounds.minY
        if roomBelow >= boxSize.height {
            return CGPoint(x: x, y: max(below, bounds.minY))
        }
        if roomAbove >= boxSize.height {
            return CGPoint(x: x, y: min(above, bounds.maxY - boxSize.height))
        }
        let goesBelow = roomBelow >= roomAbove
        let clamped = CGPoint(x: x, y: clamp(goesBelow ? below : above, bounds.minY, bounds.maxY - boxSize.height))
        guard covers(clamped, boxSize, pin) else { return clamped }
        let right = pin.maxX + gap
        let left = pin.minX - gap - boxSize.width
        if right + boxSize.width <= bounds.maxX { return CGPoint(x: right, y: clamped.y) }
        if left >= bounds.minX { return CGPoint(x: left, y: clamped.y) }
        return CGPoint(x: x, y: goesBelow ? below : above)
    }

    /// Whether a box at `origin` would lie over any of the pin.
    public static func covers(_ origin: CGPoint, _ boxSize: CGSize, _ pin: CGRect) -> Bool {
        CGRect(origin: origin, size: boxSize).intersects(pin)
    }

    /// `value` kept within `low`…`high`, `low` winning where the two cross
    /// (a pane narrower than the box keeps the box's leading edge on screen).
    private static func clamp(_ value: CGFloat, _ low: CGFloat, _ high: CGFloat) -> CGFloat {
        max(min(value, high), low)
    }
}
