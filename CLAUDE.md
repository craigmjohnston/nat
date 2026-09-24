# notion-agent-tracker

A tracker for project work executed by Claude Code agents, in Notion (or a
local SQLite plan). It ships as three faces over one Go core: `gnat`, the
native macOS app Craig mostly drives it from; the `nat` headless commands,
which agents and the app both use; and the `nat` TUI board, still maintained
and kept in step with the CLI. Agents run as fresh `claude` sessions in tmux
and reach the tracker only through the headless `nat` commands — they need no
Notion access of their own. `nat` talks to the Notion REST API directly
(`Notion-Version: 2026-03-11`, data-source model); the app talks only to `nat`.

Package detail — implementation, seams, hard-won gotchas — lives in nested
`CLAUDE.md` files, one per package, loaded only when you're working in that
package. This file is cross-cutting rules: what's true everywhere, not how
any one package does it.

## Architecture map

- `main.go` — entrypoint (`github.com/craigmjohnston/nat`). A subcommand
  runs before even the tmux check; none of them launches an agent.
- `internal/config/` — XDG config + `AgentModel` (model/effort pairs for
  `workshop_agent`/`slice_agent`); unset halves contribute no flag.
- `internal/notion/` — the Notion client. See `internal/notion/CLAUDE.md`.
- `internal/store/` — the port between nat and wherever a plan lives
  (`Notion`, `Local`/SQLite). See `internal/store/CLAUDE.md`.
- `internal/domain/` — Project/Milestone/Slice models, `StateOf`, progress math.
- `internal/actions/` — headless claim/launch/approve/landed/worktree flow,
  shared by `internal/tui` and `internal/cli`. See `internal/actions/CLAUDE.md`.
- `internal/agent/` — prompt templates + tmux session management. See
  `internal/agent/CLAUDE.md`.
- `internal/gh/`, `internal/git/`, `internal/worktree/` — thin CLI wrappers.
  See each package's `CLAUDE.md`.
- `internal/vterm/` — PTY + VT emulator behind the embedded agent terminal.
  See `internal/vterm/CLAUDE.md` (the three hard-won gotchas — read before
  touching it).
- `internal/cli/` — the headless `nat` subcommands; the macOS app's whole
  backend contract, served by the `--json` command set (`info`, `status`,
  `slice-*`, `session-*`, `pr-*`, `milestone-*`, `plan-apply`, `config-*`,
  `usage`, …; `nat help` lists them). See `internal/cli/CLAUDE.md`.
- `internal/tui/` — the board. See `internal/tui/CLAUDE.md`.
- `internal/logging/`, `internal/nudge/` — the log file and the
  write-marker file the board polls every second for near-instant refresh.
- `skills/` — `/queue-work`, `/queue-project`, `/next-slice`, embedded via
  `go:embed`, installed by `nat setup`.
- `macos/` — `gnat`, the native macOS app (SwiftPM; `NatKit` logic, `NatApp`
  views), released as a signed dmg with Sparkle updates. `NatClient` is its
  one seam onto the CLI: it shells out to `nat <command> --json` and nothing
  in Swift reimplements tracker logic. See `macos/CLAUDE.md`.

**Never log or commit the Notion token or a request body.** The token
belongs to the `ntn` CLI (`ntn auth token`) and is held in memory only for
the lifetime of one request; nat stores no credential of its own.
`internal/logging`'s redactor is the enforcement point — don't add a logging
call that bypasses it.

## Domain rules

These hold everywhere in the app — the TUI, every headless command, and the
macOS app via `NatClient`. Package-local mechanics for each are in the
nested `CLAUDE.md` named alongside each rule; don't restate the mechanics
here when you're just applying the rule.

**Lifecycle.** Todo → In progress → Done. Never edit or move the milestone
of an in-progress slice; never edit a Done one either (`internal/tui/CLAUDE.md`,
`internal/cli/CLAUDE.md` — the exact per-action refusal differs: edit is
Todo-only, move/delete refuse only In progress). In-progress is called `In
progress`. There is one project shape: nat does not convert older ones at
load.

**Claiming.** Status → In progress, + Assignee where the project has that
column (status alone otherwise). The *board* claims before tmux is touched —
a fresh Claude Code takes seconds to reach `start-slice`, and a Todo row in
the meantime is one a second agent could be launched on. `start-slice`
re-opens a claim its own holder already made rather than re-claiming, and
refuses outright where the claim didn't stick — a race is settled before any
agent gets a brief. `nat slice-launch` is a third way to reach the same
`actions.Launch` flow (`internal/actions/CLAUDE.md`).

