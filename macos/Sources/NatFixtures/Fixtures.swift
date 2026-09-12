import Foundation
import NatKit

/// Canned app states, written as plain values of NatKit's own types.
///
/// One place the app's states are written down, so a SwiftUI preview, a test
/// and the coming gallery runner all draw the same board rather than each
/// inventing a project of its own. Nothing here renders anything or reaches
/// outside the process: a fixture is a value, and `FixtureNatClient` is that
/// value handed back through the very protocol the stores already read.
///
/// Every fixture is deterministic. Times are measured back from `Fixtures.now`
/// — a pinned instant rather than `Date()` — so a snapshot taken twice is the
/// same snapshot and a test never has to allow for the clock moving.
public enum Fixtures {
    /// The instant every fixture's clock is read at: 2026-01-15 10:00:00 UTC.
    /// Pinned so "3h ago" is always three hours before the same moment.
    public static let now = Date(timeIntervalSince1970: 1_768_471_200)

    /// `now` less the given number of minutes — how a fixture says "a while
    /// back" without naming an absolute date twice.
    public static func minutesAgo(_ minutes: Double) -> Date {
        now.addingTimeInterval(-minutes * 60)
    }
}
