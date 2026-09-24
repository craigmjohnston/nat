package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceReorder places one slice directly before or after another, the way
// milestone-move places a milestone. A slice moved next to one filed under
// another milestone is refiled to it in the same write — "directly before" has
// no other honest meaning — and that refile follows slice-move's rule: work in
// flight is not refiled under its agent.
//
// Both slices are read before anything is written, so a slice or target that is
// not there refuses with the plan untouched.
func sliceReorder(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-reorder", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	beforeRef := flags.String("before", "", "the slice to sit directly before, by URL or ID")
	afterRef := flags.String("after", "", "the slice to sit directly after, by URL or ID")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-reorder: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-reorder", rest[0])
	if err != nil {
		return err
	}
	beforeSet, afterSet := strings.TrimSpace(*beforeRef) != "", strings.TrimSpace(*afterRef) != ""
	switch {
	case beforeSet && afterSet:
		return usageErrorf("slice-reorder: --before and --after name two places at once: pass one")
	case !beforeSet && !afterSet:
		return usageErrorf("slice-reorder: no destination given: pass --before or --after, naming the slice to sit beside")
	}
	before, ref := beforeSet, *afterRef
	if before {
		ref = *beforeRef
	}
	target, err := pageID("slice-reorder", ref)
	if err != nil {
		return err
	}
	if target == id {
		return usageErrorf("slice-reorder: a slice cannot be placed beside itself")
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}

	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	t, _, err := loadSlice(ctx, st, target)
	if err != nil {
		return err
	}
	if s.Status == domain.SliceClaimed && s.MilestoneID != t.MilestoneID {
		return fmt.Errorf("%q is in progress: work in flight is not refiled under its agent", s.Name)
	}

	moved, to, err := st.ReorderSlice(ctx, shape, s.ID, t.ID, before)
	if err != nil {
		return fmt.Errorf("reorder the slice: %w", err)
	}
	env.nudged()

	refiled := s.MilestoneID != moved.MilestoneID
	if *asJSON {
		return writeJSON(env.Out, sliceReorderedJSON{
			ID: moved.ID, Name: moved.Name, URL: moved.URL,
			MilestoneID: moved.MilestoneID, Refiled: refiled,
			Placement: placementWord(before),
			RelativeTo: reorderedTargetJSON{
				ID: to.ID, Name: to.Name, URL: to.URL, MilestoneID: to.MilestoneID,
			},
		})
	}
	msg := fmt.Sprintf("# %s\n\nPlaced directly %s %s.", moved.Name, placementWord(before), to.Name)
	if refiled {
		msg += fmt.Sprintf(" Refiled under %s.", milestoneLabel(moved.MilestoneID))
	}
	_, err = fmt.Fprintln(env.Out, msg)
	return err
}

// milestoneLabel names a milestone for a sentence, the slice under none being
// said so rather than named by nothing.
func milestoneLabel(id string) string {
	if id == "" {
		return "no milestone"
	}
	return id
}

// sliceReorderedJSON is a reorder as a program reads it: the slice as it now
// stands, which side it landed on, whether it changed milestone to get there,
// and what it was placed beside.
type sliceReorderedJSON struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	URL         string              `json:"url,omitempty"`
	MilestoneID string              `json:"milestone_id"`
	Refiled     bool                `json:"refiled"`
	Placement   string              `json:"placement"`
	RelativeTo  reorderedTargetJSON `json:"relative_to"`
}

// reorderedTargetJSON is the slice a reorder placed another beside.
type reorderedTargetJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	MilestoneID string `json:"milestone_id"`
}
