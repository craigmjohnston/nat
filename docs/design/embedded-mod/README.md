# The embedded mod

nat carries one Claude Code mod of its own, `nat-embedded`, and loads it into
every agent session it launches. This page is the contract.

## Where it lives

`mods/embedded/` in this repository: `.claude-plugin/plugin.json`,
`hooks/hooks.json` (`{ "modules": ["./register.ts"] }`), `hooks/register.ts`,
`types/index.d.ts` (the `$.state` contract the manifest names as `types`),
`tests/*.test.ts` and a README naming the Claude Code version it was tested
with. No `node_modules`, no build step: Claude Code loads `.ts` directly.

Package `mods` (`mods/mods.go`) embeds the manifest, `hooks/` and `types/` with
`go:embed`, as package `skills` embeds the skills — a `go install`ed nat has
no checkout to read from. The tests and README stay out, as do the
`.claude-plugin/types/` and `tsconfig.json` a build lays beside a loaded mod
(gitignored).

## How it loads

`internal/mods.Materialise` writes the mod to
`<state dir>/mods/<hash>/nat-embedded/`, under `logging.Dir()` beside
`agent-status/` and `usage-probe/`. `<hash>` is the first 12 hex digits of a
sha256 over the files as written, names and contents; the manifest's
`version` is stamped with the nat build's (`version.Version()`) as it is
written, so a new nat is a new hash even where the hooks did not change.

`internal/agent`'s `modelFlags` appends `--plugin-dir <that folder>`, so every
launch carries it — slice, fix and planning agents through `agentCommand`,
ad hoc sessions through `bareLaunchArgs`. The usage probe does not: it is no
agent.

`--plugin-dir` loads a plugin for that one session only. It is never
installed, never touches `~/.claude/settings.json`, and never reaches the
user's own sessions. (`CLAUDE_CODE_PLUGIN_DIRS` does the same where no flag
can be given; nat builds the command line, so the flag is the seam.) In the
session, `/plugin` lists it as a mod.

## Write-once per hash

Claude Code watches a `--plugin-dir` folder and hot-reloads on any change, so
a folder a live session loaded is never rewritten: a hash directory that
exists is used as it stands. It is written whole under a temp name and
renamed into place, so one that exists is always complete — a half-written
module is never loaded — and of two nats materialising the same hash at once,
the second uses the first's. Files are `0600`, directories `0700`.

## Sweeping old hashes

