package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

const slicesDS = "ds-slices"

// fakeAPI stands in for Notion: each call is a hook, so a test supplies only
// the behaviour it cares about, and every call is recorded so the requests a
// plan operation actually makes can be asserted on.
type fakeAPI struct {
	dataSource   func(id string) (*notion.DataSource, error)
	query        func(id string) ([]notion.Page, error)
	order        func(id string) ([]string, error)
	updateSchema func(id string, properties map[string]notion.PropertySchema) (*notion.DataSource, error)
	page         func(id string) (*notion.Page, error)
	createPage   func(parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	updatePage   func(id string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	blocks       func(id string) ([]notion.Block, error)
	appendBlocks func(id string, children []map[string]any) ([]notion.Block, error)
	deleteBlock  func(id string) error
	trash        func(id string) error

	calls    []string
	updates  []map[string]notion.PropertyValue
	appended [][]map[string]any
	schemas  []map[string]notion.PropertySchema
	created  []map[string]any
	deleted  []string
}

var _ API = (*fakeAPI)(nil)

// settledSchema is a Slices data source in the one shape everything reads:
// nothing for the migration to change, so a read of it is one request.
func settledSchema(assignee bool, milestones ...string) *notion.DataSource {
	schema := notion.SlicesSchema(assignee)
	schema[notion.PropMilestone] = notion.SchemaSelect(milestones...)
	schema[notion.PropDependsOn] = notion.SchemaRelation(slicesDS)
	// Notion echoes a property's type back on every read; the schema builders
	// write only the configuration, since a write says the type by shape.
	for name, p := range schema {
		p.Name, p.Type = name, schemaType(p)
		schema[name] = p
	}
	return &notion.DataSource{ID: slicesDS, Properties: schema}
}

// schemaType is the type Notion would report for a property definition.
func schemaType(p notion.PropertySchema) string {
	switch {
	case p.Title != nil:
		return "title"
	case p.RichText != nil:
		return notion.TypeRichText
	case p.Select != nil:
		return notion.TypeSelect
	case p.People != nil:
		return notion.TypePeople
	case p.URL != nil:
		return "url"
	default:
		return "relation"
	}
}

func (f *fakeAPI) GetDataSource(_ context.Context, id string) (*notion.DataSource, error) {
	f.calls = append(f.calls, "GetDataSource")
	if f.dataSource == nil {
		return settledSchema(true), nil
	}
	return f.dataSource(id)
}

func (f *fakeAPI) QueryDataSource(_ context.Context, id string, _ map[string]any, _ []notion.Sort) ([]notion.Page, error) {
	f.calls = append(f.calls, "QueryDataSource")
	if f.query == nil {
		return nil, nil
	}
	return f.query(id)
}

func (f *fakeAPI) UpdateDataSourceProperties(_ context.Context, id string, properties map[string]notion.PropertySchema) (*notion.DataSource, error) {
	f.calls = append(f.calls, "UpdateDataSourceProperties")
	f.schemas = append(f.schemas, properties)
	if f.updateSchema == nil {
		return settledSchema(true), nil
	}
	return f.updateSchema(id, properties)
}

func (f *fakeAPI) DataSourceOrder(_ context.Context, id string) ([]string, error) {
	f.calls = append(f.calls, "DataSourceOrder")
	if f.order == nil {
		return nil, nil
	}
	return f.order(id)
}

func (f *fakeAPI) GetPage(_ context.Context, id string) (*notion.Page, error) {
	f.calls = append(f.calls, "GetPage")
	if f.page == nil {
		return &notion.Page{ID: id}, nil
	}
	return f.page(id)
}

func (f *fakeAPI) CreatePage(_ context.Context, parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error) {
	f.calls = append(f.calls, "CreatePage")
	f.created = append(f.created, map[string]any{"parent": parent, "properties": properties, "children": children})
	if f.createPage == nil {
		return &notion.Page{ID: "new"}, nil
	}
	return f.createPage(parent, properties, children)
}

func (f *fakeAPI) UpdatePageProperties(_ context.Context, id string, properties map[string]notion.PropertyValue) (*notion.Page, error) {
	f.calls = append(f.calls, "UpdatePageProperties")
	f.updates = append(f.updates, properties)
	if f.updatePage == nil {
		return &notion.Page{ID: id}, nil
	}
	return f.updatePage(id, properties)
}

func (f *fakeAPI) GetBlockChildren(_ context.Context, id string) ([]notion.Block, error) {
	f.calls = append(f.calls, "GetBlockChildren")
	if f.blocks == nil {
		return nil, nil
	}
	return f.blocks(id)
}

func (f *fakeAPI) AppendBlockChildren(_ context.Context, id string, children []map[string]any) ([]notion.Block, error) {
	f.calls = append(f.calls, "AppendBlockChildren")
	f.appended = append(f.appended, children)
	if f.appendBlocks == nil {
		return nil, nil
	}
	return f.appendBlocks(id, children)
}

func (f *fakeAPI) DeleteBlock(_ context.Context, id string) error {
	f.calls = append(f.calls, "DeleteBlock")
	f.deleted = append(f.deleted, id)
	if f.deleteBlock == nil {
		return nil
	}
	return f.deleteBlock(id)
}

func (f *fakeAPI) TrashPage(_ context.Context, id string) error {
	f.calls = append(f.calls, "TrashPage")
	if f.trash == nil {
		return nil
	}
	return f.trash(id)
}

// project is the project every test here works on.
func project() Project { return Project{ID: "p1", Name: "nat", SlicesID: slicesDS} }

// slicePage is a page of the Slices data source, as Notion answers one.
func slicePage(id, name, status string, assignees ...notion.User) *notion.Page {
	props := map[string]notion.PropertyValue{
		notion.PropName:   {Type: "title", Title: []notion.RichText{{PlainText: name}}},
		notion.PropStatus: {Type: notion.TypeSelect, Select: &notion.SelectOption{Name: status}},
	}
	if assignees != nil {
		props[notion.PropAssignee] = notion.PropertyValue{Type: notion.TypePeople, People: &assignees}
	}
	return &notion.Page{ID: id, Properties: props}
}

var errBoom = errors.New("notion is down")

func TestNewClientMakesAClientThatReadsItsTokenPerRequest(t *testing.T) {
	if NewClient(func() (string, error) { return "secret", nil }) == nil {
		t.Fatal("NewClient returned no client")
	}
}

func TestShapeReadsTheProjectsColumnsAndPlan(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		return settledSchema(true, "M1", "M2"), nil
	}}
	sh, err := Over(api).Shape(context.Background(), project())
	if err != nil {
		t.Fatalf("Shape() error = %v", err)
	}
	if !sh.HasAssignee || !sh.HasBranch {
		t.Errorf("shape = %+v, want both columns read", sh)
	}
	want := []domain.Milestone{
		{ID: "M1", Name: "M1", Order: 0, SelectType: notion.TypeSelect},
		{ID: "M2", Name: "M2", Order: 1, SelectType: notion.TypeSelect},
	}
	if !reflect.DeepEqual(sh.Milestones, want) {
		t.Errorf("milestones = %+v, want %+v", sh.Milestones, want)
	}
}

