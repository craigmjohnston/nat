# notion-agent-tracker

A Go TUI over Notion for tracking project work executed by Claude Code agents.
Craig manages the plan and launches agents from the TUI; agents run as fresh
`claude` sessions in tmux and reach the tracker only through the headless `nat`
commands — they need no Notion access of their own. The TUI talks to the Notion
REST API directly (`Notion-Version: 2026-03-11`, data-source model).

## Architecture

- `main.go` — entrypoint; module path `github.com/craigmjohnston/nat`, so
  `go install github.com/craigmjohnston/nat@latest` yields a `nat` binary.
  Started outside tmux it re-execs itself into a `nat-tui` session
  (`tmux new-session -A`, so a second launch attaches rather than starting a
  rival); started inside tmux it runs in place and does not nest. `NAT_NO_TMUX=1`
  opts out. The board has to be a pane for an agent's pane to be joined beside it.
- `internal/config/` — XDG config (`~/.config/notion-agent-tracker/config.json`)
  + the Notion bearer token, read from Notion's official CLI via
  `ntn auth token` (the app stores no credential of its own)
- `internal/notion/` — hand-rolled stdlib Notion client (no third-party client
  supports data sources); minimal structs, only fields we use. Built with
  `NewWithToken(TokenFunc)`: the token is fetched per attempt and a 401 is
  retried once with a fresh one, so a token rotated by the CLI is picked up
  mid-session.
- `internal/domain/` — Project/Milestone/Slice models, progress math
- `internal/logging/` — the log file: `~/Library/Logs/notion-agent-tracker/` on
  macOS, the XDG state dir elsewhere, size-capped with one previous file kept.
  Opened by `main` and discarded until then, so importing it writes nothing.
  Failures and writes are logged by the Notion client and the tmux layer
  themselves; startup failures name the log path on stderr, because the TUI's
  tmux pane dies with the process and takes stderr with it. Everything on its
  way to the file passes through a redactor — never log a token or a request
  body.
- `internal/nudge/` — the marker file the headless commands touch after a
  successful Notion write (`nudge` in the log/state dir), and the board's way of
  reading it: the TUI stats its mtime every second and refetches the plan when
  it moves, so agent-driven changes show within a second instead of a poll
  interval. Fire-and-forget on the CLI side — a touch that fails is logged and
  swallowed, and commands are wired to it through `Env.Nudge`.
- `internal/agent/` — agent prompt template + tmux session management. A
  running agent is identified by its pane's `@nat_slice` option (the full slice
  page ID); the session name `nat-<last-8-hex-of-slice-page-id>` is only a
  human label, and takes the tail because page IDs share a leading prefix.
  `ShowPane` toggles that pane between the board's window (`join-pane`, sized by
  `agent_split_percent`, default 65% to the agent) and a session of its own.
  A joined pane dies with the board's window, so `BreakOutJoined` frees every
  such pane as the app leaves (deferred in `main`, so a panic frees them too)
  and `ReclaimStrays` re-homes on startup whatever an earlier run left joined.
  Both ways of attaching to a session — `AttachCmd`, full-screen through
  `tea.ExecProcess`, and `AttachClientCmd`, the hidden client the embedded
  viewer runs on a PTY of its own — build the same argv, `tmux -T
  <ViewerFeatures> attach-session -t <session>`, and drop `TMUX`/`TMUX_PANE`
  from the environment, since tmux refuses to nest an attach while they are
  set. Only the client command replaces `TERM` (with `xterm-256color`): the
  terminal on the far end of its PTY is the viewer's emulator, where the
  full-screen attach's is the user's own.
