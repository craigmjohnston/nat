package domain

import "time"

// Session is an ad hoc agent session: a bare Claude Code session started on a
// project with no slice and no prompt, tracked so the app can show it running,
// read what it changed, and find the pull requests it opened — including
// several from one session.
//
// Dir is where the session runs: the repository (or plain directory) it was
// launched against, exactly as a slice's own Repo names the project rather
// than a worktree path — the worktree itself, when there is one, is derived
// from Dir and Branch the same way a slice's is, rather than stored directly.
// Branch is empty for a session launched outside any git repository, which
// runs in Dir itself rather than in a worktree at all.
//
// EndedAt is the zero time until nat has seen the session gone — every PR it
// opened landed, or it was ended with none open — and is the store's own
// record of that fact, since a project tracked in Notion knows nothing of
// sessions at all.
type Session struct {
	ID        string
	StartedAt time.Time
	Dir       string
	Branch    string
	EndedAt   time.Time
}

// Ended reports whether nat has recorded this session as finished with.
func (s Session) Ended() bool { return !s.EndedAt.IsZero() }
