# internal/worktree

Drives `git worktree`, wrapped as thinly as `internal/gh` and `internal/git`
— so a slice's agent gets a worktree of its own rather than sharing the
project's one checkout with every other agent and the user.

## Placement — the one convention git has no opinion about

- A worktree lives at a sibling `<repo>` + `.worktrees` (`dirSuffix`)
  directory, one entry per branch, named by `pathSlug(branch)`: every run of
  anything but a letter, digit, dot, hyphen or underscore collapses to one
  hyphen — `slice/worktrees` → `slice-worktrees`.
- **Sibling, not child**: nothing nat cuts appears inside the checkout the
  user works in or the diffs taken from it.
- The sibling sits beside the repository **every worktree shares**
  (`rev-parse --path-format=absolute --git-common-dir`, via `root(dir)`),
  not beside whichever worktree nat happened to be launched from.
- This is nat's own rule, not git's, and it must be re-derivable on every
  relaunch — a relaunch has to arrive at the exact same path a first launch
  did. It is spelled here alone: an agent cutting its own worktree runs `nat
  slice-worktree` or `nat slice-repo` (`actions.EnsureWorktree`) rather
  than following prose (see root CLAUDE.md).

## Operations

- `Create(dir, branch, base)` — `git worktree add <path> -b <branch> <base>`.
  `base` is the caller's to resolve and passed through as-is; empty says
  nothing and leaves git to cut from wherever the repo is, since which ref a
  slice is cut from (and how current) is a question about the project, not
  about git.
- A branch that **already exists** is checked out instead —
  `git worktree add <path> <branch>`, `base` not consulted at all — since its
  commits are exactly the relaunch's work so far. Not a rare path: a
  squash-merged slice keeps its branch.
- `Path(dir, branch)` reads `git worktree list --porcelain` (one record per
  worktree, its branch as a full ref) and reports a branch with no worktree
  as such rather than an empty path — the ordinary answer for a slice nobody
  has worked yet.
- `Branches(dir)` is the same one listing for the whole repository: every
  linked worktree's branch by short name, the main worktree (the first
  record) and detached ones left out — `pr-status`'s sweep, one git per
  repository rather than a `Path` per slice.
- `Remove(dir, branch)` finds that path and runs `git worktree remove
  <path>`, leaving to git whatever removal refuses — a worktree holding
  modified or untracked files is kept: a refusal is recoverable, thrown-away
  work is not. `git branch -d` runs after, and **its refusal is logged and
  swallowed** — a squash merge leaves a branch nothing else reaches, and a
  leftover branch costs nothing since `Create` checks an existing one out
  rather than tripping over it.

- `Discard(dir, branch)` is the forced sibling, for work the user threw away
  (a cancel, the delete of an In progress slice): `git worktree remove
  --force` where the listing names a worktree, then `git branch -D` where the
  branch exists — a branch with no worktree is still deleted, since `Create`
  would otherwise check it out again. Each refusal is logged and returned.

## Conventions

- `Runner` is its own seam (not `gh.Runner`/`git.Runner`), same reasoning as
  those two packages.
- A `git` that ran and refused is `*ExitError` (first stderr line); a `git`
  not on PATH at all comes back as `os/exec`'s own report, undistinguished —
  no caller acts on the difference, since git is what the board reads a diff
  with too, so a machine without it has no working board regardless.
