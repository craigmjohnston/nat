package tui

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeNotion is a NotionAPI whose every call is a field, so each test can
// supply only the behaviour it cares about. Unset calls return nothing.
type fakeNotion struct {
	users       func() ([]notion.User, error)
	search      func(query, filterType string) ([]notion.SearchResult, error)
	getDB       func(id string) (*notion.Database, error)
	createDB    func(parentPageID, title string) (*notion.Database, error)
	newProject  func(projectsDSID, name string) (*notion.ProjectStructure, error)
	query       func(id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error)
	blocks      func(id string) ([]notion.Block, error)
	createPage  func(parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	updatePage  func(pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	appendBlock func(id string, children []map[string]any) ([]notion.Block, error)
	deleteBlock func(id string) error
	trashPage   func(pageID string) error

	// mu guards fetchedDBs: databaseParents fetches concurrently.
	mu            sync.Mutex
	searchFilters []string
	fetchedDBs    []string
	queriedDSIDs  []string
	blockParents  []string
	createdUnder  string
	createdTitle  string
	// The projects created, as the data source and name each was asked for.
	createdProjects []createProjectCall

	// The writes the mutation flows make, in the order they were made.
	created  []createPageCall
	updated  []updatePageCall
	appended []appendCall
	deleted  []string
	trashed  []string
}

// The calls fakeNotion records, so a test can assert on exactly what was sent.
type (
	createPageCall struct {
		parent     notion.Parent
		properties map[string]notion.PropertyValue
		children   []map[string]any
	}
	updatePageCall struct {
		pageID     string
		properties map[string]notion.PropertyValue
	}
	createProjectCall struct {
		projectsDSID string
		name         string
	}
	appendCall struct {
		pageID   string
		children []map[string]any
	}
)

var _ NotionAPI = (*fakeNotion)(nil)

func (f *fakeNotion) ListUsers(context.Context) ([]notion.User, error) {
	if f.users == nil {
		return nil, nil
	}
	return f.users()
}

func (f *fakeNotion) Search(_ context.Context, query, filterType string) ([]notion.SearchResult, error) {
	f.searchFilters = append(f.searchFilters, filterType)
	if f.search == nil {
		return nil, nil
	}
	return f.search(query, filterType)
}

func (f *fakeNotion) GetDatabase(_ context.Context, id string) (*notion.Database, error) {
	f.mu.Lock()
	f.fetchedDBs = append(f.fetchedDBs, id)
	f.mu.Unlock()
	if f.getDB == nil {
		return nil, errors.New("no database")
	}
	return f.getDB(id)
}

func (f *fakeNotion) CreateProjectsDatabase(_ context.Context, parentPageID, title string) (*notion.Database, error) {
	f.createdUnder, f.createdTitle = parentPageID, title
	if f.createDB == nil {
		return nil, nil
	}
	return f.createDB(parentPageID, title)
}

func (f *fakeNotion) CreateProject(_ context.Context, projectsDSID, name string) (*notion.ProjectStructure, error) {
	f.createdProjects = append(f.createdProjects, createProjectCall{projectsDSID: projectsDSID, name: name})
	if f.newProject == nil {
		return nil, nil
	}
	return f.newProject(projectsDSID, name)
}

func (f *fakeNotion) QueryDataSource(_ context.Context, id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error) {
	f.queriedDSIDs = append(f.queriedDSIDs, id)
	if f.query == nil {
		return nil, nil
	}
	return f.query(id, filter, sorts)
}

func (f *fakeNotion) GetBlockChildren(_ context.Context, id string) ([]notion.Block, error) {
	f.blockParents = append(f.blockParents, id)
	if f.blocks == nil {
		return nil, nil
	}
	return f.blocks(id)
}

func (f *fakeNotion) CreatePage(_ context.Context, parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error) {
	f.created = append(f.created, createPageCall{parent: parent, properties: properties, children: children})
	if f.createPage == nil {
		return &notion.Page{ID: "new-page"}, nil
	}
	return f.createPage(parent, properties, children)
}

func (f *fakeNotion) UpdatePageProperties(_ context.Context, pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error) {
	f.updated = append(f.updated, updatePageCall{pageID: pageID, properties: properties})
	if f.updatePage == nil {
		return &notion.Page{ID: pageID}, nil
	}
	return f.updatePage(pageID, properties)
}

func (f *fakeNotion) AppendBlockChildren(_ context.Context, id string, children []map[string]any) ([]notion.Block, error) {
	f.appended = append(f.appended, appendCall{pageID: id, children: children})
	if f.appendBlock == nil {
		return nil, nil
	}
	return f.appendBlock(id, children)
}

func (f *fakeNotion) DeleteBlock(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	if f.deleteBlock == nil {
		return nil
	}
	return f.deleteBlock(id)
}

func (f *fakeNotion) TrashPage(_ context.Context, pageID string) error {
	f.trashed = append(f.trashed, pageID)
	if f.trashPage == nil {
		return nil
	}
	return f.trashPage(pageID)
}

// harness drives an Onboarding the way the Bubble Tea runtime would: it feeds
// key presses in and runs every command the model returns, threading the
// resulting messages back through Update.
type harness struct {
	t      *testing.T
	m      *Onboarding
	client *fakeNotion

	saved   []config.Config
	saveErr error
	done    []OnboardingDoneMsg
	quit    bool
}

func newHarness(t *testing.T, cfg config.Config, client *fakeNotion) *harness {
	t.Helper()
	h := &harness{t: t, client: client}
	h.m = NewOnboarding(cfg, client, func(c config.Config) error {
		h.saved = append(h.saved, c)
		return h.saveErr
	})
	return h
}

// run executes cmd and everything it spawns until the model settles.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			h.t.Fatal("command loop did not settle")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil || blinkCmd(c) {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		if list, ok := cmdList(msg); ok {
			queue = append(queue, list...)
			continue
		}
		switch msg := msg.(type) {
		case OnboardingDoneMsg:
			h.done = append(h.done, msg)
			continue
		case tea.QuitMsg:
			h.quit = true
			continue
		}
		var next tea.Cmd
		h.m, next = h.m.Update(msg)
		queue = append(queue, next)
	}
}

