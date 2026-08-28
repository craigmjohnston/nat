package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeAPI stands in for the Notion client. Each field is the answer to the one
// call it names; the errors take precedence so a failure can be staged.
type fakeAPI struct {
	blocks    []notion.Block
	blocksErr error
	// blocksByID answers for one page in particular, for a command that reads
	// more than one; it takes precedence over blocks.
	blocksByID map[string][]notion.Block
	// blocksErrByID fails the read of one page in particular.
	blocksErrByID map[string]error
	// appends records every block append, in order.
	appends []appended
	// appendErr fails the append.
	appendErr error

	// deletes records the ID of every block trashed, in order.
	deletes []string
	// deleteErr fails the trashing.
	deleteErr error

	// gets records the ID of every single-page fetch, in order.
	gets []string
	// getErr fails the fetch.
	getErr error

	// queries records the data source ID and sorts of every query, in order.
	queries []query
	// pages answers a query by data source ID.
	pages map[string][]notion.Page
	// queryErr fails the query for the named data source.
	queryErr map[string]error

	// ordered records the data source ID of every view-order read, in order.
	ordered []string
	// order answers a view-order read by data source ID.
	order map[string][]string
	// orderErr fails the view-order read.
	orderErr error

	// schemaUpdates records every schema write, in order.
	schemaUpdates []schemaUpdate
	// schemaUpdateErr fails the schema write.
	schemaUpdateErr error

	// dataSourceReads records the ID of every schema fetch, in order.
	dataSourceReads []string
	// blockReads records the ID of every page-body read, in order.
	blockReads []string

	// dataSources answers a schema fetch by data source ID. One not named here
	// answers with the shape every project created before the app asked about
	// an assignee has, which is what a test saying nothing about the schema is
	// written against.
	dataSources map[string]notion.DataSource
	// dataSourceErr fails the schema fetch.
	dataSourceErr error

	// creates records every page creation, in order.
	creates []created
	// createErr fails the creation.
	createErr error
	// failCreateAfter is how many creations succeed before createErr starts
	// being returned, for a command that writes more than one page.
	failCreateAfter int
	// created is what a creation answers with, before its ID and URL are filled
	// in; the properties written are merged over it.
	createdPage notion.Page
	// createdPages answers successive creations in turn, for a command that
	// makes more than one page; it takes precedence over createdPage, and runs
	// out into it.
	createdPages []notion.Page

	// updates records every property write, in order.
	updates []update
	// updateErr fails the write.
	updateErr error
	// projects records every project creation, in order.
	projects []createdProject
	// projectStructure is what a creation answers with, when the default one
	// will not do; it is also what a staged projectErr comes back alongside.
	projectStructure *notion.ProjectStructure
	// projectErr fails the project creation.
	projectErr error
	// projectNothing is Notion answering with neither a project nor a reason.
	projectNothing bool

	// mangle is Notion being less obliging than asked: it is handed the page a
	// write would have produced, and whatever it does to it is what comes back.
	mangle func(*notion.Page)
}

// CreateProject records the whole project the command asked for and answers
// with the structure a real create comes back with.
func (f *fakeAPI) CreateProject(_ context.Context, projectsDSID, name string, assignee bool) (*notion.ProjectStructure, error) {
	f.projects = append(f.projects, createdProject{dsID: projectsDSID, name: name, assignee: assignee})
	if f.projectNothing {
		return nil, nil
	}
	if f.projectErr != nil {
		return f.projectStructure, f.projectErr
	}
	if f.projectStructure != nil {
		return f.projectStructure, nil
	}
	return &notion.ProjectStructure{
		PageID:     "new-project",
		PageURL:    "https://notion.so/new-project",
		SlicesDBID: "new-slices-db",
		SlicesDSID: "new-slices-ds",
	}, nil
}

type createdProject struct {
	dsID     string
	name     string
	assignee bool
}

type query struct {
	id    string
	sorts []notion.Sort
}

type update struct {
	id    string
	props map[string]notion.PropertyValue
}

type schemaUpdate struct {
	id    string
	props map[string]notion.PropertySchema
}

type appended struct {
	id string
	// after is the child the blocks were asked to land below, empty where they
	// went to the end.
	after    string
	children []map[string]any
}

type created struct {
	parent   notion.Parent
	props    map[string]notion.PropertyValue
	children []map[string]any
}

// CreatePage answers the way Notion does: with the whole new page, carrying the
// properties it was created with.
func (f *fakeAPI) CreatePage(_ context.Context, parent notion.Parent, props map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error) {
	f.creates = append(f.creates, created{parent: parent, props: props, children: children})
	if f.createErr != nil && len(f.creates) > f.failCreateAfter {
		return nil, f.createErr
	}
	page := f.createdPage
	if n := len(f.creates) - 1; n < len(f.createdPages) {
		page = f.createdPages[n]
	}
	base := page
	page.Properties = map[string]notion.PropertyValue{}
	for name, v := range base.Properties {
		page.Properties[name] = v
	}
	for name, v := range props {
		page.Properties[name] = readable(v)
	}
	return &page, nil
}

