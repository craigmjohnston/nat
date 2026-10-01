import Foundation

/// Which edge of a resizable pane its drag handle sits on — the whole of what
/// decides whether dragging rightwards grows or shrinks the pane.
public enum PaneResizeEdge: Sendable {
    /// The handle is on the pane's trailing edge (the left rail): dragging
    /// right grows the pane.
    case trailing
    /// The handle is on the pane's leading edge (a right-hand sidebar):
    /// dragging right shrinks the pane.
    case leading
}

/// The width a resize drag resolves to: the width the pane had when the drag
/// began plus the translation read in the pane's own direction, clamped to
/// the pane's bounds. Deltas are measured from the drag's start rather than
/// accumulated per event, so a drag flung past a bound parks the pane there
/// and picks straight back up when it turns around, owing nothing back.
public func paneResizedWidth(
    startWidth: Double,
    translation: Double,
    edge: PaneResizeEdge,
    minWidth: Double,
    maxWidth: Double
) -> Double {
    let delta = edge == .trailing ? translation : -translation
    return min(maxWidth, max(minWidth, startWidth + delta))
}

/// The width to persist when a resize drag ends: the live drag width, or nil
/// when there is nothing to write — no drag ever moved the pane, or it came
/// back to exactly the persisted width. The live width lives in plain view
/// state for the length of the drag; this is the one gate to the defaults.
/// A height drag (`PaneRowResizeHandle`) commits through the same gate.
public func paneCommittedWidth(live: Double?, persisted: Double) -> Double? {
    guard let live, live != persisted else { return nil }
    return live
}

/// Whether a resize drag finished with the pointer still over its handle.
/// Cursor updates are suppressed for the length of a drag and AppKit
/// re-asserts a cursor rect only as the pointer crosses into it, so the
/// cursor a drag ends under is whatever the drag last set — which leaves the
/// resize cursor up over ordinary content when a drag wanders off the strip
/// (clamped at a bound, or flung past it) and never comes back. The handle's
/// frame and the drag's last location are both read in the window's own
/// coordinate space, so this is the one question the caller has to ask.
public func paneDragEndedOverHandle(handleFrame: CGRect, endLocation: CGPoint) -> Bool {
    handleFrame.contains(endLocation)
}

/// Which edge of a vertically resizable pane its drag handle sits on — the
/// height counterpart of `PaneResizeEdge`.
public enum PaneResizeVerticalEdge: Sendable {
    /// The handle is on the pane's bottom edge (the upper pane of a split):
    /// dragging down grows the pane.
    case bottom
    /// The handle is on the pane's top edge (the lower pane of a split):
    /// dragging down shrinks the pane.
    case top
}

/// The height a resize drag resolves to — `paneResizedWidth` read down the
/// other axis: the height at the drag's start plus the translation in the
/// pane's own direction, clamped to its bounds.
public func paneResizedHeight(
    startHeight: Double,
    translation: Double,
    edge: PaneResizeVerticalEdge,
    minHeight: Double,
    maxHeight: Double
) -> Double {
    let delta = edge == .bottom ? translation : -translation
    return min(maxHeight, max(minHeight, startHeight + delta))
}

/// The upper pane's height in a two-pane split, given the persisted height
/// and the height the split has to share: never below the upper pane's own
/// floor, and never so tall that the lower pane falls below its floor. A
/// split too short for both floors favours the upper one, which is what the
/// divider hangs off — the lower pane then scrolls in whatever is left.
public func paneSplitHeight(
    stored: Double,
    available: Double,
    minUpper: Double,
    minLower: Double
) -> Double {
    max(minUpper, min(stored, available - minLower))
}