Deleting a folder a live session loaded unloads the mod from that session
there and then (checked on 2.1.294: the hint came back and `/plugin` listed
no mod), so an old hash goes only once nothing runs it. After every
`Launch`/`LaunchBare`, `internal/agent` reads the start command of every pane
on the tmux server (`list-panes -a -F '#{pane_start_command}'`, which carries
each launch's `--plugin-dir`) and hands them to `internal/mods.Sweep`, which
removes every folder under
`<state dir>/mods/` except:

- this build's own hash;
- a hash named in any live pane's start command;
- anything younger than a minute, which covers an older nat running beside
  this one that has just written its hash and not yet started its session,
  and a temp tree still being written.

Leftover `.tmp-*` trees from a failed write go the same way. A pane read that
fails removes nothing, and a removal that fails is logged and retried on the
next launch.

One race is accepted: a second nat build that *reuses* an old hash folder
(older than a minute) and is between materialising and starting its session
can lose that folder. Its session then starts without the mod, which is the
same degrade as a failed write.

## Degrading

A mod that cannot be written is logged (`embedded mod disabled for a launch`)
and the agent launches **without** the flag, as the statusline sink degrades:
a missing mod never costs an agent its launch. Inside Claude Code, a hook
that fails is skipped and a tree that does not validate is replaced by Claude
Code's own drawing, so a drift costs a feature, never a session.

## Versioning

The mods API can change between Claude Code releases. The mod is written
against the public reference (`https://code.claude.com/docs/en/plugins/mods/`)
and the declarations the installed build writes beside a loaded mod
(`<mod>/.claude-plugin/types/claude-code/index.d.ts`), never against one
build's quirks. The mod's README names the Claude Code version it was last
tested with.

## Checking it

`scripts/mod-check.sh` runs `claude plugin validate --strict mods/embedded`
and `claude plugin test mods/embedded`. Both need `claude` on PATH and no
sign-in or network. It is part of the project gate and has a CI job of its
own, which installs Claude Code with `npm install -g @anthropic-ai/claude-code`.

## The inbox

Every prompt nat sends a live agent (`agent.Tmux.SendPrompt`: `agent send`,
triage, notes, the checks nudge, the board's review comments) goes through
the mod where it can. Each launch sets `NAT_INBOX=<state dir>/agent-inbox/<session>`
with `new-session -e`; a send writes `<unix nanoseconds>.md` there (temp name
and rename, directory `0700`, file `0600`; the text never logged) and waits
up to 3 s for it to go. The mod polls once a second from `session.start`
(`$.clock.every`): it lists the inbox, and for each file in name order reads
it, removes it with `rm`, and — only where the removal worked — submits it
with `$.prompt.submit({ text, asUser: true })`, unawaited. A file still there
after the wait (no mod: an older Claude Code, a session launched before this
nat) is removed by nat and pasted instead, so a send arrives once either way;
a session with no `NAT_INBOX` (a tmux too old for `-e`) is pasted to at once.
The transcript labels each such prompt as from the `nat-embedded` plugin; the
model reads it bare, as the user's own words.

## The resume

Every nat send to a handed-back agent resumes the slice first (`nat
slice resume`, a `Resumed` card with the request, `Branch` cleared). A prompt
the user types in the pane is the one nat never sees, so the mod records it: a
slice launch (`actions.Launch`, never a planning or ad hoc one) also sets
`NAT_SLICE=<slice page ID>` and `NAT_PROJECT=<project page ID>` with
`new-session -e`, beside `NAT_INBOX`, and the mod's `prompt.submit` hook, for
a prompt whose `origin.kind` is `composer` or `bridge` (Enter in the pane, or
Remote Control) on a session with both set, runs `nat slice resume $NAT_SLICE
--project $NAT_PROJECT --note -` with the prompt on stdin before passing it
on. The origin, not a list of what the poller submitted, is what tells the
user's words apart: an inbox delivery is a plugin's, and a background task's
notification, a schedule or a peer is not the user asking for more either.
`slice resume` writes nothing where the slice is not handed back, so every
typed prompt runs it. A non-zero exit or a throw is logged to debug and the
prompt goes on. The slice prompt says nothing of it; `/next-slice`, run by
hand with no mod, keeps its own paragraph.

## The usage file

`nat usage` used to start a throwaway Claude Code for every reading. Every
live agent already has the numbers, so each launch also sets
`NAT_USAGE=<state dir>/agent-status/<session>.usage.json` with
`new-session -e`, beside `NAT_INBOX`. The mod's `session.measure` hook (after
each turn, and whenever a window moves a whole point) writes
`{"read_at": <ISO 8601>, "rate_limits": {"five_hour": {"used_percentage",
"resets_at"}, "seven_day": {…}}}` there, each window only where the
measurement carries it — a measurement with neither writes nothing — to
`<file>.tmp` first, then `mv`, as the statusline tee does. `nat usage` reads
the files of live sessions and answers the freshest under ten minutes old
(`"source": "agent"`), probing only where there is none (`"source":
"probe"`). The status sweep removes a gone session's file with its others.
Checked live on 2.1.296: opening `/usage` in a session, no model turn, fires
the hook and writes the file; `nat usage --json` then answers from it in
under half a second.

## The brief

A slice, fix, planning or new-project session is not started with its brief
as `claude`'s positional prompt, which the pane would draw in full. Where
`prepareMod` wrote the mod, `agentCommand` runs
`NAT_BRIEF=<prompt file> claude ... '<opening line>'` (the variable set in the
`sh -c` command, no tmux `-e`), leaving the file `WritePromptFile` wrote where
it is. The opening line is one sentence per prompt kind
(`agent.OpeningLine`, `PlanOpeningLine`, `NewProjectOpeningLine`) — `Work the
slice "<title>": your brief is the natBrief block of this message.`,
`Continue …` for a session resuming work — and is all the pane shows.

The mod hands the brief over twice, through two hooks:

- **`prompt.submit`**, on the conversation's opening prompt — the one
  `$.session.turns()` reads `0` for — reads `NAT_BRIEF` and adds the file to
  the prompt's `context`, wrapped as `<natBrief>…</natBrief>` (`context` is
  plain text, with no block name of its own). The opening line arrives as
  `origin.kind` `composer`, as a typed prompt does, so the turn count, not
  the origin, is what tells it apart. Every later prompt, and the prompt a
  `claude --resume` opens with, passes through: the transcript already
  holds the brief. The count is the engine's, so a hot reload of the mod
  changes nothing.
