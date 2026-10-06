package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
)

// PRReviewerEditor is what pr-reviewers needs of the GitHub CLI beyond
// reading the pull request: who could review in the repository, and asking
// or un-asking them. It names exactly those two gh calls, the way
// [PRCommenter] does for pr-comment.
type PRReviewerEditor interface {
	EditReviewers(dir, ref string, add, remove []string) error
	Collaborators(dir string) ([]string, error)
}

// prReviewers reads, or with --add/--remove edits, who is asked to review
// the pull request recorded on a slice — assigning a reviewer without
// leaving nat. A read answers with who is requested and who else could be:
// the repository's collaborators bar the author and those already requested.
// An edit answers with the edit's own result — who it asked and who it
// stopped asking, which gh accepted — and reads nothing back: the next
// reading of the pull request (gnat's settle read, pr-view) says who is
// requested now, and a read here would spend a GitHub call to say it sooner.
//
// A collaborator listing that fails concludes nothing — it is reported as
// candidates_error beside an empty list, never as "nobody could review",
// and never fails a read that otherwise worked.
func prReviewers(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("pr-reviewers", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var add, remove repeatedFlag
	flags.Var(&add, "add", "ask this login (or org/team) to review; repeatable")
	flags.Var(&remove, "remove", "withdraw the request to this login (or org/team); repeatable")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("pr-reviewers: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("pr-reviewers", rest[0])
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
	s, _, err := st.Slice(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	if s.PRURL == "" {
		return fmt.Errorf("%q has no pull request recorded: nobody to ask to review", s.Name)
	}

	workdir := actions.WorkdirFor(s, project)
	client := env.NewGH()
	if len(add) > 0 || len(remove) > 0 {
		if err := client.EditReviewers(workdir, s.PRURL, add, remove); err != nil {
			return fmt.Errorf("edit the reviewers of %s: %w", s.PRURL, err)
		}
		edit := prReviewersEditJSON{PR: s.PRURL, Added: nonNil(add), Removed: nonNil(remove)}
		if *asJSON {
			return writeJSON(env.Out, edit)
		}
		_, err = io.WriteString(env.Out, prReviewersEditMarkdown(edit))
		return err
	}
	pr, err := client.ViewPR(workdir, s.PRURL)
	if err != nil {
		return fmt.Errorf("read the pull request %s: %w", s.PRURL, err)
	}

	doc := prReviewersJSON{PR: s.PRURL, Requested: nonNil(pr.ReviewRequests), Candidates: []string{}}
	collaborators, err := client.Collaborators(workdir)
	if err != nil {
		doc.CandidatesError = err.Error()
	}
	for _, login := range collaborators {
		if login != pr.Author && !slices.Contains(pr.ReviewRequests, login) {
			doc.Candidates = append(doc.Candidates, login)
		}
	}

	if *asJSON {
		return writeJSON(env.Out, doc)
	}
	_, err = io.WriteString(env.Out, prReviewersMarkdown(doc))
	return err
}

// repeatedFlag collects every use of a flag given more than once, a comma
// in any one of them splitting it further, as gh's own --add-reviewer does.
type repeatedFlag []string

func (r *repeatedFlag) String() string { return strings.Join(*r, ",") }

func (r *repeatedFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*r = append(*r, part)
		}
	}
	return nil
}

func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// prReviewersJSON is the structured form of who reviews a pull request.
type prReviewersJSON struct {
	PR              string   `json:"pr"`
	Requested       []string `json:"requested"`
	Candidates      []string `json:"candidates"`
	CandidatesError string   `json:"candidates_error,omitempty"`
}

// prReviewersEditJSON is the structured form of an edit: who it asked to
// review and who it stopped asking.
type prReviewersEditJSON struct {
	PR      string   `json:"pr"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// prReviewersEditMarkdown reports what an edit did.
func prReviewersEditMarkdown(edit prReviewersEditJSON) string {
	var b strings.Builder
	b.WriteString("# Reviewers\n\n")
	fmt.Fprintf(&b, "- PR: %s\n", edit.PR)
	fmt.Fprintf(&b, "- Asked: %s\n", noneIfEmpty(edit.Added))
	fmt.Fprintf(&b, "- No longer asked: %s\n", noneIfEmpty(edit.Removed))
	return b.String()
}

// prReviewersMarkdown reports who is requested and who else could be.
func prReviewersMarkdown(doc prReviewersJSON) string {
	var b strings.Builder
	b.WriteString("# Reviewers\n\n")
	fmt.Fprintf(&b, "- PR: %s\n", doc.PR)
	fmt.Fprintf(&b, "- Requested: %s\n", noneIfEmpty(doc.Requested))
	if doc.CandidatesError != "" {
		fmt.Fprintf(&b, "- Could also ask: unknown (%s)\n", doc.CandidatesError)
	} else {
		fmt.Fprintf(&b, "- Could also ask: %s\n", noneIfEmpty(doc.Candidates))
	}
	return b.String()
}

// noneIfEmpty joins logins for a markdown line, or says there are none.
func noneIfEmpty(logins []string) string {
	if len(logins) == 0 {
		return "none"
	}
	return strings.Join(logins, ", ")
}
