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
CLAUDE.md's Domain rules (claim-before-tmux, fix sessions write nothing,
release note-before-status, Done-means-merged, worktree-removed-only-on-merge,
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
- `Launch` is the whole flow in order: `PlaceAgent` (worktree), write the
  prompt file, claim (skipped when `PromptContext.Fix` is set — a fix session
  claims nothing), start tmux. The claim runs **last** of what can fail before
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
- **Deliberate duplication, kept level by hand**: `mergeOutcome`/
  `mergeVerdicts`/`MergeRefusal` in `mergerefusal.go` are a hand-ported copy
  of `internal/tui/prmerge.go`'s `checkOutcome`/`mergeRefusal` — not a shared
  call, because `internal/cli` must not import `internal/tui` (bubbletea,
  huh, lipgloss, glamour for a headless command that draws nothing). A change
  to either's wording belongs in **both**; `mergerefusal.go`'s doc comment
  says so at the definition. `internal/cli/difftokens.go` is the same pattern
  again, for `internal/tui/diffsyntax.go`'s lexing rules.
