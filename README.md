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
- Notion's official CLI, `ntn` (`curl -fsSL https://ntn.dev | bash`), logged in
  with `ntn login`. The tracker reads its Notion token from the CLI rather than
  storing one of its own, so no integration or personal access token is needed —
  and because the token is workspace-scoped, there is no per-page
  ••• → Connections step.

## Setup

```sh
ntn login                # once, to authorise the CLI against your workspace
go build ./cmd/notion-agent-tracker
./notion-agent-tracker   # first run launches the onboarding wizard
```

Install the skills so `/queue-work` (plan work into the tracker) and
`/next-slice` (pick up and complete the next slice) are available in any repo:

```sh
ln -s "$(pwd)/skills/queue-work" ~/.claude/skills/queue-work
ln -s "$(pwd)/skills/next-slice" ~/.claude/skills/next-slice
```

## Status

Early development — being dogfooded on its own Notion tracker.
