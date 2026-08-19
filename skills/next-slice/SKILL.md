---
name: next-slice
description: Pick up and complete the next available slice of a project tracked in the Notion agent tracker — claim it, do the work per project conventions, and hand the branch back for review. Use when the user says things like "pick up the next slice", "work on the next slice for <project>", or "grab a slice".
---

# /next-slice — pick up and complete the next slice

You are an agent working one slice of a project tracked in the Notion agent
tracker. You work exactly one slice per session — never continue to another
when this one is done.

The `nat` CLI is how you reach the tracker: it chooses the slice, claims it,
prints the brief, and records the outcome. You never write to Notion yourself.

## 1. Claim the next slice

Run `nat next-slice`. It claims the next unclaimed Todo slice under the
lowest-ordered milestone that is not Done — a milestone has no status of its
own, so what is unfinished is what work is taken from — and prints its brief:
the slice's name, page ID and URL, the working directory to use, the slice's
own body, and the project's conventions.

- If the user asked for a particular slice, run `nat start-slice <URL|ID>`
  instead — same claim, same brief, for the slice you name.
- If the command refuses — every milestone finished, or nothing unclaimed under
  the ones that are not — report what it said and stop. The plan is the user's:
  suggest they add to it on the board (`nat`) and rerun.
- `nat` works on whichever project local config marks active. If the user
  asked for a different one, tell them to switch the active project on the
  board and rerun — the CLI has no project switch of its own.

Tell the user which slice you claimed, with its Notion URL.

## 2. Work in the slice's own worktree

A slice launched from the board is given a git worktree of its own, so its
agent works on its own branch in its own directory rather than sharing the one
checkout with every other agent and with the user. A session started from this
skill cuts the same worktree for itself, so the branch it hands back is the one
the board would have made.

The branch is derived from the slice's name, exactly as the board derives it:
`slice/` followed by the name lowercased, with every run of anything that is
not an ASCII letter or digit collapsed into a single hyphen and none left at
either end. "Teach /next-slice to work in a worktree" is
`slice/teach-next-slice-to-work-in-a-worktree`.

In the working directory the brief names, run:

```
wt switch --create slice/<slug> --no-cd
```

`--no-cd` because there is no shell of yours for worktrunk to change the
directory of — the worktree it makes is somewhere else, and you work there by
path. `wt list --format json` is what names that path: switch prints for a
person, and the list is the one machine-readable answer worktrunk gives. Work
in that directory from here on, explicitly (absolute paths / `git -C`) if it is
not where this session started.

If `wt` is not installed, or the working directory is not a git repository,
branch in place instead: those are the launch that worked before there were
worktrees, and the fallback is to make one branch for the slice in the working
directory the brief names. A `wt` that ran and refused is different — something
is wrong with the repository — so report what it said and stop rather than
working half-placed.

## 3. Do the work

- The brief is what the command printed: the slice's body first, then the
  project conventions. Read `CLAUDE.md` in the working directory too.
- Honour the brief's acceptance criteria and the project's verification gate
  before calling anything done.
- **If the work is code**: the worktree is already on the slice's branch, so
  keep the change to exactly ONE branch's worth of work, commit there, and
  push the branch — do not create a branch of your own and do not switch to
  another. (Where you fell back to branching in place, that one branch is
  yours in the same way.) Do not run `gh` and do not open a pull request — you
  hand the branch back, and the user opens the pull request from the board once
  they have reviewed it.
- **If the work is not code** (docs, research, written-up findings): produce
  the deliverable the brief asks for and link it in the summary below.

## 4. Finish

Record the outcome with the slice's page ID or URL, as printed in the brief:

```
nat complete-slice <slice> --branch <branch> --summary '<what you did>'
```

That records the branch you pushed and hands the slice back for review, writing
the summary onto the slice page: what you did, key decisions, follow-ups worth
queueing. It leaves the slice in progress deliberately — approving it on the
board is what opens the pull request and marks it Done.

Leave `--branch` off when there was no branch — a docs or research slice — and
the slice is marked Done there and then. Pipe the summary in on stdin when it
is too long for an argument.

If you cannot complete the slice, leave it claimed and say what stopped you:

```
nat complete-slice <slice> --blocked --summary '<what is blocking>'
```

Then tell the user. If every slice in the milestone is now Done, mention that
too — a milestone's status follows its slices, and there is nothing to set.

## Guardrails

- One slice per session. Never claim more than one.
- Every write to the tracker goes through `nat`. Never edit Notion directly,
  and never work a slice the CLI would not hand you.
- Never touch other slices, milestones, or the project page.
- One branch per slice, and never push to main.
- Never open or merge a pull request. Opening one is the board's job, after the
  user has reviewed the branch you handed back.
