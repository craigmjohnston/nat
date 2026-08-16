package notion

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
)

// migrateStub is a MigrationAPI recording every call, so a migration can be
// checked by the writes it made — and the order it made them in — as well as by
// what it reported.
type migrateStub struct {
	dataSource   *DataSource
	dataSourceDS map[string]*DataSource
	queries      map[string][]Page

	getErr   error
	getErrDS map[string]error
	queryErr map[string]error
	// schemaErrOn fails the n-th (1-based) schema write with schemaErr.
	schemaErrOn    int
	schemaErr      error
	pageErr        error
	deleteBlockErr error

	// calls is every write-side call in the order it was made.
	calls   []string
	queried []string
	// writes are the schema writes, in order, exactly as sent.
	writes []map[string]PropertySchema
	filed  []string
}

func (s *migrateStub) GetDataSource(_ context.Context, id string) (*DataSource, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if err := s.getErrDS[id]; err != nil {
		return nil, err
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

// UpdateDataSourceProperties echoes the way the API does: the whole schema with
// the written properties applied, IDs assigned to whatever was created — which
// is where the migration reads the new In progress option's ID back from before
// sending the list again without Claimed.
func (s *migrateStub) UpdateDataSourceProperties(_ context.Context, id string, properties map[string]PropertySchema) (*DataSource, error) {
	s.writes = append(s.writes, maps.Clone(properties))
	s.calls = append(s.calls, "schema "+id)
	if len(s.writes) == s.schemaErrOn {
		return nil, s.schemaErr
	}
	merged := maps.Clone(s.dataSource.Properties)
	for k, v := range properties {
		if v.Select != nil {
			options := make([]SelectOption, len(v.Select.Options))
			copy(options, v.Select.Options)
			for i := range options {
				if options[i].ID == "" {
					options[i].ID = "opt-" + options[i].Name
				}
			}
			v.Select = &OptionsConfig{Options: options}
		}
		merged[k] = v
	}
	s.dataSource = &DataSource{ID: id, Properties: merged, Parent: s.dataSource.Parent}
	return s.dataSource, nil
}

func (s *migrateStub) UpdatePageProperties(_ context.Context, pageID string, properties map[string]PropertyValue) (*Page, error) {
	entry := pageID
	if v, ok := properties[PropMilestone]; ok {
		entry += " milestone=" + v.SelectName()
	}
	if v, ok := properties[PropStatus]; ok {
		entry += " status=" + v.SelectName()
	}
	s.filed = append(s.filed, entry)
	s.calls = append(s.calls, "page "+pageID)
	if s.pageErr != nil {
		return nil, s.pageErr
	}
	return &Page{ID: pageID}, nil
}

func (s *migrateStub) DeleteBlock(_ context.Context, id string) error {
	s.calls = append(s.calls, "trash "+id)
	return s.deleteBlockErr
}

// dependsOnColumn is the Depends on column as a data source read returns it —
// what a project that has already been given one carries, so a test about
// anything else is not also a test of the back-fill.
func dependsOnColumn() PropertySchema {
	s := SchemaRelation("ds-slices")
	s.Type = "relation"
	return s
}

// oldShape is a Slices data source as this app first made one: milestones in a
// database of their own, and Claimed for the status an agent holds. It carries
// the dependency column, which such a project would not have — the back-fill is
// covered on its own, so the migrations tested through this one are the shape
// changes alone.
func oldShape() *DataSource {
	return &DataSource{
		ID: "ds-slices",
		Properties: map[string]PropertySchema{
			PropName:      {Type: "title"},
			PropDependsOn: dependsOnColumn(),
			PropStatus: {Type: TypeSelect, Select: &OptionsConfig{Options: []SelectOption{
				{ID: "1", Name: SliceTodo, Color: "gray"},
				{ID: "2", Name: SliceClaimed, Color: "blue"},
				{ID: "3", Name: SliceDone, Color: "green"},
			}}},
			PropMilestone: {Type: "relation", Relation: &RelationConfig{DataSourceID: "ds-milestones"}},
		},
	}
}

// oldShapeStub is a stub around a whole old-shape project: the slices, and the
// milestones database behind the relation.
func oldShapeStub() *migrateStub {
	return &migrateStub{
		dataSource: oldShape(),
		dataSourceDS: map[string]*DataSource{
			"ds-milestones": {ID: "ds-milestones", Parent: Parent{Type: ParentDatabase, DatabaseID: "db-milestones"}},
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
		PropMilestone: {Relation: &[]Relation{{ID: milestoneID}}},
	}}
}

// claimedSlice is a slice page sitting on the old Claimed status, related to
// one milestone page where it names one.
func claimedSlice(id, milestoneID string) Page {
	p := Page{ID: id, Properties: map[string]PropertyValue{
		PropStatus: {Select: &SelectOption{Name: SliceClaimed}},
	}}
	if milestoneID != "" {
		p.Properties[PropMilestone] = PropertyValue{Relation: &[]Relation{{ID: milestoneID}}}
	}
	return p
}

func TestMigrateProjectOldShape(t *testing.T) {
	stub := oldShapeStub()
	stub.queries = map[string][]Page{
		"ds-milestones": milestonePages(),
		"ds-slices": {
			claimedSlice("s-1", "m-2"),
			relatedSlice("s-2", "m-9"), // no such milestone page
			{ID: "s-3"},                // related to nothing
			relatedSlice("s-4", "M-1"),
			claimedSlice("s-5", ""), // held, but filed under nothing
		},
	}

	ds, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The first schema write moves the plan onto the Milestone column and adds
	// In progress alongside Claimed — which the API will not rename in place,
	// and which the slices being moved still sit on.
	wantFirst := map[string]PropertySchema{
		PropMilestone: SchemaSelect("M1: Groundwork", "M2: The board"),
		PropStatus: {Select: &OptionsConfig{Options: []SelectOption{
			{ID: "1", Name: SliceTodo, Color: "gray"},
			{ID: "2", Name: SliceClaimed, Color: "blue"},
			{ID: "3", Name: SliceDone, Color: "green"},
			{Name: SliceInProgress},
		}}},
	}
	// The second retires Claimed: the options exactly as the first write echoed
	// them — In progress now carrying the ID it was created with — minus the
	// one nothing sits on any more.
	wantSecond := map[string]PropertySchema{
		PropStatus: {Select: &OptionsConfig{Options: []SelectOption{
			{ID: "1", Name: SliceTodo, Color: "gray"},
			{ID: "3", Name: SliceDone, Color: "green"},
			{ID: "opt-" + SliceInProgress, Name: SliceInProgress},
		}}},
	}
	if want := []map[string]PropertySchema{wantFirst, wantSecond}; !reflect.DeepEqual(stub.writes, want) {
		t.Errorf("schema writes =\n%+v\nwant\n%+v", stub.writes, want)
	}
	// The milestones are read before the slices, and both before the schema
	// changes: converting the column is what discards the relations.
	if want := []string{"ds-milestones", "ds-slices"}; !reflect.DeepEqual(stub.queried, want) {
		t.Errorf("queried %v, want %v", stub.queried, want)
	}
	// s-4's relation is the same ID written another way; s-2 and s-3 have no
	// milestone to file them under. s-1 moves milestone and status in the one
	// write; s-5 has only its status to move.
	if want := []string{
		"s-1 milestone=M2: The board status=In progress",
		"s-4 milestone=M1: Groundwork",
		"s-5 status=In progress",
	}; !reflect.DeepEqual(stub.filed, want) {
		t.Errorf("filed %v, want %v", stub.filed, want)
	}
	// The Milestones database goes last: until everything else has succeeded it
	// is still in place to refile from.
	wantCalls := []string{
		"schema ds-slices",
		"page s-1", "page s-4", "page s-5",
		"schema ds-slices",
		"trash db-milestones",
	}
	if !reflect.DeepEqual(stub.calls, wantCalls) {
		t.Errorf("calls =\n%v\nwant\n%v", stub.calls, wantCalls)
	}
	want := Migration{
		Milestones:        []string{"M1: Groundwork", "M2: The board"},
		Slices:            2,
		StatusRenamed:     true,
		MilestonesTrashed: true,
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
		PropDependsOn: dependsOnColumn(),
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
	if stub.calls != nil || stub.queried != nil {
		t.Errorf("made calls %v after reading %v, want nothing", stub.calls, stub.queried)
	}
}

// A project made before slices could declare what they wait on has no Depends
// on column, and every dependency written against it is refused by the API
// until it gains one. Nothing else about such a project is old, so the column
// is all that is written.
func TestMigrateProjectAddsTheDependencyColumn(t *testing.T) {
	stub := &migrateStub{dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
		PropStatus:    {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{SliceTodo, SliceInProgress, SliceDone})}},
		PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
	}}}

	ds, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []map[string]PropertySchema{{PropDependsOn: SchemaRelation("ds-slices")}}
	if !reflect.DeepEqual(stub.writes, want) {
		t.Errorf("schema writes = %+v, want %+v", stub.writes, want)
	}
	if !reflect.DeepEqual(migration, Migration{DependsOnAdded: true}) {
		t.Errorf("migration = %+v, want the column added and nothing else", migration)
	}
	if _, ok := ds.Properties[PropDependsOn]; !ok {
		t.Errorf("data source = %+v, want the column it was just written", ds)
	}
	if stub.queried != nil {
		t.Errorf("queried %v, want nothing: no slice is read to add a column", stub.queried)
	}
}

