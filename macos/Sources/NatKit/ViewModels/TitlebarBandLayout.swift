import Foundation

/// Where the one titlebar band over the navigator and the main pane puts its
/// two parts: the selection's identity from the band's leading edge, and the
/// run — the view's readout or actions, then the tabs — anchored to its
/// trailing edge.
///
/// The run lives in the main pane's part of the band alone, a frame exactly
/// the main pane's width: it is shown whole while it fits there and cut at
/// its leading edge where it does not, never crossing into the navigator's
/// part. The identity takes whatever the run leaves, so a long title runs
/// on past the navigator's width into the gap and gives way only to the run.
public struct TitlebarBandLayout: Equatable, Sendable {
    /// The main pane's part of the band: what is left of it after the
    /// navigator's width.
    public let mainWidth: Double
    /// How much of the run is shown: all of it, or the main pane's width
    /// where that is less.
    public let runShownWidth: Double
    /// Where the shown run starts, from the band's leading edge.
    public let runX: Double
    /// The room the identity has, from the band's leading edge to the run.
    public let identityWidth: Double

    public init(bandWidth: Double, navigatorWidth: Double, runWidth: Double) {
        let band = max(0, bandWidth)
        mainWidth = max(0, band - navigatorWidth)
        runShownWidth = min(max(0, runWidth), mainWidth)
        runX = band - runShownWidth
        identityWidth = runX
    }
}
