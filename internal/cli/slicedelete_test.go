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

// deletableAPI answers with one slice in the given status, for slice-delete to
// trash — or refuse.
func deletableAPI(status string) *fakeAPI {
	return &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Render the board", status, "m1", "", "")},
		},
	}
}

func TestSliceDeleteTrashesTheSlice(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-delete: %v", err)
	}

	if !equalLines(api.trashes, []string{testSliceID}) {
		t.Errorf("trashes = %v, want the slice alone", api.trashes)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1", nudges)
	}
	if !strings.Contains(out.String(), "Notion's trash") {
		t.Errorf("output missing where the page went:\n%s", out.String())
	}
}

// A Done slice is allowed through: warning about dropping the record of
// finished work is the caller's confirm, and the page is still recoverable.
func TestSliceDeleteAllowsDone(t *testing.T) {
	api := deletableAPI(notion.SliceDone)
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-delete: %v", err)
	}
	if !equalLines(api.trashes, []string{testSliceID}) {
		t.Errorf("trashes = %v, want the slice trashed", api.trashes)
	}
}

func TestSliceDeleteJSON(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	env, out := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-delete --json: %v", err)
	}
	var got sliceDeletedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.ID != testSliceID || !got.Deleted {
		t.Errorf("json = %+v", got)
	}
}

// deleteEnvWithTmux is testEnv with tmux answered by runner and the worktrees
// by the fake it returns.
func deleteEnvWithTmux(t *testing.T, api *fakeAPI, runner *agentTestRunner) (Env, *fakeSessionWorktrees) {
	t.Helper()
	env, _ := testEnv(testConfig(t), api)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	w := &fakeSessionWorktrees{}
	env.NewWorktrees = func() actions.Worktrees { return w }
	return env, w
}

// A slice in progress is deleted: its live agent killed first, then the
// trash, then its worktree and branch discarded with the work in them.
func TestSliceDeleteStopsAndDiscardsASliceInProgress(t *testing.T) {
	for name, live := range map[string]map[string]string{
		"with its agent live": {testSliceID: "nat-c020efb4"},
		"with no agent":       {},
	} {
		t.Run(name, func(t *testing.T) {
			api := deletableAPI(notion.SliceInProgress)
			runner := &agentTestRunner{liveSessions: live}
			env, w := deleteEnvWithTmux(t, api, runner)

			if err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "project-1"}, env); err != nil {
				t.Fatalf("slice-delete: %v", err)
			}
			wantKills := []string(nil)
			if len(live) > 0 {
				wantKills = []string{"nat-c020efb4"}
			}
			if !reflect.DeepEqual(runner.kills, wantKills) {
				t.Errorf("kills = %v, want %v", runner.kills, wantKills)
			}
			if !equalLines(api.trashes, []string{testSliceID}) {
				t.Errorf("trashes = %v, want the slice trashed", api.trashes)
			}
			if want := []removal{{"/tmp/nat", "slice/render-the-board"}}; !reflect.DeepEqual(w.discarded, want) || len(w.removed) != 0 {
				t.Errorf("discarded = %+v, removed = %+v, want the one forced discard %+v", w.discarded, w.removed, want)
			}
		})
	}
}

// An agent that cannot be stopped — tmux unreadable, or a kill that fails —
// refuses the delete before the page is touched.
func TestSliceDeleteRefusesWhereTheAgentCannotBeStopped(t *testing.T) {
	for name, runner := range map[string]*agentTestRunner{
		"unreadable tmux": {liveFatalErr: "tmux is broken"},
		"failed kill":     {liveSessions: map[string]string{testSliceID: "nat-c020efb4"}, killErr: "permission denied"},
	} {
		t.Run(name, func(t *testing.T) {
			api := deletableAPI(notion.SliceInProgress)
			env, w := deleteEnvWithTmux(t, api, runner)

			err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "project-1"}, env)
			if err == nil {
				t.Fatal("slice-delete: want the agent's refusal")
			}
			if len(api.trashes)+len(w.discarded) != 0 {
				t.Errorf("trashed %v, discarded %+v, want nothing", api.trashes, w.discarded)
			}
		})
	}
}

func TestSliceDeleteRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestSliceDeleteRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--bogus", "--project", "project-1",
	}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceDeleteRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceDeleteRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

// A slice already hydrated into the local plan (which every project's first
// command does, and this test's fixture is part of) is read from the file,
// not the workspace — so a read that fails now is the plan's own first pull,
// not a page fetch by ID. That is what api.getErr used to stand in for and no
// longer can; api.queryErr fails the hydrate's own slices query instead.
func TestSliceDeleteReportsAFailedRead(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Errorf("err = %v, want the failed read named", err)
	}
	if len(api.trashes) != 0 {
		t.Errorf("failed read still trashed: %v", api.trashes)
	}
}

// A slice named that the file has never met, and that the workspace cannot
// answer for either, fails the load — a different guard from the one a
// failed hydrate trips, since the plan itself was read just fine.
func TestSliceDeleteReportsAFailedLoad(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", "other-slice", "Somebody else", "Todo", nil)
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(cfg, api)

	err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("slice-delete over a slice neither the file nor the workspace has: want an error")
	}
}

// The delete itself is a local write before anything is asked of the
// workspace, and a plan that cannot make that write fails the command
// outright — there is nothing to push if nothing was actually deleted.
func TestSliceDeleteReportsAFailedLocalDelete(t *testing.T) {
	cfg := testConfig(t)
	// sync, not slice_deps: the slice is read (via loadSlice) before it is
	// deleted, and that read joins against slice_deps too — dropping it would
	// fail the read this test means to get past, not the delete itself.
	seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("slice-delete over a plan that cannot delete the slice: want an error")
	}
}

// store.Mirrored.DeleteSlice drops the slice from the local file first and
// only then asks the workspace to do the same — and, unlike every other
// write, a failed push here is not something a later sync can retry (the row
// the dirty flag would have lived on is already gone), so it is only logged,
// never returned. The command succeeds, and still nudges: the file changed.
func TestSliceDeleteSucceedsThoughTheWorkspaceTrashFails(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	api.trashErr = errors.New("notion refused")
	env, _ := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	if err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("slice-delete: %v", err)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1: the file's own delete landed", nudges)
	}
	if len(api.trashes) != 1 {
		t.Errorf("trashes attempted = %+v, want one attempt even though it failed", api.trashes)
	}
}
