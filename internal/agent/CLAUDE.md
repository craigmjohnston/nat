# internal/agent

Agent prompt templates + tmux session management. This is the layer that
turns "launch an agent on this slice" into a real tmux session with a real
prompt file, and the layer everything else (tui, cli, actions) asks about a
running agent's state.

## Identity

- A running agent is identified by its pane's `@nat_slice` option
  (`SlicePaneOption`) — the **full** slice page ID. The session name
  `nat-<last-8-hex-of-slice-page-id>` (`SessionName`/`hexTail`) is only a
  human label; it takes the tail because page IDs share a leading prefix.
- A planning agent has no slice, so it's tagged with the project it's
  workshopping instead: `plan:<project page ID>` (`PlanTag`), session
  `nat-plan-<last-8-hex-of-project-page-id>` (`PlanSessionName`) — one
  planning agent per project, not per machine, so two projects can be
  workshopped at once (`LivePlan` reads only the *active* project's).
- `PlanSentinel`/`PlanSession` (bare `nat-plan`) is the legacy tag every
  planning agent used to carry, before per-project tagging. Nothing launches
  under it now; a session a pre-upgrade nat left running still does, and
  reads as a planning agent belonging to no project in particular — it can
  be attached to from any project, and refuses a second launch on every one.
- `ReclaimStrays`/`breakOut`/`breakOutAll`/`placeholderCommand` (one
  deprecation comment covers all four) re-home panes a pre-upgrade nat left
  joined into its own session (`TUISession`). Still run at startup; slated
  for removal next release. Don't extend this code path — it exists only for
  the upgrade case.

## Prompts (`prompt.go`, `fixprompt.go`)

- `Prompt(c PromptContext)` writes a fresh session's brief; `PromptContext.Fix`
  routes to `fixPrompt` instead — the one launch that is not a fresh
  session's work (a Done slice with an open PR; see root CLAUDE.md's `l`
  domain rule). `resuming(c)` says whether the prompt should tell the agent
  it's continuing rather than starting.
- **Every** `nat` command in every template — slice, fix, planning
  (`PlanPrompt`), wishlist (`WishlistPrompt`) — pins `--project <ID>`
  (`PromptContext.ProjectID`). A `ProjectConfig` cannot supply this itself —
  it's the *value* of the config's `Projects` map, not the key. There is no
  active-project fallback; an unpinned command is refused outright by the
  CLI, and the prompts say so as well as doing it. **One test walks every
  template for an unpinned `nat` invocation** — do not add a new templated
  command without pinning `--project`.
- The `SliceBranch`/`pathSlug`/`Base` naming triad (how a branch name and its
  worktree path are derived — `actions.SliceBranch`, `worktree.pathSlug`,
  `git.CLI.Base`) is spelled out **verbatim** in this package's prompts and in
  `skills/next-slice/SKILL.md` — **do not deduplicate this.** A skill is read
  by an agent, not compiled, so it can't import Go code; both copies must
  independently say the same thing.

## Usage probing (`usage.go`)

- `nat usage` reads Claude Code's own statusline JSON — the only OAuth-free
  way to see `/usage`'s numbers — through a throwaway session tagged with no
  `SlicePaneOption` at all (`LaunchUsageProbe`): it is not an agent working a
  slice, so nothing that scans panes for one should ever find it.
- `UsageProbeDir` is fixed (under `logging.Dir()`, alongside the log file and
  nudge marker), not per-probe: there is only ever one probe in flight, so a
  session or sink a killed prior run left behind is always found at the same
  path. `WriteUsageProbeSettings` writes a `--settings` file whose
  `statusLine` command is a bare `cat > sink` redirect — `--settings` is
  per-session and outranks user/project settings, so the user's own
  statusline configuration is never read, written or shadowed.
- `ParseUsageSink` reads the payload's `rate_limits.five_hour`/`seven_day`,
  each independently `nil` when absent — unknown, never 0%. `internal/cli`'s
  `usage` command is what actually drives the probe end to end (launch,
  prompt, poll, clean up); this file is only the mechanics it drives.

## Agent statusline (`agentstatus.go`)

- Every `Launch`/`LaunchBare` session's `--settings` carries a `statusLine`
  command (`statuslineSettings`) that tees each payload to
  `<state dir>/agent-status/<session>.json` (temp file + `mv`, so a reader
  never sees half a payload). It fires on every turn at no token cost; the
  real payload carries `model`, `effort.level` and
  `context_window.used_percentage` (null until the first response).
  `prepareStatusSink` also writes `<session>.launch.json` (the launch's
  model/effort — the fallback for what the payload leaves out) and clears any
  earlier payload of the same name. Failing to prepare it launches without a
  statusline; it never fails a launch.
