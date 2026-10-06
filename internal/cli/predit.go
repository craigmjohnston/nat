package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
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
// has one to edit, --session names one of an ad hoc session's instead
// ([prTarget]), and nothing is written to Notion — the slice's own
// `PR description` section is what the pull request was opened with, and stays
// the record of that.
func prEdit(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-edit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	body := flags.String("body", stdinRef, "the new description; `-` or absent reads it from stdin")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	sessionID := flags.String("session", "",
		"edit one of this ad hoc session's pull requests, named by URL or number, instead of a slice's")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	target, err := parsePRTarget("pr-edit", *sessionID, rest)
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

	workdir, pr, err := target.resolve(ctx, env, *projectRef, "edit")
	if err != nil {
		return err
	}
	if err := env.NewGH().EditPRBody(workdir, pr, text); err != nil {
		return fmt.Errorf("edit the pull request %s: %w", pr, err)
	}

	if *asJSON {
		return writeJSON(env.Out, prEditedJSON{PR: pr})
	}
	_, err = fmt.Fprintf(env.Out, "# Description edited\n\n- PR: %s\n", pr)
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
