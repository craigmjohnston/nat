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

## Prompts (`prompt.go`)

- `Prompt(c PromptContext)` writes every slice session's brief. `Resuming(c)`
  — placed on the branch the slice records, a slice with a PR recorded
  (resumed work has its branch cleared), or `PromptContext.HandedBack` (a
  hand-back on the task log: a review sent back before any PR) — says whether it tells the agent
  it's continuing rather than starting. A slice with a PR adds
  `pullRequestPassage` (the PR is open, a push updates it, the launch's
  review snapshot, `gh pr view <PR> --comments` the one `gh` allowed, never
  a PR write). `PromptContext.ConflictBase` (set by `actions.Launch` for a
  conflicted hand-back with no PR) adds `conflictPassage`: rebase onto the
  base, resolve, gate, hand back — the hand-back's own push carries the
  lease. No template tells an agent to push (`TestNoPromptTellsTheAgentToPush`
  walks them with `pushInstruction`; `skills_test.go` the skills): every
  passage says commit, and that `complete-slice` pushes. The hand-back
  example names `--branch <branch>` only where there is no worktree for nat
  to read it off (the default, unplaced case). Every
  slice prompt carries `resumePassage` (`nat
  slice-resume` before changing anything when asked for more after a
  hand-back; a Done refusal means merged). See root CLAUDE.md's Resuming
  rule.
- `nat slice-checks` is how every agent reads CI: `checksPassage` in the
  slice prompt and `ChecksPrompt` (the nudge `actions.NoticeFailingChecks`
  sends). No template names `gh pr checks`.
- **Every** `nat` command in every template — slice, planning
  (`PlanPrompt`) — pins `--project <ID>`
  (`PromptContext.ProjectID`). A `ProjectConfig` cannot supply this itself —
  it's the *value* of the config's `Projects` map, not the key. There is no
  active-project fallback; an unpinned command is refused outright by the
  CLI, and the prompts say so as well as doing it. **One test walks every
  template for an unpinned `nat` invocation** — do not add a new templated
  command without pinning `--project`.
- `namingPassage` (refer to another slice only by name) is in every slice,
  plan and new-project prompt, and `notesPassage` (`nat slice-note
  --from <own slice ID>`) in every slice prompt; tests walk each
  template for them. The skills carry the same words in their own copies.
