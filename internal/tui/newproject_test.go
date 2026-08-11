package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// testProjectsDSID is the projects data source onboarding leaves behind, which
// is what a new project is created under.
const testProjectsDSID = "projects-ds"

// newStructure is what a successful CreateProject comes back with.
func newStructure() *notion.ProjectStructure {
	return &notion.ProjectStructure{
		PageID:         "p2",
		PageURL:        "https://notion.so/p2",
		MilestonesDBID: "m-db",
		MilestonesDSID: "m-ds",
		SlicesDBID:     "s-db",
		SlicesDSID:     "s-ds",
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
func newProjectApp(client NotionAPI) *App {
	a := newWriteApp(client)
	a.cfg.ProjectDBDataSourceID = testProjectsDSID
	return a
}

// twoProjectConfig is a config holding a second project, so there is something
// to switch between. It is named to sort before the active one.
func twoProjectConfig() config.Config {
	cfg := testConfig()
	cfg.Projects["other"] = config.ProjectConfig{
		Name: "another", MilestonesDSID: "o-ms", SlicesDSID: "o-sl",
	}
	return cfg
}

func TestCreateProjectBuildsTheProjectAndItsPageBody(t *testing.T) {
	client := creatingClient()

	msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "Line one.\n\nLine two.", "/work")).(projectCreatedMsg)

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

	runMsg(t, createProject(client, testProjectsDSID, "tracker", "   \n\n  ", "/work"))

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

			msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "blurb", "/work")).(projectCreatedMsg)

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

	msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "blurb", "/work")).(projectCreatedMsg)

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

	msg := runMsg(t, createProject(client, testProjectsDSID, "tracker", "blurb", "/work")).(projectCreatedMsg)

	if msg.structure == nil {
		t.Fatal("the project exists, so it should still be recorded")
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "write project page: boom") {
		t.Errorf("err = %v, want the body failure reported", msg.err)
	}
}

func TestAppNewProjectFlowWritesConfigAndReloads(t *testing.T) {
	saved := capturedConfig(t)
	client := creatingClient()
	client.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) { return nil, nil }
	app := newProjectApp(client)
	dir := t.TempDir()

	feed(t, app, press(app, "N"))
	if app.form == nil || app.screen != screenForm {
		t.Fatalf("screen = %v, form = %v, want the new-project form", app.screen, app.form)
	}
	view := stripANSI(app.View().Content)
	for _, want := range []string{"New project", "Name", "Info", "Working directory"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	fillForm(t, app, "tracker two", "The conventions.", dir)

	if app.err != nil {
		t.Fatalf("err = %v, want a clean creation", app.err)
	}
	if app.toast != `Created "tracker two".` {
		t.Errorf("toast = %q, want the created toast", app.toast)
	}
	want := config.ProjectConfig{
		Name: "tracker two", MilestonesDSID: "m-ds", SlicesDSID: "s-ds", WorkingDir: dir,
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
	app := newProjectApp(client)
	before := len(client.queriedDSIDs)

	_, cmd := app.Update(projectCreatedMsg{structure: newStructure(), name: "tracker two", workdir: "/work"})
	feed(t, app, cmd)

	if got := client.queriedDSIDs[before:]; len(got) != 2 || got[0] != "m-ds" || got[1] != "s-ds" {
		t.Errorf("queried %v, want the new project's data sources", got)
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
	app := newProjectApp(&fakeNotion{})
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
	app := newProjectApp(creatingClient())

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

func TestAppNewProjectNeedsAProjectsDatabase(t *testing.T) {
	app := newWriteApp(&fakeNotion{})

	press(app, "N")

	if app.form != nil {
		t.Error("a form was opened with nowhere to create under")
	}
	if !strings.Contains(app.toast, "No projects database is configured") {
		t.Errorf("toast = %q, want the missing-database toast", app.toast)
	}
}

func TestAppNewProjectIsRefusedWithNothingToCreateWith(t *testing.T) {
	tests := []struct {
		name string
		app  func() *App
	}{
		{"no client", func() *App { return newProjectApp(nil) }},
		{"a write already in flight", func() *App {
			a := newProjectApp(&fakeNotion{})
			a.busy = true
			return a
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := tt.app()
			if cmd := app.newProjectFlow(); cmd != nil {
				t.Error("there is nothing to create with")
			}
			if app.form != nil {
				t.Error("no form should have opened")
			}
		})
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
	app := NewApp(twoProjectConfig(), newLoadingClient())

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
	app := NewApp(twoProjectConfig(), client)
	p := testProject()
	app.project = &p
	app.board.SetProject(&p)
	before := len(client.queriedDSIDs)

	_, cmd := app.Update(projectSwitchedMsg{id: "other", name: "another"})

	if app.project != nil {
		t.Error("the old plan should be dropped, not left on show")
	}
	feed(t, app, cmd)
	if got := client.queriedDSIDs[before:]; len(got) != 2 || got[0] != "o-ms" || got[1] != "o-sl" {
		t.Errorf("queried %v, want the picked project's data sources", got)
	}
	if app.project == nil || app.project.Name != "another" {
		t.Errorf("project = %+v, want the board on the other plan", app.project)
	}
}

func TestAppSwitchProjectReportsAFailedConfigWrite(t *testing.T) {
	failingConfig(t, errors.New("read-only"))
	app := NewApp(twoProjectConfig(), newLoadingClient())

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
	app := newWriteApp(&fakeNotion{})

	press(app, "P")

	if app.form != nil {
		t.Error("a picker was opened with only one project")
	}
	if want := "There is no other project to switch to — press N to add one."; app.toast != want {
		t.Errorf("toast = %q, want %q", app.toast, want)
	}
}

func TestAppSwitchProjectIsRefusedWhileAWriteIsInFlight(t *testing.T) {
	app := NewApp(twoProjectConfig(), &fakeNotion{})
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
	if f := newSwitchProjectForm(DefaultStyles().FormTheme, twoProjectConfig()); f.chosen != testProjectID {
		t.Errorf("chosen = %q, want the active project", f.chosen)
	}
}

func TestNewProjectFormAnnouncesItsWork(t *testing.T) {
	if got := newNewProjectForm(DefaultStyles().FormTheme).busyNote(); got != "Creating the project…" {
		t.Errorf("busy note = %q, want the creation note", got)
	}
	if got := newSwitchProjectForm(DefaultStyles().FormTheme, twoProjectConfig()).busyNote(); got != "" {
		t.Errorf("busy note = %q, want nothing announced for a switch", got)
	}
}

func TestBoardWithNoProjectPointsAtTheNewProjectKey(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	if view := app.View().Content; !strings.Contains(view, "Press N to create one") {
		t.Errorf("view = %q, want the new-project hint", view)
	}
}
