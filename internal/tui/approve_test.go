package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// prCall is one pull request the approve flow asked gh for.
type prCall struct{ dir, branch, title, body string }

// fakePRs stands in for the GitHub CLI: it records what it was asked to open
// and answers with the URL — or the refusal — the test wants gh to have given.
type fakePRs struct {
	url  string
	err  error
	made []prCall
}

var _ PRCreator = (*fakePRs)(nil)

func (f *fakePRs) CreatePR(dir, branch, title, body string) (string, error) {
	f.made = append(f.made, prCall{dir, branch, title, body})
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
//
// It has a git behind it and a window to draw in as well, because approving is
// the review screen's key: every test here reaches it by reading the diff, which
// is the whole of the change this slice made.
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
	app.differ = &fakeDiffer{base: "origin/main", out: sampleDiff}
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return app, prs, client, workdir
}

// approveWorktrees puts a fake worktree layer in for the length of one test,
// since the removal is the second half of the approve and the suite's own fake
// is shared. It answers as a working git unless the test says otherwise.
func approveWorktrees(t *testing.T) *fakeWorktrees {
	t.Helper()
	trees := &fakeWorktrees{}
	newWorktrees = func() Worktrees { return trees }
	t.Cleanup(func() { newWorktrees = func() Worktrees { return &fakeWorktrees{} } })
	return trees
}

// cursorOn puts the board's cursor on the named slice's row.
// cursorOnMilestone puts the cursor on the first milestone row of the board.
// It is not row 0: the Active section's entries are drawn above the plan.
func cursorOnMilestone(t *testing.T, a *App) {
	t.Helper()
	if !a.board.cursorTo(func(r row) bool { return r.kind == rowMilestone }) {
		t.Fatal("the board has no milestone row")
	}
}

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

// review opens the review screen on the row the cursor is on and lets the diff
// land, which is how every approve is reached now.
func review(t *testing.T, a *App) {
	t.Helper()
	a.Update(first[diffLoadedMsg](t, run(press(a, "v"))))
	if a.screen != screenDiff {
		t.Fatalf("no review screen opened: %s", a.board.confirmText)
	}
}

