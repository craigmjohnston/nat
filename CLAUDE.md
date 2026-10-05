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
- `internal/source/` — the task-source plugin protocol — external
  `nat-source-<name>` binaries own the containers a source project's tasks
  hang off. See `internal/source/CLAUDE.md`.
- `internal/plugins/` — installs, updates and uninstalls those binaries from
  plugin sources' GitHub releases (`nat-plugins.json`), behind `nat plugin-*`.
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
  `source-*`, `plugin-*` — `source-setup` reading a plugin's token from stdin
  only — `usage`, …; `nat help` lists them). See `internal/cli/CLAUDE.md`.
- `internal/tui/` — the board. See `internal/tui/CLAUDE.md`.
- `internal/logging/`, `internal/nudge/` — the log file and the
  write-marker file the board polls every second for near-instant refresh.
- `plugins/shortcut/` — `nat-source-shortcut`, the Shortcut task-source
  plugin: its own binary (`go install ./plugins/shortcut`), reusing
  `internal/source`'s types, never imported by nat. See
  `plugins/shortcut/CLAUDE.md`.
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

**Fix sessions** — a slice with a PR recorded (approved and In progress, or
Done under the old rule) whose PR is still open is launchable too
(`actions.FixLaunch`, the one discriminator the board's `l` and `nat
slice-launch` both ask): the review is unfinished work. It **claims
nothing** — the slice is already everything a claim would make it — and
writes one thing: a `Relaunched` (a failure logged, never fatal), which is
what puts the return to work on the record. Before any worktree is cut,
`actions.PRStillOpen` asks gh directly whether that PR is still open (a
merged/closed/unreadable PR each refuse the launch, and — unlike everywhere
else the app reads gh — an unread PR here refuses too, since the cost of
being wrong is an agent sent at a review that's already over). Dependencies
are not checked. `agent.fixPrompt` is what such a session is told; it ends
in a hand-back (`complete-slice --branch`, the same branch, status left
alone), and it's the one place the standing ban on agents running `gh` is
relaxed — for exactly `gh pr view --comments`. **`fixing`** is read off the
record (`store.Fixing`): In progress, PR recorded, and the latest task-log
event a `Relaunched` or `Sent back`; the hand-back that follows ends it.
`info --json` and `slice-show --json` carry it per slice.

**CI failures.** Every agent reads CI with `nat slice-checks <slice> [--log]`
(slice prompt, fix prompt, `/next-slice`), never `gh`. After every PR
listing, `nat pr-status` and the TUI's `refreshPRStates` hand each slice
reading `PRChecksFailing` to `actions.NoticeFailingChecks`: a failure is the
set of its failing checks' run URLs, news only where the latest `Checks
failed`/`Sent back` names a different set. A live session is sent
`agent.ChecksPrompt` then a `Sent back` is filed (Branch left alone); with
none, a `Checks failed` is filed. Send before record: a failed send writes
nothing, so the next reading retries.

**tmux is the user's.** Every agent nat launches runs on the user's own tmux
server, beside every other agent, so every prompt and `/next-slice` carry a
standing rule: never `tmux kill-server`, never kill, detach or send keys to a
session the agent did not create, and a tmux of its own goes on a private
`-L` socket (`TMUX_TMPDIR` does not isolate a process with `$TMUX` set).

**Hand-back.** `complete-slice --branch` (or `/next-slice`'s own end)
records the branch and leaves status alone — refused outright on a project
with no `Branch` column, before the note goes on. The hand-back note and any
`--pr-description` are filed in the **same write**, under `Handed back` and
`PR description` headings; `notion.PRDescriptionOf` reads the *last* such
section, since a slice handed back twice has one per hand-back.

**Follow-ups.** Before hand-back, a gnat-launched agent hands in work it
noticed but didn't do with `nat slice-followups` and stops; the proposals are
a `Follow-ups` section of the slice body (numbered items), and the user's
decision a later `Follow-ups triaged` section (bullets) — no column, both
stores write the same markdown. `store.PendingFollowUps` reads what's still
undecided; `complete-slice` refuses while any is (`--blocked` exempt).
`nat slice-triage` decides every pending index at once (queue → Todo slice
under the parent's milestone, blocked on it; fold in → needs a live agent;
drop), writes the record **before** its one `agent-send`. Only the gnat
prompt and `/next-slice` carry the passage — the TUI has no triage surface.
gnat's `FollowUpsSidebarView` is pane-level, shown while `slice-show`'s
`followUps` is non-empty. Design: `docs/design/follow-up-triage/`.

**Notes.** `nat slice-note <slice> --note TEXT|- [--from <slice>]
[--milestone NAME]` appends a `Note` section to a Todo or In progress
slice's body — for context a *later* slice needs, never for work (that is a
follow-up). The target is named by name (plan-apply's title match,
`--milestone` to disambiguate) or by ID/URL; no ownership check. Under its
stamp, its first paragraph is provenance nat composes, never the caller:
`From "<slice>" (<milestone>)` from `--from` (`store.SliceLabel`, parsed
back into `slice-show`'s `fromSlice`), else `From <Config.AssigneeFor name>`
— no ID or URL, by the same helper (`fromSlice`) the triage's
queued-follow-up line uses. Refused, before any write: Done, an unreadable body, an empty note, an
unknown/ambiguous name, an unreadable `--from`. A note on a Todo slice
reaches its agent as part of the brief with no further plumbing; one on a
slice with a live agent is **written before it is sent** into that session
(`agent.NoteArrivedPrompt`, one `agent-send`, as the triage sends after its
record) — except a note whose `--from` is the target itself, never sent back
to the agent that wrote it.

**Naming slices.** Every text handed to an agent that writes about slices
(slice, fix, plan and new-project prompts; every embedded skill) carries one
rule: refer to another slice only by name (+ milestone where ambiguous),
never by number, index, page ID, URL or another tracker's id. Tests walk
every template and every skill for it.

**Task log.** A slice's history is read off its body, in order, by
`store.TaskEvents`: each `Handed back`, `Sent back` (`slice-rework
--comments`, filed before the branch is cleared, as hand-back files before
its property; or a checks nudge, which clears nothing and opens with a
`From CI` line read back as its `by`), `Checks failed`
(`checks_failed`, a red reading with no live agent), `Launched` (written by
every other non-fix `actions.Launch`, the log's first word and its time —
status alone never makes a launch a relaunch), `Relaunched` (written
by a fix launch, and by a non-fix `actions.Launch` of a slice with
history — `store.HasHistory`: notes alone are not history; either line's
failure is logged, never fatal),
`Blocked`, `Summary`, `Note` (a `note` event, `by` its provenance), released
line and `Follow-ups` section, each proposal
decided by a later `Follow-ups triaged`. Every one of those sections opens
with a stamp paragraph, `At <RFC 3339 with offset>` (the released line says
`… by <name> at <RFC 3339>: …` instead; `PR description` is never stamped,
being the PR's body); a section without one predates stamps and reads at no
time. `slice-show --json`'s `events` is that list (each with its `at`), then
`approved` (a PR recorded) and `merged` (Done with a PR or branch), which
have no time. Both stores write the same markdown, through a `Clock` seam.

**Visual changes.** Where a project already has a cheap or usual way to
render what a slice changed (a gallery story, a screenshot script), every
slice agent — slice prompt, fix prompt, `/next-slice` — hands the images in
with `nat slice-visuals`, and is told never to build a way to render where
there is none. The command is incremental — `--visual` adds or replaces by
name in place (keeping its before), `--before` gives a visual its before,
`--remove` drops one — but storage is not: each hand-in appends a whole
`Visual changes` section holding the resulting set, and the **last section
wins**. Each item is a numbered name with, indented under it, its URI first
(what an older reader still reads), then optional `sha256: <hex>` (local
files only, hashed at hand-in), `Before: <uri>` and `Before sha256: <hex>`
lines — paragraphs nested under the item in Notion; both stores write the
same markdown, and an empty section reads as no visuals. `changed` is
derived, never stored: an item differing (by hash, else URI, before
included) from the section before's item of that name, or with none.
`slice-show`'s `visuals` reads them (`store.VisualChanges`); gnat's Visual
changes section shows them, and its comments go back by `agent-send`, then
`slice-rework` only where the slice is handed back. Nothing blocks hand-back
on them. A slice you hold may hand them in, and so may a Done one with a PR
recorded, assigned to you — a fix session's.

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
in `internal/agent`'s slice prompt (`repoPassage`, told to a source
project's task that has no repository yet) and in
`skills/next-slice/SKILL.md`.
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

**Task sources.** A source project is a local plan with `backend: source`
and `source: <plugin name>` in its config entry; its milestones are the
plugin's *containers* (a Shortcut card, say), keyed by the plugin's id. nat
owns the tasks; the containers are the plugin's — nat never adds, renames,
removes or moves one, and never moves a task between them (each refused in
`store.Sourced`'s own words). Every task is filed under a container
(`slice-add --container`; `--milestone` refused there). The plugin hears of
a task's lifecycle by events sent **after** nat's own write, logged and never
fatal; request and response bodies are never logged — only method, ids and
exit code. `info --json` carries a `source` block (describe + sidebar tree)
and appends a synthesized `_unlisted` group of every container with tasks
that the plugin's tree leaves out, from nat's cached title, so a task never
vanishes; a plugin that fails or is missing concludes nothing — the project
still opens and `source.error` says why. gnat talks only to `nat`, and
agents never see the plugin. **A source project has no working directory**
(`project-create --source` records none): the repository is each task's own
`Repo`. A task launched with none (`actions.RepoUnknown`) starts in the home
directory with no worktree or git read, and its prompt sends the agent to
work the repository out from the card, ask the user where it cannot tell,
record it with `nat slice-repo`, and cut its worktree by nat's own naming;
from then on every path finds it through `actions.WorkdirFor`. A new task on
a card starts from the repository of the card's latest task with one. **A
source project's name is its plugin's `describe` title** (else the plugin's
name), read fresh wherever nat or gnat names it — config holds none, and a
`name` an older entry carries is ignored. gnat
makes a plugin's one source project itself the moment the plugin is
connected (every `describe` setup field set) — there is no new-project entry
for one. The protocol and the `nat` contract are specified in
`docs/design/task-sources/README.md`.

**Plugin install.** `nat plugin-install`/`-uninstall` (and gnat's Settings
▸ Sources over them) put a plugin under `<config dir>/plugins/<name>/` from a
plugin source's release, checked against its manifest's sha256, with an
`installed.json` beside it. A plugin directory with no such record was put
there by hand and is **never overwritten**; uninstall is **refused while any
project uses the plugin** (naming them) unless `--delete-projects` — which
gnat passes only once the user confirms, naming them — deletes those projects
(plan file, then config entry), and for one found only on PATH. nat's
own repo is always the first source and **can't be removed**; a source that
can't be read is its `error`, never "no plugins". Format and contract:
`docs/design/task-sources/README.md`, "Installing plugins".

**Plugin setup.** A plugin's credentials are its own; nat never stores one.
A plugin declares what it needs as `describe`'s `setup` fields (`describe`
must answer with no credential), gnat draws them under the plugin in
Settings ▸ Sources, and `nat source-setup <plugin> --id <id>` relays the
value to the plugin's `setup` method **on stdin, never argv** — and no log
line, error or gnat request log carries it.

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
  teatest for TUI flows, golden snapshots for renders. Shared plumbing is
  tested once, where it lives — a command's tests cover only its own
  statements and refusals.
- Gate before claiming done: `go vet ./... && go test -race
  -coverprofile=coverage.out ./... && ./scripts/no-uncovered.sh &&
  golangci-lint run`. Use the profile, not `-cover`'s rounded percentage —
  `scripts/no-uncovered.sh` reads it exactly and prints any block nothing
  ran; one `go test ./...` writes it, since coverage merges across packages.
  `brew install golangci-lint` if missing.
- While iterating, run only the tests for what you are touching — one
  package, `go test -run <Name>`, `swift test --filter <Name>` — never the
  full suite or the gate above mid-loop. Run the full gate once, immediately
  before hand-back; if it fails, fix with targeted runs and run it once more.
  Batch a stage's edits and build once per batch, not once per edit.
- A macOS UI change is verified by rendering its gallery, not launching the
  app — see `macos/CLAUDE.md`/`macos/README.md`.
- Before starting work, pull the latest `main` and branch off it. Only ever
  base branches — and PRs — on `main`, never on another slice branch, so
  every PR merges into `main`.
