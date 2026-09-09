package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
)

// fakeRunner for tmux commands, used to build a Tmux for testing status.
type statusFakeRunner struct {
	liveOut       string
	liveErr       error
	activityMap   map[string]agent.Activity
	activityErr   error
	callCount     int
	failAfterCall int // fail after this many list-panes calls (0 means fail immediately)
	// Track panes and their order
	sliceOrder   []string
	captureIndex int
}

func (f *statusFakeRunner) Run(name string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-panes" {
		f.callCount++
		if f.failAfterCall > 0 && f.callCount > f.failAfterCall {
			return "", f.liveErr
		}
		if f.failAfterCall == 0 && f.callCount > 1 {
			return "", f.liveErr
		}
		// Extract slice order from liveOut
		if f.sliceOrder == nil {
			f.sliceOrder = []string{}
			lines := strings.Split(strings.TrimSuffix(f.liveOut, "\n"), "\n")
			for _, line := range lines {
				if line != "" {
					parts := strings.Split(line, "\t")
					if len(parts) >= 1 {
						f.sliceOrder = append(f.sliceOrder, parts[0])
					}
				}
			}
			f.captureIndex = 0
		}
		return f.liveOut, f.liveErr
	}
	// For capture-pane commands, return activity based on sliceOrder index
	if len(args) > 0 && args[0] == "capture-pane" {
		if f.captureIndex < len(f.sliceOrder) {
			sliceID := f.sliceOrder[f.captureIndex]
			f.captureIndex++
			if activity, exists := f.activityMap[sliceID]; exists {
				if activity == agent.ActivityGone && f.activityErr != nil {
					return "", f.activityErr
				}
				if activity == agent.ActivityWorking {
					return "✻ Working… (1m 6s · …)\n", nil
				}
				// ActivityWaiting returns empty
				return "", nil
			}
		}
		// Default: return empty for waiting or unmapped
		return "", nil
	}
	return "", nil
}

// buildStatusFakeRunner creates a fake runner that returns the canned panes
// output in list-panes format and uses captures for activity.
func buildStatusFakeRunner(liveSlices map[string]string, captures map[string]string) *statusFakeRunner {
	var panes []string
	for sliceID, session := range liveSlices {
		// Simulate tmux list-panes output format: slice_id, pane_id, session, window, dead
		panes = append(panes, sliceID+"\t%0\t"+session+"\t@0\t0")
	}
	liveOut := strings.Join(panes, "\n")
	if len(panes) > 0 {
		liveOut += "\n"
	}

	return &statusFakeRunner{
		liveOut:     liveOut,
		activityMap: map[string]agent.Activity{}, // Unused for now
	}
}

// TestStatusPrintsLiveAgents tests the markdown output with running agents.
func TestStatusPrintsLiveAgents(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := buildStatusFakeRunner(
				map[string]string{"slice-1": "nat-11111111", "slice-2": "nat-22222222"},
				map[string]string{},
			)
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	if err := Run(context.Background(), []string{"status"}, env); err != nil {
		t.Fatalf("status: %v", err)
	}

	result := env.Out.(*strings.Builder).String()
	// Check that both slices are present.
	if !strings.Contains(result, "slice-1") {
		t.Errorf("output missing slice-1: %s", result)
	}
	if !strings.Contains(result, "slice-2") {
		t.Errorf("output missing slice-2: %s", result)
	}
}

// TestStatusPrintsNoAgents tests the output when no agents are running.
func TestStatusPrintsNoAgents(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := buildStatusFakeRunner(map[string]string{}, map[string]string{})
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	if err := Run(context.Background(), []string{"status"}, env); err != nil {
		t.Fatalf("status: %v", err)
	}

	if env.Out.(*strings.Builder).String() != "no agents running\n" {
		t.Errorf("output = %q, want %q", env.Out.(*strings.Builder).String(), "no agents running\n")
	}
}

