# internal/vterm

Runs a child command on a PTY (`x/xpty`) and mirrors its screen through an
in-process VT emulator (`x/vt`), so a TUI can draw the child as a widget
instead of handing the terminal over to it. `internal/tui/agentview.go` is the
only caller.

## Hard-won gotchas — do not "simplify" these away

- The parent must close its copy of the PTY's child (slave) end, or a read
  of the parent end never reports EOF/EIO even after the child exits (xpty
  keeps both ends open; this process is itself still a writer) — but **not
  straight after `Start`**: the last close of the child end discards output
  the parent has not read, so a short-lived child's output (`echo`) was lost
  under load whenever its own exit was that last close. `Session.watchExit`
  reaps the child as soon as it exits, then `hangupPty.HangUp` waits until
  the parent end polls empty (`readable`, zero-timeout `unix.Poll` through
  `Control` — never `Fd()`, which makes the descriptor blocking), bounded by
  `drainTimeout`, and only then closes it. `reap` waits for that exit and
  kills after `reapTimeout` (cancelling `waitCtx`).
- Read the screen only through `Session.Render()`. The emulator's own
  `Draw`/`Touched` damage-tracking goes nil across a resize, so anything built
  on it silently stops updating.
- End the emulator's input pipe by closing the `*io.PipeWriter` directly
  (`stopInput`), never via the emulator's own `Close`. The emulator's `Close`
  also flips an unguarded flag the reply pump reads every pass — a data race
  this package cannot fix from outside.
- `vt.SafeEmulator`'s own lock is not trusted alone: an upstream fix for a
  race in it was merged and then reverted. Every touch of `emu` goes through
  `Session.mu` instead.

## Structure

- Two goroutines per `Session`: `readPump` moves PTY bytes into the emulator;
  `replyPump` drains the emulator's answers to the child's startup queries
  (DA1, DA2, DSR/CPR, DECRPM) back out to the PTY. Start the reply pump
  *before* the read pump — if it isn't already draining, the child's first
  query answer has nowhere to go and both sides stall.
- `newPty` and `waitProcess` are the two seams for fakes in tests.
- `cursorVisible` (DECTCEM) is an `atomic.Bool`, not under `mu`: the emulator
  reports it via a callback fired from inside a write, so it can't safely take
  the same lock.
- `SendBytes`/`Paste`/`SendKey`/`SendMouse` all go through the emulator
  (`SendBytes` via its input pipe) rather than writing the PTY directly, so
  everything stays in order with the emulator's own query replies.
- `Close` is idempotent and does not block; `Session.Done()` reports when the
  child has actually been reaped.
