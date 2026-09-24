package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// answerWhere takes the default of the form's first question — Notion.
func answerWhere(t *testing.T, a *App) { t.Helper(); nextGroup(t, a) }

// nextGroup presses enter and follows the messages that moving on to the next
// group sets off, which [feed] alone stops short of: the focus of the group
// after is its own round trip.
func nextGroup(t *testing.T, a *App) {
	t.Helper()
	cmd := press(a, "enter")
	for i := 0; i < 5 && cmd != nil; i++ {
		var next []tea.Cmd
		for _, msg := range run(cmd) {
			_, c := a.Update(msg)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
}

// isolatedPlans points the plan directory at one of this test's own, so a
// project made by a test never lands among this machine's real plans.
func isolatedPlans(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
}

// testProjectsDSID is the projects data source onboarding leaves behind, which
// is what a new project is created under.
const testProjectsDSID = "projects-ds"

// newStructure is what a successful CreateProject comes back with.
func newStructure() *notion.ProjectStructure {
	return &notion.ProjectStructure{
		PageID:     "p2",
		PageURL:    "https://notion.so/p2",
		SlicesDBID: "s-db",
		SlicesDSID: "s-ds",
	}
}

// creatingClient answers CreateProject with a whole project structure.
func creatingClient() *fakeNotion {
	return &fakeNotion{
		newProject: func(string, string) (*notion.ProjectStructure, error) { return newStructure(), nil },
	}
}

// capturedConfig redirects the config write to memory for the length of a test,
// returning where the last saved config lands.
func capturedConfig(t *testing.T) *config.Config {
	t.Helper()
	var saved config.Config
	restore := saveConfig
	saveConfig = func(c config.Config) error {
		saved = c
		return nil
	}
	t.Cleanup(func() { saveConfig = restore })
	return &saved
}

// failingConfig makes every config write fail for the length of a test.
func failingConfig(t *testing.T, err error) {
	t.Helper()
	restore := saveConfig
	saveConfig = func(config.Config) error { return err }
	t.Cleanup(func() { saveConfig = restore })
}

// newProjectApp returns an app whose config names a projects database, so the
// new-project key has somewhere to create under.
func newProjectApp(t *testing.T, client NotionAPI) *App {
	a := newWriteApp(t, client)
	a.cfg.ProjectDBDataSourceID = testProjectsDSID
	return a
}

// twoProjectConfig is a config holding a second project, so there is something
// to switch between. It is named to sort before the active one.
func twoProjectConfig(t *testing.T) config.Config {
	cfg := testConfig(t)
	cfg.Projects["other"] = config.ProjectConfig{
		Name: "another", SlicesDSID: "o-sl",
	}
	return cfg
}

func TestCreateProjectBuildsTheProjectAndItsPageBody(t *testing.T) {
	client := creatingClient()

	msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "Line one.\n\nLine two.", "/work", false)).(projectCreatedMsg)

	if msg.err != nil {
		t.Fatalf("err = %v, want a clean creation", msg.err)
	}
	if msg.name != "tracker" || msg.workdir != "/work" {
		t.Errorf("msg = %+v, want the answers carried through", msg)
	}
	if msg.structure == nil || msg.structure.PageID != "p2" {
		t.Fatalf("structure = %+v, want what the client created", msg.structure)
	}
	want := []createProjectCall{{projectsDSID: testProjectsDSID, name: "tracker"}}
	if len(client.createdProjects) != 1 || client.createdProjects[0] != want[0] {
		t.Errorf("created %+v, want %+v", client.createdProjects, want)
	}
	if len(client.appended) != 1 || client.appended[0].pageID != "p2" {
		t.Fatalf("appended = %+v, want the blurb on the project page", client.appended)
	}
	if got := paragraphTexts(t, client.appended[0].children); !equal(got, []string{"Line one.", "Line two."}) {
		t.Errorf("body = %v, want a paragraph per chunk", got)
	}
}

func TestCreateProjectWithoutABlurbWritesNoBody(t *testing.T) {
	client := creatingClient()

	runMsg(t, createProject(client, testProjectsDSID, "tracker", "   \n\n  ", "/work", false))

	if len(client.appended) != 0 {
		t.Errorf("appended %+v, want no body write for an empty blurb", client.appended)
	}
}

