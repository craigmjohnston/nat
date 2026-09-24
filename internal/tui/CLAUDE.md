# internal/tui

Bubbletea v2 (`charm.land/*/v2`). Root model in `app.go`; all Notion/gh/git
I/O goes out as a `tea.Cmd` and comes back as a typed message — nothing
blocks the update loop.

## Layout

- The window is four bands, bottom-up: status band, boxed header, hints row,
  body (whatever's left). Below the framed threshold every band draws bare
  (no border), status line included; a one-line window is that line alone.
- The body splits into the Active panel (slices in flight, `active.go`) above
  the plan (`board.go`), each framed like an independent sibling — no border
  nested inside another. They scroll independently (`App.activeOffset` vs.
  `App.boardVP`); the wheel is always the plan's, wherever it lands.
- `App.boardBox` is the plan rendered into a cached viewport; `syncBoard` is
  the **single** place that gets invalidated/rebuilt — everything that
  changes what the board shows routes through it. Don't redraw the plan any
  other way.
- The agent terminal (`agentview.go`) is a `vterm.Session` drawn in a box
  beside the board, sized from `agent_split_percent`, undisturbed by the
  board's own redraws. Its capture/render is capped at ~60fps
  (`awaitFrame`), coalescing everything the child wrote between frames into
  one read of the emulator's screen.

## Launch / approve / landed / worktrees are thin wrappers now

`internal/tui/claim.go` **no longer exists**. `launch.go`, `approve.go`,
`landed.go`, `worktrees.go` call straight into `internal/actions` for every
write and subprocess — see `internal/actions/CLAUDE.md` for the mechanics
(claim-before-tmux ordering, `PlaceAgent`, `OpenPR`/`RecordPR`,
`SettleMerged`/`ReopenUnmerged`, `RemoveWorktree`). What's left here is
TUI-only: the launch form, the launch/approve prompts anchored to a row,
`prStillOpen` (the fix-launch gh gate — stays here since `PRViewer` is the
board's own seam, not `actions`'), toasts, the after-write refetch.
**`release.go` was *not* extracted** — still calls `store.Over(client)`
directly; check each file, don't assume every one routes through `actions`.

- `Severity` here (`sevSuccess`/`sevWarning`/`sevError`) is `actions.Severity`
  under another name. `agentBranch`/`workdirFor`/`expandHome`/`existingDir`/
  `trimModel` are one-line aliases to their `actions.*` equivalents — don't
  reimplement, just call through.

## Diff screen (`diff*.go`)

- Read-only (`v`) view of a handed-back branch's diff, via `internal/git`.
  Holds **parsed files**, not rendered text — a body row is wrapped to the
  drawn width, and `render` reruns on every resize, cursor move, and
  fold/expand toggle. File-open offsets, the line cursor, comment anchors —
  all row numbers into that rebuilt body, recomputed every render, **never
  carried across one**.
- A line's shape (`lineShapeOf`) is read off its literal prefix — header
  lines (`+++`/`---`) tested *before* the `+`/`-` content lines they'd
  otherwise match. `diffsyntax.go`'s lexing sits inside the +/- colouring,
  never instead of it; ported to `internal/cli/difftokens.go` for
  `slice-diff --json` — keep the two classification rules level by hand.
- **Stale-read exception, asymmetric with the rest of the app**: a failed
  re-read of the diff **drops** what was on screen (a diff is of one branch
  at one moment) — `prview.go`'s failed re-read does the opposite, keeping
  the stale reading, since a stale PR view is still about the right PR.
- Comments are **ephemeral by design**: held only in `Diff`'s session state,
  never written to Notion or GitHub, cleared only once actually delivered to
  the agent's pane (`s`, via `agent.SendPrompt`) — a failed send leaves them
  pending. A re-read whose lines changed or now occur twice drops the
  comment, saying so.
- Zones (`diffzones.go`, GitHub-expand-style gap fill) reveal lines from
  `git.Show` (the file at the branch), not the diff, so they're lexed on
  render rather than when the branch was read. Folding a file
  (`diffmouse.go`) is screen-only state, dropped on every re-read.

## Pull request screen (`pr*.go`)

- `V` opens one PR in full via `gh.ViewPR`; `checkOutcome`/`mergeRefusal` in
  `prmerge.go` are the **canonical** version `actions.MergeRefusal` hand-ports
  for `internal/cli` — see `internal/actions/CLAUDE.md`'s duplication note,
  keep both wordings in sync on any change here.
- Stale-read handling is the diff screen's mirror: a failed re-read (`r`)
  **keeps** the last reading — the slice and its PR are still there, only
  what GitHub last said is stale.
- `m` (`prmergeflow.go`) asks on the merge box itself (`PRView`'s own
  `rowPrompt`), refuses on the first failing verdict in `mergeRefusal`'s
  wording, and on success marks the slice Done directly (not via a
  background poll) — see root CLAUDE.md's Done-means-merged rule.

## Poll / nudge / selective load

- `poll.go`: background reload on a `poll_seconds` tick (default 30), and
  `prstate.go` rides the *same* tick rather than having its own. **Suspended**
  while the wizard, a form, a row prompt, or another load is in flight (would
  clobber what the user's mid-edit); **not** suspended by an open agent
  terminal — the plan behind the split stays live.
- `nudge.go`: stats the nudge marker every second (far shorter than the poll)
  and reloads on a moved mtime. The **first** reading is a baseline, not
  news. A nudge arriving **while a load is already in flight is left
  unconsumed** — the load may have started before the write landed, so the
  next tick still sees the moved mtime and reloads against a plan the write
  is certainly in.
- `selectiveload.go`: `startSelectiveLoad` queries only slices
  `last_edited_time`-filtered since `syncedAt` — no schema re-read. Falls
  back to a full `startLoad` before the first load ever lands. Merges by ID
  (overwrite in place / append); **cannot detect a trashed slice** — it
  lingers until the next full load. `syncedAt` stamps from when the *query
  went out*, not when it landed, so a mid-flight edit isn't skipped next run.
- `setPlanSlices` (`slicerefresh.go`) is the **single choke point** every
  patched-plan write funnels through (`refreshSlice`, `slicesSynced`,
  `removeSlice`, and by extension every `sliceSavedMsg` handler): rebuilds
  milestone status from the patched slice list, restamps `syncedAt`, closes
  any row-anchored board prompt (the row may have moved), calls `syncBoard`.
  Prefer patching one row through this over triggering a full reload.

## Mouse ownership

- The agent terminal asks for all-motion reporting **only while it's on
  show**; `boardmouse.go` is the rest — a left click selects the row under
  it (`Board.RowAtLine`), the wheel scrolls 3 lines/notch (clamped so the
  cursor never lands off-screen after `syncBoard`), a click on a PR chip's
  OSC 8 hyperlink (`Board.LinkAt`) opens it via `agent.URLOpener`. The board
  is deaf to all of it while the wizard, a form, or a row prompt is up.
- Inside the diff screen, only button events are requested (not all-motion),
  which is what leaves the outer terminal's wheel scrolling the diff.

## Prompts

- `App.promptHost` is the shared interface the board and the PR screen both
  answer `promptKey` through — same row-anchored prompt mechanism, different
  redraw (`Board`'s plan viewport vs. `PRView`'s own content). A plan landing
  closes a prompt anchored to a *row* (`closeBoardPrompt`, since the row may
  have moved) but leaves one anchored to a *pull request* alone (it hasn't).
- `claimedNote(slice, verb)` (`slicemove.go`) is the shared refusal for "a
  slice In progress cannot be `<verb>`'d" — used by move/delete; `editable`
  in `sliceform.go`'s edit flow applies the same Todo-only rule `slice-edit`
  enforces headlessly (see `internal/cli/CLAUDE.md`).

## Blocked slices (board-rendering half — see root CLAUDE.md for the rule)

- `Board.Blockers` indexes blocked slices per plan load. A blocked row draws
  a `⊘` marker (the agent star wins the cell if somehow both are true), muted
  text, and is sunk to the bottom of its milestone group (`appendGroup`) —
  **drawing only**, Notion's own view order and `next-slice`/`PlanOrder` are
  untouched. What's blocking a selected row is the status band's job
  (`Board.BlockedBy`), not the row's own.

## Local projects

`N` (`NewProjectForm`) asks where the plan lives only when a projects
database gives a choice; without one the plan is local and nothing is asked,
and the assignee group is hidden for local. `App.storeFor` opens a local
project's `Local` directly (no `Mirror`), and assignee identity comes from
`Config.AssigneeFor(project)`.

## Smaller subsystems

`presence.go`/`activity.go`: the pulsing star is the live-session map refined
by a 2s `agent.Tmux.Activity` poll — a failed activity read leaves the star
as it was, never clears. `dbtree.go`/`dbsearch.go` are the `N`/`P` project
picker's two views (lazy page tree; `/`-opened flat workspace search).
`onboarding.go` takes a ready Notion client — auth is the caller's problem.
