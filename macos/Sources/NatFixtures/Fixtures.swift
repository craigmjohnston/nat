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

    // MARK: - Usage

    /// The live clock plus the given number of hours, rather than `now` plus
    /// it — a usage reading's `resetsAt` has to be in the *real* future for
    /// `buildUsageDisplay`'s own expiry rule not to drop it on sight, the
    /// same reason `activityStoreFactory`'s own comment gives for reading the
    /// live clock instead of the pinned one: a fixture measured back from a
    /// pinned instant in the past would read as already expired the moment
    /// real time has moved past it, which it always has by now.
    public static func hoursFromNow(_ hours: Double) -> Date {
        Date().addingTimeInterval(hours * 3600)
    }

    /// The at-rest reading: both windows well under the warning threshold.
    public static let usageReading = UsageReading(
        fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: hoursFromNow(6)),
        sevenDay: UsageRateLimit(usedPercentage: 45, resetsAt: hoursFromNow(30))
    )

    /// One window past the warning threshold, the other at rest — the mixed
    /// reading the brief's own example draws.
    public static let usageReadingOneWarning = UsageReading(
        fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: hoursFromNow(6)),
        sevenDay: UsageRateLimit(usedPercentage: 81, resetsAt: hoursFromNow(54))
    )

    /// Both windows past the warning threshold.
    public static let usageReadingBothWarning = UsageReading(
        fiveHour: UsageRateLimit(usedPercentage: 92, resetsAt: hoursFromNow(2)),
        sevenDay: UsageRateLimit(usedPercentage: 88, resetsAt: hoursFromNow(54))
    )
}
