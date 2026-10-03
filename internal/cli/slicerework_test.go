package cli

import (
	"context"
	"encoding/json"
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

// --comments records the review before clearing the branch — the Sent back
// append has to land first, or a slice already back out of review would read
// to this command's own refusal as never handed back.
func TestSliceReworkWithCommentsRecordsBeforeClearing(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "slice/write-the-ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}

	if err := Run(context.Background(), []string{"slice-rework", testSliceID, "--comments", "Rename the helper.", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-rework: %v", err)
	}
	if len(api.appends) != 1 {
		t.Fatalf("appends = %d, want the Sent back note filed", len(api.appends))
	}
	got, _ := json.Marshal(api.appends[0].children)
	if !strings.Contains(string(got), "Sent back") || !strings.Contains(string(got), "Rename the helper.") {
		t.Errorf("appended = %s, want the Sent back section with the comments", got)
	}
	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly the branch cleared", api.updates)
	}
}

// Comments read from stdin with "-", the same convention pr-comment follows.
func TestSliceReworkReadsCommentsFromStdin(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "slice/write-the-ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}
	env.In = strings.NewReader("Piped in comments.")

	if err := Run(context.Background(), []string{"slice-rework", testSliceID, "--comments", "-", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-rework: %v", err)
	}
	got, _ := json.Marshal(api.appends[0].children)
	if !strings.Contains(string(got), "Piped in comments.") {
		t.Errorf("appended = %s, want the piped comments", got)
	}
}

// Absent comments still file the Sent back heading, with nothing under it.
func TestSliceReworkWithNoCommentsStillFilesTheHeading(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "slice/write-the-ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}

	if err := Run(context.Background(), []string{"slice-rework", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-rework: %v", err)
	}
	got, _ := json.Marshal(api.appends[0].children)
	want := `[{"heading_3":{"rich_text":[{"text":{"content":"Sent back"},"type":"text"}]},"object":"block","type":"heading_3"}]`
	if string(got) != want {
		t.Errorf("appended = %s, want %s", got, want)
	}
}

// A slice not handed back refuses before the Sent back note is ever written,
// same as before it clears the branch.
func TestSliceReworkRefusesWhatIsNotHandedBackBeforeRecordingComments(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}

	err := Run(context.Background(), []string{"slice-rework", testSliceID, "--comments", "Rename the helper.", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "not handed back") {
		t.Fatalf("error = %v, want 'not handed back'", err)
	}
	if len(api.appends) != 0 {
		t.Errorf("appends = %+v, want nothing written", api.appends)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A stdin that cannot be read fails the command before anything is written
// at all.
func TestSliceReworkReportsAFailedStdinRead(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithBranch(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "slice/write-the-ui")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	env.Out = &strings.Builder{}
	env.In = failingReader{}

	err := Run(context.Background(), []string{"slice-rework", testSliceID, "--comments", "-", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "read the comments") {
		t.Fatalf("err = %v, want the stdin read reported", err)
	}
	if len(api.appends) != 0 || len(api.updates) != 0 {
		t.Errorf("appends = %+v, updates = %+v, want nothing written", api.appends, api.updates)
	}
}

// --comments - with no stdin at all (env.In nil) reads as empty rather than
// panicking.
func TestSliceReworkCommentsFromStdinWithNoStdin(t *testing.T) {
	text, err := reworkCommentsText("-", nil)
	if err != nil || text != "" {
		t.Errorf("reworkCommentsText(-, nil) = %q, %v, want empty and no error", text, err)
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
