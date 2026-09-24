# internal/cli

The headless `nat` subcommands: what an agent runs against its own slice,
what skills run to plan and queue work, and the macOS app's entire backend
contract (`NatClient` shells out to `nat <command> --json`). Runs before the
tmux check, with no TUI code in the path — a command prints to the terminal
it was typed in and exits. `setup` and `project-create` are the only two
that act on no already-tracked project.

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
instead of a page), works with no credential. `wishlist`/`wishlist-clear`
refuse a local project (`refuseLocal`).

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

Project-scoped reads: `info`, `slice-show` (full slice incl. computed
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
succeeds), `plan-apply`, `project-create`, `config-set`, `wishlist`/
`wishlist-clear`.

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

`slice-rework` (handed-back slices only): clears the slice's `Branch` and
nothing else (`Store.ClearBranch`), so it reads as in progress until the agent's
next `complete-slice --branch` re-records it — the deterministic signal gnat's
approve-over-comments flow waits on. The recorded PR description stays on the
page for the eventual `slice-approve`.

PR actions: `slice-approve` (`actions.OpenPR` + `actions.RecordPR`, the
approve key's two-step write, headless), `pr-comment` (`gh pr comment
--body-file -`, `--body` or stdin), `pr-merge` (re-reads the PR, applies
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
both once per launch, before the scratch project's first read.

Planning: `workshop-launch` (planning agent, `agent.PlanPrompt`).

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
