# internal/actions

The board's launch, approve, merge and worktree-removal flows without the
board: the writes and subprocesses that the `l`/`a`/`m` keys and their
headless `nat` counterparts (`slice-launch`, `slice-approve`, `pr-merge`, …)
actually run, with no bubbletea in the mix. `internal/tui` and `internal/cli`
both call into this package rather than each having their own copy — grep
either for `actions.` to see the call sites. `internal/tui/claim.go` no
longer exists; `internal/tui/{launch,approve,landed,worktrees}.go` are now
thin — bubbletea messages, toasts, board redraws — over the functions here.

For *why* any of these writes happen in the order they do, see root
CLAUDE.md's Domain rules (claim-before-tmux, resume-before-branch-clear,
release note-before-status, Done-means-merged, worktree-removed-only-once-work-ends,
etc.) — this file is the mechanics, not a restatement of the rules.

## Seams

- `Store` (`client.go`) is narrower than `store.Store` — the whole port —
  because it's exactly what a launch or an approve touches: `Slice`,
  `PRDescription`, `ClaimSlice`, `RecordPR`, `MarkDone`, `ReopenSlice`.
- `Severity` (`SevSuccess`/`SevWarning`/`SevError`) is how loudly a result
  wants to be shown. `internal/tui/toast.go` aliases it under its own name
  (`severity = actions.Severity`) rather than keeping a second enum, so a
  toast reads the same whichever key produced it.
- `Launcher` (tmux), `Worktrees`/`Repo` (git), `PRCreator`/`PRViewer` (gh) are
  each narrower interfaces than the real packages behind them — only the one
  call each flow makes — so a test can drive the whole flow with fakes.

## Claim / launch (`claim.go`, `launch.go`, `worktrees.go`)

- `ClaimSlice` re-reads the slice for its current `Shape` before writing —
  a project converted in the Notion UI since the last read may have changed
  it — then writes Status + Assignee. It does **not** verify the claim stuck;
  that's `start-slice`'s job when the agent's own session reaches it.
- `Launch` is the whole flow in order: `PlaceAgent` (worktree), claim, read
  the brief, write the prompt file, start tmux. A slice with a PR recorded
  is an ordinary relaunch that also gathers `reviewSnapshot` (comments and
  checks, through `PRReviewReader`; nil reads nothing) for the prompt's
  pull-request passage. The claim runs **last** of what can fail before
  tmux is asked for anything, so a worktree or prompt-file failure leaves the
  slice exactly where it was.
- `PlaceAgent` resolves `AgentBranch` (the branch recorded at hand-back, or
  `SliceBranch` — `slice/<slug>` — derived otherwise), reuses an existing
  worktree for that branch untouched, and only fetches+cuts a fresh one when
  there isn't one. A working directory outside any git repo falls back to the
  shared checkout (`OK: true`, a warning toast); a `git` that ran and refused
  is a hard failure (`OK: false`) — nothing is launched half-placed.
- A `LaunchResult` with `Session == ""` is a launch that placed/claimed
  nothing (a worktree failure, a lost claim race) — reported as `Toast`, not
  a Go `error`: nothing is wrong with nat, the slice is simply still there to
  launch again.
- A launch that is a **relaunch** — its just-read brief already
  carries history (`store.HasHistory` over `store.TaskEvents`) from an
  earlier pass — writes one more task-log line (`Store.RecordRelaunch`) after
  the brief is read and before the prompt is written. Status alone is not
  history: an In progress slice with nothing on the record (claimed by hand)
  launches fresh. A fresh launch writes `Store.RecordLaunch` there instead —
  a `Launched`, the log's first word and its time. Either write's own failure
  is logged (`logging.Action`) and never fails the launch.
  `Mirrored.RecordLaunch` marks the slice sent only where it was level before
  the line: a claim whose push failed leaves it ahead, and the line's push
  landing must not clear that.
- **A source project's task with no repository** (`RepoUnknown`: the
  project is `backend: source`, which has no working directory, and
  `WorkdirFor` came back empty) is launched with no `PlaceAgent` and no git
  snapshot: the session starts in the home directory and
  `PromptContext.RepoUnknown` sends the agent to find, record (`nat
  slice-repo`) and cut its own worktree. `LaunchDir` is `ExistingDir` with
  that one case let through — the TUI's launch checks use it; every other
  empty directory is still refused. Once the repo is recorded, relaunch,
  approve (`slice-approve`), merge (`pr-merge`) and the TUI's worktree
  removal all go through `WorkdirFor`, which never needs the project's own
  directory then.
