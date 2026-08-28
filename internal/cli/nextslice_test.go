package cli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// briefBlocks is a slice page body, in the shape the blocks endpoint returns it.
func briefBlocks(t *testing.T, text string) []notion.Block {
	t.Helper()
	raw := `[{"id":"b1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":` +
		mustJSON(t, text) + `}]}}]`
	var blocks []notion.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// claimableAPI answers with a plan whose next slice is unambiguous: the
// milestone work is taken from is neither the first nor the last in the plan —
// the first is finished — and the slices under it run through every reason a
// slice cannot be taken before reaching the one that can.
func claimableAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1": conventionBlocks(t),
			"s3":        briefBlocks(t, "Render the board, then stop."),
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client", "M2: Board", "M3: Later"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePage("s1", "Notion client", notion.SliceDone, "M1: Client", "Craig Johnston", ""),
				slicePage("s2", "Board scaffolding", notion.SliceDone, "M2: Board", "Craig Johnston", ""),
				slicePage("s3", "Render the board", notion.SliceTodo, "M2: Board", "", ""),
				slicePage("s4", "Style the board", notion.SliceTodo, "M2: Board", "", ""),
				slicePage("s5", "Queued work", notion.SliceTodo, "M3: Later", "", ""),
			},
		},
	}
}

// testClaimConfig is a config with an assignee to claim as, which is what
// onboarding writes and what claiming needs.
func testClaimConfig() config.Config {
	cfg := testConfig()
	cfg.AssigneeUserID = "u1"
	cfg.AssigneeUserName = "Craig Johnston"
	return cfg
}