func TestShapeWithoutAnAssigneeColumn(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		return settledSchema(false), nil
	}}
	sh, err := Over(api).Shape(context.Background(), project())
	if err != nil {
		t.Fatalf("Shape() error = %v", err)
	}
	if sh.HasAssignee {
		t.Error("a project with no Assignee column read as having one")
	}
}

func TestShapeCarriesTheMigrationsFailureUp(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	if _, err := Over(api).Shape(context.Background(), project()); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the read's failure", err)
	}
}

// A project still in the old shape is migrated on the way past, and the shape
// that comes back is the migrated one.
func TestShapeMigratesAProjectOnTheWayPast(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		ds := settledSchema(true)
		delete(ds.Properties, notion.PropBranch)
		return ds, nil
	}}
	sh, err := Over(api).Shape(context.Background(), project())
	if err != nil {
		t.Fatalf("Shape() error = %v", err)
	}
	if !sh.HasBranch {
		t.Error("the back-filled Branch column was not read back")
	}
	if len(api.schemas) != 1 {
		t.Errorf("schema writes = %d, want the one back-fill", len(api.schemas))
	}
}

func TestPlanReadsTheWholePlanInTheBoardsOrder(t *testing.T) {
	api := &fakeAPI{
		dataSource: func(string) (*notion.DataSource, error) { return settledSchema(true, "M1"), nil },
		query: func(string) ([]notion.Page, error) {
			return []notion.Page{*slicePage("s1", "First", notion.SliceTodo), *slicePage("s2", "Second", notion.SliceTodo)}, nil
		},
		order: func(string) ([]string, error) { return []string{"s2", "s1"}, nil },
	}
	plan, err := Over(api).Plan(context.Background(), project())
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Migrated != "" {
		t.Errorf("migrated = %q, want nothing changed", plan.Migrated)
	}
	if plan.Project.ID != "p1" || plan.Project.Name != "nat" {
		t.Errorf("project = %+v, want the one asked for", plan.Project)
	}
	if got := []string{plan.Project.Slices[0].ID, plan.Project.Slices[1].ID}; !reflect.DeepEqual(got, []string{"s2", "s1"}) {
		t.Errorf("slices = %v, want the view's own order", got)
	}
	if len(plan.Shape.Milestones) != 1 {
		t.Errorf("milestones = %+v, want the one the schema offers", plan.Shape.Milestones)
	}
}

