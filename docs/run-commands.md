# Run commands

A **run command** is a short label and a shell command, defined per project,
that gnat offers as a one-click run. They are for projects where a one-click
run plainly helps — playtesting a build, a prototype's dev server — and most
projects need none.

## Config

Each project's entry in `config.json` may carry `runs`, omitted until set:

```json
"runs": [
  {"label": "Play",  "command": "./scripts/play.sh"},
  {"label": "Board", "command": "go run ."},
  {"label": "Seed",  "command": "make seed", "scope": "global"},
  {"label": "Seed",  "command": "make seed DB=./worktree.db", "scope": "slice"}
]
```

- `label` is what the button says: as short as it can be (`Run`, `Debug`,
  `Play`).
- `command` is run by `sh -c`, exactly as written.
- `scope` is almost always left off: the run is offered in both places and
  does the same thing in each. Give a label a `global` and a `slice` variant
  only where the run has to do or pass something different in a worktree.
  The first run offered in each place is that place's default.

A label is offered once in each place, case ignored: once scopeless, or once
per scope. An empty label or command, a label offered twice in one place, or
an unknown scope word is refused where it is written. Runs live in the
project's config entry alone — there is no settings screen for them. Write
the whole list at once with

    nat config-set project.<id>.runs '[{"label":"Run","command":"make run"}]'

(the empty string unsets it). `nat config-show` lists every project's runs
with their scope.

## Where each scope runs

- **Global** runs are offered by the play button in gnat's titlebar, beside
  Settings, as a tree: every project with runs, then its runs. They run in
  nat's own run checkout: a worktree on the branch `run/main` beside the
  repository (`<repo>.worktrees/run-main`), cut where there is none and,
  before every run, fetched and hard-reset to origin's default branch. A failed fetch runs from the refs as last known.
  The user's own checkout is never checked out or reset.
- **Slice** runs are offered as a Run heading in a handed-back slice's
  navigator, and run in that slice's worktree — the one its agent worked in.
  The heading greys once the slice is merged, its worktree being gone.

## `nat run`

    nat run --project <id> [--slice <id|URL>] [--label <label>] [--json]

starts a run in a detached tmux session of nat's own,
`nat-run-<slice or project ID tail>-<label slug>`, and reports `session`,
`label`, `command` and `dir`. With `--slice` it picks among the project's
slice-scoped runs, without among its global ones; `--label` picks one, else
the first. Asking for a run whose session is still live kills that session
and starts it afresh. The run's pane is tagged `@nat_run`, never as an agent,
so it never appears on the board or in gnat's rail. gnat attaches its
terminal as a Run tab beside Terminal while the session is live.

Refused, each in its own words: no runs of that scope, an unknown label, a
merged slice, a slice whose branch has no worktree, and a source task with no
repository recorded.
