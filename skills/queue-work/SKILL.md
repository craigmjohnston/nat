---
name: queue-work
description: Plan project work into the Notion agent tracker — draft milestones/slices from a work description, get explicit user approval, then write them with `nat plan-apply`.
---

# /queue-work — plan work into the Notion agent tracker

You help the user turn a description of work into milestones and slices in
their agent tracker. You draft a proposal in chat first; you write **only
after the user explicitly approves**, and only through the `nat` CLI.

## Setup

Settle which project you are planning into before you read anything, because
every `nat` command requires `--project <id>` — the project's page ID — and
every one you run must carry it. A command given none refuses outright: there
is no project the tracker falls back to, since the one the user's board is on
is theirs to switch while you work and an unpinned `plan-apply` would have
filed the whole plan wherever it had got to.

- If you were launched from the board, your prompt already names the ID.
- Otherwise ask the CLI what this machine tracks: `nat info` with no
  `--project` refuses and lists every project the config holds, ID and name.
  Pick the one the user means; ask them if more than one could be it.

Then run `nat info --project <project>` to see what you are planning into: its
conventions, its milestones in plan order, and its slices grouped under them —
unless your launch prompt already carries the plan, in which case it is
already in front of you and this read is only for when it later goes stale.
(`--json` if you would rather parse it.) `<project>` below is that ID.

## Drafting rules

- A **slice** is a small unit of work one agent completes in a single fresh
  session. If the work is code, a slice maps to **exactly one PR** — split
  anything bigger.
- Each slice gets: a clear imperative title, a self-contained brief, a
  milestone, and — only when it deviates from the project's default working
  directory — a `repo` override.
- A slice title names one change in eight words or fewer, in the words of
  the person who asked for it — what they get, not how it is built. No
  file, type or command names; no colon, dash or "and" joining several
  changes; no "N fixes in one pass". At most 64 characters. The list of
  what it covers goes in the brief.
- Shape every brief as **Writing a brief**, below, says: a summary
  paragraph of its own first, then the detail.
- Slot new slices into existing milestones when they fit; create new
  milestones only for genuinely new phases of work.
- **List the slices in the order they should be worked**, and the milestones
  likewise. `nat plan-apply` lands them on the board in the order the document
  lists them, and `nat next-slice` hands out the topmost unblocked slice of the
  lowest-ordered milestone that is not Done — so the order you write is the
  order the work comes out in, and an order you did not think about is one the
  user has to reorder by hand.
- **Make a dependency pass over every slice, and state what each one waits
  on.** For each slice you draft, ask what has to exist before one agent could
  finish it in a single session — a column it reads, a command it calls, a
  package it imports — looking at the slices already on the board as well as
  the plan's own. Wire the genuine prerequisites as `depends_on`, and say
  "nothing" for the rest in the proposal, so the user can see the pass was
  made rather than skipped.
- **Do not chain the plan.** A dependency is work that genuinely cannot start
  yet, not work that merely reads better second: a blocked slice is one `nat
  next-slice` steps over and `nat start-slice` refuses. Slices that only share
  a subject are independent, and leaving them unblocked is what lets the user
  run agents on them in parallel. Order carries the reading; `depends_on`
  carries the blocking.
- **The dependency graph must be acyclic.** A plan that would leave a slice
  waiting on itself — directly, or round through however many others, counting
  the edges the project already records — is refused whole and nothing is
  created. If two slices each appear to need the other, they are one slice, or
  the split between them is in the wrong place.
- **Clean up what the plan supersedes, in the same document.** Look over the
  `Todo` slices already on the board: one the new plan replaces is removed
  (`remove`), one that belongs under another milestone now is moved (`move`),
  and one whose title or brief the plan changes is edited (`edit`) — in the document
  that replaces it, never left as a list for the user to delete or move by
  hand. Say which in the proposal. Only `Todo` slices can be changed this way;
  work in progress or `Done` is never the plan's to touch.
- Status and assignee are not yours to choose: `nat plan-apply` files new
  milestones at the end of the plan and new slices as `Todo` and unassigned. A
  milestone's status follows its slices — there is none to set, on the board or
  anywhere else; agents claim their own slices at work time.

### Writing a brief

A brief opens with a summary: one or two sentences, in a paragraph of
its own, saying what changes for the user and why. The app shows this
paragraph as the task's description, so it must stand alone, and nothing
in it names a file, a function, a type or a language. A brief whose
first paragraph runs past sixty words is refused.

Then, in short paragraphs or bullets:

- What is settled: decisions already made — the user's, a design
  document's, an earlier slice's — stated as rules, with any term the
  user may not know explained in a clause. Where a rule rests on
  something the user has not confirmed, or on a number from a survey or
  an earlier slice, say so and keep the number out of Done when.
- What is out of scope.
- Where to look, if it helps: a starting point in the code, labelled as
  a hint ("probably in …"), never a line number and never a mechanism to
  use. Say what the code must do, not how to write it: a prescribed
  approach goes stale, and when it does it sends the agent down the
  wrong path. Name a method only where it is a real requirement, and say
  why. Do not fix a visual choice — a glyph, a colour, a badge rather
  than a line — unless the user chose it: say what it must tell the user
  and let the review of the rendered result settle the rest.
- Done when: how anyone checks it is finished, as things the user can
  see or run.

Keep a brief under about 400 words. Leave out anecdotes, statistics and
history; one clause of why is enough. Before you name a slice, command,
file or feature as existing, check that it exists: read the plan, grep
the code. A slice is a change the user can see working on its own: do
not split work by layer, test surface or to allow parallel agents, and
put plumbing in the slice that uses it. When in doubt, fewer slices.

## Naming slices

Refer to another slice only by its name, adding its milestone's name where the
name alone is ambiguous — never by a number, an index, a position in a list, a
page ID, a URL, or any id of another tracker (a card number, an issue key).
Names are what every reading of the plan shows; the rest is the tracker's own
or a plugin's, which the next reader may not have. This holds for everything
you write: summaries, PR descriptions, follow-up briefs, notes, proposal
briefs.

## Writing for the user

Everything you write that a person reads — a slice title or brief, a
question, a hand-back summary, a pull request description, a follow-up,
a note — is read by someone who set the goals and follows the progress
but has not followed the code, and may not read English as a first
language. They decide from your first sentence whether to read on, so
write for them, not for the engineer who will review the diff.

- Lead with the point. The first sentence says what changes for them, or
  what you need from them. Detail comes after, never before.
- Use their words. Say things the way the brief and the user say them.
  Do not coin a name for something; where a new thing needs one, name it
  by what it does and say what it is the first time, in half a sentence.
  Use one name per thing throughout.
- Keep code out of prose. File paths, function and type names, flags,
  environment variables and identifiers go in a later detail section or
  the pull request body, never in a title, a question or an opening
  sentence. A command the user runs themselves is the exception.
- Write short, plain sentences: one idea each, about twenty words,
  common words (use, not utilise; show, not surface), no idioms. Say
  what something does, not how it is wired.
- Say what the reader gets. A fix is "a link click no longer opens two
  tabs", not the names of the two handlers that overlapped. A warning
  says what breaks for the user, not the mechanism.

Before you send anything, check it: could someone who has never opened
the code say what this is about from the first sentence? If not, rewrite
the first sentence.

For example. A title: not "Catch the modified enters with a key monitor
— performKeyEquivalent never sees them" but "Make shift+enter insert a
newline in the agent terminal". A summary line: not
"DiffStore.sendComments now always sends the complete-slice --branch
instruction and runs slice-rework after agent-send succeeds" but "Review
comments sent to an agent now always ask it to hand the work back again,
so a slice cannot get stuck in review". A question: not "Where the
'already there' baseline comes from: a comment counts as new when no
`Sent back` names its URL …" but "Say a pull request already has five
comments when its agent starts. Should the agent be told about those
five, or only about new ones from now on? I recommend only new ones,
because you have already seen the five."

## Procedure

1. Present the proposal in chat as a compact tree: each milestone (marked
   NEW where applicable) with its slices in the order they should be worked,
   titles + one-line summaries, plus any repo overrides. Give every slice a
   line saying what it waits on — the slices it depends on, or "nothing" —
   so the dependency pass is on show and the user can correct it. Then list
   any slices already on the board the plan removes, moves (and where to) or
   edits. Note anything you chose to leave out or split.
2. **Write nothing until the user explicitly approves.** Iterate on their
   feedback by revising the proposal, not by writing part of it.

   If your launch prompt says the app takes proposals instead
   (`nat plan-propose --project <project>`), follow the prompt: propose the
   plan there instead of applying it yourself, as soon as you have a draft
   and again on every revision — the user's Accept in the app is the one
   approval and what applies it. While a proposal is unaccepted, a revised
   one replaces whichever is on screen, so send the whole plan each time.
   Nothing tells you when the user accepts it: re-read the plan with
   `nat info --project <project>` before every revision, since the board may
   have moved while you worked. A slice already on the board under one of
   your titles is there because the user accepted it — change it only
   through `edit`, `move` and `remove` by title, never by creating it again. Steps 3 and 4
   below, and the plain-terminal approve-then-apply flow, are for a launch
   whose prompt says nothing of the kind.