func TestCreateProjectReportsAFailedCreation(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		create func(string, string) (*notion.ProjectStructure, error)
		want   string
	}{
		{"the call failed", func(string, string) (*notion.ProjectStructure, error) { return nil, boom },
			"create project: boom"},
		{"nothing came back", func(string, string) (*notion.ProjectStructure, error) { return nil, nil },
			"create project: no project was returned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeNotion{newProject: tt.create}

			msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "blurb", "/work", false)).(projectCreatedMsg)

			if msg.structure != nil {
				t.Errorf("structure = %+v, want nothing recorded", msg.structure)
			}
			if msg.err == nil || msg.err.Error() != tt.want {
				t.Fatalf("err = %v, want %q", msg.err, tt.want)
			}
			if len(client.appended) != 0 {
				t.Error("there is no page to write a body onto")
			}
		})
	}
}

func TestCreateProjectKeepsAProjectWhoseSchemaDidNotVerify(t *testing.T) {
	// A non-nil structure alongside an error is the client reporting a project
	// that exists but read back wrong.
	schemaErr := &notion.SchemaError{DataSource: "Slices", Problems: []string{`missing property "PR" (url)`}}
	client := &fakeNotion{
		newProject: func(string, string) (*notion.ProjectStructure, error) { return newStructure(), schemaErr },
	}

	msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "blurb", "/work", false)).(projectCreatedMsg)

	if msg.structure == nil {
		t.Fatal("the project exists, so it should still be recorded")
	}
	if !errors.Is(msg.err, schemaErr) {
		t.Errorf("err = %v, want the schema failure reported", msg.err)
	}
	if len(client.appended) != 1 {
		t.Error("the page body should still be written")
	}
}

func TestCreateProjectReportsAFailedPageBody(t *testing.T) {
	boom := errors.New("boom")
	client := creatingClient()
	client.appendBlock = func(string, []map[string]any) ([]notion.Block, error) { return nil, boom }

	msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "blurb", "/work", false)).(projectCreatedMsg)

	if msg.structure == nil {
		t.Fatal("the project exists, so it should still be recorded")
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "write project page: boom") {
		t.Errorf("err = %v, want the body failure reported", msg.err)
	}
}

// fillProjectForm drives the new-project form to completion: the three text
// fields, then the assignee question the slices schema hangs on.
func fillProjectForm(t *testing.T, a *App, name, info, workdir string, assignee bool) {
	t.Helper()
	// Where the plan lives comes first, and the default — Notion — is taken.
	answerWhere(t, a)
	fillProjectFields(t, a, name, info, workdir, assignee)
}

// fillProjectFields is the rest of the form, once the plan's home has been
// answered.
func fillProjectFields(t *testing.T, a *App, name, info, workdir string, assignee bool) {
	t.Helper()
	typeText(a, name)
	feed(t, a, press(a, "enter"))
	typeText(a, info)
	feed(t, a, press(a, "tab"))
	typeText(a, workdir)
	nextGroup(t, a)
	answer := "n"
	if assignee {
		answer = "y"
	}
	feed(t, a, press(a, answer))
	finishForm(t, a, press(a, "enter"))
}

func TestAppNewProjectFlowWritesConfigAndReloads(t *testing.T) {
	saved := capturedConfig(t)
	client := creatingClient()
	client.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) { return nil, nil }
	app := newProjectApp(t, client)
	dir := t.TempDir()

	feed(t, app, press(app, "N"))
	if app.form == nil || app.screen != screenForm {
		t.Fatalf("screen = %v, form = %v, want the new-project form", app.screen, app.form)
	}
	view := stripANSI(app.View().Content)
	if !strings.Contains(view, "Where will the plan live?") {
		t.Errorf("view is missing the question of where the plan lives:\n%s", view)
	}
	answerWhere(t, app)
	view = stripANSI(app.View().Content)
	for _, want := range []string{"New project", "Name", "Info", "Working directory"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	fillProjectFields(t, app, "tracker two", "The conventions.", dir, false)

	// The question defaults to no, and the answer is what decides whether the
	// Slices table gets an Assignee column at all.
	if len(client.createdProjects) != 1 || client.createdProjects[0].assignee {
		t.Errorf("created %+v, want one project with no assignee column", client.createdProjects)
	}

	if app.err != nil {
		t.Fatalf("err = %v, want a clean creation", app.err)
	}
	if app.toast != `Created "tracker two".` {
		t.Errorf("toast = %q, want the created toast", app.toast)
	}
	want := config.ProjectConfig{
		Name: "tracker two", SlicesDSID: "s-ds", WorkingDir: dir,
	}
	if got := saved.Projects["p2"]; got != want {
		t.Errorf("saved project = %+v, want %+v", got, want)
	}
	if saved.ActiveProjectID != "p2" {
		t.Errorf("active project = %q, want the new one", saved.ActiveProjectID)
	}
	if app.cfg.ActiveProjectID != "p2" {
		t.Errorf("app config points at %q, want the new project", app.cfg.ActiveProjectID)
	}
}

