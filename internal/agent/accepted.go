package agent

import (
	"fmt"
	"strings"
)

// ProposalAcceptedPrompt is the one turn typed at a project's live planning
// agent once the user has accepted its proposal and the plan has applied:
// that it was accepted, what is on the board now because of it — milestones
// and slices by name, never by ID — and that from here on a revision reaches
// those slices only through the document's `edit`, `move` and `remove` lists,
// by title, after re-reading the plan. Without it the agent's picture of the
// board is the one from before the accept, and its next revision re-sends the
// whole document as new slices.
//
// slices are already labelled by the caller (a name, with its milestone).
func ProposalAcceptedPrompt(projectID string, milestones, slices []string) string {
	var b strings.Builder
	b.WriteString("The user accepted your proposal, and it has been applied to the plan.\n")
	if len(milestones) > 0 {
		fmt.Fprintf(&b, "\nNew milestones on the board now: %s.\n", strings.Join(milestones, ", "))
	}
	if len(slices) > 0 {
		fmt.Fprintf(&b, "\nNew slices on the board now: %s.\n", strings.Join(slices, ", "))
	}
	b.WriteString("\nThat proposal is no longer on screen to replace. A later revision\n")
	b.WriteString("reaches these slices only through the plan document's `edit`, `move`\n")
	b.WriteString("and `remove` lists, by title — never by creating them again. Before you\n")
	b.WriteString("propose again, re-read the plan:\n\n")
	fmt.Fprintf(&b, "    nat info --project %s\n", projectID)
	return b.String()
}