// send feeds one message to the model and runs the commands it returns.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	m, cmd := h.m.Update(msg)
	h.m = m
	h.run(cmd)
}

// typeText types into the focused input, one key press at a time.
func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r), ShiftedCode: r}))
	}
}

// submit presses enter, which moves off the current field and, on the last
// one, completes the form.
func (h *harness) submit() {
	h.t.Helper()
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
}

// down moves the selection down one option.
func (h *harness) down() {
	h.t.Helper()
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
}

// blinkCmd reports whether cmd is the text cursor's blink command. It waits on
// a channel only the Bubble Tea runtime feeds, so running it here would block
// forever — and blinking never affects the wizard's state.
func blinkCmd(cmd tea.Cmd) bool {
	fn := runtime.FuncForPC(reflect.ValueOf(cmd).Pointer())
	return fn != nil && strings.Contains(fn.Name(), "/cursor.")
}

// cmdList reports whether a message is one of Bubble Tea's command carriers
// (tea.BatchMsg or the unexported sequence message), returning the commands it
// holds. Both are named slices of tea.Cmd, which a type switch cannot see, so
// this looks at the underlying type instead.
func cmdList(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeOf(tea.Cmd(nil)) {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i], _ = v.Index(i).Interface().(tea.Cmd)
	}
	return cmds, true
}

// dataSourceHit builds a data source search hit with its parent database.
func dataSourceHit(id, dbID, title string) notion.SearchResult {
	return notion.SearchResult{
		Object: notion.SearchDataSource,
		ID:     id,
		Title:  []notion.RichText{{PlainText: title}},
		Parent: notion.Parent{Type: "database_id", DatabaseID: dbID},
	}
}