func TestAppNewProjectReloadsOntoWhatItMade(t *testing.T) {
	capturedConfig(t)
	client := newLoadingClient()
	app := newProjectApp(t, client)
	before := len(client.queriedDSIDs)

	_, cmd := app.Update(projectCreatedMsg{structure: newStructure(), name: "tracker two", workdir: "/work"})
	feed(t, app, cmd)

	if got := client.queriedDSIDs[before:]; len(got) != 1 || got[0] != "s-ds" {
		t.Errorf("queried %v, want the new project's slices", got)
	}
	if app.project == nil || app.project.ID != "p2" {
		t.Errorf("project = %+v, want the board on the new one", app.project)
	}
}

func TestAppNewProjectStartsAConfigWithNoProjectsYet(t *testing.T) {
	saved := capturedConfig(t)
	app := NewApp(config.Config{ProjectDBDataSourceID: testProjectsDSID}, creatingClient())

	app.Update(projectCreatedMsg{structure: newStructure(), name: "first", workdir: "/work"})

	if len(saved.Projects) != 1 {
		t.Fatalf("saved projects = %+v, want the first one recorded", saved.Projects)
	}
	if app.cfg.Projects["p2"].Name != "first" {
		t.Errorf("config = %+v, want the project under its page ID", app.cfg.Projects)
	}
}

func TestAppNewProjectReportsAFailedCreation(t *testing.T) {
	capturedConfig(t)
	app := newProjectApp(t, &fakeNotion{})
	boom := errors.New("create project: boom")

	app.Update(projectCreatedMsg{err: boom})

	if app.err == nil || !errors.Is(app.err, boom) {
		t.Errorf("err = %v, want the failure reported", app.err)
	}
	if app.cfg.ActiveProjectID != testProjectID {
		t.Errorf("active project = %q, want the old one untouched", app.cfg.ActiveProjectID)
	}
	if app.busy {
		t.Error("a failed creation should leave the app idle")
	}
}

func TestAppNewProjectReportsAFailedConfigWrite(t *testing.T) {
	failingConfig(t, errors.New("read-only"))
	app := newProjectApp(t, creatingClient())

	app.Update(projectCreatedMsg{structure: newStructure(), name: "tracker two", workdir: "/work"})

	if app.err == nil || !strings.Contains(app.err.Error(), "save config: read-only") {
		t.Fatalf("err = %v, want the config failure reported", app.err)
	}
	if app.note != "" {
		t.Errorf("note = %q, want the error shown instead", app.note)
	}
	// The project exists, so the session still shows it.
	if app.cfg.ActiveProjectID != "p2" {
		t.Errorf("active project = %q, want the new one for this session", app.cfg.ActiveProjectID)
	}
}

// With no projects database there is no choice to make: N opens a form for a
// plan of nat's own rather than refusing, and does not ask where it lives.
func TestAppNewProjectWithoutAProjectsDatabaseMakesALocalProject(t *testing.T) {
	app := newWriteApp(t, &fakeNotion{})

	press(app, "N")

	f, ok := app.form.(*NewProjectForm)
	if !ok {
		t.Fatalf("form = %T, want the new-project form", app.form)
	}
	if !f.local {
		t.Error("with nowhere else to put it, the plan must be local")
	}
	if strings.Contains(f.View(), "Where will the plan live?") {
		t.Errorf("asked where the plan lives with no choice to make:\n%s", f.View())
	}
}

// Where Notion is there to be chosen, the form asks, and defaults to it.
func TestAppNewProjectAsksWhereThePlanLivesWhenThereIsAChoice(t *testing.T) {
	app := newProjectApp(t, &fakeNotion{})

	press(app, "N")

	f, ok := app.form.(*NewProjectForm)
	if !ok {
		t.Fatalf("form = %T, want the new-project form", app.form)
	}
	if f.local {
		t.Error("Notion is the default where there is a choice")
	}
	if !strings.Contains(f.View(), "Where will the plan live?") {
		t.Errorf("the choice was not asked:\n%s", f.View())
	}
}

