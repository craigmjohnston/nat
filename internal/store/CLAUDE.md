# internal/store

The seam between nat and wherever a project's plan lives. `Store` is the
port: callers speak in `domain.Slice`/`domain.Milestone`, never in property
types, request bodies or SQL — so a second backend plugs in without anything
above learning about it. `store.NewClient` is the **one** place a Notion
client is constructed; every other Notion access goes through a `Store`.

Two implementations answer the same interface: `Notion` (`notion.go`) and
`Local` (`local.go` + `local_write.go`, SQLite). A third backend has to
answer every method or none.

`Mirrored` (`mirrored.go`) is a third `Store`, but not a third backend: a
`Local` replica in front of a `Notion` workspace, built and tested but wired
to nothing above this package yet. See its own doc comment for the
write-through rule — local first, dirty set in that write's own transaction,
pushed after, `MarkSent` clearing the flag only on a successful push — and
why milestone writes and `AddSlice` are the two exceptions that go to the
workspace first instead.

`Project.Local` / `Project.PlanDir` (built by `store.ProjectOf` from a config
entry) say a plan is a file with no workspace behind it: `ForProject` returns
the `Local` itself (remote may be nil), `PlanPath` honours the config's
`plan_dir`, and `CreateLocalProject` lays the file down (`Local.InitProject`
writes an *unstamped* project row, so `Shape` keeps answering yes to both
columns and the plan never reads as hydrated). `Project.Source` (the plugin
name) is a source project: the same file, same `PlanPath`/`CreateLocalProject`,
wrapped in `Sourced`.

## Sourced (`sourced.go`)

A `Local` plan whose milestones are a task-source plugin's containers
(`internal/source`). `ForProject(ctx, p, remote, plugin, src)` builds it for
`p.Source != ""` (after the `p.Local` arm, before `Mirror`) and refuses a nil
`src`; the caller builds `plugin` (`source.Project`), since `Project` carries
no working dir.

- **Containers are the plugin's.** A container is a `milestones` row keyed by
  `container_id` (schema v5, partial unique index); `milestones()` answers
  `ID = container_id` where set, else `ID = name` (every older row).
  `checkMilestone` matches `(name AND container_id IS NULL) OR container_id` —
  a container's *title* never files a slice. `Sourced.AddSlice` refuses an
  empty milestone ID, then `Local.ensureMilestone(id, title)`: an existing
  `container_id` is a no-op (title drift accepted, `name` never rewritten); a
  new one goes after the last milestone, named `title (id)` where another
  milestone already holds the title (case-insensitive, `milestoneNamed`).
  A task given no `Repo` starts from the repository of the container's latest
  task with one (`containerRepo`, a plan read that fails concluding nothing)
  — a source project has no working directory of its own.
- **Refusals**, in Sourced's own words naming the plugin, before any write:
  `AddMilestones`/`RenameMilestone`/`RemoveMilestone`/`MoveMilestone`,
  `MoveSlice`, and `ReorderSlice` across containers (no `moved` event in the
  protocol). A same-container reorder delegates.
- **Events follow the write**, only on success, logged never returned
  (`fireEvent`): `AddSlice`→created, `ClaimSlice`→claimed,
  `ReleaseSlice`→released, `CompleteSlice`→handed_back *only with a branch*,
  `RecordPR`→approved, `MarkDone`→merged, `DeleteSlice`→deleted. `RecordPR`/
  `MarkDone` read the slice back for the task (a failed read sends the ID
  alone); `DeleteSlice` reads it *before* the delete. `Task.Status` is
  `StatusName` (`Todo`/`In progress`/`Done`). Everything else delegates to
  `Local` and tells nobody.
- **Narrow interfaces** in `store.go` — `Describer`, `SidebarReader`
  (answering a `source.Sidebar`: groups and an optional header menu),
  `ContainerReader`, `ActionRunner` — answered only by `*Sourced`, each
  delegating to the client with the project's `plugin`. Callers type-assert,
  as with `Puller`. `RepoSetter` (`SetSliceRepo`, the repo and nothing else)
  is answered by `*Local` and `*Sourced` — `nat slice-repo`'s.

## Task log (`taskevents.go`, `stamp.go`, `tasklog.go`)