// pageHit builds a page search hit, whose title lives in its properties the way
// Notion really reports it.
func pageHit(id, title string) notion.SearchResult {
	name, err := json.Marshal(map[string]any{
		"type":  "title",
		"title": []map[string]any{{"plain_text": title}},
	})
	if err != nil {
		panic(err)
	}
	return notion.SearchResult{
		Object:     notion.SearchPage,
		ID:         id,
		Properties: map[string]json.RawMessage{"Name": name},
	}
}

func person(id, name, email string) notion.User {
	return notion.User{ID: id, Name: name, Type: notion.UserPerson, Person: &notion.Person{Email: email}}
}

func TestOnboardingPicksAnExistingProjectDatabase(t *testing.T) {
	client := &fakeNotion{
		search: func(_, filterType string) ([]notion.SearchResult, error) {
			return []notion.SearchResult{
				dataSourceHit("ds-1", "db-1", "Agent Projects"),
				dataSourceHit("ds-2", "db-2", "Recipes"),
			}, nil
		},
		users: func() ([]notion.User, error) {
			return []notion.User{
				{ID: "bot-1", Name: "Tracker", Type: notion.UserBot},
				person("user-1", "Craig Johnston", "craig@example.com"),
			}, nil
		},
		query: func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
			return []notion.Page{{ID: "project-1"}}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	if h.m.step != stepPickProjectDB {
		t.Fatalf("step = %v, want stepPickProjectDB (err: %v)", h.m.step, h.m.err)
	}
	h.down()   // move from "Agent Projects" to "Recipes"
	h.submit() // project database

	if h.m.step != stepPickAssignee {
		t.Fatalf("step = %v, want stepPickAssignee (err: %v)", h.m.step, h.m.err)
	}
	h.submit() // assignee — only one person, already selected

	if h.m.err != nil {
		t.Fatalf("unexpected error: %v", h.m.err)
	}
	if len(h.saved) != 1 {
		t.Fatalf("saved %d configs, want 1", len(h.saved))
	}
	want := config.Config{
		ProjectDBID:           "db-2",
		ProjectDBDataSourceID: "ds-2",
		AssigneeUserID:        "user-1",
		AssigneeUserName:      "Craig Johnston",
		Projects:              map[string]config.ProjectConfig{},
	}
	if !reflect.DeepEqual(h.saved[0], want) {
		t.Errorf("saved config = %+v, want %+v", h.saved[0], want)
	}
	if len(h.done) != 1 || h.done[0].NeedsProject || !reflect.DeepEqual(h.done[0].Config, want) {
		t.Errorf("done = %+v, want one message with the saved config and NeedsProject=false", h.done)
	}
	if got := h.client.queriedDSIDs; len(got) != 1 || got[0] != "ds-2" {
		t.Errorf("queried data sources = %v, want [ds-2]", got)
	}
	if got := h.client.searchFilters; !reflect.DeepEqual(got, []string{notion.SearchDataSource, notion.SearchPage}) {
		t.Errorf("search filters = %v, want the databases and the pages", got)
	}
}

func TestOnboardingCreatesAProjectDatabase(t *testing.T) {
	client := &fakeNotion{
		search: func(_, filterType string) ([]notion.SearchResult, error) {
			if filterType == notion.SearchPage {
				return []notion.SearchResult{pageHit("page-1", "Workspace")}, nil
			}
			return nil, nil
		},
		createDB: func(string, string) (*notion.Database, error) {
			return &notion.Database{ID: "db-new", DataSources: []notion.DataSourceRef{{ID: "ds-new"}}}, nil
		},
		users: func() ([]notion.User, error) {
			return []notion.User{person("user-1", "Craig Johnston", "")}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	// No existing databases, so the only option is "create a new database".
	if h.m.step != stepPickProjectDB {
		t.Fatalf("step = %v, want stepPickProjectDB (err: %v)", h.m.step, h.m.err)
	}
	h.submit() // project database

	if h.m.step != stepNewProjectDB {
		t.Fatalf("step = %v, want stepNewProjectDB (err: %v)", h.m.step, h.m.err)
	}
	h.submit() // keep the default database name
	h.submit() // parent page
	h.submit() // assignee

	if h.m.err != nil {
		t.Fatalf("unexpected error: %v", h.m.err)
	}
	if h.client.createdUnder != "page-1" || h.client.createdTitle != ProjectsDBTitle {
		t.Errorf("created %q under %q, want %q under page-1", h.client.createdTitle, h.client.createdUnder, ProjectsDBTitle)
	}
	if len(h.saved) != 1 || h.saved[0].ProjectDBID != "db-new" || h.saved[0].ProjectDBDataSourceID != "ds-new" {
		t.Fatalf("saved = %+v, want the new database's IDs", h.saved)
	}
	if len(h.done) != 1 || !h.done[0].NeedsProject {
		t.Errorf("done = %+v, want NeedsProject=true for an empty project database", h.done)
	}
}

func TestOnboardingPicksADatabaseNestedInTheTree(t *testing.T) {
	// The database the user wants sits under a page: they open the page in the
	// tree, land on the database, and the config comes out exactly as if it had
	// been picked off a flat list.
	client := &fakeNotion{
		search: func(_, filterType string) ([]notion.SearchResult, error) {
			if filterType == notion.SearchPage {
				return []notion.SearchResult{pageHit("page-1", "Home")}, nil
			}
			return []notion.SearchResult{dataSourceHit("ds-1", "db-1", "Agent Projects")}, nil
		},
		getDB: func(id string) (*notion.Database, error) {
			return &notion.Database{ID: id, Parent: notion.PageParent("page-1")}, nil
		},
		users: func() ([]notion.User, error) {
			return []notion.User{person("user-1", "Craig Johnston", "")}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	if h.m.tree == nil {
		t.Fatalf("want the tree at step %v (err: %v)", h.m.step, h.m.err)
	}
	if view := stripANSI(h.m.View()); strings.Contains(view, "Agent Projects") {
		t.Fatalf("the database should be folded away under its page:\n%s", view)
	}
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})) // open Home
	h.down()                                             // onto the database
	h.submit()                                           // choose it

	if h.m.step != stepPickAssignee {
		t.Fatalf("step = %v, want stepPickAssignee (err: %v)", h.m.step, h.m.err)
	}
	h.submit() // assignee

	if len(h.saved) != 1 {
		t.Fatalf("saved %d configs, want 1 (err: %v)", len(h.saved), h.m.err)
	}
	if got := h.saved[0]; got.ProjectDBID != "db-1" || got.ProjectDBDataSourceID != "ds-1" {
		t.Errorf("saved = %+v, want db-1/ds-1 — the same IDs a flat pick saved", got)
	}
}

func TestOnboardingCreateNewWithoutPagesFails(t *testing.T) {
	// The escape hatch needs a page to put the database under; a workspace
	// sharing none is the errNoPages dead end, found at the moment of choice.
	h := newHarness(t, config.Config{}, &fakeNotion{})
	h.run(h.m.Init())

	if h.m.tree == nil {
		t.Fatalf("want the tree at step %v (err: %v)", h.m.step, h.m.err)
	}
	h.submit() // the only row is "Create a new database…"

	if h.m.err == nil || !strings.Contains(h.m.err.Error(), "no pages found in this workspace") {
		t.Fatalf("err = %v, want errNoPages", h.m.err)
	}
}

func TestOnboardingSizesTheTreeToTheWindow(t *testing.T) {
	client := &fakeNotion{
		search: func(string, string) ([]notion.SearchResult, error) {
			return []notion.SearchResult{dataSourceHit("ds-1", "db-1", "Agent Projects")}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)

	// A size that arrives before the tree exists is kept for it.
	h.send(tea.WindowSizeMsg{Width: 25, Height: 30})
	h.run(h.m.Init())
	if h.m.tree == nil {
		t.Fatalf("want the tree (err: %v)", h.m.err)
	}
	if h.m.tree.width != 25 || h.m.tree.height != 30 {
		t.Errorf("tree size = %dx%d, want the earlier window's 25x30", h.m.tree.width, h.m.tree.height)
	}

	// A resize while the tree is up reaches it too.
	h.send(tea.WindowSizeMsg{Width: 20, Height: 10})
	if h.m.tree.width != 20 || h.m.tree.height != 10 {
		t.Errorf("tree size = %dx%d, want the resize's 20x10", h.m.tree.width, h.m.tree.height)
	}
	if got := lipgloss.Width(h.m.View()); got > 20 {
		t.Errorf("view is %d columns in a 20-column window", got)
	}
}

func TestOnboardingTreeIgnoresOtherMessages(t *testing.T) {
	client := &fakeNotion{
		search: func(string, string) ([]notion.SearchResult, error) {
			return []notion.SearchResult{dataSourceHit("ds-1", "db-1", "Agent Projects")}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	h.send(spinnerLikeMsg{})
	if h.m.tree == nil || h.m.err != nil {
		t.Errorf("a stray message should leave the tree alone (err: %v)", h.m.err)
	}
}

// spinnerLikeMsg is any message the tree has no business reacting to.
type spinnerLikeMsg struct{}

func TestOnboardingKeepsExistingConfigFields(t *testing.T) {
	// Onboarding can run again over a partially written config file; the rest
	// of it must survive.
	cfg := config.Config{
		ActiveProjectID: "project-1",
		Projects: map[string]config.ProjectConfig{
			"project-1": {Name: "notion-agent-tracker"},
		},
	}
	client := &fakeNotion{
		search: func(string, string) ([]notion.SearchResult, error) {
			return []notion.SearchResult{dataSourceHit("ds-1", "db-1", "Agent Projects")}, nil
		},
		users: func() ([]notion.User, error) {
			return []notion.User{person("user-1", "Craig Johnston", "")}, nil
		},
	}
	h := newHarness(t, cfg, client)
	h.run(h.m.Init())
	h.submit()
	h.submit()

	if len(h.saved) != 1 {
		t.Fatalf("saved %d configs, want 1 (err: %v)", len(h.saved), h.m.err)
	}
	got := h.saved[0]
	if got.ActiveProjectID != "project-1" || len(got.Projects) != 1 {
		t.Errorf("saved config = %+v, want the existing project entries preserved", got)
	}
}

func TestOnboardingTrimsTheDatabaseName(t *testing.T) {
	client := &fakeNotion{
		search: func(_, filterType string) ([]notion.SearchResult, error) {
			if filterType == notion.SearchPage {
				return []notion.SearchResult{pageHit("page-1", "Workspace")}, nil
			}
			return nil, nil
		},
		createDB: func(string, string) (*notion.Database, error) {
			return &notion.Database{ID: "db-new", DataSources: []notion.DataSourceRef{{ID: "ds-new"}}}, nil
		},
		users: func() ([]notion.User, error) {
			return []notion.User{person("user-1", "Craig", "")}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	h.submit() // create a new database
	h.typeText("  Tracked  ")
	h.submit()
	h.submit() // parent page

	if got := h.client.createdTitle; strings.TrimSpace(got) != got || !strings.Contains(got, "Tracked") {
		t.Errorf("created title = %q, want the typed name, trimmed", got)
	}
}

func TestOnboardingAbortQuits(t *testing.T) {
	h := newHarness(t, config.Config{}, &fakeNotion{})
	h.run(h.m.Init())
	h.send(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))

	if !h.quit {
		t.Error("aborting the tree should quit the program")
	}
}

func TestOnboardingAbortingAFormQuits(t *testing.T) {
	client := &fakeNotion{
		search: func(_, filterType string) ([]notion.SearchResult, error) {
			if filterType == notion.SearchPage {
				return []notion.SearchResult{pageHit("page-1", "Workspace")}, nil
			}
			return nil, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())
	h.submit() // create a new database — the huh form takes over

	if h.m.form == nil {
		t.Fatalf("want the form at step %v (err: %v)", h.m.step, h.m.err)
	}
	if view := stripANSI(h.m.View()); !strings.Contains(view, "Name for the new project database") {
		t.Errorf("view = %q, want the form on show", view)
	}
	h.send(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if !h.quit {
		t.Error("aborting the form should quit the program")
	}
}

func TestOnboardingFatalErrors(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
		want string
	}{
		{"the load fails", projectDBsMsg{err: errors.New("search for databases: boom")}, "search for databases: boom"},
		{"create fails", projectDBCreatedMsg{err: errors.New("boom")}, "create project database: boom"},
		{"created without a data source", projectDBCreatedMsg{db: &notion.Database{ID: "db"}}, "no data source"},
		{"listing users fails", usersMsg{err: errors.New("boom")}, "list users: boom"},
		{"no people in the workspace", usersMsg{users: []notion.User{{ID: "bot", Type: notion.UserBot}}}, "no people found in this workspace"},
		{"querying projects fails", projectsCheckedMsg{err: errors.New("boom")}, "query projects: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, config.Config{}, &fakeNotion{})
			h.send(tt.msg)

			if h.m.err == nil || !strings.Contains(h.m.err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", h.m.err, tt.want)
			}
			if h.m.form != nil {
				t.Error("a fatal error should leave no form on show")
			}
			view := h.m.View()
			if !strings.Contains(view, "Onboarding failed") || !strings.Contains(view, tt.want) {
				t.Errorf("view = %q, want the failure and %q", view, tt.want)
			}
			// Further messages are ignored once the wizard has failed.
			h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if len(h.done) != 0 {
				t.Errorf("done = %+v, want none after a failure", h.done)
			}
		})
	}
}

func TestOnboardingSaveFailureIsFatal(t *testing.T) {
	h := newHarness(t, config.Config{}, &fakeNotion{})
	h.saveErr = errors.New("read-only")
	h.send(projectsCheckedMsg{count: 1})

	if h.m.err == nil || !strings.Contains(h.m.err.Error(), "save config: read-only") {
		t.Fatalf("err = %v, want the save failure", h.m.err)
	}
	if len(h.done) != 0 {
		t.Errorf("done = %+v, want none when the config could not be saved", h.done)
	}
}

func TestOnboardingIgnoresMessagesWithNoFormOnShow(t *testing.T) {
	h := newHarness(t, config.Config{}, &fakeNotion{})
	h.m.form = nil
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if h.m.err != nil {
		t.Errorf("unexpected error: %v", h.m.err)
	}
}

func TestOnboardingViewShowsTheTreeOnShow(t *testing.T) {
	client := &fakeNotion{
		search: func(string, string) ([]notion.SearchResult, error) {
			return []notion.SearchResult{dataSourceHit("ds-1", "db-1", "Agent Projects")}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	if h.m.tree == nil {
		t.Fatalf("want the tree on show at step %v (err: %v)", h.m.step, h.m.err)
	}
	got := stripANSI(h.m.View())
	for _, want := range []string{"Agent Projects", "Create a new database…", "Which database holds your projects?"} {
		if !strings.Contains(got, want) {
			t.Errorf("view = %q, want it to hold %q", got, want)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("view = %q, want a trailing newline", got)
	}
}

func TestOnboardingViewShowsTheStatusOfACallInFlight(t *testing.T) {
	h := newHarness(t, config.Config{}, &fakeNotion{})
	h.m.await(stepLoadingUsers, "Loading workspace users…", nil)

	if got := h.m.View(); !strings.Contains(got, "Loading workspace users…") {
		t.Errorf("view = %q, want the status line", got)
	}
}

func TestOnboardingFormSubmittedIgnoresAsyncSteps(t *testing.T) {
	// Guards the switch's default: only form steps have a submission to act on.
	h := newHarness(t, config.Config{}, &fakeNotion{})
	h.m.step = stepCheckingProjects
	m, cmd := h.m.formSubmitted()

	if cmd != nil || m.step != stepCheckingProjects {
		t.Errorf("step = %v, cmd = %v, want no transition", m.step, cmd)
	}
}

func TestOnboardingCommands(t *testing.T) {
	t.Run("loadProjectDBs searches data sources then pages", func(t *testing.T) {
		client := &fakeNotion{
			search: func(query, filterType string) ([]notion.SearchResult, error) {
				if query != "" {
					t.Errorf("search query = %q, want an unfiltered search", query)
				}
				if filterType == notion.SearchPage {
					return []notion.SearchResult{pageHit("page-1", "Home")}, nil
				}
				return []notion.SearchResult{dataSourceHit("ds-1", "db-1", "Projects")}, nil
			},
			getDB: func(id string) (*notion.Database, error) {
				return &notion.Database{ID: id, Parent: notion.PageParent("page-1")}, nil
			},
		}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client

		msg, _ := h.m.loadProjectDBs().(projectDBsMsg)
		if len(msg.results) != 1 || len(msg.pages) != 1 || msg.err != nil {
			t.Errorf("msg = %+v, want one database and one page", msg)
		}
		if got := msg.parents["db-1"].PageID; got != "page-1" {
			t.Errorf("parents[db-1] = %q, want page-1", got)
		}
		if got := client.searchFilters; !reflect.DeepEqual(got, []string{notion.SearchDataSource, notion.SearchPage}) {
			t.Errorf("search filters = %v, want data sources then pages", got)
		}
	})

	t.Run("loadProjectDBs reports the database search failing", func(t *testing.T) {
		client := &fakeNotion{search: func(_, filterType string) ([]notion.SearchResult, error) {
			return nil, errors.New("boom")
		}}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client

		msg, _ := h.m.loadProjectDBs().(projectDBsMsg)
		if msg.err == nil || !strings.Contains(msg.err.Error(), "search for databases: boom") {
			t.Errorf("err = %v, want the database search failure", msg.err)
		}
	})

	t.Run("loadProjectDBs reports the page search failing", func(t *testing.T) {
		client := &fakeNotion{search: func(_, filterType string) ([]notion.SearchResult, error) {
			if filterType == notion.SearchPage {
				return nil, errors.New("boom")
			}
			return nil, nil
		}}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client

		msg, _ := h.m.loadProjectDBs().(projectDBsMsg)
		if msg.err == nil || !strings.Contains(msg.err.Error(), "search for pages: boom") {
			t.Errorf("err = %v, want the page search failure", msg.err)
		}
	})

	t.Run("databaseParents fetches each parent database once", func(t *testing.T) {
		client := &fakeNotion{getDB: func(id string) (*notion.Database, error) {
			if id == "db-lost" {
				return nil, errors.New("boom")
			}
			return &notion.Database{ID: id, Parent: notion.PageParent("page-" + id)}, nil
		}}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client

		parents := h.m.databaseParents(context.Background(), []notion.SearchResult{
			dataSourceHit("ds-1", "db-1", "One"),
			dataSourceHit("ds-2", "db-1", "One again"),
			dataSourceHit("ds-3", "db-lost", "Broken"),
			{ID: "ds-4", Object: notion.SearchDataSource}, // no parent database at all
		})

		if want := map[string]notion.Parent{"db-1": notion.PageParent("page-db-1")}; !reflect.DeepEqual(parents, want) {
			t.Errorf("parents = %+v, want %+v — a failed fetch leaves its database out", parents, want)
		}
		sort.Strings(client.fetchedDBs)
		if want := []string{"db-1", "db-lost"}; !reflect.DeepEqual(client.fetchedDBs, want) {
			t.Errorf("fetched %v, want each parent database once: %v", client.fetchedDBs, want)
		}
	})

	t.Run("loadUsers passes the error through", func(t *testing.T) {
		client := &fakeNotion{users: func() ([]notion.User, error) { return nil, errors.New("boom") }}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client

		msg, _ := h.m.loadUsers().(usersMsg)
		if msg.err == nil {
			t.Error("want the list error")
		}
	})

	t.Run("checkProjects queries the configured data source", func(t *testing.T) {
		client := &fakeNotion{query: func(_ string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error) {
			if filter != nil || sorts != nil {
				t.Errorf("query(filter=%v, sorts=%v), want an unfiltered query", filter, sorts)
			}
			return []notion.Page{{ID: "p1"}, {ID: "p2"}}, nil
		}}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client
		h.m.cfg.ProjectDBDataSourceID = "ds-9"

		msg, _ := h.m.checkProjects().(projectsCheckedMsg)
		if msg.count != 2 || msg.err != nil {
			t.Errorf("msg = %+v, want a count of 2", msg)
		}
		if got := client.queriedDSIDs; len(got) != 1 || got[0] != "ds-9" {
			t.Errorf("queried %v, want [ds-9]", got)
		}
	})

	t.Run("createProjectDB reports the failure", func(t *testing.T) {
		client := &fakeNotion{createDB: func(string, string) (*notion.Database, error) {
			return nil, errors.New("boom")
		}}
		h := newHarness(t, config.Config{}, client)
		h.m.client = client

		msg, _ := h.m.createProjectDB().(projectDBCreatedMsg)
		if msg.err == nil {
			t.Error("want the create error")
		}
	})
}

func TestSetProjectDBWithoutAParentDatabase(t *testing.T) {
	m := &Onboarding{projectDBs: []notion.SearchResult{{ID: "ds-1"}}}
	m.setProjectDB("ds-unknown")

	if m.cfg.ProjectDBDataSourceID != "ds-unknown" || m.cfg.ProjectDBID != "" {
		t.Errorf("cfg = %+v, want the data source ID and no database ID", m.cfg)
	}
}

func TestSetAssigneeWithoutAMatchingUser(t *testing.T) {
	m := &Onboarding{users: []notion.User{person("user-1", "Craig", "")}}
	m.setAssignee("user-2")

	if m.cfg.AssigneeUserID != "user-2" || m.cfg.AssigneeUserName != "" {
		t.Errorf("cfg = %+v, want the ID and no name", m.cfg)
	}
}

func TestLabels(t *testing.T) {
	if got := resultLabel(dataSourceHit("ds-1", "db-1", "Projects")); got != "Projects" {
		t.Errorf("resultLabel = %q, want Projects", got)
	}
	if got := resultLabel(notion.SearchResult{ID: "ds-1"}); got != "(untitled) ds-1" {
		t.Errorf("resultLabel = %q, want the untitled fallback", got)
	}
	if got := userLabel(person("u1", "Craig", "craig@example.com")); got != "Craig <craig@example.com>" {
		t.Errorf("userLabel = %q, want name and email", got)
	}
	if got := userLabel(person("u1", "Craig", "")); got != "Craig" {
		t.Errorf("userLabel = %q, want just the name", got)
	}
	if got := userLabel(notion.User{ID: "u1", Type: notion.UserPerson}); got != "u1" {
		t.Errorf("userLabel = %q, want the ID fallback", got)
	}
}

func TestRequired(t *testing.T) {
	v := required("a name")
	if err := v("  "); err == nil || err.Error() != "please enter a name" {
		t.Errorf("err = %v, want the prompt", err)
	}
	if err := v(" x "); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDefaultNewClient(t *testing.T) {
	if c := DefaultNewClient(func() (string, error) { return "ntn_secret", nil }); c == nil {
		t.Error("want a client")
	}
}
