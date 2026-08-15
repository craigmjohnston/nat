package notion

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// migrateStub is a MigrationAPI recording every call, so a migration can be
// checked by the writes it made as well as by what it reported.
type migrateStub struct {
	dataSource   *DataSource
	dataSourceDS map[string]*DataSource
	queries      map[string][]Page

	getErr    error
	queryErr  map[string]error
	updateErr error
	pageErr   error

	queried  []string
	schema   map[string]PropertySchema
	schemaDS string
	filed    []string
}

func (s *migrateStub) GetDataSource(_ context.Context, id string) (*DataSource, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if ds, ok := s.dataSourceDS[id]; ok {
		return ds, nil
	}
	return s.dataSource, nil
}

func (s *migrateStub) QueryDataSource(_ context.Context, id string, _ map[string]any, _ []Sort) ([]Page, error) {
	s.queried = append(s.queried, id)
	if err := s.queryErr[id]; err != nil {
		return nil, err
	}
	return s.queries[id], nil
}

func (s *migrateStub) UpdateDataSourceProperties(_ context.Context, id string, properties map[string]PropertySchema) (*DataSource, error) {
	s.schemaDS, s.schema = id, properties
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &DataSource{ID: id, Properties: properties}, nil
}

func (s *migrateStub) UpdatePageProperties(_ context.Context, pageID string, properties map[string]PropertyValue) (*Page, error) {
	s.filed = append(s.filed, pageID+"="+properties[PropMilestone].SelectName())
	if s.pageErr != nil {
		return nil, s.pageErr
	}
	return &Page{ID: pageID}, nil
}

// oldShape is a Slices data source as this app first made one: milestones in a
// database of their own, and Claimed for the status an agent holds.
func oldShape() *DataSource {
	return &DataSource{
		ID: "ds-slices",
		Properties: map[string]PropertySchema{
			PropName: {Type: "title"},
			PropStatus: {Type: TypeSelect, Select: &OptionsConfig{Options: []SelectOption{
				{ID: "1", Name: SliceTodo, Color: "gray"},
				{ID: "2", Name: SliceClaimed, Color: "blue"},
				{ID: "3", Name: SliceDone, Color: "green"},
			}}},
			PropMilestone: {Type: "relation", Relation: &RelationConfig{DataSourceID: "ds-milestones"}},
		},
	}
}

// titleValue is a title property as a query returns it — the plain text Notion
// sends alongside the spans, which is what a name is read from.
func titleValue(name string) PropertyValue {
	return PropertyValue{Type: "title", Title: []RichText{{PlainText: name}}}
}

// milestonePages is the plan those milestones make, in the order the query
// returns them.
func milestonePages() []Page {
	return []Page{
		{ID: "m-1", Properties: map[string]PropertyValue{
			PropName: titleValue("M1: Groundwork"),
		}},
		{ID: "m-2", Properties: map[string]PropertyValue{
			PropName: titleValue("M2: The board"),
		}},
	}
}

// relatedSlice is a slice page related to one milestone page.
func relatedSlice(id, milestoneID string) Page {
	return Page{ID: id, Properties: map[string]PropertyValue{
		PropMilestone: {Relation: []Relation{{ID: milestoneID}}},
	}}
}

