# The embedded mod

nat carries one Claude Code mod of its own, `nat-embedded`, and loads it into
every agent session it launches. This page is the contract.

## Where it lives

`mods/embedded/` in this repository: `.claude-plugin/plugin.json`,
`hooks/hooks.json` (`{ "modules": ["./register.ts"] }`), `hooks/register.ts`,
`tests/*.test.ts` and a README naming the Claude Code version it was tested
with. No `node_modules`, no build step: Claude Code loads `.ts` directly.

Package `mods` (`mods/mods.go`) embeds the manifest and `hooks/` with
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

## Agents never know

Nothing in any agent prompt or embedded skill mentions the mod. It changes
what the session looks like and does, not what the agent is asked to do.