- `PromptContext.RepoUnknown` (a source project's task with no repository)
  swaps "Already in your context" for `repoPassage`: work the repository out
  from the card's facts and links, ask the user where it cannot tell, record
  it with `nat slice-repo <id> --project <id> --repo <path>`, which cuts the
  worktree and prints its path — the agent works there. The session starts in the home directory, so the git-status and
  CLAUDE.md lines are left out too. The template walks (`--project` pinning,
  the naming rule, the note command) cover it as `repoUnknownContext`.
- `tmuxPassage` (never `tmux kill-server`, never touch another session; a
  private `-L` socket for any tmux of the agent's own) is in every prompt,
  since every agent nat launches sits on the user's own tmux server. One
  `kill-server` under a `TMUX_TMPDIR` the agent thought isolated it took down
  every running agent twice — `$TMUX` wins while set. A test walks each
  template for it; `/next-slice` carries the rule in its own words.
- `waitingPassage` (run `nat agent-waiting` before ending a turn on
  something only the user can supply, `nat agent-working` first thing once
  answered; not for hand-back, follow-ups or a blocked note) is in every
  slice, fix, plan and new-project prompt; tests walk each for it. Pinned
  prompts (`waitingPassage(true)`) also say the two take no `--project` —
  the only commands `TestEveryCommandInAPromptNamesTheProject` exempts; the
  new-project prompt, which never names the flag, gets `false`. The skills
  don't carry it: an agent run by hand isn't in a pane nat launched.
- The `SliceBranch`/`pathSlug`/`Base` naming triad (how a branch name and its
  worktree path are derived — `actions.SliceBranch`, `worktree.pathSlug`,
  `git.CLI.Base`) is never spelled out in prose: `repoPassage` names `nat
  slice-repo`, `/next-slice` names `nat slice-worktree`, and each says to
  work in the path printed. `TestPromptSourceSpellsOutNoWorktreeNaming`
  (and the skill's test) refuse `.worktrees` and `symbolic-ref`.

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
- The same `-e` gate carries `DISABLE_UPDATES=1` (`noUpdates`,
  `agentEnvArgs`) into every Claude Code session nat makes — `Launch`,
  `LaunchBare`, `LaunchUsageProbe` (the probe carries no PATH) — PATH or no
  PATH. It hides the pane's "Update available! Run: brew upgrade …" line
  (verified live on 2.1.290 under a Homebrew cask), since gnat shows the one
  notice (`nat claude-version`) and updates from nat's own process.
  `DISABLE_AUTOUPDATER` leaves the line up;
  `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` hides it but turns off feature
  flags and with them Remote Control — don't swap to either.
- Every session's shell `cd`s into its workdir itself (`inWorkdir`, in
  `agentCommand`, `bareLaunchArgs` and `usageProbeCommand`) — `-c` alone is
  not enough. A tmux server keeps the working directory it started in; one
  started from a worktree a merge later removed puts every new pane in that
  deleted directory whatever `-c` says, and `claude` refuses to start there
  (exit 1, no transcript — the session just vanishes). `ExecRunner` runs
  every tmux from the home directory (`stableDir`) so a server nat starts
  never has that problem; one someone else started still can.
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
- Every `Launch`/`LaunchBare` (and `LaunchArgs`) loads nat's embedded mod:
  `modelFlags` appends `--plugin-dir <path>` from `prepareMod`
  (`internal/mods.Materialise`, write-once per content hash under the state
  dir). A mod that cannot be written is logged and the launch goes ahead
  without the flag, as `prepareStatusSink` degrades. The usage probe carries
  none. After the pane is tagged, `sweepMods` reads every pane's
  `#{pane_start_command}` and `mods.Sweep` removes hash folders nothing
  live names: deleting a loaded folder unloads the mod from that session,
  so a failed read removes nothing. No prompt or skill mentions the mod.
  With the mod, `agentCommand` hands the brief over by path —
  `NAT_BRIEF=<prompt file> claude … '<opening line>'` (`OpeningLine`/
  `PlanOpeningLine`/`NewProjectOpeningLine`, `opening.go`) — and the mod
  appends it as the `natBrief` context block; without one, the brief is the
  positional prompt as before. A compaction re-reads the file, so it is
  never removed under a live session: `WritePromptFile` writes it to
  `<state dir>/agent-brief/<session>.md` (dir `0700`, file `0600`, temp +
  rename, overwritten by the session's next launch) — never `$TMPDIR`, which
  macOS cleans of files left unread for days — and `ReadStatuses` sweeps the
  briefs of sessions not live, after the same `sweepGrace` as status files. Contract:
  `docs/design/embedded-mod/README.md`.
- `Activity()` is one `list-panes` scan, no screen read: a dead pane is
  gone, a pane carrying `@nat_waiting` (`WaitingPaneOption`, a field of
  `listPanesFormat`) is waiting, every other live tagged pane is working —
  slices, planning agents and ad hoc sessions alike, keyed by their tag. The
  agent sets the flag itself (`nat agent-waiting` → `SetWaiting`, cleared by
  `nat agent-working`); nat never infers it from the screen (the old
  `capture-pane` match on Claude Code's status line read every finished turn
  as waiting and broke whenever the line's shape changed). The embedded mod
  (`mods/embedded`) is a second writer of the same flag through the same two
  commands, on the waits Claude Code itself knows (an AskUserQuestion
  dialog, a permission prompt, an MCP elicitation, a turn ended on an error
  or refusal); `waitingPassage` stays for a question asked in prose. `SetWaiting`
  refuses (`ErrNotAgentPane`) a pane with no `@nat_slice` tag, read back with
  `display-message` by pane ID *and* tag — tmux answers one aimed at a
  missing pane with an empty line, not an error. The flag lives on the pane
  alone, so a relaunch starts clear. **A send clears it**: `SendPrompt`,
  once delivered (inbox or paste), reads the session's panes (`list-panes -s`) and
  runs `SetWaiting(pane, false)` on a tagged pane that is waiting — an agent
  just told something is no longer waiting on the user, and every sender
  (`agent-send`, triage, notes, the checks nudge) goes through
  it. A pane not waiting is left alone; a failed clear is logged, never the
  send's error. `SendKeys`/`Interrupt` leave it: an interrupt answers nothing. It's a poll with no timer of its own;
  the caller decides cadence.
- `SendPrompt` delivers through the session's **inbox** first (`inbox.go`):
  every `Launch`/`LaunchBare` carries `-e NAT_INBOX=<state dir>/agent-inbox/<session>`
  (`inboxEnvArgs`, under the `-e` gate — the mod gets the whole path, never
  works out the state dir), and a send reads it back with `show-environment`
  — none (old tmux, a session from before) pastes at once. Else it writes
  `<unix nanos>.md` (temp + rename, dir `0700`, file `0600`) and waits
  `inboxWait` (3 s, polled every 200 ms) for the mod to remove it; not
  removed, it removes it and pastes. The mod removes **before** submitting
  and submits only where its `rm` worked, so a file nat took back is never
  delivered twice. The log says `via` inbox or paste; `clearWaiting` runs on
  both. Verified live on 2.1.294: a draft in the composer is left as typed.
- The paste fallback (`pastePrompt`) goes through a **paste buffer**
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
