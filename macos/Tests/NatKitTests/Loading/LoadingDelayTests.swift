import XCTest
@testable import NatKit

final class LoadingDelayTests: XCTestCase {
    func testShouldRevealTrueAfterTheDelayElapses() async {
        let delay = LoadingDelay(duration: .milliseconds(10))

        let revealed = await delay.shouldReveal()

        XCTAssertTrue(revealed)
    }

    func testShouldRevealFalseWhenCancelledBeforeTheDelayElapses() async {
        let delay = LoadingDelay(duration: .seconds(30))

        let task = Task { await delay.shouldReveal() }
        // Give the task a moment to actually start sleeping before cancelling
        // it, so this exercises the cancellation path rather than a race.
        try? await Task.sleep(for: .milliseconds(5))
        task.cancel()

        let revealed = await task.value

        XCTAssertFalse(revealed, "a load that finished (or a view that disappeared) before the delay elapsed should reveal nothing")
    }

    func testDefaultDurationIs250Milliseconds() {
        XCTAssertEqual(LoadingDelay.defaultDuration, .milliseconds(250))
    }

    func testDefaultInitUsesTheDefaultDuration() {
        let delay = LoadingDelay()
        XCTAssertEqual(delay.duration, LoadingDelay.defaultDuration)
    }
}
