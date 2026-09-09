package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
)

// PRCommenter is what pr-comment needs of the GitHub CLI: one comment posted
// on a pull request already open, in the slice's repository. It names exactly
// the one gh call this command makes, the way [PRViewer] does for pr-view.
type PRCommenter interface {
	CommentPR(dir, ref, body string) (string, error)
}

// prComment posts a comment on the pull request recorded on a slice — a note
// left on it without leaving nat, from the same place every other read and
// write on a slice's pull request happens.
//
// Only a slice with a pull request recorded has one to comment on, the same
// refusal pr-view and pr-merge give a slice with none. Nothing is written to
// Notion by this: the comment lands on GitHub alone, exactly where a reviewer
// leaving it by hand would have put it.
func prComment(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-comment", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	body := flags.String("body", stdinRef, "the comment text; `-` or absent reads it from stdin")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("pr-comment: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("pr-comment", rest[0])
	if err != nil {
		return err
	}
	// The comment is settled before anything is read from Notion, so a
	// pr-comment whose stdin cannot be read fails having posted nothing.
	text, err := commentText(*body, env.In)
	if err != nil {
		return err
	}
	if text == "" {
		return usageErrorf("pr-comment: no comment given: pass --body or pipe one in")
	}

	_, _, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	page, err := client.GetPage(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	s := domain.SliceFromPage(*page)
	if s.PRURL == "" {
		return fmt.Errorf("%q has no pull request recorded: nothing to comment on", s.Name)
	}

	workdir := actions.WorkdirFor(s, project)
	url, err := env.NewGH().CommentPR(workdir, s.PRURL, text)
	if err != nil {
		return fmt.Errorf("comment on the pull request %s: %w", s.PRURL, err)
	}

	if *asJSON {
		return writeJSON(env.Out, prCommentedJSON{PR: s.PRURL, CommentURL: url})
	}
	_, err = io.WriteString(env.Out, prCommentedMarkdown(s.PRURL, url))
	return err
}

// commentText settles the comment body: the flag as it was given, or stdin for
// the default "-", which is how a comment longer than a shell argument gets
// in — the same convention [briefText] follows, kept apart from it only so
// the error a failed stdin read gives names a comment rather than a
// description.
func commentText(body string, in io.Reader) (string, error) {
	if body != stdinRef {
		return strings.TrimSpace(body), nil
	}
	if in == nil {
		return "", nil
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read the comment: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// prCommentedJSON is the structured form of a posted comment.
type prCommentedJSON struct {
	PR         string `json:"pr"`
	CommentURL string `json:"comment_url,omitempty"`
}

// prCommentedMarkdown reports the comment as posted.
func prCommentedMarkdown(pr, commentURL string) string {
	var b strings.Builder
	b.WriteString("# Comment posted\n\n")
	fmt.Fprintf(&b, "- PR: %s\n", pr)
	if commentURL != "" {
		fmt.Fprintf(&b, "- Comment: %s\n", commentURL)
	}
	return b.String()
}
