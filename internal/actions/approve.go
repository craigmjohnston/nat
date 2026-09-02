package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// PRCreator is what an approve needs of the GitHub CLI: one pull request,
// from the branch an agent handed back, in the repository the slice belongs
// to, titled and bodied with what that agent wrote at hand-back. It is an
// interface so the flow can be driven without gh — or a network, or a GitHub
// account.
type PRCreator interface {
	CreatePR(dir, branch, title, body string) (string, error)
}

// OpenPR runs gh in the slice's repository and returns the pull request it
// opened.
//
// The description the agent wrote at hand-back is read off the slice page
// first: it lives there rather than in the launch that put it there, so an
// approve days later opens the pull request with it. Its first line is the
// title and the rest the body; a page with no such section — every hand-back
// written before there was a flag for one — leaves both empty and gh fills the
// pull request from the commits, as it always did. A read that fails stops
// the approve rather than falling back, since a pull request opened with the
// wrong title is not one this can open again.
func OpenPR(ctx context.Context, client Client, prs PRCreator, s domain.Slice, dir string) (string, error) {
	blocks, err := client.GetBlockChildren(ctx, s.ID)
	if err != nil {
		return "", fmt.Errorf("read the pull request description: %w", err)
	}
	title, body := PRTitleBody(notion.PRDescriptionOf(blocks))
	return prs.CreatePR(dir, s.Branch, title, body)
}

// PRTitleBody splits a recorded description into what gh is given: its first
// line as the title, everything after as the body. A description of one line
// is a title and no body, which is a perfectly good pull request; an empty
// one is no description at all, and both come back empty so the caller can
// let gh fill it instead.
func PRTitleBody(description string) (title, body string) {
	title, body, _ = strings.Cut(strings.TrimSpace(description), "\n")
	return strings.TrimSpace(title), strings.TrimSpace(body)
}

// RecordPR writes the pull request onto the slice and marks it Done, which is
// what approving the work means to the plan.
//
// The page is read first for the type of its Status column, which a project
// converted in the Notion UI may have changed under the app — the same read
// complete-slice makes for the same reason.
//
// Only this write can leave anything half done: a pull request opened and
// not recorded. Running the action again says so rather than opening a
// second one, because gh refuses a branch that already has a pull request.
//
// The slice's worktree stays exactly where it is. Approving is the review
// starting rather than the work ending: the pull request is open, and a
// review that asks for one more commit needs the checkout that commit is
// written in. What takes the worktree away is the merge.
func RecordPR(ctx context.Context, client Client, s domain.Slice, url string) error {
	page, err := client.GetPage(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("record the pull request for %q: %w", s.Name, err)
	}
	properties := map[string]notion.PropertyValue{
		notion.PropPR:     notion.NewURL(url),
		notion.PropStatus: notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceDone),
	}
	if _, err := client.UpdatePageProperties(ctx, s.ID, properties); err != nil {
		return fmt.Errorf("record the pull request for %q: %w", s.Name, err)
	}
	return nil
}
