package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// PRCreator is what the approve flow needs of the GitHub CLI: one pull request,
// from the branch an agent handed back, in the repository the slice belongs to,
// titled and bodied with what that agent wrote at hand-back. It is an interface
// so the flow can be driven without gh — or a network, or a GitHub account.
type PRCreator interface {
	CreatePR(dir, branch, title, body string) (string, error)
}

// The approve flow's edge, held as a variable so the tests can stand in for it:
// the real one shells out to gh.
var newPRCreator = defaultPRCreator

// defaultPRCreator is the real gh on PATH.
func defaultPRCreator() PRCreator { return gh.New() }

// prOpenedMsg reports the pull request gh opened for a handed-back slice, or
// the failure that stopped it. The write that records the URL is a second step,
// so that a gh that refused is reported as itself rather than as a Notion
// write that never happened.
type prOpenedMsg struct {
	slice domain.Slice
	url   string
	err   error
}

// approveNote is what the status bar says while gh is opening the pull request.
const approveNote = "Opening the pull request…"

// approveDiffFlow is the approve key on the review screen, which is the one
// place approving is offered: the diff on show is the work, and approving it is
// what reading it is answered with. It acts on the spot rather than asking to be
// confirmed — the screen is the confirmation, since nothing gets here without
// the change having been put in front of the user, which is exactly what a key
// on the board could not say.
//
// Only a handed-back slice is ever opened here — in progress with a branch
// recorded on it — so that rule is applied where the screen is opened rather
// than a second time here; what is left to refuse is a screen that has never
// been pointed at a branch, which has nothing to approve.
//
// Pending comments stop it. A review with something still to say is not one that
// approves the work, and the comments are held nowhere but this session: sending
// them is what empties them, and until it has they are what the key reports
// instead.
func (a *App) approveDiffFlow() tea.Cmd {
	if !a.canWrite() || a.prs == nil {
		return nil
	}
	name, branch, dir := a.diff.Target()
	if branch == "" {
		return nil
	}
	if n := a.diff.Pending(); n > 0 {
		return a.showToast(fmt.Sprintf("%d %s still pending — send them with s, or take them back, before approving.",
			n, plural(n, "comment", "comments")), sevWarning)
	}
	s := domain.Slice{ID: a.diff.SliceID(), Name: name, Branch: branch}
	// The review is over either way: the pull request is about to exist, and a
	// gh that refuses is read on the board like every other refusal — the
	// confirmation this ends in is anchored to the slice's row.
	a.setScreen(screenBoard)
	return a.startApprove(s, dir)
}

// startApprove is the approve itself: gh in the slice's repository, and the
// status bar saying so until it answers.
//
// The repository is checked here rather than left to gh, which would otherwise
// fail deep inside a subprocess over something the board already knows.
func (a *App) startApprove(s domain.Slice, dir string) tea.Cmd {
	dir = expandHome(strings.TrimSpace(dir))
	if err := existingDir(dir); err != nil {
		return a.showConfirm(fmt.Sprintf("Cannot open a pull request for %q: %v.", s.Name, err), sevError)
	}
	a.busy, a.note = true, approveNote
	return openPR(a.prs, a.client, s, dir)
}

// openPR runs gh in the slice's repository and reports the pull request it
// opened.
//
// The description the agent wrote at hand-back is read off the slice page
// first: it lives there rather than in the launch that put it there, so an
// approve days later opens the pull request with it. Its first line is the
// title and the rest the body; a page with no such section — every hand-back
// written before there was a flag for one — leaves both empty and gh fills the
// pull request from the commits, as it always did. A read that fails stops the
// approve rather than falling back, since a pull request opened with the wrong
// title is not one this key can open again.
func openPR(prs PRCreator, client NotionAPI, s domain.Slice, dir string) tea.Cmd {
	return func() tea.Msg {
		blocks, err := client.GetBlockChildren(context.Background(), s.ID)
		if err != nil {
			return prOpenedMsg{slice: s, err: fmt.Errorf("read the pull request description: %w", err)}
		}
		title, body := prTitleBody(notion.PRDescriptionOf(blocks))
		url, err := prs.CreatePR(dir, s.Branch, title, body)
		if err != nil {
			return prOpenedMsg{slice: s, err: err}
		}
		return prOpenedMsg{slice: s, url: url}
	}
}

// prTitleBody splits a recorded description into what gh is given: its first
// line as the title, everything after as the body. A description of one line is
// a title and no body, which is a perfectly good pull request; an empty one is
// no description at all, and both come back empty so the caller can let gh fill
// it instead.
func prTitleBody(description string) (title, body string) {
	title, body, _ = strings.Cut(strings.TrimSpace(description), "\n")
	return strings.TrimSpace(title), strings.TrimSpace(body)
}

// prOpened takes the pull request on to Notion, or reports why there is none.
//
// A gh that refused is a toast rather than an error banner: the branch is still
// there, the slice is still handed back, and the reason — a pull request that
// already exists, a branch never pushed, an unauthenticated gh — is something
// to read and act on outside nat rather than a state the board has to be
// dismissed out of.
func (a *App) prOpened(msg prOpenedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.busy, a.note = false, ""
		return a, a.showToast(fmt.Sprintf("Could not open a pull request for %q: %v", msg.slice.Name, msg.err), sevError)
	}
	// Still busy: the pull request exists but nothing records it yet, and the
	// write that does is the other half of the same action.
	return a, recordPR(a.client, msg.slice, msg.url)
}

// recordPR writes the pull request onto the slice and marks it Done, which is
// what approving the work means to the plan.
//
// The page is read first for the type of its Status column, which a project
// converted in the Notion UI may have changed under the app — the same read
// complete-slice makes for the same reason.
//
// Only this write can leave anything half done: a pull request opened and not
// recorded. Running the action again says so rather than opening a second one,
// because gh refuses a branch that already has a pull request.
//
// The slice's worktree stays exactly where it is. Approving is the review
// starting rather than the work ending: the pull request is open, and a review
// that asks for one more commit needs the checkout that commit is written in.
// What takes the worktree away is the merge — see [App.removeLanded].
func recordPR(client NotionAPI, s domain.Slice, url string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		page, err := client.GetPage(ctx, s.ID)
		if err != nil {
			return sliceSavedMsg{err: fmt.Errorf("record the pull request for %q: %w", s.Name, err)}
		}
		properties := map[string]notion.PropertyValue{
			notion.PropPR:     notion.NewURL(url),
			notion.PropStatus: notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceDone),
		}
		if _, err := client.UpdatePageProperties(ctx, s.ID, properties); err != nil {
			return sliceSavedMsg{err: fmt.Errorf("record the pull request for %q: %w", s.Name, err)}
		}
		return sliceSavedMsg{note: fmt.Sprintf("Opened the pull request for %q.", s.Name), sliceID: s.ID}
	}
}
