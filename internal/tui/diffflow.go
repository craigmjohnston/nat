package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
)

// Differ is what the review screen needs of git: the change a slice's branch
// makes to its repository, the base it was measured against, and the lines of
// each file it touches, which is what fills the gaps the diff leaves between its
// hunks. It is an interface so the flow can be driven without git, or a
// repository.
type Differ interface {
	Diff(dir, branch string) (base, diff string, err error)
	Show(dir, branch, path string) ([]string, error)
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
	base    string
	files   []git.File
	sources map[string][]string
	err     error
}

// diffSliceFlow opens the review screen on the slice the cursor is on: the diff
// of the branch its agent handed back, against the base that branch was cut
// from.
//
// Only a handed-back slice has one — in progress with a branch recorded on it —
// and this is now the one place that rule is applied, since approving is the
// review screen's own key: a branch is what there is to read, and approving it
// is what reading it is answered with.
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
	a.diff.Start(s.ID, s.Name, s.Branch, dir)
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
	a.diff.Start(a.diff.SliceID(), slice, branch, dir)
	return tea.Batch(a.spinner.Tick, readDiff(a.differ, branch, dir))
}

// readDiff runs git in the slice's repository, splits what it wrote into files,
// and reads each of those files at the branch, which is what the expand zones
// around the change are drawn from.
func readDiff(differ Differ, branch, dir string) tea.Cmd {
	return func() tea.Msg {
		base, diff, err := differ.Diff(dir, branch)
		if err != nil {
			return diffLoadedMsg{base: base, err: fmt.Errorf("read the diff of %s: %w", branch, err)}
		}
		files := git.ParseFiles(diff)
		return diffLoadedMsg{base: base, files: files,
			sources: readSources(differ, branch, dir, files)}
	}
}

// readSources is each file's own lines at the branch, by path — the whole of a
// file rather than the few lines around each change, so the gaps between the
// hunks have somewhere to be filled from.
//
// A file git will not show is passed over rather than failing the read: a file
// the change deleted is not on the branch at all, a binary one is not lines, and
// what either costs is the expanding around one file's diff and not the diff.
// git logs the refusal itself.
func readSources(differ Differ, branch, dir string, files []git.File) map[string][]string {
	sources := make(map[string][]string, len(files))
	for _, f := range files {
		if f.Binary || f.Path == "" || sources[f.Path] != nil {
			continue
		}
		if lines, err := differ.Show(dir, branch, f.Path); err == nil {
			sources[f.Path] = lines
		}
	}
	return sources
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
//
// A read that moved the lines a pending comment was left on takes that comment
// with it, which is said out loud: the comments are held nowhere else, and one
// that vanished from the gutter without a word would be one the user goes on
// believing they have left.
func (a *App) diffLoaded(msg diffLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.diff.Fail(msg.err)
		return a, nil
	}
	if dropped := a.diff.SetFiles(msg.base, msg.files, msg.sources); dropped > 0 {
		return a, a.showToast(fmt.Sprintf("%d %s dropped — the lines they were on have changed.",
			dropped, plural(dropped, "comment", "comments")), sevWarning)
	}
	return a, nil
}