// approve reads the slice's diff and presses the approve key on it, which opens
// the pull request there and then: the screen is the confirmation, so there is
// no prompt to answer.
func approve(t *testing.T, a *App) {
	t.Helper()
	review(t, a)
	drive(t, a, press(a, "a"))
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

	want := []prCall{{workdir, "slice/approve", "", ""}}
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

// TestApproveOpensThePullRequestWithTheRecordedDescription is the other half of
// the action: the description the agent filed at hand-back is read off the
// slice page and is what gh is given — its first line the title, the rest the
// body — so the pull request reads as the agent wrote it rather than as its
// commits happen to.
func TestApproveOpensThePullRequestWithTheRecordedDescription(t *testing.T) {
	app, prs, client, workdir := approveApp(t)
	client.blocks = func(id string) ([]notion.Block, error) {
		if id != handedBack {
			t.Errorf("read the body of %q, want the slice being approved", id)
		}
		return []notion.Block{
			block(t, "heading_3", "Handed back"),
			block(t, "paragraph", "Did the work."),
			block(t, "heading_3", notion.PRDescriptionHeading),
			block(t, "paragraph", "Open the PR with the recorded description"),
			block(t, "paragraph", "What it does, and why."),
		}, nil
	}
	cursorOn(t, app, handedBack)

	approve(t, app)

	want := prCall{workdir, "slice/approve",
		"Open the PR with the recorded description", "What it does, and why."}
	if len(prs.made) != 1 || prs.made[0] != want {
		t.Fatalf("gh was asked for %v, want %v", prs.made, want)
	}
}

// TestApproveWithoutARecordedDescription covers every hand-back written before
// there was a flag for one: nothing is read off the page, so gh is given no
// title and fills the pull request from the commits as it always did.
func TestApproveWithoutARecordedDescription(t *testing.T) {
	app, prs, client, workdir := approveApp(t)
	client.blocks = func(string) ([]notion.Block, error) {
		return []notion.Block{block(t, "heading_3", "Handed back"), block(t, "paragraph", "Did the work.")}, nil
	}
	cursorOn(t, app, handedBack)

	approve(t, app)

	want := prCall{workdir, "slice/approve", "", ""}
	if len(prs.made) != 1 || prs.made[0] != want {
		t.Fatalf("gh was asked for %v, want %v", prs.made, want)
	}
}

// TestApproveWithAnUnreadableDescription covers the page body failing to load:
// nothing is opened, because a pull request opened with the wrong title is not
// one this key can open again. The reason is a toast — the branch is still
// there and the slice is still handed back.
func TestApproveWithAnUnreadableDescription(t *testing.T) {
	app, prs, client, _ := approveApp(t)
	client.blocks = func(string) ([]notion.Block, error) { return nil, errors.New("notion is down") }
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(prs.made) != 0 {
		t.Errorf("gh was asked for %v with the description unread", prs.made)
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %v with the description unread", client.updated)
	}
	if !strings.Contains(app.toast, "pull request description") {
		t.Errorf("toast = %q, want it to name what could not be read", app.toast)
	}
	if app.busy {
		t.Error("the board is still busy after the read failed")
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

// TestApproveClosesTheReview covers where the user is left: the review is over,
// so the board is what the confirmation — and any refusal — is read on.
func TestApproveClosesTheReview(t *testing.T) {
	app, _, _, _ := approveApp(t)
	cursorOn(t, app, handedBack)

	approve(t, app)

	if app.screen != screenBoard {
		t.Errorf("screen = %v, want the board back once the work is approved", app.screen)
	}
}

// TestTheBoardHasNoApproveKey covers the other half of moving the action: p on
// a handed-back row does nothing at all, since reading the change is how it is
// approved now.
func TestTheBoardHasNoApproveKey(t *testing.T) {
	app, prs, client, _ := approveApp(t)
	cursorOn(t, app, handedBack)

	feed(t, app, press(app, "p"))

	if app.board.Prompting() || app.board.confirmText != "" {
		t.Errorf("p answered with %q, want nothing at all", app.board.confirmText)
	}
	if len(prs.made) != 0 || len(client.updated) != 0 {
		t.Errorf("p opened %v and wrote %v", prs.made, client.updated)
	}
}

// TestApproveWithCommentsPending covers a review that still has something to
// say: the comments are held nowhere but this session, so approving over them
// would lose them. The key reports them instead and the screen stays up.
func TestApproveWithCommentsPending(t *testing.T) {
	app, prs, _, _ := approveApp(t)
	cursorOn(t, app, handedBack)
	review(t, app)
	path, start, span, _, ok := app.diff.Selection()
	if !ok {
		t.Fatal("the diff has no line to comment on")
	}
	app.diff.SetComment(path, start, span, "this wants a test")

	drive(t, app, press(app, "a"))

	if len(prs.made) != 0 {
		t.Errorf("gh was asked for %v with a comment still pending", prs.made)
	}
	if !strings.Contains(app.toast, "still pending") {
		t.Errorf("toast = %q, want it to name the pending comments", app.toast)
	}
	if app.screen != screenDiff {
		t.Errorf("screen = %v, want the review still up", app.screen)
	}
	if app.diff.Pending() != 1 {
		t.Errorf("%d comments pending, want the comment left where it was", app.diff.Pending())
	}
}

// TestApproveWithoutABranchOnScreen covers the key pressed on a screen that has
// never been pointed at one, which has nothing to approve.
func TestApproveWithoutABranchOnScreen(t *testing.T) {
	app, prs, _, _ := approveApp(t)
	if cmd := app.approveDiffFlow(); cmd != nil {
		t.Error("the key should do nothing with no branch on screen")
	}
	if len(prs.made) != 0 {
		t.Errorf("gh was asked for %v", prs.made)
	}
}

// TestApproveWithoutTheRepo covers a working directory that went while the diff
// was being read: the app says so rather than letting gh fail inside a
// subprocess over it.
func TestApproveWithoutTheRepo(t *testing.T) {
	app, prs, _, workdir := approveApp(t)
	cursorOn(t, app, handedBack)
	review(t, app)
	if err := os.RemoveAll(workdir); err != nil {
		t.Fatal(err)
	}

	drive(t, app, press(app, "a"))

	if len(prs.made) != 0 {
		t.Errorf("gh was asked for %v with no repo to run in", prs.made)
	}
	if !strings.Contains(app.board.confirmText, "is not there") {
		t.Errorf("said %q, want it to name the missing directory", app.board.confirmText)
	}
	if app.busy {
		t.Error("the board was left busy by an approve that never started")
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
			// The review is opened first: each of these is the state the app is
			// in when the key is pressed, not a reason it could not be read.
			review(t, app)
			tt.set(app)

			feed(t, app, press(app, "a"))

			if app.toast != "" || app.board.confirmText != "" {
				t.Errorf("the key answered with %q / %q, want nothing at all",
					app.toast, app.board.confirmText)
			}
			if app.screen != screenDiff {
				t.Errorf("screen = %v, want the review left up", app.screen)
			}
			if len(prs.made) != 0 {
				t.Errorf("gh was asked for %v", prs.made)
			}
		})
	}
}

// TestApproveLeavesTheWorktreeInPlace is what approving does not do: the pull
// request is open and the slice is Done, but the review has only just started
// and a review that asks for one more commit needs the checkout that commit is
// written in. Git is asked nothing at all — what takes the worktree away is the
// merge; see landed_test.go.
func TestApproveLeavesTheWorktreeInPlace(t *testing.T) {
	app, _, client, _ := approveApp(t)
	trees := approveWorktrees(t)
	cursorOn(t, app, handedBack)

	approve(t, app)

	if len(client.updated) != 1 {
		t.Fatalf("wrote %d pages, want the slice marked Done", len(client.updated))
	}
	if len(trees.removes) != 0 || len(trees.looks) != 0 {
		t.Errorf("git was asked about %v / %v, want the worktree left alone", trees.looks, trees.removes)
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

// TestApproveIsInTheHintsAndHelp keeps the key discoverable where it now lives:
// the review screen's hints row offers a, and the help screen lists it among
// that screen's keys. The board offers no approve at all — p there is nothing,
// and the diff key is the way to it.
func TestApproveIsInTheHintsAndHelp(t *testing.T) {
	d := NewDiff(NewStyles(true))
	if !hintedKey(d.hints(defaultKeyMap().Back), "a") {
		t.Errorf("the review hints do not offer a: %v", d.hints(defaultKeyMap().Back))
	}
	if !bindsKey(d.keys.bindings(), "a") {
		t.Error("the help screen does not list a")
	}
	b := newTestBoard()
	if hintedKey(b.sliceHints(), "p") || bindsKey(b.helpBindings(), "p") {
		t.Error("the board still offers an approve key of its own")
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
