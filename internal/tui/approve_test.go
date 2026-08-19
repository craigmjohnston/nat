package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// prCall is one pull request the approve flow asked gh for.
type prCall struct{ dir, branch string }

// fakePRs stands in for the GitHub CLI: it records what it was asked to open
// and answers with the URL — or the refusal — the test wants gh to have given.
type fakePRs struct {
	url  string
	err  error
	made []prCall
}

var _ PRCreator = (*fakePRs)(nil)

func (f *fakePRs) CreatePR(dir, branch string) (string, error) {
	f.made = append(f.made, prCall{dir, branch})
	return f.url, f.err
}

// The plan the approve tests work on: one milestone holding a slice of each
// state the key has an answer for, the first of them the handed-back one.
const (
	handedBack = "hb"
	stillTodo  = "td"
	alreadyPR  = "dn"
)

// approvePlan is that plan, with repo the directory its slices' project
// resolves to.
func approvePlan() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Hand-back"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: handedBack, Name: "Approve action", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Hand-back", AssigneeName: "Craig Johnston", Branch: "slice/approve"},
			{ID: stillTodo, Name: "Startup checks", Status: domain.SliceTodo, MilestoneID: "M1: Hand-back"},
			{ID: alreadyPR, Name: "Hand back", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Hand-back"},
		})
}

// approveApp returns an app showing that plan, with a fake gh and a real
// working directory for the flow to find, and the directory it resolves to.
func approveApp(t *testing.T) (*App, *fakePRs, *fakeNotion, string) {
	t.Helper()
	workdir := t.TempDir()

	cfg := testConfig()
	project := cfg.Projects[testProjectID]
	project.WorkingDir = workdir
	cfg.Projects[testProjectID] = project

	client := &fakeNotion{}
	app := NewApp(cfg, client)
	p := approvePlan()
	app.project = &p
	app.board.hideDone = false // the Done slice is a row the refusals need
	app.board.SetProject(&p)
	prs := &fakePRs{url: "https://github.test/craig/nat/pull/9"}
	app.prs = prs
	return app, prs, client, workdir
}

// approveWorktrees puts a fake worktrunk in for the length of one test, since
// the removal is the second half of the approve and the suite's own fake is
// shared. It answers as a working worktrunk unless the test says otherwise.
func approveWorktrees(t *testing.T) *fakeWorktrees {
	t.Helper()
	trees := &fakeWorktrees{}
	newWorktrees = func() Worktrees { return trees }
	t.Cleanup(func() { newWorktrees = func() Worktrees { return &fakeWorktrees{} } })
	return trees
}

// cursorOn puts the board's cursor on the named slice's row.
func cursorOn(t *testing.T, a *App, id string) {
	t.Helper()
	for i, r := range a.board.rows {
		if r.kind != rowSlice {
			continue
		}
		if a.board.groups[r.group].Slices[r.slice].ID == id {
			a.board.cursor = i
			return
		}
	}
	t.Fatalf("no row for slice %q", id)
}

// approve presses p on the row the cursor is on and takes the prompt's first
// choice, which is the one that opens the pull request.
func approve(t *testing.T, a *App) {
	t.Helper()
	feed(t, a, press(a, "p"))
	if !a.board.Prompting() {
		t.Fatalf("no approve prompt opened: %s", a.board.confirmText)
	}
	drive(t, a, press(a, "enter"))
}

// TestApproveOpensThePullRequestAndClosesTheSlice is the whole action: gh is
// run in the slice's repository from the branch it was handed back on, and the
// URL it gives back goes onto the slice as it is marked Done.
func TestApproveOpensThePullRequestAndClosesTheSlice(t *testing.T) {
	app, prs, client, workdir := approveApp(t)
	// A project whose Status column was converted in the Notion UI, so the
	// write has to take the shape the page was read in.
	client.getPage = func(id string) (*notion.Page, error) {
		return &notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
			notion.PropStatus: {Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceInProgress}},
		}}, nil
	}
	cursorOn(t, app, handedBack)

	approve(t, app)

	want := []prCall{{workdir, "slice/approve"}}
	if len(prs.made) != 1 || prs.made[0] != want[0] {
		t.Fatalf("gh was asked for %v, want %v", prs.made, want)
	}
	if len(client.updated) != 1 {
		t.Fatalf("wrote %d pages, want exactly the slice", len(client.updated))
	}
	wrote := client.updated[0]
	if wrote.pageID != handedBack {
		t.Errorf("wrote page %q, want %q", wrote.pageID, handedBack)
	}
	if got := wrote.properties[notion.PropPR].URL; got != prs.url {
		t.Errorf("PR = %q, want %q", got, prs.url)
	}
	status := wrote.properties[notion.PropStatus]
	if status.Status == nil || status.Status.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want the status shape saying Done", status)
	}
	if !strings.Contains(app.board.confirmText, "Approve action") {
		t.Errorf("confirmation = %q, want it to name the slice", app.board.confirmText)
	}
	if app.busy {
		t.Error("the board is still busy after the pull request was recorded")
	}
}

