# Task sources — design and protocol v1

Status: **agreed with Craig 2026-10-03**, implemented on branch
`task-source-plugins`. This file is both the feature design and the
canonical **plugin protocol, version 1**: a plugin (the Shortcut one first)
is written against the *Wire contract* section and nothing else. The mock
it was designed from sits beside it — see *The mock*. A working reference
plugin is `examples/nat-source-demo/`.

## Problem

nat's tasks come from one place: a project's plan, in Notion or a local
SQLite file, shaped project → milestone → slice. At work, though, the work
is tracked in Shortcut, as stories on a board. Today the only bridge is by
hand — a gnat project per piece of work, with a token Shortcut card as a
signpost — so the card's story, comments and links live in one app and the
agent work in another, and nothing tells Shortcut what the agents did.

Craig wants Shortcut work visible and workable in gnat beside nat's own
plans, as a second **task source** — and wants it as an *optionally
installed plugin*, so the home install never carries Shortcut code, a
Shortcut token, or a Shortcut-shaped corner in the UI.

## The shape of it

**nat owns the tasks; the plugin owns the containers.** A *source project*
is an ordinary `Local` SQLite plan whose milestones are the plugin's items —
*containers* (a Shortcut story is one). A task hangs off a container
exactly as a slice hangs off a milestone, and from there goes through the
ordinary slice flow untouched: launch → worktree → agent → hand-back →
review → approve → merge. The plugin never stores, claims or completes a
task. It supplies:

- the **tree** the sidebar draws (groups, one level of sub-groups,
  containers, per-row menus);
- the **detail** of one container (facts, prose, comments, links);
- a few named **actions** (refresh, comment, rename a segment…);
- a reaction to **events** — told after the fact when one of its
  containers' tasks is created, claimed, released, handed back, approved,
  merged or deleted (so a Shortcut card can move to Done on the last merge).

**One top-level sidebar section per source project.** Within it the plugin
organises itself; nat imposes no Doing/Ready/Done of its own.

**Everything a source customises is data it declares**, not code that runs
inside gnat: facts, prose, comments, links, coloured badges, an icon, an
external URL, a handful of actions and a grouping. gnat never talks to a
plugin — `nat` does, and gnat reaches it only through `NatClient`, as for
everything else. Agents never see the plugin; they reach the container's
story through their prompt.

### What the Shortcut mock asks for

The hierarchy is not project → milestone → slice, but:

```
source (Shortcut)                 its own top-level sidebar section
└─ group                          Doing · Ready · Done   (Done: count only, loads on expand)
   └─ (Ready only) segment        a saved filter: Mine / Board / Bugs — Shortcut's own business
      └─ card                     a Shortcut story: the container a task hangs off
         └─ task                  an ordinary nat slice, parented to a card instead of a milestone
```

A card shows its id (`sc-4821`), title, Shortcut project (code + colour
tag), workflow state, type, estimate, epic, labels, owner, requester,
created, updated and iteration; its story body; comments (by, when, text)
with a composer; links (PRs with open/merged state, external docs); and
its tasks, `n/m done`.

Where the source customises display:

1. **Sidebar container row** — icon, title, hover meta (the estimate),
   coloured project tag, `+` for a new task; tasks nest beneath. Group
   headers carry counts and a menu (rename / duplicate / remove segment,
   refresh, new segment).
2. **Active rows** for a source's tasks carry the source tag (`SC`).
3. **Container navigator** — titlebar source icon + `SC` + title; a
   *Story* section (facts list, action **New task**) and a *Links* section.
4. **Container main pane** — the story body and a Comments thread with a
   composer; titlebar **Open in Shortcut**.
5. **Task navigator under a container** — Brief facts show `card`,
   `project`, `estimate` instead of `milestone`; the PR section gains
   "Linked to sc-4821. Merging moves the card to Done when it's the last
   open task."
6. **Status bar** — `<container> / <task>`.
7. **Sidebar `+` menu** — a new-project entry for the source.

## Non-goals (v1)

- **Plugin-owned tasks.** Tasks are always nat slices in a local plan; a
  plugin cannot supply, store or mutate one.
- **Moving a task between containers** (`slice-move`, a cross-container
  reorder). Refused on a source project, since there is no event to tell
  the plugin.
- **A `moved` event**, for the same reason. Added with the move itself.
- **TUI container detail.** The TUI board draws containers as milestones
  and nothing more; facts, prose, comments and links are gnat's.
- **nat storing plugin settings or credentials.** A plugin keeps its own,
  keyed by project id if it wants per-project settings.
