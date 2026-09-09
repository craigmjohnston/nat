package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceApprove opens a pull request for a handed-back branch and records it
// on the slice — the same two-step write the board's approve key makes:
// [actions.OpenPR] against gh, then [actions.RecordPR] against Notion.
func sliceApprove(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-approve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-approve: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-approve", rest[0])
	if err != nil {
		return err
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
	// Only a handed-back slice — in progress with a branch recorded on it —
	// is one there is a pull request to open for.
	if s.Branch == "" {
		return fmt.Errorf("%q is not handed back: only a slice with a branch waiting review can be approved", s.Name)
	}
	if s.Status == domain.SliceDone {
		return fmt.Errorf("%q is already Done: approve is for handed-back work", s.Name)
	}

	workdir := s.Repo
	if workdir == "" {
		workdir = project.WorkingDir
	}

	url, err := actions.OpenPR(ctx, client, env.NewGH(), s, workdir)
	if err != nil {
		return err
	}
	if err := actions.RecordPR(ctx, client, s, url); err != nil {
		return err
	}

	env.nudged()
	if *asJSON {
		return writeApproveJSON(env.Out, url)
	}
	_, err = io.WriteString(env.Out, approveMarkdown(url))
	return err
}

// approveJSON is the structured form of the approve output.
type approveJSON struct {
	URL string `json:"url"`
}

// writeApproveJSON encodes the approve result.
func writeApproveJSON(out io.Writer, url string) error {
	doc := approveJSON{URL: url}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// approveMarkdown renders the approve result.
func approveMarkdown(url string) string {
	return fmt.Sprintf("# Pull request opened\n\n- URL: %s\n", url)
}