// TestApproveWritesASelectStatus covers the shape every project this app made
// is in: a plain select, which is what a page read back with no type on its
// Status column is written as.
func TestApproveWritesASelectStatus(t *testing.T) {
	app, _, client, _ := approveApp(t)
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(client.updated) != 1 {
		t.Fatalf("wrote %d pages, want exactly the slice", len(client.updated))
	}
	status := client.updated[0].properties[notion.PropStatus]
	if status.Select == nil || status.Select.Name != notion.SliceDone {
		t.Errorf("Status = %+v, want the select shape saying Done", status)
	}
}

// TestApproveUsesTheSlicesOwnRepo covers a slice whose Repo overrides the
// project's default: that is the checkout the branch is on, so that is where gh
// runs.
func TestApproveUsesTheSlicesOwnRepo(t *testing.T) {
	app, prs, _, _ := approveApp(t)
	repo := t.TempDir()
	slices := append([]domain.Slice(nil), app.project.Slices...)
	slices[0].Repo = repo
	p := domain.NewProject(app.project.ID, app.project.Name, app.project.Milestones, slices)
	app.project = &p
	app.board.SetProject(&p)
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(prs.made) != 1 || prs.made[0].dir != repo {
		t.Errorf("gh ran in %v, want the slice's own repo %q", prs.made, repo)
	}
}

// TestApproveCancelled takes the prompt's other choice: nothing is opened and
// nothing is written.
func TestApproveCancelled(t *testing.T) {
	app, prs, client, _ := approveApp(t)
	cursorOn(t, app, handedBack)

	feed(t, app, press(app, "p"))
	feed(t, app, press(app, "right"))
	drive(t, app, press(app, "enter"))

	if len(prs.made) != 0 || len(client.updated) != 0 {
		t.Errorf("cancelling opened %v and wrote %v", prs.made, client.updated)
	}
	if app.board.Prompting() {
		t.Error("the prompt is still up after it was answered")
	}
}

// TestApproveAbandoned covers esc, which leaves without an answer at all.
func TestApproveAbandoned(t *testing.T) {
	app, prs, _, _ := approveApp(t)
	cursorOn(t, app, handedBack)

	feed(t, app, press(app, "p"))
	feed(t, app, press(app, "esc"))

	if app.board.Prompting() {
		t.Error("esc left the prompt up")
	}
	if len(prs.made) != 0 {
		t.Errorf("esc opened %v", prs.made)
	}
}

// TestApproveRefusals covers the rows the key has nothing to do on: it says why
// on the row and opens no prompt, so nothing is opened by mistake.
func TestApproveRefusals(t *testing.T) {
	tests := []struct {
		name   string
		set    func(a *App)
		reason string
	}{
		{"on a milestone", func(a *App) { a.board.cursor = 0 }, "Move to a slice"},
		{"a Todo slice", func(a *App) { cursorOn(t, a, stillTodo) }, "only a handed-back slice"},
		{"a Done slice", func(a *App) { cursorOn(t, a, alreadyPR) }, "only a handed-back slice"},
		{"in progress with no branch", func(a *App) {
			cursorOn(t, a, handedBack)
			a.board.groups[a.board.rows[a.board.cursor].group].Slices[0].Branch = ""
		}, "no branch handed back yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, prs, _, _ := approveApp(t)
			tt.set(app)

			feed(t, app, press(app, "p"))

			if app.board.Prompting() {
				t.Fatal("a prompt opened on a row that cannot be approved")
			}
			if !strings.Contains(app.board.confirmText, tt.reason) {
				t.Errorf("said %q, want it to mention %q", app.board.confirmText, tt.reason)
			}
			if len(prs.made) != 0 {
				t.Errorf("gh was asked for %v", prs.made)
			}
		})
	}
}