- **Filtering done by nat.** A segment is the plugin's own saved filter;
  nat draws whatever groups it is given.

## Wire contract (protocol v1)

### Discovery

A plugin named `<name>` is an executable called `nat-source-<name>`. nat
looks for it, in order:

1. `<config dir>/plugins/<name>/nat-source-<name>`, where `<config dir>` is
   `$XDG_CONFIG_HOME/notion-agent-tracker`, else
   `~/.config/notion-agent-tracker`;
2. `nat-source-<name>` on `PATH`.

The plugins directory wins over `PATH`. The file must be executable; one
that isn't is not a plugin, and is not discovered (nor listed by
`source-list`).
`<name>` is lower-case letters, digits and `-`. A symlink is fine — the
installed path may point into a checkout.

### Invocation

```
nat-source-<name> <method>
```

- **stdin**: exactly one JSON object, the request. nat closes stdin after
  writing it.
- **stdout**: exactly one JSON object, the response (surrounding whitespace
  is fine; nothing else). At most 4 MiB.
- **exit 0** on success. Any **non-zero exit** is an error; the **first
  non-empty line of stderr** is the message nat shows (the rest of stderr
  is discarded). With no stderr, nat names the exit code. Exit 0 with
  stdout that isn't one JSON object is an error too ("returned malformed
  JSON").
- **Unknown JSON fields are ignored on both sides** — nat ignores fields a
  plugin adds, and a plugin must ignore request fields it doesn't know. A
  missing optional field and a `null` one mean the same thing.
- **Timeouts**: 20 s per call, 10 s for `event`. On a timeout nat kills the
  process and treats the call as failed.
- The process inherits nat's environment. Its working directory is
  unspecified — use `project.working_dir` where a directory matters.
- **Never print a secret to stderr.** nat logs the first stderr line (and
  only that: request and response bodies are never logged — only the
  method, ids and exit code).

Example error, a plugin with no token:

```
$ echo '{"project":{"id":"…","name":"Work","working_dir":"/Users/craig/work/app"}}' | nat-source-shortcut sidebar
Shortcut token missing — run nat-source-shortcut login        ← stderr
$ echo $?
1
```

nat shows it as `nat-source-shortcut sidebar: Shortcut token missing — run
nat-source-shortcut login` (the binary, the method, the plugin's line).

A response that decodes but breaks one of the rules below (a bad `tag`, a
malformed tree, a `choice` with no options) is refused as `nat-source-<name>
<method>: invalid response: <the rule>` — naming the rule and the ids
involved, never quoting the response.

### The envelope

Every request carries the project it is about:

```json
{ "project": { "id": "5f0c2b7e-8a41-4d3e-9a51-3c1d2e7f9b10", "name": "Work", "working_dir": "/Users/craig/work/app" } }
```

`id` is nat's project id (page-ID-shaped, stable for the project's life);
`name` and `working_dir` are as configured and may change. nat stores no
plugin settings: a plugin that wants per-project settings (a Shortcut team,
a workspace) keeps them itself, keyed by `project.id`, and asks for them
however it likes (its own `login`/`setup` subcommand, a file, a `text`
action). A plugin with nothing configured for a project should fail with a
stderr line saying how to configure it.

The method-specific fields below are added to this object.

### Shared types

```
Group     { id, label, count?, lazy?, menu?: [Action], children?: [Group], containers?: [Container] }
Container { id, title, external_url?, badges?: [Badge], meta?, menu?: [Action] }
Badge     { text, color, title? }
Fact      { label, value, color? }
Section   { id, title, kind: "prose" | "comments" | "links", body?, comments?: [Comment],
            links?: [Link], composer?: Action }
Comment   { by, when, text }
Link      { label, text, state?, url }
Action    { id, label, input: "none" | "text" | "choice", options?: [string], destructive? }
```

- **Ids** are non-empty strings, opaque to nat. Ids beginning with `_` are
  reserved for nat (`_unlisted`). **Container ids must be stable forever**:
  nat stores them in the plan as the parent of every task filed under one.
  Group ids must be stable across calls too — gnat remembers which groups
  are folded, and `expand` names them — and unique across the whole tree.
- **Colours** (`Badge.color`, `Fact.color`) are `#rrggbb`. gnat draws one
  it can't parse in secondary ink.
- **Text** is plain unless stated: only `Section.body` is Markdown.
- **`Action.input`**: `none` runs on click; `text` asks for a line of text
  (a small sheet); `choice` offers `options` (a submenu) and is invalid
  without them — nat refuses a `describe`, `sidebar` or `container`
  response carrying one, on any menu or composer. `destructive` actions
  draw in red and confirm first.

