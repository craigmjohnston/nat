package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// nudgeCounter wires a counting Nudge into an Env, standing in for the marker
// touch a real command makes.
func nudgeCounter(env *Env) *int {
	count := 0
	env.Nudge = func() { count++ }
	return &count
}

// Every mutating command tells the board once when its write lands, and a
// command refused before it writes tells it nothing.
func TestMutatingCommandsNudgeTheBoardOnce(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  func(t *testing.T) Env
	}{
		{
			name: "next-slice",
			args: []string{"next-slice", "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testClaimConfig(t), claimableAPI(t))
				return env
			},
		},
		{
			name: "start-slice",
			args: []string{"start-slice", startSliceID, "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testClaimConfig(t), startableAPI(t))
				return env
			},
		},
		{
			name: "complete-slice",
			args: []string{"complete-slice", sliceID, "--summary", "Rendered the board.", "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := completeEnv(t, completableAPI())
				return env
			},
		},
		{
			name: "milestone-add",
			args: []string{"milestone-add", "M4: Polish", "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(t), plannedAPI(addedMilestoneID))
				return env
			},
		},
		{
			name: "milestone-rename",
			args: []string{"milestone-rename", "M2: Board", "M2: The board", "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(t), renamableAPI())
				return env
			},
		},
		{
			name: "slice-add",
			args: []string{"slice-add", "Frame the board", "--milestone", "M2: Board", "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(t), plannedAPI(addedSliceID))
				return env
			},
		},
		{
			name: "plan-apply",
			args: []string{"plan-apply", "--project", "project-1"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(t), planAPI(3))
				env.In = strings.NewReader(samplePlan)
				return env
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.env(t)
			nudges := nudgeCounter(&env)

			if err := Run(context.Background(), tc.args, env); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if *nudges != 1 {
				t.Errorf("nudges = %d, want exactly 1: one write landed", *nudges)
			}
		})
	}
}

// A command that never gets to write leaves the marker alone: there is nothing
// for a board to refetch.
func TestARefusedCommandDoesNotNudge(t *testing.T) {
	api := startableAPI(t)
	env, _ := testEnv(testClaimConfig(t), api)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"milestone-add", "M1: Client", "--project", "project-1"}, env)

	if err == nil {
		t.Fatal("milestone-add should refuse a name the plan already holds")
	}
	if *nudges != 0 {
		t.Errorf("nudges = %d, want none: nothing was written", *nudges)
	}
}

// The nudge follows the write, not the command: a claim that lands is on the
// board's plan whether or not the brief can still be freshly read afterward.
// store.Mirrored.Body no longer fails a read the workspace cannot answer — it
// falls back to the file's own stale copy, empty for a slice just taken into
// a plan that has never fetched its body — so a claim like this one no longer
// fails the command at all: it succeeds, with an empty brief, and still
// nudges exactly once for the write that landed.
func TestAClaimThatLandsNudgesThoughTheBriefFails(t *testing.T) {
	api := startableAPI(t)
	api.blocksErrByID = map[string]error{startSliceID: errors.New("boom")}
	env, _ := testEnv(testClaimConfig(t), api)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"start-slice", startSliceID, "--project", "project-1"}, env)

	if err != nil {
		t.Fatalf("start-slice: %v", err)
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want 1: the claim was written before the brief was read", *nudges)
	}
}

// A plan that fails partway has still created its first half — the error says
// so — and the board deserves to hear about that half too.
func TestAHalfAppliedPlanStillNudges(t *testing.T) {
	api := planAPI(3)
	api.createErr = errors.New("boom")
	api.failCreateAfter = 1
	env, _ := testEnv(testConfig(t), api)
	env.In = strings.NewReader(samplePlan)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"plan-apply", "--project", "project-1"}, env)

	if err == nil {
		t.Fatal("plan-apply should report the create that failed")
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want 1: the first half of the plan was written", *nudges)
	}
}

// A plan that fails before anything is created nudges nobody.
func TestAPlanThatWroteNothingDoesNotNudge(t *testing.T) {
	api := planAPI(3)
	api.schemaUpdateErr = errors.New("boom")
	env, _ := testEnv(testConfig(t), api)
	env.In = strings.NewReader(samplePlan)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"plan-apply", "--project", "project-1"}, env)

	if err == nil {
		t.Fatal("plan-apply should report the schema write that failed")
	}
	if *nudges != 0 {
		t.Errorf("nudges = %d, want none: nothing was written", *nudges)
	}
}
