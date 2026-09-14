package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakeGHRunner is a fake gh runner for testing.
type fakeGHRunner struct {
	out string
	err error
	dir string
}

func (f *fakeGHRunner) Run(dir, name string, args ...string) (string, error) {
	f.dir = dir
	return f.out, f.err
}

func TestSliceApproveRefusesNotHandedBack(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(&fakeGHRunner{}) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-approve: expected error for non-handed-back slice")
	}
	if !strings.Contains(err.Error(), "not handed back") {
		t.Errorf("slice-approve error: %v, want 'not handed back'", err)
	}
}

func TestSliceApproveRefusesDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceDone, "m1", "main")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "already Done") {
		t.Errorf("slice-approve error = %v, want 'already Done'", err)
	}
}

func TestSliceApproveOpensAndRecordsPR(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
		blocksByID: map[string][]notion.Block{
			testSliceID: {}, // Empty blocks for PR description lookup
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{out: "https://github.test/craig/nat/pull/42\n"})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Errorf("slice-approve: unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "https://github.test/craig/nat/pull/42") {
		t.Errorf("slice-approve output: %q, want URL", out.String())
	}
	if len(api.updates) != 1 || api.updates[0].id != testSliceID {
		t.Fatalf("updates = %+v, want the pull request recorded on the slice", api.updates)
	}
	if got := api.updates[0].props[notion.PropPR].URL; got != "https://github.test/craig/nat/pull/42" {
		t.Errorf("PR = %q, want the opened pull request", got)
	}
	// The slice stays in progress: Done means the work is on main, and the
	// merge is what writes it.
	if _, wrote := api.updates[0].props[notion.PropStatus]; wrote {
		t.Errorf("props = %+v, want the status left alone at approve", api.updates[0].props)
	}
}

func TestSliceApproveGHFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
		blocksByID: map[string][]notion.Block{
			testSliceID: {},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{
			err: &gh.ExitError{Code: 1, Stderr: "already exists"},
		})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("slice-approve error = %v, want gh's own reason", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing recorded when gh refused", api.updates)
	}
}

// A record that cannot be pushed to the workspace no longer fails the
// command: the pull request lands in the local plan first, and that write is
// what the command answers for. The push failing is real, but it is not this
// run's to report — the slice is left dirty for a later sync to send instead
// of losing the record of a pull request that really did open on GitHub.
func TestSliceApproveRecordsLocallyEvenWhenThePushFails(t *testing.T) {
	cfg := testClaimConfig(t)
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
		blocksByID: map[string][]notion.Block{
			testSliceID: {},
		},
		updateErr: errors.New("notion is down"),
	}
	env, _ := testEnv(cfg, api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{out: "https://github.test/craig/nat/pull/42\n"})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-approve: %v, want it to succeed: the record landed in the plan file "+
			"even though the push to the workspace failed", err)
	}
	if !strings.Contains(out.String(), "https://github.test/craig/nat/pull/42") {
		t.Errorf("output = %q, want the pull request URL reported", out.String())
	}

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	local, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() {
		if err := local.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	}()
	dirty, err := local.Dirty(context.Background(), testSliceID)
	if err != nil {
		t.Fatalf("read whether the slice is dirty: %v", err)
	}
	if !dirty {
		t.Error("slice not marked dirty, want the failed push left for a later sync to send")
	}
}

func TestSliceApproveRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-approve: expected error for missing slice")
	}
	if !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("slice-approve error: %v, want 'want exactly one'", err)
	}
}

func TestSliceApproveRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceApproveRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-approve", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceApproveRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

// A plan file never pulled from the workspace is hydrated on the way to
// reading anything at all, and a workspace that will not answer that pull
// fails the whole command before the slice itself is ever read.
func TestSliceApproveReportsAFailedHydrate(t *testing.T) {
	boom := errors.New("notion is down")
	api := &fakeAPI{dataSourceErr: boom}
	env, _ := testEnv(testClaimConfig(t), api)

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--project", "project-1"}, env)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

// Recording the pull request is a write to the local plan first, inside one
// transaction with marking the slice sent for a later sync — and a plan
// whose sync bookkeeping cannot be written fails the command outright, since
// nothing was actually recorded for the push to send at all.
func TestSliceApproveReportsAFailedLocalRecord(t *testing.T) {
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Write the UI", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET branch = 'main' WHERE id = ?`, testSliceID); err != nil {
			t.Fatalf("seed the branch: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{out: "https://github.test/craig/nat/pull/42\n"})
	}

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Error("slice-approve over a plan that cannot record the slice sent: want an error")
	}
}

func TestSliceApproveReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testClaimConfig(t), api)

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestSliceApproveJSON(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
		blocksByID: map[string][]notion.Block{
			testSliceID: {},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{out: "https://github.test/craig/nat/pull/42\n"})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-approve: unexpected error: %v", err)
	}

	var got approveJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if want := (approveJSON{URL: "https://github.test/craig/nat/pull/42"}); got != want {
		t.Errorf("json = %+v, want %+v", got, want)
	}
}
