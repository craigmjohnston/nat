import XCTest
@testable import NatKit

final class TerminalLifecycleTests: XCTestCase {
    // MARK: - The named transitions

    func testIdleStartRequestedGoesToAttaching() {
        var lifecycle = TerminalLifecycle()
        XCTAssertEqual(lifecycle.state, .idle)

        let next = lifecycle.handle(.startRequested)

        XCTAssertEqual(next, .attaching)
        XCTAssertEqual(lifecycle.state, .attaching)
    }

    func testAttachingProcessLaunchedGoesToAttached() {
        var lifecycle = TerminalLifecycle(state: .attaching)

        let next = lifecycle.handle(.processLaunched)

        XCTAssertEqual(next, .attached)
    }

    func testAttachedProcessTerminatedWithSessionStillThereGoesToExitedDetached() {
        var lifecycle = TerminalLifecycle(state: .attached)

        let next = lifecycle.handle(.processTerminated(sessionStillExists: true))

        XCTAssertEqual(next, .exited(.detached))
    }

    func testAttachedProcessTerminatedWithSessionGoneGoesToExitedSessionGone() {
        var lifecycle = TerminalLifecycle(state: .attached)

        let next = lifecycle.handle(.processTerminated(sessionStillExists: false))

        XCTAssertEqual(next, .exited(.sessionGone))
    }

    // A process can end before it ever finished attaching — a bad tmux
    // binary, a session that vanished between launch and the first read —
    // and the machine still has to land in exited rather than get stuck.
    func testAttachingProcessTerminatedWithSessionStillThereGoesToExitedDetached() {
        var lifecycle = TerminalLifecycle(state: .attaching)

        let next = lifecycle.handle(.processTerminated(sessionStillExists: true))

        XCTAssertEqual(next, .exited(.detached))
    }

    func testAttachingProcessTerminatedWithSessionGoneGoesToExitedSessionGone() {
        var lifecycle = TerminalLifecycle(state: .attaching)

        let next = lifecycle.handle(.processTerminated(sessionStillExists: false))

        XCTAssertEqual(next, .exited(.sessionGone))
    }

    func testExitedStartRequestedGoesToAttachingAgain() {
        var lifecycle = TerminalLifecycle(state: .exited(.detached))

        let next = lifecycle.handle(.startRequested)

        XCTAssertEqual(next, .attaching)
    }

    func testExitedSessionGoneStartRequestedGoesToAttachingAgain() {
        var lifecycle = TerminalLifecycle(state: .exited(.sessionGone))

        let next = lifecycle.handle(.startRequested)

        XCTAssertEqual(next, .attaching)
    }

    // MARK: - Everything that is not a transition leaves the state alone

    func testIdleProcessLaunchedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .idle)

        let next = lifecycle.handle(.processLaunched)

        XCTAssertEqual(next, .idle)
    }

    func testIdleProcessTerminatedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .idle)

        let next = lifecycle.handle(.processTerminated(sessionStillExists: true))

        XCTAssertEqual(next, .idle)
    }

    func testAttachingStartRequestedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .attaching)

        let next = lifecycle.handle(.startRequested)

        XCTAssertEqual(next, .attaching)
    }

    func testAttachedStartRequestedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .attached)

        let next = lifecycle.handle(.startRequested)

        XCTAssertEqual(next, .attached)
    }

    func testAttachedProcessLaunchedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .attached)

        let next = lifecycle.handle(.processLaunched)

        XCTAssertEqual(next, .attached)
    }

    func testExitedProcessLaunchedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .exited(.detached))

        let next = lifecycle.handle(.processLaunched)

        XCTAssertEqual(next, .exited(.detached))
    }

    func testExitedProcessTerminatedIsIgnored() {
        var lifecycle = TerminalLifecycle(state: .exited(.sessionGone))

        let next = lifecycle.handle(.processTerminated(sessionStillExists: true))

        XCTAssertEqual(next, .exited(.sessionGone))
    }

    // MARK: - The pure transition function, independent of the mutable wrapper

    func testTransitionFunctionMatchesHandle() {
        XCTAssertEqual(
            TerminalLifecycle.transition(from: .idle, on: .startRequested),
            .attaching
        )
    }

    func testDefaultInitIsIdle() {
        XCTAssertEqual(TerminalLifecycle().state, .idle)
    }
}
