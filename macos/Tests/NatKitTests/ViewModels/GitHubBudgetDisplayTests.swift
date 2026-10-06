import XCTest
@testable import NatKit

/// The status bar's GitHub readout and Settings ▸ About's Diagnostics rows.
final class GitHubBudgetDisplayTests: XCTestCase {
    private var calendar: Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Europe/London")!
        return c
    }

    /// 6 October 2026, 13:00 in London.
    private let now = Date(timeIntervalSince1970: 1_791_288_000)

    private func limit(
        remaining: Int = 4211, resetIn: TimeInterval = 46 * 60, projected: Int? = 4100, throttled: Bool = false,
        paused: Bool = false
    ) -> GitHubRateLimit {
        GitHubRateLimit(
            limit: 5000, remaining: remaining, resetAt: now.addingTimeInterval(resetIn),
            projectedRemainingAtReset: projected, throttled: throttled,
            pausedUntil: paused ? now.addingTimeInterval(resetIn) : nil, pollAfterSeconds: 30, cost: 1)
    }

    /// The readout's three states: nothing while healthy (or before any
    /// reading), what is left once throttled, the reset once paused.
    func testReadoutStates() {
        XCTAssertEqual(GitHubBudgetReadout(nil), .healthy)
        XCTAssertNil(GitHubBudgetReadout(nil).text(now: now, calendar: calendar))
        XCTAssertNil(GitHubBudgetReadout(limit()).text(now: now, calendar: calendar))
        XCTAssertNil(GitHubBudgetReadout(limit()).tooltip(now: now, calendar: calendar))
        XCTAssertNil(GitHubBudgetReadout(limit()).stateWord)

        let throttled = GitHubBudgetReadout(limit(remaining: 412, projected: 120, throttled: true))
        XCTAssertEqual(throttled.text(now: now, calendar: calendar), "GitHub \u{00B7} 412 left")
        XCTAssertEqual(throttled.tooltip(now: now, calendar: calendar),
                       "GitHub polling slowed to keep a reserve for actions: 412 of 5000 points left, "
                       + "120 projected at the reset at 13:46")
        XCTAssertEqual(throttled.stateWord, "throttled")
        let noProjection = GitHubBudgetReadout(limit(remaining: 412, projected: nil, throttled: true))
        XCTAssertEqual(noProjection.tooltip(now: now, calendar: calendar),
                       "GitHub polling slowed to keep a reserve for actions: 412 of 5000 points left, resets 13:46")

        let paused = GitHubBudgetReadout(limit(remaining: 0, throttled: true, paused: true))
        XCTAssertEqual(paused, .paused(until: now.addingTimeInterval(46 * 60)), "the pause wins over the throttle")
        XCTAssertEqual(paused.text(now: now, calendar: calendar), "GitHub limit \u{00B7} resets 13:46")
        XCTAssertEqual(paused.tooltip(now: now, calendar: calendar),
                       "GitHub refused on its API limit; polling paused until 13:46. Actions still run.")
        XCTAssertEqual(paused.stateWord, "paused")
    }

    /// The budget row over a table: no reading, healthy, throttled, paused,
    /// and a reset tomorrow, which names its date.
    func testBudgetRow() {
        let cases: [(GitHubRateLimit?, String)] = [
            (nil, "no reading yet"),
            (limit(), "4211 / 5000 \u{00B7} resets 13:46"),
            (limit(remaining: 412, throttled: true), "412 / 5000 \u{00B7} resets 13:46 \u{00B7} throttled"),
            (limit(remaining: 0, paused: true), "0 / 5000 \u{00B7} resets 13:46 \u{00B7} paused"),
            (limit(resetIn: 12 * 3600), "4211 / 5000 \u{00B7} resets 7 Oct 01:00"),
        ]
        for (reading, want) in cases {
            XCTAssertEqual(DiagnosticsFormat.budget(reading, now: now, calendar: calendar), want)
        }
    }

    /// The session's tally, singular where it is one.
    func testUsageRow() {
        XCTAssertEqual(DiagnosticsFormat.usage(points: 37, readings: 29, actions: 8),
                       "37 points \u{00B7} 29 readings \u{00B7} 8 actions")
        XCTAssertEqual(DiagnosticsFormat.usage(points: 1, readings: 1, actions: 0),
                       "1 point \u{00B7} 1 reading \u{00B7} 0 actions")
    }

    /// How long the app has been open: minutes under an hour, hours and
    /// minutes under a day, days and hours past one.
    func testSessionLength() {
        let cases: [(TimeInterval, String)] = [
            (-5, "0m"), (30, "0m"), (899, "14m"), (3600, "1h 0m"), (8040, "2h 14m"),
            (86_400, "1d 0h"), (269_940, "3d 2h"),
        ]
        for (elapsed, want) in cases {
            XCTAssertEqual(DiagnosticsFormat.sessionLength(from: now, to: now.addingTimeInterval(elapsed)), want)
        }
    }

    /// The reading's policy fields decode, and an older nat's block without
    /// them reads as nothing to say.
    func testRateLimitDecodes() throws {
        let full = try JSONDecoder().decode(GitHubRateLimit.self, from: Data(#"""
            {"limit":5000,"remaining":412,"reset_at":"2026-10-06T13:46:00Z","projected_remaining_at_reset":120,
             "throttled":true,"paused_until":"2026-10-06T13:46:00Z","poll_after_seconds":300,"cost":1}
            """#.utf8))
        XCTAssertEqual(full.projectedRemainingAtReset, 120)
        XCTAssertTrue(full.throttled)
        XCTAssertEqual(full.pausedUntil, full.resetAt)
        XCTAssertEqual(full.pollAfterSeconds, 300)
        XCTAssertEqual(full.cost, 1)

        let old = try JSONDecoder().decode(GitHubRateLimit.self, from: Data(
            #"{"limit":5000,"remaining":412,"reset_at":"2026-10-06T13:46:00Z"}"#.utf8))
        XCTAssertNil(old.projectedRemainingAtReset)
        XCTAssertFalse(old.throttled)
        XCTAssertNil(old.pausedUntil)
        XCTAssertNil(old.pollAfterSeconds)
        XCTAssertEqual(old.cost, 0)

        XCTAssertThrowsError(try JSONDecoder().decode(GitHubRateLimit.self, from: Data(
            #"{"limit":1,"remaining":1,"reset_at":"2026-10-06T13:46:00Z","paused_until":"later"}"#.utf8)))
    }
}