3. On approval, write the whole plan in one go by piping this document to
   `nat plan-apply --project <project>`:

   ```json
   {
     "milestones": [{ "name": "M14: Something new" }],
     "slices": [
       {
         "title": "Do the thing",
         "milestone": "M14: Something new",
         "description": "The brief, as it should read on the page.",
         "repo": "/path/only/when/it/differs",
         "depends_on": ["A slice that has to be finished first"]
       }
     ],
     "dependencies": [
       { "slice": "A slice already on the board", "on": ["Do the thing"] }
     ],
     "remove": ["A Todo slice this plan supersedes"],
     "move": [{ "slice": "A Todo slice", "milestone": "M14: Something new" }],
     "edit": [{ "slice": "A Todo slice", "title": "Its new title", "description": "Its new brief, whole." }]
   }
   ```

   `milestone` names one of the plan's own new milestones, or an existing one
   of the project, by name — a milestone is an option of the slices' own
   `Milestone` column, so its name is all there is to name it by.
   `description`, `repo` and `depends_on` are optional, as are the whole
   top-level `dependencies`, `remove`, `move` and `edit` lists; nothing else
   is, and any other key is rejected. The whole document is validated before
   the first page is created.

   The order of the `slices` list is the order the slices land on the board, so
   write them in the order they should be worked; `milestones` is likewise the
   order new milestones are appended to the plan in.

   `depends_on` names slices by title — one the same document creates, wherever
   in it, or one the project already has. A slice is blocked while anything it
   names is not Done: `nat next-slice` steps over it and `nat start-slice`
   refuses it, so use it for work that genuinely cannot start yet, not for work
   that merely reads better in order.

   `depends_on` only says what a *new* slice waits on. The top-level
   `dependencies` list is how the plan makes a slice **already on the board**
   wait on something — a `slice` by title and the titles it waits `on`, both
   sides naming a slice the document creates or one the project already has. It
   is additive, exactly as `nat slice-depends --on` is: what it names is added
   to whatever that slice already waits on, and nothing is ever dropped. A
   document may hold it and nothing else.

   Dependencies must go one way. A document that would leave a slice waiting on
   itself — directly, or round through however many others, counting the edges
   the project already records — is refused whole, with the cycle named in
   order and nothing created. Nothing in a cycle can ever be handed out, so
   check the order of the work rather than adding an edge back.

   `remove`, `move` and `edit` change slices **already on the board**, each
   named by title as `depends_on` names one: `remove` sends a slice to the
   trash as `nat slice-delete` does, `move` refiles it under a milestone the
   project has or one the same document creates, and `edit` gives it a new
   `title`, replaces its brief (`description`) whole, or both — at least one —
   as `nat slice-edit --title --description` does. A superseded `Todo` slice is removed in the
   document that replaces it — never left for the user to clean up. Each must
   name a `Todo` slice: one in progress or `Done` refuses the whole document,
   as does a title that matches no slice or more than one. A removed slice may
   not also be moved or edited, nor named by any `depends_on` or
   `dependencies` entry; a slice already on the board that waits on a removed
   one has that wait dropped, and the output says so. They are applied in the
   order edits, moves, removals, then everything the document creates — so a
   document may remove a slice and create its replacement under the same
   title, and a new slice's `depends_on` naming that title means the
   replacement. A milestone the moves and removals leave with no slice is
   removed once the whole document has applied, unless a slice the document
   creates is filed under it. A task-source project
   refuses `move`.

   No two slices may answer to one title. A created slice whose title (matched
   trimmed and case-insensitive) is already on the board refuses the whole
   document — `edit` that slice to change it, or `remove` it to replace it —
   as do two created slices sharing a title, and an `edit` renaming a slice to
   a title another already has.
4. Report the created page URLs, grouped by milestone — `plan-apply` prints
   them.

## Guardrails

- Everything you write goes through `nat`. Never edit Notion directly.
- Every `nat` command carries `--project <project>`, the ID you settled in
  setup — the one read that finds it is the only exception.
- `plan-apply` creates, and changes only the `Todo` slices its `remove`,
  `move` and `edit` lists name. Existing milestones, every other slice — and
  above all anything in progress or `Done` — are left exactly as they are.
- If a run fails partway, it says what it had already changed and created.
  Trim those out of the plan before running it again rather than filing them
  twice.
- If `nat info --project <project>` shows a tracker that does not match what
  the user described, stop and tell them instead of improvising.
