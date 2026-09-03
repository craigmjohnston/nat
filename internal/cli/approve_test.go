package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
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
	env, _ := testEnv(testClaimConfig(), api)
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
	env, _ := testEnv(testClaimConfig(), api)
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
	env, _ := testEnv(testClaimConfig(), api)
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
		t.Fatalf("updates = %+v, want the slice marked Done", api.updates)
	}
	if got := api.updates[0].props[notion.PropStatus]; got.Select == nil || got.Select.Name != notion.SliceDone {
		t.Errorf("status = %+v, want Done", got)
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
	env, _ := testEnv(testClaimConfig(), api)
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

func TestSliceApproveReportsAFailedRecord(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "main")},
		},
		blocksByID: map[string][]notion.Block{
			testSliceID: {},
		},
		updateErr: errors.New("notion is down"),
	}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{out: "https://github.test/craig/nat/pull/42\n"})
	}
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-approve", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), `record the pull request for "Write the UI"`) {
		t.Errorf("slice-approve error = %v, want the write's failure named", err)
	}
}

func TestSliceApproveRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
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
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceApproveRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-approve", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceApproveRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-approve", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceApproveReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testClaimConfig(), api)

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
	env, _ := testEnv(testClaimConfig(), api)
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