// The column goes on last, after the shape changes, so a project refused or
// failed part way through them is left as those steps found it.
func TestMigrateProjectAddsTheDependencyColumnLast(t *testing.T) {
	stub := oldShapeStub()
	delete(stub.dataSource.Properties, PropDependsOn)
	stub.queries = map[string][]Page{
		"ds-milestones": milestonePages(),
		"ds-slices":     {relatedSlice("s-1", "m-1")},
	}

	_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantCalls := []string{
		"schema ds-slices", "page s-1", "schema ds-slices",
		"trash db-milestones", "schema ds-slices",
	}
	if !reflect.DeepEqual(stub.calls, wantCalls) {
		t.Errorf("calls =\n%v\nwant\n%v", stub.calls, wantCalls)
	}
	if last := stub.writes[len(stub.writes)-1]; !reflect.DeepEqual(last,
		map[string]PropertySchema{PropDependsOn: SchemaRelation("ds-slices")}) {
		t.Errorf("last schema write = %+v, want the dependency column", last)
	}
	if !migration.DependsOnAdded || !migration.MilestonesTrashed {
		t.Errorf("migration = %+v, want both the shape changes and the column", migration)
	}
}

// A project already on one page whose status still says Claimed has only that
// to migrate: the option is appended, the slices holding the old name are moved,
// and the old option is dropped — with nothing else touched.
func TestMigrateProjectStatusOnly(t *testing.T) {
	stub := &migrateStub{
		dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
			PropStatus: {Type: TypeSelect, Select: &OptionsConfig{Options: []SelectOption{
				{ID: "2", Name: SliceClaimed},
			}}},
			PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
			PropDependsOn: dependsOnColumn(),
		}},
		queries: map[string][]Page{"ds-slices": {
			claimedSlice("s-1", ""),
			{ID: "s-2"}, // not held: nothing to move
		}},
	}

	_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Migration{StatusRenamed: true}
	if !reflect.DeepEqual(migration, want) {
		t.Errorf("migration = %+v, want %+v", migration, want)
	}
	wantCalls := []string{"schema ds-slices", "page s-1", "schema ds-slices"}
	if !reflect.DeepEqual(stub.calls, wantCalls) {
		t.Errorf("calls = %v, want %v", stub.calls, wantCalls)
	}
	if want := []string{"s-1 status=In progress"}; !reflect.DeepEqual(stub.filed, want) {
		t.Errorf("filed %v, want %v", stub.filed, want)
	}
	if _, ok := stub.writes[0][PropMilestone]; ok {
		t.Errorf("first schema write = %+v, want the Status column alone", stub.writes[0])
	}
}

