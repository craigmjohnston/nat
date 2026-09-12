import Foundation

/// How tall the rail's pinned band is drawn — the band holding the in-flight
/// sections above the scrolling plan.
///
/// The band is the height of what it holds, so with a quiet ACTIVE section
/// the rail reads exactly as it did when everything was one scroll: the
/// separator sits directly under the last entry rather than half way down an
/// empty column. Past `maxShare` of the rail it stops growing and scrolls
/// within itself instead, since a dozen agents running at once must not be
/// able to push TODO off the rail altogether.
public enum RailPinnedBand {
    /// The most of the rail's height the pinned band may take. Half: the
    /// plan is the other thing the rail is for, and an even split is the one
    /// share that says neither is the lesser.
    public static let maxShare: Double = 0.5

    /// The height to draw the band at, given what it holds and how tall the
    /// rail is. A rail not yet measured — the first pass, before the
    /// geometry lands — gives the band its whole content rather than nothing,
    /// so a band that is never measured is still a band that draws.
    public static func height(content: Double, rail: Double) -> Double {
        guard content > 0 else { return 0 }
        guard rail > 0 else { return content }
        return min(content, rail * maxShare)
    }

    /// Whether the band has to scroll within itself: its content is taller
    /// than the height it is drawn at. Read off `height` rather than
    /// computed a second way, so the band and its scrolling cannot disagree.
    /// The hair of tolerance is for the fractional heights layout hands
    /// back — a band a twentieth of a point over its content is not one with
    /// anything to scroll to.
    public static func scrolls(content: Double, rail: Double) -> Bool {
        content - height(content: content, rail: rail) > 0.5
    }
}
