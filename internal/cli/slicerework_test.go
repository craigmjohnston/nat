package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

func TestSliceReworkClearsTheBranchAndNothingElse(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "slice/write-the-ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	var out strings.Builder
	env.Out = &out
	var nudges int
	env.Nudge = func() { nudges++ }

	if err := Run(context.Background(), []string{"slice-rework", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-rework: %v", err)
	}
	if len(api.updates) != 1 || api.updates[0].id != testSliceID || len(api.updates[0].props) != 1 {
		t.Fatalf("updates = %+v, want exactly the branch written to the slice", api.updates)
	}
	if _, ok := api.updates[0].props[notion.PropBranch]; !ok {
		t.Errorf("props = %+v, want the Branch property", api.updates[0].props)
	}
	if nudges != 1 {
		t.Errorf("nudged %d times, want once", nudges)
	}
	if !strings.Contains(out.String(), "Sent back for rework") {
		t.Errorf("output = %q, want the confirmation", out.String())
	}
}

func TestSliceReworkRefusesWhatIsNotHandedBack(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}

	err := Run(context.Background(), []string{"slice-rework", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "not handed back") {
		t.Fatalf("error = %v, want 'not handed back'", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

func TestSliceReworkMisuseAndFailures(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "b")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}

	for name, args := range map[string][]string{
		"no slice":     {"slice-rework", "--project", "project-1"},
		"not a slice":  {"slice-rework", "the board", "--project", "project-1"},
		"bad flag":     {"slice-rework", testSliceID, "--nope", "--project", "project-1"},
		"no project":   {"slice-rework", testSliceID},
		"unknown proj": {"slice-rework", testSliceID, "--project", "nope"},
	} {
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSliceReworkReadFailures(t *testing.T) {
	// A slice the plan does not hold, and a workspace whose schema cannot be
	// read: each stops the command before anything is written.
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {}}}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}
	if err := Run(context.Background(), []string{"slice-rework", testSliceID, "--project", "project-1"}, env); err == nil {
		t.Error("an unknown slice: want an error")
	}

	broken := &fakeAPI{dataSourceErr: errors.New("boom")}
	env, _ = testEnv(testClaimConfig(t), broken)
	env.Out = &strings.Builder{}
	if err := Run(context.Background(), []string{"slice-rework", testSliceID, "--project", "project-1"}, env); err == nil {
		t.Error("an unreadable schema: want an error")
	}
}
