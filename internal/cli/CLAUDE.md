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
- `config-set`'s `project.<id>.working_dir` key does its own copy of this
  match (`projectKeyFor`) against the config already in memory, rather than
  calling `namedProject` and re-reading the file it is about to write back.

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
- `project-create --source <name>` (exclusive with `--local`):
  `describePlugin` before any write, then `createPlanProject` — the same
  file-then-config path `createLocalProject` takes.
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
neither Notion nor config), `paths` (prints config/log/nudge paths),
`status` (live tmux sessions + activity, no Notion at all; `--json` also gives each agent's `model`, `effort` and `context_percent` from its teed statusline — see `internal/agent/CLAUDE.md` — each omitted when unknown), `usage` (see
below — a property of the logged-in Claude account, not of any project).

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
(mutually exclusive `--branch`/`--pr`/`--blocked` endings — see root
CLAUDE.md's Domain rules, this package only parses flags and calls
`store.Store`), `release-slice`, `slice-add`, `milestone-add`/`-rename`/
`-remove`/`-move`, `slice-depends` (cycle refusal — reads the *whole* plan
graph before writing, so `--clear` alone never needs to and always
succeeds), `plan-apply`, `project-create`, `config-set`.

- `slice-edit` — **Todo-only**, same rule as the board's edit key: In
  progress refuses with "work in flight cannot be edited under its agent",
  Done with "a finished slice's brief is not edited after the fact"
  (`editable`). Replaces the whole body; does not append.
- `slice-move` — refuses only **In progress** (not Done — moving milestones
  is plan bookkeeping, not touching the work).
- `slice-reorder <slice> (--before|--after <slice>)` — places one slice beside
  another (`Store.ReorderSlice`); a target under another milestone refiles
  the slice to it in the same write, so the in-progress refusal applies to
  that case only — a reorder within a milestone is always allowed. Position
  is the plan file's alone: a Notion-backed project sends a request only for
  the refile, and a same-milestone reorder sets no dirty flag.
- `slice-delete` — refuses only **In progress**; Done is allowed through
  (Notion's trash is the recovery, not a CLI refusal) — the same asymmetry
  the board's `d` confirm draws with its warning-vs-refusal split.

Agent control (tmux only, no Notion read beyond the claim check):
`slice-launch` (`actions.Launch`, same flow the board's `l` key and
`start-slice`'s self-claim both use — a **third** way to get an agent
running, the one the macOS app's launch button drives; accepts Todo or
in-progress-with-no-live-session only, **never** a fix launch — Done is
refused outright, unlike the board's `l`), `agent-interrupt` (Claude Code's
interrupt key), `agent-kill` (`kill-session`; a session already gone is
success, not failure), `agent-send` (paste-buffer delivery, `--text` or
stdin — same mechanism `internal/agent.SendPrompt` uses for review
comments).

`slice-followups` (held slices only; `--follow-up` repeatable, first line
title, rest brief; refuses none, an empty title/brief, a brief with no line
beginning `Done when:` — a queued one is the new slice's brief verbatim — or a
duplicate title)
and `slice-triage` (`--queue/--fold/--drop N`, every pending index exactly
once, or `--drop-all` alone; refuses a Todo slice, nothing pending, and any
`--fold` with no live session; queues, records via `Store.RecordTriage`,
nudges, then sends one message — a failed send exits non-zero with the
record standing). See root CLAUDE.md's Follow-ups rule.

`slice-note <slice> --note TEXT|- [--from <slice>] [--milestone NAME]`
(Todo/In progress slices, anyone's): the target by ID/URL (`pageID`) or by
name (`sliceResolver` — trimmed, case-insensitive, as plan-apply resolves a
`depends_on`; `--milestone` narrows, and is refused beside an ID). Refuses an
empty note, an unknown or ambiguous name (listing the matches with their
milestones), Done, an unreadable body, and an unreadable `--from` — all before
`Store.RecordNote`. Provenance is `fromSlice` (`From "<name>" (<milestone>)`,
the label `store.SliceLabel` writes and `TaskEvents` parses back) or
`fromPerson` (the config's assignee name), never an ID; `slice-triage`'s
queued-follow-up line is `fromSlice` too. See root CLAUDE.md's Notes rule.

`slice-show --json`'s `events` is the slice's whole task log: every
`store.TaskEvent` its body carries (`handed_back`/`sent_back`/`relaunched`/
`released`/`blocked`/`summary`/`follow_ups`/`note`), in body order, plus — read off
the slice's properties rather than its body — an `approved` event where a
pull request is recorded and a `merged` event where the slice is Done with a
pull request or branch recorded. Always an array, never `omitempty`: the app
ranges over it with no nil check. Each body event carries `at` (RFC 3339, off
its section's stamp; omitted where the section predates stamps — and always
on `approved`/`merged`, which have no time source), and a note from a slice
`fromSlice: {name, milestone}` — by name, never resolved to an ID here, since
slice-show reads no plan; the app matches it against the plan it holds.

`slice-visuals` (held slices, or — `canHandInVisuals` — a Done slice with a
PR recorded, assigned to you where the project has an Assignee column: a fix
session's, its PR not re-checked with gh; `--visual` repeatable, first line the
name, the next the image's path or URI; refuses none, an empty name/URI, a URI
over more than one line, or a duplicate name). A bare path or `file://` URI is
made absolute against `getwd` and refused if `os.Stat` fails; any other scheme
is filed as given. `Store.RecordVisuals`, nudge, print — never blocks
`complete-slice`. `slice-show --json`'s `visuals` reads back the last section
(`store.VisualChanges`). See root CLAUDE.md's Visual changes rule.

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

PR actions: `slice-approve` (`actions.OpenPR` + `actions.RecordPR`, the
approve key's two-step write, headless), `pr-comment` (`gh pr comment
--body-file -`, `--body` or stdin), `pr-reviewers` (`--add`/`--remove`
run `gh pr edit` first, then the PR is read back for `requested`;
`candidates` are the repo's collaborators bar the author and the requested,
and a failed collaborator listing is `candidates_error`, never "nobody"),
`pr-merge` (re-reads the PR, applies
`actions.MergeRefusal` before ever calling `gh pr merge`, marks Done on
success — the merge landed regardless of whether this last write does, so
its own failure says so rather than pretending the merge never happened),
`pr-status` (`prReadings` — the headless mirror of the board's
`refreshPRStates`; writes `actions.ReopenUnmerged` for any Done-at-approve
legacy row whose PR still reads open — see root CLAUDE.md's Domain rules on
`StateOf`), `slice-status` (reads one page by ID directly, `--project` only
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

`plan-proposal (--workspace <id> | --project <id>) --json` reads back what
`plan-propose` wrote for that key (`{"proposal": null}` with none yet — the
app polls it on every nudge; a file that won't parse is an error, which the
app logs and ignores).

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
a recorded PR, `gh.ViewPR`'s `BaseRefName` is used instead (a PR opened
against anything but the default branch is measured against what it would
actually merge into) — a `gh` that cannot answer is logged and the command
falls back to the default rather than failing the diff over it.

## `slice-file`

The lines a diff leaves out between its hunks, for gnat's expand controls:
`git show <ref>:<path>` (the slice's branch, or `--commit`'s sha) cut to
`--from`..`--to` (1-based, inclusive; `--to` off reads to the end), under
`slice-diff`'s own refusals (`handedBackSlice`). The JSON carries the file's
`total` length — a diff says where its hunks end and nothing about how much
file follows — and lexes each line as `slice-diff` lexes a context line.
