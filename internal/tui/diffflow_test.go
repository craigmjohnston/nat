package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
)

// diffCall is one diff the review flow asked git for.
type diffCall struct{ dir, branch string }

// showCall is one file the review flow asked git to show at the branch.
type showCall struct{ dir, branch, path string }

// fakeDiffer stands in for git: it records what it was asked to diff and
// answers with the diff — or the refusal — the test wants git to have given.
// It shows no file by default, which is a git that refused every one of them:
// the diff still comes back, with no expand zones around it.
type fakeDiffer struct {
	base  string
	out   string
	err   error
	files map[string][]string
	made  []diffCall
	shown []showCall
}

var _ Differ = (*fakeDiffer)(nil)

func (f *fakeDiffer) Diff(dir, branch string) (string, string, error) {
	f.made = append(f.made, diffCall{dir, branch})
	return f.base, f.out, f.err
}

func (f *fakeDiffer) Show(dir, branch, path string) ([]string, error) {
	f.shown = append(f.shown, showCall{dir, branch, path})
	lines, ok := f.files[path]
	if !ok {
		return nil, errors.New("fatal: path does not exist in " + branch)
	}
	return lines, nil
}

// diffApp returns an app showing the hand-back plan with a fake git wired in,
// sized so the screen has a band to draw in, and the working directory the
// flow should resolve to.
func diffApp(t *testing.T) (*App, *fakeDiffer, string) {
	t.Helper()
	app, _, _, workdir := approveApp(t)
	differ := &fakeDiffer{base: "origin/main", out: sampleDiff}
	app.differ = differ
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return app, differ, workdir
}

// setSlice changes one slice of the plan on show and puts the board back on it,
// since the board reads the plan through groups it builds afresh each time.
func setSlice(a *App, id string, mut func(*domain.Slice)) {
	for i := range a.project.Slices {
		if a.project.Slices[i].ID == id {
			mut(&a.project.Slices[i])
		}
	}
	p := domain.NewProject(a.project.ID, a.project.Name, a.project.Milestones, a.project.Slices)
	a.project = &p
	a.board.SetProject(a.project)
}

// TestDiffKeyOpensTheReviewScreen covers the whole of v on a handed-back row:
// the screen comes up, git is asked for that branch in that repository, and
// what comes back is drawn.
func TestDiffKeyOpensTheReviewScreen(t *testing.T) {
	app, differ, workdir := diffApp(t)
	cursorOn(t, app, handedBack)

	cmd := press(app, "v")
	if app.screen != screenDiff {
		t.Fatalf("screen = %v, want the review screen", app.screen)
	}
	if !app.diff.Busy() {
		t.Error("the screen should show a read in flight")
	}
	msg := first[diffLoadedMsg](t, run(cmd))
	if len(differ.made) != 1 || differ.made[0] != (diffCall{workdir, "slice/approve"}) {
		t.Fatalf("git was asked for %+v, want the branch in the project's directory", differ.made)
	}
	if msg.err != nil {
		t.Fatalf("diff = %v, want the branch's files", msg.err)
	}

	app.Update(msg)
	if len(app.diff.files) != 4 || app.diff.base != "origin/main" {
		t.Errorf("screen shows %d files against %q, want the diff that came back",
			len(app.diff.files), app.diff.base)
	}
	if got := app.headerName(); got != "Diff slice/approve" {
		t.Errorf("header = %q, want the branch named", got)
	}
	if got := app.chipText(); got != "diff" {
		t.Errorf("chip = %q, want the screen named", got)
	}
	if body := app.body(); !strings.Contains(body, "internal/tui/board.go") {
		t.Error("the body should draw the diff")
	}
}

// TestDiffKeyUsesTheSliceRepo covers a slice with a repository of its own,
// which is where its branch is and so where git is run.
func TestDiffKeyUsesTheSliceRepo(t *testing.T) {
	app, differ, _ := diffApp(t)
	repo := t.TempDir()
	setSlice(app, handedBack, func(s *domain.Slice) { s.Repo = repo })
	cursorOn(t, app, handedBack)

	run(press(app, "v"))
	if len(differ.made) != 1 || differ.made[0].dir != repo {
		t.Errorf("git ran in %+v, want the slice's own repository", differ.made)
	}
}

