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
skill asks nat for the same worktree, so the branch it hands back is the one
the board would have made:

```
nat slice-worktree <slice> --project <project>
```

It prints the worktree's path on one line. Where the slice already has a
worktree — the last session on it left off there — that is the one it prints,
untouched; otherwise it cuts one, on the slice's branch, from the project's
base as it stands on origin now, exactly as a launch from the board does. The
repository is the slice's own, else the project's working directory; pass
`--repo <path>` only where the brief names neither. Cut no worktree or branch
of your own: the path and branch name are nat's convention, and a relaunch
finds the worktree only by nat arriving at the same one.

Work in that directory from here on, explicitly (absolute paths / `git -C`) if
it is not where this session started.

If it refuses, it says why in git's own words. A working directory that is no
git repository has no branch to work on: there the work is not code, so do it
where the brief says and hand back with `--no-branch`. A git that refused in a
repository is something wrong with the repository — report what it said and
stop rather than working half-placed.

## 3. Asking the user

The brief already carries the slice's own body, the project's conventions and
a digest of its milestone — every sibling slice's status, and the hand-back
summary of each Done one, which is often where a design decision that binds
this one already got settled.

Decide what you can, ask what you cannot. Ask only where different
answers would change the work materially, or before something
destructive or hard to undo; otherwise take the reading the brief and
the code best support, say so in one line, and carry on. A failing check
on your pull request, a flaky test you hit, or a loose end in code you
touched is part of this slice: fix it and say what you did, rather than
asking whether to. Do everything that does not depend on the answer
before you ask.

Raise an architecture question — a decision neither the brief nor a Done
slice in the milestone settles — before you write code, not at
hand-back, where an hour of the wrong shape is already sunk. Never run
`complete-slice` on work whose shape is still unsettled that way: a
hand-back is a claim the shape is right.

Shape every question so it can be answered without opening the code:

- The first line is the decision, in one plain sentence, and why it
  matters to the user.
- Then the options, two to four, each with what the user gets and gives
  up, in a line. Put the one you recommend first and say why. Letter or
  number them so the reply can be "1b".
- No code identifiers, file paths or flags, unless the user must choose
  between them. No term the brief or the user has not used, unless you
  say what it means in the same sentence.
- One decision per question, under about 120 words, and the question
  before any report, never buried after one.
- Never ask the user to observe what they cannot (what a run printed,
  which input device failed): find out yourself, or say exactly what to
  click and what each outcome would mean.
- Never ask what is already decided: by the brief, a design it cites,
  the project's rules, or an answer earlier in this session. Read what
  the brief cites before you choose an approach; if it names something
  that does not exist, say so in your first message, before building
  anything.

