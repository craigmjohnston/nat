package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/store"
)

// localApp is a board on one project whose plan is kept in a file of nat's own,
// with no Notion client at all — which is the whole point: nothing on this
// board's path reaches a workspace.
func localApp(t *testing.T, dir string) (*App, string) {
	t.Helper()
	id := "p-local"
	if _, err := store.CreateLocalProject(dir, id, "tracker", "Branch per slice."); err != nil {
		t.Fatalf("create the plan: %v", err)
	}
	cfg := config.Config{
		AssigneeUserName: "Craig Johnston",
		ActiveProjectID:  id,
		Projects: map[string]config.ProjectConfig{
			id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: dir},
		},
	}
	a := NewApp(cfg, nil)
	t.Cleanup(a.closeLocalStores)
	return a, id
}

// The store for a local project is opened once and kept: a board switches back
// and forth by a keystroke, and re-opening the file on every plan load would be
// a keystroke's worth of work each time.
func TestPlanStoreKeepsALocalPlanOpen(t *testing.T) {
	a, _ := localApp(t, t.TempDir())

	first := a.planStore()
	if first == nil {
		t.Fatal("no store for a local project")
	}
	if _, ok := first.(*store.Local); !ok {
		t.Errorf("store = %T, want a local one", first)
	}
	if again := a.planStore(); again != first {
		t.Error("the plan should be opened once and kept")
	}
}

// A Notion project is the client the board already holds, and a board with no
// client has no store at all rather than one that fails at the first request.
func TestPlanStoreAnswersForNotionProjects(t *testing.T) {
	if st := NewApp(testConfig(), &fakeNotion{}).planStore(); st == nil {
		t.Fatal("no store for a Notion project")
	}
	if st := NewApp(testConfig(), nil).planStore(); st != nil {
		t.Errorf("store = %v, want none with no client", st)
	}
	if st := NewApp(config.Config{}, &fakeNotion{}).planStore(); st != nil {
		t.Errorf("store = %v, want none with no active project", st)
	}
}

// A plan that will not open answers as none, with the reason kept for the load
// that is about to find no store — which is what puts it on the board.
func TestPlanStoreReportsAPlanThatWillNotOpen(t *testing.T) {
	dir := t.TempDir()
	id := "p-local"
	if err := os.WriteFile(filepath.Join(dir, id+".db"), []byte("not a database"), 0o644); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	a := NewApp(config.Config{
		ActiveProjectID: id,
		Projects: map[string]config.ProjectConfig{
			id: {Name: "tracker", Backend: config.BackendLocal, PlanDir: dir},
		},
	}, nil)

	if st := a.planStore(); st != nil {
		t.Fatalf("store = %v, want none", st)
	}
	if a.localErr == nil {
		t.Fatal("want the reason kept")
	}
	cmd := a.startLoad()
	if cmd == nil {
		t.Fatal("want the load to run and report why it cannot")
	}
	if msg := first[notionErrMsg](t, run(cmd)); msg.err == nil {
		t.Errorf("msg = %v, want the failure reported", msg)
	}
}

// Closing the board gives back every plan it opened.
func TestQuitClosesTheLocalPlans(t *testing.T) {
	a, _ := localApp(t, t.TempDir())
	st := a.planStore()
	if st == nil {
		t.Fatal("no store to close")
	}

	if !isQuitCmd(a.quit()) {
		t.Error("quitting should quit")
	}
	if a.localStores != nil {
		t.Error("the plans should be given back")
	}
	// A store that has been closed answers nothing, which is how the close is
	// seen from outside.
	if _, _, err := st.Slice(context.Background(), "s1"); err == nil {
		t.Error("the plan was not closed")
	}
}

// A close that fails is logged and nothing else: the process is going either
// way, and a file that would not close is not news anybody can act on.
func TestQuitSurvivesAPlanThatWillNotClose(t *testing.T) {
	a, _ := localApp(t, t.TempDir())
	st := a.planStore()
	if err := st.Close(); err != nil {
		t.Fatalf("close the plan: %v", err)
	}

	if !isQuitCmd(a.quit()) {
		t.Error("quitting should quit even when a plan will not close")
	}
}

// The board loads a local plan through the store the same way it loads a Notion
// one, with nothing in the session that could have reached a workspace.
func TestBoardLoadsALocalPlan(t *testing.T) {
	a, id := localApp(t, t.TempDir())

	msg := first[projectLoadedMsg](t, run(a.startLoad()))
	if msg.project.ID != id || msg.project.Name != "tracker" {
		t.Errorf("project = %+v, want the local one", msg.project)
	}
	// The conventions are the project's page body, read through the same store.
	a.info.Reset()
	body := first[infoLoadedMsg](t, run(a.startInfoLoad()))
	if body.markdown != "Branch per slice." {
		t.Errorf("conventions = %q, want the ones written", body.markdown)
	}
}

// A plan kept in a file has no page to read a wishlist off, and no "edited
// since" to ask about: the first is nothing to fetch, the second a full load,
// which against a file on this machine is cheaper than the round trip the
// selective load was written to save.
func TestALocalProjectHasNoWishlistAndNoSelectiveLoad(t *testing.T) {
	a, _ := localApp(t, t.TempDir())

	if cmd := a.fetchWishlist(a.cfg.ActiveProjectID); cmd != nil {
		t.Error("a local project has no wishlist page to read")
	}

	a.Update(first[projectLoadedMsg](t, run(a.startLoad())))
	a.loading, a.syncedAt = false, timeNow()
	// A full load, rather than the changed-slices query a Notion project takes.
	if got := first[projectLoadedMsg](t, run(a.startSelectiveLoad())); got.project.Name != "tracker" {
		t.Error("a local project's selective load should be a full one")
	}
}

