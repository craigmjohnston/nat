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

// A move is one schema write and nothing else: the options go back in the new
// order, and no page is touched, since no slice has to be refiled by it.
func TestMilestoneMoveReordersThePlan(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		options []string
		want    string
	}{
		{
			name:    "before another milestone",
			args:    []string{"M3: Agents", "--before", "M1: Client"},
			options: []string{"M3: Agents", "M1: Client", "M2: Board"},
			want: `# M3: Agents

Moved in nat to milestone 1, directly before M1: Client.
`,
		},
		{
			name:    "after another milestone",
			args:    []string{"  m3: agents  ", "--after", "  m1: client  "},
			options: []string{"M1: Client", "M3: Agents", "M2: Board"},
			want: `# M3: Agents

Moved in nat to milestone 2, directly after M1: Client.
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
			env, out := testEnv(testConfig(), api)
			nudges := nudgeCounter(&env)

			args := append(append([]string{"milestone-move"}, tt.args...), "--project", "project-1")
			if err := Run(context.Background(), args, env); err != nil {
				t.Fatalf("milestone-move: %v", err)
			}

			if want := tt.want + "\n- " + reorderNote + "\n"; out.String() != want {
				t.Errorf("output =\n%s\nwant:\n%s", out.String(), want)
			}
			if len(api.schemaUpdates) != 1 {
				t.Fatalf("schema writes = %+v, want exactly one", api.schemaUpdates)
			}
			if got := api.schemaUpdates[0].props[notion.PropMilestone].OptionNames(); !reflect.DeepEqual(got, tt.options) {
				t.Errorf("options written = %v, want %v", got, tt.options)
			}
			if len(api.updates) != 0 || len(api.creates) != 0 {
				t.Errorf("page writes = %+v %+v, want none: only the order changed", api.updates, api.creates)
			}
			if *nudges != 1 {
				t.Errorf("nudges = %d, want one: the move landed", *nudges)
			}
		})
	}
}

func TestMilestoneMovePrintsJSON(t *testing.T) {
	env, out := testEnv(testConfig(), plannedAPI(addedMilestoneID))

	if err := Run(context.Background(), []string{
		"milestone-move", "M1: Client", "--after", "M3: Agents", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("milestone-move: %v", err)
	}

	var got milestoneMovedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	// The milestone it was placed beside has a new place too, since everything
	// from the lower of the two onwards has shifted.
	want := milestoneMovedJSON{
		Milestone:  movedMilestoneJSON{ID: "M1: Client", Name: "M1: Client", Order: 2},
		Placement:  "after",
		RelativeTo: movedMilestoneJSON{ID: "M3: Agents", Name: "M3: Agents", Order: 1},
	}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

// Every refusal happens before the write, and each names what it refused over.
func TestMilestoneMoveRefusals(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "a name the plan does not hold",
			args: []string{"milestone-move", "M9: Nothing", "--before", "M1: Client"},
			want: []string{`no milestone named "M9: Nothing"`, `"M1: Client"`},
		},
		{
			name: "a target the plan does not hold",
			args: []string{"milestone-move", "M1: Client", "--after", "M9: Nothing"},
			want: []string{`no milestone named "M9: Nothing"`, `"M3: Agents"`},
		},
		{
			name: "a move relative to itself",
			args: []string{"milestone-move", "M2: Board", "--before", "  m2: board  "},
			want: []string{
				`"M2: Board" cannot be moved relative to itself`,
				"name the milestone it is to sit beside",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
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

// A plan with no milestones at all says so rather than listing nothing.
func TestMilestoneMoveNamesTheMilestonesThePlanHas(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	api.dataSources["slices-ds"] = selectMilestoneSlicesDS()
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(),
		[]string{"milestone-move", "M1: Client", "--before", "M2: Board", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "its milestones are none") {
		t.Fatalf("err = %v, want the empty plan said out loud", err)
	}
}

func TestMilestoneMoveRejectsAMisusedCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no name", args: []string{"milestone-move", "--before", "M1: Client"}, want: "given 0"},
		{
			name: "two names",
			args: []string{"milestone-move", "M2: Board", "M3: Agents", "--before", "M1: Client"},
			want: "given 2",
		},
		{name: "a blank name", args: []string{"milestone-move", "   ", "--before", "M1: Client"}, want: "name is empty"},
		{
			name: "no destination",
			args: []string{"milestone-move", "M3: Agents"},
			want: "no destination given: pass --before or --after",
		},
		{
			name: "a blank destination",
			args: []string{"milestone-move", "M3: Agents", "--after", "   "},
			want: "no destination given: pass --before or --after",
		},
		{
			name: "both destinations",
			args: []string{"milestone-move", "M3: Agents", "--before", "M1: Client", "--after", "M2: Board"},
			want: "--before and --after name two places at once",
		},
		{
			name: "unknown flag",
			args: []string{"milestone-move", "M3: Agents", "--before", "M1: Client", "--nope"},
			want: "not defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
			env, out := testEnv(testConfig(), api)

			err := Run(context.Background(), append(tt.args, "--project", "project-1"), env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "milestone-move:") {
				t.Errorf("err = %q, want it to name the command", err)
			}
			noWritesBut(t, api, 0)
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneMoveReportsAFailedCall(t *testing.T) {
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
			name: "the schema write",
			fail: func(api *fakeAPI) { api.schemaUpdateErr = boom },
			want: "reorder the plan",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := plannedAPI(addedMilestoneID)
			tt.fail(api)
			env, out := testEnv(testConfig(), api)
			nudges := nudgeCounter(&env)

			err := Run(context.Background(),
				[]string{"milestone-move", "M3: Agents", "--before", "M1: Client", "--project", "project-1"}, env)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
			if *nudges != 0 {
				t.Errorf("nudges = %d, want none: the move did not land", *nudges)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want nothing", out.String())
			}
		})
	}
}

func TestMilestoneMoveNeedsAConfiguredProject(t *testing.T) {
	api := plannedAPI(addedMilestoneID)
	env, _ := testEnv(testConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(),
		[]string{"milestone-move", "M3: Agents", "--before", "M1: Client", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Fatalf("err = %v, want the setup reported", err)
	}
	noWritesBut(t, api, 0)
}

func TestMilestoneMoveReportsAFailedWrite(t *testing.T) {
	for _, extra := range [][]string{nil, {"--json"}} {
		t.Run(strings.Join(append([]string{"milestone-move"}, extra...), " "), func(t *testing.T) {
			env, _ := testEnv(testConfig(), plannedAPI(addedMilestoneID))
			env.Out = failingWriter{}

			args := append([]string{"milestone-move", "M3: Agents", "--before", "M1: Client"}, extra...)
			err := Run(context.Background(), append(args, "--project", "project-1"), env)

			if !errors.Is(err, errWrite) {
				t.Errorf("err = %v, want %v", err, errWrite)
			}
		})
	}
}
