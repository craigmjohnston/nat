# internal/cli

The headless `nat` subcommands: what an agent runs against its own slice,
what skills run to plan and queue work, and the macOS app's entire backend
contract (`NatClient` shells out to `nat <command> --json`). Runs before the
tmux check, with no TUI code in the path — a command prints to the terminal
it was typed in and exits. `setup`, `project-create`, `source-list`,
`source-setup` and the `plugin-*` commands act on no already-tracked project.

`plugin-list`/`-install`/`-uninstall`/`-source-add`/`-source-remove` are thin:
flags, `Env.Load`, then `internal/plugins` through `Env.NewPlugins` (a
`*plugins.Manager`; tests point one at an httptest TLS server). The source
edits need a config that exists and `Save` it; the rest read a missing
config as no extra sources and no projects. `plugin-list` then describes
each installed plugin through `Env.NewSource` (`describeInstalled`): its
entry gains `setup` (always an array) and `describe_error` — the plugin's
own first stderr line (`*source.ExitError`), else nat's error — so gnat
draws a setup form and why a plugin is broken from one read. (`describe`
needs no credential, so a missing token is not a describe failure: the
Shortcut plugin's "token missing" shows up in a source project's
`source.error`, not here.)

`source-setup <plugin> --id <id>` reads the value from **stdin only** (all
of it, one trailing `\n`/`\r\n` trimmed — never a flag, so a token is never
in argv), after describing the plugin and refusing an id its `setup` list
doesn't name; an empty value is refused too, all before `Client.Setup` is
called. `--json` → `{"message"}`.

## Per-project base branch

