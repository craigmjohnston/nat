package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// PRCreator is what the approve flow needs of the GitHub CLI: one pull request,
// from the branch an agent handed back, in the repository the slice belongs to.
// It is an interface so the flow can be driven without gh — or a network, or a
// GitHub account.
type PRCreator interface {
	CreatePR(dir, branch string) (string, error)
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
//
// The repository is carried along because the step after the write runs there
// too: the worktree the slice's agent was given is taken off the same checkout
// gh was run in.
type prOpenedMsg struct {
	slice domain.Slice
	dir   string
	url   string
	err   error
}

// approveNote is what the status bar says while gh is opening the pull request.
const approveNote = "Opening the pull request…"

// The choices the approve prompt offers, in the order they read. Approving
// comes first, so a single enter opens the pull request; the way out is beside
// it as well as on esc, because this is the one board action that reaches
// outside Notion and cannot be taken back from here.
var approveChoices = []string{"approve", "cancel"}

const (
	choiceApprove = iota
	choiceCancelApprove
)

// approveSliceFlow anchors the approve prompt to the slice the cursor is on.
// Only a handed-back slice can be approved: one still in progress with a branch
// recorded on it, which is work waiting to be reviewed. A Todo slice has had no
// agent on it, a Done one is finished, and one in progress with no branch is
// still being worked.
func (a *App) approveSliceFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || !a.canWrite() || a.prs == nil {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to approve it.", sevWarning)
	}
	if s.Status != domain.SliceClaimed {
		return a.showConfirm(fmt.Sprintf("%q is %s — only a handed-back slice can be approved.",
			s.Name, statusWord(s)), sevWarning)
	}
	if s.Branch == "" {
		return a.showConfirm(fmt.Sprintf("%q has no branch handed back yet — nothing to open a pull request from.",
			s.Name), sevWarning)
	}
	dir := workdirFor(s, project)
	return a.openPrompt(approveChoices, func(choice int) tea.Cmd {
		return a.approveChosen(s, dir, choice)
	})
}

// approveChosen is what answering the prompt does. Cancelling says nothing —
// nothing was in flight, and the row is as it was.
//
// The repository is checked here rather than left to gh, which would otherwise
// fail deep inside a subprocess over something the board already knows.
func (a *App) approveChosen(s domain.Slice, dir string, choice int) tea.Cmd {
	if choice != choiceApprove {
		return nil
	}
	dir = expandHome(strings.TrimSpace(dir))
	if err := existingDir(dir); err != nil {
		return a.showConfirm(fmt.Sprintf("Cannot open a pull request for %q: %v.", s.Name, err), sevError)
	}
	a.busy, a.note = true, approveNote
	return openPR(a.prs, s, dir)
}

// removeWorktree takes the slice's worktree off its repository, once the pull
// request exists and the slice is Done.
//
// Everything it can fail on is dropped after a line in the log: a worktree with
// uncommitted changes in it, a slice whose agent never had one, a machine with
// no worktrunk at all. None of them is worth a toast — the approve has already
// happened and cannot be taken back — and none of them loses any work, since
// worktrunk refuses a dirty worktree outright and deletes the branch only where
// it has been merged. gh was run in the shared checkout, so nothing here can
// pull the ground out from under it either.
func removeWorktree(w Worktrees, dir, branch string) {
	if err := w.Remove(dir, branch); err != nil {
		// worktrunk's own failure is already in the log; this is the decision
		// taken about it.
		logging.Action("left the slice's worktree in place", "dir", dir, "branch", branch, "error", err)
	}
}

// openPR runs gh in the slice's repository and reports the pull request it
// opened.
func openPR(prs PRCreator, s domain.Slice, dir string) tea.Cmd {
	return func() tea.Msg {
		url, err := prs.CreatePR(dir, s.Branch)
		if err != nil {
			return prOpenedMsg{slice: s, dir: dir, err: err}
		}
		return prOpenedMsg{slice: s, dir: dir, url: url}
	}
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
	return a, recordPR(a.client, newWorktrees(), msg.slice, msg.dir, msg.url)
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
// The slice's worktree goes once that write has landed, and only then: the
// checkout is what the branch was written in, and a slice still handed back is
// one whose work is still being reviewed.
func recordPR(client NotionAPI, w Worktrees, s domain.Slice, dir, url string) tea.Cmd {
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
		removeWorktree(w, dir, s.Branch)
		return sliceSavedMsg{note: fmt.Sprintf("Opened the pull request for %q.", s.Name), sliceID: s.ID}
	}
}