// readable is a written property value as Notion hands it back: text spans
// carry their content in plain_text on a read, and only in text.content on a
// write, so a fake that echoed the write unchanged would make every created
// page look nameless.
func readable(v notion.PropertyValue) notion.PropertyValue {
	v.Title, v.RichText = plainText(v.Title), plainText(v.RichText)
	return v
}

func plainText(spans []notion.RichText) []notion.RichText {
	out := make([]notion.RichText, len(spans))
	for i, s := range spans {
		if s.Text != nil {
			s.PlainText = s.Text.Content
		}
		out[i] = s
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetPage answers with whichever page of the fake plan carries the ID, so a
// command that reads one page directly sees the same workspace a query does.
func (f *fakeAPI) GetPage(_ context.Context, id string) (*notion.Page, error) {
	f.gets = append(f.gets, id)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if p, ok := f.page(id); ok {
		return &p, nil
	}
	return nil, fmt.Errorf("notion: no page %s", id)
}

// page finds a page of the fake plan by ID.
func (f *fakeAPI) page(id string) (notion.Page, bool) {
	for _, pages := range f.pages {
		for _, p := range pages {
			if p.ID == id {
				return p, true
			}
		}
	}
	return notion.Page{}, false
}

func (f *fakeAPI) AppendBlockChildren(_ context.Context, id string, children []map[string]any) ([]notion.Block, error) {
	f.appends = append(f.appends, appended{id: id, children: children})
	if f.appendErr != nil {
		return nil, f.appendErr
	}
	return nil, nil
}

// AppendBlockChildrenAfter records where the append was asked to land, which is
// the whole point of the call: a block put back into the middle of a page.
func (f *fakeAPI) AppendBlockChildrenAfter(_ context.Context, id, after string, children []map[string]any) ([]notion.Block, error) {
	f.appends = append(f.appends, appended{id: id, after: after, children: children})
	if f.appendErr != nil {
		return nil, f.appendErr
	}
	return nil, nil
}

// DeleteBlock records the block trashed, in order.
func (f *fakeAPI) DeleteBlock(_ context.Context, id string) error {
	f.deletes = append(f.deletes, id)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return nil
}

func (f *fakeAPI) GetBlockChildren(_ context.Context, id string) ([]notion.Block, error) {
	f.blockReads = append(f.blockReads, id)
	if err := f.blocksErrByID[id]; err != nil {
		return nil, err
	}
	if blocks, ok := f.blocksByID[id]; ok {
		return blocks, nil
	}
	return f.blocks, f.blocksErr
}

// UpdatePageProperties answers the way Notion does: with the whole page, the
// written properties merged over the ones it already had.
func (f *fakeAPI) UpdatePageProperties(_ context.Context, id string, props map[string]notion.PropertyValue) (*notion.Page, error) {
	f.updates = append(f.updates, update{id: id, props: props})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	page := notion.Page{ID: id, Properties: map[string]notion.PropertyValue{}}
	if existing, ok := f.page(id); ok {
		page.URL = existing.URL
		for name, v := range existing.Properties {
			page.Properties[name] = v
		}
	}
	for name, v := range props {
		page.Properties[name] = v
	}
	if f.mangle != nil {
		f.mangle(&page)
	}
	// The workspace keeps the write, so a command that reads the plan again
	// after writing to it sees what it wrote — which is what a migration on the
	// way past depends on.
	for _, pages := range f.pages {
		for i, p := range pages {
			if p.ID == id {
				pages[i] = page
			}
		}
	}
	return &page, nil
}

// GetDataSource answers with the schema of the named data source, which is how
// a command learns the shape of the project's Slices table.
func (f *fakeAPI) GetDataSource(_ context.Context, id string) (*notion.DataSource, error) {
	f.dataSourceReads = append(f.dataSourceReads, id)
	if f.dataSourceErr != nil {
		return nil, f.dataSourceErr
	}
	if ds, ok := f.dataSources[id]; ok {
		return &ds, nil
	}
	ds := assigneeSlicesDS()
	return &ds, nil
}

// UpdateDataSourceProperties records the schema write and answers with the data
// source it left behind: the properties written merged over the ones it had, the
// same way Notion hands the whole schema back.
func (f *fakeAPI) UpdateDataSourceProperties(_ context.Context, id string, props map[string]notion.PropertySchema) (*notion.DataSource, error) {
	f.schemaUpdates = append(f.schemaUpdates, schemaUpdate{id: id, props: props})
	if f.schemaUpdateErr != nil {
		return nil, f.schemaUpdateErr
	}
	ds := notion.DataSource{ID: id, Properties: map[string]notion.PropertySchema{}}
	if existing, ok := f.dataSources[id]; ok {
		for name, s := range existing.Properties {
			ds.Properties[name] = s
		}
	}
	for name, s := range props {
		ds.Properties[name] = s
	}
	if f.dataSources != nil {
		f.dataSources[id] = ds
	}
	return &ds, nil
}

// assigneeSlicesDS is the Slices schema of a project that tracks an assignee,
// its plan the milestones named — which is what a test saying nothing about the
// schema is written against.
func assigneeSlicesDS(milestones ...string) notion.DataSource {
	ds := selectMilestoneSlicesDS(milestones...)
	ds.Properties[notion.PropAssignee] = notion.PropertySchema{Type: notion.TypePeople}
	return ds
}

// soloSlicesDS is the same project without one: a single-player plan, where
// ownership is decided on status alone.
func soloSlicesDS() notion.DataSource {
	return selectMilestoneSlicesDS()
}

func (f *fakeAPI) QueryDataSource(_ context.Context, id string, _ map[string]any, sorts []notion.Sort) ([]notion.Page, error) {
	f.queries = append(f.queries, query{id: id, sorts: sorts})
	if err := f.queryErr[id]; err != nil {
		return nil, err
	}
	return f.pages[id], nil
}

func (f *fakeAPI) DataSourceOrder(_ context.Context, id string) ([]string, error) {
	f.ordered = append(f.ordered, id)
	if err := f.orderErr; err != nil {
		return nil, err
	}
	return f.order[id], nil
}

// testEnv builds an Env around a fake client and an in-memory config, and
// returns the buffer its commands write to.
func testEnv(cfg config.Config, api *fakeAPI) (Env, *bytes.Buffer) {
	var out bytes.Buffer
	return Env{
		Tokens:    config.StaticToken("ntn_o_test"),
		Load:      func() (config.Config, bool, error) { return cfg, true, nil },
		Save:      func(config.Config) error { return nil },
		NewClient: func(notion.TokenFunc) API { return api },
		Out:       &out,
	}, &out
}

// testConfig is a config pointing at one project, as onboarding writes it.
func testConfig() config.Config {
	return config.Config{
		ActiveProjectID: "project-1",
		Projects: map[string]config.ProjectConfig{
			"project-1": {
				Name:       "nat",
				SlicesDSID: "slices-ds",
				WorkingDir: "/tmp/nat",
			},
		},
	}
}

func TestIsCommandTellsACommandFromTheBoard(t *testing.T) {
	if IsCommand(nil) {
		t.Error("no arguments should open the board")
	}
	if !IsCommand([]string{"info"}) {
		t.Error("a subcommand should run headless")
	}
}

func TestRunPrintsTheHelpText(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			env, out := testEnv(testConfig(), &fakeAPI{})

			if err := Run(context.Background(), []string{arg}, env); err != nil {
				t.Fatalf("Run(%q) = %v", arg, err)
			}
			if out.String() != Usage {
				t.Errorf("output = %q, want the usage text", out.String())
			}
		})
	}
}

