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
  opts out. The board is a pane so the tmux bar under it can draw the status
  line and so the agents it launches have a server to live in; viewing one no
  longer needs it — the terminal beside the board is nat's own, and there is
  nothing to hand back on the way out.
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
  Viewing an agent joins no panes: the board runs an attach client on a PTY of
  its own and draws it, so an agent's pane stays in the session it launched in
  and nothing here makes a stray. `ReclaimStrays` — with its private
  `breakOut`/`breakOutAll`/`placeholderCommand`, all under one deprecation
  comment — is the exception, still run at startup to re-home the panes a
  pre-upgrade nat left joined, and comes out next release.
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
  `Render`/`Cursor` to draw it, `SendKey`/`SendBytes`/`Paste` to type at it and
  `SendMouse` to click and scroll at it — a cell, a button, the modifiers held
  and a kind of event (press, release, motion, wheel), which the emulator
  encodes for the child's active mouse modes, or drops where the child has asked
  for no reporting; the modifiers are not decoration, since tmux binds
  `C-MouseDown1Pane` as well as `MouseDown1Pane` and a click stripped of its
  ctrl fires the wrong one,
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
  emulator's own `Close`, which races the reply pump. `internal/tui/agentview.go`
  is its one caller.
- `internal/tui/` — bubbletea v2 (`charm.land/*/v2` imports); root model in
  `app.go` routes screens; all Notion I/O via tea.Cmd → typed msgs. `poll.go` is
  the background refetch of the plan, for the changes no nudge reports because
  no `nat` command made them: a tick every `poll_seconds` (default 30, bounded)
  running the same load the refresh key does, so the plan stays on screen while
  it is in flight and a failure leaves the board as it was. It is passed over —
  never cancelled, so it resumes on the next tick by itself — while the wizard,
  a form, a row prompt, a write or another load is in flight, since a plan
  landing under any of those would clobber what the user is in the middle of. An
  open agent terminal never suspends it: the plan behind the split stays live.
  `agentview.go` is that terminal — `t` (and `w` for the planning agent) runs
  `AttachClientCmd` on a `vterm.Session` and draws it in a box beside the board,
  sized from `agent_split_percent`, so nat owns the whole rectangle while the
  agent goes on living in its own tmux session and survives a board restart. One
  is on show at a time; it opens unfocused (`tab` focuses, `ctrl+\` comes back)
  and while it is focused every key is the agent's — `ctrl+c` included, which is
  deliberate — bar the outer tmux prefix, which is swallowed. A key that stands
  for characters is written as those characters and only the rest goes to the
  emulator's key encoder, which writes a printable key only when it carries no
  modifier at all and so typed nothing for a capital letter or any shifted
  punctuation; ctrl is excluded, since a ctrl combination is a control byte
  however it was decoded. It is drawn only on
  the board and only in a framed window, and stays alive undrawn behind help,
  info and a form. Its redraw is capped at the renderer's own frame: the wait
  re-armed after a capture (`awaitFrame`) holds off `frameInterval` — 1/60s —
  before listening again, and the session coalesces everything the child wrote
  meanwhile into its one pending notification, so a burst costs one read of the
  emulator's screen and one render of the window per frame instead of one per
  write. The board beside it is drawn once and kept (`App.boardBox`, dropped by
  `syncBoard`, which everything that changes what the board shows goes through),
  so an agent writing flat out redraws only its own box.
  An agent that exits leaves its last frame on screen marked
  `exited`, and the after-viewing refetch — the slice's page, or the whole plan
  for the planning agent — happens once, whichever of the client's EOF and the
  live poll notices first. `T` is the hatch to the old full-screen attach. The
  mouse is the terminal's too: all-motion reporting is asked for on the view
  only while one is on show — with it off the user's own selection and
  scrollback are left alone, and there is nothing on screen a key does not reach
  — and an event inside the box is turned into a cell of the child's screen and
  handed to `SendMouse` with the modifiers it arrived with, a press there taking
  the keyboard as well. Everything outside the box is dropped, bar a press, which
  hands the keyboard back. Nothing between swallows it: tmux passes mouse
  reporting through unless its own `mouse` option is on, which nat sets for the
  sessions it makes for agents and never for the one it hosts the board in. What
  the agent's own tmux does with what arrives is unchanged by the hop: a click on
  an OSC 8 hyperlink fires the `MouseDown1Pane` binding and opens it, and a wheel
  enters copy-mode over the pane's scrollback and leaves it again on the way back
  down (tmux's stock `copy-mode -e`), exactly as they did through a joined pane.
  `boardmouse.go` is the other half of that, since reporting takes the mouse off
  the outer terminal over the whole window rather than over the terminal's box:
  what the viewer does not take, the board does. A left click selects the row
  the line it landed on belongs to — `Board.RowAtLine`, off the same `rowLines`
  the cursor's own span and the animation's re-sync are measured from — and the
  wheel scrolls the plan three lines a notch, dragging the cursor no further than
  the nearest row still whole on screen, because `syncBoard` would otherwise
  scroll straight back to it. A click on the PR chip opens the pull request:
  `Board.LinkAt` walks the drawn row for the OSC 8 hyperlink covering that cell
  and the app hands the URL to the platform opener (`agent.URLOpener`), which is
  what the terminal itself would have done with the click had nat not taken it.
  The board is deaf to all of it while the wizard, a form or a row prompt is up,
  for the same reason the keys are.
  `presence.go` is the star a slice with an agent on it is marked with: it
  pulses — the same star swelling and settling, one cell wide at every frame so
  the row does not shift under it — while the agent works, and holds a star of
  its own steady when the agent has stopped for input. Liveness is the live
  map's answer and the classification only refines it, so an agent that has gone
  has no star whatever was last read of it, and one nobody has classified draws
  as working — which is every agent until the activity watcher lands. The whole
  board animates off one timer, armed by the live read and stopping itself as
  soon as nothing is pulsing; each frame re-syncs the board, since its rows are
  cached in a viewport. Both the glyph and the colour move, because a selected
  row is drawn without any chip's styling.
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
