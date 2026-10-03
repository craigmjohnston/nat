# Build `nat-source-shortcut`, the Shortcut task-source plugin for nat

You are building a plugin for **nat** (`github.com/craigmjohnston/nat`, the
tracker Craig drives from the macOS app **gnat**). nat has a task-source
plugin system: an external binary `nat-source-<name>` owns the *containers*
a project's tasks hang off, and nat owns the tasks. You are writing the one
for **Shortcut** (shortcut.com), so Craig's work stories appear in gnat as
cards he can file nat tasks under and launch agents on.

Read these first, in this order, in the nat checkout at
`~/Projects/notion-agent-tracker`:

1. `docs/design/task-sources/README.md` — **the spec**. The "Wire contract"
   section is the whole protocol with a JSON example per method, "Edge
   cases" says what nat tolerates and refuses, and "What the Shortcut mock
   asks for" is the UI this plugin is meant to fill. Everything you return
   is validated by nat (`internal/source/validate.go`): a `tag` of 1–3
   `[A-Z0-9]`, group ids unique and never `_`-prefixed, a group has
   `children` *or* `containers`, one level of nesting, a `choice` action has
   `options`, stdout under 4 MiB, 20 s per call (10 s for `event`).
2. `examples/nat-source-demo/nat-source-demo` — a complete reference
   implementation in Python. Your plugin must answer every method the demo
   answers, with the same shapes.
3. `docs/design/task-sources/gsc-data.jsx` and `gsc-nav.jsx` — the mock's
   card data and facts list, i.e. which Shortcut fields Craig wants to see.
4. `internal/source/CLAUDE.md` — how nat invokes you, logs, and fails.

## Where it lives

In the nat repo itself, at `plugins/shortcut/`, inside nat's own module
(`github.com/craigmjohnston/nat`): `plugins/shortcut/main.go` builds one
binary, `nat-source-shortcut`, and its packages sit under
`plugins/shortcut/internal/…`. Standard library only (no framework;
`net/http` and `encoding/json` are enough), and the wire types and response
validator are nat's own `internal/source`, not a copy. Installed with
`go install ./plugins/shortcut` and then symlinked or copied to
`~/.config/notion-agent-tracker/plugins/shortcut/nat-source-shortcut`.
Follow nat's conventions: nat's full gate green at the repo root before you
claim done, httptest fakes for the Shortcut API asserting exact requests,
100% coverage of new code, a `plugins/shortcut/CLAUDE.md` in nat's style
saying what is true everywhere in the plugin.

## The protocol, from your side

`nat-source-shortcut <method>` with one JSON object on stdin and one on
stdout, exit 0; non-zero exit with the message as the first stderr line.
Every request carries `project: {id, name, working_dir}`. **`describe` is
also called with an empty project** (by `nat source-list` and
`project-create --source`), so it must answer with no project at all.
Unknown JSON fields are ignored both ways. Never print a token to stderr.

Beyond the five methods, the binary may have its own subcommands for
humans — `login`, `config` — that nat never calls. Exit 2 for anything else.

## Shortcut mapping

Use the Shortcut REST API v3 (`https://api.app.shortcut.com/api/v3`, header
`Shortcut-Token`). Resources you need: `GET /member` (who I am),
`GET /members`, `GET /workflows` (states, each with `type` unstarted |
started | done), `GET /groups` (teams; each has a `color` and a mention
name), `GET /epics`, `GET /iterations`, `GET /labels`,
`GET /search/stories?query=…` (Shortcut's search syntax: `owner:me`,
`state:"Ready for Dev"`, `type:bug`, `label:x`, `epic:x`, `team:x`,
`is:done`, `!is:done`), `GET /stories/{id}`, `POST /stories/{id}/comments`,
`PUT /stories/{id}`, `POST /stories/{id}/tasks`, `PUT/DELETE
/stories/{id}/tasks/{task-id}`. Check the live API docs before trusting any
of this; field names below are from the public docs and may have moved.

**`describe`** → `name: "shortcut"`, `title: "Shortcut"`, `tag: "SC"`,
`icon_symbol: "rectangle.on.rectangle.angled"`, `icon_svg`: the Shortcut
mark from the mock's `gsc-shell.jsx` (`I` component, the 48×48 path) as a
single-colour `<svg>` using `currentColor`, `container_noun: "card"`,
`task_noun: "task"`, `menu: [Refresh (none), New segment (text)]`.

**`sidebar`** → three top-level groups, ids `doing`, `ready`, `done`:

- `doing` — stories in a workflow state of type `started` where I am an
  owner. Containers in Shortcut's own order (position).
- `ready` — `children`: one segment per saved search the user has configured
  for this project (see Settings), each a group `ready/<slug>` with the
  stories matching `<query> !is:done` restricted to unstarted states, and a
  `menu` of Rename (text), Edit query (text), Remove (destructive). The
  default configuration has one segment, "Mine", query `owner:me`.