- `ReadStatuses(live)` is file reads only, keyed by session name; every field
  absent (empty/nil) when unknown, never zero. It also sweeps files of
  sessions not live, but only once older than `sweepGrace`: a launch writes
  its record before tmux has tagged the pane. `nat status --json` surfaces it.
- Tests in this package run with `HOME`/`XDG_STATE_HOME` pinned (`TestMain`).

## Sessions (`tmux.go`)

- Every tmux call runs with `-u`, forcing a UTF-8 client regardless of the
  caller's locale. `launchd`'s environment names no locale at all — what a
  Finder-launched macOS app's child `nat` inherits — and a non-UTF-8 client
  sanitises tmux's own control characters, turning the tabs `listPanesFormat`
  separates fields with into underscores and making every live agent
  unreadable. Every session nat creates has tmux's status bar off — "the bar says
  nothing its session does not." Sessions the user was already in (nat's own
  terminal included) are never nat's to set options on.
- `Launch`/`LaunchArgs` carry the **launching process's PATH** into the new
  session via `new-session -e`. A session's environment otherwise comes from
  whichever process started the tmux *server* — for the macOS app that's
  launchd's bare PATH, with no `nat` on it. An empty PATH writes nothing
  (says nothing, rather than clobbering the server's with an empty value).
  `supportsSessionEnv` (`tmux -V` read as ≥ 3.2) gates whether `-e` is even
  passed — an older tmux refuses the whole launch over an unsupported flag,
  and a version that can't be read is treated as "don't know," not "old."
- `agentCommand` pins every launch's `--settings` to `{"theme":"auto"}`,
  unconditionally — there is no lighter/darker choice threaded in from the
  caller any more. `"auto"` is what makes Claude Code speak the *live*
  re-theme protocol: `CSI ?2031h` (subscribe) and an OSC 11 probe once at
  startup, then a fresh probe and re-theme whenever it later receives a `CSI
  ?997;1n`/`?997;2n` report on its stdin. A pinned `"light"`/`"dark"` value
  was tried first (the superseded slice this one replaced) and verified not
  to re-theme a running session at all — Claude Code only reads a pinned
  theme once. Answering the OSC 11 probe and sending the CSI reports is
  entirely the attach-client PTY's job, not this package's: a session
  launched detached, with nothing attached to its pane yet, gets no answer
  until a viewer attaches — same as every launch before `"auto"` existed. The
  Go embedded viewer (`internal/vterm.Session`) answers the probe from the
  `bg`/`fg` it was started with, which is static for the process's lifetime —
  the Go TUI has no live "the outer terminal's appearance just changed" event
  of its own to push a `CSI ?997` report on, unlike gnat's explicit
  light/dark/system switcher, so it sends none. `AttachCmd`'s full-screen
  attach hands the fd straight to the user's real terminal and needs nothing
  from nat either way — whatever speaks the protocol there is the real
  terminal's own doing. See `macos/CLAUDE.md` for gnat's half: it pushes the
  `CSI ?997` report itself, on every appearance change.
- `Activity()` scans every tagged pane once (`capture-pane -p -J`) and
  classifies each as working / waiting / gone / unknown, matched by **shape**
  against Claude Code's own status line — a verb that trails off, then
  elapsed time in brackets, e.g. `✻ Quantumizing… (1m 6s · …)` — never by
  wording (the words change; the shape is on every busy screen and no idle
  one). A dead pane is gone without a capture; **a capture that fails leaves
  the state unread, never "gone"** — an agent still running must not read as
  vanished because of one bad poll. It's a poll with no timer of its own; the
  caller decides cadence.
- `SendPrompt` delivers text to a running agent through a **paste buffer**
  (`set-buffer` then `paste-buffer -d -p`), never `send-keys`'s literal mode
  — a multi-line prompt sent key-by-key would submit at the first newline.
  The `Enter` after the paste is what sends the turn; tmux's bracketing is
  how Claude Code's composer tells a pasted newline from a typed one. A
  paste that never happened deletes its own buffer back off the server
  rather than leaving it. **The prompt text is never logged** — it's the
  user's own words about their own code.
- `Interrupt(session)` sends `Escape` (Claude Code's interrupt key) — ends a
  turn, leaves the session running. `Kill(session)` runs `kill-session`,
  ending the session (and the agent) entirely; a session already gone is
  treated as success, not failure (`kill-session`'s "can't find session" is
  matched and swallowed).
- `AttachCmd` (full-screen, `tea.ExecProcess`) and `AttachClientCmd` (the
  embedded viewer's hidden client on its own PTY) build the same argv —
  `tmux -T <ViewerFeatures> attach-session -t <session>` — and both strip
  `TMUX`/`TMUX_PANE` from the environment, since tmux refuses to nest an
  attach while they're set. Only the client command replaces `TERM` (with
  `xterm-256color`): its PTY's far end is the viewer's own emulator, where
  the full-screen attach's is the user's real terminal.