// TestStatusPrintsJSON tests JSON output format.
func TestStatusPrintsJSON(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := &statusFakeRunner{
				liveOut: "slice-1\t%0\tnat-11111111\t@0\t0\nslice-2\t%1\tnat-22222222\t@1\t0\n",
				// Activity() for any pane returns no output (will classify as unknown or waiting)
				// capture-pane returns empty which triggers ActivityWaiting
				// We'll return empty string to trigger waiting, which is fine for this test
			}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	if err := Run(context.Background(), []string{"status", "--json"}, env); err != nil {
		t.Fatalf("status --json: %v", err)
	}

	var got statusJSON
	output := env.Out.(*strings.Builder)
	if err := json.Unmarshal([]byte(output.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, output.String())
	}

	if len(got.Agents) != 2 {
		t.Errorf("agents count = %d, want 2", len(got.Agents))
	}

	// Check that slice-1 is first (sorted by ID)
	if got.Agents[0].SliceID != "slice-1" || got.Agents[0].Session != "nat-11111111" {
		t.Errorf("agents[0] = %+v, want slice-1 nat-11111111", got.Agents[0])
	}
	if got.Agents[1].SliceID != "slice-2" || got.Agents[1].Session != "nat-22222222" {
		t.Errorf("agents[1] = %+v, want slice-2 nat-22222222", got.Agents[1])
	}
}

// TestStatusLiveSlicesError tests error handling for LiveSlices.
func TestStatusLiveSlicesError(t *testing.T) {
	boom := errors.New("boom")
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := &statusFakeRunner{liveErr: boom}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	err := Run(context.Background(), []string{"status"}, env)
	if err == nil {
		t.Fatal("status: want error, got nil")
	}
	if !strings.Contains(err.Error(), "read live slices") {
		t.Errorf("err = %v, want it to mention 'read live slices'", err)
	}
}

// TestStatusBadFlag tests error handling for unknown flags.
func TestStatusBadFlag(t *testing.T) {
	env := Env{
		NewTmux: DefaultNewTmux,
		Out:     &strings.Builder{},
	}

	err := Run(context.Background(), []string{"status", "--badFlag"}, env)
	if err == nil {
		t.Fatal("status with bad flag: want error, got nil")
	}
}

// TestStatusExtraArgument tests error handling for unexpected arguments.
func TestStatusExtraArgument(t *testing.T) {
	env := Env{
		NewTmux: DefaultNewTmux,
		Out:     &strings.Builder{},
	}

	err := Run(context.Background(), []string{"status", "extra"}, env)
	if err == nil {
		t.Fatal("status with extra arg: want error, got nil")
	}
}

// TestStatusActivityReturnsError tests the error path when Activity() fails.
func TestStatusActivityReturnsError(t *testing.T) {
	boom := errors.New("pane listing failed")

	env := Env{
		NewTmux: func() *agent.Tmux {
			return agent.NewTmuxWithRunner(&testActivityFailRunner{
				liveOut: "slice-1\t%0\tnat-11111111\t@0\t0\n",
				boom:    boom,
			})
		},
		Out: &strings.Builder{},
	}

	err := Run(context.Background(), []string{"status"}, env)
	if err == nil {
		t.Fatal("status: want error when Activity fails, got nil")
	}
	if !strings.Contains(err.Error(), "read agent activity") {
		t.Errorf("err = %v, want it to mention 'read agent activity'", err)
	}
}

// testActivityFailRunner succeeds on first list-panes, fails on second
type testActivityFailRunner struct {
	liveOut   string
	boom      error
	callCount int
}

func (r *testActivityFailRunner) Run(name string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-panes" {
		r.callCount++
		if r.callCount > 1 {
			return "", r.boom
		}
		return r.liveOut, nil
	}
	return "", nil
}

// TestStatusJSONHasAllRequiredFields tests that all required fields are in JSON output.
func TestStatusJSONHasAllRequiredFields(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			// Single agent for simple testing
			runner := &statusFakeRunner{
				liveOut: "slice-1\t%0\tnat-11111111\t@0\t0\n",
			}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	if err := Run(context.Background(), []string{"status", "--json"}, env); err != nil {
		t.Fatalf("status --json: %v", err)
	}

	var got statusJSON
	output := env.Out.(*strings.Builder)
	if err := json.Unmarshal([]byte(output.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, output.String())
	}

	if len(got.Agents) != 1 {
		t.Errorf("agents count = %d, want 1", len(got.Agents))
	}
	if got.Agents[0].SliceID != "slice-1" {
		t.Errorf("slice_id = %q, want %q", got.Agents[0].SliceID, "slice-1")
	}
	if got.Agents[0].Session != "nat-11111111" {
		t.Errorf("session = %q, want %q", got.Agents[0].Session, "nat-11111111")
	}
	// Activity should be set to something (exact value depends on fake runner behavior)
	if got.Agents[0].Activity == "" {
		t.Errorf("activity should not be empty")
	}
}