func TestPlanSaysWhatLoadingItChanged(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		ds := settledSchema(true)
		delete(ds.Properties, notion.PropBranch)
		return ds, nil
	}}
	plan, err := Over(api).Plan(context.Background(), project())
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if !strings.Contains(plan.Migrated, notion.PropBranch) {
		t.Errorf("migrated = %q, want the added column named", plan.Migrated)
	}
}

func TestPlanFailures(t *testing.T) {
	tests := []struct {
		name string
		api  *fakeAPI
	}{
		{"the schema", &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}},
		{"the slices", &fakeAPI{query: func(string) ([]notion.Page, error) { return nil, errBoom }}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Over(tt.api).Plan(context.Background(), project()); !errors.Is(err, errBoom) {
				t.Errorf("err = %v, want the failure", err)
			}
		})
	}
}

// An order nobody can read is no order at all rather than a plan nobody can
// read: the slices come back as the query gave them.
func TestPlanSurvivesAnUnreadableOrder(t *testing.T) {
	api := &fakeAPI{
		query: func(string) ([]notion.Page, error) {
			return []notion.Page{*slicePage("s1", "First", notion.SliceTodo)}, nil
		},
		order: func(string) ([]string, error) { return nil, errBoom },
	}
	plan, err := Over(api).Plan(context.Background(), project())
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Project.Slices) != 1 {
		t.Errorf("slices = %+v, want the plan read anyway", plan.Project.Slices)
	}
}

func TestSliceReadsTheSliceAndItsWriteShape(t *testing.T) {
	api := &fakeAPI{page: func(id string) (*notion.Page, error) {
		return slicePage(id, "Info view", notion.SliceInProgress, notion.User{ID: "u1", Name: "Craig"}), nil
	}}
	s, sh, err := Over(api).Slice(context.Background(), "s5")
	if err != nil {
		t.Fatalf("Slice() error = %v", err)
	}
	if s.Name != "Info view" || s.Status != domain.SliceClaimed {
		t.Errorf("slice = %+v, want the page read", s)
	}
	if !sh.HasAssignee {
		t.Error("a page carrying an Assignee read as recording none")
	}
}

func TestSliceCarriesTheReadsFailureUp(t *testing.T) {
	api := &fakeAPI{page: func(string) (*notion.Page, error) { return nil, errBoom }}
	if _, _, err := Over(api).Slice(context.Background(), "s5"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the read's failure", err)
	}
}

// A slice's own page says what type its status is written in; which columns
// the project has stays the schema's answer, since a column holding nothing
// may simply not appear on a page.
func TestShapeOnTakesTheStatusTypeFromThePage(t *testing.T) {
	api := &fakeAPI{page: func(id string) (*notion.Page, error) {
		p := slicePage(id, "Converted", notion.SliceTodo)
		p.Properties[notion.PropStatus] = notion.PropertyValue{
			Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceTodo}}
		return p, nil
	}}
	st := Over(api)
	schema, err := st.Shape(context.Background(), project())
	if err != nil {
		t.Fatalf("Shape() error = %v", err)
	}
	_, pageShape, err := st.Slice(context.Background(), "s5")
	if err != nil {
		t.Fatalf("Slice() error = %v", err)
	}
	write := schema.On(pageShape)
	if !write.HasAssignee || !write.HasBranch || len(write.Milestones) != len(schema.Milestones) {
		t.Errorf("write shape = %+v, want the schema's columns kept", write)
	}
	if err := st.MarkDone(context.Background(), "s5", write); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}
	if api.updates[0][notion.PropStatus].Status == nil {
		t.Errorf("status written as %+v, want the page's own type", api.updates[0][notion.PropStatus])
	}
}

func TestHolds(t *testing.T) {
	held := domain.Slice{Status: domain.SliceClaimed, AssigneeIDs: []string{"u1"}}
	tests := []struct {
		name  string
		slice domain.Slice
		shape Shape
		want  bool
	}{
		{"in progress and held by the user", held, Shape{HasAssignee: true}, true},
		{"in progress and held by somebody else",
			domain.Slice{Status: domain.SliceClaimed, AssigneeIDs: []string{"u2"}}, Shape{HasAssignee: true}, false},
		{"in progress and held by nobody",
			domain.Slice{Status: domain.SliceClaimed}, Shape{HasAssignee: true}, false},
		{"a project recording no ownership decides on status alone",
			domain.Slice{Status: domain.SliceClaimed}, Shape{}, true},
		{"not in progress at all", domain.Slice{Status: domain.SliceTodo}, Shape{HasAssignee: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Holds(tt.slice, tt.shape, "u1"); got != tt.want {
				t.Errorf("Holds() = %v, want %v", got, tt.want)
			}
		})
	}
}
