---
name: next-slice
description: Pick up and complete the next available slice of a project tracked in the Notion agent tracker — claim it, do the work per project conventions, and mark it Done. Use when the user says things like "pick up the next slice", "work on the next slice for <project>", or "grab a slice".
---

# /next-slice — pick up and complete the next slice

You are an agent working one slice of a project tracked in the Notion agent
tracker. Your job: select the next available slice, claim it, complete it
according to project conventions, and record the result. You work exactly one
slice per session — never continue to another slice when done.

## 1. Resolve the project

- Read `~/.config/notion-agent-tracker/config.json`.
- If the user named a project ("for X"), match it case-insensitively against
  the `name` fields under `projects`. Otherwise use `active_project_id`; if
  config has exactly one project, use it.
- If no match or ambiguous, list the project names and ask.
- Note the project's `slices_ds_id`, `milestones_ds_id`, `working_dir`, and
  the top-level `assignee_user_id` / `assignee_user_name`.

## 2. Select the next slice

- Via the Notion MCP, query the milestones data source; keep milestones with
  Status `Active`, sorted by `Order` ascending.
- Query the slices data source for slices with Status `Todo` and no
  Assignee, belonging to those milestones. Order within a milestone: oldest
  created first, unless the user asked for a specific slice by name.
- Pick the first such slice from the lowest-Order Active milestone.
- If there are none: report the counts you found (e.g. remaining Claimed
  slices, next Queued milestone name) and stop. Do not activate a Queued
  milestone yourself — suggest the user queue it (TUI or ask) and rerun.

## 3. Claim it — before any work

- Update the slice: set `Assignee` to the configured user and `Status` to
  `Claimed`.
- Re-fetch the slice and verify the assignee is the configured user (another
  agent may have raced you). If the claim didn't stick, go back to step 2
  and pick the next slice.
- Tell the user which slice you claimed, with its Notion URL.

## 4. Do the work

- Read, in order: the slice page body (the brief), the parent project page
  body (conventions), and the repo's `CLAUDE.md`.
- Work in the slice's `Repo` directory if that property is set, otherwise
  the project's `working_dir` from config. If that differs from your current
  working directory, work there explicitly (absolute paths / `git -C`).
- Honor the brief's acceptance criteria and the project's verification gate
  (for this repo: `go vet ./... && go test -race -cover ./...`).
- **If the work is code**: create a branch for the slice, keep the change to
  exactly ONE pull request, commit, push the branch, open the PR (do not
  merge it), and write its URL to the slice's `PR` property.
- **If the work is not code** (docs, research, Notion content): produce the
  deliverable the brief asks for and link/attach it on the slice page.

## 5. Finish

- On completion: set the slice `Status` to `Done`, then append a short
  summary to the slice page body — what you did, key decisions, any
  follow-ups worth queueing.
- If every slice in the milestone is now Done, mention that to the user —
  but do not change the milestone's Status.
- If you cannot complete the slice: leave `Status` as `Claimed` and append a
  note to the slice page explaining exactly what's blocking, then tell the
  user.

## Guardrails

- One slice per session. Never claim more than one.
- Never modify other slices, milestones, or the project page.
- Never touch slices already Claimed or Done by anyone.
- Never change milestone Status.
- Never merge PRs or push to main.
