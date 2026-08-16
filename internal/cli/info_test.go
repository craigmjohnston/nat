package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// conventionBlocks is a project page body, in the shape the blocks endpoint
// returns it — the payload is only reachable through a decode, so the fixture
// is JSON rather than a struct literal.
func conventionBlocks(t *testing.T) []notion.Block {
	t.Helper()
	const raw = `[
		{"id":"b1","type":"paragraph","paragraph":{"rich_text":[{"plain_text":"Branch per slice."}]}}
	]`
	var blocks []notion.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

// title is a title property as a read returns it: the text lives in plain_text,
// which is the field Notion populates for every span type.
func title(s string) notion.PropertyValue {
	return notion.PropertyValue{Title: []notion.RichText{{PlainText: s}}}
}

// slicePage is a row of a Slices data source, its milestone named by the option
// the plan is kept as. Empty assignee, PR or milestone are left off the page
// entirely, the way Notion returns an unset property.
func slicePage(id, name, status, milestone, assignee, pr string) notion.Page {
	props := map[string]notion.PropertyValue{
		notion.PropName:   title(name),
		notion.PropStatus: notion.NewSelect(status),
	}
	if milestone != "" {
		props[notion.PropMilestone] = notion.NewSelect(milestone)
	}
	if assignee != "" {
		props[notion.PropAssignee] = notion.PropertyValue{People: []notion.User{{ID: "u1", Name: assignee}}}
	}
	if pr != "" {
		props[notion.PropPR] = notion.NewURL(pr)
	}
	return notion.Page{ID: id, URL: "https://notion.so/" + id, Properties: props}
}

// populatedAPI answers with a plan of two milestones and three slices, one of
// them belonging to no milestone.
func populatedAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		blocks:      conventionBlocks(t),
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client", "M2: Board")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePage("s1", "Notion client", notion.SliceDone, "M1: Client", "Craig Johnston", "https://github.com/nat/pull/1"),
				slicePage("s2", "Render the board", notion.SliceTodo, "M2: Board", "", ""),
				slicePage("s3", "Stray idea", "", "", "", ""),
			},
		},
	}
}

func TestInfoPrintsTheProjectAsMarkdown(t *testing.T) {
	env, out := testEnv(testConfig(), populatedAPI(t))

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}

	want := `# nat

Branch per slice.

## Milestones

- 0. M1: Client — Done
- 1. M2: Board — Queued

## Slices

### M1: Client

- Notion client — Done · Craig Johnston · PR https://github.com/nat/pull/1

### M2: Board

- Render the board — Todo

### Unassigned

- Stray idea — (no status)
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
}

// The plan is the Milestone column's options, which the schema already carries,
// so the only query is for the slices — oldest first, which is the order agents
// pick them up in.
func TestInfoQueriesOnlyTheSlices(t *testing.T) {
	api := populatedAPI(t)
	env, _ := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}

	want := query{id: "slices-ds", sorts: []notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}}}
	if len(api.queries) != 1 {
		t.Fatalf("queries = %+v, want only %+v", api.queries, want)
	}
	if q := api.queries[0]; q.id != want.id || len(q.sorts) != 1 || q.sorts[0] != want.sorts[0] {
		t.Errorf("query = %+v, want %+v", q, want)
	}
}

// The schema says where the project keeps its milestones, so a read that fails
// stops the command rather than having it guess a shape.
func TestInfoReportsAFailedSchemaRead(t *testing.T) {
	boom := errors.New("boom")
	env, _ := testEnv(testConfig(), &fakeAPI{dataSourceErr: boom})

	err := Run(context.Background(), []string{"info"}, env)
	if err == nil || !strings.Contains(err.Error(), "load the slices schema") {
		t.Fatalf("err = %v, want the schema read reported", err)
	}
	if !errors.Is(err, boom) {
		t.Error("the underlying error should be wrapped, not swallowed")
	}
}

// selectMilestoneSlicesDS is the Slices schema of a project keeping its whole
// plan on one page: the Milestone column is a select naming the milestones,
// with no Milestones data source behind it.
func selectMilestoneSlicesDS(milestones ...string) notion.DataSource {
	return notion.DataSource{
		ID: "slices-ds",
		Properties: map[string]notion.PropertySchema{
			notion.PropStatus:    notion.SchemaSelect(notion.SliceTodo, notion.SliceInProgress, notion.SliceDone),
			notion.PropMilestone: notion.SchemaSelect(milestones...),
			notion.PropDependsOn: dependsOnColumn("slices-ds"),
			notion.PropBranch:    branchColumn(),
		},
	}
}

// branchColumn is the Branch column as a read returns it, there for the same
// reason dependsOnColumn is: every current project has one, and a fixture
// without it is a project the load-time back-fill writes to first.
func branchColumn() notion.PropertySchema {
	schema := notion.SchemaRichText()
	schema.Type = notion.TypeRichText
	return schema
}

// dependsOnColumn is the Depends on column as a read returns it. Every current
// project has one, and a fixture without it is a project the load-time
// back-fill writes to before anything else the test is about.
func dependsOnColumn(dsID string) notion.PropertySchema {
	schema := notion.SchemaRelation(dsID)
	schema.Type = "relation"
	return schema
}

// A project with nothing in it yet still prints its headings, so the output
// says "there is nothing here" rather than looking truncated.
func TestInfoPrintsAnEmptyProject(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{})

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}

	want := `# nat

## Milestones

_none_

## Slices

