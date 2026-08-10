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

// claimableAPI answers with a plan whose next slice is unambiguous: the Active
// milestone is neither the first nor the last in the plan, and the slices under
// it run through every reason a slice cannot be taken before reaching the one
// that can.
func claimableAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1": conventionBlocks(t),
			"s3":        briefBlocks(t, "Render the board, then stop."),
		},
		pages: map[string][]notion.Page{
			"milestones-ds": {
				milestonePage("m1", "M1: Client", 1, notion.MilestoneDone),
				milestonePage("m2", "M2: Board", 2, notion.MilestoneActive),
				milestonePage("m3", "M3: Later", 3, notion.MilestoneQueued),
			},
			"slices-ds": {
				slicePage("s1", "Notion client", notion.SliceTodo, "m1", "", ""),
				slicePage("s2", "Board scaffolding", notion.SliceDone, "m2", "Craig Johnston", ""),
				slicePage("s3", "Render the board", notion.SliceTodo, "m2", "", ""),
				slicePage("s4", "Style the board", notion.SliceTodo, "m2", "", ""),
				slicePage("s5", "Queued work", notion.SliceTodo, "m3", "", ""),
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
// Active milestone: not a Done or Queued milestone's, not one already held, and
// not a later one under the same milestone.
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
	if name := got.props[notion.PropStatus].SelectName(); name != notion.SliceClaimed {
		t.Errorf("status = %q, want %q", name, notion.SliceClaimed)
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
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	status := api.updates[0].props[notion.PropStatus]
	if status.Status == nil || status.Select != nil {
		t.Errorf("status = %+v, want a status value", status)
	}
}

// The milestones come back in plan order and the slices oldest first, which is
// what makes "the next slice" mean anything.
func TestNextSliceQueriesInPlanOrder(t *testing.T) {
	api := claimableAPI(t)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	want := []query{
		{id: "milestones-ds", sorts: []notion.Sort{{Property: notion.PropOrder, Direction: notion.SortAscending}}},
		{id: "slices-ds", sorts: []notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}}},
	}
	if len(api.queries) != len(want) {
		t.Fatalf("queries = %+v, want %+v", api.queries, want)
	}
	for i, q := range api.queries {
		if q.id != want[i].id || len(q.sorts) != 1 || q.sorts[0] != want[i].sorts[0] {
			t.Errorf("query %d = %+v, want %+v", i, q, want[i])
		}
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
	cfg.Projects["project-1"] = config.ProjectConfig{Name: "nat", MilestonesDSID: "milestones-ds", SlicesDSID: "slices-ds"}
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

	var got nextSliceJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := nextSliceJSON{
		Slice: briefSliceJSON{
			ID: "s3", Name: "Render the board", Status: notion.SliceClaimed,
			Assignee: "Craig Johnston", MilestoneID: "m2", MilestoneName: "M2: Board",
			Repo: "/tmp/nat", Brief: "Render the board, then stop.", URL: "https://notion.so/s3",
		},
		Project: projectJSON{ID: "project-1", Name: "nat", Conventions: "Branch per slice."},
	}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// With nothing to claim the command fails and writes nothing — an agent reading
// it must not mistake "none left" for a brief — and touches no page.
func TestNextSliceReportsNothingToClaim(t *testing.T) {
	tests := []struct {
		name    string
		pages   map[string][]notion.Page
		wantErr []string
	}{
		{
			name: "no Active milestone",
			pages: map[string][]notion.Page{
				"milestones-ds": {milestonePage("m1", "M1: Client", 1, notion.MilestoneQueued)},
				"slices-ds":     {slicePage("s1", "Notion client", notion.SliceTodo, "m1", "", "")},
			},
			wantErr: []string{"no Active milestone"},
		},
		{
			name: "every slice taken",
			pages: map[string][]notion.Page{
				"milestones-ds": {milestonePage("m2", "M2: Board", 2, notion.MilestoneActive)},
				"slices-ds": {
					slicePage("s1", "Held", notion.SliceClaimed, "m2", "Craig Johnston", ""),
					slicePage("s2", "Finished", notion.SliceDone, "m2", "Craig Johnston", ""),
					slicePage("s3", "Todo but held", notion.SliceTodo, "m2", "Someone Else", ""),
					slicePage("s4", "Another milestone's", notion.SliceTodo, "m9", "", ""),
				},
			},
			wantErr: []string{"no unclaimed Todo slice in the Active milestone", "M2: Board"},
		},
		{
			name: "several Active milestones, none with work",
			pages: map[string][]notion.Page{
				"milestones-ds": {
					milestonePage("m1", "M1: Client", 1, notion.MilestoneActive),
					milestonePage("m2", "M2: Board", 2, notion.MilestoneActive),
				},
			},
			wantErr: []string{"Active milestones", "M1: Client, M2: Board"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{pages: tt.pages}
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
			name: "milestones",
			api: func(t *testing.T) *fakeAPI {
				return &fakeAPI{queryErr: map[string]error{"milestones-ds": boom}}
			},
			want: "load milestones",
		},
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
