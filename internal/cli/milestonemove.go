package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
)

// milestoneMove moves one milestone in the plan, to sit directly before or after
// another. It is the last of the one-off milestone edits and the only one about
// the plan's order: a phase that turns out to belong earlier — or later — put
// where it belongs without opening the board.
//
// It changes the order of the plan and nothing else. A milestone's order is its
// place among the others, so a move leaves every name as it was and every slice
// filed where it was, which is what the store arranges and says out loud.
func milestoneMove(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("milestone-move", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	beforeRef := flags.String("before", "", "the milestone to sit directly before, by name")
	afterRef := flags.String("after", "", "the milestone to sit directly after, by name")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("milestone-move: want exactly one milestone name, given %d", len(rest))
	}
	name := strings.TrimSpace(rest[0])
	if name == "" {
		return usageErrorf("milestone-move: the milestone name is empty")
	}
	target, before, err := placement(*beforeRef, *afterRef)
	if err != nil {
		return err
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
	m, to, err := st.MoveMilestone(ctx, sp, shape, name, target, before)
	if err != nil {
		return err
	}
	env.nudged()

	if *asJSON {
		return writeJSON(env.Out, milestoneMovedJSON{
			Milestone:  movedMilestoneJSON{ID: m.ID, Name: m.Name, Order: m.Order},
			Placement:  placementWord(before),
			RelativeTo: movedMilestoneJSON{ID: to.ID, Name: to.Name, Order: to.Order},
		})
	}
	_, err = io.WriteString(env.Out, milestoneMovedMarkdown(m, to, before, project.Name))
	return err
}

// placement reads the two flags that say where the milestone is to land as the
// one answer they are: the milestone it sits beside, and which side of it.
//
// Exactly one of them is required, because a move with no target names nowhere
// to go and a move with both names two places at once — and neither is a
// mistake this command can pick a side of.
func placement(beforeRef, afterRef string) (string, bool, error) {
	before, after := strings.TrimSpace(beforeRef), strings.TrimSpace(afterRef)
	switch {
	case before != "" && after != "":
		return "", false, usageErrorf("milestone-move: --before and --after name two places at once: pass one")
	case before != "":
		return before, true, nil
	case after != "":
		return after, false, nil
	default:
		return "", false, usageErrorf(
			"milestone-move: no destination given: pass --before or --after, naming the milestone to sit beside")
	}
}

// placementWord is which side of the target the milestone landed on, said in the
// very word the flag is spelled, so the output reads as the command that made
// it.
func placementWord(before bool) string {
	if before {
		return "before"
	}
	return "after"
}

// milestoneMovedJSON is the move as a program reads it: the milestone as it now
// stands, where it was put and what it was put beside — which has a new place in
// the plan too, since everything from the lower of the two onwards has shifted.
type milestoneMovedJSON struct {
	Milestone  movedMilestoneJSON `json:"milestone"`
	Placement  string             `json:"placement"`
	RelativeTo movedMilestoneJSON `json:"relative_to"`
}

// movedMilestoneJSON is a milestone as a move reports it: its name and where in
// the plan it now sits, and no status. A milestone has no status of its own —
// the slices filed under it answer for it — and a move reads no slices, so
// naming one here would be reporting something nothing asked about.
type movedMilestoneJSON struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Order float64 `json:"order"`
}

// milestoneMovedMarkdown reports the move, saying the one thing it changed —
// where in the plan the milestone sits — and, in the note, the everything else
// it did not.
func milestoneMovedMarkdown(m, to domain.Milestone, before bool, projectName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", m.Name)
	fmt.Fprintf(&b, "Moved in %s to milestone %s, directly %s %s.\n\n",
		projectName, planPosition(m), placementWord(before), to.Name)
	fmt.Fprintf(&b, "- %s\n", reorderNote)
	return b.String()
}

// reorderNote is what a move leaves alone, which is everything but the order:
// the options of the slices' Milestone column are sent back in a new order and
// otherwise exactly as they were, so no milestone is renamed and no slice is
// refiled.
const reorderNote = "Only the order of the slices' Milestone column changed — every option is the " +
	"one it was, and the slices filed under each are untouched."
