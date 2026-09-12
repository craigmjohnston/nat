package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
)

// milestoneRemove drops one milestone from the plan. It is the last of the
// one-off milestone edits: a phase that turned out not to be one, taken off
// without opening the board.
//
// It removes the milestone and nothing else. A milestone with slices still
// filed under it is refused by the store, naming them, because a milestone is
// nothing but the name those slices carry — emptying it first, with the move
// and delete commands that already exist, is the caller's call about the work
// rather than this command's about the plan.
func milestoneRemove(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("milestone-remove", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("milestone-remove: want exactly one milestone name, given %d", len(rest))
	}
	name := strings.TrimSpace(rest[0])
	if name == "" {
		return usageErrorf("milestone-remove: the milestone name is empty")
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(projectID, project)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	sp := storeProject(projectID, project)

	shape, err := st.Shape(ctx, sp)
	if err != nil {
		return err
	}
	m, err := st.RemoveMilestone(ctx, sp, shape, name)
	if err != nil {
		return err
	}
	env.nudged()

	if *asJSON {
		return writeJSON(env.Out, milestoneRemovedJSON{Milestone: milestoneJSON{
			ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status),
		}})
	}
	_, err = io.WriteString(env.Out, milestoneRemovedMarkdown(m, project.Name))
	return err
}

// milestoneRemovedJSON is the removal as a program reads it: the milestone as it
// last stood, its place in the plan included, since that place is what a
// caller's own record of the plan is keyed by and what has just closed up.
type milestoneRemovedJSON struct {
	Milestone milestoneJSON `json:"milestone"`
}

// milestoneRemovedMarkdown reports the removal, saying where in the plan the
// milestone was — the one thing about it there is no longer anywhere to read —
// and that it was empty, which is what let it go at all.
func milestoneRemovedMarkdown(m domain.Milestone, projectName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", m.Name)
	fmt.Fprintf(&b, "Removed from %s, where it was milestone %s and held no slices.\n\n",
		projectName, planPosition(m))
	fmt.Fprintf(&b, "- %s\n", removedNote)
	return b.String()
}

// removedNote is what became of the milestone: it had no page to trash, being
// an option of the slices' own Milestone column, so the line that would say
// where to recover it says what was dropped instead.
const removedNote = "Dropped as an option of the slices' Milestone column, which is all a " +
	"milestone is — the options after it close up, and nothing else about the column changes."