func TestAppNewProjectIsRefusedWhileBusy(t *testing.T) {
	app := newProjectApp(t, &fakeNotion{})
	app.busy = true
	if cmd := app.newProjectFlow(); cmd != nil {
		t.Error("a write is already in flight")
	}
	if app.form != nil {
		t.Error("no form should have opened")
	}
}

// A local project is made without the client: the plan file is laid down first,
// and only then does the message that records it in config go back.
func TestCreateLocalProjectLaysDownThePlanBeforeAnythingIsRecorded(t *testing.T) {
	isolatedPlans(t)
	msg := runMsg(t, createLocalProject("mine", "Be small.", "/work")).(projectCreatedMsg)
	if msg.err != nil || msg.localID == "" || msg.structure != nil {
		t.Fatalf("msg = %+v", msg)
	}
	if msg.name != "mine" || msg.workdir != "/work" {
		t.Errorf("answers not carried through: %+v", msg)
	}
	path, err := store.LocalPath(msg.localID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the plan file is not there: %v", err)
	}
}

func TestCreateLocalProjectReportsAPlanThatCannotBeLaid(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	msg := runMsg(t, createLocalProject("mine", "", "/work")).(projectCreatedMsg)
	if msg.err == nil || msg.localID != "" {
		t.Errorf("msg = %+v, want a failure with nothing to record", msg)
	}
}

func TestLocalFormSaveCreatesALocalProject(t *testing.T) {
	isolatedPlans(t)
	f := newNewProjectForm(DefaultStyles().FormTheme, true)
	f.local, f.name, f.workdir = true, " mine ", t.TempDir()
	msg := runMsg(t, f.save(newProjectApp(t, &fakeNotion{}))).(projectCreatedMsg)
	if msg.localID == "" || msg.name != "mine" {
		t.Errorf("msg = %+v", msg)
	}
}

func TestAppRecordsALocalProjectAsLocal(t *testing.T) {
	saved := capturedConfig(t)
	app := newWriteApp(t, &fakeNotion{})
	app.busy = true
	app.projectCreated(projectCreatedMsg{localID: "loc-1", name: "mine", workdir: "/work"})
	want := config.ProjectConfig{Name: "mine", WorkingDir: "/work", Backend: config.BackendLocal}
	if got := saved.Projects["loc-1"]; got != want {
		t.Errorf("entry = %+v, want %+v", got, want)
	}
	if saved.ActiveProjectID != "loc-1" || app.busy {
		t.Errorf("active = %q, busy = %v", saved.ActiveProjectID, app.busy)
	}
}

func TestAppOnboardingHandsOverToTheNewProjectFlow(t *testing.T) {
	client := creatingClient()
	o := NewOnboarding(config.Config{}, client, func(config.Config) error { return nil })
	app := NewAppWithOnboarding(config.Config{}, client, o)

	_, cmd := app.Update(OnboardingDoneMsg{
		Config:       config.Config{ProjectDBDataSourceID: testProjectsDSID},
		NeedsProject: true,
	})
	feed(t, app, cmd)

	if _, ok := app.form.(*NewProjectForm); !ok {
		t.Fatalf("form = %T, want the new-project form", app.form)
	}
	if !strings.Contains(app.toast, "No projects yet") {
		t.Errorf("toast = %q, want the hand-over toast", app.toast)
	}
	if len(client.queriedDSIDs) != 0 {
		t.Errorf("queried %v, want no plan loaded before there is one", client.queriedDSIDs)
	}
}