- Every task-log section either store writes opens with a stamp paragraph,
  `At <RFC 3339, local time with offset>` (`stamped`): Handed back /
  Blocked / Summary (`CompleteSlice`), Sent back, Note (stamp, then the
  `From …` provenance, then the note), Launched, Relaunched, Follow-ups and
  Follow-ups triaged. `PR description` is **never** stamped — every line under
  it is the PR's body. The released line carries its time in the sentence
  (`… by <name> at <RFC 3339>: …`), the ` at …` optional on read.
- The time comes from each store's `Clock` (nil → `time.Now`); tests set it
  (`fixedClock`, `clocked`) so bodies and Notion request JSON stay exact.
- `TaskEvents` reads each stamp into `TaskEvent.At` (zero where absent —
  old plans read exactly as before), and a Follow-ups triaged section's into
  each item it decides' `TaskFollowUp.DecidedAt` (`slice-show`'s `decidedAt`), and
  `HandbackSummaryOf` strips it.
- Every `Follow-ups` section is a batch (`TaskEvent.Batch`, its ordinal); a
  triage record's line decides the earliest still-undecided item of its title
  in any batch before it (`applyDecision`). `PendingFollowUps` is
  `TaskEvents`' undecided items — never a parser of its own, so the two
  cannot disagree.
- `SliceLabel` writes a note's `"Name" (Milestone)` provenance and `sliceLabelOf` reads it back
  into `FromSlice` — write and parse kept together. `HasHistory` is the one
  rule for "launched before": any event but a note.

## Shape