Do not end a turn on a status line ("waiting on CI", "I'll check again
in five"): wait inside the turn, or end with what you are waiting for
and when you will report.

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
  keep the change to exactly ONE branch's worth of work and commit there — do
  not create a branch of your own and do not switch to another. Do not push it
  yourself: handing back pushes it. Do not run `gh` and
  do not open a pull request — you hand the branch back, and the user opens the
  pull request from the board once they have reviewed it.
- **If the work is not code** (docs, research, written-up findings): produce
  the deliverable the brief asks for and link it in the summary below.

The project's checks run on the pull request, once the slice is approved. If
you are told they failed, read how they stand — each check, and each failed
step's log — with:

```
nat slice-checks <slice> --log --project <project>
```

That is the one way to read CI: never `gh`. It reads a check still running
too — the step it is on and how long it has been there — so look there, not at
`gh`, when one check has sat pending far longer than the rest or than it
usually takes.

When a check fails or stalls for a reason that is not your change's — a flaky
test, a runner that died, a job stuck on a step — re-run it:

```
nat slice-checks-rerun <slice> --check '<check name>' --project <project>
```

`--failed` in place of `--check` re-runs every failed job. If the check is
still running, the re-run cancels it first on its own; a cancel stops every
job of that run, and the output says which ones it stopped. Never use it to
retry a real failure without fixing it — pushing a commit re-runs CI by itself
— and never `gh` for it either.

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
reads it as part of the brief; where that slice has a live agent, the note
reaches it in its session too. A note is never work to be done — that is a
follow-up, not a note — and never goes on a Done slice.

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

## 5. Finish

Work you noticed but did not do — a bug beside your change, a test gap in code
you didn't touch, a refactor the brief didn't ask for — is not yours to do and
not yours to lose. When the gate is green, before `complete-slice`, hand each
one in and **stop**:

```
nat slice-followups <slice> --project <project> \
    --follow-up '<title line>

<the problem, as the user sees it, and what leaving it costs>
<what you recommend, and why>
<the change: what, where, instead of what>
Done when: <how anyone checks it is finished>'
```

A follow-up is work the user has not asked for that you noticed and did
not do; anything this slice's own `Done when:` covers is this slice's
work, so do it rather than file it. Write each one as a slice brief: if
the user queues it, this text is the brief of a new slice, word for
word, read by an agent with nothing else. The title is an imperative
action in plain words, eight words or fewer, naming no file or type. The
body opens with one line saying the problem as the user would see it and
what it costs to leave it, then one line saying what you recommend
(queue it, fold it in now, or drop it) and why. Then the change — what,
where, instead of what — and a line starting `Done when:` saying how
anyone checks it is finished, as something they can see or run. Write a
decision, not a question: where there is a choice, pick one and name the
alternative rejected; no "could", "might", "consider" or "worth looking
at". Before filing one, check the plan for a later slice that already
covers it, and fold housekeeping you noticed — a flaky test, lint drift,
dead code — into one item or fix it now. If saying what to change needs
a look at the code, take that look now; if it genuinely needs
investigation, the investigation is the deliverable and `Done when:`
says what it produces.

A slice title names one change in eight words or fewer, in the words of
the person who asked for it — what they get, not how it is built. No
file, type or command names; no colon, dash or "and" joining several
changes; no "N fixes in one pass". At most 64 characters. The list of
what it covers goes in the brief.

`--follow-up` repeats, one per follow-up. A later hand-in carries only what is
new — never a repeat of a follow-up already handed in. The user decides on the
board — queue it as a slice, fold it into this one, or drop it — and the
decision arrives here as a message naming what to fold in. Do that, then hand
back as below.
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

`--visual` repeats, one per image. Each hand-in adds to what is already filed:
hand in only the images that are new or re-rendered, and one under a name
already filed replaces it — there is no need to remove an image to update it.
Take out an image that no longer shows anything relevant to the change (a view
the work stopped touching, a render a differently named one has superseded) with
`--remove '<its name>'`. Where the change is best judged against what was there
before, add `--before '<the visual's name>
<absolute path to the before image>'` — rendered to its own file, never the one
the after is rendered to, since a before the after overwrote shows nothing. Do
not build a way to render when the project has none — hand back without images
instead. The user reviews them in the app; their comments, if any, arrive here
as a message.

Record the outcome with the slice's page ID or URL, as printed in the brief:

```
nat complete-slice <slice> --project <project> \
    --summary '- <what now works, in the user's words>
- <a decision you made>' \
    --pr-description '<title line>

<what the PR does and why>'
```

nat reads the branch off the slice's worktree. It refuses
while the worktree holds anything uncommitted — commit it first — then pushes
the branch itself (with a lease, so a rebased branch goes too), records it and
hands the slice back for review, writing the summary onto the slice page. A
push it reports refused is yours to sort out before handing back again. It
leaves the slice in progress deliberately — approving it on the board is what
opens the pull request, and the merge of that pull request is what marks the
slice Done.

`--summary` is shown to the user on the task log and quoted to later
agents in the milestone's digest. Its first bullet says what now works
or what changed, in the user's words, as one sentence they could read
alone. Then at most three bullets: decisions you made and anything the
reviewer must check. Identifiers only where a reviewer needs them to
find the place. Never a narrative of the session.

`--pr-description` is what the pull request is opened with: its first
line is the title and the rest the body, ready to publish. The first
paragraph says what the change does and why, in plain words, for whoever
reviews it on GitHub. Technical detail follows under its own heading. No
test counts, no list of files touched, no report of your session. It is filed on the slice page under its own heading, so
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

If the user asks for more or different work after you have handed back, put it
on the record before changing anything:

```
nat slice-resume <slice> --project <project> --note '<what they asked for>'
```

That takes the slice back out of review, so the user's board reads it as work
in progress again. Then do the work, commit, and hand back again with the same
`complete-slice` command. If it refuses because the slice is Done, the
work is merged: say so to the user and stop.

Pass `--no-branch` when there was no branch — a docs or research slice — and
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
