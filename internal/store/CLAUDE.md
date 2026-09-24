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
columns and the plan never reads as hydrated).

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
- The wishlist is still read outside this package (`nat wishlist`,
  `wishlist-clear`, the workshop launch) — it's a section with its own
  editing rules rather than prose, and belongs to no `Shape`.

## Notion backend (`notion.go`)

- Built over `API`, a narrow interface (`notion.MigrationAPI` + six calls:
  `DataSourceOrder`, `GetPage`, `CreatePage`, `GetBlockChildren`,
  `AppendBlockChildren`, `TrashPage`) so request cost stays visible in one
  place and a fake can drive it.
- Every read runs `notion.MigrateProject` on the way in — a stale-shape
  project is migrated silently but logged/toasted, never left half-read.
- Milestone rename/remove/move mechanics **live here**, not in `internal/cli`
  — the CLI commands are thin wrappers. See root CLAUDE.md's Domain rules
  for the reasoning (rename-goes-the-long-way, remove-refuses-while-filed,
  move-is-cheapest-of-three); this is where it's actually implemented.
- `AddMilestones`/`RenameMilestone`/`RemoveMilestone`/`MoveMilestone` each
  read the plan before their first write and refuse before it.
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
- DSN: `journal_mode(wal)`, `busy_timeout(5000)`, `foreign_keys(on)`,
  **`_txlock=immediate`** — every transaction is `BEGIN IMMEDIATE`, never
  deferred. A deferred transaction takes its read lock at the first `SELECT`
  and only asks for the write lock later, which is the **one** lock upgrade
  SQLite refuses outright rather than waits out the busy timeout for — so
  two concurrent writers fail fast rather than queue as intended.
- Schema is stamped in `PRAGMA user_version` (currently `1`). `OpenLocal`
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