func TestNextSliceClaimsAndPrintsTheBrief(t *testing.T) {
	api := claimableAPI(t)
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	want := `# Render the board

Claimed for Craig Johnston. Work exactly this slice.

- Project: nat
- Project page ID: project-1 (pass it as --project on every nat command)
- Milestone: M2: Board
- Notion page: s3
- Notion URL: https://notion.so/s3
- Working directory: /tmp/nat

## Brief

Render the board, then stop.

## Project conventions

Branch per slice.
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
}

// The slice taken is the oldest unclaimed Todo one under the lowest-ordered
// milestone still open: not a finished or later milestone's, not one already
// held, and not a later one under the same milestone.
func TestNextSliceClaimsTheRightSlice(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	got := api.updates[0]
	if got.id != "s3" {
		t.Errorf("claimed %q, want s3", got.id)
	}
	if ids := got.props[notion.PropAssignee].PeopleIDs(); len(ids) != 1 || ids[0] != "u1" {
		t.Errorf("assignee = %v, want [u1]", ids)
	}
	if name := got.props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
		t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
	}
	if got.props[notion.PropStatus].Select == nil {
		t.Errorf("status = %+v, want a select, the shape the page was read in", got.props[notion.PropStatus])
	}
}

// A Status column converted to Notion's own status type in the UI is written
// back in that shape, not as the select this app would have created.
func TestNextSliceWritesTheStatusShapeItRead(t *testing.T) {
	api := claimableAPI(t)
	slices := api.pages["slices-ds"]
	slices[2].Properties[notion.PropStatus] = notion.PropertyValue{
		Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceTodo},
	}
	ds := assigneeSlicesDS("M1: Client", "M2: Board", "M3: Later")
	ds.Properties[notion.PropStatus] = notion.PropertySchema{
		Type:   notion.TypeStatus,
		Status: &notion.OptionsConfig{Options: []notion.SelectOption{{Name: notion.SliceInProgress}}},
	}
	api.dataSources = map[string]notion.DataSource{"slices-ds": ds}
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	status := api.updates[0].props[notion.PropStatus]
	if status.Status == nil || status.Select != nil {
		t.Errorf("status = %+v, want a status value", status)
	}
}

// The plan comes with the schema, so the slices are the only query — oldest
// first, which is what makes "the next slice" mean anything.
func TestNextSliceQueriesOnlyTheSlices(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	want := query{id: "slices-ds", sorts: []notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}}}
	if len(api.queries) != 1 {
		t.Fatalf("queries = %+v, want only %+v", api.queries, want)
	}
	if q := api.queries[0]; q.id != want.id || len(q.sorts) != 1 || q.sorts[0] != want.sorts[0] {
		t.Errorf("query = %+v, want %+v", q, want)
	}
}

// A slice carrying a Repo override is worked there rather than in the project's
// default directory.
func TestNextSliceHonoursARepoOverride(t *testing.T) {
	api := claimableAPI(t)
	api.pages["slices-ds"][2].Properties[notion.PropRepo] = notion.PropertyValue{
		RichText: []notion.RichText{{PlainText: "/tmp/other"}},
	}
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if !strings.Contains(out.String(), "- Working directory: /tmp/other\n") {
		t.Errorf("output =\n%s\nwant the slice's own repo", out.String())
	}
}

// A slice with no brief written on it, in a project with no conventions, still
// prints both headings — an empty one reads as output that got cut off.
func TestNextSlicePrintsEmptyBodies(t *testing.T) {
	api := claimableAPI(t)
	api.blocksByID = nil
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if strings.Count(out.String(), "_none_\n") != 2 {
		t.Errorf("output =\n%s\nwant both bodies reported as empty", out.String())
	}
}

// A page with no URL — which a Notion page always has, but a fixture need not —
// simply leaves the line out rather than printing an empty one.
func TestNextSliceOmitsAMissingURL(t *testing.T) {
	api := claimableAPI(t)
	api.pages["slices-ds"][2].URL = ""
	cfg := testClaimConfig()
	cfg.Projects["project-1"] = config.ProjectConfig{Name: "nat", SlicesDSID: "slices-ds"}
	env, out := testEnv(cfg, api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if strings.Contains(out.String(), "Notion URL") || strings.Contains(out.String(), "Working directory") {
		t.Errorf("output =\n%s\nwant no empty facts", out.String())
	}
}

func TestNextSlicePrintsJSON(t *testing.T) {
	api := claimableAPI(t)
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice", "--json"}, env); err != nil {
		t.Fatalf("next-slice --json: %v", err)
	}

	var got briefJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := briefJSON{
		Slice: briefSliceJSON{
			ID: "s3", Name: "Render the board", Status: notion.SliceInProgress,
			Assignee: "Craig Johnston", MilestoneID: "M2: Board", MilestoneName: "M2: Board",
			Repo: "/tmp/nat", Brief: "Render the board, then stop.", URL: "https://notion.so/s3",
		},
		Project: projectJSON{ID: "project-1", Name: "nat", Conventions: "Branch per slice."},
	}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// A claim that comes back without the assignee on it — the shape Notion answers
// with when it will not record the write — is a claim that did not happen.
func TestNextSliceReportsAClaimThatDidNotStick(t *testing.T) {
	tests := []struct {
		name   string
		mangle func(*notion.Page)
	}{
		{
			name:   "assignee dropped",
			mangle: func(p *notion.Page) { delete(p.Properties, notion.PropAssignee) },
		},
		{
			name: "someone else holds it",
			mangle: func(p *notion.Page) {
				p.Properties[notion.PropAssignee] = notion.NewPeople("u2")
			},
		},
		{
			name: "status unchanged",
			mangle: func(p *notion.Page) {
				p.Properties[notion.PropStatus] = notion.NewSelect(notion.SliceTodo)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := claimableAPI(t)
			api.mangle = tt.mangle
			env, out := testEnv(testClaimConfig(), api)

			err := Run(context.Background(), []string{"next-slice"}, env)

			if err == nil || !strings.Contains(err.Error(), "did not stick") {
				t.Fatalf("err = %v, want a refused claim", err)
			}
			if !strings.Contains(err.Error(), "Render the board") {
				t.Errorf("err = %q, want it to name the slice", err)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestNextSliceReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name string
		api  func(*testing.T) *fakeAPI
		want string
	}{
		{
			name: "slices",
			api: func(t *testing.T) *fakeAPI {
				return &fakeAPI{queryErr: map[string]error{"slices-ds": boom}}
			},
			want: "load slices",
		},
		{
			name: "the claim",
			api: func(t *testing.T) *fakeAPI {
				api := claimableAPI(t)
				api.updateErr = boom
				return api
			},
			want: "claim the slice",
		},
		{
			name: "the brief",
			api: func(t *testing.T) *fakeAPI {
				api := claimableAPI(t)
				api.blocksErrByID = map[string]error{"s3": boom}
				return api
			},
			want: `claimed "Render the board" but could not read its brief`,
		},
		{
			name: "the conventions",
			api: func(t *testing.T) *fakeAPI {
				api := claimableAPI(t)
				api.blocksErrByID = map[string]error{"project-1": boom}
				return api
			},
			want: `claimed "Render the board" but could not read the project conventions`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, out := testEnv(testClaimConfig(), tt.api(t))

			err := Run(context.Background(), []string{"next-slice"}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// Claiming needs someone to claim as, and that comes from the config the board
// wrote. Without it nothing is read and nothing is written.
func TestNextSliceNeedsAnAssignee(t *testing.T) {
	api := claimableAPI(t)
	env, out := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"next-slice"}, env)

	if err == nil || !strings.Contains(err.Error(), "no assignee in the config") {
		t.Fatalf("err = %v, want it to ask for an assignee", err)
	}
	if len(api.queries) != 0 || len(api.updates) != 0 {
		t.Errorf("calls = %+v %+v, want none", api.queries, api.updates)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

// Setup that has not happened yet is reported before anything is claimed.
func TestNextSliceReportsUnfinishedSetup(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"next-slice"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want it to point at setup", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want none", api.updates)
	}
}

func TestNextSliceReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{{"next-slice"}, {"next-slice", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _ := testEnv(testClaimConfig(), claimableAPI(t))
			env.Out = failingWriter{}

			err := Run(context.Background(), args, env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}

func TestNextSliceRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"next-slice", "--nope"}, want: "not defined"},
		{name: "stray argument", args: []string{"next-slice", "extra"}, want: `unexpected argument "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := claimableAPI(t)
			env, out := testEnv(testClaimConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "next-slice:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			if len(api.queries) != 0 || len(api.updates) != 0 {
				t.Errorf("calls = %+v %+v, want none: the command line was rejected", api.queries, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A project created without an Assignee column is claimed on status alone: the
// in-progress option is the one its own schema offers, and no people property
// is written to a table that has none.
func TestNextSliceClaimsAProjectWithNoAssigneeColumn(t *testing.T) {
	api := claimableAPI(t)
	api.dataSources = map[string]notion.DataSource{
		"slices-ds": selectMilestoneSlicesDS("M1: Client", "M2: Board", "M3: Later"),
	}
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	got := api.updates[0]
	if _, wrote := got.props[notion.PropAssignee]; wrote {
		t.Errorf("props = %+v, want no assignee written to a table without the column", got.props)
	}
	if name := got.props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
		t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
	}
	if !strings.Contains(out.String(), "Claimed for Craig Johnston") {
		t.Errorf("output =\n%s\nwant the brief for the slice it took", out.String())
	}
}

// The slice handed out is the one at the top of the milestone on the project's
// own board, not whichever the query happened to return first: a plan written
// in one go shares a created time to the minute, so the board's order is the
// only order it has.
func TestNextSliceTakesTheTopSliceOfTheBoard(t *testing.T) {
	api := claimableAPI(t)
	api.order = map[string][]string{"slices-ds": {"s1", "s2", "s5", "s4", "s3"}}
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != "s4" {
		t.Fatalf("updates = %+v, want s4, the board's first Todo slice of M2: Board", api.updates)
	}
	if len(api.ordered) != 1 || api.ordered[0] != "slices-ds" {
		t.Errorf("order reads = %v, want the slices' own view read once", api.ordered)
	}
}

// An order that cannot be read is not worth refusing to work over: the slices
// stay in the order they were queried and the next one is still handed out.
func TestNextSliceWorksWithoutAReadableBoardOrder(t *testing.T) {
	api := claimableAPI(t)
	api.orderErr = errors.New("notion: 500")
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != "s3" {
		t.Fatalf("updates = %+v, want s3, the plan in the order it was queried", api.updates)
	}
}

// With nothing left to take the refusal says so in the terms the plan is kept
// in: there are no statuses to activate, so it is the milestones themselves
// that are finished or empty. Nothing is written either — an agent reading the
// refusal must not mistake "none left" for a brief.
func TestNextSliceReportsNothingToClaim(t *testing.T) {
	tests := []struct {
		name    string
		options []string
		slices  []notion.Page
		wantErr []string
	}{
		{
			name:    "every milestone Done",
			options: []string{"M1: Client", "M2: Board"},
			slices: []notion.Page{
				slicePage("s1", "Notion client", notion.SliceDone, "M1: Client", "", ""),
				slicePage("s2", "Render the board", notion.SliceDone, "M2: Board", "", ""),
			},
			wantErr: []string{"no unfinished milestone", "every milestone in the plan is Done"},
		},
		{
			name:    "no milestone in the plan at all",
			options: []string{},
			slices:  []notion.Page{slicePage("s1", "Stray idea", notion.SliceTodo, "", "", "")},
			wantErr: []string{"no unfinished milestone"},
		},
		{
			name:    "nothing unclaimed under the unfinished milestones",
			options: []string{"M1: Client", "M2: Board"},
			slices: []notion.Page{
				slicePage("s1", "Notion client", notion.SliceInProgress, "M1: Client", "", ""),
				slicePage("s2", "Render the board", notion.SliceDone, "M2: Board", "", ""),
			},
			wantErr: []string{"no unclaimed Todo slice in the unfinished milestone", "M1: Client"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{
				dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS(tt.options...)},
				pages:       map[string][]notion.Page{"slices-ds": tt.slices},
			}
			env, out := testEnv(testClaimConfig(), api)

			err := Run(context.Background(), []string{"next-slice"}, env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to mention %q", err, want)
				}
			}
			if len(api.updates) != 0 {
				t.Errorf("updates = %+v, want none", api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A slice whose milestone is not in the plan — an option since deleted out from
// under it — is not work anyone is owed, so it is passed over rather than
// claimed under a milestone that isn't there.
func TestNextSlicePassesOverASliceOutsideThePlan(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client")},
		pages:       map[string][]notion.Page{"slices-ds": {slicePage("s1", "Orphan", notion.SliceTodo, "gone", "", "")}},
	}
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"next-slice"}, env)

	if err == nil || !strings.Contains(err.Error(), "no unclaimed Todo slice") {
		t.Fatalf("err = %v, want the orphan passed over", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want none", api.updates)
	}
}

// The schema is read before anything is claimed, so a failure to read it leaves
// the plan untouched.
func TestNextSliceReportsAFailedSchemaRead(t *testing.T) {
	api := claimableAPI(t)
	api.dataSourceErr = errors.New("boom")
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"next-slice"}, env)
	if err == nil || !strings.Contains(err.Error(), "load the slices schema") {
		t.Fatalf("err = %v, want the schema read named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A project still in the shape this app started with is migrated on the way to
// the command that reads it, so an agent works a plan of the one shape whatever
// the project was stored as.
func TestNextSliceMigratesAnOldProject(t *testing.T) {
	old := notion.DataSource{ID: "slices-ds", Properties: map[string]notion.PropertySchema{
		notion.PropStatus:    notion.SchemaSelect(notion.SliceTodo, notion.SliceClaimed, notion.SliceDone),
		notion.PropDependsOn: dependsOnColumn("slices-ds"),
		notion.PropBranch:    branchColumn(),
		notion.PropMilestone: {
			Type:     "relation",
			Relation: &notion.RelationConfig{DataSourceID: "milestones-ds"},
		},
		notion.PropAssignee: {Type: notion.TypePeople},
	}}
	api := &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1": conventionBlocks(t),
			"s2":        briefBlocks(t, "Render the board, then stop."),
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": old,
			"milestones-ds": {ID: "milestones-ds",
				Parent: notion.Parent{Type: notion.ParentDatabase, DatabaseID: "milestones-db"}},
		},
		pages: map[string][]notion.Page{
			"milestones-ds": {
				{ID: "m1", Properties: map[string]notion.PropertyValue{notion.PropName: title("M1: Client")}},
				{ID: "m2", Properties: map[string]notion.PropertyValue{notion.PropName: title("M2: Board")}},
			},
			"slices-ds": {
				relatedSlicePage("s1", "Notion client", notion.SliceDone, "m1"),
				relatedSlicePage("s2", "Render the board", notion.SliceTodo, "m2"),
			},
		},
	}
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	// The milestones moved onto the slices' own column, the slices were refiled
	// under them, and the claim went to the slice under M2 — all of which needs
	// the plan to have been read in its new shape.
	if len(api.schemaUpdates) != 2 {
		t.Fatalf("schema writes = %+v, want the migration's two", api.schemaUpdates)
	}
	written := api.schemaUpdates[0].props
	if got := written[notion.PropMilestone].OptionNames(); !reflect.DeepEqual(got, []string{"M1: Client", "M2: Board"}) {
		t.Errorf("options = %v, want the milestones moved onto the column", got)
	}
	// In progress arrives alongside Claimed first — the API will not rename an
	// option in place — and Claimed is retired by the second write.
	if got := written[notion.PropStatus].OptionNames(); !reflect.DeepEqual(got,
		[]string{notion.SliceTodo, notion.SliceClaimed, notion.SliceDone, notion.SliceInProgress}) {
		t.Errorf("status options = %v, want the new name appended", got)
	}
	if got := api.schemaUpdates[1].props[notion.PropStatus].OptionNames(); !reflect.DeepEqual(got,
		[]string{notion.SliceTodo, notion.SliceDone, notion.SliceInProgress}) {
		t.Errorf("status options = %v, want the old name retired", got)
	}
	if want := []string{"milestones-db"}; !reflect.DeepEqual(api.deletes, want) {
		t.Errorf("deletes = %v, want the Milestones database trashed", api.deletes)
	}
	var claimed string
	for _, u := range api.updates {
		if _, ok := u.props[notion.PropStatus]; ok {
			claimed = u.id
		}
	}
	if claimed != "s2" {
		t.Errorf("claimed %q, want s2", claimed)
	}
	if !strings.Contains(out.String(), "- Milestone: M2: Board\n") {
		t.Errorf("output =\n%s\nwant the migrated milestone named", out.String())
	}
}

// relatedSlicePage is a slice of a project in the old shape: its milestone a
// relation to a page of a Milestones data source.
func relatedSlicePage(id, name, status, milestoneID string) notion.Page {
	p := slicePage(id, name, status, "", "", "")
	p.Properties[notion.PropMilestone] = notion.PropertyValue{Relation: &[]notion.Relation{{ID: milestoneID}}}
	return p
}
