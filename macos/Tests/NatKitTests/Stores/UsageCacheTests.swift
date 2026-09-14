import XCTest
@testable import NatKit

final class UsageCacheTests: XCTestCase {
    private var fileURL: URL!
    private var cache: DiskUsageCache!

    override func setUpWithError() throws {
        try super.setUpWithError()
        fileURL = URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
            .appendingPathComponent("usage-cache-tests-" + UUID().uuidString, isDirectory: true)
            .appendingPathComponent("usage.json", isDirectory: false)
        cache = DiskUsageCache(fileURL: fileURL)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: fileURL.deletingLastPathComponent())
        try super.tearDownWithError()
    }

    func testRoundTrip() async {
        let reading = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 38, resetsAt: Date(timeIntervalSince1970: 1_700_000_000)))
        await cache.write(reading)

        let read = await cache.read()
        XCTAssertEqual(read, reading)
    }

    func testWriteReplacesTheLastReading() async {
        await cache.write(UsageReading(fiveHour: UsageRateLimit(usedPercentage: 10, resetsAt: Date())))
        let second = UsageReading(fiveHour: UsageRateLimit(usedPercentage: 90, resetsAt: Date()))
        await cache.write(second)

        let read = await cache.read()
        XCTAssertEqual(read?.fiveHour?.usedPercentage, 90)
    }

    func testMissingFileReadsAsNothing() async {
        let read = await cache.read()
        XCTAssertNil(read)
    }

    func testCorruptFileReadsAsNothing() async throws {
        try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("{ not a reading".utf8).write(to: fileURL)

        let read = await cache.read()
        XCTAssertNil(read)
    }

    func testDefaultFileURLIsUnderTheBundleID() {
        let defaultCache = DiskUsageCache()
        XCTAssertEqual(defaultCache.fileURL.lastPathComponent, "usage.json")
        XCTAssertEqual(
            defaultCache.fileURL.deletingLastPathComponent().lastPathComponent,
            DiskPlanCache.bundleID
        )
        XCTAssertEqual(DiskUsageCache.defaultFileURL, defaultCache.fileURL)
    }
}