func TestAppSwitchProjectPicksAnotherPlan(t *testing.T) {
	saved := capturedConfig(t)
	app := NewApp(twoProjectConfig(t), newLoadingClient())

	feed(t, app, press(app, "P"))
	if _, ok := app.form.(*SwitchProjectForm); !ok {
		t.Fatalf("form = %T, want the project picker", app.form)
	}
	view := stripANSI(app.View().Content)
	for _, want := range []string{"Switch project", "another", "tracker"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	// The picker opens on the active project, so "another" is one step up.
	press(app, "k")
	finishForm(t, app, press(app, "enter"))

	if app.toast != `Switched to "another".` {
		t.Errorf("toast = %q, want the switched toast", app.toast)
	}
	if saved.ActiveProjectID != "other" || app.cfg.ActiveProjectID != "other" {
		t.Errorf("active project = %q/%q, want the picked one", saved.ActiveProjectID, app.cfg.ActiveProjectID)
	}
}

func TestAppSwitchProjectReloadsOntoTheOtherPlan(t *testing.T) {
	capturedConfig(t)
	client := newLoadingClient()
	app := NewApp(twoProjectConfig(t), client)
	p := testProject()
	app.project = &p
	app.board.SetProject(&p)
	before := len(client.queriedDSIDs)

	_, cmd := app.Update(projectSwitchedMsg{id: "other", name: "another"})

	if app.project != nil {
		t.Error("the old plan should be dropped, not left on show")
	}
	feed(t, app, cmd)
	if got := client.queriedDSIDs[before:]; len(got) != 1 || got[0] != "o-sl" {
		t.Errorf("queried %v, want the picked project's slices", got)
	}
	if app.project == nil || app.project.Name != "another" {
		t.Errorf("project = %+v, want the board on the other plan", app.project)
	}
}

func TestAppSwitchProjectReportsAFailedConfigWrite(t *testing.T) {
	failingConfig(t, errors.New("read-only"))
	app := NewApp(twoProjectConfig(t), newLoadingClient())

	app.Update(projectSwitchedMsg{id: "other", name: "another"})

	if app.err == nil || !strings.Contains(app.err.Error(), "save config: read-only") {
		t.Fatalf("err = %v, want the config failure reported", app.err)
	}
	if app.note != "" {
		t.Errorf("note = %q, want the error shown instead", app.note)
	}
	if app.cfg.ActiveProjectID != "other" {
		t.Error("the switch should still hold for this session")
	}
}

func TestAppSwitchProjectNeedsSomewhereToSwitchTo(t *testing.T) {
	app := newWriteApp(t, &fakeNotion{})

	press(app, "P")

	if app.form != nil {
		t.Error("a picker was opened with only one project")
	}
	if want := "There is no other project to switch to — press N to add one."; app.toast != want {
		t.Errorf("toast = %q, want %q", app.toast, want)
	}
}

func TestAppSwitchProjectIsRefusedWhileAWriteIsInFlight(t *testing.T) {
	app := NewApp(twoProjectConfig(t), &fakeNotion{})
	app.busy = true

	if cmd := app.switchProjectFlow(); cmd != nil {
		t.Error("a write is already in flight")
	}
	if app.form != nil {
		t.Error("no picker should have opened")
	}
}

func TestSwitchProjectFormOrdersByNameAndFallsBackToTheID(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"z": {Name: "beta"},
		"a": {Name: "beta"},
		"b": {},
	}}

	f := newSwitchProjectForm(DefaultStyles().FormTheme, cfg)

	// The unnamed project lists under its ID, which sorts before "beta"; two
	// projects sharing a name are separated by their IDs.
	if got := sortedProjectIDs(cfg.Projects); !equal(got, []string{"b", "a", "z"}) {
		t.Errorf("order = %v, want the unnamed one first", got)
	}
	if f.names["b"] != "b" {
		t.Errorf("label = %q, want the ID standing in for a missing name", f.names["b"])
	}
	// Nothing is active, so the picker opens on the first option.
	if f.chosen != "b" {
		t.Errorf("chosen = %q, want the first option", f.chosen)
	}
}

func TestSwitchProjectFormOpensOnTheActiveProject(t *testing.T) {
	if f := newSwitchProjectForm(DefaultStyles().FormTheme, twoProjectConfig(t)); f.chosen != testProjectID {
		t.Errorf("chosen = %q, want the active project", f.chosen)
	}
}

func TestNewProjectFormAnnouncesItsWork(t *testing.T) {
	if got := newNewProjectForm(DefaultStyles().FormTheme, true).busyNote(); got != "Creating the project…" {
		t.Errorf("busy note = %q, want the creation note", got)
	}
	if got := newSwitchProjectForm(DefaultStyles().FormTheme, twoProjectConfig(t)).busyNote(); got != "" {
		t.Errorf("busy note = %q, want nothing announced for a switch", got)
	}
}

// projectPage is a row of the projects database as the picker reads it: the
// title comes back as plain text, which is what a query answers with.
func projectPage(id, name string) notion.Page {
	return notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
		notion.PropName: {Title: []notion.RichText{{PlainText: name}}},
	}}
}

