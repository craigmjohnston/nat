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
every `nat` command takes `--project <id>` — the project's page ID — and every
one you run should carry it. Without it a command acts on
whichever project the user's board is on, and that is something they switch
while you work: an unpinned `plan-apply` files the whole plan into the wrong
project.

- If you were launched from the board, your prompt already names the ID.
- Otherwise read it off the active project: `nat info --json` prints it as
  `project.id`. That one call is the only unpinned command in this skill.
- If the user meant a project other than the active one, `--project` is how you
  reach it — you just need its page ID. `nat info --project <whatever they
  called it>` refuses an ID the config does not know by listing every project it
  does know, ID and name. They do not need to switch the board.

Then run `nat info --project <project>` to see what you are planning into: its
conventions, its milestones in plan order, and its slices grouped under them.
(`--json` if you would rather parse it.) `<project>` below is that ID.

## Drafting rules

- A **slice** is a small unit of work one agent completes in a single fresh
  session. If the work is code, a slice maps to **exactly one PR** — split
  anything bigger.
- Each slice gets: a clear imperative title, a 2–6 sentence description
  (written as a self-contained brief: what, where, acceptance criteria), a
  milestone, and — only when it deviates from the project's default working
  directory — a `repo` override.
- Slot new slices into existing milestones when they fit; create new
  milestones only for genuinely new phases of work.
- Status, order and assignee are not yours to choose: `nat plan-apply` files
  new milestones at the end of the plan and new slices as `Todo` and
  unassigned. A milestone's status follows its slices — there is none to set,
  on the board or anywhere else; agents claim their own slices at work time.

## Procedure

1. Present the proposal in chat as a compact tree: each milestone (marked
   NEW where applicable) with its slices, titles + one-line summaries, plus
   any repo overrides. Note anything you chose to leave out or split.
2. **Write nothing until the user explicitly approves.** Iterate on their
   feedback by revising the proposal, not by writing part of it.
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
     ]
   }
   ```

   `milestone` names one of the plan's own new milestones, or an existing one
   of the project, by name — a milestone is an option of the slices' own
   `Milestone` column, so its name is all there is to name it by.
   `description`, `repo` and `depends_on` are optional, as is the whole
   top-level `dependencies` list; nothing else is, and any other key is
   rejected. The whole document is validated before the first page is created.

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
4. Report the created page URLs, grouped by milestone — `plan-apply` prints
   them.

## Launched on the wishlist

The board can start you on the project's wishlist — the ideas the user has
been jotting on the project page — in which case your prompt carries the items
and their block IDs, and they are the request: draft from them rather than
asking what to work on. If you were not launched that way,
`nat wishlist --project <project>` (add `--json` for the IDs) reads the same
items.

Clearing captured items is the last step, after step 4 above:

```
nat wishlist-clear <block-id>... --project <project>
```

- Only after the plan is written. An item cleared before that is an idea lost.
- Only the items you read, named one by one. The command never empties the
  section, because an idea the user typed while you were drafting has to
  survive your tidy-up.
- Only the items the plan actually covers. Anything the user set aside stays
  on the wishlist for next time — say which ones you left.

## Guardrails

- Everything you write goes through `nat`. Never edit Notion directly.
- Every `nat` command carries `--project <project>`, the ID you settled in
  setup — the one read that finds it is the only exception.
- `plan-apply` only ever creates. Existing milestones and slices — and above
  all anything in progress or `Done` — are left exactly as they are.
- If a run fails partway, it says what it had already created. Trim those out
  of the plan before running it again rather than filing them twice.
- If `nat info --project <project>` shows a tracker that does not match what
  the user described, stop and tell them instead of improvising.