_none_
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestInfoPrintsJSON(t *testing.T) {
	env, out := testEnv(testConfig(), populatedAPI(t))

	if err := Run(context.Background(), []string{"info", "--json"}, env); err != nil {
		t.Fatalf("info --json: %v", err)
	}

	var got infoJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := infoJSON{
		Project: projectJSON{ID: "project-1", Name: "nat", Conventions: "Branch per slice."},
		Milestones: []milestoneJSON{
			{ID: "M1: Client", Name: "M1: Client", Order: 0, Status: "Done"},
			{ID: "M2: Board", Name: "M2: Board", Order: 1, Status: "Queued"},
		},
		Slices: []sliceJSON{
			{ID: "s1", Name: "Notion client", Status: "Done", MilestoneID: "M1: Client", Assignee: "Craig Johnston", PR: "https://github.com/nat/pull/1", URL: "https://notion.so/s1"},
			{ID: "s2", Name: "Render the board", Status: "Todo", MilestoneID: "M2: Board", URL: "https://notion.so/s2"},
			{ID: "s3", Name: "Stray idea", URL: "https://notion.so/s3"},
		},
	}
	if !reflectEqual(got, want) {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// An empty project encodes as empty arrays rather than nulls, so a consumer can
// range over them without a nil check.
func TestInfoJSONHasEmptyListsNotNulls(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{})

	if err := Run(context.Background(), []string{"info", "--json"}, env); err != nil {
		t.Fatalf("info --json: %v", err)
	}

	if !strings.Contains(out.String(), `"milestones": []`) || !strings.Contains(out.String(), `"slices": []`) {
		t.Errorf("output =\n%s\nwant empty arrays", out.String())
	}
}

func TestInfoJSONReportsAFailedWrite(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"info", "--json"}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

func TestInfoMarkdownReportsAFailedWrite(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	env.Out = failingWriter{}

	err := Run(context.Background(), []string{"info"}, env)

	if !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

func TestInfoReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name string
		api  *fakeAPI
		want string
	}{
		{
			name: "project page",
			api:  &fakeAPI{blocksErr: boom},
			want: "load project page",
		},
		{
			name: "slices",
			api:  &fakeAPI{queryErr: map[string]error{"slices-ds": boom}},
			want: "load slices",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, out := testEnv(testConfig(), tt.api)

			err := Run(context.Background(), []string{"info"}, env)

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

func TestInfoRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"info", "--nope"}, want: "not defined"},
		{name: "stray argument", args: []string{"info", "extra"}, want: `unexpected argument "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := populatedAPI(t)
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if len(api.queries) != 0 {
				t.Errorf("queries = %+v, want none: the command line was rejected", api.queries)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// The command reads the project named by the config's active project, whichever
// of several that is.
func TestInfoReadsTheActiveProject(t *testing.T) {
	cfg := testConfig()
	cfg.ActiveProjectID = "project-2"
	cfg.Projects["project-2"] = config.ProjectConfig{Name: "other", SlicesDSID: "other-slices"}
	api := &fakeAPI{}
	env, out := testEnv(cfg, api)

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}

	if got := api.queries; len(got) != 1 || got[0].id != "other-slices" {
		t.Errorf("queried %v, want the other project's slices", got)
	}
	if !strings.HasPrefix(out.String(), "# other\n") {
		t.Errorf("output =\n%s\nwant it to name the other project", out.String())
	}
}

// reflectEqual compares two info documents by their JSON, which is the form
// that matters and the only one this package promises.
func reflectEqual(a, b infoJSON) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// A plan has no Order to sort its slices by, so info prints them in the order
// the project's own view puts them in.
func TestInfoOrdersThePlanByItsBoard(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client")},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePage("s1", "First read", notion.SliceTodo, "M1: Client", "", ""),
				slicePage("s2", "Second read", notion.SliceTodo, "M1: Client", "", ""),
			},
		},
		order: map[string][]string{"slices-ds": {"s2", "s1"}},
	}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}

	if got := api.ordered; len(got) != 1 || got[0] != "slices-ds" {
		t.Fatalf("read the order of %v, want the slices data source once", got)
	}
	if !strings.Contains(out.String(), "- Second read — Todo\n- First read — Todo") {
		t.Errorf("output =\n%s\nwant the board's order", out.String())
	}
}

// An order that cannot be read costs the plan its order, not its printing.
func TestInfoPrintsThePlanWhenTheOrderCannotBeRead(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client")},
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage("s1", "First read", notion.SliceTodo, "M1: Client", "", "")},
		},
		orderErr: errors.New("boom"),
	}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(out.String(), "- First read — Todo") {
		t.Errorf("output =\n%s\nwant the plan printed anyway", out.String())
	}
}

// A milestone with nothing under it gets a line in the plan but no section of
// its own in the slices, which would otherwise be a heading with nothing after
// it.
func TestInfoLeavesAnEmptyMilestoneOutOfTheSlices(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{"slices-ds": selectMilestoneSlicesDS("M1: Client", "M2: Board")},
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage("s1", "Render the board", notion.SliceTodo, "M2: Board", "", "")},
		},
	}
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"info"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}

	if !strings.Contains(out.String(), "- 0. M1: Client — Queued\n") {
		t.Errorf("output =\n%s\nwant the empty milestone in the plan", out.String())
	}
	if strings.Contains(out.String(), "### M1: Client") {
		t.Errorf("output =\n%s\nwant no slices section for a milestone with none", out.String())
	}
}
