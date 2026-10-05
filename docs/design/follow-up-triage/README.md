# Follow-up triage — design

Status: **agreed with Craig 2026-10-01**, filed as a slice. Written against
`origin/main` (post-M56). `mock/` holds the rendered screens this describes:
`1-proposed.png`, `2-applied.png`, `3-handed-back.png` and the three sidebar
states; the `.html` beside each is its source.

**Amended 2026-10-05: several batches at once.** Every *Follow-ups* section is
a batch, pending until its own items are decided — no section supersedes
another — and the app draws one triage card per pending batch in the Task log
(the pane-level sidebar below has since gone; the card replaced it). The
sections below are updated where they said last-section-wins or all-at-once.

## Problem

A slice agent routinely notices work beside its change — a bug in the file
it was editing, a test gap next door, a refactor the brief didn't ask for.
The prompt tells it to list "follow-ups worth queueing" as bullets in
`--summary`, so they land in the slice page's *Handed back* section as
prose. Nothing reads them back out: they're quoted to a future agent in the
milestone digest and otherwise lost. The user has no way to act on them
short of retyping each one as a slice.

## The shape of it

**The agent proposes before it hands back, and waits.** When the work is
done and the gate is green, an agent with follow-ups runs
`nat slice-followups` instead of `nat complete-slice`, and stops. The slice
stays *In progress*; the rail reads *Waiting for input*; the pane stays on
the Agent tab. gnat shows the proposals in a **Follow-ups sidebar** and the
user decides, per item:

- **Queue** — it becomes a Todo slice, blocked on this one.
- **Fold in** — the agent does it as part of this slice before handing back.
- **Drop** — ignore it.

**Discard All** drops everything in one click. **Apply** sends one message to
the agent carrying the whole decision; the agent folds in what it was told
to and then runs `complete-slice` exactly as it always has. `complete-slice`
refuses while a decision is outstanding, so a slice never reaches review
with proposals undecided — the workflow stage machine needs no new case.

## Non-goals (v1)

- Editing a proposal's title or brief before queueing — a queued slice is
  Todo, so its brief is editable the ordinary way straight after.
- Choosing the milestone a queued slice lands in (it goes under the
  parent's; move it afterwards).
- A TUI surface. The `nat` commands make it possible later.
- Ad hoc sessions (scratch tab). They don't go through `complete-slice`.

## The three screens (`mock/`)

1. **Proposed — agent waiting** (`1-proposed.png`). Agent tab. The terminal
   shows the gate passing, then `nat slice-followups … --follow-up … ×3`
   and the agent's note that it has not handed back. Rail: the slice's dot
   and state are the *Waiting for input* yellow, trailing meta
   "3 follow-ups"; the project tab's dot is yellow too (attention
   `.waiting` already outranks `.review`). Stepper: Brief ✓ → **Agent** →
   Diff → PR. The Follow-ups sidebar is up at the pane's right edge.
2. **Applied — agent folding in** (`2-applied.png`). Agent tab, no sidebar.
   The terminal shows the decision message arriving and the agent working
   on the fold-in. Rail: *Working* (lavender, pulsing), "folding in 1";
   under the milestone in TODO the queued slice sits as a blocked row
   (nosign glyph, tertiary ink). Status bar carries a one-line toast
   "Queued 1 slice · folding in 1 · dropped 1".
3. **Handed back** (`3-handed-back.png`). The ordinary Diff tab after the
   agent ran `complete-slice`: *Needs review*, file list, Approve. Nothing
   about follow-ups is left on screen except the blocked slice in TODO.

## The sidebar

### Placement

A **pane-level sidebar**, drawn by `PaneView` outside the tab switch, so it
is on Brief, Agent, Diff and PR alike while proposals are pending, and
absent — not collapsed, absent — otherwise. Precedent:
`BriefTabView`'s `if let detail … { briefSidebar(detail) }`. No toggle
button; data presence shows and hides it, as with every other sidebar in
the app. It is normally seen on the Agent tab, since that is where the pane
sits while the slice is Working.

### Anatomy