// A Status column offering both names is left alone: moving one option's slices
// onto the other is a decision about somebody's slices rather than a rename.
func TestMigrateProjectLeavesBothNamesAlone(t *testing.T) {
	stub := &migrateStub{dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
		PropStatus: {Type: TypeSelect, Select: &OptionsConfig{
			Options: selectOptions([]string{SliceTodo, SliceClaimed, SliceInProgress}),
		}},
		PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
		PropDependsOn: dependsOnColumn(),
	}}}

	_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !migration.Empty() || stub.calls != nil {
		t.Errorf("migration = %+v, calls %v; want neither", migration, stub.calls)
	}
}

// A Status column converted in the Notion UI cannot have its options written,
// so the one edit to make is named — and nothing is written first, so the
// project is not left half-migrated.
func TestMigrateProjectRefusesAStatusColumn(t *testing.T) {
	stub := oldShapeStub()
	// Without the dependency column too, so that nothing at all being written
	// is something to assert rather than something there was none of.
	delete(stub.dataSource.Properties, PropDependsOn)
	stub.dataSource.Properties[PropStatus] = PropertySchema{Type: TypeStatus, Status: &OptionsConfig{
		Options: selectOptions([]string{SliceTodo, SliceClaimed}),
	}}

	_, _, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err == nil {
		t.Fatal("err = nil, want a refusal")
	}
	for _, want := range []string{"status", SliceClaimed, SliceInProgress} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if stub.calls != nil || stub.queried != nil {
		t.Error("something was written before the refusal")
	}
}

// A project whose milestones data source names no database cannot have that
// database trashed, and is refused whole before the first write rather than
// half-migrated.
func TestMigrateProjectRefusesWithoutMilestonesDatabase(t *testing.T) {
	stub := oldShapeStub()
	delete(stub.dataSource.Properties, PropDependsOn)
	stub.dataSourceDS["ds-milestones"] = &DataSource{ID: "ds-milestones"}
	stub.queries = map[string][]Page{"ds-milestones": milestonePages()}

	_, _, err := MigrateProject(context.Background(), stub, "ds-slices")
	if err == nil || !strings.Contains(err.Error(), "no database to trash") {
		t.Fatalf("err = %v, want a refusal naming the missing database", err)
	}
	if stub.calls != nil {
		t.Error("something was written before the refusal")
	}
}

