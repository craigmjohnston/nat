# nat — notion-agent-tracker

A TUI for tracking project work in Notion, executed by Claude Code agents.

- **You** manage milestones and slices (small units of work) from the TUI and
  launch agents on them.
- **Agents** run locally as fresh `claude` sessions in tmux, claim their slice
  in Notion (via the Notion MCP), do the work, and mark it Done.
- **Notion** is the source of truth: a Project DB contains project pages; each
  project page holds its own Slices DB and Milestones DB, plus free-form
  project info in the page body.

## Requirements

- macOS, Go 1.25.x, tmux, the `claude` CLI
- Notion's official CLI, `ntn` (`curl -fsSL https://ntn.dev | bash`), logged in
  with `ntn login`. The tracker reads its Notion token from the CLI rather than
  storing one of its own, so no integration or personal access token is needed —
  and because the token is workspace-scoped, there is no per-page
  ••• → Connections step.

## Setup

```sh
ntn login                # once, to authorise the CLI against your workspace
go install github.com/craigmjohnston/nat@latest
nat                      # first run launches the onboarding wizard
```

The repo is private, so the module proxy cannot fetch it. Configure the Go
toolchain to go straight to GitHub over SSH, once per machine:

```sh
go env -w GOPRIVATE=github.com/craigmjohnston/*
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

To build from a clone instead: `make build && ./nat`.

## Running

`nat` hosts itself in tmux: started from an ordinary terminal it re-execs into a
tmux session called `nat-tui`, so that a running agent's pane can later be shown
beside the board. Run it again and you attach to the session already there
rather than starting a second board. Started from inside tmux it runs in place,
in whatever window you are in — it never nests a session inside a pane.

To run without tmux at all — accepting that the split view is unavailable — set
`NAT_NO_TMUX=1`:

```sh
NAT_NO_TMUX=1 nat
```

### Headless commands

Given a subcommand, `nat` runs it and exits rather than opening the board — and
without tmux, so the output lands in the terminal it was typed in. `nat help`
lists them.

`nat info` prints the active project as markdown: its conventions (the project
page body), its milestones in plan order, and its slices grouped under them with
their status, assignee and PR. `nat info --json` prints the same thing
structured, for an agent to parse:

```sh
nat info
nat info --json
```

### Watching an agent

`t` on a slice with a running agent shows that agent in a pane beside the board;
`t` again sends it back to a session of its own. The board keeps the keyboard
while the agent runs next to it, and the mouse — enabled for the `nat-tui`
session only, so your own tmux settings are left alone — moves between the two:
click the agent to type at it, click the board to come back.

The agent's share of the window defaults to 65%. To change it, add
`agent_split_percent` to `~/.config/notion-agent-tracker/config.json`:

```json
{
  "agent_split_percent": 75
}
```

Anything outside 10–90 is treated as a typo and the default is used instead.

Started outside tmux (`NAT_NO_TMUX=1`, or with tmux unavailable), there is no
window to split: `t` hands the whole terminal to the agent's session instead,
and detaching with `ctrl-b d` comes back to the board.

## Skills

`/queue-work` (plan work into the tracker) and `/next-slice` (pick up and
complete the next slice) come with the binary. `nat setup` installs them into
`~/.claude/skills`, which makes them available in any repo:

```sh
nat setup
```

Run it again after upgrading: each skill is reported as created, updated or
unchanged, so an install left behind by an older binary is one command away from
current. Nothing in `~/.claude/skills` other than the tracker's own skills is
read or written — and a skill directory that is a symlink, which is how a
checkout of this repo works on the skills in place, is left alone and said so.

## Status

Early development — being dogfooded on its own Notion tracker.