Same stack as the Diff/PR/Brief sidebars: `InspectorActionsBar` →
content → `InspectorStatusFoot`, `PaneResizeHandle` on its leading edge,
width persisted under `@AppStorage("followUpsSidebarWidth")` (default 232,
as Diff's).

- Actions bar: **Apply** (primary, full width) over **Discard All**
  (secondary, full width, destructive red ink).
- Then a section label `FOLLOW-UPS · 3` with a green dot, and a caption
  "Proposed by the agent before hand-back".
- One row per proposal, hairline-separated: title (body emphasized), the
  agent's brief (callout, secondary ink, wraps in full, no truncation), and
  a three-way **native `Picker(.segmented)`** — Queue / Fold in / Drop —
  unset by default. Selected segment inked green for Queue, orange for
  Fold in, label for Drop. No custom checkbox component exists in the app
  and none should be invented; the segmented picker is what Settings and
  the New Project sheet already use.
- Rows scroll; actions bar and foot stay pinned (as `PRSidebarView`).
- **Apply** is enabled only when every row has a choice (`State-Undecided`
  mock: dimmed Apply, foot "Decide every follow-up to apply, or discard
  them all."). Partial decisions are never applied; that is what makes the
  sidebar's disappearance mean "all dealt with".
- **Discard All** needs no confirmation: the proposals stay on the Notion
  page under their heading, with the drop recorded beneath.
- **Fold in** is disabled when no live agent exists (`State-NoAgent` mock:
  the segment dimmed, warning foot "No live agent, so nothing can be folded
  in. Relaunch the slice first, or queue it instead."). Queue and Drop
  always work.
- While applying (`State-Applying` mock): Apply shows a spinner and reads
  "Applying…", every control disabled, foot "Queued 1 · sending 1 to the
  agent…".
- Pending-attention colour: the rail row and tab dot use the existing
  `.waiting` yellow, because that is literally what the agent is doing. The
  sidebar's own dot is `.success` green. No new ink role.

### State

Triage choices are ephemeral view state in a small `FollowUpStore`
(mirroring `DiffStore.comments`), keyed by slice ID so switching tabs or
slices doesn't lose them. Not persisted across launches; the proposals
themselves are, on the page.

## Data model and storage

Proposals live **in the slice page body** as a heading-delimited section —
the same mechanism as *Handed back* and *PR description*. Both stores
already implement it; Notion users see the proposals on the page; no schema
change, no new column, no sidecar file.

### Written by `slice-followups`

Notion (one `AppendBlockChildren` call):

- `heading_3` **Follow-ups**
- one `numbered_list_item` per follow-up — rich_text = title, `children` =
  one `paragraph` per blank-line chunk of the brief (`paragraphBlocks`).

Local (`appendSection`) writes the markdown `notion.Markdown` renders the
above to:

```
### Follow-ups

1. Persist the conversation split width per project

   The split's width lives under one AppStorage key, so every project
   shares it. Store both per project the way pane widths already are.

2. Render the emoji picker open in a gallery story

   …
```

A test must assert the two stores' bodies read back identically for the
same proposals — the local writer is hand-rolled to match what Notion
renders, and this is the only thing that keeps them agreeing.

### Written by `slice-triage`

```
### Follow-ups triaged

- Queued: Persist the conversation split width per project → https://notion.so/…
- Folded in: Render the emoji picker open in a gallery story
- Dropped: Remove the dead reply-threading code in PRConversationView
```

Notion: `heading_3` + one `bulleted_list_item` per line.

### Reading it back

`store.TaskEvents(body)` reads every *Follow-ups* section as a **batch** (a
`follow_ups` event, numbered by its ordinal among the body's *Follow-ups*
sections, 1-based); items begin at a `^\d+\. ` line (title), their brief is
the de-indented lines up to the next item or the section end. A *Follow-ups
triaged* section decides items of any batch written before it: each of its
lines decides the **earliest still-undecided** item with that title, exact —
so two batches proposing the same title are two items, decided by two lines.
Fence-aware like its neighbour. `store.PendingFollowUps(body) []FollowUp` is
that reading's undecided items, in body order, each with its `Batch` and an
`Index` counting every pending item — the matching lives once, in
`TaskEvents`.

A second `slice-followups` call (the agent proposing again after talking to
the user) starts a new batch, pending beside the first; neither supersedes
the other. The agent is told a later hand-in carries only what is new.

A Done slice's undecided items (a batch an older nat let a later one
supersede, never triaged) are history: `store.PendingFollowUpsOf` reads
nothing pending on a Done slice, so nothing refuses on them and the app draws
them as a plain record.

## The `nat` contract

### New: `slice-followups`

```
nat slice-followups <slice> --project <id> --follow-up '<title line>

<brief>' [--follow-up …]
```

Repeatable; the value's first line is the title, the rest the brief — the
same shape as `--pr-description`, so agents already know the quoting.
Refused: on a slice the caller doesn't hold (`store.Holds`, as
`complete-slice`); with no `--follow-up`; with an empty title or brief;
with two identical titles (titles key the triage record). Writes the
section, nudges, prints what it filed and the sentence the agent is told to
heed: *waiting for the user's decision — it arrives as a message; do not
hand back before it does.*

### `complete-slice` refuses while a decision is outstanding

Before any write, after the holder check: if `PendingFollowUps` of the
slice's body is non-empty, refuse — "N follow-ups await the user's
decision; hand back once it has arrived". `--blocked` is exempt: a blocked
agent is allowed to stop.

### `slice-show --json` gains `followUps`

```json
"followUps": [
  { "batch": 1, "index": 1, "title": "Persist the conversation split width per project", "brief": "…" },
  { "batch": 2, "index": 2, "title": "Name the pane's activity states in one enum", "brief": "…" }
]
```

Pending ones only, every batch's, each with its batch and its 1-based index
among all pending items. Absent/empty when none (always on a Done slice).
Each `follow_ups` event in `events` carries the same `batch`, so the app pairs
a log event with its own pending items and nothing else. The app's one read;
it already calls `slice-show` on selection.

### New: `slice-triage`

```
nat slice-triage <slice> --project <id> --json \
    [--queue N]... [--fold N]... [--drop N]... | --drop-all
```

`N` is the index from `slice-show`. A call decides **whole batches**: every
pending item of each batch it names must be named exactly once, other
batches may be left untouched, or `--drop-all` given alone (every batch) — a
partial batch is refused before any write, mirroring the card's Apply rule.
So is deciding an item whose title an earlier, still-pending batch also
proposes, unless that batch is decided in the same call: the record's line
would decide the earlier one's item instead. The record names only what this
call decided, in pending order, and the message tells the agent only that. Refused on a Todo
slice and when nothing is pending. Any `--fold` is refused when the slice
has no live session (`tmux.LiveSlices`, as `agent-send`), since the
decision can't be delivered.

In order:

1. Read the body, compute pending, validate the flags against it.
2. **Queue**: `AddSlice` under the parent's milestone, Todo, unassigned,
   `DependsOn` = the parent slice. Brief = the follow-up's brief, a blank
   line, then `Proposed by the agent working "<parent title>" (<url>).`
   The dependency is the right default: a follow-up almost always builds
   on the parent's branch, and the parent isn't Done until that merges.
3. Append the *Follow-ups triaged* record (new `Store.RecordTriage`, both
   backends) naming every item, with queued slices' URLs.
4. **Tell the agent**, one `agent-send`, whatever the mix — it is waiting
   for exactly this:

   ```
   Follow-ups decided. Queued as slices: 1 (<title>). Dropped: 3.
   Fold in before handing back:

   2. <title>
      <brief>

   Then hand back with nat complete-slice as usual.
   ```

   With nothing to fold in the middle goes: `Nothing to fold in — hand back
   now with nat complete-slice as usual.`
5. Nudge. `--json` prints `{ queued: [{title, id, url}], folded: [titles],
   dropped: [titles] }`.

Step 3 before step 4 is deliberate: once the record is written,
`complete-slice` stops refusing, so the agent can hand back the moment the
message lands. If step 4 fails anyway (session died between the check and
the send) the command exits non-zero naming it; the app shows that in the
foot and the user relaunches the slice — the record stands, and the
relaunch prompt already tells an agent it is continuing.

### `NatClient`

- `sliceShow` decodes the new field into `SliceDetail.followUps`.
- New `sliceTriage(projectID:sliceRef:queue:fold:drop:)` → the command
  above; a card's Discard all is the same call dropping its batch's indexes
  (`--drop-all` would take every batch with it).
- `FixtureNatClient` grows it, for the gallery.

### Rail and attention

Nothing new to compute. The agent is idle at its prompt after proposing,
which `AgentActivity` already classes as *Waiting for input*; the rail row
and `ProjectAttention` go yellow on their own. The trailing meta on the
rail row shows the pending count ("3 follow-ups") where a review row shows
its `+n −m` — read from `slice-show` for the selected slice, so it is
present for the slice the user is looking at and may be absent for others.

## Prompt and skill changes

`internal/agent/prompt.go` *Finish* section and `skills/next-slice/SKILL.md`
step 5 — **both copies, independently**, per the repo's standing rule on
prose handed to an LLM. Replace the "follow-ups worth queueing" clause in
the `--summary` guidance with a *Follow-ups* passage:

> Work you noticed but did not do — a bug beside your change, a test gap
> in code you didn't touch, a refactor the brief didn't ask for — is not
> yours to do and not yours to lose. When the gate is green, before
> `complete-slice`, hand each one in with
> `nat slice-followups <slice> --project <id> --follow-up '<title line>\n\n<the change: which file or function, what it does instead, and why>\nDone when: <how anyone checks it is finished>'`
> (repeatable) and **stop**. Write each one as a slice brief: if the user
> queues it, this text is the brief of a new slice, word for word — an
> imperative title, the concrete change, a `Done when:` line, and a decision
> rather than a question. The user decides on the board — queue as a
> slice, fold into this one, or drop — and the decision arrives here as a
> message naming what to fold in. Do that, then hand back as below.
> `complete-slice` refuses while the decision is outstanding. Never widen
> your branch to include a follow-up on your own, and never write them into
> the summary or the brief instead. No follow-ups: hand back straight away.

The fix prompt (`agent.fixPrompt`) gets the same passage. The existing
template-walking test that checks every `nat` invocation is
`--project`-pinned covers the new example.

## Edge cases

- **Done slice**: unreachable with pending proposals, since `complete-slice`
  refuses — except via `--blocked`, where the slice stays In progress and
  the sidebar simply remains until the user decides.
- **Agent dies while waiting**: Fold in disabled, Queue/Drop still apply;
  relaunch resumes it. A relaunched agent sees the triage record in the
  page body if it reads `slice-show`; the fold-in list is also the
  relaunch's own concern to re-send — out of scope, the user can type it.
- **Fold-in with pending diff comments**: independent; a Working slice has
  no review to comment on yet.
- **Local store / Notion**: identical behaviour; the markdown shape is the
  contract between them.

## Testing

- Go: `slice-followups` flag validation and both stores' writers (httptest
  asserting exact block JSON for Notion; SQLite body for Local) plus the
  round-trip equality test; `complete-slice`'s refusal; the parser over
  hand-written bodies (none, one section, superseded section, partial
  record, fenced `#` inside a brief); `slice-triage` end to end against a
  fake store + fake tmux for each mix including the send failure;
  `slice-show --json` shape.
- Swift: `FollowUpStore` apply-enabled rule; `SliceDetail` decoding;
  gallery stories `followups-pending` (three rows, one of each choice,
  Apply enabled), `followups-undecided`, `followups-no-agent`,
  `followups-applying` — rendered with `gnat --story <name> --out <png>`
  and compared against `mock/`.

## Alternatives considered

- **Propose at hand-back, triage in review.** The first draft. Rejected by
  Craig: the agent's work isn't finished until the follow-ups are decided,
  so it must not hand back first, and the pane must not move on to Diff.
- **Real slices in a `Proposed` status.** Rejected: a Status option on every
  project's select, new state for `StateOf`/`next-slice`/the rail to look
  past, and the plan fills with agent chatter the user hasn't accepted.
- **A local sidecar file.** Rejected: invisible in Notion, lost across
  machines, a third place slice state lives.
- **JSON or a file for the proposals.** Rejected in favour of a repeatable
  flag in a shape agents already use.