- A launch of a slice with a `MilestoneID`, on a store answering
  `store.ContainerReader` (only `store.Sourced`), fills
  `PromptContext.Container` (`promptContainer`): title, URL, the prose
  sections joined, the noun from `store.Describer` else `container`. A failed
  read is logged and leaves it nil — the launch goes on.

## Resume (`resume.go`)

- `Resume(st, s, note)` — refuses not In progress (Done by name); no
  `Branch` writes nothing and answers false; otherwise `TakeBack` with
  `RecordResumed`. `TakeBack(st, id, record)` is the one statement of
  record-then-`ClearBranch`, shared with `slice-rework`'s `Sent back`.
  `ResumeStore` (`RecordResumed` + `ClearBranch`) is what it needs.

## Emptied milestones (`prune.go`)

- `PruneEmptied(st, sp, left...)` is every slice write's tail that takes a
  slice out of a milestone (`slice-move`/`-delete`/refiling `-reorder`,
  `plan-apply` once at the end, the board's move and delete): one fresh
  `Plan` read, then `RemoveMilestone` for each named milestone no slice of
  any status is filed under, `Shape` re-read between removals. Skips a
  source project (`sp.Source`) and empty IDs without reading; a failed read
  or removal is logged and that milestone stays. Answers the names removed.

## CI failures (`checks.go`)

- `NoticeFailingChecks` acts on a reading's red PRs (`nat pr-status`, the
  TUI's `refreshPRStates`): identity is the set of failing run URLs (name
  where none), compared with the latest `Checks failed`/`Sent back` event's
  bullets (`sameFailure`). Live session → `Resume` (note
  `ChecksResumeNote`; a refusal or failed write skips the slice), then
  `agent.ChecksPrompt`, then `RecordSentBack`; none (or no sender) →
  `RecordChecksFailed`. The Sent back follows the send, so a failed send is
  retried; an unreadable task log passes the slice over.

## Approve / merge (`approve.go`, `merged.go`, `mergerefusal.go`, `landed.go`)

- `OpenPR` reads the slice's last-filed `PR description` section
  (`PRTitleBody` splits it: first line title, rest body) and hands it to
  `PRCreator`; an empty description lets `gh --fill` build it from commits.
  `RecordPR` writes the URL only — the slice stays `In progress`.
- `MarkDone` is the **only** function that writes Done, and always re-reads
  `Shape` first. `SettleMerged` (nat not running when a merge happened on
  GitHub) and `ReopenUnmerged` (a legacy Done row whose PR is still open) are
  both built on it — see root CLAUDE.md's `StateOf`/Done rules.
- `RemoveWorktree` treats "no worktree for this branch" as success (every
  sweep after the first), and a `git` refusal (dirty worktree, unreadable
  repo) as a logged no-op, never an error — the slice is Done regardless of
  what happens to the checkout.
- `RemoveSliceWorktree(w, s, p)` is that for one slice: `WorkdirFor` (`~`
  expanded) and `AgentBranch`; an empty repository (`RepoUnknown`) asks git
  nothing. `pr-merge`, `pr-status`'s settle, a Done-closing `complete-slice`
  and `slice-delete` call it.
- `SweepLanded(w, live, p, landed)` takes the slices the caller judged
  landed (`pr-status`'s `landed`), lists each repository once
  (`Worktrees.Branches`, a failure logged and that repository skipped),
  and removes only a slice whose `AgentBranch` the listing names. `live`
  (tmux `LiveSlices`) is asked only once something matched; a live slice
  is skipped, and an unreadable tmux removes nothing.
- **Deliberate duplication, kept level by hand**: `mergeOutcome`/
  `mergeVerdicts`/`MergeRefusal` in `mergerefusal.go` are a hand-ported copy
  of `internal/tui/prmerge.go`'s `checkOutcome`/`mergeRefusal` — not a shared
  call, because `internal/cli` must not import `internal/tui` (bubbletea,
  huh, lipgloss, glamour for a headless command that draws nothing). A change
  to either's wording belongs in **both**; `mergerefusal.go`'s doc comment
  says so at the definition. The one exception is which GitHub check word
  means what: both read `gh.Check.Outcome`, the single table, also behind
  the board's checks verdict. `internal/cli/difftokens.go` is the same pattern
  again, for `internal/tui/diffsyntax.go`'s lexing rules.
