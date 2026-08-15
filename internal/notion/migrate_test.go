package notion

import (
	"context"
	"errors"
	"fmt"
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
	// views answers ListViews; view answers GetView for any ID.
	views []View
	view  *View

	getErr   error
	getErrDS map[string]error
	queryErr map[string]error
	// schemaErrOn fails the n-th (1-based) schema write with schemaErr.
	schemaErrOn    int
	schemaErr      error
	pageErr        error
	inlineErr      error
	listViewsErr   error
	getViewErr     error
	createViewErr  error
	deleteViewErr  error
	deleteBlockErr error
	// noPropertyIDs echoes schema writes without assigning property IDs, the
	// way a misbehaving API would.
	noPropertyIDs bool

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
// is where the migration reads the Milestone property ID and the new In
// progress option's ID back from.
func (s *migrateStub) UpdateDataSourceProperties(_ context.Context, id string, properties map[string]PropertySchema) (*DataSource, error) {
	sent := maps.Clone(properties)
	s.writes = append(s.writes, sent)
	s.calls = append(s.calls, "schema "+id)
	if len(s.writes) == s.schemaErrOn {
		return nil, s.schemaErr
	}
	merged := maps.Clone(s.dataSource.Properties)
	for k, v := range properties {
		if !s.noPropertyIDs {
			if v.ID == "" {
				v.ID = "prop-" + k
			}
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

func (s *migrateStub) SetDatabaseInline(_ context.Context, id string, inline bool) error {
	s.calls = append(s.calls, fmt.Sprintf("inline %s %v", id, inline))
	return s.inlineErr
}

func (s *migrateStub) ListViews(_ context.Context, dataSourceID string) ([]View, error) {
	s.calls = append(s.calls, "list-views "+dataSourceID)
	if s.listViewsErr != nil {
		return nil, s.listViewsErr
	}
	return s.views, nil
}

func (s *migrateStub) GetView(_ context.Context, id string) (*View, error) {
	s.calls = append(s.calls, "get-view "+id)
	if s.getViewErr != nil {
		return nil, s.getViewErr
	}
	if s.view != nil {
		return s.view, nil
	}
	return &View{ID: id, Type: ViewTypeTable}, nil
}

func (s *migrateStub) CreateBoardView(_ context.Context, databaseID, dataSourceID, name, groupPropertyID string) (*View, error) {
	s.calls = append(s.calls, fmt.Sprintf("create-board %s %s %s %s", databaseID, dataSourceID, name, groupPropertyID))
	if s.createViewErr != nil {
		return nil, s.createViewErr
	}
	return &View{ID: "v-board", Name: name, Type: ViewTypeBoard}, nil
}

func (s *migrateStub) DeleteView(_ context.Context, id string) error {
	s.calls = append(s.calls, "delete-view "+id)
	return s.deleteViewErr
}

func (s *migrateStub) DeleteBlock(_ context.Context, id string) error {
	s.calls = append(s.calls, "trash "+id)
	return s.deleteBlockErr
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
		Parent: Parent{Type: ParentDatabase, DatabaseID: "db-slices"},
	}
}

// oldShapeStub is a stub around a whole old-shape project: the slices, the
// milestones database behind the relation, and the default table view.
func oldShapeStub() *migrateStub {
	return &migrateStub{
		dataSource: oldShape(),
		dataSourceDS: map[string]*DataSource{
			"ds-milestones": {ID: "ds-milestones", Parent: Parent{Type: ParentDatabase, DatabaseID: "db-milestones"}},
		},
		views: []View{{ID: "v-table"}},
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

// claimedSlice is a slice page sitting on the old Claimed status, related to
// one milestone page where it names one.
func claimedSlice(id, milestoneID string) Page {
	p := Page{ID: id, Properties: map[string]PropertyValue{
		PropStatus: {Select: &SelectOption{Name: SliceClaimed}},
	}}
	if milestoneID != "" {
		p.Properties[PropMilestone] = PropertyValue{Relation: []Relation{{ID: milestoneID}}}
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
	// The page shape follows the data, and the Milestones database goes last:
	// until everything else has succeeded it is still in place to refile from.
	wantCalls := []string{
		"schema ds-slices",
		"page s-1", "page s-4", "page s-5",
		"schema ds-slices",
		"inline db-slices true",
		"list-views ds-slices",
		"create-board db-slices ds-slices Board prop-" + PropMilestone,
		"get-view v-table",
		"delete-view v-table",
		"trash db-milestones",
	}
	if !reflect.DeepEqual(stub.calls, wantCalls) {
		t.Errorf("calls =\n%v\nwant\n%v", stub.calls, wantCalls)
	}
	want := Migration{
		Milestones:        []string{"M1: Groundwork", "M2: The board"},
		Slices:            2,
		StatusRenamed:     true,
		Board:             true,
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

// A project already on one page whose status still says Claimed has only that
// to migrate: the option is appended, the slices holding the old name are moved,
// and the old option is dropped — with the page's shape left alone.
func TestMigrateProjectStatusOnly(t *testing.T) {
	stub := &migrateStub{
		dataSource: &DataSource{ID: "ds-slices", Properties: map[string]PropertySchema{
			PropStatus: {Type: TypeSelect, Select: &OptionsConfig{Options: []SelectOption{
				{ID: "2", Name: SliceClaimed},
			}}},
			PropMilestone: {Type: TypeSelect, Select: &OptionsConfig{}},
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

// A project whose data sources name no databases cannot have its page shaped or
// its Milestones database trashed, and is refused whole before the first write
// rather than half-migrated.
func TestMigrateProjectRefusesWithoutDatabases(t *testing.T) {
	t.Run("no slices database", func(t *testing.T) {
		stub := oldShapeStub()
		stub.dataSource.Parent = Parent{}

		_, _, err := MigrateProject(context.Background(), stub, "ds-slices")
		if err == nil || !strings.Contains(err.Error(), "no database to put on the page") {
			t.Fatalf("err = %v, want a refusal naming the missing database", err)
		}
		if stub.calls != nil || stub.queried != nil {
			t.Error("something was written before the refusal")
		}
	})
	t.Run("no milestones database", func(t *testing.T) {
		stub := oldShapeStub()
		stub.dataSourceDS["ds-milestones"] = &DataSource{ID: "ds-milestones"}
		stub.queries = map[string][]Page{"ds-milestones": milestonePages()}

		_, _, err := MigrateProject(context.Background(), stub, "ds-slices")
		if err == nil || !strings.Contains(err.Error(), "no database to trash") {
			t.Fatalf("err = %v, want a refusal naming the missing database", err)
		}
		if stub.calls != nil {
			t.Error("something was written before the refusal")
		}
	})
}

// Only the lone default table this app created with the database is replaced by
// the board; views someone arranged themselves are left alongside it.
func TestMigrateProjectKeepsCuratedViews(t *testing.T) {
	tests := []struct {
		name  string
		views []View
		view  *View
	}{
		{"several views", []View{{ID: "v-1"}, {ID: "v-2"}}, nil},
		{"one view, not a table", []View{{ID: "v-1"}}, &View{ID: "v-1", Type: ViewTypeBoard}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := oldShapeStub()
			stub.views, stub.view = tt.views, tt.view
			stub.queries = map[string][]Page{"ds-milestones": milestonePages()}

			_, migration, err := MigrateProject(context.Background(), stub, "ds-slices")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !migration.Board {
				t.Errorf("migration = %+v, want the board reported", migration)
			}
			for _, call := range stub.calls {
				if strings.HasPrefix(call, "delete-view") {
					t.Errorf("calls %v deleted a view, want them kept", stub.calls)
				}
			}
		})
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
			"the migrated schema carries no property IDs",
			func() *migrateStub {
				stub := planned()
				stub.noPropertyIDs = true
				return stub
			},
			"names no Milestone property to group the board by",
		},
		{
			"the database cannot be inlined",
			func() *migrateStub {
				stub := planned()
				stub.inlineErr = boom
				return stub
			},
			"inline the slices database",
		},
		{
			"the views cannot be listed",
			func() *migrateStub {
				stub := planned()
				stub.listViewsErr = boom
				return stub
			},
			"list the plan's views",
		},
		{
			"the board view cannot be created",
			func() *migrateStub {
				stub := planned()
				stub.createViewErr = boom
				return stub
			},
			"create the board view",
		},
		{
			"the prior view cannot be read",
			func() *migrateStub {
				stub := planned()
				stub.getViewErr = boom
				return stub
			},
			"read the plan's view",
		},
		{
			"the table view cannot be removed",
			func() *migrateStub {
				stub := planned()
				stub.deleteViewErr = boom
				return stub
			},
			"remove the table view",
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
			Migration{Milestones: []string{"M1"}, Slices: 1, StatusRenamed: true, Board: true, MilestonesTrashed: true},
			`Migrated this project: 1 milestone moved onto the slices, 1 slice refiled, ` +
				`"Claimed" renamed to "In progress", the plan brought onto the project page as a board, ` +
				`the old Milestones database moved to Notion's trash.`,
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