Every git driver a project-scoped command builds is `Env.gitFor(project)` —
`NewGit()` given the project's `base_branch` (`git.CLI.WithBase`, through the
`baseConfigurable` assertion; a fake that is not a `git.CLI` keeps its own
base) — so worktrees (`slice-launch`, `session-launch`), `slice-diff`,
`session-diff`, `slice-show`'s `base`, `pr-status`'s branch conflict test and
`run`'s checkout all read it. `slice-approve` passes it to `gh pr create
--base`; `pr-merge` passes `merge_method`/`delete_branch`; `info --json`
carries it as `project.base_branch`.

## `--project` pinning

Every command that touches a tracked project requires `--project <page ID>`
(`projectFlag`). There is no active-project fallback — the active project is
the *board's* idea of where the user is looking, and a headless write that
fell back to it would land wherever the board had scrolled to rather than
where the caller meant.

- `Env.projectFor(ref)` is the one place this is resolved: `noProject` (ref
  empty) and `namedProject` (ref given) both refuse with `knownProjects` —
  every project this machine tracks, by ID — rather than silently reading
  nothing.
- An ID is matched as typed, then normalised (`domain.NormaliseID`, dashes
  and case stripped — an ID copied from a page URL has neither).
- `config-set`'s `project.<id>.*` keys — `.working_dir`, `.runs` (a JSON
  array, `config.ValidRuns`) and `.color` (a `config.ProjectColors` name, or
  `auto` to clear it so the save picks one; anything else, the empty string
  included, a usage error; the report names what auto chose; refused for the
  scratch project and a source project, which take none — `Config.Colorable`)
  and `.name` (trimmed; empty a usage error, refused on a source project,
  which its plugin names), and `projectFieldSetters`' keys —
  `.slice_agent.model`/`.effort` and `.workshop_agent.model`/`.effort`
  (written as given, each half over the global pair through
  `Config.SliceAgentFor`/`WorkshopAgentFor` at every launch), `.merge_method`
  (merge/squash/rebase, else a usage error), `.delete_branch` (true/false),
  `.base_branch` (trimmed) and `.tag` (`config.NormaliseTag`: 1–3 letters or
  digits, uppercased, reported as stored) — do their own
  copy of this match (`projectKeyFor`) against the config already in memory,
  rather than calling `namedProject` and re-reading the file they write back.

`Env.projectFor` returns the config with its assignee fields already resolved
for the project (`Config.AssigneeFor`), and `Env.storeFor` builds no Notion
client for a local project — so `project-create --local`, and everything run
against such a project (`slice-status` included, which reads the plan file
instead of a page), works with no credential.

## Source projects

A source project (`backend: source`, `source: <plugin>`) is a local plan
file whose milestones are a task-source plugin's containers — see
`internal/source/CLAUDE.md`, `internal/store/CLAUDE.md` (Sourced) and the
spec, `docs/design/task-sources/README.md`.

- `Env.storeFor` has three arms: local (file alone), source (file wrapped in
  `store.Sourced` over `Env.NewSource(project.Source)`, the plugin told
  `source.Project{ID, Name, WorkingDir}`), else Notion. **Neither of the
  first two builds a Notion client.** A plugin `NewSource` can't find is
  logged and replaced by `source.Unavailable{err}` — the project still opens,
  every plugin read failing in its place. `DefaultNewSource` is `source.Find`
  under `config.Dir()` → `source.New`.
- `sourceStore` (Store + Describer/SidebarReader/ContainerReader/ActionRunner)
  is what `sourceStoreFor` hands back, refusing a non-source project by name
  before its plan is opened; `info`/`slice-show`/`slice-add` type-assert
  instead, since they serve every project.
- `source-list` (no `--project`): `source.Discover(config.Dir())`, each
  described best-effort through `describePlugin` (`NewSource` +
  `describeSource`, an empty envelope and a protocol check); a failure is the
  entry's `error`, never a dropped entry.
- `container-show <id>`: the plugin's `ContainerDetail` as-is + the plan's
  slices under it through `sliceJSONOf` (info's builder). A failed plugin
  read is the command's error.
- `source-action --action [--group|--container] [--input|-]`: the input
  through `briefText` (stdin on `-`), nudge on success.
- `info --json` (`sourceInfo`): describe, then — only if that worked —
  sidebar with `--expand`; then nat's `_unlisted` group (`unlistedGroups`):
  every plan milestone with a slice whose id is nowhere in the tree, titled
  from the cached name, omitted when empty. A failed read is `source.error`,
  nouns default to container/task, `groups` keeps `_unlisted`.
- `slice-show --json`'s `container`: the plugin's detail, falling back to id
  + cached title (logged) on a failed read; absent for other projects.
- `slice-add --container <id>`: required on a source project, where
  `--milestone` is refused; refused anywhere else. An id already in the plan
  is filed under by its cached title; a new one is read for its title (blank
  → the id), and a failed read refuses the add.
- `project-create [<name>] --source <plugin>` (exclusive with `--local`,
  `--repo` refused with it): `describePlugin` before any write, then
  `createPlanProject` — the same file-then-config path `createLocalProject`
  takes — with **no working directory** (each task's repository is its own)
  and **no name**: a source project is named by `sourceProjectName` (the
  plugin's describe title, else its name) wherever nat names it —
  `projectFor` fills it, `info` puts it over a name the plan file holds,
  `config-show` and `knownProjects` through `withSourceNames` (a copy,
  never saved), `plugin-uninstall` passes it to `Manager.Uninstall`.
- `slice-repo <slice> --repo <dir>` (`slicerepo.go`): records a task's
  repository through `store.RepoSetter` (a plan of nat's own; a Notion
  project is refused by name). A Todo slice takes it from anyone, one in
  progress only from its holder (`notOursError`), Done never; the path is
  `~`-expanded, made absolute and must be a directory (`repoFlagDir`). What
  a source project's agent runs once it has worked out its card's
  repository: after the write (and nudge) it finds or cuts the slice's
  worktree there (`ensureWorktree`, below) and prints it — `Worktree:
  <path>`, JSON `worktree`. A cut git refuses is the error, the repository
  left recorded.
- `slice-worktree <slice> [--repo <dir>] [--json]` (`sliceworktree.go`, any
  project, any status, no ownership check — it writes nothing to the plan):
  `actions.EnsureWorktree` in `--repo`, else `WorkdirFor` (`~` expanded),
  through `NewWorktrees` and `gitFor` (the project's base) — prints the
  path alone, JSON `{path, branch, base, created}`. No repository at all,
  or git refusing (outside a repository included), is the error in git's
  words. `/next-slice` step 2 is this command.
- `info --json`'s `source.menu` is the `sidebar` response's header menu
  where the plugin sent one, else `describe`'s.
- Refusals: `project-mirror` and `done-clear` refuse a source project by
  name (no Notion column for containers; a cleared Done task would tell the
  plugin `deleted`); `slice-status` takes the local path.

## Deliberate duplication — ports, not calls

`internal/cli` must not import `internal/tui` (bubbletea/huh/lipgloss/glamour
for a package that draws nothing) or `internal/tui` import `internal/cli`.
Where both need the same logic, it is ported by hand and kept level by
comment, never shared by refactoring into a common import:

- `difftokens.go` ports `internal/tui/diffsyntax.go`'s lexer-match /
  line-shape / token-kind rules, for `slice-diff --json`'s `tokens` field —
  same three-line prefix classification (`lineShapeOf`), same chroma kind
  mapping, wire form is `[kind, length]` pairs instead of styled runs.
- `actions.MergeRefusal` (see `internal/actions/CLAUDE.md`) is itself the
  port of `internal/tui/prmerge.go`'s `mergeRefusal`; `pr-merge` calls it
  directly rather than re-deriving it, so cli's refusal wording and the
  board's merge-box wording can never drift against each other through this
  path — only the tui↔actions copy needs hand-syncing.

## Command reference

Reads only, no `--project` needed: `setup` (installs skills, talks to
neither Notion nor config), `paths` (prints config/log/nudge paths; with
`--project`, matched by `namedProject`, also that project's plan file —
`store.PlanPath`, `plan` in JSON — none for a Notion project — and
`default_base`, its working directory's `Base` with no configured base, by
name: gnat's base-branch placeholder),
`status` (live tmux sessions + activity, no Notion at all; `--json` also gives each agent's `model`, `effort` and `context_percent` from its teed statusline — see `internal/agent/CLAUDE.md` — each omitted when unknown), `usage` (see
below — a property of the logged-in Claude account, not of any project),
`claude-version` (`claudeversion.go`: `installed` the first token of `claude
--version`, `latest` the newest on the install's own channel
(`claudeChannel`: a Homebrew cask by name — `claude-code` stable,
`claude-code@latest` latest — else `autoUpdatesChannel` in
`~/.claude/settings.json`, default latest): latest from GitHub's
`anthropics/claude-code` release feed, stable from the
`downloads.claude.ai/claude-code-releases/stable` pointer, each one
unauthenticated HTTPS read — no gh — kept per channel in `<state
dir>/claude-version-<channel>.json` for an hour; either side unread is absent and
`update_available` false, never a failure) and `claude-update` (`brew upgrade
<cask>` where claude's real path holds `/Caskroom/<cask>/` — there `claude
update` installs nothing, only printing the brew command and exiting 0 —
else `claude update`; output relayed, `--json` `{output}`, a failure the
command's error carrying it).

Project-scoped reads: `info` (reads the replica as it stands —
`store.StoredPlan`, never [`Mirrored`]'s staleness pull, since every `nat`
is a fresh process with no board around to watch that copy age; `--refresh`
restores the staleness-pull behaviour every other reader of a `Mirrored`
still gets), `slice-show` (full slice incl. computed
`State` — **computed with `domain.AgentNone`/`domain.PRUnread`, never a live
tmux/gh reading**, so `working`/`awaiting review`/`ready to merge` collapse
to whatever the page alone implies; only `slice-status`'s narrower read
tells a legacy Done-with-open-PR row apart, see below), `pr-view`,
`config-show`.

Plan mutation: `next-slice`/`start-slice` (claim), `complete-slice`
(mutually exclusive endings — a hand-back, the default, `--pr`, `--blocked`
or `--no-branch`, straight to Done; see root CLAUDE.md's Domain rules).
A hand-back (`--branch` optional) runs `actions.PushHandBack` after the
holds and follow-ups checks and before any page write: the branch is
`--branch`, else what the slice's worktree (`WorkdirFor` +
`AgentBranch`'s `Worktrees.Path`) has checked out; a worktree with anything
`git status --porcelain --untracked-files=normal` names is refused path by
path; then `git push --force-with-lease -u origin <branch>` in the worktree
(or the repository, for a named branch with no worktree), a refusal the
command's error with git's whole stderr. No worktree and no `--branch` is a
usage error naming `--branch` and `--no-branch`; a project with no `Branch`
column refuses a hand-back before any push. `--blocked` and `--pr` neither
check nor push. `release-slice`, `slice-add`, `milestone-add`/`-rename`/
`-remove`/`-move`, `slice-depends` (cycle refusal — reads the *whole* plan
graph before writing, so `--clear` alone never needs to and always
succeeds), `plan-apply`, `project-create`, `config-set`.

- `slice-edit` — **Todo-only**, same rule as the board's edit key: In
  progress refuses with "work in flight cannot be edited under its agent",
  Done with "a finished slice's brief is not edited after the fact"
  (`editable`). `--description` replaces the whole body (no append);
  `--title` renames (`Store.SetSliceTitle`, written first); either or both,
  neither refused. No duplicate check — a direct edit, like a direct
  `slice-add`, is the caller's deliberate act.
- **Title cap**: `domain.CheckSliceTitle` (`MaxSliceTitleLen`, 64 runes
  trimmed) refuses, naming the title and its length, in `slice-add`,
  `slice-edit --title`, `slice-followups` (each first line) and the plan
  path (created titles, `edit` titles) — before any write. Titles already on
  the board are never re-validated.
- **Brief opening**: `domain.CheckBriefOpening` refuses a brief whose first
  paragraph runs past `MaxBriefOpeningWords` (60, `strings.Fields`; an
  empty brief passes) — in the plan path (`validatePlan`: created slices'
  and `edit` descriptions, naming the slice) and `slice-followups` (each
  brief), before any write. Not in `slice-add`/`slice-edit`, which take the
  user's own brief; briefs on the board are never re-validated.
- `slice-move` — refuses only **In progress** (not Done — moving milestones
  is plan bookkeeping, not touching the work).
- `slice-move`, `slice-delete`, a refiling `slice-reorder` and `plan-apply`
  (its moves and removals, pruned once after the whole document — a slice it
  creates under such a milestone keeps it) each end in
  `actions.PruneEmptied` over the milestone(s) the slice left: text output
  gains a `Removed <name>` line, `--json` `removed_milestone` (omitted where
  none) or `plan-apply`'s `milestones_removed` (always an array).
  `done-clear` keeps its own wider sweep.
- `slice-reorder <slice> (--before|--after <slice>)` — places one slice beside
  another (`Store.ReorderSlice`); a target under another milestone refiles
  the slice to it in the same write, so the in-progress refusal applies to
  that case only — a reorder within a milestone is always allowed. Position
  is the plan file's alone: a Notion-backed project sends a request only for
  the refile, and a same-milestone reorder sets no dirty flag.
- `slice-delete` — refuses only **In progress**; Done is allowed through
  (Notion's trash is the recovery, not a CLI refusal) — the same asymmetry
  the board's `d` confirm draws with its warning-vs-refusal split. After the
  trash, `actions.RemoveSliceWorktree`; `complete-slice` does the same where
  it closes a slice Done (`--no-branch`).

Agent control (tmux only, no Notion read beyond the claim check):
`slice-launch` (`actions.Launch`, same flow the board's `l` key and
`start-slice`'s self-claim both use — a **third** way to get an agent
running, the one the macOS app's launch button drives; accepts Todo,
in-progress-with-no-live-session — one with a PR recorded is an ordinary
relaunch, its dependencies not asked and gh built only to gather its review
snapshot; Done is refused, PR or not), `agent-interrupt` (Claude Code's
interrupt key), `agent-kill` (`kill-session`; a session already gone is
success, not failure), `agent-send` (`internal/agent.SendPrompt`'s delivery —
the session's inbox, else a paste — `--text` or stdin, as review comments
go).

`agent-waiting` / `agent-working` (`agentwaiting.go`): the calling agent
marks its **own** pane — `$TMUX_PANE`, never an argument — waiting on the
user or back at work (`Tmux.SetWaiting`), which is all `status` reads
`waiting` from. Not project-scoped (no `--project`, refused as an unknown
flag) and writes nothing to the plan. Idempotent; refused before any tmux
write with `$TMUX_PANE` unset or a pane with no nat tag. Nudges on success,
which gnat (`ActivityStore.reread`) and the board (`nudged`'s activity read)
answer with an immediate activity reading.

`slice-followups` (held slices only; `--follow-up` repeatable, first line
title, rest brief; refuses none, an empty title/brief, a brief with no line
beginning `Done when:` — a queued one is the new slice's brief verbatim — or a
duplicate title)
and `slice-triage` (`--queue/--fold/--drop N`, whole batches: every pending
index of each batch it names exactly once, other batches untouched, or
`--drop-all` alone; refuses a Todo slice, nothing pending
(`store.PendingFollowUpsOf` — none on a Done slice), part of a batch, an item
whose title an earlier still-pending batch shares unless decided with it, and
any `--fold` with no live session; queues, records via `Store.RecordTriage`
only what it decided, in pending order, nudges, then sends one message naming
only that — a failed send exits non-zero with the record standing).
`slice-show --json`'s `followUps` carry `batch`, as do its `follow_ups`
events. See root CLAUDE.md's Follow-ups rule.

`slice-note <slice> --note TEXT|- [--from <slice>] [--milestone NAME]`
(Todo/In progress slices, anyone's): the target by ID/URL (`pageID`) or by
name (`sliceResolver` — trimmed, case-insensitive, as plan-apply resolves a
`depends_on`; `--milestone` narrows, and is refused beside an ID). Refuses an
empty note, an unknown or ambiguous name (listing the matches with their
milestones), Done, an unreadable body, and an unreadable `--from` — all before
`Store.RecordNote`. Provenance is `fromSlice` (`From "<name>" (<milestone>)`,
the label `store.SliceLabel` writes and `TaskEvents` parses back) or
`fromPerson` (the config's assignee name), never an ID; `slice-triage`'s
queued-follow-up line is `fromSlice` too. After the write and its nudge, a
target with a live session (`liveSessionFor`) is sent
`agent.NoteArrivedPrompt` — unless `--from` resolves to the target itself (an
agent noting its own slice is not sent its own note); a failed tmux listing is
logged and concludes nothing, a failed send exits non-zero with the note
standing, and the output adds `Its live agent was told.` only where one was.
See root CLAUDE.md's Notes rule.

`slice-show --json`'s `events` is the slice's whole task log: every
`store.TaskEvent` its body carries (`handed_back`/`sent_back`/`resumed`/`launched`/`relaunched`/
`released`/`blocked`/`summary`/`follow_ups`/`note`/`checks_failed`), in body order, plus — read off
the slice's properties rather than its body — an `approved` event where a
pull request is recorded and a `merged` event where the slice is Done with a
pull request or branch recorded. Always an array, never `omitempty`: the app
ranges over it with no nil check. Each body event carries `at` (RFC 3339, off
its section's stamp; omitted where the section predates stamps — and always
on `approved`/`merged`, which have no time source), and a note from a slice
`fromSlice: {name, milestone}` — by name, never resolved to an ID here, since
slice-show reads no plan; the app matches it against the plan it holds.

`slice-visuals` (held slices only — `store.Holds`; a Done slice is never
held). Incremental, each flag repeatable:
`--visual` (first line the name, the next the image's path or URI) adds or
replaces by name in place, keeping the before it has; `--before` (the
visual's name, then the before's path) sets the before of a visual in the
set or given in the same command; `--remove NAME` drops one with its before.
`handInOf` refuses, before any read: nothing given, an empty name/URI, a URI
over more than one line, a name twice to one flag, a name both removed and
given, a path `os.Stat`-less. A bare path or `file://` URI is made absolute
against `getwd` and hashed (sha256 of its bytes); any other scheme is filed
as given with no hash. Then the body is read, `handIn.apply` works the
command into `store.VisualChanges`' current set — refusing a `--remove` or
`--before` naming nothing there, listing what is filed — and the whole result
goes to `Store.RecordVisuals` (an empty set files a bare heading), nudge, and
a print of what was added, updated and removed and the set as it stands —
never blocks `complete-slice`. `slice-show --json`'s `visuals` reads back the
last section: `index`, `name`, `uri`, `hash` and `before` `{uri, hash}` each
omitted where none, and `changed`. See root CLAUDE.md's Visual changes rule.

