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
