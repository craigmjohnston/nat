---
name: next-slice
description: Pick up and complete the next available slice of a project tracked in the Notion agent tracker — claim it, do the work per project conventions, and mark it Done. Use when the user says things like "pick up the next slice", "work on the next slice for <project>", or "grab a slice".
---

# /next-slice — pick up and complete the next slice

You are an agent working one slice of a project tracked in the Notion agent
tracker. You work exactly one slice per session — never continue to another
when this one is done.

The `nat` CLI is how you reach the tracker: it chooses the slice, claims it,
prints the brief, and records the outcome. You never write to Notion yourself.

## 1. Claim the next slice

Run `nat next-slice`. It claims the next unclaimed Todo slice under the
lowest-ordered milestone still open — Active, or for a project that keeps its
whole plan on one page and so has no milestone statuses to set, simply the
lowest-ordered milestone that is not Done — and prints its brief: the slice's
name, page ID and URL, the working directory to use, the slice's own body, and
the project's conventions.

- If the user asked for a particular slice, run `nat start-slice <URL|ID>`
  instead — same claim, same brief, for the slice you name.
- If the command refuses — no milestone open, or nothing unclaimed under the
  ones that are — report what it said and stop. Do not activate a
  milestone; suggest the user does it on the board (`nat`) and rerun.
- `nat` works on whichever project local config marks active. If the user
  asked for a different one, tell them to switch the active project on the
  board and rerun — the CLI has no project switch of its own.

Tell the user which slice you claimed, with its Notion URL.

## 2. Do the work

- The brief is what the command printed: the slice's body first, then the
  project conventions. Read `CLAUDE.md` in the working directory too.
- Work in the working directory the brief names. If that is not where this
  session started, work there explicitly (absolute paths / `git -C`).
- Honour the brief's acceptance criteria and the project's verification gate
  before calling anything done.
- **If the work is code**: create a branch for the slice, keep the change to
  exactly ONE pull request, commit, push the branch, and open the PR — do not
  merge it.
- **If the work is not code** (docs, research, written-up findings): produce
  the deliverable the brief asks for and link it in the summary below.

## 3. Finish

Record the outcome with the slice's page ID or URL, as printed in the brief:

```
nat complete-slice <slice> --pr <URL> --summary '<what you did>'
```

That marks it Done and writes the summary onto the slice page: what you did,
key decisions, follow-ups worth queueing. Leave `--pr` off when there was no
pull request; pipe the summary in on stdin when it is too long for an
argument.

If you cannot complete the slice, leave it claimed and say what stopped you:

```
nat complete-slice <slice> --blocked --summary '<what is blocking>'
```

Then tell the user. If every slice in the milestone is now Done, mention that
too — but never change a milestone's status.

## Guardrails

- One slice per session. Never claim more than one.
- Every write to the tracker goes through `nat`. Never edit Notion directly,
  and never work a slice the CLI would not hand you.
- Never touch other slices, milestones, or the project page.
- Never merge PRs or push to main.