func TestMigrateProjectMilestoneNames(t *testing.T) {
	stub := oldShapeStub()
	stub.queries = map[string][]Page{
		"ds-milestones": {
			{ID: "m-1", Properties: map[string]PropertyValue{PropName: titleValue(" M1: Groundwork ")}},
			{ID: "m-2", Properties: map[string]PropertyValue{PropName: titleValue("")}},
			{ID: "m-3", Properties: map[string]PropertyValue{PropName: titleValue("m1: groundwork")}},
		},
		"ds-slices": {relatedSlice("s-1", "m-3"), relatedSlice("s-2", "m-2")},
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
	if want := []string{"s-1 milestone=M1: Groundwork"}; !reflect.DeepEqual(stub.filed, want) {
		t.Errorf("filed %v, want %v", stub.filed, want)
	}
}

func TestMigrateProjectErrors(t *testing.T) {
	boom := errors.New("boom")
	// planned is an old-shape stub far enough along to reach the later steps.
	planned := func() *migrateStub {
		stub := oldShapeStub()
		stub.queries = map[string][]Page{
			"ds-milestones": milestonePages(),
			"ds-slices":     {relatedSlice("s-1", "m-1")},
		}
		return stub
	}
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
				stub := oldShapeStub()
				stub.dataSource.Properties[PropMilestone] = PropertySchema{Type: "relation", Relation: &RelationConfig{}}
				return stub
			},
			"names no data source",
		},
		{
			"the milestones cannot be read",
			func() *migrateStub {
				stub := oldShapeStub()
				stub.queryErr = map[string]error{"ds-milestones": boom}
				return stub
			},
			"load milestones",
		},
		{
			"the milestones database cannot be read",
			func() *migrateStub {
				stub := planned()
				stub.getErrDS = map[string]error{"ds-milestones": boom}
				return stub
			},
			"load the milestones database",
		},
		{
			"the slices cannot be read",
			func() *migrateStub {
				stub := planned()
				stub.queryErr = map[string]error{"ds-slices": boom}
				return stub
			},
			"load slices",
		},
		{
			"the schema cannot be written",
			func() *migrateStub {
				stub := planned()
				stub.schemaErrOn, stub.schemaErr = 1, boom
				return stub
			},
			"migrate the slices schema",
		},
		{
			"a slice cannot be refiled",
			func() *migrateStub {
				stub := planned()
				stub.pageErr = boom
				return stub
			},
			`file slice s-1 under "M1: Groundwork"`,
		},
		{
			"a held slice cannot be moved",
			func() *migrateStub {
				return &migrateStub{
					dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
						PropStatus:    {Type: TypeSelect, Select: &OptionsConfig{Options: []SelectOption{{ID: "2", Name: SliceClaimed}}}},
						PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
						PropDependsOn: dependsOnColumn(),
					}},
					queries: map[string][]Page{"ds-slices": {claimedSlice("s-1", "")}},
					pageErr: boom,
				}
			},
			`move slice s-1 to "In progress"`,
		},
		{
			"the old option cannot be retired",
			func() *migrateStub {
				stub := planned()
				stub.schemaErrOn, stub.schemaErr = 2, boom
				return stub
			},
			`retire the "Claimed" option`,
		},
		{
			"the dependency column cannot be added",
			func() *migrateStub {
				stub := &migrateStub{dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
					PropStatus:    {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{SliceTodo})}},
					PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
				}}}
				stub.schemaErrOn, stub.schemaErr = 1, boom
				return stub
			},
			`add the "Depends on" column`,
		},
		{
			"the dependency column cannot be added after the shape changes",
			func() *migrateStub {
				stub := planned()
				delete(stub.dataSource.Properties, PropDependsOn)
				stub.schemaErrOn, stub.schemaErr = 3, boom
				return stub
			},
			`add the "Depends on" column`,
		},
		{
			"the Milestones database cannot be trashed",
			func() *migrateStub {
				stub := planned()
				stub.deleteBlockErr = boom
				return stub
			},
			"trash the Milestones database",
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
			Migration{Milestones: []string{"M1"}, Slices: 1, StatusRenamed: true, MilestonesTrashed: true, DependsOnAdded: true},
			`Migrated this project: 1 milestone moved onto the slices, 1 slice refiled, ` +
				`"Claimed" renamed to "In progress", the old Milestones database moved to Notion's trash, ` +
				`a "Depends on" column added.`,
		},
		{
			"several",
			Migration{Milestones: []string{"M1", "M2"}, Slices: 7},
			"Migrated this project: 2 milestones moved onto the slices, 7 slices refiled.",
		},
		{
			"the column alone",
			Migration{DependsOnAdded: true},
			`Migrated this project: a "Depends on" column added.`,
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
