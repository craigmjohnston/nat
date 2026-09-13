import XCTest
@testable import NatKit

final class NudgeWatcherTests: XCTestCase {
    private var tempDirURL: URL!
    private var testFilePath: String!

    override func setUp() {
        super.setUp()
        tempDirURL = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString
        )
        try? FileManager.default.createDirectory(
            at: tempDirURL,
            withIntermediateDirectories: true
        )
        testFilePath = tempDirURL.appendingPathComponent("nudge").path
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: tempDirURL)
        super.tearDown()
    }

    /// Spins the current thread's run loop so `NudgeWatcher`'s real `Timer`
    /// actually fires: an `await Task.sleep` merely suspends the task and
    /// never drives the run loop that owns the timer.
    private func pump(_ seconds: TimeInterval) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    func testBaselineReadingIsNotReported() async throws {
        let watcher = NudgeWatcher()
        var callCount = 0

        // Create the file before starting the watcher
        try "initial".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        watcher.start(path: testFilePath) {
            callCount += 1
        }

        // Wait a bit for the initial poll
        try await Task.sleep(nanoseconds: 100_000_000) // 100ms

        // Baseline reading should not trigger the callback
        XCTAssertEqual(callCount, 0)

        watcher.stop()
    }

    func testFileModificationTriggersCallback() throws {
        let watcher = NudgeWatcher()

        // Create the file before starting the watcher
        try "initial".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        var callCount = 0
        watcher.start(path: testFilePath) {
            callCount += 1
        }

        // One full poll tick with no change: the baseline read must not fire.
        pump(1.2)
        XCTAssertEqual(callCount, 0)

        try "modified".write(toFile: testFilePath, atomically: true, encoding: .utf8)
        pump(1.2)
        XCTAssertEqual(callCount, 1)

        watcher.stop()
    }

    func testMissingFileIsNotAnError() async throws {
        let watcher = NudgeWatcher()
        var callCount = 0

        // Start watching a file that doesn't exist yet
        watcher.start(path: testFilePath) {
            callCount += 1
        }

        // Wait for a poll
        try await Task.sleep(nanoseconds: 100_000_000) // 100ms

        // No error should be thrown, callback not called
        XCTAssertEqual(callCount, 0)

        watcher.stop()
    }

    func testFileCreationTriggersCallback() throws {
        let watcher = NudgeWatcher()

        // Start watching a file that doesn't exist
        var callCount = 0
        watcher.start(path: testFilePath) {
            callCount += 1
        }

        // One full poll tick with the file still absent: no callback expected.
        pump(1.2)
        XCTAssertEqual(callCount, 0)

        try "created".write(toFile: testFilePath, atomically: true, encoding: .utf8)
        pump(1.2)
        XCTAssertEqual(callCount, 1)

        watcher.stop()
    }

    func testStopPreventsCallbacks() async throws {
        let watcher = NudgeWatcher()
        var callCount = 0

        try "initial".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        watcher.start(path: testFilePath) {
            callCount += 1
        }

        // Wait for baseline
        try await Task.sleep(nanoseconds: 100_000_000) // 100ms

        watcher.stop()

        // Modify the file
        try "modified".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        // Wait to see if callback is called
        try await Task.sleep(nanoseconds: 1_500_000_000) // 1.5s

        // Callback should not have been called after stop
        XCTAssertEqual(callCount, 0)
    }

    func testMultipleModificationsAllTriggerCallbacks() throws {
        let watcher = NudgeWatcher()
        var callCount = 0

        try "initial".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        watcher.start(path: testFilePath) {
            callCount += 1
        }
        pump(1.2)
        XCTAssertEqual(callCount, 0)

        // Each modification is given its own poll tick, so each is counted.
        for i in 1...2 {
            try "modification \(i)".write(toFile: testFilePath, atomically: true, encoding: .utf8)
            pump(1.2)
            XCTAssertEqual(callCount, i)
        }

        watcher.stop()
    }

    func testDeallocStopsWatcher() async throws {
        var watcher: NudgeWatcher? = NudgeWatcher()
        var callCount = 0

        try "initial".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        watcher?.start(path: testFilePath) {
            callCount += 1
        }

        try await Task.sleep(nanoseconds: 100_000_000) // 100ms

        // Deallocate watcher
        watcher = nil

        try "modified".write(toFile: testFilePath, atomically: true, encoding: .utf8)

        try await Task.sleep(nanoseconds: 1_500_000_000) // 1.5s

        // Callback should not be called after deallocation
        XCTAssertEqual(callCount, 0)
    }
}