// openingClient answers the projects database with the given rows and every
// project page with a plan behind it, which is the whole of what opening one
// asks of Notion.
func openingClient(pages ...notion.Page) *loadingClient {
	c := newLoadingClient()
	slices := c.query
	c.query = func(id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error) {
		if id == testProjectsDSID {
			return pages, nil
		}
		return slices(id, filter, sorts)
	}
	c.resolve = func(pageID string) (*notion.ResolvedProject, error) {
		return &notion.ResolvedProject{Name: "opened", SlicesDSID: pageID + "-sl"}, nil
	}
	return c
}

// openingApp is the board with somewhere to list projects from, which is what
// makes the picker offer more than local config knows.
func openingApp(t *testing.T, client NotionAPI) *App {
	a := NewApp(twoProjectConfig(t), client)
	a.cfg.ProjectDBDataSourceID = testProjectsDSID
	return a
}

// switchFormOf is the picker on show, failing the test when the modal open over
// the board is something else.
func switchFormOf(t *testing.T, a *App) *SwitchProjectForm {
	t.Helper()
	f, ok := a.form.(*SwitchProjectForm)
	if !ok {
		t.Fatalf("form = %T, want the project picker", a.form)
	}
	return f
}

func TestAppSwitchProjectOffersTheWorkspacesOwnProjects(t *testing.T) {
	capturedConfig(t)
	client := openingClient(projectPage("p9", "notion-only"), projectPage(testProjectID, "tracker"))
	app := openingApp(t, client)

	feed(t, app, press(app, "P"))

	f := switchFormOf(t, app)
	if !f.unopened["p9"] || f.unopened[testProjectID] {
		t.Errorf("unopened = %v, want only the page config has never seen", f.unopened)
	}
	view := stripANSI(app.View().Content)
	if want := "notion-only" + openSuffix; !strings.Contains(view, want) {
		t.Errorf("view is missing %q:\n%s", want, view)
	}
	if got := strings.Count(view, "tracker"); got != 1 {
		t.Errorf("the configured project is listed %d times, want once:\n%s", got, view)
	}
}

func TestAppSwitchProjectOpensOneItHasNeverSeen(t *testing.T) {
	saved := capturedConfig(t)
	client := openingClient(projectPage("p9", "notion-only"))
	app := openingApp(t, client)
	feed(t, app, press(app, "P"))

	// The picker opens on the active project, and the workspace's own list sits
	// below the configured ones: one step down is the unopened page.
	press(app, "j")
	finishForm(t, app, press(app, "enter"))

	if got := client.resolvedPages; !equal(got, []string{"p9"}) {
		t.Fatalf("resolved %v, want the picked page read", got)
	}
	want := config.ProjectConfig{Name: "opened", SlicesDSID: "p9-sl"}
	if got := saved.Projects["p9"]; got != want {
		t.Errorf("recorded %+v, want %+v — and no working directory", got, want)
	}
	if saved.ActiveProjectID != "p9" || app.cfg.ActiveProjectID != "p9" {
		t.Errorf("active project = %q/%q, want the opened one", saved.ActiveProjectID, app.cfg.ActiveProjectID)
	}
	if !strings.Contains(app.toast, `Opened "opened"`) || !strings.Contains(app.toast, "working directory") {
		t.Errorf("toast = %q, want the open reported and a directory asked for", app.toast)
	}
	if got := client.queriedDSIDs[len(client.queriedDSIDs)-1]; got != "p9-sl" {
		t.Errorf("last query = %q, want the board reloaded onto the opened plan", got)
	}
}

func TestAppSwitchProjectReportsAPageThatIsNoProject(t *testing.T) {
	saved := capturedConfig(t)
	client := openingClient(projectPage("p9", "notion-only"))
	client.resolve = func(string) (*notion.ResolvedProject, error) {
		return nil, &notion.NoPlanError{PageID: "p9", Title: "notion-only", Reason: `it holds no "Slices" database`}
	}
	app := openingApp(t, client)
	feed(t, app, press(app, "P"))

	press(app, "j")
	finishForm(t, app, press(app, "enter"))

	if !strings.Contains(app.toast, `Could not open "notion-only"`) ||
		!strings.Contains(app.toast, "holds no") {
		t.Errorf("toast = %q, want the refusal named", app.toast)
	}
	if _, ok := saved.Projects["p9"]; ok || app.cfg.Projects["p9"].Name != "" {
		t.Error("a page that would not resolve should leave local config alone")
	}
	if app.cfg.ActiveProjectID != testProjectID {
		t.Errorf("active project = %q, want the board left where it was", app.cfg.ActiveProjectID)
	}
}

