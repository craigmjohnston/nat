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
