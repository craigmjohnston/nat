import Darwin
import Foundation

/// Reaps a child process nothing else will — the agent terminal's attach
/// client, once it has been told to end.
///
/// SwiftTerm's `LocalProcess.terminate()` sends the child SIGTERM and, in the
/// same call, cancels the exit monitor whose handler is its only `waitpid`;
/// the child dies a moment later with nobody left to collect it, so every
/// tab switch away from a live terminal left a `<defunct>` under gnat (56 of
/// them over one 2.5-hour session). A child that exits on its own is still
/// SwiftTerm's to reap, its monitor never cancelled — this is only for one
/// gnat terminated itself.
public enum ChildReaper {
    /// Waits for `pid` to exit and reaps it, on a thread of its own so that
    /// a child slow to die holds nothing up.
    ///
    /// A blocking `waitpid` rather than another exit monitor: a pid nobody
    /// has reaped yet cannot be handed to another process, so the wait is
    /// race-free however soon the child dies — even before this is called —
    /// where an exit source armed after the fact has to be trusted to fire
    /// for a child already gone. The caller's half of that bargain is never
    /// to pass a pid something else may already have reaped.
    ///
    /// A non-positive pid is ignored: a process that never started reads 0,
    /// and `waitpid(0, …)` would wait on the whole process group instead.
    public static func reap(_ pid: pid_t) {
        guard pid > 0 else { return }
        Thread.detachNewThread {
            var status: Int32 = 0
            while waitpid(pid, &status, 0) == -1 && errno == EINTR {}
        }
    }
}
