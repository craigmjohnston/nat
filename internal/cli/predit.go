package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
)

// PRBodyEditor is what pr-edit needs of the GitHub CLI: the body of a pull
// request already open replaced, in the slice's repository. It names exactly
// the one gh call this command makes, the way [PRCommenter] does for
// pr-comment.
type PRBodyEditor interface {
	EditPRBody(dir, ref, body string) error
}

// prEdit replaces the description of the pull request recorded on a slice —
// the body under its title, as gnat's PR section edits it in place.
//
// It is pr-comment line for line: only a slice with a pull request recorded
// has one to edit, and nothing is written to Notion — the slice's own
// `PR description` section is what the pull request was opened with, and stays
// the record of that.
func prEdit(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-edit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	body := flags.String("body", stdinRef, "the new description; `-` or absent reads it from stdin")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("pr-edit: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("pr-edit", rest[0])
	if err != nil {
		return err
	}
	// The description is settled before anything is read from Notion, so a
	// pr-edit whose stdin cannot be read fails having written nothing.
	text, err := editText(*body, env.In)
	if err != nil {
		return err
	}
	if text == "" {
		return usageErrorf("pr-edit: no description given: pass --body or pipe one in")
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	s, _, err := st.Slice(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	if s.PRURL == "" {
		return fmt.Errorf("%q has no pull request recorded: nothing to edit", s.Name)
	}

	workdir := actions.WorkdirFor(s, project)
	if err := env.NewGH().EditPRBody(workdir, s.PRURL, text); err != nil {
		return fmt.Errorf("edit the pull request %s: %w", s.PRURL, err)
	}

	if *asJSON {
		return writeJSON(env.Out, prEditedJSON{PR: s.PRURL})
	}
	_, err = fmt.Fprintf(env.Out, "# Description edited\n\n- PR: %s\n", s.PRURL)
	return err
}

// editText settles the new description: the flag as given, or stdin for the
// default "-" — [commentText]'s convention, kept apart only so a failed stdin
// read names a description rather than a comment.
func editText(body string, in io.Reader) (string, error) {
	if body != stdinRef {
		return strings.TrimSpace(body), nil
	}
	if in == nil {
		return "", nil
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read the description: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// prEditedJSON is the structured form of an edited description.
type prEditedJSON struct {
	PR string `json:"pr"`
}
