# The terminal board

Running `nat` with no subcommand opens a board in the terminal: the same
plan the macOS app shows, drawn in text. It is kept in step with the CLI,
for when you live in a terminal. This page covers what is particular to it;
the commands, skills and configuration shared with the app are in the
[README](../README.md).

The board hosts itself in nothing: it draws its own status band and shows an
agent in a box of its own beside it. Started from inside a tmux session of
your own it behaves exactly the same.

tmux is still needed for the agents — each one runs in a detached session,
which is what lets it outlive the board and be shown again later — so `nat`
checks for it on startup and says how to install it if it is missing. The
headless commands launch nothing and need none of it.

## Keeping the board current

The board does not need restarting to notice a change. A write made through a
`nat` command — an agent claiming or closing out a task — shows within a
second. A change made elsewhere (in Notion, for a Notion-backed project) is
picked up by a background poll, every 30 seconds by default; `r` refetches at
once. A poll is skipped while a form, a prompt or a write is in flight, so
nothing lands on top of what you are typing, and resumes on the next one. A
poll that fails leaves the plan on the board as it was and says so on the
status line.

To change the interval, add `poll_seconds` to
`~/.config/notion-agent-tracker/config.json`:

```json
{
  "poll_seconds": 120
}
```

Anything outside 5–3600 is treated as a typo and the default is used instead.

## Which Claude Code an agent runs as

The `workshop_agent` and `slice_agent` pairs described in the README are
prefilled defaults, not fixed: the planning form (`w` and `W`) and the launch
options (`l`, then "configure & launch") show the pair and let you change it
for that one launch, leaving the config as it is.

## Watching an agent

`t` on a task with a running agent shows that agent in a pane beside the
board; `t` again sends it back to a session of its own. The board keeps the
keyboard while the agent runs next to it, and the mouse — reported to nat
only while an agent is on show, so your own selection and scrollback are
otherwise left alone — moves between the two: click the agent to type at it,
click the board to come back.

The agent's share of the window defaults to 65%. To change it, add
`agent_split_percent` to `~/.config/notion-agent-tracker/config.json`:

```json
{
  "agent_split_percent": 75
}
```

Anything outside 10–90 is treated as a typo and the default is used instead.

`T` is the way out of the split: it hands the whole terminal to the agent's
session, and detaching with `ctrl-b d` comes back to the board.
