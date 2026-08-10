package tui

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeNotion is a NotionAPI whose every call is a field, so each test can
// supply only the behaviour it cares about. Unset calls return nothing.
type fakeNotion struct {
	me          func() (*notion.User, error)
	searchPaged func(query, filterType, cursor string) ([]notion.SearchResult, string, error)
	pageEntries func(id string) ([]notion.PageEntry, error)
	getDB       func(id string) (*notion.Database, error)
	breadcrumb  func(parent notion.Parent) []string
	createDB    func(parentPageID, title string) (*notion.Database, error)
	newProject  func(projectsDSID, name string) (*notion.ProjectStructure, error)
	query       func(id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error)
	blocks      func(id string) ([]notion.Block, error)
	createPage  func(parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	updatePage  func(pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	appendBlock func(id string, children []map[string]any) ([]notion.Block, error)
	deleteBlock func(id string) error
	trashPage   func(pageID string) error

	searchFilters []string
	searchCursors []string
	entriesFor    []string
	fetchedDBs    []string
	queriedDSIDs  []string
	crumbParents  []notion.Parent
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

func (f *fakeNotion) Me(context.Context) (*notion.User, error) {
	if f.me == nil {
		return nil, errors.New("no token owner")
	}
	return f.me()
}

func (f *fakeNotion) SearchPaged(_ context.Context, query, filterType, cursor string) ([]notion.SearchResult, string, error) {
	f.searchFilters = append(f.searchFilters, filterType)
	f.searchCursors = append(f.searchCursors, cursor)
	if f.searchPaged == nil {
		return nil, "", nil
	}
	return f.searchPaged(query, filterType, cursor)
}

func (f *fakeNotion) PageEntries(_ context.Context, id string) ([]notion.PageEntry, error) {
	f.entriesFor = append(f.entriesFor, id)
	if f.pageEntries == nil {
		return nil, nil
	}
	return f.pageEntries(id)
}

func (f *fakeNotion) GetDatabase(_ context.Context, id string) (*notion.Database, error) {
	f.fetchedDBs = append(f.fetchedDBs, id)
	if f.getDB == nil {
		return nil, errors.New("no database")
	}
	return f.getDB(id)
}

func (f *fakeNotion) Breadcrumb(_ context.Context, parent notion.Parent) []string {
	f.crumbParents = append(f.crumbParents, parent)
	if f.breadcrumb == nil {
		return nil
	}
	return f.breadcrumb(parent)
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

// expand opens the page under the tree's cursor, running the fetch it costs.
func (h *harness) expand() {
	h.t.Helper()
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
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

// rootPage builds a search hit for a page sitting at the workspace root, whose
// title lives in its properties the way Notion really reports it.
func rootPage(id, title string) notion.SearchResult {
	p := pageHit(id, title)
	p.Parent = notion.Parent{Type: "workspace"}
	return p
}

// pageHit builds a page search hit with no parent set.
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

// tokenOf is what GET /users/me returns for a personal access token: a bot
// user naming the person it acts for.
func tokenOf(owner notion.User) func() (*notion.User, error) {
	return func() (*notion.User, error) {
		return &notion.User{ID: "bot-1", Name: "Notion CLI", Type: notion.UserBot,
			Bot: &notion.Bot{Owner: &notion.BotOwner{Type: notion.OwnerUser, User: &owner}}}, nil
	}
}

// singleDBClient is a workspace with one root page holding one database, the
// shortest path through the picker: expand Home, step onto the database,
// choose it.
func singleDBClient() *fakeNotion {
	return &fakeNotion{
		searchPaged: func(_, _, _ string) ([]notion.SearchResult, string, error) {
			return []notion.SearchResult{rootPage("page-1", "Home")}, "", nil
		},
		pageEntries: func(id string) ([]notion.PageEntry, error) {
			return []notion.PageEntry{{ID: "db-1", Title: "Agent Projects", Database: true}}, nil
		},
		getDB: func(id string) (*notion.Database, error) {
			return &notion.Database{ID: id, DataSources: []notion.DataSourceRef{{ID: "ds-1"}}}, nil
		},
		me: tokenOf(person("user-1", "Craig Johnston", "")),
	}
}

func TestOnboardingPicksADatabaseInsideAPage(t *testing.T) {
	client := &fakeNotion{
		searchPaged: func(_, _, _ string) ([]notion.SearchResult, string, error) {
			return []notion.SearchResult{rootPage("page-1", "Home")}, "", nil
		},
		pageEntries: func(id string) ([]notion.PageEntry, error) {
			return []notion.PageEntry{
				{ID: "db-1", Title: "Tracker", Database: true},
				{ID: "db-2", Title: "Agent Projects", Database: true},
			}, nil
		},
		getDB: func(id string) (*notion.Database, error) {
			return &notion.Database{ID: id, DataSources: []notion.DataSourceRef{{ID: "ds-2"}}}, nil
		},
		me: tokenOf(person("user-1", "Craig Johnston", "craig@example.com")),
		query: func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
			return []notion.Page{{ID: "project-1"}}, nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	if h.m.step != stepPickProjectDB {
		t.Fatalf("step = %v, want stepPickProjectDB (err: %v)", h.m.step, h.m.err)
	}
	if got := client.fetchedDBs; len(got) != 0 {
		t.Fatalf("fetched databases %v before anything was selected", got)
	}
	h.expand() // open Home, fetching its contents
	h.down()   // onto Tracker
	h.down()   // onto Agent Projects
	h.submit() // choose it — the one GetDatabase call

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
	if got := h.client.entriesFor; !reflect.DeepEqual(got, []string{"page-1"}) {
		t.Errorf("fetched entries for %v, want the expanded page only", got)
	}
	if got := h.client.fetchedDBs; !reflect.DeepEqual(got, []string{"db-2"}) {
		t.Errorf("fetched databases %v, want the chosen one only, at selection time", got)
	}
	if got := h.client.queriedDSIDs; len(got) != 1 || got[0] != "ds-2" {
		t.Errorf("queried data sources = %v, want [ds-2]", got)
	}
	if got := h.client.searchFilters; !reflect.DeepEqual(got, []string{notion.SearchPage}) {
		t.Errorf("search filters = %v, want one page search", got)
	}
}

func TestOnboardingStreamsRootPagesIntoThePicker(t *testing.T) {
	client := &fakeNotion{
		searchPaged: func(_, _, cursor string) ([]notion.SearchResult, string, error) {
			if cursor == "" {
				row := pageHit("row-1", "A database row")
				row.Parent = notion.Parent{Type: "data_source_id", DataSourceID: "ds-9"}
				return []notion.SearchResult{rootPage("page-1", "Home"), row}, "cur-1", nil
			}
			return []notion.SearchResult{rootPage("page-2", "Docs")}, "", nil
		},
	}
	h := newHarness(t, config.Config{}, client)

	// The first batch is on show before the search has finished.
	m, cmd := h.m.Update(h.m.Init()())
	h.m = m
	view := stripANSI(h.m.View())
	if !strings.Contains(view, "Home") || strings.Contains(view, "Docs") {
		t.Fatalf("view after the first batch should show Home and not yet Docs:\n%s", view)
	}
	if h.m.rootsDone {
		t.Fatal("the search is not done while a cursor remains")
	}

	h.run(cmd)
	if !h.m.rootsDone {
		t.Fatal("the search should be done once the cursor runs out")
	}
	view = stripANSI(h.m.View())
	if !strings.Contains(view, "Docs") {
		t.Errorf("view should show the second batch:\n%s", view)
	}
	if strings.Contains(view, "A database row") {
		t.Errorf("a database row must not become a root:\n%s", view)
	}
	if got := h.client.searchCursors; !reflect.DeepEqual(got, []string{"", "cur-1"}) {
		t.Errorf("search cursors = %v, want the stream followed", got)
	}
	rows := h.m.tree.rows
	if got := rows[len(rows)-1].value; got != createNewChoice {
		t.Errorf("last row = %q, want the create-new escape hatch kept last", rows[len(rows)-1].label)
	}
}

func TestOnboardingCreatesAProjectDatabase(t *testing.T) {
	client := &fakeNotion{
		searchPaged: func(_, _, _ string) ([]notion.SearchResult, string, error) {
			return []notion.SearchResult{rootPage("page-1", "Workspace")}, "", nil
		},
		createDB: func(string, string) (*notion.Database, error) {
			return &notion.Database{ID: "db-new", DataSources: []notion.DataSourceRef{{ID: "ds-new"}}}, nil
		},
		me: tokenOf(person("user-1", "Craig Johnston", "")),
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	if h.m.step != stepPickProjectDB {
		t.Fatalf("step = %v, want stepPickProjectDB (err: %v)", h.m.step, h.m.err)
	}
	h.down()   // off the Workspace page, onto "create a new database"
	h.submit() // choose it

	if h.m.step != stepNewProjectDB {
		t.Fatalf("step = %v, want stepNewProjectDB (err: %v)", h.m.step, h.m.err)
	}
	h.submit() // keep the default database name
	h.submit() // parent page — the assignee comes off the token, unprompted

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

func TestOnboardingCreateNewWaitsForTheSearch(t *testing.T) {
	// Choosing the escape hatch before any page has streamed in is not yet a
	// dead end: the search may still surface one, so the choice is a no-op
	// rather than a failure.
	h := newHarness(t, config.Config{}, &fakeNotion{})
	m, _ := h.m.Update(rootPagesMsg{cursor: "cur-1"}) // first batch: nothing yet
	h.m = m

	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if h.m.err != nil || h.m.tree == nil {
		t.Fatalf("choosing create-new mid-search should keep the tree (err: %v)", h.m.err)
	}

	// The search ends with a page; the same choice now opens the form.
	h.send(rootPagesMsg{results: []notion.SearchResult{rootPage("page-1", "Home")}})
	h.down()
	h.submit()
	if h.m.step != stepNewProjectDB || h.m.form == nil {
		t.Fatalf("step = %v, want the new-database form (err: %v)", h.m.step, h.m.err)
	}
}

func TestOnboardingExpandFailureIsFatal(t *testing.T) {
	client := singleDBClient()
	client.pageEntries = func(string) ([]notion.PageEntry, error) {
		return nil, errors.New("boom")
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())
	h.expand()

	if h.m.err == nil || !strings.Contains(h.m.err.Error(), "load the page's contents: boom") {
		t.Fatalf("err = %v, want the fetch failure", h.m.err)
	}
}

func TestOnboardingNestedPagesExpandFurther(t *testing.T) {
	client := singleDBClient()
	client.pageEntries = func(id string) ([]notion.PageEntry, error) {
		if id == "page-1" {
			return []notion.PageEntry{{ID: "page-2"}}, nil // an untitled subpage
		}
		return []notion.PageEntry{{ID: "db-1", Title: "Agent Projects", Database: true}}, nil
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	h.expand() // Home
	h.down()   // onto the subpage
	if view := stripANSI(h.m.View()); !strings.Contains(view, "(untitled) page-2") {
		t.Fatalf("an untitled subpage should be listed by ID:\n%s", view)
	}
	h.expand() // fetch the subpage's contents
	h.down()   // onto the database
	h.submit() // choose it

	if got := h.client.entriesFor; !reflect.DeepEqual(got, []string{"page-1", "page-2"}) {
		t.Errorf("fetched entries for %v, want each expanded page once", got)
	}
	if h.m.err != nil || h.m.step != stepDone {
		t.Errorf("step = %v (err: %v), want the flow to run to the end", h.m.step, h.m.err)
	}
}

func TestOnboardingLateMessagesAfterThePickAreIgnored(t *testing.T) {
	// The rest of the search stream, and a page fetch still in flight, can
	// land after the database is picked; they must not disturb the next step.
	h := newHarness(t, config.Config{}, singleDBClient())
	h.run(h.m.Init())
	h.expand()
	h.down()
	h.submit() // picked: the tree is gone

	if h.m.tree != nil {
		t.Fatal("the tree should be gone after the pick")
	}
	step := h.m.step
	h.send(rootPagesMsg{results: []notion.SearchResult{rootPage("page-9", "Late")}, cursor: "cur-9"})
	h.send(pageEntriesMsg{node: &treeNode{}, entries: []notion.PageEntry{{ID: "db-9"}}})

	if h.m.err != nil || h.m.step != step {
		t.Errorf("step = %v (err: %v), want the late messages ignored", h.m.step, h.m.err)
	}
	// The lapsed search asked for no further pages.
	if got := h.client.searchCursors; !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("search cursors = %v, want no follow-up after the pick", got)
	}
}

func TestOnboardingResolveFailures(t *testing.T) {
	t.Run("the fetch fails", func(t *testing.T) {
		client := singleDBClient()
		client.getDB = func(string) (*notion.Database, error) { return nil, errors.New("boom") }
		h := newHarness(t, config.Config{}, client)
		h.run(h.m.Init())
		h.expand()
		h.down()
		h.submit()

		if h.m.err == nil || !strings.Contains(h.m.err.Error(), "load the chosen database: boom") {
			t.Fatalf("err = %v, want the resolve failure", h.m.err)
		}
	})

	t.Run("the database has no data source", func(t *testing.T) {
		client := singleDBClient()
		client.getDB = func(id string) (*notion.Database, error) { return &notion.Database{ID: id}, nil }
		h := newHarness(t, config.Config{}, client)
		h.run(h.m.Init())
		h.expand()
		h.down()
		h.submit()

		if h.m.err == nil || !strings.Contains(h.m.err.Error(), "no data source") {
			t.Fatalf("err = %v, want errNoDataSource", h.m.err)
		}
	})
}

func TestOnboardingSizesTheTreeToTheWindow(t *testing.T) {
	h := newHarness(t, config.Config{}, singleDBClient())

	// A size that arrives before the search answers still reaches the tree.
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
	h := newHarness(t, config.Config{}, singleDBClient())
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
	h := newHarness(t, cfg, singleDBClient())
	h.run(h.m.Init())
	h.expand()
	h.down()
	h.submit() // the database
	h.submit() // the assignee

	if len(h.saved) != 1 {
		t.Fatalf("saved %d configs, want 1 (err: %v)", len(h.saved), h.m.err)
	}
	got := h.saved[0]
	if got.ActiveProjectID != "project-1" || len(got.Projects) != 1 {
		t.Errorf("saved config = %+v, want the existing project entries preserved", got)
	}
	if got.ProjectDBID != "db-1" || got.ProjectDBDataSourceID != "ds-1" {
		t.Errorf("saved config = %+v, want db-1/ds-1 recorded", got)
	}
}

func TestOnboardingTrimsTheDatabaseName(t *testing.T) {
	client := &fakeNotion{
		searchPaged: func(_, _, _ string) ([]notion.SearchResult, string, error) {
			return []notion.SearchResult{rootPage("page-1", "Workspace")}, "", nil
		},
		createDB: func(string, string) (*notion.Database, error) {
			return &notion.Database{ID: "db-new", DataSources: []notion.DataSourceRef{{ID: "ds-new"}}}, nil
		},
		me: tokenOf(person("user-1", "Craig", "")),
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())

	h.down()
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
		searchPaged: func(_, _, _ string) ([]notion.SearchResult, string, error) {
			return []notion.SearchResult{rootPage("page-1", "Workspace")}, "", nil
		},
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())
	h.down()
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
		{"the search fails", rootPagesMsg{err: errors.New("search for pages: boom")}, "search for pages: boom"},
		{"a page fetch fails", pageEntriesMsg{err: errors.New("boom")}, "load the page's contents: boom"},
		{"the resolve fails", projectDBResolvedMsg{err: errors.New("boom")}, "load the chosen database: boom"},
		{"resolved without a data source", projectDBResolvedMsg{db: &notion.Database{ID: "db"}}, "no data source"},
		{"create fails", projectDBCreatedMsg{err: errors.New("boom")}, "create project database: boom"},
		{"created without a data source", projectDBCreatedMsg{db: &notion.Database{ID: "db"}}, "no data source"},
		{"identifying the token's owner fails", assigneeMsg{err: errors.New("boom")}, "identify the token's owner: boom"},
		{"a token owned by the workspace", assigneeMsg{me: &notion.User{Type: notion.UserBot,
			Bot: &notion.Bot{Owner: &notion.BotOwner{Type: notion.OwnerWorkspace}}}}, "owned by the workspace rather than by a person"},
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
	h.m.tree = nil
	h.m.form = nil
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if h.m.err != nil {
		t.Errorf("unexpected error: %v", h.m.err)
	}
}

func TestOnboardingViewShowsTheTreeOnShow(t *testing.T) {
	h := newHarness(t, config.Config{}, singleDBClient())
	h.run(h.m.Init())

	if h.m.tree == nil {
		t.Fatalf("want the tree on show at step %v (err: %v)", h.m.step, h.m.err)
	}
	got := stripANSI(h.m.View())
	for _, want := range []string{"Home", "Create a new database…", "Which database holds your projects?"} {
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
	h.m.await(stepResolvingAssignee, "Identifying you…", nil)

	if got := h.m.View(); !strings.Contains(got, "Identifying you…") {
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
	t.Run("searchRoots asks for pages from the given cursor", func(t *testing.T) {
		client := &fakeNotion{
			searchPaged: func(query, filterType, cursor string) ([]notion.SearchResult, string, error) {
				if query != "" {
					t.Errorf("search query = %q, want an unfiltered search", query)
				}
				if filterType != notion.SearchPage {
					t.Errorf("search filter = %q, want pages", filterType)
				}
				return []notion.SearchResult{rootPage("page-1", "Home")}, "cur-2", nil
			},
		}
		h := newHarness(t, config.Config{}, client)

		msg, _ := h.m.searchRoots("cur-1")().(rootPagesMsg)
		if len(msg.results) != 1 || msg.cursor != "cur-2" || msg.err != nil {
			t.Errorf("msg = %+v, want one page and the next cursor", msg)
		}
		if got := client.searchCursors; !reflect.DeepEqual(got, []string{"cur-1"}) {
			t.Errorf("search cursors = %v, want the one given", got)
		}
	})

	t.Run("searchRoots reports the search failing", func(t *testing.T) {
		client := &fakeNotion{searchPaged: func(_, _, _ string) ([]notion.SearchResult, string, error) {
			return nil, "", errors.New("boom")
		}}
		h := newHarness(t, config.Config{}, client)

		msg, _ := h.m.searchRoots("")().(rootPagesMsg)
		if msg.err == nil || !strings.Contains(msg.err.Error(), "search for pages: boom") {
			t.Errorf("err = %v, want the search failure", msg.err)
		}
	})

	t.Run("loadPageEntries fetches the node's page", func(t *testing.T) {
		client := &fakeNotion{pageEntries: func(id string) ([]notion.PageEntry, error) {
			return []notion.PageEntry{{ID: "db-1", Database: true}}, nil
		}}
		h := newHarness(t, config.Config{}, client)
		node := &treeNode{label: "Home", pageID: "page-1"}

		msg, _ := h.m.loadPageEntries(node)().(pageEntriesMsg)
		if msg.node != node || len(msg.entries) != 1 || msg.err != nil {
			t.Errorf("msg = %+v, want the node and its entry", msg)
		}
		if got := client.entriesFor; !reflect.DeepEqual(got, []string{"page-1"}) {
			t.Errorf("fetched entries for %v, want [page-1]", got)
		}
	})

	t.Run("resolveProjectDB fetches the chosen database", func(t *testing.T) {
		client := &fakeNotion{getDB: func(id string) (*notion.Database, error) {
			return &notion.Database{ID: id}, nil
		}}
		h := newHarness(t, config.Config{}, client)
		h.m.chosenDBID = "db-7"

		msg, _ := h.m.resolveProjectDB().(projectDBResolvedMsg)
		if msg.err != nil || msg.db.ID != "db-7" {
			t.Errorf("msg = %+v, want db-7", msg)
		}
	})

	t.Run("loadAssignee reads the token's owner", func(t *testing.T) {
		h := newHarness(t, config.Config{}, &fakeNotion{me: tokenOf(person("user-1", "Craig", ""))})

		msg, _ := h.m.loadAssignee().(assigneeMsg)
		owner, ok := msg.me.OwnerPerson()
		if msg.err != nil || !ok || owner.ID != "user-1" {
			t.Errorf("msg = %+v, want the owning person", msg)
		}
	})

	t.Run("loadAssignee passes the error through", func(t *testing.T) {
		client := &fakeNotion{me: func() (*notion.User, error) { return nil, errors.New("boom") }}
		h := newHarness(t, config.Config{}, client)

		msg, _ := h.m.loadAssignee().(assigneeMsg)
		if msg.err == nil {
			t.Error("want the error")
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

		msg, _ := h.m.createProjectDB().(projectDBCreatedMsg)
		if msg.err == nil {
			t.Error("want the create error")
		}
	})
}

func TestLabels(t *testing.T) {
	if got := resultLabel(rootPage("page-1", "Projects")); got != "Projects" {
		t.Errorf("resultLabel = %q, want Projects", got)
	}
	if got := resultLabel(notion.SearchResult{ID: "page-1"}); got != "(untitled) page-1" {
		t.Errorf("resultLabel = %q, want the untitled fallback", got)
	}
	if got := entryLabel(notion.PageEntry{ID: "db-1", Title: "Slices"}); got != "Slices" {
		t.Errorf("entryLabel = %q, want Slices", got)
	}
	if got := entryLabel(notion.PageEntry{ID: "db-1"}); got != "(untitled) db-1" {
		t.Errorf("entryLabel = %q, want the untitled fallback", got)
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

// slash opens the search view over the tree.
func (h *harness) slash() {
	h.t.Helper()
	h.send(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
}

// escape closes the search view, returning to the tree.
func (h *harness) escape() {
	h.t.Helper()
	h.send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
}

// searchableClient is a workspace whose search finds two same-named databases
// in different places — the case browsing cannot tell apart.
func searchableClient() *fakeNotion {
	client := singleDBClient()
	client.searchPaged = func(query, filterType, _ string) ([]notion.SearchResult, string, error) {
		if filterType == notion.SearchPage {
			return []notion.SearchResult{rootPage("page-1", "Home")}, "", nil
		}
		hits := []notion.SearchResult{dsHit("ds-1", "Ops Board", "db-1"), dsHit("ds-2", "Ops Board", "db-2")}
		if query == "ops b" {
			hits = hits[1:]
		}
		return hits, "", nil
	}
	client.breadcrumb = func(parent notion.Parent) []string {
		return []string{"Engineering", parent.DatabaseID}
	}
	client.getDB = func(id string) (*notion.Database, error) {
		return &notion.Database{ID: id, DataSources: []notion.DataSourceRef{{ID: "ds-for-" + id}}}, nil
	}
	return client
}

func TestOnboardingSearchSelectsADatabase(t *testing.T) {
	client := searchableClient()
	h := newHarness(t, config.Config{}, client)
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.run(h.m.Init())
	h.slash()

	if h.m.search == nil {
		t.Fatalf("want the search view (err: %v)", h.m.err)
	}
	if h.m.search.width != 80 || h.m.search.height != 24 {
		t.Errorf("search size = %dx%d, want the window measured before it opened",
			h.m.search.width, h.m.search.height)
	}
	// The view opens on the unfiltered query rather than an empty list, and
	// every visible hit carries the trail of pages above it.
	view := stripANSI(h.m.View())
	for _, want := range []string{"Ops Board — Engineering / db-1", "Ops Board — Engineering / db-2"} {
		if !strings.Contains(view, want) {
			t.Errorf("view = %q, want it to hold %q", view, want)
		}
	}
	if got := client.crumbParents; len(got) != 2 {
		t.Errorf("resolved %d trails, want one per visible hit", len(got))
	}

	h.down()   // onto the second Ops Board
	h.submit() // choose it — the assignee comes off the token, unprompted

	if h.m.err != nil {
		t.Fatalf("unexpected error: %v", h.m.err)
	}
	if len(h.saved) != 1 {
		t.Fatalf("saved %d configs, want 1", len(h.saved))
	}
	// Selection from search records the same pair of IDs as the tree does: the
	// database that holds the hit, fetched for its data source.
	if got := h.saved[0]; got.ProjectDBID != "db-2" || got.ProjectDBDataSourceID != "ds-for-db-2" {
		t.Errorf("saved config = %+v, want db-2/ds-for-db-2", got)
	}
	if got := client.fetchedDBs; !reflect.DeepEqual(got, []string{"db-2"}) {
		t.Errorf("fetched databases %v, want the chosen one only", got)
	}
	if got := client.searchFilters; !reflect.DeepEqual(got, []string{notion.SearchPage, notion.SearchDataSource}) {
		t.Errorf("search filters = %v, want the browse then the search", got)
	}
}

func TestOnboardingSearchNarrowsAsTheQueryChanges(t *testing.T) {
	client := searchableClient()
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())
	h.slash()
	h.typeText("ops b")

	if h.m.search.query != "ops b" {
		t.Fatalf("query = %q, want the typed text", h.m.search.query)
	}
	view := stripANSI(h.m.View())
	if strings.Contains(view, "db-1") || !strings.Contains(view, "db-2") {
		t.Errorf("want the list narrowed to the one match:\n%s", view)
	}
	// One search per keystroke, plus the unfiltered one the view opened on.
	if got := len(client.searchFilters); got != len("ops b")+2 {
		t.Errorf("made %d searches, want one per keystroke", got)
	}
	// The trails are resolved once per hit, not once per search.
	if got := len(client.crumbParents); got != 2 {
		t.Errorf("resolved %d trails, want one per distinct hit", got)
	}
}

func TestOnboardingSearchDropsOvertakenAnswers(t *testing.T) {
	h := newHarness(t, config.Config{}, searchableClient())
	h.run(h.m.Init())
	h.slash()
	h.typeText("ops")

	h.send(dsSearchMsg{query: "op", results: []notion.SearchResult{dsHit("ds-9", "Stale", "db-9")}})
	if view := stripANSI(h.m.View()); strings.Contains(view, "Stale") {
		t.Errorf("an answer to an earlier query must not land:\n%s", view)
	}
}

func TestOnboardingSearchDropsHitsWithNoDatabase(t *testing.T) {
	// A data source is opened through the database that holds it, so a hit
	// reporting none cannot be chosen and must not be offered.
	h := newHarness(t, config.Config{}, searchableClient())
	h.run(h.m.Init())
	h.slash()
	h.send(dsSearchMsg{results: []notion.SearchResult{
		{ID: "ds-9", Title: []notion.RichText{{PlainText: "Orphan"}}},
		dsHit("ds-1", "Ops Board", "db-1"),
	}})

	if got := len(h.m.search.results); got != 1 {
		t.Fatalf("results = %d, want the orphan dropped", got)
	}
	if view := stripANSI(h.m.View()); strings.Contains(view, "Orphan") {
		t.Errorf("want the orphan off the list:\n%s", view)
	}
}

func TestOnboardingSearchFailureIsNotFatal(t *testing.T) {
	client := searchableClient()
	client.searchPaged = func(_, filterType, _ string) ([]notion.SearchResult, string, error) {
		if filterType == notion.SearchDataSource {
			return nil, "", errors.New("boom")
		}
		return []notion.SearchResult{rootPage("page-1", "Home")}, "", nil
	}
	h := newHarness(t, config.Config{}, client)
	h.run(h.m.Init())
	h.slash()

	if h.m.err != nil || h.m.search == nil {
		t.Fatalf("a failed search should stay in the search view (err: %v)", h.m.err)
	}
	if view := stripANSI(h.m.View()); !strings.Contains(view, "search for databases: boom") {
		t.Errorf("want the failure shown in place:\n%s", view)
	}

	// Typing is the retry, and clears the message on its way.
	client.searchPaged = func(_, _, _ string) ([]notion.SearchResult, string, error) {
		return []notion.SearchResult{dsHit("ds-1", "Ops Board", "db-1")}, "", nil
	}
	h.typeText("o")
	if view := stripANSI(h.m.View()); strings.Contains(view, "boom") || !strings.Contains(view, "Ops Board") {
		t.Errorf("want the retry's hits:\n%s", view)
	}
}

func TestOnboardingSearchEscapeReturnsToTheTree(t *testing.T) {
	h := newHarness(t, config.Config{}, searchableClient())
	h.run(h.m.Init())
	h.expand() // open Home, so the tree has expansion state to keep
	h.down()   // onto the database inside it

	before := h.m.tree.cursor
	h.slash()
	h.down() // move about in the search, which the tree must not follow
	h.escape()

	if h.m.search != nil || h.m.tree == nil {
		t.Fatalf("escape should return to the tree (err: %v)", h.m.err)
	}
	if h.m.tree.cursor != before {
		t.Errorf("tree cursor = %d, want the %d it was left on", h.m.tree.cursor, before)
	}
	view := stripANSI(h.m.View())
	if !strings.Contains(view, "Agent Projects") {
		t.Errorf("want the expanded page still open:\n%s", view)
	}
	// The tree is live again: choosing the database it was left on carries on.
	h.submit()
	if h.m.err != nil || h.m.step != stepDone {
		t.Errorf("step = %v (err: %v), want the flow to run to the end", h.m.step, h.m.err)
	}
}

func TestOnboardingSearchAbortQuits(t *testing.T) {
	h := newHarness(t, config.Config{}, searchableClient())
	h.run(h.m.Init())
	h.slash()
	h.send(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))

	if !h.quit {
		t.Error("aborting the search should quit the program")
	}
}

func TestOnboardingSearchResizeResolvesTheRowsItRevealed(t *testing.T) {
	client := searchableClient()
	client.searchPaged = func(_, filterType, _ string) ([]notion.SearchResult, string, error) {
		if filterType == notion.SearchPage {
			return []notion.SearchResult{rootPage("page-1", "Home")}, "", nil
		}
		return []notion.SearchResult{
			dsHit("ds-1", "One", "db-1"),
			dsHit("ds-2", "Two", "db-2"),
			dsHit("ds-3", "Three", "db-3"),
		}, "", nil
	}
	h := newHarness(t, config.Config{}, client)
	h.send(tea.WindowSizeMsg{Width: 80, Height: searchChromeHeight + 1})
	h.run(h.m.Init())
	h.slash()

	if got := len(client.crumbParents); got != 1 {
		t.Fatalf("resolved %d trails, want only the one row that fits", got)
	}
	h.send(tea.WindowSizeMsg{Width: 80, Height: searchChromeHeight + 3})
	if got := len(client.crumbParents); got != 3 {
		t.Errorf("resolved %d trails, want the rows the taller window revealed", got)
	}
	if h.m.search.height != searchChromeHeight+3 {
		t.Errorf("search height = %d, want the resize", h.m.search.height)
	}
}

func TestOnboardingSearchIgnoresMessagesOnceItIsGone(t *testing.T) {
	// A search still in flight, and a trail still being walked, can land after
	// the database has been picked; neither must disturb the next step.
	h := newHarness(t, config.Config{}, searchableClient())
	h.run(h.m.Init())
	h.slash()
	h.submit() // choose the first hit — the search view is gone

	if h.m.search != nil || h.m.tree != nil {
		t.Fatalf("the picker should be gone after the pick (err: %v)", h.m.err)
	}
	step := h.m.step
	h.send(dsSearchMsg{results: []notion.SearchResult{dsHit("ds-9", "Late", "db-9")}})
	h.send(breadcrumbMsg{id: "ds-9", trail: []string{"Late"}})

	if h.m.err != nil || h.m.step != step {
		t.Errorf("step = %v (err: %v), want the late messages ignored", h.m.step, h.m.err)
	}
}

func TestOnboardingSearchIgnoresNonKeyMessages(t *testing.T) {
	h := newHarness(t, config.Config{}, searchableClient())
	h.run(h.m.Init())
	h.slash()
	h.send(spinnerLikeMsg{})

	if h.m.search == nil || h.m.err != nil {
		t.Errorf("a stray message should leave the search alone (err: %v)", h.m.err)
	}
}

func TestOnboardingResolveBreadcrumbCommand(t *testing.T) {
	client := &fakeNotion{breadcrumb: func(notion.Parent) []string { return []string{"Home"} }}
	h := newHarness(t, config.Config{}, client)
	hit := dsHit("ds-1", "Ops Board", "db-1")

	msg, _ := h.m.resolveBreadcrumb(hit)().(breadcrumbMsg)
	if msg.id != "ds-1" || !reflect.DeepEqual(msg.trail, []string{"Home"}) {
		t.Errorf("msg = %+v, want the hit's trail", msg)
	}
	if got := client.crumbParents; !reflect.DeepEqual(got, []notion.Parent{hit.Parent}) {
		t.Errorf("walked %+v, want the hit's parent", got)
	}
}
