package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
)

// milestoneRename gives one milestone another name, in place. It is the other
// half of milestone-add: a phase of the plan that has been renamed in the
// user's head and nowhere else, put right without opening the board.
//
// The plan keeps its order and the milestone keeps its slices — which is the
// whole of what "in place" means, and is the store's to arrange, since how a
// backend renames one is its own business.
func milestoneRename(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("milestone-rename", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return usageErrorf("milestone-rename: want the milestone's name and its new one, given %d", len(rest))
	}
	old, name := strings.TrimSpace(rest[0]), strings.TrimSpace(rest[1])
	if old == "" || name == "" {
		return usageErrorf("milestone-rename: a milestone name is empty")
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
	m, err := st.RenameMilestone(ctx, sp, shape, old, name)
	if err != nil {
		return err
	}
	env.nudged()

	if *asJSON {
		return writeJSON(env.Out, milestoneRenamedJSON{
			From: old,
			Milestone: milestoneJSON{
				ID: m.ID, Name: m.Name, Order: m.Order, Status: string(m.Status),
			},
		})
	}
	_, err = io.WriteString(env.Out, milestoneRenamedMarkdown(old, m, project.Name))
	return err
}

// milestoneRenamedJSON is the rename as a program reads it: the milestone as it
// now stands, and the name it went by, since the caller's own record of the plan
// is keyed by that name and has to be told which entry moved.
type milestoneRenamedJSON struct {
	From      string        `json:"from"`
	Milestone milestoneJSON `json:"milestone"`
}

// milestoneRenamedMarkdown reports the rename, saying the two things a rename
// leaves alone: where in the plan the milestone sits, and the slices under it.
func milestoneRenamedMarkdown(old string, m domain.Milestone, projectName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", m.Name)
	fmt.Fprintf(&b, "Renamed from %s in %s, still milestone %s, %s.\n\n",
		old, projectName, planPosition(m), blank(string(m.Status)))
	fmt.Fprintf(&b, "- %s\n", optionNote)
	return b.String()
}