### `describe`

Who the plugin is. Called by `source-list`, on `project-create --source`,
and on every `info`.

Request: the envelope alone.

```json
{ "project": { "id": "5f0c2b7e-8a41-4d3e-9a51-3c1d2e7f9b10", "name": "Work", "working_dir": "/Users/craig/work/app" } }
```

Response:

```json
{
  "protocol": 1,
  "name": "shortcut",
  "title": "Shortcut",
  "tag": "SC",
  "icon_symbol": "rectangle.stack",
  "icon_svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 16 16\"><path fill=\"currentColor\" d=\"M2 3h12v3H2zM2 8h12v5H2z\"/></svg>",
  "container_noun": "card",
  "task_noun": "task",
  "menu": [
    { "id": "refresh", "label": "Refresh", "input": "none" },
    { "id": "new-segment", "label": "New Segment…", "input": "text" }
  ]
}
```

- `protocol` must be exactly **1**. nat refuses a plugin that says
  otherwise, naming it and both versions: `source plugin shortcut speaks
  protocol 2; this nat speaks protocol 1`.
- `name` should equal the `<name>` it was discovered by.
- `title` is the human name ("Open in Shortcut", "New Shortcut project…").
- `tag` is 1–3 upper-case letters or digits, drawn on Active rows of the
  source's tasks and in the container titlebar. A `describe` with a bad tag
  is refused.
- `icon_symbol` is an SF Symbol name, the fallback icon.
- `icon_svg` (optional) is full `<svg>` markup, at most 8 KB, single
  colour, drawn as a template image (paint with `currentColor`). Over the
  limit or unparseable, it is ignored and `icon_symbol` is drawn.
- `container_noun` / `task_noun` are singular, lower case ("card",
  "task"); gnat and the agent prompt use them in place of "milestone" /
  "slice" (`## The card`, "Other cards", "New task").
- `menu` (optional) is the source section header's menu; its actions are
  run with `target: {}`.

### `sidebar`

The tree the source's sidebar section draws.

Request adds `expand`, the ids of `lazy` groups the user has opened
(always present, possibly empty; treat a missing one as empty):

```json
{ "project": { "id": "5f0c2b7e-8a41-4d3e-9a51-3c1d2e7f9b10", "name": "Work", "working_dir": "/Users/craig/work/app" },
  "expand": ["done"] }
```

Response:

```json
{
  "groups": [
    { "id": "doing", "label": "Doing", "count": 2,
      "containers": [
        { "id": "4821", "title": "Improve diff review ergonomics",
          "external_url": "https://app.shortcut.com/acme/story/4821",
          "badges": [ { "text": "NA", "color": "#4f6bd8", "title": "Native App" } ],
          "meta": "3 pts" },
        { "id": "4790", "title": "Board mouse support",
          "external_url": "https://app.shortcut.com/acme/story/4790",
          "badges": [ { "text": "BD", "color": "#2a9d8f", "title": "Board" } ],
          "meta": "2 pts" }
      ] },
    { "id": "ready", "label": "Ready", "count": 3,
      "children": [
        { "id": "seg-mine", "label": "Mine", "count": 1,
          "menu": [
            { "id": "rename-segment", "label": "Rename…", "input": "text" },
            { "id": "segment-owner", "label": "Owner", "input": "choice", "options": ["any", "me", "unassigned"] },
            { "id": "remove-segment", "label": "Remove Segment", "input": "none", "destructive": true }
          ],
          "containers": [
            { "id": "4802", "title": "Kanban column view",
              "badges": [ { "text": "BD", "color": "#2a9d8f", "title": "Board" } ], "meta": "3 pts" }
          ] },
        { "id": "seg-bugs", "label": "Bugs", "count": 1,
          "containers": [
            { "id": "4811", "title": "Wheel scrolling in the Active panel",
              "badges": [ { "text": "BD", "color": "#2a9d8f", "title": "Board" } ], "meta": "1 pt" }
          ] }
      ] },
    { "id": "done", "label": "Done", "count": 36, "lazy": true,
      "containers": [
        { "id": "4756", "title": "Pluggable plan storage",
          "badges": [ { "text": "NA", "color": "#4f6bd8", "title": "Native App" } ], "meta": "5 pts" }
      ] }
  ]
}
```