- `internal/cli/` — the headless commands (`nat info [--json]`,
  `nat next-slice [--json]`, which claims the next Todo slice under the
  lowest-ordered open milestone and prints its brief,
  `nat start-slice <slice> [--json]`, which claims one named Todo slice and
  prints the same brief — the command a board-launched agent runs, since it is
  pointed at a slice rather than choosing one —
  `nat complete-slice <slice> [--pr URL] [--summary TEXT] [--blocked]`, which
  closes out a slice the configured user holds, and the one-off additions
  `nat milestone-add <name>` (Queued, at the end of the plan) and
  `nat slice-add <title> --milestone <name> [--description TEXT|-]
  [--repo DIR]` (Todo and unassigned, description as the page body;
  `--description -` reads it from stdin, so a slice-add typed with no brief
  does not wait on one), the wishlist pair `nat wishlist [--json]`, which prints
  the pending items written under the project page's Wishlist heading (with
  their block IDs under `--json`), and `nat wishlist-clear <block-id>...`,
  which trashes exactly the named items — never the section wholesale, so an
  idea typed while a workshop session runs survives it — and leaves the section
  holding one empty bullet, and `nat plan-apply [FILE]`, which creates a whole
  drafted plan of milestones and slices from a JSON document (read from FILE or
  stdin, validated entirely before the first write, and only ever creating
  pages), and `nat setup`, which installs the embedded skills into
  `~/.claude/skills` — the only command that talks to neither Notion nor the
  config file, since it is what a machine with only the binary runs first), what
  the binary does when given a subcommand. Run before the tmux hosting step and with no TUI
  code in the path: a command prints to the terminal it was typed in and exits.
- `internal/vterm/` — a child command run on a PTY (`x/xpty`) with its screen
  mirrored by an in-process VT emulator (`x/vt`), so the TUI can draw an agent
  as a widget instead of joining its tmux pane. `Start` returns a `Session`:
  `Render`/`Cursor` to draw it, `SendKey`/`SendBytes`/`Paste` to type at it,
  `Resize`, `Output` to know when to redraw and `Done`/`Err` when it has gone.
  Two goroutines: the read pump feeds the PTY into the emulator under the
  Session's own mutex (`vt.SafeEmulator`'s lock is not trusted alone — an
  upstream race fix there was merged and reverted), and the reply pump drains
  the emulator's answers to the child's startup queries back to the PTY, which
  a child that asks DA1/DSR stalls without. Two seams for fakes, `newPty` and
  `waitProcess`. Three details the packages make you find out the hard way: the
  parent's copy of the PTY's child end has to be closed after `Start` or the
  read never ends; the screen is read only through `Render`, since the damage
  list `Draw`/`Touched` work from goes nil across a resize; and the emulator's
  input pipe is ended by closing it directly rather than through the
  emulator's own `Close`, which races the reply pump. Not yet imported by the
  app.
- `internal/tui/` — bubbletea v2 (`charm.land/*/v2` imports); root model in
  `app.go` routes screens; all Notion I/O via tea.Cmd → typed msgs. `poll.go` is
  the background refetch of the plan, for the changes no nudge reports because
  no `nat` command made them: a tick every `poll_seconds` (default 30, bounded)
  running the same load the refresh key does, so the plan stays on screen while
  it is in flight and a failure leaves the board as it was. It is passed over —
  never cancelled, so it resumes on the next tick by itself — while the wizard,
  a form, a row prompt, a write or another load is in flight, since a plan
  landing under any of those would clobber what the user is in the middle of.
- `skills/` — the agent skills (/queue-work planning, /next-slice execution),
  embedded in the binary with `go:embed` and installed by `nat setup`. A
  checkout works on them in place by symlinking them into `~/.claude/skills/`,
  which `nat setup` leaves alone rather than writing back through.

## Domain rules

- Slice workflow: Todo → in progress → Done. Never edit an in-progress or Done
  slice.
- The in-progress status is called `In progress`. Projects made before the app
  asked called it `Claimed`; that name is migrated away at load
  (`notion.MigrateProject`) rather than read anywhere. `notion.ShapeOf` still
  reads the types of the `Status` and `Milestone` columns off the Slices data
  source — either may have been converted to Notion's own status type in the UI
  — and whether the project has an `Assignee` column at all.
- Claiming = Status → `In progress`, plus Assignee (people
  property, the configured real Notion user) where the project has that column.
  Without one, ownership is decided on status alone.
- Slices ↔ PRs are 1:1 when work is code; PR URL recorded in the `PR` property.
- Slices may carry a `Repo` override; otherwise the project default working
  dir from local config applies.