`slice-rework` (handed-back slices only): `--comments` (optional, `-` reads
stdin) records what the review said under a `Sent back` heading
(`Store.RecordSentBack`) **before** clearing the slice's `Branch` — same
order as a hand-back's own note before its status write, for the same reason:
a slice already cleared back out of review would read, to this command's own
refusal, as never handed back, so the comments would be lost rather than
retried. `ClearBranch` leaves everything else alone, so it reads as in
progress until the agent's next `complete-slice --branch` re-records it — the
deterministic signal gnat's approve-over-comments flow waits on. The recorded
PR description stays on the page for the eventual `slice-approve`.

`slice-resume <slice> --note TEXT|-` (`sliceresume.go`): `actions.Resume`
— a stamped `Resumed` (the note, required) then `ClearBranch`, through
`actions.TakeBack`, the same order `slice-rework` now takes through it too.
Refuses an empty note before any read, a slice not In progress (Done by
name); one In progress with no `Branch` writes nothing, says so and exits 0
(no nudge). No ownership check, as for `slice-rework`. See root CLAUDE.md's
Resuming rule.

`slice-show --json` and `info --json` carry `resumed` per slice
(`domain.Slice.Resumed` against the plan's `Shape.HasBranch` — no body
read), and their `state` is `domain.StateOf` with that same `HasBranch`.
`taken_back` (`takenBack`: In progress, `Branch` empty, `HasBranch`, and
`holdsHandBack` on the body — `info` reads a body only for slices passing the
rest, through `taskLogOf`, an unreadable one concluding false;
`container-show` leaves it false) marks any slice handed back and taken back
to work, PR or not. On a taken-back slice both also carry `fixing_checks`
(`fixingChecks`, off the same body read: the check names of the latest
`Checks failed` or CI `Sent back` with no `Handed back` after it) — what
keeps gnat's failing mark on a resumed slice while its fix's checks run.
`container-show` passes the plan's shape too. `pr-view --json` carries
`head_ref_oid` (gh's `headRefOid`), how gnat tells a PR whose head moved.

`slice-checks <slice> [--log] [--json]` (any status, a read only): the
recorded PR's `gh.ViewPR` checks through `gh.Verdict`, one line per check;
no PR says so and exits 0. `--log` adds, per failed GitHub Actions check,
`gh run view --log-failed` cut to its last 200 lines — or, where gh refuses
that because a sibling job is still running, the job's whole log through `gh
api` (`gh.CLI.FailedLog`); an external status has its URL alone, an
unreadable log is logged and shown as `log not available: <why>` under its
check (`log_error` in JSON). For a **pending** check whose URL names an
Actions job, `--log` reads the job (`gh.CLI.ActionsJob`) and prints under the
check where it stands — `queued, no runner yet — waiting <d>` (from
`created_at`), or `in progress for <d> on <runner>, at step "<s>" for <d>` —
then every step with its status/conclusion and duration (`… so far` for the
step under way), all against `checksNow` (a package var, pinned in tests);
then its log (`gh.CLI.JobLog`), cut as a failed one is under a `## <name> log`
heading, or — GitHub gives none until the job ends (`gh.ErrLogNotReady`) — the
plain line `log: GitHub gives no log until the job ends` (`log_pending` in
JSON, never `log_error`). JSON adds `job` (`status`, `created_at`,
`started_at`, `runner`, `steps[]`) and `job_error`, all `omitempty`; a
failed job read is `job not available: <why>`, logged, and concludes nothing.
A run-only URL or another service's status gets nothing extra.

`slice-checks-rerun <slice> (--all | --failed | --check NAME...)` and
`slice-checks-cancel <slice> [--check NAME...]` (`slicechecksrerun.go`) are
the one way gnat or an agent re-runs or cancels CI. Both: any status with a PR
recorded, the PR re-read through one batched reading of it alone
(`readOnePR` — its state and checks' run URLs, no `gh pr view`) and refused
unless `OPEN` (a failed read, or a PR GitHub could not resolve, refuses — the
cost of being wrong is CI spent on a finished review); the
checks grouped into Actions runs by owner/repo/run (`loadCITarget`), so a run
is one gh call whatever number of its checks; a check with no run behind it is
`skipped` (`--check` naming one is refused, as is a name that is no check,
listing the names). `rerun`: exactly one mode; `--all` re-runs every run whole,
`--failed` each run with a failing check (`--failed`; none refused), `--check`
each named check's job (`RerunJob`; a run-only URL re-runs the run whole). A
run still going (any of its checks pending) is **cancelled first, every such
run before any wait**, then polled (`RunStatus`) to `completed` —
`rerunPolls`×`rerunPollEvery`, about two minutes, through `checksSleep` —
REST (`gh run view --json status`), a budget apart from GraphQL's; a
timeout is the error, says what was cancelled and that nothing was re-run —
and then re-run **whole**, whatever the mode: a cancel stops every job of the
run, and GitHub's docs don't say "failed jobs" picks cancelled ones up.
`cancel`: every run still going, or only the named checks' runs; none going is
refused; returns once the cancels are sent. Output and `--json`
(`{cancelled, rerun, skipped}`, always arrays; cancel has no `rerun`) name
every check stopped — the asked-for and their pending siblings — apart from
those re-run. A failure part-way names what was already sent. Nudge on
success only.

`pr-view --json`'s checks carry `rerunnable` (an Actions run behind it) and
`run` (that run's id, omitted otherwise) — how gnat tells which checks stop
together.

`session-list` and `session-status` ask GitHub nothing: each of a session's
branches (its five most recent, `sessionBranches`) takes its pull requests
from the last `pr-status` reading kept on disk (`lastReading.Sessions`), and
a branch that reading has not read is stale — `prs_stale`, and never a
session ended on it.

PR actions: `slice-approve` (`actions.OpenPR` + `actions.RecordPR`, the
approve key's two-step write, headless), `pr-comment` (`gh pr comment
--body-file -`, `--body` or stdin), `pr-edit` (`gh pr edit --body-file -`:
the description replaced, `--body` or stdin — pr-comment's shape and
refusals, nothing written to Notion; `--json` → `{pr}`) — both also take
`--session <id> <PR URL|number>` (`prtarget.go`: the session by
`lookupSession`, the lookup `pr-view --session` shares, gh in its directory;
refused unless the last reading kept that pull request for the session,
`sessionHeldPR`), `pr-reviewers` (a read is `ViewPR` for
`requested` and `candidates` — the repo's collaborators bar the author and
the requested, a failed collaborator listing `candidates_error`, never
"nobody"; `--add`/`--remove` run `gh pr edit` and answer with the edit's own
result, `{pr, added, removed}`, reading nothing back),
`pr-merge` (re-reads the PR through one batched reading of it alone —
`readOnePR`, 1 point, no `gh pr view` — applies
`actions.MergeRefusal` before ever calling `gh pr merge` — a mergeability
GitHub is still working out refuses nothing there, so gh is asked and its
own refusal relayed verbatim — marks Done on
success — the merge landed regardless of whether this last write does, so
its own failure says so rather than pretending the merge never happened —
then `actions.RemoveSliceWorktree`),
`pr-status` (`--project` **repeats**: one batched reading for every project
named — `openProjectReading` per project, `readBatch` once, each PR asked
about once however many projects name it — through `PRBatchReader.ReadPRs`;
see `internal/gh/CLAUDE.md` for the document. One project prints as it
always did; several key each project's doc by ID under `projects`, with
`rate_limit` and `detail` once at the top. Worth asking:
`actions.PRsWorthAsking` — every In progress slice with a PR, a Done one only
while its worktree exists (`actions.ListedOnce`, so deciding and sweeping
list each repository once); a Done slice with no worktree, and a PR URL that
names no pull request, read `unread`. Each not-ended ad hoc session rides
the reading too (`sessionHeads`: its five most recent branches —
`maxSessionBranches` — in the repository its origin names, `git remote
get-url origin`; one with no GitHub origin is asked nothing and reads
stale), printed under `sessions` `{id, prs, prs_stale}`. `--detail <PR URL>`
adds that pull request in full under `detail`, `pr-view --json`'s shape.
It is a polling read (`PRPoller.PollPRs`, `gh.CLI.PollPRs`): while the
budget's refusal stop holds it runs no gh and every PR reads unread;
`--settle` (gnat's read after an action) reads through `ReadPRs`, which always
runs. Every reading that asked anything carries `rate_limit {limit,
remaining, reset_at, projected_remaining_at_reset, throttled, paused_until,
poll_after_seconds, cost}` (`rateLimitOf`, from `PRPoller.Outlook` at the
config's `PollInterval`; the reading's own figures, else the last kept) —
markdown: a `GitHub budget:` line saying the same (`budgetLine`) — absent
where nothing was asked, since then no gh runs at all. `Run` returns a
`*gh.LimitError` bare, whatever a command wrapped it in, so every action
refused on the limit fails with the retry time alone. What the reading found that a later command wants is kept in
`<state dir>/github-reading.json` (`lastReading`, `Env.ReadingPath`; nil in
tests keeps none): each PR's base by normalised URL, each session's
branches' PRs — merged over the last, written atomically.
`prReadings` — the headless mirror of the board's `refreshPRStates`; an In
progress slice reading MERGED is `SettleMerged` off the reading itself (no
view) and loses its worktree, and `landed` — Done, no PR or one the reading
found merged or closed — goes to `actions.SweepLanded` with tmux's live
slices; neither changes the output;
writes `actions.ReopenUnmerged` for any Done-at-approve
legacy row whose PR still reads open — see root CLAUDE.md's Domain rules on
`StateOf`; `--json` carries `checks` `{verdict, failing: [{name, url}],
checks: [{name, state, url}]}` per PR the listing read — `checks.checks` every
check the verdict rolled up (`gh.PRStatus.All`, gh's order and workflow-led
names, `state` the raw word `Check.Outcome` reads; no extra request — gnat's
PR section lists from it), text unchanged — and the red ones go to `actions.NoticeFailingChecks`;
a tmux that can't list live sessions concludes nothing; every entry carries
`conflicting` — true only where gh positively said so, `mergeable`
CONFLICTING or merge state DIRTY (`gh.PRStatus.Conflicting`, the merge
refusal's words), false for UNKNOWN and for a PR the reading never read —
and `base` where it read one; not a readiness word, and nothing nudges on
it. `branches` (`branchReadings`) is every hand-back awaiting review —
In progress, `Branch` set, no PR — tested by `git.CLI.ConflictsWithBase`
(a fetch each), `{slice_id, name, branch, base, conflicting}`, `base` being
`CLI.Base`'s ref (`origin/main`); an unknown reading is left out, never
conflicting, and the markdown lists only conflicted ones under "Branches
awaiting review". gnat takes one reading of every open project a tick), `slice-status` (reads one page by ID directly, `--project` only
for credentials — no plan is read at all, so it is the one read that can
never show a phantom state from a stale cached plan; built for the macOS
app's session reaper, see `SessionReaping.swift`).

Scratch project: `scratch-open` (no `--project`; creates the reserved local
"Scratch" project through `createLocalProject` — the same path as
`project-create --local` — and records it as config's `scratch_project`,
then only reads it back) and `done-clear` (local projects only: deletes Done
slices and ended sessions, then milestones left empty; refuses a project with
a workspace behind it by name, so it can never trash Notion pages). gnat runs
both once per launch, before the scratch project's first read. On the
scratch project alone, `slice-add` takes no `--milestone`: the slice is filed
under the reserved `Unfiled` milestone (`unfiledMilestone`, added on first
use), which `info --json` marks `"unfiled": true` — gnat draws its slices
loose at the head of the Scratch fold, never as a folder.

`project-open-folder <dir>` (no `--project`; the starter card's "From
filesystem" tile) records the local plan a folder already holds. It finds
`<slug of project id>.db` via `store.ReadLocalPlan`, which opens **read-only**
(`OpenLocal` would migrate a stray SQLite file into a plan) and refuses a file
that is not a plan; a folder with none is refused saying what was looked for,
one with several is refused, a project already in config answers with its
existing entry.

`project-mirror --project <local id> --parent <id> --parent-kind page|database`
puts a local project into Notion: `CreateProjectIn` makes the page (a row of a
database's data source, or a child of a page), the local plan is filed through
the new project's ordinary `Mirrored` store, and config gets a Notion entry
under the **page's ID** (the project's ID changes; the local entry goes only
once the plan is entirely in, the plan file is never deleted). Refused before
Notion is touched: a project already in Notion, or any slice past Todo (its
branch/PR/worktree names an ID that would not follow it). Dependencies are
written to the workspace directly first, because `Mirrored`'s own write-through
swallows a failed push. `notion-search [--query]` (no `--project`) lists the
pages and databases a page could go under, for `--parent`.

Planning: `workshop-launch` (planning agent, `agent.PlanPrompt`). With
`--workspace <id>` (exclusive with `--project`, `--request` required) it is the
starter card's launch instead: `agent.NewProjectPrompt`, keyed by the app's
Untitled-tab workspace id (`plan:<id>`), run in a scratch dir nat makes at
`<state>/workspaces/<id>`; a live one refuses, and the app attaches.
`agent-kill --workshop --workspace <id>` ends only that tab's own session —
never the legacy bare one.

`plan-propose`, `plan-proposal` and `plan-accept` each take exactly one of
`--workspace <id>` (a brand-new project, still being workshopped) or
`--project <id>` (a revision to a project already tracked) — both or
neither is a usage error on all three. The proposal file is keyed by
whichever id named it (`proposals/<key>.json`); `proposalDoc` carries both
`Workspace` and `Project`, each `omitempty`, so only the one that applies is
ever written.

`plan-propose --workspace <id> --name <name> [FILE]` validates a drafted plan
with nothing of a project's own to resolve against — every milestone a slice
names, and everything `depends_on` reaches, has to be something the same
document creates, and a top-level `dependencies` list is refused outright —
and writes it to the proposal file instead of Notion. `plan-propose --project
<id> [--name <name>] [FILE]` (name optional) validates instead against that
project's current shape and, where the plan depends on anything, its slices
— `validateAgainstProject` is the one implementation this, `plan-apply`, and
`plan-accept --project` all call, so a plan a live project would refuse is
refused the same way by whichever of the three asks. Running either again for
the same key replaces its proposal — how a revision lands.

A plan document may also hold `remove` (titles), `move` (`{slice,
milestone}`) and `edit` (`{slice, title, description}` — a new title, a new
brief, or both, never neither; `title` omitempty, `description` always
written since gnat decodes it as present), each naming a **Todo**
slice already on the board by title (`resolveChanges`, `planchanges.go`):
in progress/Done, no match, more than one match, a removed slice also moved
or edited, or any `depends_on`/`dependencies` naming a removed slice each
refuse the whole document before a write; `move` on a source project is
refused in `validateAgainstProject`. Resolution and the cycle check see the
board as the removals leave it (removed slices gone, every wait on one
stripped). `applyPlan` writes edits, then milestones (a move may name a new
one), moves, the dropped waits (`SetDependencies` on each slice that waited
on a removed one — reported as the removal's `dependents`), the removals,
then creations — so a replacement may take a removed slice's title.
`plan-apply`'s output gains `edited`/`moved`/`removed` (an edited entry's
`title` is its new one, omitted where not renamed); `plan-accept`'s JSON
their counts. `plan-propose --workspace` refuses all three lists.

**No duplicate titles** (`checkDuplicateTitles`, in `validatePlan` after
`resolveChanges`, so all three plan commands run it): a created slice whose
title (trimmed, case-insensitive) a board slice the document does not
`remove` already has — "slice N ("…") is already on the board as a Todo
slice: edit it to change its brief, or remove it to replace it" — or
another created slice has, or a retitling `edit` gives, is refused; so is an
`edit` renaming to a held title. A retitled slice frees its old title.
Board slices already sharing a title that the plan doesn't touch are left
alone. `validateAgainstProject` therefore reads the project's slices
whenever the plan creates any, so `plan-propose --project` refuses at
propose time; `--workspace` checks the document against itself only.

`plan-proposal (--workspace <id> | --project <id>) --json` reads back what
`plan-propose` wrote for that key (`{"proposal": null}` with none yet — the
app polls it on every nudge; a file that won't parse is an error, which the
app logs and ignores).

`plan-withdraw (--workspace <id> | --project <id>) [--json]` removes that
key's proposal file (`{"withdrawn": bool}`; none is not an error, and an
accept's `.accepting-<pid>` claim is never touched), nudging only where it
removed one. Only the app runs it — when the user writes to the workshop's
agent with a proposal up — and both gnat planning prompts carry
`agent.ProposalWithdrawnRule`, so the agent proposes again every turn.

`plan-accept (--workspace <id> --name <name> | --project <id>)` is the user's
Accept. `--workspace` makes a local project (`createLocalProject`, no working
dir — Settings gives it later, as for an opened folder), then files the
proposal's plan through `applyPlan`, then drops the proposal file; no
`--name` with `--project`, since the project already has one. `--project`
re-validates the proposal's plan against that project's *current* plan with
`validateAgainstProject` — a plan the project has outgrown since the
proposal was written (a milestone renamed, a slice a `depends_on` named
since deleted) is refused here, not half-applied — applies it, then drops the
proposal file. **A proposal is accepted once:** both halves first claim it
(`claimProposal` renames it to `<key>.json.accepting-<pid>`, out of the
proposal path) and file exactly what they claimed — a second accept finds no
proposal, and no reader sees a proposal whose plan is in even if the claimed
file will not then go; one that cannot be claimed is refused before anything
is written. Success drops the claimed file, then nudges, so the last nudge
always finds the plan in and the proposal gone together. Refusals (no
proposal, empty name, an invalid or outgrown plan) all land before anything
is written; a failure after the claim leaves the project (made or already
there) and what was filed, puts the proposal back (`os.Link`, so a revision
proposed meanwhile is never overwritten), then nudges.

## `usage`

Probes Claude Code's own statusline for the account's Pro/Max rate-limit
state — the only OAuth-free source of what `/usage` shows. One synchronous
run: lay a throwaway `--settings` file, launch a detached tmux session
(`agent.LaunchUsageProbe`), send one minimal prompt (`rate_limits` appears
only after the session's first API response), poll for the sink file up to
`usageProbeTimeout`, then kill the session and delete the sink and the
probe's own transcript — success or not. Every failure mode (no tmux, a
timeout, an unparseable payload) reads the same to the caller: both windows
absent, printed as `{}` under `--json` or "usage unavailable" otherwise —
`nat usage` never fails loudly over an account with nothing to report. The
disk-cache-then-refresh pattern ("show last-known, then probe") is gnat's
own job (`UsageStore`/`DiskUsageCache` in `macos/Sources/NatKit`), not this
command's: each `nat usage` call is a fresh, synchronous probe.

## `slice-diff`

Same branch the board's `v` key reads (`git.DiffFrom`), plus two finer reads
sharing its refusals: `--commits` (history since the merge base, no diff)
and `--commit <sha>` (one commit against its own parent) — mutually
exclusive, and `--commit`'s JSON reuses the whole-diff shape with `sha^` as
the base. The base is **not always the repo default**: where the slice has
a recorded PR, the base the last `pr-status` reading kept for it
(`lastReading.Bases`) is used instead (a PR opened against anything but the
default branch is measured against what it would actually merge into) — no
`gh` at all, since this runs on every Changes tab load and tally refresh; a
PR no reading has reached yet diffs against the default.

A slice with no `Branch` that is In progress and whose task log holds a
`Handed back` — resumed, or sent back — is read on `actions.AgentBranch`
(`handedBackSlice`, `handedBackBefore`; an unreadable body concludes
nothing), so `slice-diff` and `slice-file` keep a reading while the work is
redone. One never handed back is still refused.

## `slice-file`

The lines a diff leaves out between its hunks, for gnat's expand controls:
`git show <ref>:<path>` (the slice's branch, or `--commit`'s sha) cut to
`--from`..`--to` (1-based, inclusive; `--to` off reads to the end), under
`slice-diff`'s own refusals (`handedBackSlice`). The JSON carries the file's
`total` length — a diff says where its hunks end and nothing about how much
file follows — and lexes each line as `slice-diff` lexes a context line.
