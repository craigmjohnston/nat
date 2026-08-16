package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
)

// Differ is what the review screen needs of git: the change a slice's branch
// makes to its repository, and the base it was measured against. It is an
// interface so the flow can be driven without git, or a repository.
type Differ interface {
	Diff(dir, branch string) (base, diff string, err error)
}

// The review screen's edge, held as a variable so the tests can stand in for
// it: the real one shells out to git.
var newDiffer = defaultDiffer

// defaultDiffer is the real git on PATH.
func defaultDiffer() Differ { return git.New() }

// diffLoadedMsg carries the diff of a handed-back branch, already split into
// its files, or the failure that stopped it. The screen reports its own
// failures, so they do not go through notionErrMsg — nothing about this one
// came from Notion.
type diffLoadedMsg struct {
	base  string
	files []git.File
	err   error
}

// diffSliceFlow opens the review screen on the slice the cursor is on: the diff
// of the branch its agent handed back, against the base that branch was cut
// from.
//
// Only a handed-back slice has one — in progress with a branch recorded on it —
// which is the same rule the approve key applies, and for the same reason: a
// branch is what there is to read, and reading it is what the approve key is
// answered with.
func (a *App) diffSliceFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.differ == nil {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to read its diff.", sevWarning)
	}
	if s.Status != domain.SliceClaimed {
		return a.showConfirm(fmt.Sprintf("%q is %s — only a handed-back slice has a branch to read.",
			s.Name, statusWord(s)), sevWarning)
	}
	if s.Branch == "" {
		return a.showConfirm(fmt.Sprintf("%q has no branch handed back yet — there is nothing to read.",
			s.Name), sevWarning)
	}
	dir := expandHome(strings.TrimSpace(workdirFor(s, project)))
	// The repository is checked here rather than left to git, which would
	// otherwise fail deep inside a subprocess over something the board knows.
	if err := existingDir(dir); err != nil {
		return a.showConfirm(fmt.Sprintf("Cannot read the diff for %q: %v.", s.Name, err), sevError)
	}
	a.diff.Start(s.Name, s.Branch, dir)
	a.setScreen(screenDiff)
	return tea.Batch(a.spinner.Tick, readDiff(a.differ, s.Branch, dir))
}

// startDiffLoad reads the branch the screen is already showing again, which is
// what the refresh key does while it is up: an agent that pushes another commit
// is exactly when a second read is wanted. It returns nil when the screen has
// never been pointed at a branch, and so has nothing to read again.
func (a *App) startDiffLoad() tea.Cmd {
	if a.differ == nil || !a.diff.Loadable() {
		return nil
	}
	slice, branch, dir := a.diff.Target()
	a.diff.Start(slice, branch, dir)
	return tea.Batch(a.spinner.Tick, readDiff(a.differ, branch, dir))
}

// readDiff runs git in the slice's repository and splits what it wrote into
// files.
func readDiff(differ Differ, branch, dir string) tea.Cmd {
	return func() tea.Msg {
		base, diff, err := differ.Diff(dir, branch)
		if err != nil {
			return diffLoadedMsg{base: base, err: fmt.Errorf("read the diff of %s: %w", branch, err)}
		}
		return diffLoadedMsg{base: base, files: git.ParseFiles(diff)}
	}
}

// diffHeading is what the header calls the review screen: the branch it is
// showing, which is what a diff is of. Before one has been asked for there is
// only the screen's own name.
func (a *App) diffHeading() string {
	_, branch, _ := a.diff.Target()
	if branch == "" {
		return "Diff"
	}
	return "Diff " + branch
}

// diffLoaded shows the diff that came back, or the failure that came instead.
func (a *App) diffLoaded(msg diffLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.diff.Fail(msg.err)
		return a, nil
	}
	a.diff.SetFiles(msg.base, msg.files)
	return a, nil
}
