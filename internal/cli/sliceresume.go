package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
)

// sliceResume takes the work on a handed-back slice back up ([actions.Resume]):
// a Resumed section carrying --note goes on its task log, and then its Branch
// is cleared, so it reads as work in progress until its agent's next
// `complete-slice --branch` records the branch again.
//
// It is what an agent runs when the user asks it for more after its
// hand-back, and what the app runs on the user's behalf before it reaches the
// agent — so, as with slice-rework, there is no ownership check beyond the
// status one. A slice not in progress is refused, a Done one by name; one
// already back at work writes nothing and still succeeds, so an agent that
// runs it twice leaves one record.
func sliceResume(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	noteFlag := flags.String("note", "", "why the work is taken back up — what the user asked for; `-` reads it from stdin")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-resume: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-resume", rest[0])
	if err != nil {
		return err
	}
	// Settled before anything is read, so a command whose note is missing or
	// whose stdin cannot be read fails having written nothing.
	note, err := briefText("slice-resume", "--note", *noteFlag, env.In)
	if err != nil {
		return err
	}
	if note == "" {
		return usageErrorf("slice-resume: no note given: say what the user asked for with --note")
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	wrote, err := actions.Resume(ctx, st, s, note)
	if err != nil {
		return err
	}
	if !wrote {
		_, err = fmt.Fprintf(env.Out, "# %s\n\nAlready in progress: nothing to resume, and nothing was written.\n", s.Name)
		return err
	}

	env.nudged()
	_, err = fmt.Fprintf(env.Out, "# %s\n\nResumed. It reads as in progress until its next hand-back.\n", s.Name)
	return err
}
