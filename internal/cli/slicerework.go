package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// sliceRework takes a handed-back slice back out of review: its Branch is
// emptied and nothing else on the page is touched, so the slice reads as in
// progress with an agent at work until that agent hands back again.
//
// It is the write behind approving over pending review comments: the comments
// go to the agent, this marks the work as being fixed, and the agent's own
// `complete-slice --branch` at the end of the fixes re-records the branch —
// the one event a caller can watch for to know the fixes are in.
//
// The PR description a hand-back filed stays on the page, which is where
// slice-approve reads it from when the hand-back that follows is approved.
func sliceRework(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-rework", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-rework: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-rework", rest[0])
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	// One error path for the three ways this can stop — the slice unreadable,
	// not handed back, or the write refused — each named by what it says.
	s, _, err := loadSlice(ctx, st, id)
	if err == nil && !s.HandedBack() {
		err = fmt.Errorf("%q is not handed back: only a slice with a branch waiting review can be sent back for rework", s.Name)
	}
	if err == nil {
		err = st.ClearBranch(ctx, s.ID)
	}
	if err != nil {
		return err
	}

	env.nudged()
	_, err = fmt.Fprintf(env.Out, "# %s\n\nSent back for rework. It reads as in progress until its next hand-back.\n", s.Name)
	return err
}