func TestMigrateProjectOldShape(t *testing.T) {
	stub := &migrateStub{
		dataSource: oldShape(),
		queries: map[string][]Page{
			"ds-milestones": milestonePages(),
			"ds-slices": {
				relatedSlice("s-1", "m-2"),
				relatedSlice("s-2", "m-9"), // no such milestone page
				{ID: "s-3"},                // related to nothing
				relatedSlice("s-4", "M-1"),
			},
		},
	}

	ds, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSchema := map[string]PropertySchema{
		PropMilestone: SchemaSelect("M1: Groundwork", "M2: The board"),
		PropStatus: {Select: &OptionsConfig{Options: []SelectOption{
			{ID: "1", Name: SliceTodo, Color: "gray"},
			{ID: "2", Name: SliceInProgress, Color: "blue"},
			{ID: "3", Name: SliceDone, Color: "green"},
		}}},
	}
	if stub.schemaDS != "ds-slices" || !reflect.DeepEqual(stub.schema, wantSchema) {
		t.Errorf("schema write to %q =\n%+v\nwant\n%+v", stub.schemaDS, stub.schema, wantSchema)
	}
	// The milestones are read before the slices, and both before the schema
	// changes: converting the column is what discards the relations.
	if want := []string{"ds-milestones", "ds-slices"}; !reflect.DeepEqual(stub.queried, want) {
		t.Errorf("queried %v, want %v", stub.queried, want)
	}
	// s-4's relation is the same ID written another way; s-2 and s-3 have no
	// milestone to file them under.
	if want := []string{"s-1=M2: The board", "s-4=M1: Groundwork"}; !reflect.DeepEqual(stub.filed, want) {
		t.Errorf("filed %v, want %v", stub.filed, want)
	}
	want := Migration{
		Milestones:    []string{"M1: Groundwork", "M2: The board"},
		Slices:        2,
		StatusRenamed: true,
	}
	if !reflect.DeepEqual(migration, want) {
		t.Errorf("migration = %+v, want %+v", migration, want)
	}
	if ds == nil || ds.Properties[PropMilestone].Select == nil {
		t.Errorf("data source = %+v, want the migrated schema", ds)
	}
}

// The second load of a project finds nothing to do, which is what makes running
// this on every load safe.
func TestMigrateProjectIsIdempotent(t *testing.T) {
	migrated := &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
		PropStatus:    {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{SliceTodo, SliceInProgress, SliceDone})}},
		PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{"M1: Groundwork"})}},
	}}
	stub := &migrateStub{dataSource: migrated}

	ds, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !migration.Empty() {
		t.Errorf("migration = %+v, want nothing to do", migration)
	}
	if ds != migrated {
		t.Errorf("data source = %+v, want the one it read", ds)
	}
	if stub.schema != nil || stub.filed != nil || stub.queried != nil {
		t.Errorf("wrote %v / %v after reading %v, want nothing", stub.schema, stub.filed, stub.queried)
	}
}

func TestMigrateProjectStatusOnly(t *testing.T) {
	stub := &migrateStub{dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
		PropStatus: {Type: TypeSelect, Select: &OptionsConfig{Options: []SelectOption{
			{ID: "2", Name: SliceClaimed},
		}}},
		PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
	}}}

	_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Migration{StatusRenamed: true}
	if !reflect.DeepEqual(migration, want) {
		t.Errorf("migration = %+v, want %+v", migration, want)
	}
	if _, ok := stub.schema[PropMilestone]; ok {
		t.Errorf("schema write = %+v, want the Status column alone", stub.schema)
	}
	if stub.queried != nil {
		t.Errorf("queried %v, want nothing: there is no plan to move", stub.queried)
	}
}

// A Status column offering both names is left alone: renaming one onto the
// other would ask Notion for two options of a name.
func TestMigrateProjectLeavesBothNamesAlone(t *testing.T) {
	stub := &migrateStub{dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
		PropStatus: {Type: TypeSelect, Select: &OptionsConfig{
			Options: selectOptions([]string{SliceTodo, SliceClaimed, SliceInProgress}),
		}},
		PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
	}}}

	_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !migration.Empty() || stub.schema != nil {
		t.Errorf("migration = %+v, schema %+v; want neither", migration, stub.schema)
	}
}

// A Status column converted in the Notion UI cannot have its options written,
// so the one edit to make is named — and nothing is written first, so the
// project is not left half-migrated.
func TestMigrateProjectRefusesAStatusColumn(t *testing.T) {
	ds := oldShape()
	ds.Properties[PropStatus] = PropertySchema{Type: TypeStatus, Status: &OptionsConfig{
		Options: selectOptions([]string{SliceTodo, SliceClaimed}),
	}}
	stub := &migrateStub{dataSource: ds}

	_, _, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err == nil {
		t.Fatal("err = nil, want a refusal")
	}
	for _, want := range []string{"status", SliceClaimed, SliceInProgress} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if stub.schema != nil || stub.filed != nil || stub.queried != nil {
		t.Error("something was written before the refusal")
	}
}

