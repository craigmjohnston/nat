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

// removableAPI is a plan of three milestones with two slices filed under the
// middle one, so the same fixture says something about both endings: the empty
// milestone goes, and the one holding work is refused.
func removableAPI() *fakeAPI {
	api := plannedAPI(addedMilestoneID)
	api.pages = map[string][]notion.Page{"slices-ds": {
		filedSlice("s1", "Frame the board", "M1: Client"),
		filedSlice("s2", "Draw a row", "M2: Board"),
		filedSlice("s3", "Draw the plan", "M2: Board"),
	}}
	return api
}

// An empty milestone goes in one schema write, and the options that survive it
// are sent back as they were read — which is what leaves the rest of the plan
// in the order it was in.
func TestMilestoneRemoveDropsAnEmptyMilestone(t *testing.T) {
	api := removableAPI()
	env, out := testEnv(testConfig(), api)
	nudges := nudgeCounter(&env)

	if err := Run(context.Background(),
		[]string{"milestone-remove", "M3: Agents", "--project", "project-1"}, env); err != nil {
		t.Fatalf("milestone-remove: %v", err)
	}

	want := `# M3: Agents

Removed from nat, where it was milestone 3 and held no slices.

- ` + removedNote + `
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
	}
	if len(api.schemaUpdates) != 1 {
		t.Fatalf("schema writes = %+v, want exactly one", api.schemaUpdates)
	}
	if got := api.schemaUpdates[0].props[notion.PropMilestone].OptionNames(); !reflect.DeepEqual(got,
		[]string{"M1: Client", "M2: Board"}) {
		t.Errorf("options written = %v, want the plan minus the one removed, in order", got)
	}
	if len(api.updates) != 0 || len(api.creates) != 0 {
		t.Errorf("page writes = %+v %+v, want none: a milestone is an option", api.updates, api.creates)
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want one: the removal landed", *nudges)
	}
}

func TestMilestoneRemovePrintsJSON(t *testing.T) {
	env, out := testEnv(testConfig(), removableAPI())

	if err := Run(context.Background(),
		[]string{"milestone-remove", "  m3: agents  ", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("milestone-remove: %v", err)
	}

	var got milestoneRemovedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	// The place it held is reported, since that is the one thing about a removed
	// milestone there is no longer anywhere to read.
	want := milestoneRemovedJSON{Milestone: milestoneJSON{
		ID: "M3: Agents", Name: "M3: Agents", Order: 2, Status: notion.MilestoneQueued,
	}}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// Both refusals happen before anything is written, and each names what it
// refused over — the slices by name, since moving them is what happens next.
func TestMilestoneRemoveRefusals(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "a name the plan does not hold",
			args: []string{"milestone-remove", "M9: Nothing"},
			want: []string{`no milestone named "M9: Nothing"`, `"M1: Client"`},
		},
		{
			name: "a milestone with slices still filed under it",
			args: []string{"milestone-remove", "m2: board"},
			want: []string{
				`the milestone "M2: Board" still holds 2 slices`,
				`"Draw a row", "Draw the plan"`,
				"move or delete them first",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := removableAPI()
			env, out := testEnv(testConfig(), api)
			nudges := nudgeCounter(&env)

			err := Run(context.Background(), append(tt.args, "--project", "project-1"), env)

			if err == nil {
				t.Fatal("err = nil, want a refusal")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want it to mention %q", err, want)
				}
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

// One slice under a milestone is one slice, not "1 slices".
func TestMilestoneRemoveCountsOneSlice(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	api.pages = map[string][]notion.Page{"slices-ds": {filedSlice("s1", "Draw a row", "M2: Board")}}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"milestone-remove", "M2: Board", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), `still holds 1 slice ("Draw a row")`) {
		t.Fatalf("err = %v, want the one slice named in the singular", err)
	}
}

// A plan with no milestones at all says so rather than listing nothing.
func TestMilestoneRemoveNamesTheMilestonesThePlanHas(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	api.dataSources["slices-ds"] = selectMilestoneSlicesDS()
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"milestone-remove", "M1: Client", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "its milestones are none") {
		t.Fatalf("err = %v, want the empty plan said out loud", err)
	}
}

func TestMilestoneRemoveRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no name", args: []string{"milestone-remove", "--project", "project-1"}, want: "given 0"},
		{
			name: "two names",
			args: []string{"milestone-remove", "M2: Board", "M3: Agents", "--project", "project-1"},
			want: "given 2",
		},
		{
			name: "a blank name",
			args: []string{"milestone-remove", "   ", "--project", "project-1"},
			want: "name is empty",
		},
		{
			name: "unknown flag",
			args: []string{"milestone-remove", "M3: Agents", "--nope", "--project", "project-1"},
			want: "not defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := removableAPI()
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "milestone-remove:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			noWritesBut(t, api, 0)
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneRemoveReportsAFailedCall(t *testing.T) {
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
			want: `retire the "M3: Agents" option`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := removableAPI()
			tt.fail(api)
			env, out := testEnv(testConfig(), api)
			nudges := nudgeCounter(&env)

			err := Run(context.Background(),
				[]string{"milestone-remove", "M3: Agents", "--project", "project-1"}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if *nudges != 0 {
				t.Errorf("nudges = %d, want none: the removal did not land", *nudges)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneRemoveNeedsAConfiguredProject(t *testing.T) {
	api := removableAPI()
	env, _ := testEnv(testConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"milestone-remove", "M3: Agents", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want the setup reported", err)
	}
	noWritesBut(t, api, 0)
}

func TestMilestoneRemoveReportsAFailedWrite(t *testing.T) {
	for _, extra := range [][]string{nil, {"--json"}} {
		t.Run(strings.Join(append([]string{"milestone-remove"}, extra...), " "), func(t *testing.T) {
			env, _ := testEnv(testConfig(), removableAPI())
			env.Out = failingWriter{}

			args := append([]string{"milestone-remove", "M3: Agents"}, extra...)
			err := Run(context.Background(), append(args, "--project", "project-1"), env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}