// TestDiffKeyRefusesRowsWithNoBranch covers every row the key has nothing to
// show for: each is refused where it stands, with the board still up.
func TestDiffKeyRefusesRowsWithNoBranch(t *testing.T) {
	for _, tt := range []struct {
		name string
		on   func(t *testing.T, a *App)
		want string
	}{
		{"a milestone", func(t *testing.T, a *App) { cursorOnMilestone(t, a) },
			"Move to a slice"},
		{"a Todo slice", func(t *testing.T, a *App) { cursorOn(t, a, stillTodo) },
			"only a handed-back slice"},
		{"a Done slice", func(t *testing.T, a *App) { cursorOn(t, a, alreadyPR) },
			"only a handed-back slice"},
		{"a slice still being worked", func(t *testing.T, a *App) {
			setSlice(a, handedBack, func(s *domain.Slice) { s.Branch = "" })
			cursorOn(t, a, handedBack)
		}, "no branch handed back yet"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, differ, _ := diffApp(t)
			tt.on(t, app)
			press(app, "v")
			if app.screen != screenBoard {
				t.Errorf("screen = %v, want the board", app.screen)
			}
			if len(differ.made) != 0 {
				t.Errorf("git ran %+v, want it left alone", differ.made)
			}
			if got := app.board.confirmText; !strings.Contains(got, tt.want) {
				t.Errorf("said %q, want it to mention %q", got, tt.want)
			}
		})
	}
}

// TestDiffKeyRefusesAMissingRepository covers a working directory that is not
// there, which the board knows about before git would have failed over it.
func TestDiffKeyRefusesAMissingRepository(t *testing.T) {
	app, differ, workdir := diffApp(t)
	project := app.cfg.Projects[testProjectID]
	project.WorkingDir = workdir + "-gone"
	app.cfg.Projects[testProjectID] = project
	cursorOn(t, app, handedBack)

	press(app, "v")
	if app.screen != screenBoard || len(differ.made) != 0 {
		t.Errorf("screen = %v after %d calls, want the board and no git", app.screen, len(differ.made))
	}
	if got := app.board.confirmText; !strings.Contains(got, "Cannot read the diff") {
		t.Errorf("said %q, want it to refuse the directory", got)
	}
}

// TestDiffKeyWithNothingToDiffWith covers the guards that stop the flow before
// it starts: no git wired in, and no project to resolve a directory from.
func TestDiffKeyWithNothingToDiffWith(t *testing.T) {
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	app.differ = nil
	if cmd, _ := app.boardWrite(keyPress("v")); cmd != nil {
		t.Error("the key should do nothing with no git behind it")
	}

	app, _, _ = diffApp(t)
	app.cfg.ActiveProjectID = ""
	if cmd, _ := app.boardWrite(keyPress("v")); cmd != nil {
		t.Error("the key should do nothing with no project")
	}
	if app.startDiffLoad() != nil {
		t.Error("a reread should do nothing with no branch to read")
	}
}

// TestDiffReadsTheFilesBehindTheChange covers the second half of a read: every
// file the change touches is asked for at the branch, so the gaps between its
// hunks have somewhere to be filled from. A binary one is not asked for at all —
// it is not lines — and one git refuses costs that file its expanding and
// nothing else.
func TestDiffReadsTheFilesBehindTheChange(t *testing.T) {
	app, differ, _ := diffApp(t)
	differ.files = map[string][]string{"internal/tui/board.go": {"package tui"}}
	cursorOn(t, app, handedBack)

	msg := first[diffLoadedMsg](t, run(press(app, "v")))
	var asked []string
	for _, c := range differ.shown {
		if c.branch != "slice/approve" {
			t.Errorf("git was asked for %q at %q, want the handed-back branch", c.path, c.branch)
		}
		asked = append(asked, c.path)
	}
	want := []string{"internal/tui/board.go", "internal/tui/diff.go",
		"a/very/deeply/nested/directory/somewhere/settings.go"}
	if !slices.Equal(asked, want) {
		t.Errorf("git was asked for %q, want %q — the binary file is not lines", asked, want)
	}
	if got := msg.sources["internal/tui/board.go"]; !slices.Equal(got, []string{"package tui"}) {
		t.Errorf("the file came back as %q, want what git showed", got)
	}
	if _, ok := msg.sources["internal/tui/diff.go"]; ok {
		t.Error("a file git refused should be left out rather than held as nothing")
	}
	app.Update(msg)
	if app.diff.state != diffReady {
		t.Errorf("state = %v, want the diff on screen whatever git would not show", app.diff.state)
	}
}

