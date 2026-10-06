import Darwin
import SwiftTerm
import XCTest
@testable import NatKit

/// The agent terminal's attach client is SwiftTerm's `LocalProcess`, and its
/// `terminate()` cancels the exit monitor that would have reaped the child
/// in the same breath as it sends the SIGTERM — so every tab switch away
/// from a live terminal left a `<defunct>` behind. These drive the real
/// `LocalProcess` over a harmless `/bin/sleep`.
@MainActor
final class ChildReaperTests: XCTestCase {
    /// Pins the SwiftTerm behaviour `ChildReaper` exists for. Should a
    /// SwiftTerm update start reaping on terminate, this fails — and the
    /// reap in `AgentTerminalHostView.Coordinator.detach` can go.
    func testLocalProcessTerminateLeavesItsChildAZombie() throws {
        let process = LocalProcess(delegate: SilentDelegate())
        process.startProcess(executable: "/bin/sleep", args: ["60"])
        let pid = process.shellPid
        XCTAssertGreaterThan(pid, 0)
        defer { reapNow(pid) }

        process.terminate()

        XCTAssertTrue(waitFor { Self.isZombie(pid) }, "the SIGTERM should have ended the child")
        // Anything SwiftTerm had queued on the main queue gets its chance.
        RunLoop.main.run(until: Date().addingTimeInterval(0.3))
        XCTAssertTrue(Self.isZombie(pid), "SwiftTerm reaped its child — ChildReaper may no longer be needed")
    }

    func testReapAfterTerminateLeavesNoZombie() {
        let process = LocalProcess(delegate: SilentDelegate())
        process.startProcess(executable: "/bin/sleep", args: ["60"])
        let pid = process.shellPid
        XCTAssertGreaterThan(pid, 0)

        process.terminate()
        ChildReaper.reap(pid)

        XCTAssertTrue(waitFor { Self.isGone(pid) }, "terminated child left unreaped")
    }

    /// The reap waits for an exit still to come rather than giving up on a
    /// child that is slow to die. Spawned bare, not through `LocalProcess`,
    /// whose own monitor would reap a child that exits by itself.
    func testReapWaitsForAChildThatExitsLater() throws {
        var pid: pid_t = 0
        let argv: [UnsafeMutablePointer<CChar>?] = [strdup("/bin/sleep"), strdup("0.3"), nil]
        defer { argv.forEach { free($0) } }
        XCTAssertEqual(posix_spawn(&pid, "/bin/sleep", nil, nil, argv, environ), 0)

        ChildReaper.reap(pid)
        XCTAssertFalse(Self.isGone(pid), "the child should still be sleeping")

        XCTAssertTrue(waitFor { Self.isGone(pid) }, "child that exited later left unreaped")
    }

    /// A process that never started has pid 0, and `waitpid(0, …)` would
    /// wait on the whole process group — the reap must refuse it outright.
    func testReapIgnoresANonPositivePid() {
        ChildReaper.reap(0)
        ChildReaper.reap(-1)
    }

    // MARK: - Helpers

    private final class SilentDelegate: LocalProcessDelegate {
        func processTerminated(_ source: LocalProcess, exitCode: Int32?) {}
        func dataReceived(slice: ArraySlice<UInt8>) {}
        func getWindowSize() -> winsize { winsize(ws_row: 24, ws_col: 80, ws_xpixel: 0, ws_ypixel: 0) }
    }

    /// Polls `condition` for up to three seconds, spinning the main run loop
    /// between reads so main-queue work (SwiftTerm's) keeps running.
    private func waitFor(_ condition: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(3)
        while Date() < deadline {
            if condition() { return true }
            RunLoop.main.run(until: Date().addingTimeInterval(0.02))
        }
        return condition()
    }

    /// Whether `pid` no longer exists at all — not even as a zombie, which
    /// `kill(pid, 0)` still answers for.
    private static func isGone(_ pid: pid_t) -> Bool {
        kill(pid, 0) == -1 && errno == ESRCH
    }

    private static func isZombie(_ pid: pid_t) -> Bool {
        var info = kinfo_proc()
        var size = MemoryLayout<kinfo_proc>.stride
        var mib: [Int32] = [CTL_KERN, KERN_PROC, KERN_PROC_PID, pid]
        guard sysctl(&mib, 4, &info, &size, nil, 0) == 0, size > 0 else { return false }
        return info.kp_proc.p_stat == SZOMB
    }

    /// Test cleanup only: reaps `pid` if it is still ours to reap.
    private func reapNow(_ pid: pid_t) {
        var status: Int32 = 0
        _ = waitpid(pid, &status, WNOHANG)
    }
}
