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
			args: []string{"next-slice"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testClaimConfig(), claimableAPI(t))
				return env
			},
		},
		{
			name: "start-slice",
			args: []string{"start-slice", startSliceID},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testClaimConfig(), startableAPI(t))
				return env
			},
		},
		{
			name: "complete-slice",
			args: []string{"complete-slice", sliceID, "--summary", "Rendered the board."},
			env: func(t *testing.T) Env {
				env, _ := completeEnv(completableAPI())
				return env
			},
		},
		{
			name: "milestone-add",
			args: []string{"milestone-add", "M4: Polish"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(), plannedAPI(addedMilestoneID))
				return env
			},
		},
		{
			name: "slice-add",
			args: []string{"slice-add", "Frame the board", "--milestone", "M2: Board"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(), plannedAPI(addedSliceID))
				return env
			},
		},
		{
			name: "plan-apply",
			args: []string{"plan-apply"},
			env: func(t *testing.T) Env {
				env, _ := testEnv(testConfig(), planAPI(3))
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
	env, _ := testEnv(testClaimConfig(), api)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"milestone-add", "M1: Client"}, env)

	if err == nil {
		t.Fatal("milestone-add should refuse a name the plan already holds")
	}
	if *nudges != 0 {
		t.Errorf("nudges = %d, want none: nothing was written", *nudges)
	}
}

// The nudge follows the write, not the command: a claim that lands is on the
// board's plan even when reading the brief afterwards fails.
func TestAClaimThatLandsNudgesThoughTheBriefFails(t *testing.T) {
	api := startableAPI(t)
	api.blocksErrByID = map[string]error{startSliceID: errors.New("boom")}
	env, _ := testEnv(testClaimConfig(), api)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"start-slice", startSliceID}, env)

	if err == nil {
		t.Fatal("start-slice should report the brief it could not read")
	}
	if *nudges != 1 {
		t.Errorf("nudges = %d, want 1: the claim was written before the read failed", *nudges)
	}
}

// A plan that fails partway has still created its first half — the error says
// so — and the board deserves to hear about that half too.
func TestAHalfAppliedPlanStillNudges(t *testing.T) {
	api := planAPI(3)
	api.createErr = errors.New("boom")
	api.failCreateAfter = 1
	env, _ := testEnv(testConfig(), api)
	env.In = strings.NewReader(samplePlan)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"plan-apply"}, env)

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
	env, _ := testEnv(testConfig(), api)
	env.In = strings.NewReader(samplePlan)
	nudges := nudgeCounter(&env)

	err := Run(context.Background(), []string{"plan-apply"}, env)

	if err == nil {
		t.Fatal("plan-apply should report the schema write that failed")
	}
	if *nudges != 0 {
		t.Errorf("nudges = %d, want none: nothing was written", *nudges)
	}
}
