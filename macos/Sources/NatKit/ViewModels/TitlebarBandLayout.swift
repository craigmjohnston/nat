import Foundation

/// Where the one titlebar band over the navigator and the main pane puts its
/// three parts: the selection's breadcrumb from the band's leading edge; the
/// trailing item — a handed-back slice's run button, where it has one —
/// against its trailing edge; and the run of the main pane's tabs just before
/// that.
///
/// The trailing item and the tabs live in the main pane's part of the band
/// alone, a frame exactly the main pane's width: the trailing item takes its
/// width first, and the tabs what is left of it — each shown whole while it
/// fits and cut at its leading edge where it does not, never crossing into
/// the navigator's part. The breadcrumb takes whatever the two leave, so a
/// long title runs on past the navigator's width into the gap and gives way
/// only to them.
public struct TitlebarBandLayout: Equatable, Sendable {
    /// The main pane's part of the band: what is left of it after the
    /// navigator's width.
    public let mainWidth: Double
    /// How much of the trailing item is shown: all of it, or the main pane's
    /// width where that is less.
    public let trailingShownWidth: Double
    /// Where the shown trailing item starts, from the band's leading edge.
    public let trailingX: Double
    /// How much of the tabs is shown: all of them, or what of the main
    /// pane's width the trailing item leaves where that is less.
    public let runShownWidth: Double
    /// Where the shown tabs start, from the band's leading edge.
    public let runX: Double
    /// The room the identity has, from the band's leading edge to the tabs.
    public let identityWidth: Double

    public init(bandWidth: Double, navigatorWidth: Double, runWidth: Double, trailingWidth: Double = 0) {
        let band = max(0, bandWidth)
        mainWidth = max(0, band - navigatorWidth)
        trailingShownWidth = min(max(0, trailingWidth), mainWidth)
        trailingX = band - trailingShownWidth
        runShownWidth = min(max(0, runWidth), mainWidth - trailingShownWidth)
        runX = trailingX - runShownWidth
        identityWidth = runX
    }

    /// The band's tabs left to right: they fill from the right, the first
    /// rightmost and each later one added to its left — so the first
    /// (Terminal) never moves as the others appear.
    public static func leftToRight<Tab>(_ tabs: [Tab]) -> [Tab] {
        tabs.reversed()
    }
}
