package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceMove refiles a slice under another milestone — the headless half of the
// board's m key, and what the macOS app's move menu runs. Only the Milestone
// column is written: the slice's own brief, status and repo say nothing about
// where in the plan it sits.
//
// A slice in progress is refused, the same rule the board applies: the plan
// may still describe it, but the page underneath belongs to whoever took it.
func sliceMove(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-move", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	milestoneRef := flags.String("milestone", "", "the milestone to refile the slice under, by name")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-move: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-move", rest[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(*milestoneRef) == "" {
		return usageErrorf("slice-move: no milestone given: pass --milestone")
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

	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	milestone, err := resolveMilestone(*milestoneRef, shape.Milestones)
	if err != nil {
		return err
	}

	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if s.Status == domain.SliceClaimed {
		return fmt.Errorf("%q is in progress: work in flight is not refiled under its agent", s.Name)
	}
	// A move to where the slice already is would be a real write for the sake
	// of nothing — refused so the caller learns the plan already reads as they
	// wanted it, rather than being told a change happened.
	if s.MilestoneID == milestone.ID {
		return fmt.Errorf("%q is already filed under %s", s.Name, milestone.Name)
	}

	if err := st.MoveSlice(ctx, s.ID, milestone); err != nil {
		return fmt.Errorf("move the slice: %w", err)
	}

	env.nudged()
	if *asJSON {
		return writeJSON(env.Out, sliceMovedJSON{
			ID: s.ID, Name: s.Name, URL: s.URL,
			MilestoneID: milestone.ID, MilestoneName: milestone.Name,
		})
	}
	_, err = fmt.Fprintf(env.Out, "# %s\n\nMoved to %s; the work itself is untouched.\n", s.Name, milestone.Name)
	return err
}

// sliceMovedJSON is the structured form of a successful move.
type sliceMovedJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url,omitempty"`
	MilestoneID   string `json:"milestone_id"`
	MilestoneName string `json:"milestone_name"`
}