- `done` — `lazy: true`, `count` = stories in a `done` state where I am an
  owner, `containers` (the 25 most recently completed) only when `done` is in
  `expand`.

A container: `id` = the story id as a string, `title` = story name,
`external_url` = `app_url`, `badges` = `[{text: <team mention name
uppercased, 2–3 chars>, color: <team color>, title: <team name>}]` (fall back
to the epic when the story has no team), `meta` = the estimate when set,
`menu` = Assign to me (none), Follow (none). A story may legitimately appear
under Doing and a Ready segment at once; that is fine.

**`container`** → `facts` in this order: id (`sc-<id>`), team, state
(workflow state name), type, epic, labels (comma-joined), owner(s),
requester, created, updated, iteration. `sections`: `story` (kind `prose`,
`body` = the story description, which is already Markdown), `comments`
(kind `comments`, author display names and a relative time, oldest first,
`composer: {id: "comment", label: "Comment", input: "text"}`), `links` (kind
`links`: the story's pull requests and branches from its VCS integration
with `state` open/merged, then `story_links` as "blocks"/"blocked by"/
"relates to", then any external links). `task_note`: "Linked to sc-<id>.
Merging moves the card to Done when it's the last open task."
`menu`: Assign to me, Follow.

**`action`**: `comment` (container, text) posts a comment; `assign` adds me
to owners; `follow` adds me to followers; `refresh` drops the cache;
`new-segment` (text = name) adds a segment with query `owner:me`; on a
segment `rename`/`edit-query`/`remove` do what they say. Every action
returns a short `message` and invalidates the cache. nat re-reads `sidebar`
after any action.

**`event`** — keep Shortcut in step with nat, idempotently (nat may send an
event twice, and out of order), and never block: a failure is exit 1 with a
line of stderr and nat moves on. Match a nat task to a Shortcut *story task*
whose description ends with `(nat:<task id>)`:

- `created` → add a story task "<title> (nat:<id>)".
- `claimed` → if the story is in an unstarted state, move it to the first
  `started` state of its workflow and add me as an owner.
- `handed_back` → comment "Branch `<branch>` is ready for review." once per
  branch (skip if the last comment already says so).
- `approved` → comment with the PR URL, once per URL.
- `merged` → mark the story task complete; if every story task is complete
  and no nat task on this card is still open (nat tells you nothing about
  the others — read the story tasks), move the story to the first `done`
  state of its workflow.
- `released` → nothing (the task is still filed). `deleted` → delete the
  story task.

Which states count as "started"/"done" comes from the workflow's state
types, not names; where a workflow has several of a type, take the first in
position order, and let the user override both per project in config.

## Settings, token, cache

- Token: `nat-source-shortcut login` stores it in the macOS Keychain
  (service `nat-source-shortcut`); `SHORTCUT_API_TOKEN` overrides for CI.
  A missing token makes every method but `describe` and `setup` fail with
  stderr "Shortcut token missing — set it in gnat's Settings ▸ Sources or
  run nat-source-shortcut login". nat shows that line in gnat. (Amended
  after the build: `describe` must answer with no token, and lists the
  token as its one `setup` field, which gnat's Settings sends back through
  the `setup` method — see the protocol spec.)
- Per-project config, keyed by the nat project id, in
  `~/.config/nat-source-shortcut/config.json`: the segments (name, query),
  optional started/done state overrides, optional team filter. nat stores
  nothing for you. `nat-source-shortcut config <project id>` prints it.
- Cache: nat calls `sidebar` on every poll of the app (every few seconds).
  Cache each response under `$XDG_CACHE_HOME/nat-source-shortcut/` (else
  `~/Library/Caches/nat-source-shortcut`) with a 30 s TTL, keyed by project
  and `expand`; serve stale on an API error and say so in stderr only when
  nothing cached exists. Never let a call run past 20 s: set an HTTP timeout
  of 8 s and return what you have.

## Verify

1. Unit tests against httptest fakes for every method and every event,
   including duplicate and out-of-order events, a missing token, an API
   error with and without a cache, and the validation rules nat applies
   (write a test that runs nat's validator over your `describe` and
   `sidebar` output — vendor the rules from the spec rather than importing
   nat).
2. Drive it with nat itself, in throwaway XDG dirs so nothing of Craig's is
   touched: `nat source-list`, `nat project-create Work --source shortcut
   --plan-dir …`, `nat info --project <id> --json`, `--expand done`,
   `nat slice-add 'Try it' --container <story id> --project <id>`,
   `nat container-show <story id> --project <id>`, `nat source-action
   --action comment --container <story id> --input 'hi' --project <id>`,
   and confirm the story task and comment appear in Shortcut. Use a scratch
   story in a scratch workspace or epic for this, never a real one, and
   clean up after.
3. In gnat: `make dev` in the nat checkout with the plugin symlinked into
   the plugins dir; the Work fold should show Doing / Ready › Mine / Done
   and a card's Story pane should show its description and comments.

Report what you built, every place the live API disagreed with this brief
and what you did about it, and the transcript of step 2.
