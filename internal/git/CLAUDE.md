# internal/git

A thin wrapper on the `git` binary — the one thing the board (and the macOS
app) asks of it is a slice's handed-back branch: its diff, its commits, and
the files behind them, so the work can be read before it becomes a pull
request.

## Base resolution — read this before touching `Diff`/`Commits`/`Base`

- `Base(dir)` reads `refs/remotes/origin/HEAD` (the repo's default branch as
  the clone last recorded it) and falls back to `fallbackBase` — first
  `origin/main`, then a bare `main` — only where that's unreadable too. Both
  steps are **logged and swallowed, never returned**: a diff against `main`
  beats refusing to show one at all.
- `origin/HEAD` is missing more often than it sounds — git writes it at clone
  time and nothing maintains it afterwards, so any checkout made another way,
  or pruned from, has none until `git remote set-head origin --auto`.
- The remote ref is tried before the local branch because the local branch is
  exactly the staleness `Fetch` exists to get past: a checkout nobody has
  pulled in a fortnight has a `main` that far behind, and a slice cut from it
  starts life that far behind too.
- `baseNamed(dir, name)` resolves a base the caller already knows by *name*
  (the branch a PR actually records as its base) rather than the default:
  tries `refs/remotes/origin/<name>` then `refs/heads/<name>` (full refs, not
  short names — a local branch literally called `origin/main` would
  otherwise answer for the remote's), and falls back to the whole `Base`
  chain, logged, where the name resolves to nothing.
- `Fetch(dir)` runs `git fetch origin` and returns nothing — failure (no
  network, no origin, a remote that refused) is logged and swallowed, since
  working off the refs as last fetched is what every offline git command
  already does.

## Diff / Commits

- `Diff`/`DiffFrom` run `git diff --no-color --no-ext-diff
  --src-prefix=a/ --dst-prefix=b/ --merge-base <base> <branch>` — merge-base,
  not the base's tip, because what the branch did is the point, not
  everything main has moved on by since. Prefixes pinned and external diff
  drivers refused because the **output is parsed, not shown as-is** — a repo
  configured with `diff.noprefix` or a diff driver would hand back something
  else entirely.
- `Commits`/`CommitsFrom` resolve the base exactly the same way (`baseNamed`)
  so a caller reading a branch's diff and its commit list is reading the same
  stretch of history. `git log --format="%H%x00%s%x00%an%x00%aI" base..branch`
  — NUL-separated, since a comma or pipe isn't guaranteed absent from a
  subject line; `%aI` is strict ISO 8601, parsed with `time.RFC3339`.
- `CommitDiff(dir, sha)` diffs one commit against its first parent
  (`sha^`..`sha`, same pinned prefixes as `Diff`). A root commit (no parent)
  is refused as `ErrNoParent` *before* the diff runs — diffing against the
  empty tree would answer a different question (everything the commit ever
  added) than the one asked.
- `Show(dir, branch, path)` (`git show --no-textconv <branch>:<path>`) is the
  whole file at the branch — the only place the lines between a diff's hunks
  can come from. Textconv refused for the same reason as diff drivers; a
  deleted file (or an unresolvable path) is a plain refusal, logged.
- `ParseFiles` splits diff output into `File`s. Paths come from the
  `+++`/`---` lines, not the `diff --git` header — the header pairs two
  paths with one space and can't be split where a filename holds spaces.

## Conventions

- `Runner` is `internal/git`'s own type, not `gh.Runner` — a package about
  git has no business importing the GitHub CLI to borrow a type off it.
- A `git` that ran and refused is `*ExitError` (first non-empty stderr
  line, same shape as `gh.ExitError`); a `git` binary that isn't there at
  all comes back as `os/exec`'s own error, undistinguished — no caller acts
  on the difference, since a machine without git has no working board to
  fall back to regardless.
