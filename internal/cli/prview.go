package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
)

// PRViewer is what pr-view needs of the GitHub CLI: one pull request in full,
// read in the slice's repository. It is an interface for the reason
// [actions.PRCreator] is — a test can drive the command without a real gh on
// PATH — and it names exactly the one gh call this command makes, the way
// [internal/tui.PRViewer] does for the board's own v key.
type PRViewer interface {
	ViewPR(dir, ref string) (gh.PR, error)
}

// prView prints one pull request in full: the board's v key without the
// board. Only a slice with a pull request recorded has anything to read —
// the same refusal slice-approve and slice-diff give a slice with no branch
// or PR — and the read itself is gh's, in the slice's repository.
func prView(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-view", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("pr-view: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("pr-view", rest[0])
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
		return fmt.Errorf("%q has no pull request recorded: nothing to view", s.Name)
	}

	workdir := actions.WorkdirFor(s, project)
	pr, err := env.NewGH().ViewPR(workdir, s.PRURL)
	if err != nil {
		return fmt.Errorf("read the pull request %s: %w", s.PRURL, err)
	}

	if *asJSON {
		return writeJSON(env.Out, prJSON(pr))
	}
	_, err = io.WriteString(env.Out, prMarkdown(pr))
	return err
}

// checkJSON is one entry of the status check rollup.
type checkJSON struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Link  string `json:"link"`
}

// reviewJSON is one review left on the pull request.
type reviewJSON struct {
	Author      string    `json:"author"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	SubmittedAt time.Time `json:"submitted_at,omitempty"`
}

// commentJSON is one comment left on the pull request's own conversation.
type commentJSON struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	URL       string    `json:"url"`
}

// prDoc is the structured form of a pull request, gh's fields kept as gh
// wrote them — GitHub's own vocabulary rather than a word this command
// invented, exactly as [gh.PR] itself does.
type prDoc struct {
	Number           int           `json:"number"`
	Title            string        `json:"title"`
	Body             string        `json:"body"`
	State            string        `json:"state"`
	IsDraft          bool          `json:"is_draft"`
	Author           string        `json:"author"`
	BaseRefName      string        `json:"base_ref_name"`
	HeadRefName      string        `json:"head_ref_name"`
	URL              string        `json:"url"`
	Checks           []checkJSON   `json:"checks"`
	Reviews          []reviewJSON  `json:"reviews"`
	Comments         []commentJSON `json:"comments"`
	ReviewDecision   string        `json:"review_decision"`
	Mergeable        string        `json:"mergeable"`
	MergeStateStatus string        `json:"merge_state_status"`
	// Additions, Deletions, ChangedFiles and Commits are the pull request's
	// change stats — gh's own numbers, passed through as numbers rather than
	// turned into a summary sentence.
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changed_files"`
	Commits      int `json:"commits"`
}

// prJSON maps gh's answer onto the structured form, keeping every field it
// carries rather than the subset any one command draws.
func prJSON(pr gh.PR) prDoc {
	doc := prDoc{
		Number:           pr.Number,
		Title:            pr.Title,
		Body:             pr.Body,
		State:            pr.State,
		IsDraft:          pr.IsDraft,
		Author:           pr.Author,
		BaseRefName:      pr.BaseRefName,
		HeadRefName:      pr.HeadRefName,
		URL:              pr.URL,
		Checks:           make([]checkJSON, 0, len(pr.Checks)),
		Reviews:          make([]reviewJSON, 0, len(pr.Reviews)),
		Comments:         make([]commentJSON, 0, len(pr.Comments)),
		ReviewDecision:   pr.ReviewDecision,
		Mergeable:        pr.Mergeable,
		MergeStateStatus: pr.MergeStateStatus,
		Additions:        pr.Additions,
		Deletions:        pr.Deletions,
		ChangedFiles:     pr.ChangedFiles,
		Commits:          pr.Commits,
	}
	for _, c := range pr.Checks {
		doc.Checks = append(doc.Checks, checkJSON{Name: c.Name, State: c.State, Link: c.URL})
	}
	for _, r := range pr.Reviews {
		doc.Reviews = append(doc.Reviews, reviewJSON{
			Author: r.Author, State: r.State, Body: r.Body, SubmittedAt: r.SubmittedAt,
		})
	}
	for _, c := range pr.Comments {
		doc.Comments = append(doc.Comments, commentJSON{
			Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt, URL: c.URL,
		})
	}
	return doc
}

// prMarkdown renders the pull request as markdown: what it is, then what
// stands between it and its base, then the conversation.
func prMarkdown(pr gh.PR) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# #%d %s\n\n", pr.Number, pr.Title)
	fmt.Fprintf(&b, "- State: %s\n", prStateWord(pr))
	fmt.Fprintf(&b, "- Author: %s\n", blank(pr.Author))
	fmt.Fprintf(&b, "- Branch: %s → %s\n", pr.HeadRefName, pr.BaseRefName)
	fmt.Fprintf(&b, "- URL: %s\n", pr.URL)
	fmt.Fprintf(&b, "- Review: %s\n", blank(pr.ReviewDecision))
	fmt.Fprintf(&b, "- Mergeable: %s\n", blank(pr.Mergeable))
	fmt.Fprintf(&b, "- Merge state: %s\n", blank(pr.MergeStateStatus))

	b.WriteString("\n## Description\n\n")
	if desc := strings.TrimSpace(pr.Body); desc != "" {
		fmt.Fprintf(&b, "%s\n", desc)
	} else {
		b.WriteString("_none_\n")
	}

	b.WriteString("\n## Checks\n\n")
	if len(pr.Checks) == 0 {
		b.WriteString("_none_\n")
	}
	for _, c := range pr.Checks {
		fmt.Fprintf(&b, "- %s: %s\n", c.Name, blank(c.State))
	}

	b.WriteString("\n## Reviews\n\n")
	if len(pr.Reviews) == 0 {
		b.WriteString("_none_\n")
	}
	for _, r := range pr.Reviews {
		fmt.Fprintf(&b, "- %s (%s): %s\n", r.Author, r.State, r.Body)
	}

	b.WriteString("\n## Comments\n\n")
	if len(pr.Comments) == 0 {
		b.WriteString("_none_\n")
	}
	for _, c := range pr.Comments {
		fmt.Fprintf(&b, "- %s: %s\n", c.Author, c.Body)
	}
	return b.String()
}

// prStateWord is the pull request's own state, in the same four words the
// board's chip draws: draft is tested against the state as well, since one
// merged or closed is no longer a draft whatever the flag says.
func prStateWord(pr gh.PR) string {
	switch {
	case pr.State == gh.PRStateMerged:
		return "merged"
	case pr.State == gh.PRStateClosed:
		return "closed"
	case pr.IsDraft:
		return "draft"
	default:
		return "open"
	}
}
