# internal/source

The client side of task sources: external `nat-source-<name>` binaries that
put another tracker's items (Shortcut cards, say) in front of nat as the
containers a source project's tasks hang off. The protocol — methods, wire
shapes, events — is specified in `docs/design/task-sources/README.md`; the
types in `source.go` mirror it field for field, so change the spec and the
types together.

**nat owns the tasks; the plugin owns the containers.** Nothing in this
package stores anything: it describes, lists, reads, acts and tells. The
plan itself is an ordinary `store.Local`, wrapped by `store.Sourced`.

## Wire

- `nat-source-<name> <method>` — argv is exactly the method. One JSON request
  on stdin (always `{"project": {id, name, working_dir}, ...}`), one JSON
  response on stdout, run in the project's working dir. A non-zero exit is
  `*ExitError`, whose `Error()` is the first non-empty stderr line
  (`firstLine`); `Exec` wraps every failure as `nat-source-<name> <method>: …`.
- `describe` refuses any `protocol` but `ProtocolVersion` (naming the plugin
  and both numbers) rather than half-understanding a newer plugin.
- Methods: `describe`, `sidebar`, `container`, `action`, `event`, `setup`.
- `sidebar` always sends `expand` as a list, never `null`, and answers a
  `Sidebar` — the groups and, optionally, the header `menu` that replaces
  `describe`'s for that project (a filter's choices are per project).
- `event` is fire-and-forget: its stdout is not read at all.
- `setup` (`Exec.Setup(ctx, id, input)`) is about no project, like
  `describe`: the envelope is empty and the run's dir is `""`. It sends
  `{id, input}` and reads `{message}`. The input is a credential — it goes
  on the plugin's stdin and nowhere else; the log line is plugin, method and
  `id`.
- **Validation** (`validate.go`) runs after decode: `Describe` →
  `ValidateDescribe` (tag `^[A-Z0-9]{1,3}$`, menu, `setup` fields: id
  `^[a-z0-9-]+$` and unique, input `secret`|`text`; `set` — `*bool`, nil
  where the plugin doesn't say — is not checked), `Sidebar` →
  `ValidateSidebar` (`ValidateGroups` — children *xor* containers, one level
  of children, unique group ids, no empty or `_`-prefixed group/container id —
  `_` is nat's, for `_unlisted` — and every menu — then the header menu),
  `Container` → `ValidateContainer` (menu and composers). Every action check
  is the same: a `choice` with no options, and any `secret` input (a setup
  field's alone), is refused; a `filter` with no fields, an empty or
  repeated field or option id, two values in a field not `multi`, or a value
  its field doesn't offer, is refused (`validateFilter`). A filter's answer
  is the action's `input`, a string holding the JSON object. The error is `nat-source-<name> <method>: invalid response: <rule>`
  — the rule, never the body (`Exec.invalid`).
- **Stdout is capped** at 4 MiB (`maxStdout`, a var for tests): `capWriter`
  refuses the write past it, which closes the plugin's pipe, and the cap is
  reported ahead of whatever exit that caused.
- `Unavailable{Err}` is a client answering `Err` to everything — what a
  caller hands a source project whose plugin can't be found, so the project
  still opens (the plan is nat's own) and every plugin read fails as a broken
  plugin's would.

## Never log a body

Request and response bodies — stdin, stdout — never reach a log call or an
error string. A container's body is someone's ticket from another system,
free text that may carry anything, and the log file was never agreed as a
place it goes. That includes the JSON decoder's own message, which can quote
the response: a decode failure is `nat-source-<name> <method>: malformed
response` and nothing more. A log line carries the plugin, method, project
ID, the ids the call was about and an exit code — nothing else. A `setup`
input is the sharpest case: `TestSetupNeverLogsTheInput` opens the real log
(through `internal/logging`'s redactor, never around it) and checks.

## Conventions

- `Runner`/`StdinRunner`/`ExecRunner`/`ExitError`/`firstLine` are
  `internal/gh`'s seam **re-declared on purpose**, not shared: each wrapped
  binary keeps its own, so neither package reaches into the other.
- Timeouts: `callTimeout` (20s) per call, `eventTimeout` (10s) for `event`,
  both package vars so tests can shorten them. `New` builds two
  `ExecRunner`s, one per timeout; `ExecRunner{}` (zero `Timeout`) means
  `callTimeout`. A timeout is reported as one, not as the killed process's
  exit code. Every call runs through `internal/subprocess`, which kills the
  plugin's whole process group at the limit and bounds the wait on its pipes.
- `Discover(configDir)`: `<configDir>/plugins/<name>/nat-source-<name>` first,
  then PATH (`pathEnv`, a test seam); the plugins dir wins a name clash. A
  plugin must be a regular file (a symlink to one counts) with an execute
  bit. A missing plugins dir is no plugins; an unreadable one is an error;
  an unreadable PATH entry is skipped. `Find` is `Discover` filtered.

## Faking it

- Inside this package: `NewWithRunner(name, path, r)` with a fake
  `StdinRunner` (see `exec_test.go`'s `fakeRunner`), which runs events
  through the same runner.
- Everywhere else: `source.Fake` — canned `DescribeResult`/`Groups`/`SidebarMenu`/`Details`/
  `ActionResult`/`SetupMessage`, per-method `…Err`, and recorded `Expands`,
  `ContainerIDs`, `Actions`, `Events`, `Setups` to assert what was sent. Its
  zero value works (a nil `Details` answers the zero detail).
- `demo_test.go` runs `examples/nat-source-demo` for real through `New`
  (skipped with no `python3`): describe, and its `setup` refusal.
