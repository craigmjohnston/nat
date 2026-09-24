import XCTest
@testable import NatKit

final class OneShotActionTests: XCTestCase {
    func testFreshActionIsEnabledOnlyWhenAvailable() {
        let action = OneShotAction()
        XCTAssertTrue(action.isEnabled(available: true))
        XCTAssertFalse(action.isEnabled(available: false))
    }

    func testRunningDisables() {
        var action = OneShotAction()
        action.begin()
        XCTAssertTrue(action.isRunning)
        XCTAssertFalse(action.isEnabled(available: true))
    }

    func testSuccessStaysDisabledWhileStateStillReadsAvailable() {
        var action = OneShotAction()
        action.begin()
        action.succeed()
        XCTAssertFalse(action.isEnabled(available: true))
        // A stale reading that still says available changes nothing.
        action.observe(available: true)
        XCTAssertFalse(action.isEnabled(available: true))
    }

    func testFailureRearms() {
        var action = OneShotAction()
        action.begin()
        action.fail()
        XCTAssertTrue(action.isEnabled(available: true))
    }

    func testRecurrenceAfterSuccessRearms() {
        var action = OneShotAction()
        action.begin()
        action.succeed()
        action.observe(available: false)
        XCTAssertFalse(action.isEnabled(available: false))
        action.observe(available: true)
        XCTAssertTrue(action.isEnabled(available: true))
    }

    func testObserveIgnoredWhenNothingSucceededOrWhileRunning() {
        var action = OneShotAction()
        action.observe(available: false)
        action.observe(available: true)
        XCTAssertTrue(action.isEnabled(available: true))

        action.begin()
        action.observe(available: false)
        action.succeed()
        // The drop seen mid-run does not count once it has succeeded.
        action.observe(available: true)
        XCTAssertFalse(action.isEnabled(available: true))
    }
}

@MainActor
final class SliceActionTrackerTests: XCTestCase {
    private struct Boom: LocalizedError {
        var errorDescription: String? { "boom" }
    }

    func testAdvancesPerKind() {
        XCTAssertEqual(SliceActionKind.launch.advance, StageAdvance(from: .brief, to: .agent))
        XCTAssertEqual(SliceActionKind.approve.advance, StageAdvance(from: .diff, to: .pr))
        XCTAssertNil(SliceActionKind.merge.advance)
    }

    func testSuccessAdvancesImmediatelyThenDisables() async {
        let tracker = SliceActionTracker()
        var selected: [WorkflowTab] = []
        var advanceDuring: StageAdvance?
        var runningDuring = false

        await tracker.run(.approve, sliceID: "s", select: { selected.append($0) }) {
            advanceDuring = tracker.advance(for: "s")
            runningDuring = tracker.isRunning(.approve, sliceID: "s")
        }

        XCTAssertEqual(selected, [.pr])
        XCTAssertEqual(advanceDuring, StageAdvance(from: .diff, to: .pr))
        XCTAssertTrue(runningDuring)
        XCTAssertNil(tracker.advance(for: "s"))
        XCTAssertNil(tracker.error(.approve, sliceID: "s"))
        XCTAssertFalse(tracker.isEnabled(.approve, sliceID: "s", available: true))
    }

    func testFailureBacksOutWithErrorAndRearms() async {
        let tracker = SliceActionTracker()
        var selected: [WorkflowTab] = []

        await tracker.run(.approve, sliceID: "s", select: { selected.append($0) }) { throw Boom() }

        XCTAssertEqual(selected, [.pr, .diff])
        XCTAssertNil(tracker.advance(for: "s"))
        XCTAssertEqual(tracker.error(.approve, sliceID: "s"), "boom")
        XCTAssertTrue(tracker.isEnabled(.approve, sliceID: "s", available: true))

        // A retry clears the error.
        await tracker.run(.approve, sliceID: "s", select: { _ in }) {}
        XCTAssertNil(tracker.error(.approve, sliceID: "s"))
    }

    func testMergeDisablesWithoutMovingThePane() async {
        let tracker = SliceActionTracker()
        var selected: [WorkflowTab] = []

        await tracker.run(.merge, sliceID: "s", select: { selected.append($0) }) {}
        XCTAssertTrue(selected.isEmpty)
        XCTAssertFalse(tracker.isEnabled(.merge, sliceID: "s", available: true))

        await tracker.run(.merge, sliceID: "t", select: { selected.append($0) }) { throw Boom() }
        XCTAssertTrue(selected.isEmpty)
        XCTAssertTrue(tracker.isEnabled(.merge, sliceID: "t", available: true))
    }

    func testSecondRunAfterSuccessIsIgnoredUntilAvailabilityRecurs() async {
        let tracker = SliceActionTracker()
        var runs = 0
        await tracker.run(.launch, sliceID: "s", select: { _ in }) { runs += 1 }
        await tracker.run(.launch, sliceID: "s", select: { _ in }) { runs += 1 }
        XCTAssertEqual(runs, 1)

        tracker.observe(.launch, sliceID: "s", available: false)
        tracker.observe(.launch, sliceID: "s", available: true)
        XCTAssertTrue(tracker.isEnabled(.launch, sliceID: "s", available: true))
        await tracker.run(.launch, sliceID: "s", select: { _ in }) { runs += 1 }
        XCTAssertEqual(runs, 2)
    }

    func testConcurrentRunIsIgnored() async {
        let tracker = SliceActionTracker()
        var runs = 0
        await tracker.run(.merge, sliceID: "s", select: { _ in }) {
            await tracker.run(.merge, sliceID: "s", select: { _ in }) { runs += 1 }
        }
        XCTAssertEqual(runs, 0)
    }

    func testObserveUnknownActionIsNoOp() {
        let tracker = SliceActionTracker()
        tracker.observe(.merge, sliceID: "nope", available: false)
        XCTAssertTrue(tracker.isEnabled(.merge, sliceID: "nope", available: true))
        XCTAssertFalse(tracker.isRunning(.merge, sliceID: "nope"))
    }

    func testMessagePrefersCommandFailedText() {
        XCTAssertEqual(SliceActionTracker.message(for: NatError.commandFailed("refused")), "refused")
        XCTAssertEqual(SliceActionTracker.message(for: Boom()), "boom")
    }
}