// TestApproveWithoutTheRepo covers a working directory that is not there: the
// board says so rather than letting gh fail inside a subprocess.
func TestApproveWithoutTheRepo(t *testing.T) {
	app, prs, _, workdir := approveApp(t)
	project := app.cfg.Projects[testProjectID]
	project.WorkingDir = filepath.Join(workdir, "gone")
	app.cfg.Projects[testProjectID] = project
	cursorOn(t, app, handedBack)

	feed(t, app, press(app, "p"))
	drive(t, app, press(app, "enter"))

	if len(prs.made) != 0 {
		t.Errorf("gh was asked for %v with no repo to run in", prs.made)
	}
	if !strings.Contains(app.board.confirmText, "is not there") {
		t.Errorf("said %q, want it to name the missing directory", app.board.confirmText)
	}
	if app.busy {
		t.Error("the board was left busy by a launch that never started")
	}
}

// TestApproveReportsAGhFailure covers gh refusing: the reason is a toast, the
// board is not left busy, and nothing at all is written to Notion — the slice is
// still handed back, which is exactly what it was.
func TestApproveReportsAGhFailure(t *testing.T) {
	app, prs, client, _ := approveApp(t)
	prs.err = errors.New(`a pull request for branch "slice/approve" already exists`)
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(client.updated) != 0 {
		t.Errorf("wrote %v after gh refused", client.updated)
	}
	if !strings.Contains(app.toast, "already exists") {
		t.Errorf("toast = %q, want gh's own reason", app.toast)
	}
	if app.toastSev != sevError {
		t.Errorf("toast severity = %v, want an error", app.toastSev)
	}
	if app.err != nil {
		t.Errorf("err = %v, want gh's refusal left off the error banner", app.err)
	}
	if app.busy {
		t.Error("the board is still busy after gh refused")
	}
}

// TestApproveReportsAFailedWrite covers the pull request being opened and
// Notion refusing to record it. That is the one half-done state the action has,
// so it is raised rather than passed over.
func TestApproveReportsAFailedWrite(t *testing.T) {
	for _, tt := range []struct {
		name string
		brk  func(c *fakeNotion)
	}{
		{"the page cannot be read", func(c *fakeNotion) {
			c.getPage = func(string) (*notion.Page, error) { return nil, errors.New("notion is down") }
		}},
		{"the write is refused", func(c *fakeNotion) {
			c.updatePage = func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
				return nil, errors.New("notion is down")
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, prs, client, _ := approveApp(t)
			tt.brk(client)
			cursorOn(t, app, handedBack)

			approve(t, app)

			if len(prs.made) != 1 {
				t.Fatalf("gh was asked for %v, want the one pull request", prs.made)
			}
			if app.err == nil || !strings.Contains(app.err.Error(), "record the pull request") {
				t.Errorf("err = %v, want the failed write reported", app.err)
			}
			if app.busy {
				t.Error("the board is still busy after the write failed")
			}
		})
	}
}

// TestApproveWaitsForTheBoard covers the guards the key shares with every other
// write: nothing at all happens without a project, a client, or a free board.
func TestApproveWaitsForTheBoard(t *testing.T) {
	tests := []struct {
		name string
		set  func(a *App)
	}{
		{"a write already in flight", func(a *App) { a.busy = true }},
		{"no project configured", func(a *App) { a.cfg.ActiveProjectID = "" }},
		{"no gh to run", func(a *App) { a.prs = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, prs, _, _ := approveApp(t)
			cursorOn(t, app, handedBack)
			tt.set(app)

			feed(t, app, press(app, "p"))

			if app.board.Prompting() || app.board.confirmText != "" {
				t.Errorf("the key answered with %q, want nothing at all", app.board.confirmText)
			}
			if len(prs.made) != 0 {
				t.Errorf("gh was asked for %v", prs.made)
			}
		})
	}
}

// TestApproveRemovesTheWorktree is the other half of the action: once the pull
// request exists and the slice is Done, the worktree its agent worked in goes,
// taken off the same repository gh ran in and named by the branch that was
// handed back.
func TestApproveRemovesTheWorktree(t *testing.T) {
	app, _, _, workdir := approveApp(t)
	trees := approveWorktrees(t)
	cursorOn(t, app, handedBack)

	approve(t, app)

	want := worktreeCall{workdir, "slice/approve"}
	if len(trees.removes) != 1 || trees.removes[0] != want {
		t.Fatalf("wt was asked to remove %v, want %v", trees.removes, want)
	}
}

