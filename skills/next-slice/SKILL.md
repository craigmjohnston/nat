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
the working directory to use, the slice's own body, a digest of the other
slices in its milestone, and the project's conventions.

- If the user asked for a particular slice, run
  `nat start-slice <URL|ID> --project <project>` instead — same claim, same
  brief, for the slice you name.
- If the command refuses — every milestone finished, or nothing unclaimed under
  the ones that are not — report what it said and stop. The plan is the user's:
  suggest they add to it on the board (`nat`) and rerun.

Tell the user which slice you claimed, with its URL (or its ID, where it has
no URL).

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

## 3. Before you write code

The brief already carries the slice's own body, the project's conventions and
a digest of its milestone — every sibling slice's status, and the hand-back
summary of each Done one, which is often where a design decision that binds
this one already got settled.

If this slice turns on an architecture question that neither the brief nor a
Done slice in the milestone actually settles, do not guess and start writing:
raise it with the user right away, before code — not at hand-back, once an
hour or more of the wrong shape is already sunk. And never run
`complete-slice` on work whose architecture is still unsettled that way; a
hand-back is a claim that the shape is right, not a place to flag that it
might not be.

## 4. Do the work

- The brief is what the command printed: the slice's body first, then the
  project conventions. Read `CLAUDE.md` in the working directory too. The body
  may end in `Note` sections earlier sessions left for whoever worked the slice
  next: they are part of the brief.
- Honour the brief's acceptance criteria and the project's verification gate
  before calling anything done.
- While you iterate, run only the tests for what you are touching — one
  package, `go test -run <Name>`, `swift test --filter <Name>` — never the full
  suite or the coverage gate mid-loop. Run the full verification gate once,
  immediately before you hand back; if it fails, fix it with targeted runs and
  run the gate once more. Batch a stage's edits and build once per batch, not
  once per edit.
- **If the work is code**: the worktree is already on the slice's branch, so
  keep the change to exactly ONE branch's worth of work, commit there, and
  push the branch — do not create a branch of your own and do not switch to
  another. (Where you fell back to branching in place, that one branch is
  yours in the same way.) Do not run `gh` and do not open a pull request — you
  hand the branch back, and the user opens the pull request from the board once
  they have reviewed it.
- **If the work is not code** (docs, research, written-up findings): produce
  the deliverable the brief asks for and link it in the summary below.

The project's checks run on the pull request, once the slice is approved. If
you are told they failed, read how they stand — each check, and each failed
step's log — with:

```
nat slice-checks <slice> --log --project <project>
```

That is the one way to read CI: never `gh`.

If this session finds out something a *later* slice needs to know — a
constraint, a seam that moved, an assumption in another slice's brief that is
no longer true — leave a note on that slice, named by its name, from your own:

```
nat slice-note '<slice name>' --from <slice> --project <project> \
    --note '<what it needs to know, and why>'
```

Add `--milestone '<milestone name>'` where that name is filed under more than
one milestone, and `--note -` to pipe a long note in. The note ends that
slice's brief, with where it came from written by nat, so whoever works it next
reads it as part of the brief. A note is never work to be done — that is a
follow-up, not a note — and never goes on a Done slice.

## Naming slices

Refer to another slice only by its name, adding its milestone's name where the
name alone is ambiguous — never by a number, an index, a position in a list, a
page ID, a URL, or any id of another tracker (a card number, an issue key).
Names are what every reading of the plan shows; the rest is the tracker's own
or a plugin's, which the next reader may not have. This holds for everything
you write: summaries, PR descriptions, follow-up briefs, notes, proposal
briefs.

## 5. Finish

Work you noticed but did not do — a bug beside your change, a test gap in code
you didn't touch, a refactor the brief didn't ask for — is not yours to do and
not yours to lose. When the gate is green, before `complete-slice`, hand each
one in and **stop**:

```
nat slice-followups <slice> --project <project> \
    --follow-up '<title line>

<the change: which file or function, what it does instead, and why>
Done when: <how anyone checks it is finished>'
```

Write each one as a slice brief: if the user queues it, this text is the brief
of a new slice, word for word, read by an agent with nothing else. The title is
an imperative action ("Make the sidebar's post-write refresh read the
replica"), not a symptom. The body is the change — which file or function, what
it does instead, and why — then a line starting `Done when:` saying how anyone
checks it is finished. Write a decision, not a question: where there is a
choice, pick one and name the alternative rejected; no "could", "might",
"consider" or "worth looking at". If saying what to change needs a look at the
code, take that look now — it is usually one read; if it genuinely needs
investigation, the investigation is the deliverable and `Done when:` says what
it produces.

`--follow-up` repeats, one per follow-up. The user decides on the board — queue
it as a slice, fold it into this one, or drop it — and the decision arrives here
as a message naming what to fold in. Do that, then hand back as below.
`complete-slice` refuses while the decision is outstanding. Never widen your
branch to include a follow-up on your own, and never write them into the
summary or the brief instead. No follow-ups: hand back straight away.

If what you changed is visible — a pane, a page, a rendered component — and the
project already has a cheap or usual way to render it (a gallery story, a
screenshot script, a storybook), render the result and hand the images in
before `complete-slice`:

```
nat slice-visuals <slice> --project <project> \
    --visual '<what it shows, one line>
<absolute path to the image>'
```

`--visual` repeats, one per image. Hand in the full set each time: a later
hand-in replaces an earlier one. Do not build a way to render when the project
has none — hand back without images instead. The user reviews them in the app;
their comments, if any, arrive here as a message.

Record the outcome with the slice's page ID or URL, as printed in the brief:

```
nat complete-slice <slice> --project <project> --branch <branch> \
    --summary '- <what changed>
- <key decision>' \
    --pr-description '<title line>

<what the PR does and why>'
```

That records the branch you pushed and hands the slice back for review, writing
the summary onto the slice page. `--summary` is quoted back to a future agent
in its milestone's digest, not read by a person, so keep it a handful of terse
bullet points — what changed and key decisions — never a narrative of the
session. It leaves the slice in progress deliberately — approving it on the
board is what opens the pull request, and the merge of that pull request is
what marks the slice Done.

`--pr-description` is what that pull request is opened with — its first line
becomes the title and the rest the body — so write it ready to publish: what
the change does and why, addressed to whoever reviews it on GitHub, not a
report of your session. It is filed on the slice page under its own heading, so
the user can approve the branch days later and still get it. Pass
`--pr-description -` to read it from stdin when it is too long for an argument,
and give `--summary` as a flag then, since stdin is taken.

Handing the same slice back a second time, leave `--pr-description` off where
the one already filed still describes the change: the last one filed is what
the pull request opens with, so restating it unchanged only sends it again.
Where the change has moved, amend the one you filed and pass it whole — it
replaces the earlier one, it is not added to it.

Make no unverifiable claims in either one: say only what you actually checked,
never what you assume or expect to be true. A summary that says a value was
"unchanged" or "nothing invented" when you interpolated it rather than read it
is exactly the kind of line that costs somebody else an hour redoing the work
to find out it was wrong.

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
- Never touch other slices, milestones, or the project page, beyond a note on
  a later slice's brief.
- One branch per slice, and never push to main.
- Never open or merge a pull request. Opening one is the board's job, after the
  user has reviewed the branch you handed back.
- Never run `tmux kill-server`, and never kill, detach or send keys to a tmux
  session you did not create: when nat launched you, you are on the user's own
  server (`$TMUX` names it), beside every other agent. A tmux of your own gets
  a private socket, `tmux -L <name>` on every command — `TMUX_TMPDIR` does not
  isolate you while `$TMUX` is set.