- Groups draw in the order given. A group has **either** `children` **or**
  `containers`, never both, and `children` nest **one level** at most (a
  child group has `containers` only). nat refuses a response that breaks
  either rule, or that repeats a group id, or gives a group or container an
  empty or `_`-prefixed id.
- `count` is the plugin's own number, drawn as-is beside the label; nat
  never computes it. It may differ from the containers listed (a `lazy`
  group, a paged one).
- A **`lazy`** group omits `containers` unless its id is in `expand`, but
  always gives `count`. gnat draws it folded, and opening it re-reads
  `sidebar` with its id added to `expand`.
- A container may appear in more than one group (segments are filters over
  the same cards). It is one container wherever it appears: one selection,
  the same tasks beneath each row.
- `meta` is short trailing text (an estimate) shown on hover; `badges` draw
  after the title, in order; `external_url` backs "Open in <title>".
- The tasks under each container are nat's, not the plugin's: gnat nests
  the plan's tasks under the row by container id.

### `container`

The detail of one container, for its navigator and main pane, a task's
brief facts, and the agent prompt.

Request adds `id`:

```json
{ "project": { "id": "5f0c2b7e-8a41-4d3e-9a51-3c1d2e7f9b10", "name": "Work", "working_dir": "/Users/craig/work/app" },
  "id": "4821" }
```

Response:

```json
{
  "id": "4821",
  "title": "Improve diff review ergonomics",
  "external_url": "https://app.shortcut.com/acme/story/4821",
  "facts": [
    { "label": "id", "value": "sc-4821" },
    { "label": "project", "value": "NA · Native App", "color": "#4f6bd8" },
    { "label": "state", "value": "In Development" },
    { "label": "type", "value": "feature" },
    { "label": "estimate", "value": "3" },
    { "label": "epic", "value": "Native app parity" },
    { "label": "labels", "value": "diff, agent" },
    { "label": "owner", "value": "Craig" },
    { "label": "requester", "value": "Dana Wolfe" }
  ],
  "sections": [
    { "id": "story", "title": "Story", "kind": "prose",
      "body": "Comments left on a diff in the app never reach the working agent.\n\n**Acceptance:** a comment posted mid-session shows up in the transcript within a second." },
    { "id": "comments", "title": "Comments", "kind": "comments",
      "comments": [
        { "by": "Dana Wolfe", "when": "3d ago", "text": "Pairs with the syntax highlighting work." },
        { "by": "Craig", "when": "2d ago", "text": "Agreed. Splitting into three tasks." }
      ],
      "composer": { "id": "comment", "label": "Comment", "input": "text" } },
    { "id": "links", "title": "Links", "kind": "links",
      "links": [
        { "label": "PR #418", "text": "Diff comments reach the agent", "state": "open", "url": "https://github.com/acme/app/pull/418" },
        { "label": "Design", "text": "Figma · Review pane v3", "url": "https://www.figma.com/file/abc/review-pane-v3" }
      ] }
  ],
  "menu": [ { "id": "unassign-me", "label": "Remove Me as Owner", "input": "none" } ],
  "task_note": "Linked to sc-4821. Merging moves the card to Done when it's the last open task."
}
```

