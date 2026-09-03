package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

// PRMerger is what pr-merge needs of the GitHub CLI: the pull request merged,
// in the repository the slice belongs to. It names exactly the one gh call
// this command makes, the way [PRViewer] does for pr-view.
type PRMerger interface {
	MergePR(dir, ref string) error
}

// prMerge merges a slice's recorded pull request: the board's merge key
// without the board. Nothing is written to Notion by any of this — a merged
// pull request is the work landing, and the slice was marked Done as the pull
// request was opened.
//
// The refusal is the merge box's own: the pull request is read again first,
// and a review not yet approved, a check still failing or a branch conflicting
// with its base refuses in the same words the board would show, before gh is
// ever asked to attempt the merge. A pull request already merged or closed has
// nothing left to merge, whatever the verdicts say.
func prMerge(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-merge", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("pr-merge: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("pr-merge", rest[0])
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
	if s.PRURL == "" {
		return fmt.Errorf("%q has no pull request recorded: nothing to merge", s.Name)
	}

	workdir := actions.WorkdirFor(s, project)
	ghClient := env.NewGH()
	pr, err := ghClient.ViewPR(workdir, s.PRURL)
	if err != nil {
		return fmt.Errorf("read the pull request %s: %w", s.PRURL, err)
	}
	if pr.State == gh.PRStateMerged || pr.State == gh.PRStateClosed {
		return fmt.Errorf("#%d is %s: nothing to merge", pr.Number, prStateWord(pr))
	}
	if reason, refused := actions.MergeRefusal(pr); refused {
		return fmt.Errorf("cannot merge #%d — %s", pr.Number, reason)
	}

	if err := ghClient.MergePR(workdir, s.PRURL); err != nil {
		return fmt.Errorf("merge #%d: %w", pr.Number, err)
	}

	if *asJSON {
		return writeMergedJSON(env.Out)
	}
	_, err = io.WriteString(env.Out, fmt.Sprintf("# Merged\n\nMerged #%d.\n\n- PR: %s\n", pr.Number, s.PRURL))
	return err
}

// mergedJSON is the structured form of a successful merge.
type mergedJSON struct {
	Merged bool `json:"merged"`
}

// writeMergedJSON encodes the merge result.
func writeMergedJSON(out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(mergedJSON{Merged: true})
}