// TestDiffLoadFailureIsShownOnTheScreen covers git refusing: the screen says
// so, naming the branch and quoting git, and the board behind it is untouched.
func TestDiffLoadFailureIsShownOnTheScreen(t *testing.T) {
	app, differ, _ := diffApp(t)
	differ.err = errors.New("fatal: bad revision 'slice/approve'")
	cursorOn(t, app, handedBack)

	msg := first[diffLoadedMsg](t, run(press(app, "v")))
	if msg.err == nil {
		t.Fatal("diff = nil, want git's refusal")
	}
	app.Update(msg)
	if app.diff.state != diffFailed {
		t.Errorf("state = %v, want the failure on the screen", app.diff.state)
	}
	if app.err != nil {
		t.Errorf("app error = %v, want the screen to report its own", app.err)
	}
	want := "read the diff of slice/approve: fatal: bad revision 'slice/approve'"
	if got := app.diff.err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestRefreshRereadsTheDiffOnShow covers r while the review screen is up: the
// branch is read again, because an agent that pushed another commit is what a
// refresh is being asked about.
func TestRefreshRereadsTheDiffOnShow(t *testing.T) {
	app, differ, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	run(press(app, "v"))

	run(press(app, "r"))
	if len(differ.made) != 2 {
		t.Errorf("git ran %d times, want the diff read again", len(differ.made))
	}
	if !app.diff.Busy() {
		t.Error("the screen should show the reread in flight")
	}
}

// TestRefreshOnTheBoardLeavesGitAlone covers r anywhere else: the diff screen
// is not on show, so there is nothing to read again.
func TestRefreshOnTheBoardLeavesGitAlone(t *testing.T) {
	app, differ, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	run(press(app, "v"))
	press(app, "esc")

	run(press(app, "r"))
	if len(differ.made) != 1 {
		t.Errorf("git ran %d times, want only the first read", len(differ.made))
	}
}

// TestDiffScreenLeavesOnEsc covers the way out, which is the one every screen
// over the board shares.
func TestDiffScreenLeavesOnEsc(t *testing.T) {
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	press(app, "v")
	press(app, "esc")
	if app.screen != screenBoard {
		t.Errorf("screen = %v, want the board back", app.screen)
	}
}

// TestDiffScreenTakesTheKeys covers the screen owning its own navigation: n on
// the review screen jumps a file rather than doing anything to the board.
func TestDiffScreenTakesTheKeys(t *testing.T) {
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	row := app.board.cursor
	app.Update(first[diffLoadedMsg](t, run(press(app, "v"))))

	press(app, "n")
	if app.diff.cursor != 1 {
		t.Errorf("diff cursor = %d, want the next file", app.diff.cursor)
	}
	if app.board.cursor != row {
		t.Error("the board should not move while the review screen is up")
	}
}

// TestDiffScreenHintsAreItsOwn covers the bottom row while the screen is up:
// the board's keys say nothing there, and the file jumps say what does.
func TestDiffScreenHintsAreItsOwn(t *testing.T) {
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	press(app, "v")
	hints := strings.Join(app.hintLines(), " ")
	for _, want := range []string{"next file", "previous file", "back"} {
		if !strings.Contains(hints, want) {
			t.Errorf("hints = %q, want %q on them", hints, want)
		}
	}
	if strings.Contains(hints, "launch agent") {
		t.Errorf("hints = %q, want the board's keys off them", hints)
	}
}

// TestHelpListsTheDiffKeys covers the help screen, which is where a key the
// hints row has no room for is still findable.
func TestHelpListsTheDiffKeys(t *testing.T) {
	app := NewApp(testConfig(), &fakeNotion{})
	body := app.helpBody()
	for _, want := range []string{"review diff", "next file", "previous file"} {
		if !strings.Contains(body, want) {
			t.Errorf("help = %q, want %q listed", body, want)
		}
	}
}

// TestDiffHeadingBeforeABranch covers the header before the screen has been
// pointed at anything, which is the state a fresh app is in.
func TestDiffHeadingBeforeABranch(t *testing.T) {
	app := NewApp(testConfig(), &fakeNotion{})
	if got := app.diffHeading(); got != "Diff" {
		t.Errorf("heading = %q, want the screen's own name", got)
	}
}

// TestSwitchingProjectDropsTheDiff covers the review screen across a project
// switch: the branch it was showing is in the project being left.
func TestSwitchingProjectDropsTheDiff(t *testing.T) {
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	app.Update(first[diffLoadedMsg](t, run(press(app, "v"))))

	app.showActiveProject()
	if app.diff.Loadable() || len(app.diff.files) != 0 {
		t.Error("switching project should leave nothing of the other project's diff")
	}
}

// TestSpinnerTurnsForTheDiff covers the one spinner the app turns: a read in
// flight on the review screen keeps it going.
func TestSpinnerTurnsForTheDiff(t *testing.T) {
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	press(app, "v")
	if _, cmd := app.Update(app.spinner.Tick()); cmd == nil {
		t.Error("the spinner should turn while a diff is being read")
	}
}

// TestNewDifferDrivesTheRealGit pins what the app is wired to when nothing
// stands in for git.
func TestNewDifferDrivesTheRealGit(t *testing.T) {
	if _, ok := newDiffer().(git.CLI); !ok {
		t.Errorf("newDiffer() = %T, want the real git", newDiffer())
	}
}
