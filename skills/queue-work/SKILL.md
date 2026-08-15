---
name: queue-work
description: Plan project work into the Notion agent tracker — draft milestones/slices from a work description, get explicit user approval, then write them with `nat plan-apply`.
---

# /queue-work — plan work into the Notion agent tracker

You help the user turn a description of work into milestones and slices in
their agent tracker. You draft a proposal in chat first; you write **only
after the user explicitly approves**, and only through the `nat` CLI.

## Setup

Run `nat info` to see the project you are planning into: its conventions, its
milestones in plan order, and its slices grouped under them. (`nat info
--json` if you would rather parse it.)

`nat` works on whichever project local config marks active. If the user meant
a different one, tell them to switch the active project on the board (`nat`)
and rerun — the CLI has no project switch of its own.

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
  new milestones as `Queued` at the end of the plan and new slices as `Todo`
  and unassigned. The user activates milestones from the board; agents claim
  their own slices at work time.

## Procedure

1. Present the proposal in chat as a compact tree: each milestone (marked
   NEW where applicable) with its slices, titles + one-line summaries, plus
   any repo overrides. Note anything you chose to leave out or split.
2. **Write nothing until the user explicitly approves.** Iterate on their
   feedback by revising the proposal, not by writing part of it.
3. On approval, write the whole plan in one go by piping this document to
   `nat plan-apply`:

   ```json
   {
     "milestones": [{ "name": "M14: Something new" }],
     "slices": [
       {
         "title": "Do the thing",
         "milestone": "M14: Something new",
         "description": "The brief, as it should read on the page.",
         "repo": "/path/only/when/it/differs"
       }
     ]
   }
   ```

   `milestone` names one of the plan's own new milestones, or an existing one
   by name, URL or ID. `description` and `repo` are optional; nothing else is,
   and any other key is rejected. The whole document is validated before the
   first page is created.
4. Report the created page URLs, grouped by milestone — `plan-apply` prints
   them.

## Launched on the wishlist

The board can start you on the project's wishlist — the ideas the user has
been jotting on the project page — in which case your prompt carries the items
and their block IDs, and they are the request: draft from them rather than
asking what to work on. If you were not launched that way, `nat wishlist` (or
`nat wishlist --json`, which gives the IDs) reads the same items.

Clearing captured items is the last step, after step 4 above:

```
nat wishlist-clear <block-id>...
```

- Only after the plan is written. An item cleared before that is an idea lost.
- Only the items you read, named one by one. The command never empties the
  section, because an idea the user typed while you were drafting has to
  survive your tidy-up.
- Only the items the plan actually covers. Anything the user set aside stays
  on the wishlist for next time — say which ones you left.

## Guardrails

- Everything you write goes through `nat`. Never edit Notion directly.
- `plan-apply` only ever creates. Existing milestones and slices — and above
  all anything in progress or `Done` — are left exactly as they are.
- If a run fails partway, it says what it had already created. Trim those out
  of the plan before running it again rather than filing them twice.
- If `nat info` shows a tracker that does not match what the user described,
  stop and tell them instead of improvising.
