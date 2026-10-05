package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
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
// --comments files what the review said under a Sent back heading, in the
// task log every store keeps, before ever clearing the branch — the same
// order a hand-back's own note goes on before its status write, and for the
// same reason: a slice already cleared back out of review reads, to the
// refusal every write here opens with, as never handed back at all, so the
// comments have to land first or they are lost rather than retried. Comments
// are optional, and an absent or empty value still files the heading — a
// slice sent back is an event in the log whether or not anything was said.
//
// The PR description a hand-back filed stays on the page, which is where
// slice-approve reads it from when the hand-back that follows is approved.
func sliceRework(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-rework", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	comments := flags.String("comments", "", "review comments to send back with the slice; `-` reads them from stdin")
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
	// Settled before anything is read from Notion, so a command whose stdin
	// cannot be read fails having written nothing.
	text, err := reworkCommentsText(*comments, env.In)
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

	// One error path for the ways this can stop — the slice unreadable, not
	// handed back, or either write refused — each named by what it says.
	s, _, err := loadSlice(ctx, st, id)
	if err == nil && !s.HandedBack() {
		err = fmt.Errorf("%q is not handed back: only a slice with a branch waiting review can be sent back for rework", s.Name)
	}
	if err == nil {
		err = actions.TakeBack(ctx, st, s.ID, func() error { return st.RecordSentBack(ctx, s.ID, text) })
	}
	if err != nil {
		return err
	}

	env.nudged()
	_, err = fmt.Fprintf(env.Out, "# %s\n\nSent back for rework. It reads as in progress until its next hand-back.\n", s.Name)
	return err
}

// reworkCommentsText settles --comments: the flag as given, or stdin where it
// is exactly "-" — the same convention [commentText] follows for pr-comment,
// except the flag's own default is "" rather than stdinRef, since comments
// here are optional and an absent flag should read nothing rather than block
// on a pipe nobody is feeding.
func reworkCommentsText(comments string, in io.Reader) (string, error) {
	if comments != stdinRef {
		return strings.TrimSpace(comments), nil
	}
	if in == nil {
		return "", nil
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read the comments: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
