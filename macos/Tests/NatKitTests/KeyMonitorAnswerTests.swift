import XCTest
@testable import NatKit

final class KeyMonitorAnswerTests: XCTestCase {
    private final class Owner {}

    /// The bug this exists for: a swallow must stay a swallow, not be turned
    /// back into the event by a fallback meant for a view that is gone.
    func testALiveOwnersNilSwallowsTheEvent() {
        let owner = Owner()
        let answer = KeyMonitorAnswer.answer("shift+return", owner: owner) { _, _ in nil }
        XCTAssertNil(answer)
    }

    func testALiveOwnersEventIsDelivered() {
        let owner = Owner()
        let answer = KeyMonitorAnswer.answer("a", owner: owner) { _, event in event }
        XCTAssertEqual(answer, "a")
    }

    func testALiveOwnerIsTheOneAsked() {
        let owner = Owner()
        var asked: Owner?
        _ = KeyMonitorAnswer.answer("a", owner: owner) { got, event in
            asked = got
            return event
        }
        XCTAssertTrue(asked === owner)
    }

    func testAGoneOwnerPassesTheEventOnWithoutAsking() {
        var asked = false
        let answer = KeyMonitorAnswer.answer("shift+return", owner: Owner?.none) { _, _ in
            asked = true
            return nil
        }
        XCTAssertEqual(answer, "shift+return")
        XCTAssertFalse(asked)
    }
}