// Who the board works a local project's slices as is the name it knows, since
// there is no directory of users behind a plan kept in a file.
func TestOwnerFollowsWhereThePlanIsKept(t *testing.T) {
	a, _ := localApp(t, t.TempDir())
	if me := a.owner(); me.ID != "Craig Johnston" || me.Name != "Craig Johnston" {
		t.Errorf("owner = %+v, want the name standing for both", me)
	}

	cfg := testConfig()
	cfg.AssigneeUserID = "u1"
	n := NewApp(cfg, &fakeNotion{})
	if me := n.owner(); me.ID != "u1" || me.Name != "Craig Johnston" {
		t.Errorf("owner = %+v, want the workspace user and their name", me)
	}
}

// The new-project form's local half lays the plan down and reports it the way a
// created Notion project is reported, with nothing in Notion to report.
func TestCreateLocalProjectWritesThePlan(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	msg := runMsg(t, createLocalProject("tracker", "Branch per slice.", "/work")).(projectCreatedMsg)

	if msg.err != nil {
		t.Fatalf("err = %v", msg.err)
	}
	if !msg.local || msg.id == "" || msg.structure != nil {
		t.Fatalf("msg = %+v, want a local project with an ID of nat's own", msg)
	}
	if _, err := os.Stat(msg.plan); err != nil {
		t.Fatalf("the plan file was not written: %v", err)
	}
}

// A plan that cannot be laid down is the creation's own failure, reported with
// nothing recorded.
func TestCreateLocalProjectReportsAPlanItCannotWrite(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	msg := runMsg(t, createLocalProject("tracker", "", "/work")).(projectCreatedMsg)
	if msg.err == nil {
		t.Fatal("want the failure reported")
	}
	if msg.local {
		t.Error("nothing was created")
	}
}

// Creating a local project from the board records it as local, makes it the
// active one, and loads the board onto its plan.
func TestAppNewProjectCreatesALocalOne(t *testing.T) {
	saved := capturedConfig(t)
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	app := newProjectApp(&fakeNotion{})
	t.Cleanup(app.closeLocalStores)

	feed(t, app, press(app, "N"))
	// Down onto "This machine", then through the rest of the form.
	feed(t, app, press(app, "down"))
	feed(t, app, press(app, "enter"))
	typeText(app, "tracker two")
	feed(t, app, press(app, "enter"))
	typeText(app, "The conventions.")
	feed(t, app, press(app, "tab"))
	typeText(app, t.TempDir())
	feed(t, app, press(app, "enter"))
	feed(t, app, press(app, "n"))
	finishForm(t, app, press(app, "enter"))

	id := app.cfg.ActiveProjectID
	entry, ok := app.cfg.Projects[id]
	if !ok || !entry.IsLocal() {
		t.Fatalf("config = %+v, want the new project recorded as local", app.cfg.Projects)
	}
	if entry.Name != "tracker two" {
		t.Errorf("name = %q, want the one typed", entry.Name)
	}
	if !saved.Projects[id].IsLocal() {
		t.Error("the config written to disk should say so too")
	}
	// And the plan it wrote is the one the board now reads.
	plan := first[projectLoadedMsg](t, run(app.startLoad()))
	if plan.project.Name != "tracker two" {
		t.Errorf("plan = %+v, want the project just created", plan.project)
	}
}

// A creation that failed is reported and records nothing, which for a local
// project is the only half-done state there could be.
func TestAppNewProjectReportsAFailedLocalCreation(t *testing.T) {
	app := newProjectApp(&fakeNotion{})
	boom := errors.New("boom")

	app.Update(projectCreatedMsg{err: boom})

	if !errors.Is(app.err, boom) {
		t.Errorf("err = %v, want the failure shown", app.err)
	}
	if len(app.cfg.Projects) != 1 {
		t.Errorf("projects = %+v, want nothing recorded", app.cfg.Projects)
	}
}

// The form asks where the plan lives only where there is a choice, and the
// question it asks is answered by the save.
func TestNewProjectFormDefaultsToNotion(t *testing.T) {
	f := newNewProjectForm(DefaultStyles().FormTheme, true)
	f.SetSize(80, 24)
	f.Init()
	if f.backend != config.BackendNotion {
		t.Errorf("backend = %q, want Notion", f.backend)
	}
	if !strings.Contains(stripANSI(f.View()), "Where the plan lives") {
		t.Errorf("view does not ask where the plan lives:\n%s", f.View())
	}
}

// closeFails is a store that will not give its file back, which is the one
// thing closing has to survive: the process is going either way.
type closeFails struct{ store.Store }

func (closeFails) Close() error { return errors.New("boom") }

func TestQuitSurvivesAPlanThatRefusesToClose(t *testing.T) {
	a, _ := localApp(t, t.TempDir())
	a.localStores = map[string]store.Store{"p-local": closeFails{}}

	if !isQuitCmd(a.quit()) {
		t.Error("quitting should quit even when a plan refuses to close")
	}
	if a.localStores != nil {
		t.Error("the plans should be let go of either way")
	}
}