- `Shape` is read, not assumed: exported `HasAssignee`/`HasBranch`/
  `Milestones` (both backends fill honestly), plus unexported write-time
  state (`Notion`'s `statusType`, the milestone `PropertySchema`) only
  `Notion` needs. **A caller takes a `Shape` from a read and hands the same
  one back to a write — never constructs or peeks inside one.**
- `Shape.On(page)` — `statusType` comes from the *page's own* shape (a
  `Status` column converted in Notion's UI differs per page); which columns
  exist stays the *project schema's* answer.
- `Holds(slice, shape, userID)` is the ownership check every caller-scoped
  write asks first. Lives here rather than `domain` because it's as much
  about the shape as the slice.

## Errors

- Single-write ops (`RecordPR`, `MarkDone`, `ReopenSlice`, `ClearBranch`, `MoveSlice`,
  `DeleteSlice`) return the raw backend error — the caller's own sentence
  ("delete the slice") is the context, not the store's.
- Multi-write ops (`ReleaseSlice`, `CompleteSlice`, `AddMilestones`,
  `RenameMilestone`, `RemoveMilestone`, `MoveMilestone`, `EditSlice`,
  `SetSliceBrief`) wrap with which step failed — different states need
  different recovery (a release's line vs. its status; a completion's note
  vs. its properties).
- Every backend refuses **in its own words** — never invent a shared error
  vocabulary here.

## Notion backend (`notion.go`)

- Built over `API`, a narrow interface (the data-source and page reads/writes + six calls:
  `DataSourceOrder`, `GetPage`, `CreatePage`, `GetBlockChildren`,
  `AppendBlockChildren`, `TrashPage`) so request cost stays visible in one
  place and a fake can drive it.
- Every read starts with `GetDataSource` on the Slices data source; there is
  no load-time migration, so a project is expected to be in the one shape.
- Milestone rename/remove/move mechanics **live here**, not in `internal/cli`
  — the CLI commands are thin wrappers. See root CLAUDE.md's Domain rules
  for the reasoning (rename-goes-the-long-way, remove-refuses-while-filed,
  move-is-cheapest-of-three); this is where it's actually implemented.
- `AddMilestones`/`RenameMilestone`/`RemoveMilestone`/`MoveMilestone` each
  read the plan before their first write and refuse before it.
- `MoveSlice`, `ReorderSlice` and `DeleteSlice` write the slice alone; the
  milestone a write empties is removed afterwards by the caller, through
  `actions.PruneEmptied` and `RemoveMilestone`, never inside the store.
- `AddMilestones(names=[])` is a **silent no-op** (`nil, nil`) — rewriting an
  option list to a copy of itself is a real schema edit for nothing, so it's
  skipped rather than performed.
- `RenameMilestone(old, name)` where `name == old` is **refused as a
  duplicate**, not accepted as a no-op — "nothing to do" still isn't silently
  accepted.
- `SetSliceBrief` has no replace-content call to use: it trashes every
  top-level block one by one (children go with their parent), then appends
  fresh ones.

## Local backend (`local.go`, `local_write.go`)

- SQLite via `github.com/ncruces/go-sqlite3` — pure-Go/WASM, no cgo, so
  `go install ...@latest` and the release pipeline's per-arch cross-builds
  both keep working. One `.db` file per project under `LocalDir()`
  (`~/Library/Application Support/notion-agent-tracker/plans` on macOS, else
  `$XDG_DATA_HOME` or `~/.local/share/.../plans`), named
  `localSlug(projectID)+".db"` — the *same slugging rule* as
  `worktree.pathSlug`, reused, not shared code.
- DSN: `busy_timeout(5000)`, `journal_mode(wal)`, `foreign_keys(on)`,
  **`_txlock=immediate`** — every transaction is `BEGIN IMMEDIATE`, never
  deferred. A deferred transaction takes its read lock at the first `SELECT`
  and only asks for the write lock later, which is the **one** lock upgrade
  SQLite refuses outright rather than waits out the busy timeout for — so
  two concurrent writers fail fast rather than queue as intended.
- **`busy_timeout` must stay the first pragma.** The driver runs `_pragma`s
  in DSN order on every new connection and drops its own default timeout when
  any is given, so a pragma before ours runs with no timeout at all —
  `journal_mode(wal)` first was the "invalid _pragma: database is locked" the
  app's concurrent `nat` processes kept hitting. On top of that, `Local.retry`
  retries a `sqlite3.BUSY` twice with backoff (100ms, 300ms) around the open's
  migrate, every `withTx`, and every direct `l.db` read — at the leaf, never
  nested; helpers taking a `localQuerier` inside a transaction are covered by
  `withTx`'s.
- Schema is stamped in `PRAGMA user_version` (currently `5`; each step is a
  `localSchemaVN` in `localMigrations`). `OpenLocal`
  creates directory + file + schema when none exists (an untouched project
  is an empty plan, not an error). A plan written by a newer nat is refused
  by name, never read through the wrong schema.
- Every mutation is **one transaction**, and reads what it's about to write
  *inside* that transaction before writing — never off what the caller was
  told earlier. `updateSlice` is this factored out: read `before`, run the
  mutation, read `after`, return `after`; a slice deleted since the caller's
  last read fails the re-read rather than silently updating nothing.
- `Local` doesn't need Notion's note-before-status write ordering (it has
  real transactions) but **keeps the same order and rule anyway** —
  deliberate consistency across backends, not an accident of the schema.
- Tables: `project`, `milestones`, `slices`, `slice_deps`, `sync`. `sync`
  (`slice_id`, `dirty`, `synced_at`) exists in the schema but nothing reads
  or writes it except `DeleteSlice`'s cleanup — scaffolding for a future
  "Notion as replica" sync, not dead code and not yet meaningful to read.
- `position` (REAL) is the order column for both slices and milestones — no
  view read, no second round trip. Slice ties break on `id`, so two writers
  landing on the same position get a stable order rather than a flapping
  board.
- `ReorderSlice` writes the midpoint between the target and its neighbour
  *within the target's milestone* (positions are only ordered within one —
  `Hydrate` numbers per milestone, so they tie across milestones), one step
  past the target at an end; a tie with the neighbour is refused. Only a
  refile marks dirty; `Notion.ReorderSlice` is the refile alone.
- Foreign keys make an impossible dependency literally impossible:
  `slice_deps` references `slices(id)` both ways, so a dependency on a slice
  not in the plan is refused by SQLite itself. `DeleteSlice` clears
  `slice_deps` rows on both sides (`slice_id OR depends_on`) before deleting
  the slice, to avoid tripping that FK.
- Ownership has no user directory: `AssigneeName` is a bare string, and that
  string **is** the identity `Holds` compares against.
- `newLocalID()` mints a UUID-shaped id (`crypto/rand`, `8-4-4-4-12` hex)
  since there's no page-create to hand one back.
- The last-`## PR description`-section read is `notion.PRDescriptionOf`'s
  rule reimplemented over raw markdown instead of blocks — same
  last-matching-heading-wins semantics, fence-aware so a description quoting
  a diff or shell session isn't cut short by a `#` inside a code fence.
- The full-text index the design eventually wants is still the next slice's
  work — `user_version` is exactly what makes adding it later one migration
  rather than a second format.