func TestOpenProjectReportsAnEmptyAnswer(t *testing.T) {
	msg := runMsg(t, openProject(&fakeNotion{}, "p9", "notion-only")).(projectOpenedMsg)

	if msg.err == nil || !strings.Contains(msg.err.Error(), "no project came back") {
		t.Errorf("err = %v, want the empty answer reported", msg.err)
	}
}

func TestAppProjectOpenedRecordsTheFirstProjectOfAll(t *testing.T) {
	saved := capturedConfig(t)
	app := NewApp(config.Config{}, newLoadingClient())

	app.Update(projectOpenedMsg{id: "p9", project: &notion.ResolvedProject{Name: "first", SlicesDSID: "p9-sl"}})

	if got := saved.Projects["p9"]; got.SlicesDSID != "p9-sl" {
		t.Errorf("recorded %+v, want the project in a config that had no map", got)
	}
}

// The picker is worth opening with one project configured as long as there is a
// projects database to list the rest of the workspace from.
func TestAppSwitchProjectOpensOverOneConfiguredProject(t *testing.T) {
	client := openingClient(projectPage("p9", "notion-only"))
	app := newWriteApp(t, client)
	app.cfg.ProjectDBDataSourceID = testProjectsDSID

	feed(t, app, press(app, "P"))

	f := switchFormOf(t, app)
	if !f.unopened["p9"] {
		t.Errorf("unopened = %v, want the workspace's own project offered", f.unopened)
	}
	if app.toast != "" {
		t.Errorf("toast = %q, want the picker rather than a refusal", app.toast)
	}
}

// A projects database that will not answer costs the picker nothing: the
// configured projects are already in it, and switching between them is what the
// key mostly does.
func TestAppSwitchProjectSurvivesAFailedListing(t *testing.T) {
	client := openingClient()
	client.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
		return nil, errors.New("no")
	}
	app := openingApp(t, client)

	feed(t, app, press(app, "P"))

	f := switchFormOf(t, app)
	if len(f.options) != 2 || len(f.unopened) != 0 {
		t.Errorf("options = %d, unopened = %v, want the configured projects alone", len(f.options), f.unopened)
	}
	if app.toast != "" {
		t.Errorf("toast = %q, want the failure logged rather than shown", app.toast)
	}
}

// An answer that arrives after the picker has been closed has nothing to fill
// in, and must not be mistaken for another form's message.
func TestAppWorkspaceProjectsAfterThePickerClosedAreDropped(t *testing.T) {
	app := NewApp(twoProjectConfig(t), newLoadingClient())

	app.Update(workspaceProjectsMsg{projects: []workspaceProject{{ID: "p9", Name: "notion-only"}}})

	if app.form != nil {
		t.Errorf("form = %T, want nothing opened by a stray answer", app.form)
	}
}

func TestSwitchProjectFormOffersWorkspaceProjectsUnderTheConfiguredOnes(t *testing.T) {
	f := newSwitchProjectForm(DefaultStyles().FormTheme, twoProjectConfig(t))

	f.offer([]workspaceProject{
		{ID: testProjectID, Name: "tracker"},
		{ID: "p9"},
	})

	if len(f.options) != 3 {
		t.Fatalf("options = %d, want the one unconfigured page added", len(f.options))
	}
	if got := f.options[2].Key; got != "p9"+openSuffix {
		t.Errorf("label = %q, want the untitled page listed under its ID", got)
	}
	if f.names["p9"] != "p9" {
		t.Errorf("name = %q, want the ID standing in for a missing title", f.names["p9"])
	}
}

// A machine that has opened nothing yet still gets a picker: what it offers is
// on its way from the projects database.
func TestSwitchProjectFormOpensEmpty(t *testing.T) {
	f := newSwitchProjectForm(DefaultStyles().FormTheme, config.Config{})

	if f.chosen != "" || len(f.options) != 0 {
		t.Fatalf("chosen = %q, options = %d, want an empty picker", f.chosen, len(f.options))
	}
	f.offer([]workspaceProject{{ID: "p9", Name: "notion-only"}})
	if f.chosen != "p9" {
		t.Errorf("chosen = %q, want the first project offered", f.chosen)
	}
}

func TestSwitchProjectFormAnnouncesAnOpen(t *testing.T) {
	f := newSwitchProjectForm(DefaultStyles().FormTheme, twoProjectConfig(t))
	f.offer([]workspaceProject{{ID: "p9", Name: "notion-only"}})
	f.chosen = "p9"

	if got := f.busyNote(); got != "Opening the project…" {
		t.Errorf("busy note = %q, want the open announced", got)
	}
}

