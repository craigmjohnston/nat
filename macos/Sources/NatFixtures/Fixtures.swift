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

    // MARK: - Claude Code version

    /// Up to date: no notice — every story but the notice's own.
    public static let claudeVersionCurrent = ClaudeVersion(
        installed: "2.1.295", latest: "2.1.295", updateAvailable: false)

    /// A newer Claude Code released: the status bar's notice.
    public static let claudeVersionBehind = ClaudeVersion(
        installed: "2.1.294", latest: "2.1.295", updateAvailable: true)

    /// What `claude update` prints on success.
    public static let claudeUpdateOutput = "Current version: 2.1.294\nChecking for updates...\n"
        + "Successfully updated from 2.1.294 to version 2.1.295\n"

    // MARK: - Usage

    /// `now` plus the given number of hours: a usage window's reset. The
    /// gallery measures it against `now` too (its `clock`), so the window
    /// is in the future there and its label is the same every run.
    public static func hoursFromNow(_ hours: Double) -> Date {
        now.addingTimeInterval(hours * 3600)
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