- `facts` draw in order as a label/value list (the navigator's facts card,
  and a task's Brief facts in place of `milestone`). `color` tints a
  value's leading dot.
- `sections` draw in order, one per `kind`:
  - `prose` — `body`, Markdown. The **first** `prose` section is also what
    the agent prompt quotes as the container's story.
  - `comments` — `comments` oldest first; `when` is free text ("3d ago",
    "18 Sep"). `composer`, an Action with `input: "text"`, draws as a
    compose box; sending runs it with `target: {"container": id}` and the
    text as `input`.
  - `links` — `label` (a short tag, "PR #418"), `text`, `url`; `state` is
    free text drawn as a pill ("open", "merged").
  A section of an unknown `kind` is skipped, not refused.
- `task_note` (optional) is drawn in the PR section of every task under
  this container.
- An unknown `id` is an error (exit non-zero, a stderr line).

### `action`

Run one of the plugin's own actions.

Request adds `action` (an Action's `id`), `target` and, for `text` and
`choice` actions, `input`:

```json
{ "project": { "id": "5f0c2b7e-8a41-4d3e-9a51-3c1d2e7f9b10", "name": "Work", "working_dir": "/Users/craig/work/app" },
  "action": "segment-owner",
  "target": { "group": "seg-mine" },
  "input": "unassigned" }
```

Response:

```json
{ "message": "Mine now shows unassigned cards" }
```

- `target` says whose menu the action came from: `{}` the source header
  (`describe`'s `menu`), `{"group": id}` a group's, `{"container": id}` a
  container's menu or a section composer.
- `input`: absent for `none`; the text for `text`; exactly one of
  `options` for `choice`.
- `message` (optional) is shown to the user (a toast in gnat; printed by
  `nat source-action`).
- **nat re-reads `sidebar` after every action**, successful or not, and
  gnat re-reads the open container's detail — an action never needs to
  return the new state.
- A failed action is an error like any other: non-zero exit, one stderr
  line, which the user sees.

### `event`

Told after nat has written a change to one of the container's tasks.

Request adds `container`, `task` and `event`:

```json
{ "project": { "id": "5f0c2b7e-8a41-4d3e-9a51-3c1d2e7f9b10", "name": "Work", "working_dir": "/Users/craig/work/app" },
  "container": "4821",
  "task": { "id": "1a2b3c4d-5e6f-4a1b-8c2d-3e4f5a6b7c8d", "title": "Diff comments reach the agent",
            "status": "Done", "branch": "slice/diff-comments-reach-the-agent",
            "pr": "https://github.com/acme/app/pull/418" },
  "event": "merged" }
```

Response:

```json
{}
```

| `event` | fired after | `task.status` |
|---|---|---|
| `created` | a task is filed under the container | `Todo` |
| `claimed` | a task is claimed (launch, `start-slice`) | `In progress` |
| `released` | a task is released back | `Todo` |
| `handed_back` | an agent hands back **with a branch** | `In progress` |
| `approved` | the PR is opened and recorded | `In progress` |
| `merged` | the PR is merged; the task is Done | `Done` |
| `deleted` | a task is deleted (fields as they were before) | as it was |

- `task.status` is `Todo`, `In progress` or `Done`; `branch` and `pr` are
  `""` when there is none (`pr` is the PR's URL).
- **Fire-and-forget.** nat sends the event after its own write has
  succeeded, logs a failure, and never fails or undoes the write that
  caused it. Nothing is retried.
- A plugin **must tolerate duplicates and out-of-order delivery** (two
  `nat` processes, a retried command), and must ignore an `event` name it
  doesn't know — a later protocol may add some.
- To act on "the last open task merged", a plugin counts by itself, from
  the events it has seen or from the remote; nat sends no task list.

## The `nat` contract

All project-scoped commands take `--project <id>` as everywhere else.

### New: `source-list`

```
nat source-list [--json]
```

Every discovered plugin, each `describe`d best-effort. A plugin that won't
describe (fails, times out, wrong protocol, an invalid response) is listed
with its error, never dropped. The plain form prints one line per plugin:
name, path, then `title (tag)` or `error: …`.

```json
[
  { "name": "demo", "path": "/Users/craig/.config/notion-agent-tracker/plugins/demo/nat-source-demo",
    "describe": { "protocol": 1, "name": "demo", "title": "Demo source", "tag": "DM", "icon_symbol": "rectangle.on.rectangle.angled", "container_noun": "card", "task_noun": "task" } },
  { "name": "shortcut", "path": "/opt/homebrew/bin/nat-source-shortcut",
    "error": "source plugin shortcut speaks protocol 2; this nat speaks protocol 1" }
]
```

`describe` here is sent with an empty envelope project (`id`, `name`,
`working_dir` all `""`), since no project is in question; a plugin must
answer `describe` without one.

### New: `container-show`

```
nat container-show <container id> --project <id> --json
```

`{ "container": <the plugin's container response, as-is>, "tasks": [ … ] }`
— `tasks` are the plan's slices under that container, each in the same
shape `info --json` gives a slice. Refused on a project that isn't a
source project. A failed plugin read is the command's error. Without
`--json` it prints the title, the external URL, the facts and the tasks.

### New: `source-action`

```
nat source-action --project <id> --action <action id> [--group <id> | --container <id>] [--input <text> | --input -] [--json]
```

Runs `action` with the target built from `--group`/`--container` (neither:
the source header). `--input -` reads the input from stdin (multi-line
comments); the input is trimmed of surrounding whitespace either way.
Prints the plugin's `message` (`Ran <action id>.` when it gave none);
`--json` prints `{ "message": "…" }`. Refused on a non-source project, with
no `--action`, and with both `--group` and `--container`. A board running
on the machine is nudged after a successful action.

### `project-create --source`

```
nat project-create --name <name> --source <plugin name> [--plan-dir <dir>] …
```

In order: the plugin must be discovered and `describe` with protocol 1
(refused otherwise, before any write); then the plan file is written; then
the config entry (backend `source`, `source: <plugin name>`, `plan_dir`
as for a local project). Mutually exclusive with `--local`. Needs no
Notion token. `--json` prints the project as `--local` does, with
`"backend": "source"` and `"source": "<plugin name>"`.

### `info --json`

A source project's info gains `source`:

```json
"source": {
  "name": "shortcut", "title": "Shortcut", "tag": "SC",
  "icon_symbol": "rectangle.stack", "icon_svg": "<svg …>",
  "container_noun": "card", "task_noun": "task",
  "menu": [ { "id": "refresh", "label": "Refresh", "input": "none" } ],
  "groups": [ …the plugin's groups…,
    { "id": "_unlisted", "label": "Other cards", "count": 1,
      "containers": [ { "id": "4611", "title": "Search result ranking" } ] } ],
  "error": ""
}
```

- `--expand <group id>` (repeatable) is passed through as `sidebar`'s
  `expand`.
- **`_unlisted`** is appended by nat, labelled "Other <container_noun>s",
  holding every container that has tasks in the plan but appears nowhere in
  the plugin's tree (a card that scrolled into a lazy group, or out of a
  filter), titled from nat's cached title. Omitted when empty. A task never
  vanishes from the sidebar because its container did.
- **A failed plugin read concludes nothing.** If `describe` or `sidebar`
  fails, `info` still succeeds: `error` carries the message, the fields it
  couldn't read are empty (`name` is the configured plugin name either
  way, the nouns default to `container` / `task`), and `groups` is
  `_unlisted` alone — so every container with tasks is still drawn, from
  the cache. A failed `describe` is not followed by a `sidebar` call. A
  plugin that can't be found at all reads the same way: the project still
  opens, over a client that fails every call.
- `menu` and `groups` are always arrays; `error` is always present (`""`
  when nothing failed).
- Containers otherwise appear in `milestones` exactly as milestones do
  (their `id` is the container id), so every existing reader keeps working.

### `slice-show --json`

A task in a source project gains `container`:

```json
"container": {
  "id": "4821", "title": "Improve diff review ergonomics",
  "external_url": "https://app.shortcut.com/acme/story/4821",
  "task_note": "Linked to sc-4821. Merging moves the card to Done when it's the last open task.",
  "facts": [ { "label": "id", "value": "sc-4821" }, … ]
}
```

On a failed `container` read, only `id` and the cached `title` are filled.
Absent for a non-source project.

### `slice-add --container`

```
nat slice-add <title> --container <container id> [--description TEXT|-] … --project <id>
```

Source projects only; there `--milestone` is refused, `--container` is
required, and `--container` is refused anywhere else. A container already
in the plan is filed under directly; a new one is read with `container`
first for its title (a blank title falls back to the id), and the add
refused if that read fails (nat won't file under a container it can't
name). Fires `created`.

### `config-show`

Each project's entry carries `source` (the plugin name) where its backend
is `source` (`source=<name>` in the plain form).

### Refusals and fixes

- `project-mirror` refuses a source project by name: its tasks hang off the
  plugin's containers, which Notion has nowhere to keep.
- `done-clear` refuses a source project by name: each Done task deleted
  would tell the plugin `deleted` of work that merged, and the containers
  it would then remove are the plugin's.
- `slice-status` takes the local path for a source project.
- `milestone-add`, `milestone-rename`, `milestone-remove`,
  `milestone-move`, `slice-move`, and a `slice-reorder` across containers
  are refused on a source project, in the store's own words ("cards belong
  to Shortcut; nat can't …"). `plan-apply` reaches the same refusal for
  any milestone it would create.
- The TUI's `n` (add slice) is refused with a toast on a source project;
  tasks are added from gnat or `nat slice-add --container`.

## Data model

- **Config.** `ProjectConfig.source` (omitted when empty) and backend
  `source`. Like `local`, a source project needs no Notion token, and
  `Config.AssigneeFor` names who works its tasks. A backend word this build
  doesn't know still reads as Notion.
- **Local schema v5.** `ALTER TABLE milestones ADD COLUMN container_id
  TEXT` with a partial unique index (`WHERE container_id IS NOT NULL`). A
  v4 plan migrates silently and every existing milestone keeps
  `container_id` NULL, so nothing about a non-source plan changes.
- **Containers are milestones.** A milestone row with a `container_id` is
  a container: `milestones()` gives it `ID = container_id` (else `Name`,
  as today), and `checkMilestone` matches `name = ? OR container_id = ?`.
  The row's `name` is the container's title as first filed — nat's
  **cached title**, used by `_unlisted`, the TUI and failed reads.
- **`ensureMilestone(id, title)`** is the only way a container row is
  made: idempotent on `container_id`; on a `name` collision with another
  container (two cards titled alike) it inserts as `title (id)`.
- **`store.Sourced`** wraps the project's `*Local` and the plugin client
  and is the `Store` for a source project. It delegates everything to the
  local plan except: `AddSlice` (ensure the container, add, fire
  `created`); `ClaimSlice`, `ReleaseSlice`, `CompleteSlice`, `RecordPR`,
  `MarkDone`, `DeleteSlice` (write, then fire the event — **after** the
  write, logged and never fatal, as `Mirrored.push`); and the refusals
  above. `CompleteSlice` fires `handed_back` only when it records a branch;
  `--blocked` and plain endings are not source events. Narrow interfaces
  (`ContainerReader`, `SidebarReader`, `ActionRunner`, `Describer`) are
  answered only by `*Sourced`, so a caller asks with a type assertion.
- **The agent prompt** of a task under a container gains a `## The
  <container_noun>` block (title, external URL, the first prose section),
  filled by `actions.Launch` through `ContainerReader`; a failed read
  degrades to no block. The prompt's own `Slice ID` / `Slice URL` lines
  are unchanged in meaning.

## gnat

- **Sidebar.** Projects with a `source` are pulled out of the project list
  into **one top-level fold each**: plugin icon (`icon_svg`, else
  `icon_symbol`), the project's name, the header `menu`. Inside, group
  headers with `count` and their `menu` (actions → `source-action`; `text`
  through a small sheet, `choice` through a submenu, `destructive`
  confirmed), one level of sub-groups, then container rows (title,
  `badges`, `meta` on hover, `menu`, `+` → the New Task sheet with the
  container preset) with the plan's tasks at depth 2 beneath. A `lazy`
  group opens by re-reading `info --expand`. Active rows of a source's
  tasks carry its `tag`.
- **`+` menu.** One "New <title> project…" per discovered plugin
  (`source-list`), opening a small sheet (name, working dir) →
  `project-create --source`.
- **Container selected.** A third selection kind beside slices and
  sessions, mutually exclusive with both. Navigator: titlebar icon + tag +
  title; a facts card with **New task**; one section per `sections` entry
  by kind (prose, comments with composer, links). Main pane: the prose and
  comments; titlebar **Open in <title>** (`external_url`).
- **Task under a container.** Brief facts show the container's `facts`
  instead of the milestone; the PR section appends `task_note`; the status
  bar crumb is `<container title> / <task>`.
- **Settings.** A read-only **Sources** tab: each discovered plugin's
  name, title, tag, icon and path, or its error.
- **Gallery stories**: `sidebar-source`, `window-container`,
  `window-container-comments`, `window-source-task-brief`,
  `window-source-task-pr`, `new-source-project`, `settings-sources`.

## Edge cases

- **Plugin missing or broken** (uninstalled, token expired, wrong
  protocol): the project still opens. `info` carries `source.error` and
  draws every container that has tasks under `_unlisted`, from the cache;
  tasks launch, hand back, merge as usual. Events fail and are logged.
  Container detail shows the error.
- **A container disappears from the tree** (filtered out, moved to a lazy
  group, deleted in Shortcut): its tasks stay, under `_unlisted`.
- **A container renamed in the remote**: the sidebar shows the plugin's
  title; the cached title (TUI, `_unlisted`) keeps the old one in v1.
- **Two containers with one title**: the second is cached as `title (id)`.
- **A task moved between containers**: impossible in v1 (refused).
- **The same container in two groups**: one container; selecting either
  row selects it, and both rows nest the same tasks.
- **Duplicate or out-of-order events**: the plugin's problem, by contract.
  A `released` can arrive after a later `claimed`'s write if two `nat`
  processes race; the plugin should read the remote rather than trust
  order where it matters.
- **Relaunch, rework, reopen** (`RecordRelaunch`, `ClearBranch`,
  `ReopenSlice`) fire no event in v1.
- **Project id changes**: never; a plugin may key settings on it.
- **Secrets**: the plugin owns them; nat never sees, stores or logs one,
  and logs only a failing plugin's first stderr line.

## Testing

- **`internal/source`**: `Exec` against a fake `StdinRunner` — the exact
  request JSON per method, response decoding, unknown fields ignored, the
  first-stderr-line error, exit codes, malformed JSON, the size cap,
  timeouts, the protocol check; `Discover` over a temp config dir and
  `PATH` (dir wins, non-executable skipped).
- **`internal/store`**: `Sourced` over a real `*Local` in `t.TempDir()`
  with `source.Fake` — each event fired after its write with the right
  task fields, a failing event never failing the write, `handed_back` only
  with a branch, every refusal; Local v4 → v5 migration, `container_id`
  round trip, the collision rename.
- **`internal/cli`**: each new command's statements and refusals;
  `info --json`'s `source` (including `_unlisted` and the failed-read
  shape), `slice-show`'s `container`, `slice-add --container`,
  `project-create --source`.
- **`internal/agent` / `internal/tui`**: the `## The card` block present
  and absent; one golden render of a source project; `n` refused.
- **gnat**: decoding of every new model leniently; `NatClient` arguments;
  the seven stories rendered and compared, cropped, against the mock's
  Shortcut-specific parts.
- **End to end**: `examples/nat-source-demo` installed — `source-list`,
  `project-create --source demo`, `info --expand done`, `slice-add
  --container c1`, `container-show c1`, `source-action`, and the demo's
  `events.log` showing `created` (see that README).

## Alternatives considered

- **The plugin implements the full `Store` over the wire** (all ~30
  methods). Rejected: every plugin would reimplement claiming, hand-back,
  follow-ups, the task log and the body-section markdown that both stores
  must already agree on, and every future `Store` change would break every
  plugin. Owning tasks in a local plan keeps the plugin to five methods
  about *its* data.
- **Go `plugin` package, or plugins compiled in.** The `plugin` package is
  Linux/macOS-only, needs the exact same toolchain and module versions,
  and can't be unloaded; compiling in puts Shortcut code in the home
  install, the one thing Craig asked not to happen.
- **One long-lived JSON-RPC process per plugin.** Faster, but `nat` is a
  short-lived process per command, so it would need a daemon or respawn
  per command anyway; process-per-call is simpler to write a plugin for
  (any language, no framing), to test (a shell pipe), and to reason about
  under timeouts.
- **A nat-stored settings blob per project** handed to the plugin on each
  call. Rejected: it would put a Shortcut token in nat's config, which nat
  otherwise keeps free of credentials, and give nat a schema it doesn't
  understand to carry.
- **Variant B of the mock** (one Shortcut section with a fixed Ready
  segments row) and **variant A** (each saved filter its own top-level
  section). Both superseded by one top-level section per source project
  with the plugin organising within: Shortcut can express either inside
  its own tree, and nat stays ignorant of segments.

## The mock

Export of the Claude Design project **"macOS native UI for TUI app"**
(`claude.ai/design/p/e81457f6-c2ca-40ae-8f1f-77062e2aa319`), file
`gnat Shortcut.html` and the JSX/CSS it imports. The React is a *spec to
read*, not code to run or port.

- `gnat-shortcut.html` — the canvas page; mounts the app from the files
  below.
- `gsc-data.jsx` — fixtures: Shortcut cards (with story, comments, links),
  Shortcut project codes and colours, the Done count, saved-filter sources
  (variant A) and Ready segments (variant B), filter options, and the
  slices — some under projects, some under cards.
- `gsc-shell.jsx` — icons, the sidebar (source headers, group headers with
  menus, the filters popover, card rows with tasks nested beneath, the `+`
  menu), and the status bar.
- `gsc-nav.jsx` — the navigator: a task's Brief · Thread · Changes · PR
  sections, and a card's Story · Comments · Links.
- `gsc-main.jsx` — the main pane (brief editor, card story + comments,
  terminal, diff) and the app root.
- `gsc.css` — the mock's styles.
- `ui-shortcut.jsx` — an earlier iteration (avatars, filter pills), kept
  for reference only.

### Deliberate departures

- **Variants A and B are both superseded** by one top-level section per
  source, the plugin organising within (see *Alternatives considered*).
- **The filters popover becomes a group menu** of `choice` actions (Owner,
  Epic, Label, Type); nat draws no filter UI of its own.
- **"Doing cards in Active" mode is not built.** Active stays flat task
  rows, each carrying the source tag.
- **The status-bar crumb shows the container's title**, not its short id;
  a plugin that wants the id visible puts it in a badge or a fact.
- **`ui-shortcut.jsx`** (avatars, filter pills) is kept for reference
  only.
- Changes the mock makes to existing chrome (unrelated to the source) are
  ignored.