func TestListWorkspaceProjectsOrdersByNameThenID(t *testing.T) {
	client := &fakeNotion{query: func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
		return []notion.Page{projectPage("z", "beta"), projectPage("b", ""), projectPage("a", "beta")}, nil
	}}

	msg := runMsg(t, listWorkspaceProjects(client, testProjectsDSID)).(workspaceProjectsMsg)

	if msg.err != nil {
		t.Fatalf("err = %v, want the rows read", msg.err)
	}
	got := []string{msg.projects[0].ID, msg.projects[1].ID, msg.projects[2].ID}
	if !equal(got, []string{"b", "a", "z"}) {
		t.Errorf("order = %v, want the untitled one first and the ID breaking the tie", got)
	}
}

func TestListWorkspaceProjectsReportsAFailedRead(t *testing.T) {
	client := &fakeNotion{query: func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
		return nil, errors.New("boom")
	}}

	msg := runMsg(t, listWorkspaceProjects(client, testProjectsDSID)).(workspaceProjectsMsg)

	if msg.err == nil || !strings.Contains(msg.err.Error(), "read the projects database: boom") {
		t.Errorf("err = %v, want the read failure named", msg.err)
	}
}

func TestBoardWithNoProjectPointsAtTheNewProjectKey(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	if view := app.View().Content; !strings.Contains(view, "Press N to create one") {
		t.Errorf("view = %q, want the new-project hint", view)
	}
}

// Answering yes to the assignee question reproduces the older schema, which is
// what a project with more than one person working it wants.
func TestAppNewProjectTracksAnAssigneeWhenAsked(t *testing.T) {
	capturedConfig(t)
	client := creatingClient()
	client.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) { return nil, nil }
	app := newProjectApp(t, client)

	feed(t, app, press(app, "N"))
	fillProjectForm(t, app, "tracker two", "", t.TempDir(), true)

	if len(client.createdProjects) != 1 || !client.createdProjects[0].assignee {
		t.Errorf("created %+v, want one project with an assignee column", client.createdProjects)
	}
}

// Choosing a local file takes the form straight past the assignee question — a
// plan with no directory of users has no Assignee column to ask about — and
// records a project that touched Notion nowhere.
func TestAppNewProjectChosenLocalSkipsTheAssigneeQuestion(t *testing.T) {
	isolatedPlans(t)
	saved := capturedConfig(t)
	client := &fakeNotion{}
	app := newProjectApp(t, client)

	feed(t, app, press(app, "N"))
	feed(t, app, press(app, "down"))
	answerWhere(t, app)
	typeText(app, "mine")
	feed(t, app, press(app, "enter"))
	typeText(app, "Be small.")
	feed(t, app, press(app, "tab"))
	dir := t.TempDir()
	typeText(app, dir)
	finishForm(t, app, press(app, "enter"))

	if len(client.createdProjects) != 0 || len(client.appended) != 0 {
		t.Errorf("Notion was written to: %+v %+v", client.createdProjects, client.appended)
	}
	var id string
	for k, p := range saved.Projects {
		if p.Name == "mine" {
			id = k
			if !p.IsLocal() || p.WorkingDir != dir || p.SlicesDSID != "" {
				t.Errorf("entry = %+v", p)
			}
		}
	}
	if id == "" || saved.ActiveProjectID != id {
		t.Errorf("the new project was not recorded and made active: %+v", saved)
	}
}

// A project kept in a file loads through that file alone: no workspace is
// mirrored.
func TestAppLoadsALocalProjectFromItsFileAlone(t *testing.T) {
	isolatedPlans(t)
	local := config.ProjectConfig{Name: "mine", Backend: config.BackendLocal}
	if err := store.CreateLocalProject(t.Context(), store.ProjectOf("loc-1", local), ""); err != nil {
		t.Fatal(err)
	}
	client := &fakeNotion{}
	app := NewApp(config.Config{
		ActiveProjectID: "loc-1",
		Projects:        map[string]config.ProjectConfig{"loc-1": local},
	}, client)

	for _, msg := range run(app.startLoad(false)) {
		app.Update(msg)
	}
	if _, ok := app.stores["loc-1"].(*store.Local); !ok {
		t.Errorf("store = %T, want the plan file itself", app.stores["loc-1"])
	}
	if app.project == nil || app.project.Name != "mine" {
		t.Errorf("project = %+v", app.project)
	}
}
