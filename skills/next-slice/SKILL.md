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

## The project, pinned first

Every `nat` command below requires `--project <id>`, naming the project by its
page ID, and every one you run must carry it. A command given none refuses
outright: there is no project the tracker falls back to, because the one the
user's board is on is theirs to switch while you work and a claim or hand-back
that landed there would land in a plan you never read.

Settle the ID once, before anything else:

- If you were launched from the board, your prompt already names it. Use that.
- Otherwise ask the CLI what this machine tracks: `nat info` with no
  `--project` refuses and lists every project the config holds, ID and name.
  Pick the one the user asked for; ask them if more than one could be it.
- Either way the brief you are about to claim repeats it, as its
  `- Project page ID:` line. Check the two agree: an ID that is not the one the
  slice came from is one you were given for a different project.

Everything below writes `<project>` where that ID goes.

## 1. Claim the next slice

Run `nat next-slice --project <project>`. It claims the next unclaimed Todo
slice under the lowest-ordered milestone that is not Done — a milestone has no
status of its own, so what is unfinished is what work is taken from — and
prints its brief: the slice's name, page ID and URL, the project's own page ID,
the working directory to use, the slice's own body, and the project's
conventions.

- If the user asked for a particular slice, run
  `nat start-slice <URL|ID> --project <project>` instead — same claim, same
  brief, for the slice you name.
- If the command refuses — every milestone finished, or nothing unclaimed under
  the ones that are not — report what it said and stop. The plan is the user's:
  suggest they add to it on the board (`nat`) and rerun.

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

Where that worktree goes is nat's convention rather than anything git decides,
and it has to be followed exactly, because a relaunch from either side finds
the worktree by arriving at the same path: a sibling `<repo>.worktrees`
directory, one entry per branch, named by the branch with every run of anything
that is not a letter, a digit, a dot, a hyphen or an underscore collapsed into
a single hyphen — so `slice/teach-next-slice` under a repository at
`/repos/nat` is `/repos/nat.worktrees/slice-teach-next-slice`. `<repo>` is the
directory holding the git directory every worktree of the repository shares,
which is what `git rev-parse --path-format=absolute --git-common-dir` names the
parent of — the common one, so a session already inside a worktree still cuts
the next one beside the repository.

First look for a worktree the branch already has, in the working directory the
brief names:

```
git worktree list --porcelain
```

One record per worktree, opened by its path and naming the branch it has
checked out as a full ref, so the path under `branch refs/heads/slice/<slug>`
is the answer. If there is one, work there: it is where the last session on
this slice left off, and its commits are exactly what a relaunch wants. Nothing
below is run in that case — a worktree that already exists is not re-cut and
not rebased.

Otherwise cut it:

```
git fetch origin
git worktree add <repo>.worktrees/<path slug> -b slice/<slug> <origin's default branch>
```

The fetch first, and the base explicitly, because otherwise git cuts the branch
from wherever the repository happens to be — whatever stale state the shared
checkout was last left in — and the work starts life behind. The base is
whatever `git symbolic-ref --short refs/remotes/origin/HEAD` names
(`origin/main`, `origin/master`); where there is no such ref, `origin/main` if
the repository has one. Git writes origin/HEAD at clone time and nothing
maintains it afterwards, so plenty of checkouts have none — and falling back to
the local `main` there would put you back on whatever the checkout last pulled,
which is the thing the fetch was for. Only a repository with no origin at all
falls back to `main`, where the local branch is all there is. A fetch that
fails is not a reason to stop: work against the refs as last fetched.

If the branch already exists but has no worktree — a slice whose branch was
pushed and merged, since a squash merge leaves the branch behind — check it out
instead of cutting it again, and do not consult the base at all:
`git worktree add <repo>.worktrees/<path slug> slice/<slug>`.

Work in the worktree's directory from here on, explicitly (absolute paths /
`git -C`) if it is not where this session started.

If git is not installed, or the working directory is not a git repository,
branch in place instead: those are the launch that worked before there were
worktrees, and the fallback is to make one branch for the slice in the working
directory the brief names — off the same fetched base, `git fetch origin` and
then `git switch -c slice/<slug> <origin's default branch>`. A git that ran and
refused is different — something is wrong with the repository — so report what
it said and stop rather than working half-placed.

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
nat complete-slice <slice> --project <project> --branch <branch> \
    --summary '<what you did>' \
    --pr-description '<title line>

<what the PR does and why>'
```

That records the branch you pushed and hands the slice back for review, writing
the summary onto the slice page: what you did, key decisions, follow-ups worth
queueing. It leaves the slice in progress deliberately — approving it on the
board is what opens the pull request and marks it Done.

`--pr-description` is what that pull request is opened with — its first line
becomes the title and the rest the body — so write it ready to publish: what
the change does and why, addressed to whoever reviews it on GitHub, not a
report of your session. It is filed on the slice page under its own heading, so
the user can approve the branch days later and still get it. Pass
`--pr-description -` to read it from stdin when it is too long for an argument,
and give `--summary` as a flag then, since stdin is taken.

Leave `--branch` off when there was no branch — a docs or research slice — and
the slice is marked Done there and then, with no pull request to describe. Pipe
the summary in on stdin when it is too long for an argument.

If you cannot complete the slice, leave it claimed and say what stopped you:

```
nat complete-slice <slice> --project <project> --blocked \
    --summary '<what is blocking>'
```

Then tell the user. If every slice in the milestone is now Done, mention that
too — a milestone's status follows its slices, and there is nothing to set.

## Guardrails

- One slice per session. Never claim more than one.
- Every write to the tracker goes through `nat`. Never edit Notion directly,
  and never work a slice the CLI would not hand you.
- Every `nat` command carries `--project <project>`, the ID you settled at the
  start — the one read that finds it is the only exception.
- Never touch other slices, milestones, or the project page.
- One branch per slice, and never push to main.
- Never open or merge a pull request. Opening one is the board's job, after the
  user has reviewed the branch you handed back.
