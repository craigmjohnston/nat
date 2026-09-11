package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
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
func OpenPR(ctx context.Context, st Store, prs PRCreator, s domain.Slice, dir string) (string, error) {
	description, err := st.PRDescription(ctx, s.ID)
	if err != nil {
		return "", fmt.Errorf("read the pull request description: %w", err)
	}
	title, body := PRTitleBody(description)
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

// RecordPR writes the pull request onto the slice — and nothing else. The
// slice stays in progress: Done means the work is on main, and a pull request
// just opened is a review still running, so what moves the status is the
// merge — [MarkDone], written by nat's own merge or by the reading that finds
// GitHub already made one.
//
// Only this write can leave anything half done: a pull request opened and
// not recorded. Running the action again says so rather than opening a
// second one, because gh refuses a branch that already has a pull request.
//
// The slice's worktree stays exactly where it is. Approving is the review
// starting rather than the work ending: the pull request is open, and a
// review that asks for one more commit needs the checkout that commit is
// written in. What takes the worktree away is the merge.
func RecordPR(ctx context.Context, st Store, s domain.Slice, url string) error {
	if err := st.RecordPR(ctx, s.ID, url); err != nil {
		return fmt.Errorf("record the pull request for %q: %w", s.Name, err)
	}
	return nil
}