- **`prompt.context`**, which fires when the engine computes the context
  blocks of a conversation's first user message (`claudeMd`,
  `currentDate`, …), appends `{ name: 'natBrief', text }` after the
  engine's blocks — except where the count is `0`: those are the opening
  prompt's own blocks, computed just after `prompt.submit` briefed it. The
  next request after a compaction or a `/clear` computes them afresh, and
  the brief comes back with them.

`prompt.submit` is the one every machine runs. On a Team or Enterprise
plan, or a machine with managed settings, Claude Code seats its built-in
guard, `sec-default@builtin`, ahead of every user mod, and the guard's
`prompt.context` hook (`next.to(e, 'append')`) skips the whole user tier,
where a `--plugin-dir` mod sits: under `prompt.context` alone, the brief
never arrived there. The guard leaves `prompt.submit` and its additive
context to a person's mods. Behind it, nothing re-reads the brief: a
compaction keeps only what its summary says of it (checked here with the
`prompt.submit` hook alone — a short session's `/compact` dropped the
brief's text), and a `/clear` loses it.

A read that fails is logged to debug and the prompt or the blocks pass
through: the agent has the opening line alone and asks. A turn count that
cannot be read is logged too; it briefs no prompt, and briefs the blocks,
so the model reads the brief twice rather than not at all.

Checked live on Claude Code 2.1.296 (haiku, a private tmux socket):

- the pane shows the opening line and the agent's reply, never the block —
  under ctrl+o either;
- the model sees one `natBrief` block on a fresh session, not two;
- `/compact` re-runs `prompt.context` on the next request and re-reads the
  file: the brief file was changed after launch, and after the compaction
  the block held the new text, so the file must stay where it is for the
  session's life;
- `claude --resume` of the session keeps the block **as the transcript
  recorded it** — it is not re-read on resume (the file had changed again,
  and the resumed session still answered from the old text);
- the opening line's `prompt.submit` reads `origin.kind` `composer` and a
  turn count of `0`; a resumed session's opening prompt reads `1` or more,
  and so does the first prompt after a `/clear` (the `/clear` counts).

`prompt.submit` on a Team plan behind the guard was checked on that
machine (2.1.296): the context it attaches is hidden in the pane and under
ctrl+o, and present on `claude --resume`.

Degrade: where no mod could be written, the brief stays the positional prompt
exactly as before. A session whose mod is written but never loads (an older
Claude Code with no mods) starts on the opening line alone and asks what to
work — visible, not silent.

## The session record

A relaunch (`actions.Launch` of a slice picking work up) resumes the slice's
earlier Claude Code session where it can, rather than briefing a fresh one.
Every launch sets `NAT_SESSION_RECORD=<state dir>/agent-status/<session>.session.json`
with `new-session -e`, beside `NAT_INBOX` (the whole path: the state directory
is nat's to resolve, never the mod's), and the mod's `session.start` writes
`{ "session_id": $.session.id(), "cwd": e.cwd, "started_at": <ISO 8601> }`
there — `$.fs.write` to `<path>.tmp`, then `mv` into place. A resume or a
`/clear` starts the session again and rewrites it. A failure is logged to
debug; with no record, the relaunch is fresh. Nat's side — when it is read,
removed and resumed — is in `internal/agent/CLAUDE.md`.

## Agents never know

Nothing in any agent prompt or embedded skill mentions the mod. It changes
what the session looks like and does, not what the agent is asked to do —
the opening line names the `natBrief` block, not where it came from.
