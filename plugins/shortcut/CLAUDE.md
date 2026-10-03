# plugins/shortcut

`nat-source-shortcut`, nat's task-source plugin for Shortcut, built from
this tree inside nat's own module. nat owns the tasks; this binary owns the
*containers* they hang off — Shortcut stories, drawn in gnat as cards. nat
never imports it: it is a separate binary nat runs over the wire contract
in `docs/design/task-sources/README.md` (protocol v1). The brief it was
built from is `docs/design/task-sources/shortcut-plugin-brief.md`.

## Map

- `main.go` — hands the real process to `plugin.Run`; its edges (args,
  stdio, exit, env, the Keychain) are package variables `main_test.go`
  swaps, as nat's own `main.go` does.
- `internal/plugin/` — the program: the six methods (`describe`, `sidebar`,
  `container`, `action`, `event`, `setup`), the human subcommands (`login`,
  `config <project id>`) and `warm`, the sidebar's own detached epic fetch
  (`warm.go`). `read.go` is the two reads, `write.go` actions and
  events, `refs.go` the workspace reference data and every formatting rule.
- The wire types are nat's own `internal/source` (`Describe`, `Group`,
  `ContainerDetail`, `Task`, …), and tests hold every `describe`, `sidebar`
  and `container` response to its `Validate*`. Only the request union
  (`plugin.request`) and the sidebar envelope are declared here, since nat
  keeps those as unexported structs inside `source.Exec`'s methods.
- `internal/shortcut/` — the REST v3 client, cut to the fields used.
- `internal/settings/` — per-project config and the XDG paths.
- `internal/cache/` — the on-disk response cache.
- `internal/keychain/` — the token, through the `security` CLI.
- `internal/fakeshortcut/` + `cmd/fakeshortcut/` — an in-memory Shortcut
  (seeded scratch workspace, request recorder, failure injection) for the
  tests and for driving the plugin end to end through nat with no live token.

## True everywhere

- **One request in, one response out.** A method reads one JSON object on
  stdin and writes one on stdout, exit 0. A failure is exit 1 with **one**
  stderr line, which nat shows the user verbatim — word it for a person,
  `shortcut: …`. Usage errors exit 2. Unknown request fields are ignored.
