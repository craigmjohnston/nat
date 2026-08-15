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
- `internal/agent/` — agent prompt template + tmux session management. A
  running agent is identified by its pane's `@nat_slice` option (the full slice
  page ID); the session name `nat-<last-8-hex-of-slice-page-id>` is only a
  human label, and takes the tail because page IDs share a leading prefix.
  `ShowPane` toggles that pane between the board's window (`join-pane`, sized by
  `agent_split_percent`, default 65% to the agent) and a session of its own.
  A joined pane dies with the board's window, so `BreakOutJoined` frees every
  such pane as the app leaves (deferred in `main`, so a panic frees them too)
  and `ReclaimStrays` re-homes on startup whatever an earlier run left joined.
- `internal/cli/` — the headless commands (`nat info [--json]`,
  `nat next-slice [--json]`, which claims the next Todo slice under the
  lowest-ordered open milestone and prints its brief,
  `nat start-slice <slice> [--json]`, which claims one named Todo slice and
  prints the same brief — the command a board-launched agent runs, since it is
  pointed at a slice rather than choosing one —
  `nat complete-slice <slice> [--pr URL] [--summary TEXT] [--blocked]`, which
  closes out a slice the configured user holds, and the one-off additions
  `nat milestone-add <name>` (Queued, at the end of the plan) and
  `nat slice-add <title> --milestone <name|URL|ID> [--description TEXT|-]
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
- `internal/tui/` — bubbletea v2 (`charm.land/*/v2` imports); root model in
  `app.go` routes screens; all Notion I/O via tea.Cmd → typed msgs
- `skills/` — the agent skills (/queue-work planning, /next-slice execution),
  embedded in the binary with `go:embed` and installed by `nat setup`. A
  checkout works on them in place by symlinking them into `~/.claude/skills/`,
  which `nat setup` leaves alone rather than writing back through.

## Domain rules

- Slice workflow: Todo → in progress → Done. Never edit an in-progress or Done
  slice.
- The in-progress status has two names: projects created before the app asked
  call it `Claimed`, newer ones `In progress`. Nothing is migrated —
  `notion.ShapeOf` reads the name (and whether the project has an `Assignee`
  column at all) off the Slices data source, and `domain` maps both names onto
  the one `SliceClaimed` status so the board and the progress math ask one
  question.
- Claiming = Status → the project's in-progress option, plus Assignee (people
  property, the configured real Notion user) where the project has that column.
  Without one, ownership is decided on status alone.
- Slices ↔ PRs are 1:1 when work is code; PR URL recorded in the `PR` property.
- Slices may carry a `Repo` override; otherwise the project default working
  dir from local config applies.
- Milestones live in their own DB (Name/Order/Status: Queued/Active/Done);
  slices relate via the `Milestone` relation. A project may instead keep its
  whole plan on one page: no Milestones DB, and a `Milestone` **select** on the
  Slices data source whose options are the milestones, in plan order. Nothing is
  migrated — `notion.ShapeOf` reads which shape the Slices data source has, and
  a plan read from a select becomes the same `domain.Project` (via
  `domain.MilestonesFromOptions`, milestone name as ID, option index as order,
  no URL), so the board and the progress math ask one question. Such a
  milestone is marked `Derived` and has no status of its own:
  `domain.NewProject` computes it from the slices under it — Queued until one
  starts, Active while any is in progress or only some are Done, Done when they
  all are — and nothing can be written to it, so the board's Q refuses it, the
  hints row leaves it off a derived milestone's row, and the help screen leaves
  it off a plan with no milestone pages at all. Filing a slice under a milestone
  — the board's `a` and `m` — writes the `Milestone` column in the shape the plan
  is kept in: a relation to a page, or the option naming a derived milestone
  (`domain.Milestone.Ref()`, written as `SelectType` — the column's own type).
  The planning commands work either shape too: `milestone-add` and `plan-apply`
  create milestone pages under the relation shape, and under the select shape
  append options to the `Milestone` column instead — one schema write per run,
  after the options already there, since their order is the plan's order. A name
  the plan already holds is refused before that write, because such a milestone
  is nothing but its name. `slice-add` and `plan-apply` file slices with
  whichever `Milestone` value the shape calls for. The agent commands work
  either shape too: `next-slice` reads the plan the way the board does and takes
  work from the lowest-ordered milestone still open — Active under the relation
  shape, and under the select shape merely not Done, since a derived milestone
  is Queued until a slice under it starts and gating on Active would leave a
  plan on which nothing has begun with no way to begin. `start-slice` names a
  slice's milestone from the schema's options rather than fetching a page that
  does not exist, and `complete-slice` touches no milestone at all.
- Slice order under such a plan is where the slices sit in the project's own
  board: `notion.PlanOrder` reads the row order of the Slices data source's
  first view (`GET /views`, then `POST /views/{id}/queries`, which is exposed
  from `2026-03-11`) and `domain.InViewOrder` applies it, with anything the view
  does not name trailing in the order it was queried. A failure to read it is
  logged and the plan drawn unordered rather than not at all. Notion records
  `created_time` only to the minute, so the created-time sort every other read
  uses is no order at all for a plan written in one go — which is why this one
  is read. A project with a Milestones database orders its plan by their
  `Order` and reads no view. `next-slice` reads it too, so the slice it hands
  out is the one at the top of the milestone on the board rather than whichever
  the query happened to return first.

## Conventions

- Bubble Tea v2 idioms: `View()` returns `tea.View`; match `tea.KeyPressMsg`;
  `tea.ExecProcess` for tmux attach.
- Tests: aim for 100% coverage of new code. httptest for the Notion client
  (assert exact request JSON), interfaces + fakes for the ntn CLI/tmux, teatest
  for TUI flows, golden snapshots for renders.
- Gate before claiming done: `go vet ./... && go test -race -cover ./...`
  (plus `golangci-lint run` if installed).
- Never log or commit the Notion token; it belongs to the `ntn` CLI and is only
  ever held in memory for the lifetime of a request.
- Before starting work, pull the latest `main` and branch off it. Only ever
  base branches — and PRs — on `main`, never on another slice branch, so every
  PR merges into `main`.
