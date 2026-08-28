---
name: queue-project
description: Turn a workshopped plan into a whole new project in the Notion agent tracker — draft the project's name, its conventions and its milestones/slices, get explicit user approval, then create it with `nat project-create` and file the plan with `nat plan-apply --project`.
---

# /queue-project — start a new tracked project from a plan

You help the user turn work they have just workshopped — in this repo, in this
session — into a **new** project in their agent tracker: the project page, the
conventions its agents will read, and the milestones and slices under it.

Use this when there is no project for the work yet. If the work belongs to a
project the tracker already has, use `/queue-work` instead: it files milestones
and slices into a project already there and creates nothing else.

You draft everything in chat first. You write **only after the user explicitly
approves**, and only through the `nat` CLI.

## Setup

Two things settle before you draft:

- **The repo the plan is for.** Ordinarily the checkout this session is in —
  that is where its agents will work. Say which directory you mean, and use an
  absolute path.
- **Whether `nat` is set up on this machine at all.** `nat info` with no
  `--project` refuses and lists the projects the config holds, ID and name.
  Read that list twice over: it is how you check the work really has no home
  yet, and a machine that tracks nothing at all has never been onboarded — stop
  there and tell the user to run the board (`nat`) once, since creating a
  project needs the workspace the onboarding picked.

That setup read, and `project-create` itself, are the only `nat` commands here
that name no project: one because it is asking what the projects are, the other
because it makes one. Every command after step 3 carries `--project <id>` with
the ID `project-create` printed, and so must any other `nat` command you run
about the new project. A command given none refuses outright: there is
no project the tracker falls back to, and in particular not whichever project
the user's board is on, which is never the one you just made.

## Drafting rules

Draft three things. All of them are the user's to approve before anything is
written.

**The project name.** Short, and what the user will recognise it by on the
board's switch picker.

**The project's brief** — the conventions its agents read. It is written as the
project page's body, and every `nat info`, `nat next-slice` and `nat
start-slice` prints it after the slice's own brief, so it is the standing
instruction to every agent that ever works the project. Cover:

- the repo: the default working directory, the module or package name, and to
  read the repo's own `CLAUDE.md` before working;
- what a slice is in this project, and that a code slice is exactly one PR;
- the stack and version lines that matter;
- the verification gate — the exact command that has to pass before anything
  is called done.

Write it as markdown headings and bullets. Keep it to what an agent needs on
every slice: anything true of one slice alone belongs in that slice's brief.

**The milestones and slices**, under `/queue-work`'s rules:

- A **slice** is a small unit of work one agent completes in a single fresh
  session. If the work is code, a slice maps to **exactly one PR** — split
  anything bigger.
- Each slice gets a clear imperative title and a 2–6 sentence description
  written as a self-contained brief: what, where, acceptance criteria. The
  agent that picks it up reads the brief and the project's conventions and
  nothing else of this conversation.
- Milestones are phases of the work, in the order they should happen: the
  plan's order is the order they are written in, and `nat next-slice` hands
  work out from the lowest-ordered milestone that is not Done.
- Status, order and assignee are not yours to choose. New slices are filed
  `Todo` and unassigned; a milestone's status follows its slices, so there is
  none to set anywhere.
- `depends_on` names the slices a slice genuinely cannot start before — a
  blocked slice is one `nat next-slice` steps over and `nat start-slice`
  refuses — not the ones that merely read better in order.

## Procedure

1. Present the proposal in chat: the project name, the repo, the brief in full,
   and the plan as a compact tree of milestones with their slices (titles plus
   one-line summaries). Note anything you left out or split.
2. **Write nothing until the user explicitly approves.** Iterate on their
   feedback by revising the proposal, not by creating the project and fixing it
   afterwards. There is no `nat` command that deletes a project.
3. On approval, create the project, passing the brief on stdin:

   ```
   nat project-create 'The Project' --repo /abs/path/to/repo --description - <<'BRIEF'
   ## Repo
   ...
   BRIEF
   ```

   It creates the project page and its Slices database, writes the brief as the
   page body, and registers the project in this machine's local config so the
   board can open it. It prints the project's Notion page ID and URL — the ID
   is what the next step needs. (`--json` prints the same as `{"project":
   {"id": ...}}` if you would rather parse it.)

   `--repo` is the working directory the project's agents get. Leave it off
   only when the command is being run from inside that directory, which is
   where it defaults.

4. File the plan into the project you just created:

   ```
   nat plan-apply --project <the id project-create printed> <<'PLAN'
   {
     "milestones": [{ "name": "M1: Foundations" }],
     "slices": [
       {
         "title": "Do the thing",
         "milestone": "M1: Foundations",
         "description": "The brief, as it should read on the page.",
         "depends_on": ["A slice that has to be finished first"]
       }
     ]
   }
   PLAN
   ```

   `--project` is what makes this land in the new project, and **without it
   `plan-apply` files nothing at all** — it is refused before it
   writes anything. Pass the page ID `project-create` printed.

   `milestone` names one of the plan's own new milestones by name. `description`,
   `repo` and `depends_on` are optional; nothing else is, and any other key is
   rejected. The whole document is validated before the first page is created.

   `repo` on a slice is only for work that happens somewhere other than the
   project's own working directory — a plan for one repo needs none.

5. Report the project's Notion URL and the created page URLs grouped by
   milestone, which `plan-apply` prints. Then tell the user the last step,
   which is theirs:

   > The new project is not the active one. Open the board (`nat`) and press
   > `P` to switch to it.

   Say it plainly — the board is the only thing there is a switch for, and
   until they use it the new project is reached by `--project` alone, which is
   what every command in step 4 carried.

## Guardrails

- Everything you write goes through `nat`. Never edit Notion directly.
- Nothing is written before the user approves — not the project, not a
  milestone, not one slice.
- `project-create` creates a project every time it is run. If you are unsure
  whether you already created one, check the URL it printed rather than running
  it again — or run `nat info` bare and read the list of projects it refuses
  with, which the new one is now in.
- `plan-apply` only ever creates, and if a run fails partway it says what it
  had already created. Trim those out of the plan before running it again
  rather than filing them twice.
- Never switch the active project, and never touch the plan of the project that
  is active — this skill adds a project beside it.
