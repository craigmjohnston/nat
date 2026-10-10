package agent

import (
	"fmt"
	"strings"
	"time"
)

// ResumeContext is what [ResumePrompt] is written from: the slice, the project
// it is pinned to, when the resumed session started (its [SessionRecord]) and
// what the slice's task log has recorded since, already rendered by the caller
// ("" for nothing).
type ResumeContext struct {
	SliceName string
	SliceID   string
	ProjectID string
	StartedAt time.Time
	Changes   string
}

// ResumePrompt is the short prompt a relaunch that resumes the slice's earlier
// Claude Code session ([Tmux.LaunchResumed]) opens with, in place of the full
// brief the transcript already holds: the slice, that the conversation is the
// agent's own earlier session, what changed on the board since it started,
// and the standing rules tests hold every prompt to — every command pinned to
// the project, slices named by name, the user's tmux left alone, and waiting
// said out loud. Those are restated rather than left to the transcript, which
// a compaction may since have summarised away.
func ResumePrompt(c ResumeContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are continuing the slice %q (slice ID %s). This conversation is your\n", c.SliceName, c.SliceID)
	b.WriteString("own earlier session on it, resumed after that session stopped before\n")
	b.WriteString("the work was handed back: your brief and everything you did are above.\n")
	b.WriteString("Carry on from where you left off, without reading the brief again. Look\n")
	b.WriteString("at the working tree first: work in progress may not be committed.\n")

	b.WriteString("\n## Since that session started\n\n")
	if changes := strings.TrimSpace(c.Changes); changes != "" {
		fmt.Fprintf(&b, "The slice's task log has recorded this since %s. It\n", c.StartedAt.Format(time.RFC3339))
		b.WriteString("is part of your brief now, and where it asks for something, do it.\n\n")
		b.WriteString(changes)
		b.WriteString("\n")
	} else {
		b.WriteString("Nothing has been recorded on the slice since then.\n")
	}

	b.WriteString("\n## Standing rules\n\n")
	b.WriteString("Every `nat` command names the project this slice is in, as your brief\n")
	b.WriteString("says — a command given no project is refused:\n\n")
	fmt.Fprintf(&b, "    --project %s\n\n", c.ProjectID)
	b.WriteString("The hand-back is the one your brief gives:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --project %s ...\n", c.SliceID, c.ProjectID)
	b.WriteString(namingPassage)
	b.WriteString(tmuxPassage)
	return b.String()
}
