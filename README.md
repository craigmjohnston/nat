# notion-agent-tracker

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
- A Notion internal integration API key (stored in the macOS Keychain on
  first run) with access to your project pages and the *user information*
  capability

## Setup

```sh
go build ./cmd/notion-agent-tracker
./notion-agent-tracker   # first run launches the onboarding wizard
```

Install the planning skill so `/queue-work` is available in any repo:

```sh
ln -s "$(pwd)/skills/queue-work" ~/.claude/skills/queue-work
```

## Status

Early development — being dogfooded on its own Notion tracker.
