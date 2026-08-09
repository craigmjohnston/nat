# notion-agent-tracker

A Go TUI over Notion for tracking project work executed by Claude Code agents.
Craig manages the plan and launches agents from the TUI; agents run as fresh
`claude` sessions in tmux and talk to Notion via the Notion MCP. The TUI talks
to the Notion REST API directly (`Notion-Version: 2026-03-11`, data-source
model).

## Architecture

- `cmd/notion-agent-tracker/` — entrypoint
- `internal/config/` — XDG config (`~/.config/notion-agent-tracker/config.json`)
  + Keychain via zalando/go-keyring (service `notion-agent-tracker`)
- `internal/notion/` — hand-rolled stdlib Notion client (no third-party client
  supports data sources); minimal structs, only fields we use
- `internal/domain/` — Project/Milestone/Slice models, progress math
- `internal/agent/` — agent prompt template + tmux session management
  (sessions named `nat-<first-8-hex-of-slice-page-id>`)
- `internal/tui/` — bubbletea v2 (`charm.land/*/v2` imports); root model in
  `app.go` routes screens; all Notion I/O via tea.Cmd → typed msgs
- `skills/queue-work/` — the /queue-work planning skill (symlinked into
  `~/.claude/skills/`)

## Domain rules

- Slice workflow: Todo → Claimed → Done. Never edit Claimed/Done slices.
- Claiming = set Assignee (people property, the configured real Notion user)
  + Status → Claimed.
- Slices ↔ PRs are 1:1 when work is code; PR URL recorded in the `PR` property.
- Slices may carry a `Repo` override; otherwise the project default working
  dir from local config applies.
- Milestones live in their own DB (Name/Order/Status: Queued/Active/Done);
  slices relate via the `Milestone` relation.

## Conventions

- Bubble Tea v2 idioms: `View()` returns `tea.View`; match `tea.KeyPressMsg`;
  `tea.ExecProcess` for tmux attach.
- Tests: aim for 100% coverage of new code. httptest for the Notion client
  (assert exact request JSON), interfaces + fakes for keyring/tmux, teatest
  for TUI flows, golden snapshots for renders.
- Gate before claiming done: `go vet ./... && go test -race -cover ./...`
  (plus `golangci-lint run` if installed).
- Never log or commit the Notion API key; it lives in the macOS Keychain only.
