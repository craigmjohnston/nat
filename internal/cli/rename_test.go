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

// filedSlice is a slice page as the plan answers one: its title and the
// milestone it is filed under, which is all a rename reads of it.
func filedSlice(id, name, milestone string) notion.Page {
	return notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
		notion.PropName:      title(name),
		notion.PropStatus:    {Type: notion.TypeSelect, Select: &notion.SelectOption{Name: notion.SliceTodo}},
		notion.PropMilestone: {Type: notion.TypeSelect, Select: &notion.SelectOption{Name: milestone}},
	}}
}

// renamableAPI is a plan of three milestones with two slices filed under the
// middle one, which is what a rename has to carry over.
func renamableAPI() *fakeAPI {
	api := plannedAPI(addedMilestoneID)
	api.pages = map[string][]notion.Page{"slices-ds": {
		filedSlice("s1", "Frame the board", "M1: Client"),
		filedSlice("s2", "Draw a row", "M2: Board"),
		filedSlice("s3", "Draw the plan", "M2: Board"),
	}}
	return api
}

// A rename goes the long way Notion's own silence about renaming an option in
// place calls for, and the milestone keeps both its place in the plan and the
// slices filed under it.
func TestMilestoneRenameRenamesInPlace(t *testing.T) {
	api := renamableAPI()
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(),
		[]string{"milestone-rename", "M2: Board", "M2: The board", "--project", "project-1"}, env); err != nil {
		t.Fatalf("milestone-rename: %v", err)
	}

	want := `# M2: The board

Renamed from M2: Board in nat, still milestone 2, Queued.

- ` + optionNote + `
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
	if len(api.schemaUpdates) != 2 {
		t.Fatalf("schema writes = %+v, want two: the option added, then the old one dropped", api.schemaUpdates)
	}
	if got := api.schemaUpdates[0].props[notion.PropMilestone].OptionNames(); !reflect.DeepEqual(got,
		[]string{"M1: Client", "M2: Board", "M2: The board", "M3: Agents"}) {
		t.Errorf("options written first = %v, want the new name beside the old", got)
	}
	if got := api.schemaUpdates[1].props[notion.PropMilestone].OptionNames(); !reflect.DeepEqual(got,
		[]string{"M1: Client", "M2: The board", "M3: Agents"}) {
		t.Errorf("options written last = %v, want the plan renamed in place", got)
	}
	var refiled []string
	for _, u := range api.updates {
		if got := u.props[notion.PropMilestone].SelectName(); got != "M2: The board" {
			t.Errorf("slice %s refiled under %q, want the new name", u.id, got)
		}
		refiled = append(refiled, u.id)
	}
	if !reflect.DeepEqual(refiled, []string{"s2", "s3"}) {
		t.Errorf("slices refiled = %v, want the milestone's own two", refiled)
	}
	if len(api.creates) != 0 {
		t.Errorf("creates = %+v, want none: a milestone is an option, not a page", api.creates)
	}
}

func TestMilestoneRenamePrintsJSON(t *testing.T) {
	env, out := testEnv(testConfig(), renamableAPI())

	if err := Run(context.Background(),
		[]string{"milestone-rename", "m2: board", "M2: The board", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("milestone-rename: %v", err)
	}

	var got milestoneRenamedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	// The name the milestone went by is reported too: a caller's own record of
	// the plan is keyed by it and has to be told which entry moved.
	want := milestoneRenamedJSON{
		From: "m2: board",
		Milestone: milestoneJSON{
			ID: "M2: The board", Name: "M2: The board", Order: 1, Status: notion.MilestoneQueued,
		},
	}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// Both refusals happen before anything is written, and each names what it
// refused over.
func TestMilestoneRenameRefusals(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a new name the plan already holds",
			args: []string{"milestone-rename", "M2: Board", "  m3: agents  "},
			want: `already has a milestone named "M3: Agents"`,
		},
		{
			name: "the name it already has",
			args: []string{"milestone-rename", "M2: Board", "M2: Board"},
			want: "already has a milestone",
		},
		{
			name: "an old name the plan does not hold",
			args: []string{"milestone-rename", "M9: Nothing", "M4: Polish"},
			want: `no milestone named "M9: Nothing"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := renamableAPI()
			env, out := testEnv(testConfig(), api)
			nudges := nudgeCounter(&env)

			err := Run(context.Background(), append(tt.args, "--project", "project-1"), env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			noWritesBut(t, api, 0)
			if *nudges != 0 {
				t.Errorf("nudges = %d, want none: nothing was written", *nudges)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

// A plan with no milestones at all says so rather than listing nothing.
func TestMilestoneRenameNamesTheMilestonesThePlanHas(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	api.dataSources["slices-ds"] = selectMilestoneSlicesDS()
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(),
		[]string{"milestone-rename", "M1: Client", "M1: The client", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "its milestones are none") {
		t.Fatalf("err = %v, want the empty plan said out loud", err)
	}
}

func TestMilestoneRenameRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no names", args: []string{"milestone-rename", "--project", "project-1"}, want: "given 0"},
		{name: "one name", args: []string{"milestone-rename", "M2: Board", "--project", "project-1"}, want: "given 1"},
		{
			name: "three names",
			args: []string{"milestone-rename", "M2", "M3", "M4", "--project", "project-1"},
			want: "given 3",
		},
		{
			name: "a blank old name",
			args: []string{"milestone-rename", "   ", "M4", "--project", "project-1"},
			want: "name is empty",
		},
		{
			name: "a blank new name",
			args: []string{"milestone-rename", "M2: Board", "  ", "--project", "project-1"},
			want: "name is empty",
		},
		{
			name: "unknown flag",
			args: []string{"milestone-rename", "M2", "M3", "--nope", "--project", "project-1"},
			want: "not defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := renamableAPI()
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "milestone-rename:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			noWritesBut(t, api, 0)
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneRenameReportsAFailedCall(t *testing.T) {
	boom := errors.New("notion: 500")
	tests := []struct {
		name string
		fail func(*fakeAPI)
		want string
	}{
		{
			name: "the schema read",
			fail: func(api *fakeAPI) { api.dataSourceErr = boom },
			want: "load the slices schema",
		},
		{
			name: "the plan read",
			fail: func(api *fakeAPI) { api.queryErr = map[string]error{"slices-ds": boom} },
			want: "load slices",
		},
		{
			name: "the schema write",
			fail: func(api *fakeAPI) { api.schemaUpdateErr = boom },
			want: `add the "M2: The board" option`,
		},
		{
			name: "a slice that could not be refiled",
			fail: func(api *fakeAPI) { api.updateErr = boom },
			want: `refile slice s2 under "M2: The board"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := renamableAPI()
			tt.fail(api)
			env, out := testEnv(testConfig(), api)
			nudges := nudgeCounter(&env)

			err := Run(context.Background(),
				[]string{"milestone-rename", "M2: Board", "M2: The board", "--project", "project-1"}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if *nudges != 0 {
				t.Errorf("nudges = %d, want none: the rename did not land", *nudges)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneRenameNeedsAConfiguredProject(t *testing.T) {
	api := renamableAPI()
	env, _ := testEnv(testConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(),
		[]string{"milestone-rename", "M2: Board", "M2: The board", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want the setup reported", err)
	}
	noWritesBut(t, api, 0)
}

func TestMilestoneRenameReportsAFailedWrite(t *testing.T) {
	for _, extra := range [][]string{nil, {"--json"}} {
		t.Run(strings.Join(append([]string{"milestone-rename"}, extra...), " "), func(t *testing.T) {
			env, _ := testEnv(testConfig(), renamableAPI())
			env.Out = failingWriter{}

			args := append([]string{"milestone-rename", "M2: Board", "M2: The board"}, extra...)
			err := Run(context.Background(), append(args, "--project", "project-1"), env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}
