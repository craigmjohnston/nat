---
name: queue-work
description: Plan project work into the Notion agent tracker — draft milestones/slices from a work description, get explicit user approval, then write to Notion via the Notion MCP.
---

# /queue-work — plan work into the Notion agent tracker

You help the user turn a description of work into milestones and slices in
their Notion agent tracker. You draft a proposal in chat first; you write to
Notion **only after the user explicitly approves**.

## Setup

1. Read `~/.config/notion-agent-tracker/config.json`. It contains the project
   DB, the active project, and per-project `slices_ds_id` /
   `milestones_ds_id` (Notion data source IDs). If the file is missing or has
   multiple projects and the target is ambiguous, ask which project to use.
2. Via the Notion MCP, fetch the current state: all milestones (Name, Order,
   Status) and all non-Done slices of the active project. Also skim the
   project page body for conventions.

## Drafting rules

- A **slice** is a small unit of work one agent completes in a single fresh
  session. If the work is code, a slice maps to **exactly one PR** — split
  anything bigger.
- Each slice gets: a clear imperative title, a 2–6 sentence description
  (written as a self-contained brief: what, where, acceptance criteria), a
  milestone, and — only when it deviates from the project's default working
  directory — a `Repo` override.
- Milestones get a Name, an `Order` continuing the existing sequence, and
  Status `Queued` (never `Active` — the user activates milestones from the
  TUI). New slices get Status `Todo`.
- Slot new slices into existing milestones when they fit; create new
  milestones only for genuinely new phases of work.

## Procedure

1. Present the proposal in chat as a compact tree: each milestone (marked
   NEW where applicable) with its slices, titles + one-line summaries, plus
   any repo overrides. Note anything you chose to leave out or split.
2. **Write nothing to Notion until the user explicitly approves.** Iterate
   on their feedback by revising the proposal, not by partial writes.
3. On approval: create milestone pages first (in `milestones_ds_id`), then
   slice pages (in `slices_ds_id`) with the Milestone relation set and the
   description as the page body.
4. Report the created page URLs, grouped by milestone.

## Guardrails

- Never modify slices with Status `Claimed` or `Done`.
- Never set or change `Assignee` — claiming is the agent's job at work time.
- Never change milestone Status of existing milestones.
- If the tracker schema doesn't match expectations (missing properties),
  stop and tell the user instead of improvising.