- A project keeps its whole plan on one page: no Milestones database, and a
  `Milestone` **select** on the Slices data source whose options are the
  milestones, in plan order. `domain.MilestonesFromOptions` maps them —
  milestone name as ID, option index as order — so a milestone is nothing but
  its name. It has no status of its own: `domain.NewProject` computes it from
  the slices under it — Queued until one starts, Active while any is in progress
  or only some are Done, Done when they all are — so there is nothing on a
  milestone to write, and the board has no key that would.
- A project still in the old shape — a Milestones database with a `Milestone`
  relation on the Slices data source, and/or a `Claimed` status option — is
  migrated in place the first time it is loaded, by the board and by every
  headless command alike (`notion.MigrateProject`, run from `tui.fetchProject`
  and `cli.slicesDataSource`): the milestone pages become the options of a
  `Milestone` select, in plan order; every slice is refiled onto the option its
  relation named; and the Milestones database, its plan now wholly on the
  column, goes to Notion's trash — recoverable, and only after everything else
  has succeeded. The Slices database itself — a full-page child of the project
  page, its first view the table the plan's order is read from — is left
  exactly as it is. `Claimed` becomes `In progress` the long way — appended,
  the slices holding it moved over, then dropped — because the API silently
  ignores renaming an option in place (a 200 whose body still says the old
  name). The whole migration is settled before the first write — a `Status`
  column converted in the Notion UI cannot have its options written, and such a
  project is refused with the one edit to make there rather than half-migrated,
  as is one whose milestones data source names no database to trash — and the
  plan is read in full before the schema changes, because converting the column
  is what discards the relations it is read from. It is idempotent: a project
  already in the one shape is read and left alone, which is what every load
  after the first does. What changed is logged, and the board says so in a
  toast.
- Filing a slice under a milestone — the board's `a` and `m`, `slice-add`,
  `plan-apply` — writes the option naming it (`domain.Milestone.Ref()`, sent as
  `SelectType`, the column's own type). `milestone-add` and `plan-apply` add
  milestones by appending options to the `Milestone` column, one schema write
  per run, after the options already there, since their order is the plan's
  order. A name the plan already holds is refused before that write, because
  such a milestone is nothing but its name — and for the same reason a milestone
  is named by name alone, never by URL or ID. `next-slice` reads the plan the
  way the board does and takes work from the lowest-ordered milestone that is
  not Done: a milestone is Queued until a slice under it starts, so gating on
  Active would leave a plan on which nothing has begun with no way to begin.
  `start-slice` names a slice's milestone from the schema's options, and
  `complete-slice` touches no milestone at all.
- Slice order is where the slices sit in the project's own board:
  `notion.PlanOrder` reads the row order of the Slices data source's first view
  (`GET /views`, then `POST /views/{id}/queries`, which is exposed from
  `2026-03-11`) and `domain.InViewOrder` applies it, with anything the view does
  not name trailing in the order it was queried. A failure to read it is logged
  and the plan drawn unordered rather than not at all. Notion records
  `created_time` only to the minute, so the created-time sort every other read
  uses is no order at all for a plan written in one go — which is why this one
  is read. `next-slice` reads it too, so the slice it hands out is the one at
  the top of the milestone on the board rather than whichever the query happened
  to return first.

## Conventions

- Bubble Tea v2 idioms: `View()` returns `tea.View`; match `tea.KeyPressMsg`;
  `tea.ExecProcess` for tmux attach.
- Tests: aim for 100% coverage of new code. httptest for the Notion client
  (assert exact request JSON), interfaces + fakes for the ntn CLI/tmux, teatest
  for TUI flows, golden snapshots for renders.
- Gate before claiming done: `go vet ./... && go test -race -cover ./... &&
  golangci-lint run`. All three run in CI, so a failing lint fails the PR;
  `brew install golangci-lint` if the binary is missing. `.golangci.yml` runs
  the default linter set with one exclusion — see the file — so an unchecked
  error is either handled or assigned to `_` where the reason can be read.
- Never log or commit the Notion token; it belongs to the `ntn` CLI and is only
  ever held in memory for the lifetime of a request.
- Before starting work, pull the latest `main` and branch off it. Only ever
  base branches — and PRs — on `main`, never on another slice branch, so every
  PR merges into `main`.