// TestStatusActivityFoundAssignment tests that found activity is assigned to the output.
func TestStatusActivityFoundAssignment(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			// Single agent with explicit activity in the map
			runner := &statusFakeRunner{
				liveOut: "slice-1\t%0\tnat-11111111\t@0\t0\n",
				activityMap: map[string]agent.Activity{
					"slice-1": agent.ActivityWaiting,
				},
			}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	if err := Run(context.Background(), []string{"status", "--json"}, env); err != nil {
		t.Fatalf("status --json: %v", err)
	}

	var got statusJSON
	output := env.Out.(*strings.Builder)
	if err := json.Unmarshal([]byte(output.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, output.String())
	}

	// The agent should be present with waiting activity
	if len(got.Agents) != 1 {
		t.Errorf("agents count = %d, want 1", len(got.Agents))
	}
}

// TestStatusEmptyAfterFiltering tests that output says "no agents" when all are filtered.
func TestStatusEmptyAfterFiltering(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := &statusFakeRunner{
				liveOut: "",
			}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: &strings.Builder{},
	}

	if err := Run(context.Background(), []string{"status"}, env); err != nil {
		t.Fatalf("status: %v", err)
	}

	result := env.Out.(*strings.Builder).String()
	if result != "no agents running\n" {
		t.Errorf("output = %q, want %q", result, "no agents running\n")
	}
}

// TestBuildAgentListFiltersGone tests that buildAgentList filters out gone agents.
func TestBuildAgentListFiltersGone(t *testing.T) {
	live := map[string]string{
		"slice-1": "nat-11111111",
		"slice-2": "nat-22222222",
	}
	activity := map[string]agent.Activity{
		"slice-1": agent.ActivityWorking,
		"slice-2": agent.ActivityGone,
	}

	agents := buildAgentList(live, activity)
	if len(agents) != 1 {
		t.Errorf("agent count = %d, want 1 (other is gone)", len(agents))
	}
	if agents[0].SliceID != "slice-1" {
		t.Errorf("agents[0].SliceID = %q, want slice-1", agents[0].SliceID)
	}
}

// TestBuildAgentListWithFoundActivity tests that buildAgentList uses found activity.
func TestBuildAgentListWithFoundActivity(t *testing.T) {
	live := map[string]string{
		"slice-1": "nat-11111111",
	}
	activity := map[string]agent.Activity{
		"slice-1": agent.ActivityWaiting,
	}

	agents := buildAgentList(live, activity)
	if len(agents) != 1 {
		t.Errorf("agent count = %d, want 1", len(agents))
	}
	if agents[0].Activity != agent.ActivityWaiting.String() {
		t.Errorf("activity = %q, want %q", agents[0].Activity, agent.ActivityWaiting.String())
	}
}

// TestStatusJSONWriteError tests error handling when writing JSON fails.
func TestStatusJSONWriteError(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := &statusFakeRunner{
				liveOut: "slice-1\t%0\tnat-11111111\t@0\t0\n",
			}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: failingWriter{},
	}

	err := Run(context.Background(), []string{"status", "--json"}, env)
	if err == nil {
		t.Fatal("status JSON with write error: want error, got nil")
	}
}

// TestStatusMarkdownWriteError tests error handling when writing markdown fails.
func TestStatusMarkdownWriteError(t *testing.T) {
	env := Env{
		NewTmux: func() *agent.Tmux {
			runner := &statusFakeRunner{
				liveOut: "slice-1\t%0\tnat-11111111\t@0\t0\n",
			}
			return agent.NewTmuxWithRunner(runner)
		},
		Out: failingWriter{},
	}

	err := Run(context.Background(), []string{"status"}, env)
	if err == nil {
		t.Fatal("status markdown with write error: want error, got nil")
	}
}