// The help text is the one thing worth saying when a write fails, and there is
// nowhere left to say it — so the error is handed back to the caller.
func TestRunReportsAFailedWrite(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"help"}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

// failingWriter is an output that cannot be written to.
type failingWriter struct{}

var errWrite = errors.New("no room")

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestRunRejectsAnUnknownCommand(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"bogus"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
	if !strings.Contains(err.Error(), `"bogus"`) {
		t.Errorf("err = %q, want it to name the command", err)
	}
	if !strings.Contains(err.Error(), "nat help") {
		t.Errorf("err = %q, want it to point at the help", err)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

func TestRunRejectsAnEmptyCommandLine(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), nil, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestNamedProjectReportsAFailedConfigRead(t *testing.T) {
	want := errors.New("disk gone")
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, want }

	err := Run(context.Background(), []string{"info", "--project", "project-1"}, env)

	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestNamedProjectReportsUnfinishedSetup(t *testing.T) {
	tests := []struct {
		name  string
		cfg   config.Config
		found bool
		want  string
	}{
		{name: "no config file", found: false, want: "run `nat` once to set it up"},
		{
			name:  "a config file that tracks nothing",
			cfg:   config.Config{},
			found: true,
			want:  "no project project-1 in the config file: it tracks no projects yet",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := testEnv(config.Config{}, &fakeAPI{})
			env.Load = func() (config.Config, bool, error) { return tt.cfg, tt.found, nil }

			err := Run(context.Background(), []string{"info", "--project", "project-1"}, env)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// DefaultNewClient is the one place the real client is built; nothing else in
// the package knows the concrete type.
func TestDefaultNewClientBuildsANotionClient(t *testing.T) {
	client := DefaultNewClient(func() (string, error) { return "ntn_o_test", nil })

	if _, ok := client.(*notion.Client); !ok {
		t.Errorf("client is %T, want *notion.Client", client)
	}
}