func TestMigrateProjectMilestoneNames(t *testing.T) {
	stub := &migrateStub{
		dataSource: oldShape(),
		queries: map[string][]Page{
			"ds-milestones": {
				{ID: "m-1", Properties: map[string]PropertyValue{PropName: titleValue(" M1: Groundwork ")}},
				{ID: "m-2", Properties: map[string]PropertyValue{PropName: titleValue("")}},
				{ID: "m-3", Properties: map[string]PropertyValue{PropName: titleValue("m1: groundwork")}},
			},
			"ds-slices": {relatedSlice("s-1", "m-3"), relatedSlice("s-2", "m-2")},
		},
	}

	_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// An unnamed milestone cannot be an option, and two of a name cannot be told
	// apart: the second files its slices under the first.
	if want := []string{"M1: Groundwork"}; !reflect.DeepEqual(migration.Milestones, want) {
		t.Errorf("milestones = %v, want %v", migration.Milestones, want)
	}
	if want := []string{"s-1=M1: Groundwork"}; !reflect.DeepEqual(stub.filed, want) {
		t.Errorf("filed %v, want %v", stub.filed, want)
	}
}

func TestMigrateProjectErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		stub func() *migrateStub
		want string
	}{
		{
			"the schema cannot be read",
			func() *migrateStub { return &migrateStub{getErr: boom} },
			"load the slices schema",
		},
		{
			"the relation names no data source",
			func() *migrateStub {
				ds := oldShape()
				ds.Properties[PropMilestone] = PropertySchema{Type: "relation", Relation: &RelationConfig{}}
				return &migrateStub{dataSource: ds}
			},
			"names no data source",
		},
		{
			"the milestones cannot be read",
			func() *migrateStub {
				return &migrateStub{dataSource: oldShape(), queryErr: map[string]error{"ds-milestones": boom}}
			},
			"load milestones",
		},
		{
			"the slices cannot be read",
			func() *migrateStub {
				return &migrateStub{
					dataSource: oldShape(),
					queries:    map[string][]Page{"ds-milestones": milestonePages()},
					queryErr:   map[string]error{"ds-slices": boom},
				}
			},
			"load slices",
		},
		{
			"the schema cannot be written",
			func() *migrateStub {
				return &migrateStub{
					dataSource: oldShape(),
					queries:    map[string][]Page{"ds-milestones": milestonePages()},
					updateErr:  boom,
				}
			},
			"migrate the slices schema",
		},
		{
			"a slice cannot be refiled",
			func() *migrateStub {
				return &migrateStub{
					dataSource: oldShape(),
					queries: map[string][]Page{
						"ds-milestones": milestonePages(),
						"ds-slices":     {relatedSlice("s-1", "m-1")},
					},
					pageErr: boom,
				}
			},
			`file slice s-1 under "M1: Groundwork"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, migration, err := MigrateProject(context.Background(), tt.stub(), "ds-slices")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
			if !migration.Empty() {
				t.Errorf("migration = %+v, want nothing claimed on a failure", migration)
			}
		})
	}
}

func TestMigrationSummary(t *testing.T) {
	tests := []struct {
		name      string
		migration Migration
		want      string
	}{
		{"nothing", Migration{}, "nothing to migrate"},
		{
			"one of each",
			Migration{Milestones: []string{"M1"}, Slices: 1, StatusRenamed: true},
			`Migrated this project: 1 milestone moved onto the slices, 1 slice refiled, "Claimed" renamed to "In progress".`,
		},
		{
			"several",
			Migration{Milestones: []string{"M1", "M2"}, Slices: 7},
			"Migrated this project: 2 milestones moved onto the slices, 7 slices refiled.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.migration.Summary(); got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}