- **Never print the token or a body.** Shortcut errors are method, path
  (no query — a search query is the user's text) and status; a decode
  failure is "malformed response". The token travels only in the
  `Shortcut-Token` header, to the configured base URL only (a search page's
  `next` link keeps just its query). `login` never sees the token: it runs
  `security add-generic-password … -w` with `-w` last so `security` prompts
  on the terminal itself. `setup` (gnat's Settings ▸ Sources, through `nat
  source-setup`) does see it, and keeps it out of every argv:
  `Keychain.Save` runs `security -i` and writes the `add-generic-password -U
  -s … -a … -w …` line to its **stdin**, each argument double-quoted with
  `\`/`"` escaped, refusing a token or account with a control character
  (a newline would start a second command). `Feed` discards what `security`
  prints.
- **Token**: `SHORTCUT_API_TOKEN`, else the Keychain (service
  `nat-source-shortcut`). Without one every method but `describe` and
  `setup` fails with exactly `Shortcut token missing — set it in gnat's
  Settings ▸ Sources or run nat-source-shortcut login`
  (`plugin.TokenMissing`). **`describe` reads no credential** — no token, no
  project, no request (`nat source-list` sends it an empty project) — and
  lists the one setup field, `token` (secret, hinted "Shortcut ▸ Settings ▸
  API Tokens"), so gnat can ask for it. Its `set` is presence alone:
  `SHORTCUT_API_TOKEN`, else `Keychain.Has` (`security
  find-generic-password -s … -a $USER`, **no `-w`**, output discarded); a
  failed lookup is `false`, never an error.
- **`setup`** takes only id `token` and a non-blank input (trimmed), stores
  it through `Tokens.Save` under `$USER` (else `nat`), then checks it
  exactly as `login` does — read back, `GET /member` — and answers `Logged
  in to <workspace url_slug> as <name>` (`/member` carries no workspace
  name). A refused token stays stored, as with `login`.
- **`SHORTCUT_API_URL`** overrides the API base (default
  `https://api.app.shortcut.com/api/v3`) — the fake server's address in a
  scratch run.
- **Time budgets.** Each request is capped at 8 s (`shortcut.Timeout`); a
  method at 18 s, an event at 9 s, inside nat's 20 s / 10 s kill. Reads a
  method needs are fetched **in parallel** in one round (`parallel`, which
  reports the first error in argument order so the shown line is stable).
- **`owner:me` never reaches Shortcut.** The live search accepts it and
  matches nothing, so every query goes out with each whole `owner:me` term
  (any case, `!`/`-` kept) rewritten to `owner:<mention_name>` from
  `GET /member` (`withMe`). The fake mirrors the live API: `owner:me`
  matches nothing there either.
- **The sidebar.** Doing (`owner:me is:started`), then each segment as a
  top-level group (`ready/<id>`, unstarted stories), then Done — `owner:me
  is:done completed:<Monday>..*`, this week from Monday in `a.now`'s own
  time (`weekStart`), lazy with a count. Every query is one `refs.search`:
  the filter's terms — `team:<mention>`, `project:<id>`, `epic:"<name>"`
  (the search takes an epic's title; the id is looked up, the id itself
  where it can't be), `label:"<name>"` each — then the group's own. Doing and
  Done use the section filter (`settings.Project.Filter`); a segment uses
  `merged(section, segment)` — each field the segment sets replaces the
  section's, labels as a whole. The response's top-level `menu` is the
  header's: describe's actions plus the section's Filter…; each segment's
  menu is Rename, its Filter… (fields carrying `inherited`, the section's
  value "Any" falls through to) and Remove. A filter action's options are the
  workspace's unarchived teams, projects (`GET /projects`, read beside
  groups; a failure is no projects), epics and labels (`GET /labels?slim=true`,
  through the 1 h cache); a saved choice no longer offered is offered still.
- **Badges** are a story's Shortcut project — `abbreviation` (else a code
  from its name), its hex `color` (else grey), title the name — else its team,
  else its epic, else none. `project` is a fact after `team`.
- **Team colours** come from `color_key` (Shortcut sends `color: null`),
  mapped to hex by `colorKeys`; a hex `color`, if ever sent, wins; an
  unknown key is grey. Archived teams are never offered in any list of
  teams (none exists yet — keep it that way when one is added).
- **Never `GET /epics` with descriptions, and never `GET /iterations`** —
  the full epic list is megabytes on a real workspace. Ids the stories at
  hand reference are looked up one by one (`/epics/{id}`, `/iterations/{id}`;
  the sidebar only for project- and team-less stories' badges and the
  filters' epics), through a 1 h cache shared by every project on the same
  API (`lookup`), stale on error, an empty fact when nothing is cached. The
  one exception is the filter editor's epic list: `GET
  /epics?includes_description=false`, **only** from the `warm` subcommand,
  never on a method's own path. A sidebar reads it from the 1 h cache alone
  (`cachedEpics`); with nothing cached, or only a stale list, it starts
  `nat-source-shortcut warm` detached (`Env.Spawn` — its own session, stdio
  `/dev/null`, not waited on, so nat's kill of the sidebar never reaches it;
  at most once a minute, `warmingKey`), sends the epic field `loading`, and
  keeps that sidebar out of the response cache so gnat's one re-read finds
  the list. `warm` prints nothing and exits 0 with the list cached, 1
  without. Refresh drops the cache, list and all; other actions don't.
- **Cache.** `sidebar` (keyed by sorted `expand`) and `container` (by id)
  responses are cached per project for 30 s under
  `$XDG_CACHE_HOME/nat-source-shortcut` (else
  `~/Library/Caches/nat-source-shortcut`). A failed rebuild serves the stale
  entry silently; only with nothing cached does the error reach nat. Every
  action drops the project's cache, success or not. The cache is
  best-effort: it never fails a call.
- **Settings** live in `$XDG_CONFIG_HOME/nat-source-shortcut/config.json`
  (else `~/.config/…`), keyed by nat project id: `segments` (id, name,
  `filter` — team mention name, project id, epic id, label names),
  `started_state`, `done_state` (a state name or id in the story's
  workflow), `filter` (the section's, narrowing every search; an older
  entry's `team` reads into it, and a segment's older `query` is not read).
  The `filter` action sets the section's (no group) or a segment's, its input
  a JSON object of field id to choices. A project with no entry gets one
  segment, Ready, with an empty filter; nil segments mean default,
  an empty list means the user removed them all. A segment's id is fixed at
  creation — its group id is `ready/<id>`, which nat and gnat remember, so a
  rename never changes it. Nothing is written until an action changes it.
- **States by type, never by name.** started/done is the workflow state's
  `type`; where a workflow has several, the first by `position` — unless the
  project overrides it.
- **Events are idempotent and order-proof.** Each reads the story first and
  writes only what isn't already so; a nat task is the story task whose
  description ends `(nat:<task id>)`. `merged` makes the story task
  (complete) if `created` never landed, so a late `created` finds it. A
  claim on a story already started or done is left alone. `released` and
  unknown events do nothing, successfully.
- **Lenient decoding.** Every Shortcut field may be missing, null or
  renamed; it degrades to an empty fact (`—`), never a failed call.
  `shortcut.Time` reads anything unparseable as zero.

## Gate

nat's own, run at the repo root — this tree is in it. Tests run against
`fakeshortcut` through httptest and assert the exact writes
(`Server.Writes`, method + path + body). No test touches the real Keychain
(`keychain.ExecRunner` is exercised only on `echo`/`true`/`grep`;
`main_test.go` swaps the token source out; `keychain_test.go` asserts the
exact `security -i` stdin and an argv with no token in it).

## Install

```sh
go install ./plugins/shortcut        # or: go build -o <dir> ./plugins/shortcut
mkdir -p ~/.config/notion-agent-tracker/plugins/shortcut
ln -s ~/go/bin/nat-source-shortcut ~/.config/notion-agent-tracker/plugins/shortcut/nat-source-shortcut
nat-source-shortcut login
```

Build into a scratch dir, never the tree: a built binary is never
committed (both are in the root `.gitignore`). A scratch run against the
fake: `go run ./plugins/shortcut/cmd/fakeshortcut -token fake -addr
127.0.0.1:47811`, then `SHORTCUT_API_TOKEN=fake
SHORTCUT_API_URL=http://127.0.0.1:47811/api/v3` and throwaway
`HOME`/`XDG_*` for every `nat` call (on macOS nat puts plans and logs under
`~/Library` whatever `XDG_*` say, so `HOME` must be scratch too).