// TestApproveRemovesTheWorktreeFromTheSlicesOwnRepo covers a slice whose Repo
// overrides the project's default: that is the checkout the worktree was cut
// from, the way it is the one gh runs in.
func TestApproveRemovesTheWorktreeFromTheSlicesOwnRepo(t *testing.T) {
	app, _, _, _ := approveApp(t)
	trees := approveWorktrees(t)
	repo := t.TempDir()
	slices := append([]domain.Slice(nil), app.project.Slices...)
	slices[0].Repo = repo
	p := domain.NewProject(app.project.ID, app.project.Name, app.project.Milestones, slices)
	app.project = &p
	app.board.SetProject(&p)
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(trees.removes) != 1 || trees.removes[0].dir != repo {
		t.Errorf("wt ran in %v, want the slice's own repo %q", trees.removes, repo)
	}
}

// TestApproveSurvivesAFailedRemoval covers everything worktrunk refuses over —
// a dirty worktree, a slice that never had one, a machine with no wt at all.
// The pull request has been opened and the slice is Done: the approve stands,
// and the refusal is left in the log rather than raised on the board.
func TestApproveSurvivesAFailedRemoval(t *testing.T) {
	app, _, client, _ := approveApp(t)
	trees := approveWorktrees(t)
	trees.removeErr = &worktree.ExitError{Code: 1, Stderr: "worktree has uncommitted changes\n"}
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(client.updated) != 1 {
		t.Fatalf("wrote %d pages, want the slice marked Done regardless", len(client.updated))
	}
	if app.err != nil || app.toast != "" {
		t.Errorf("err = %v, toast = %q, want the refusal passed over", app.err, app.toast)
	}
	if !strings.Contains(app.board.confirmText, "Approve action") {
		t.Errorf("confirmation = %q, want the approve reported as it always is", app.board.confirmText)
	}
	if app.busy {
		t.Error("the board is still busy after the removal was passed over")
	}
}

// TestApproveKeepsTheWorktreeUntilTheSliceIsDone covers the two ways the action
// stops short: gh refusing, and Notion refusing to record what gh opened. The
// slice is still handed back either way, and a slice being reviewed keeps the
// checkout its work is in.
func TestApproveKeepsTheWorktreeUntilTheSliceIsDone(t *testing.T) {
	for _, tt := range []struct {
		name string
		brk  func(a *App, c *fakeNotion)
	}{
		{"gh refused", func(a *App, _ *fakeNotion) {
			a.prs.(*fakePRs).err = errors.New("no such branch")
		}},
		{"the write was refused", func(_ *App, c *fakeNotion) {
			c.updatePage = func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
				return nil, errors.New("notion is down")
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _, client, _ := approveApp(t)
			trees := approveWorktrees(t)
			tt.brk(app, client)
			cursorOn(t, app, handedBack)

			approve(t, app)

			if len(trees.removes) != 0 {
				t.Errorf("wt was asked to remove %v on a slice still handed back", trees.removes)
			}
		})
	}
}

// TestDefaultPRCreatorIsGh pins what a real app opens pull requests through.
func TestDefaultPRCreatorIsGh(t *testing.T) {
	if _, ok := defaultPRCreator().(gh.CLI); !ok {
		t.Errorf("defaultPRCreator() = %T, want the GitHub CLI", defaultPRCreator())
	}
	if NewApp(testConfig(), &fakeNotion{}).prs == nil {
		t.Error("a new app has nothing to open pull requests with")
	}
}

// TestApproveIsInTheHintsAndHelp keeps the key discoverable: the hints row
// offers it on a slice, and the help screen lists it among the writes.
func TestApproveIsInTheHintsAndHelp(t *testing.T) {
	b := newTestBoard()
	if !hintedKey(b.sliceHints(), "p") {
		t.Errorf("the slice hints do not offer p: %v", b.sliceHints())
	}
	if !bindsKey(b.helpBindings(), "p") {
		t.Error("the help screen does not list p")
	}
}

// hintedKey reports whether the hints row offers the named key.
func hintedKey(hints []hint, want string) bool {
	for _, h := range hints {
		if h.binding.Help().Key == want {
			return true
		}
	}
	return false
}

// bindsKey reports whether any of the bindings is for the named key.
func bindsKey(bindings []key.Binding, want string) bool {
	for _, b := range bindings {
		if b.Help().Key == want {
			return true
		}
	}
	return false
}
