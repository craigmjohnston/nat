import XCTest
@testable import NatKit

final class UsageDisplayTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_700_000_000)
    private let utc = TimeZone(identifier: "UTC")!

    func testNilReadingDrawsNothing() {
        let display = buildUsageDisplay(from: nil, now: now, timeZone: utc)
        XCTAssertTrue(display.isEmpty)
    }

    func testEmptyReadingDrawsNothing() {
        let display = buildUsageDisplay(from: .empty, now: now, timeZone: utc)
        XCTAssertTrue(display.isEmpty)
    }

    func testBothWindowsAtRest() {
        let reading = UsageReading(
            fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: now.addingTimeInterval(3600)),
            sevenDay: UsageRateLimit(usedPercentage: 45, resetsAt: now.addingTimeInterval(3600 * 30))
        )
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertEqual(display.windows.count, 2)
        XCTAssertFalse(display.windows[0].warning)
        XCTAssertFalse(display.windows[1].warning)
        XCTAssertTrue(display.windows[0].text.hasPrefix("Session 38%"))
        XCTAssertTrue(display.windows[1].text.hasPrefix("Week 45%"))
    }

    func testAWindowAtOrAboveThresholdWarns() {
        let reading = UsageReading(
            fiveHour: UsageRateLimit(usedPercentage: 79, resetsAt: now.addingTimeInterval(3600)),
            sevenDay: UsageRateLimit(usedPercentage: 80, resetsAt: now.addingTimeInterval(3600 * 30))
        )
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertFalse(display.windows[0].warning)
        XCTAssertTrue(display.windows[1].warning)
    }

    func testBothWindowsWarn() {
        let reading = UsageReading(
            fiveHour: UsageRateLimit(usedPercentage: 92, resetsAt: now.addingTimeInterval(3600)),
            sevenDay: UsageRateLimit(usedPercentage: 88, resetsAt: now.addingTimeInterval(3600 * 30))
        )
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertTrue(display.windows[0].warning)
        XCTAssertTrue(display.windows[1].warning)
    }

    func testAnExpiredWindowIsDropped() {
        // A last-known reading holds until its own reset, matching /usage's
        // own behaviour — once that reset has passed, the clause is exactly
        // as absent as one the payload never carried.
        let reading = UsageReading(
            fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: now.addingTimeInterval(-1)),
            sevenDay: UsageRateLimit(usedPercentage: 81, resetsAt: now.addingTimeInterval(3600 * 30))
        )
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertEqual(display.windows.count, 1)
        XCTAssertTrue(display.windows[0].text.hasPrefix("Week 81%"))
    }

    func testBothWindowsExpiredDrawsNothing() {
        let reading = UsageReading(
            fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: now.addingTimeInterval(-1)),
            sevenDay: UsageRateLimit(usedPercentage: 81, resetsAt: now.addingTimeInterval(-1))
        )
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertTrue(display.isEmpty)
    }

    func testPercentIsRounded() {
        let reading = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 37.6, resetsAt: now.addingTimeInterval(3600)))
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertTrue(display.windows[0].text.hasPrefix("Session 38%"))
    }

    func testFormatsTheResetClauses() {
        // 1_700_003_600 is 2023-11-14 23:53:20 UTC + 1h => a fixed clock time
        // and weekday, checked against the exact strings the brief's own
        // example draws with.
        let reading = UsageReading(
            fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: Date(timeIntervalSince1970: 1_700_071_200)),
            sevenDay: UsageRateLimit(usedPercentage: 81, resetsAt: Date(timeIntervalSince1970: 1_700_600_000))
        )
        let display = buildUsageDisplay(from: reading, now: now, timeZone: utc)

        XCTAssertEqual(display.windows[0].text, "Session 38% · resets 6:00 PM")
        XCTAssertEqual(display.windows[1].text, "Week 81% · resets Tue")
    }
}
