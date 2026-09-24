# internal/notion

Hand-rolled stdlib Notion client (`Notion-Version: 2026-03-11`, data-source
model) — no third-party client supports data sources. Minimal structs, only
the fields used. Nothing outside `internal/store` constructs one:
`store.NewClient` is the single call to `NewWithToken`.

## Client / auth (`client.go`)

- `NewWithToken(TokenFunc, ...Option)` fetches the token **per attempt**, not
  once — a token rotated by `ntn` mid-session is picked up on the next call.
- A 401 is retried **exactly once**, with a fresh token (`refreshed` flag);
  the retry does not consume a rate-limit attempt. A second 401 means the
  credential itself is bad, not stale — looping on it would only hammer the
  CLI.
- A 429 is always retried (nothing was applied server-side). A 502/503 is
  only retried for requests **safe to repeat** (`safeToRepeat`): every GET,
  plus Notion's read-shaped POSTs (`/query`, `/search`) — never a POST that
  creates or updates a page, which could duplicate the write.
- Backoff: `Retry-After` wins when Notion sends a usable one (429s do);
  otherwise exponential from one second, capped.
- Never log the token or a request body — `internal/logging`'s redactor is
  the enforcement point, but don't add a new logging call here that bypasses
  it.

## Views & plan order (`views.go`)

- `PlanOrder`/`DataSourceOrder`/`ViewOrder` are the **only** way to read a
  data source's row order — a row's position is not a property and nothing
  in the API writes it. `GET /views` (or `?data_source_id=`) returns bare
  stubs; the actual order comes from `POST /views/{id}/queries` (then
  cursor-paginated `GET`s under the same query ID — Notion caches a query
  for 15 minutes, and each call here starts a fresh one).
  - `DataSourceOrder` always takes the data source's **first view** — the one
    Notion opens by default, so the plan is read the way its owner reads it,
    whatever that view's shape.
  - `PlanOrder`'s failure is logged and swallowed, **not returned** — an
    unordered plan is still a plan, drawn in whatever order the pages were
    queried in.
  - Notion records `created_time` only to the **minute**: that's no order at
    all for a plan written in one shot, which is exactly why `PlanOrder`
    exists instead of sorting by creation time.

## Migration (`migrate.go`) — read before touching an old-shape project

`MigrateProject(ctx, api, slicesDSID)` converts a project still in the old
shape (a Milestones database + a `Milestone` relation, and/or a `Claimed`
status option) in place, on **every** load, by both the board and every
headless command:

- Milestone pages → options of a `Milestone` select, in plan order; every
  slice refiled onto the option its relation named; the Milestones database
  trashed (recoverable) **only after everything else succeeds**.
- `Claimed` → `In progress` the **long way**: append the new option, refile
  every slice holding the old one, then drop it. **Renaming a select option
  in place is silently ignored by the API** — a 200 whose body still echoes
  the old name. This is a general Notion API gotcha, not specific to this
  one rename: check the echoed value, never the status code, for any select
  option write.
- The **whole plan is read before any schema change** — converting a column
  discards the relation it was read from, so reading after would lose data.
- Idempotent: an already-migrated project is read and left untouched on
  every later load.
- A `Status` column converted to Notion's own status type **in the Notion
  UI** cannot have its options rewritten by the API at all — such a project
  is refused outright, with the one manual edit to make, rather than
  half-migrated.
- One step runs for every project regardless of old/new shape: a missing
  `Depends on` or `Branch` column is added, and a `Depends on` still held one
  -sided is given its `Blocks` reciprocal (`addColumns`) — `CreateProject`
  only writes these for projects it creates.
- `milestone-rename` uses the identical long-way pattern (`OptionInsertedAfter`
  next to the old option, not appended, so order is preserved; refile;
  `WithoutOption` last) for the same silently-ignored-rename reason.

## Self-relations mirror — why `Depends on` needs a `Blocks` half (`schema.go`)

- A **single-property** self-relation is mirrored by Notion automatically:
  writing `A.dependsOn=[B]` also writes `B.dependsOn=[A]` on the far page.
  For a directional relation like a dependency, that's fatal — it reads back
  as a **mutual block**, wedging both slices, since neither `next-slice` nor
  the launch key will step past a slice waiting on one that's waiting on it.
- The fix is `RelationConfig.Kind = "dual_property"`: give the relation a
  **second** column (`Blocks`) for Notion to mirror into instead of back into
  `Depends on`. `Blocks` is never read by anything — it exists purely to give
  the API's own mirroring somewhere else to write, so `Depends on` stays
  genuinely one-directional.
- `notion.SingleSelfRelation` converts a project whose `Depends on` predates
  this fix. It only converts a relation that points at the **slices
  themselves** — one pointing anywhere else is a same-named column that
  happens to share the name, and re-targeting it would discard what it holds.

## Other API-shape gotchas (verified live against the API)

- A `child_database` block looks identical whether the database is inline or
  full-page — `is_inline` lives on the **database object**
  (`GET /v1/databases/{id}`), never on the block. Never infer inline-ness
  from a block listing.
- Select option-list writes are a **wholesale replace**: entries with an ID
  keep their stored name (a rename-by-ID is silently ignored, same rule as
  above); name-only entries are created; entries omitted from the list are
  removed. `PropertySchema.AppendedOptions`/`OptionInsertedAfter`/
  `WithoutOption`/`OptionMoved` all exist because of this — they always
  round-trip every existing option (ID, colour) to avoid an accidental drop.

## PR descriptions (`prdescription.go`)

- `PRDescriptionOf(blocks)` reads the blocks between a `## PR description`
  heading (matched case-insensitively, any heading level) and the next heading of the
  same-or-higher level, rendered back to markdown.
- The **last** matching section wins, not the first — a slice handed back
  twice (reviewed, commented on, pushed again) carries one section per
  hand-back, and the current description is the one written last.
- No such heading (every hand-back written before the flag existed) reads as
  empty — the caller's cue to let `gh --fill` build the PR from commits.

## Shape (`project.go`)

- `ShapeOf(ds)` reads a Slices data source's shape once: `StatusType`
  (select or status — either may have been converted in the UI),
  `HasAssignee`/`HasBranch` (column present **and** the expected type),
  `MilestoneType`/`MilestoneOptions` (in schema order = plan order). This is
  the `notion.SliceShape` that becomes `store.Shape`'s Notion half — see
  `internal/store/CLAUDE.md` for how a caller is meant to hold and reuse it.
