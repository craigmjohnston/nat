package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/notion"
)

// cancelEnv is the command's Env over api with tmux standing in as runner and
// the worktrees answered by the fake it returns.
func cancelEnv(t *testing.T, api *fakeAPI, runner *agentTestRunner) (Env, *strings.Builder, *fakeSessionWorktrees) {
	t.Helper()
	env, _ := testEnv(testClaimConfig(t), api)
	var out strings.Builder
	env.Out = &out
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	w := &fakeSessionWorktrees{}
	env.NewWorktrees = func() actions.Worktrees { return w }
	return env, &out, w
}

// The whole command on a slice in progress with its agent live: the session
// killed, the line and one properties write — Todo, unassigned, pull request
// cleared — then the worktree and branch discarded, and a nudge.
func TestSliceCancelStopsTheAgentAndDiscardsTheWork(t *testing.T) {
	api := releasableAPI()
	runner := &agentTestRunner{liveSessions: map[string]string{sliceID: "nat-c020efb4"}}
	env, out, w := cancelEnv(t, api, runner)
	var nudges int
	env.Nudge = func() { nudges++ }

	if err := Run(context.Background(), []string{"slice-cancel", sliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-cancel: %v", err)
	}

	if !reflect.DeepEqual(runner.kills, []string{"nat-c020efb4"}) {
		t.Errorf("kills = %v, want the slice's session", runner.kills)
	}
	if len(api.appends) != 1 {
		t.Fatalf("appends = %+v, want the cancel's line", api.appends)
	}
	if got := blockTexts(t, api.appends[0].children); len(got) != 1 ||
		!strings.HasPrefix(got[0], "paragraph: Cancelled by Craig Johnston at ") {
		t.Errorf("blocks = %v, want the cancel's line", got)
	}
	props := api.updates[len(api.updates)-1].props
	if name := props[notion.PropStatus].SelectName(); name != notion.SliceTodo {
		t.Errorf("status = %q, want Todo", name)
	}
	if ids := props[notion.PropAssignee].PeopleIDs(); len(ids) != 0 {
		t.Errorf("assignee = %v, want it cleared", ids)
	}
	if pr, _ := json.Marshal(props[notion.PropPR]); string(pr) != `{"url":null}` {
		t.Errorf("PR = %s, want it cleared", pr)
	}
	if want := []removal{{"/tmp/nat", "slice/render-the-board"}}; !reflect.DeepEqual(w.discarded, want) {
		t.Errorf("discarded = %+v, want %+v", w.discarded, want)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1", nudges)
	}
	if !strings.Contains(out.String(), "Cancelled.") || !strings.Contains(out.String(), "left open on GitHub") {
		t.Errorf("output = %q, want what the cancel did", out.String())
	}
}

// --json says what happened; no live agent is nothing to stop, and a discard
// git refuses is logged and leaves the command's answer as it was.
func TestSliceCancelJSONWithNoAgent(t *testing.T) {
	api := releasableAPI()
	runner := &agentTestRunner{liveSessions: map[string]string{}}
	env, out, w := cancelEnv(t, api, runner)
	w.discardErr = refusedRemoval

	if err := Run(context.Background(), []string{"slice-cancel", sliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-cancel --json: %v", err)
	}
	if len(runner.kills) != 0 {
		t.Errorf("kills = %v, want none", runner.kills)
	}
	var got sliceCancelledJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got != (sliceCancelledJSON{ID: sliceID, Name: "Render the board", Cancelled: true}) {
		t.Errorf("json = %+v", got)
	}
}

// Every refusal lands before anything is killed or written.
func TestSliceCancelRefusals(t *testing.T) {
	tests := []struct {
		name   string
		status string
		runner *agentTestRunner
		args   []string
		want   string
	}{
		{"todo", notion.SliceTodo, &agentTestRunner{}, nil, "is Todo"},
		{"done", notion.SliceDone, &agentTestRunner{}, nil, "is Done"},
		{"unreadable tmux", notion.SliceInProgress, &agentTestRunner{liveFatalErr: "tmux is broken"}, nil,
			"could not read live sessions"},
		{"failed kill", notion.SliceInProgress,
			&agentTestRunner{liveSessions: map[string]string{sliceID: "nat-c020efb4"}, killErr: "permission denied"}, nil,
			"stop its agent"},
		{"no slice", notion.SliceInProgress, &agentTestRunner{}, []string{"slice-cancel", "--project", "project-1"},
			"want exactly one"},
		{"not a slice", notion.SliceInProgress, &agentTestRunner{}, []string{"slice-cancel", "nope", "--project", "project-1"},
			"not a slice"},
		{"unknown project", notion.SliceInProgress, &agentTestRunner{}, []string{"slice-cancel", sliceID, "--project", "nope"},
			"no project nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := releasableAPI()
			api.pages["slices-ds"] = []notion.Page{heldSlice(sliceID, "Render the board", tt.status, "u1", "Craig Johnston")}
			env, _, w := cancelEnv(t, api, tt.runner)
			args := tt.args
			if args == nil {
				args = []string{"slice-cancel", sliceID, "--project", "project-1"}
			}
			err := Run(context.Background(), args, env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if len(api.appends)+len(api.updates)+len(w.discarded)+len(tt.runner.kills) != 0 {
				t.Errorf("wrote %+v / %+v, discarded %+v, killed %v — want nothing",
					api.appends, api.updates, w.discarded, tt.runner.kills)
			}
		})
	}
}

func TestSliceCancelRefusesAnUnknownFlag(t *testing.T) {
	env, _, _ := cancelEnv(t, releasableAPI(), &agentTestRunner{})
	err := Run(context.Background(), []string{"slice-cancel", sliceID, "--bogus", "--project", "project-1"}, env)
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

// The line names who cancelled it, so a config with nobody to name refuses
// before the plan is opened.
func TestSliceCancelRefusesWithNoAssignee(t *testing.T) {
	env, _ := testEnv(testConfig(t), releasableAPI())
	err := Run(context.Background(), []string{"slice-cancel", sliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no assignee") {
		t.Errorf("err = %v, want the missing assignee named", err)
	}
}

// A plan that cannot be opened is the command's failure.
func TestSliceCancelReportsAStoreThatWillNotOpen(t *testing.T) {
	api := releasableAPI()
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}
	env, _, _ := cancelEnv(t, api, &agentTestRunner{})
	if err := Run(context.Background(), []string{"slice-cancel", sliceID, "--project", "project-1"}, env); err == nil {
		t.Error("slice-cancel over a plan that cannot be read: want an error")
	}
}

// A write that fails is the command's failure, and nothing is discarded.
func TestSliceCancelReportsAFailedWrite(t *testing.T) {
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", sliceID, "Render the board", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	w := &fakeSessionWorktrees{}
	env.NewWorktrees = func() actions.Worktrees { return w }
	if err := Run(context.Background(), []string{"slice-cancel", sliceID, "--project", "project-1"}, env); err == nil {
		t.Fatal("slice-cancel over a plan that cannot write: want an error")
	}
	if len(w.discarded) != 0 {
		t.Errorf("discarded = %+v, want nothing after a failed write", w.discarded)
	}
}

// A hand-back before a cancel went with the work: a slice relaunched since
// is not read as taken back, and one handed back since is.
func TestHoldsHandBackReadsOnlySinceTheLastCancel(t *testing.T) {
	handBack := "### Handed back\n\nDid it.\n\n"
	cancel := "Cancelled by Craig at 2026-10-03T23:14:05+01:00: the work so far was discarded and it is back at Todo.\n\n"
	if holdsHandBack(handBack + cancel) {
		t.Error("a hand-back before a cancel read as one")
	}
	if !holdsHandBack(handBack + cancel + handBack) {
		t.Error("a hand-back since the cancel did not read as one")
	}
}
