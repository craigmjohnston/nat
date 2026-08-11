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

// startSliceID is the page ID the slice is named by throughout, in the shape
// the command insists on: a real Notion ID, not a fixture's shorthand.
const startSliceID = "3b838308f654816da085f46dd135ade3"

// startableAPI answers with a plan holding one takeable slice under one
// milestone, which is all a command handed a slice by name ever looks at.
func startableAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1":  conventionBlocks(t),
			startSliceID: briefBlocks(t, "Render the board, then stop."),
		},
		pages: map[string][]notion.Page{
			"milestones-ds": {milestonePage("m2", "M2: Board", 2, notion.MilestoneActive)},
			"slices-ds":     {slicePage(startSliceID, "Render the board", notion.SliceTodo, "m2", "", "")},
		},
	}
}

func TestStartSliceClaimsTheNamedSliceAndPrintsTheBrief(t *testing.T) {
	api := startableAPI(t)
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"start-slice", startSliceID}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	want := `# Render the board

Claimed for Craig Johnston. Work exactly this slice.

- Project: nat
- Milestone: M2: Board
- Notion page: 3b838308f654816da085f46dd135ade3
- Notion URL: https://notion.so/3b838308f654816da085f46dd135ade3
- Working directory: /tmp/nat

## Brief

Render the board, then stop.

## Project conventions

Branch per slice.
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
	if len(api.updates) != 1 || api.updates[0].id != startSliceID {
		t.Fatalf("updates = %+v, want exactly the named slice claimed", api.updates)
	}
	if ids := api.updates[0].props[notion.PropAssignee].PeopleIDs(); len(ids) != 1 || ids[0] != "u1" {
		t.Errorf("assignee = %v, want [u1]", ids)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceClaimed {
		t.Errorf("status = %q, want %q", name, notion.SliceClaimed)
	}
	if len(api.queries) != 0 {
		t.Errorf("queries = %+v, want none: the slice was named, not chosen", api.queries)
	}
}

// The slice may be named by its Notion URL as readily as by its ID, since the
// brief prints both.
func TestStartSliceTakesAURL(t *testing.T) {
	api := startableAPI(t)
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"start-slice",
		"https://www.notion.so/Render-the-board-" + startSliceID + "?pvs=4"}, env)

	if err != nil {
		t.Fatalf("start-slice: %v", err)
	}
	if len(api.updates) != 1 || api.updates[0].id != startSliceID {
		t.Errorf("updates = %+v, want the slice the URL named claimed", api.updates)
	}
}

// Flags may come either side of the slice, which is the order anyone writes
// them in.
func TestStartSlicePrintsJSON(t *testing.T) {
	for _, args := range [][]string{{"start-slice", startSliceID, "--json"}, {"start-slice", "--json", startSliceID}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, out := testEnv(testClaimConfig(), startableAPI(t))

			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("%v: %v", args, err)
			}

			var got briefJSON
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("output is not JSON: %v\n%s", err, out.String())
			}
			want := briefJSON{
				Slice: briefSliceJSON{
					ID: startSliceID, Name: "Render the board", Status: notion.SliceClaimed,
					Assignee: "Craig Johnston", MilestoneID: "m2", MilestoneName: "M2: Board",
					Repo: "/tmp/nat", Brief: "Render the board, then stop.", URL: "https://notion.so/" + startSliceID,
				},
				Project: projectJSON{ID: "project-1", Name: "nat", Conventions: "Branch per slice."},
			}
			if got != want {
				t.Errorf("json = %+v\nwant %+v", got, want)
			}
		})
	}
}

// A slice's own Repo override wins over the project default, the same way it
// does for next-slice.
func TestStartSliceHonoursARepoOverride(t *testing.T) {
	api := startableAPI(t)
	api.pages["slices-ds"][0].Properties[notion.PropRepo] = notion.PropertyValue{
		RichText: []notion.RichText{{PlainText: "/tmp/other"}},
	}
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"start-slice", startSliceID}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	if !strings.Contains(out.String(), "- Working directory: /tmp/other\n") {
		t.Errorf("output =\n%s\nwant the slice's own repo", out.String())
	}
}

// A slice belonging to no milestone is still startable — the board shows those
// too — and the brief simply has no milestone to name.
func TestStartSliceTakesASliceWithNoMilestone(t *testing.T) {
	api := startableAPI(t)
	delete(api.pages["slices-ds"][0].Properties, notion.PropMilestone)
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"start-slice", startSliceID}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	if strings.Contains(out.String(), "Milestone") {
		t.Errorf("output =\n%s\nwant no milestone line", out.String())
	}
	if len(api.updates) != 1 {
		t.Errorf("updates = %+v, want the slice claimed", api.updates)
	}
}

// A slice somebody has already started is refused, and nothing at all is
// written — the point of naming a slice is that the caller may have named the
// wrong one.
func TestStartSliceRefusesASliceAlreadyUnderway(t *testing.T) {
	tests := []struct {
		name  string
		slice notion.Page
		want  []string
	}{
		{
			name:  "claimed",
			slice: slicePage(startSliceID, "Render the board", notion.SliceClaimed, "m2", "Craig Johnston", ""),
			want:  []string{`"Render the board" is Claimed, not Todo`},
		},
		{
			name:  "done",
			slice: slicePage(startSliceID, "Render the board", notion.SliceDone, "m2", "Craig Johnston", ""),
			want:  []string{`"Render the board" is Done, not Todo`},
		},
		{
			name:  "no status at all",
			slice: slicePage(startSliceID, "Render the board", "", "m2", "", ""),
			want:  []string{"(no status), not Todo"},
		},
		{
			name:  "todo but assigned to someone else",
			slice: slicePage(startSliceID, "Render the board", notion.SliceTodo, "m2", "Someone Else", ""),
			want:  []string{"Todo but assigned to Someone Else", "leave it to them"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := startableAPI(t)
			api.pages["slices-ds"][0] = tt.slice
			env, out := testEnv(testClaimConfig(), api)

			err := Run(context.Background(), []string{"start-slice", startSliceID}, env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to mention %q", err, want)
				}
			}
			var usage *UsageError
			if errors.As(err, &usage) {
				t.Errorf("err = %v, want a plain error: the command line was fine", err)
			}
			if len(api.updates) != 0 || len(api.appends) != 0 {
				t.Errorf("writes = %+v %+v, want none", api.updates, api.appends)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A claim that comes back held by somebody else is a race lost, and is reported
// rather than papered over with a brief.
func TestStartSliceReportsAClaimThatDidNotStick(t *testing.T) {
	api := startableAPI(t)
	api.mangle = func(p *notion.Page) { p.Properties[notion.PropAssignee] = notion.NewPeople("u2") }
	env, out := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"start-slice", startSliceID}, env)

	if err == nil || !strings.Contains(err.Error(), "did not stick") {
		t.Fatalf("err = %v, want a refused claim", err)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

func TestStartSliceReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name string
		api  func(*testing.T) *fakeAPI
		err  error
		want string
	}{
		{
			name: "the slice",
			api: func(t *testing.T) *fakeAPI {
				api := startableAPI(t)
				api.getErr = boom
				return api
			},
			err:  boom,
			want: "load the slice",
		},
		{
			name: "the claim",
			api: func(t *testing.T) *fakeAPI {
				api := startableAPI(t)
				api.updateErr = boom
				return api
			},
			err:  boom,
			want: "claim the slice",
		},
		{
			name: "the brief",
			api: func(t *testing.T) *fakeAPI {
				api := startableAPI(t)
				api.blocksErrByID = map[string]error{startSliceID: boom}
				return api
			},
			err:  boom,
			want: `claimed "Render the board" but could not read its brief`,
		},
		{
			name: "the conventions",
			api: func(t *testing.T) *fakeAPI {
				api := startableAPI(t)
				api.blocksErrByID = map[string]error{"project-1": boom}
				return api
			},
			err:  boom,
			want: `claimed "Render the board" but could not read the project conventions`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, out := testEnv(testClaimConfig(), tt.api(t))

			err := Run(context.Background(), []string{"start-slice", startSliceID}, env)

			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
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

// A milestone that cannot be read leaves the slice claimed but the brief
// unprintable, which is said as such rather than reported as a failed claim.
func TestStartSliceReportsAnUnreadableMilestone(t *testing.T) {
	api := startableAPI(t)
	api.pages["milestones-ds"] = nil
	env, out := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"start-slice", startSliceID}, env)

	if err == nil || !strings.Contains(err.Error(), `claimed "Render the board" but could not read its milestone`) {
		t.Fatalf("err = %v, want the milestone read reported", err)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}

// A slice named as something that is not a page at all never reaches Notion.
func TestStartSliceRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no slice", args: []string{"start-slice"}, want: "given 0"},
		{name: "two slices", args: []string{"start-slice", startSliceID, "s4"}, want: "given 2"},
		{name: "not a page", args: []string{"start-slice", "the board one"}, want: "is not a slice"},
		{name: "unknown flag", args: []string{"start-slice", startSliceID, "--nope"}, want: "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := startableAPI(t)
			env, out := testEnv(testClaimConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "start-slice:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			if len(api.gets) != 0 || len(api.updates) != 0 {
				t.Errorf("calls = %+v %+v, want none: the command line was rejected", api.gets, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// Claiming needs someone to claim as, and setup that has not happened yet is
// reported before the slice is so much as read.
func TestStartSliceNeedsAConfiguredProject(t *testing.T) {
	tests := []struct {
		name string
		env  func(*Env)
		want string
	}{
		{
			name: "no assignee",
			env:  func(e *Env) { e.Load = func() (config.Config, bool, error) { return testConfig(), true, nil } },
			want: "no assignee in the config",
		},
		{
			name: "no config file",
			env:  func(e *Env) { e.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil } },
			want: "run `nat` once to set it up",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := startableAPI(t)
			env, out := testEnv(testClaimConfig(), api)
			tt.env(&env)

			err := Run(context.Background(), []string{"start-slice", startSliceID}, env)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
			if len(api.gets) != 0 || len(api.updates) != 0 {
				t.Errorf("calls = %+v %+v, want none", api.gets, api.updates)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestStartSliceReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{{"start-slice", startSliceID}, {"start-slice", startSliceID, "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, _ := testEnv(testClaimConfig(), startableAPI(t))
			env.Out = failingWriter{}

			err := Run(context.Background(), args, env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}

// The same project shape, taken by name rather than chosen.
func TestStartSliceClaimsAProjectWithNoAssigneeColumn(t *testing.T) {
	api := startableAPI(t)
	api.dataSources = map[string]notion.DataSource{"slices-ds": inProgressSlicesDS()}
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"start-slice", startSliceID}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	if _, wrote := api.updates[0].props[notion.PropAssignee]; wrote {
		t.Errorf("props = %+v, want no assignee written", api.updates[0].props)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceInProgress {
		t.Errorf("status = %q, want %q", name, notion.SliceInProgress)
	}
	if !strings.Contains(out.String(), "Claimed for Craig Johnston") {
		t.Errorf("output =\n%s\nwant the brief", out.String())
	}
}

func TestStartSliceReportsAFailedSchemaRead(t *testing.T) {
	api := startableAPI(t)
	api.dataSourceErr = errors.New("boom")
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"start-slice", startSliceID}, env)
	if err == nil || !strings.Contains(err.Error(), "read the slices schema") {
		t.Fatalf("err = %v, want the schema read named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}