**Releasing** (`R` / `nat release-slice`) writes its note **before** flipping
Status back to Todo — the same order `complete-slice` writes in, since a
slice already back at Todo would refuse a note added to it. Assignee cleared;
everything else on the page (brief, `Depends on`, `Repo`, any `Branch`)
untouched. Refused on a slice with a live agent.

**Launching** (`l`) covers two states: Todo, and In progress with no live
session (a relaunch — placed back on `agentBranch`, told it's continuing).
A slice with a live agent is refused outright, whatever its status.

**Fix sessions** — a Done slice with a PR still open is launchable too
(`fixLaunch`): the review is unfinished work. This is the one launch that
**writes nothing at all** — no claim, since the slice is already everything
a claim would make it. Before any worktree is cut, `prStillOpen` asks gh
directly whether that PR is still open (a merged/closed/unreadable PR each
refuse the launch with a toast, and — unlike everywhere else the app reads
gh — an unread PR here refuses too, since the cost of being wrong is an
agent sent at a review that's already over). Dependencies are not checked.
`agent.fixPrompt` is what such a session is told, and it's the one place the
standing ban on agents running `gh` is relaxed — for exactly `gh pr view
--comments` and `gh pr checks`.

**Hand-back.** `complete-slice --branch` (or `/next-slice`'s own end)
records the branch and leaves status alone — refused outright on a project
with no `Branch` column, before the note goes on. The hand-back note and any
`--pr-description` are filed in the **same write**, under `Handed back` and
`PR description` headings; `notion.PRDescriptionOf` reads the *last* such
section, since a slice handed back twice has one per hand-back.

**Approving** (`a` on the diff screen, or `nat slice-approve`) opens the PR
and records only its URL — status stays In progress. **Done means the work
is on main**, and only the merge writes it: `m` / `nat pr-merge`, or
`actions.SettleMerged` (the board's background PR-state read, and headless
`nat pr-status`) catching a merge made on GitHub directly. A slice marked
Done under the *old* rule (Done written at approve, before this rewrite)
whose PR reads open is corrected the opposite way, lazily, one slice at a
time as each is next read: `actions.ReopenUnmerged` writes it back to In
progress. The macOS app mirrors this exact gate in Swift
(`RailModel.isReviewSlice`/`isActiveSlice` — see `macos/CLAUDE.md`); change
one side, change the other.

**Approving over comments** (gnat's diff tab, `nat slice-rework`): with
comments pending, approve sends them (`agent-send`, prompt ending in the
`complete-slice --branch` instruction) and takes the slice out of review by
clearing its `Branch` — no PR opens. The agent's re-hand-back re-records the
branch, and `AppModel.settlePendingApprovals` (in-memory mark, armed only once
a refresh has *seen* the slice un-handed-back) then runs `slice-approve`. A
lost mark degrades to a normal review.

**Worktree lifecycle.** A slice's worktree is removed **only** on merge,
witnessed once at the transition and swept again (idempotently) on every
plan load as a retry. Both the launch's placement and the merge's removal
name the checkout by `actions.AgentBranch` (the branch recorded at
hand-back, else the derived `slice/<slug>`) — the two must never disagree.
An existing branch's worktree is reused, never re-cut; a removal git refuses
is logged and left, since the PR is merged either way. `R` deliberately
*keeps* the worktree — the work so far is what the next session wants.

**Dependencies.** `Depends on` is a dual-property relation (`Blocks` is its
unread reciprocal, there only so Notion has somewhere to mirror the far end
— see `internal/notion/CLAUDE.md` for why a single-property version would
read as a mutual block). A slice is blocked while anything it names isn't
Done; an unreadable dependency is logged and never counted, so a trashed
page can't wedge the plan forever. A write that would leave a cycle is
refused before it happens (`plan-apply`, `slice-depends --on`); a cycle
already on the board is reported as one, not an ordinary wait, everywhere it
matters (status line, launch refusal, `next-slice`). `next-slice` steps over
a blocked slice; `start-slice`, pointed at one slice, refuses it by name.

**One state, one source.** Notion's status is the *only* source of
lifecycle truth (`domain.StateOf`). A Done slice is Done, full stop, and is
never re-derived into some other state even with a stale open PR — that's
`ReopenUnmerged`'s job to *correct on Notion*, not `StateOf`'s to paper over
by reading around it. For a slice still in progress, state is read in the
order the facts are true in: a live agent (freshest reading) beats
everything else on the page; then handed-back-but-not-agent work (a
`Branch`/`PR`); then a dependency wait; then plain "in progress, nothing
happening." `domain.AgentPresence` and `domain.PRReadiness` fold the board's
tmux and gh readings into this rule — their zero values mean "no PR / never
read / no longer open" indistinguishably, on purpose: nowhere here needs
those three told apart. Absent a gh reading at all, the refinement is simply
absent, not wrong — that's what keeps a Done slice with no PR-state read out
of the Active panel rather than flooding it with a project's entire history.

**`--project` pinning.** Every project-scoped `nat` command requires
`--project <page ID>`, no active-project fallback — every template (slice,
fix, planning prompts) and every skill spells this out explicitly, and one test walks every template for an unpinned invocation. The
`SliceBranch`/`pathSlug`/`Base` naming triad (how a branch name and its
worktree path are derived — implemented once, in `internal/actions`,
`internal/worktree` and `internal/git`) is **re-spelled in prose twice**:
in `internal/agent`'s prompt templates and in `skills/next-slice/SKILL.md`.
**Never deduplicate this** — a prompt and a skill are both text handed to an
LLM, not code, so neither can call the Go implementation; both copies must
independently say the same thing.

**Milestones.** A `Milestone` select column, options in plan order — a
milestone is nothing but its name, never referenced by URL or ID. Renaming
one goes the long way (Notion silently ignores an in-place option rename;
see `internal/notion/CLAUDE.md`); removing one refuses while any slice is
still filed under it; moving one changes only its place among the options,
reading and writing no slice at all.

**Projects with no workspace.** A project's config entry carries an optional
`backend` (`local`, else Notion — the empty string and any word a later nat
invented read as Notion, since only `local` is one this build can open a file
for) and, for a local one, a `plan_dir`. Both are omitted until they mean
something, so an old config round-trips unchanged. A local project's ID is
nat's own (`store.NewProjectID`, page-ID-shaped so nothing carrying one can
tell the two apart), its plan is the SQLite file alone (`store.ForProject`
returns the `Local` with no `Mirrored` and no client), and it needs no Notion
token on any path: startup fetches one only where `Config.UsesNotion`, and
who works its slices is `Config.AssigneeFor` — the name *is* the identity,
falling back to whoever is logged in. Creating one writes the plan file
**before** the config entry (`nat project-create --local [--plan-dir]`; the
board's `N`, which asks where the plan lives only when a projects database
gives a choice). `config-show` says every project's backend. gnat's
`ProjectConfig` / `ConfigDocProject` decode all of it and tolerate a missing `slices_ds_id`.

**Plan order.** Read from the Slices data source's first view's own row
order (`notion.PlanOrder`), never from `created_time` — Notion records that
only to the minute, which is no order at all for a plan written in one
sitting. A failed read logs and draws the plan unordered rather than not at
all.

**Reads that fail conclude nothing** — the default posture everywhere in
this app: a failed `OpenPRs` listing is no news (never read as "merged" or
"closed"), a failed `Fetch` cuts from refs as last known, an unreadable
dependency never blocks. The one deliberate exception is the diff screen: a
failed re-read of a handed-back branch **drops** what was on screen, since a
diff is of one branch at one moment and an old one under a fresh push would
be showing the wrong change (the pull request screen's own failed re-read
does the opposite — keeps its stale reading, since a stale PR view is still
about the right PR).

## Conventions

- Read files with the Read tool, not `cat`/`sed`/`head` through Bash — Read
  handles offsets for files too big to read whole, where a shell read has to
  choose a fixed window up front or bring back the whole file. Edit files with
  Edit or Write, not a shell heredoc — a heredoc edit re-transmits the whole
  old block and the whole new one, which roughly doubles edit-phase output on
  a codebase whose house style is this much comment prose, since every touch
  re-quotes it. The shell is for running things — tests, git, the verification
  gate — not for reading or editing files.
- Bubble Tea v2 idioms: `View()` returns `tea.View`; match `tea.KeyPressMsg`;
  `tea.ExecProcess` for tmux attach.
- Tests: aim for 100% coverage of new code. httptest for the Notion client
  (assert exact request JSON), interfaces + fakes for the ntn CLI/tmux,
  teatest for TUI flows, golden snapshots for renders.
- Gate before claiming done: `go vet ./... && go test -race
  -coverprofile=coverage.out ./... && ./scripts/no-uncovered.sh &&
  golangci-lint run`. Use the profile, not `-cover`'s rounded percentage —
  `scripts/no-uncovered.sh` reads it exactly and prints any block nothing
  ran; one `go test ./...` writes it, since coverage merges across packages.
  `brew install golangci-lint` if missing.
- A macOS UI change is verified by rendering its gallery, not launching the
  app — see `macos/CLAUDE.md`/`macos/README.md`.
- Before starting work, pull the latest `main` and branch off it. Only ever
  base branches — and PRs — on `main`, never on another slice branch, so
  every PR merges into `main`.
