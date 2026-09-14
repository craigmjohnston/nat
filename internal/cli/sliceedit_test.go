package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// editableAPI answers with one Todo slice carrying an existing brief of two
// blocks, which is what slice-edit has to clear before it writes the new one.
func editableAPI() *fakeAPI {
	return &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Render the board", notion.SliceTodo, "m1", "", "")},
		},
		blocksByID: map[string][]notion.Block{
			testSliceID: {{ID: "b1"}, {ID: "b2"}},
		},
	}
}

func TestSliceEditReplacesTheBrief(t *testing.T) {
	api := editableAPI()
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "The new brief.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-edit: %v", err)
	}

	if len(api.blockReads) != 1 || api.blockReads[0] != testSliceID {
		t.Errorf("block reads = %v, want one, of the slice", api.blockReads)
	}
	if got := api.deletes; !equalLines(got, []string{"b1", "b2"}) {
		t.Errorf("deletes = %v, want the two existing blocks trashed", got)
	}
	if len(api.appends) != 1 || api.appends[0].id != testSliceID {
		t.Fatalf("appends = %+v, want exactly one, to the slice", api.appends)
	}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, []string{"paragraph: The new brief."}) {
		t.Errorf("blocks = %v, want the new brief alone", got)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1", nudges)
	}
	if !strings.Contains(out.String(), "The new brief.") {
		t.Errorf("output missing the new brief:\n%s", out.String())
	}
}

func TestSliceEditReadsTheDescriptionFromStdin(t *testing.T) {
	api := editableAPI()
	env, _ := testEnv(testConfig(t), api)
	env.In = strings.NewReader("Piped in brief.")

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "-", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-edit: %v", err)
	}
	if got := blockTexts(t, api.appends[0].children); !equalLines(got, []string{"paragraph: Piped in brief."}) {
		t.Errorf("blocks = %v, want the piped-in brief", got)
	}
}

func TestSliceEditJSON(t *testing.T) {
	api := editableAPI()
	env, out := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "The new brief.", "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-edit --json: %v", err)
	}
	var got sliceEditedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.ID != testSliceID || got.Brief != "The new brief." {
		t.Errorf("json = %+v", got)
	}
}

// TestSliceEditReportsTheSlicesOwnRepo covers a slice with a repo override:
// the reported working directory is its own, not the project default.
func TestSliceEditReportsTheSlicesOwnRepo(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithAllFields(testSliceID, "Render the board", notion.SliceTodo,
				"m1", "", "", "", "custom/repo/path")},
		},
		blocksByID: map[string][]notion.Block{testSliceID: nil},
	}
	env, out := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "New text.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-edit: %v", err)
	}
	if !strings.Contains(out.String(), "Working directory: custom/repo/path") {
		t.Errorf("output missing the slice's own repo:\n%s", out.String())
	}
}

func TestSliceEditRefusesNoDescription(t *testing.T) {
	env, _ := testEnv(testConfig(t), editableAPI())

	err := Run(context.Background(), []string{"slice-edit", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no description given") {
		t.Errorf("err = %v, want 'no description given'", err)
	}
}

func TestSliceEditRefusesInProgress(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Render the board", notion.SliceInProgress, "m1", "", "")},
		},
	}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "New text.", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("err = %v, want 'in progress'", err)
	}
	if len(api.blockReads) != 0 || len(api.deletes) != 0 || len(api.appends) != 0 {
		t.Errorf("refused edit still wrote: blockReads=%v deletes=%v appends=%v",
			api.blockReads, api.deletes, api.appends)
	}
}

func TestSliceEditRefusesDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Render the board", notion.SliceDone, "m1", "", "")},
		},
	}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "New text.", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "already Done") {
		t.Errorf("err = %v, want 'already Done'", err)
	}
}

// A push to the workspace that fails does not fail slice-edit: the brief
// already landed in the local plan file, so the command reports success and
// leaves the slice dirty for the next sync to resend.
func TestSliceEditLeavesTheSliceDirtyOnAFailedPush(t *testing.T) {
	tests := []struct {
		name string
		set  func(*fakeAPI)
	}{
		{name: "the read", set: func(a *fakeAPI) {
			a.blocksErrByID = map[string]error{testSliceID: errors.New("notion is down")}
		}},
		{name: "the clear", set: func(a *fakeAPI) { a.deleteErr = errors.New("notion refused") }},
		{name: "the write", set: func(a *fakeAPI) { a.appendErr = errors.New("notion refused") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := editableAPI()
			tt.set(api)
			env, out := testEnv(testConfig(t), api)

			err := Run(context.Background(), []string{
				"slice-edit", testSliceID, "--description", "New text.", "--project", "project-1",
			}, env)
			if err != nil {
				t.Fatalf("slice-edit: %v", err)
			}
			if out.Len() == 0 {
				t.Errorf("output = %q, want the edit reported despite the failed push", out.String())
			}

			path, err := store.LocalPath("project-1")
			if err != nil {
				t.Fatal(err)
			}
			local, err := store.OpenLocal(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = local.Close() }()
			dirty, err := local.Dirty(context.Background(), testSliceID)
			if err != nil {
				t.Fatal(err)
			}
			if !dirty {
				t.Error("dirty = false, want the slice left dirty for the next sync")
			}
		})
	}
}

func TestSliceEditRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-edit", "--description", "x", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestSliceEditRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "x", "--bogus", "--project", "project-1",
	}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceEditRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-edit", "not-a-uuid", "--description", "x", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceEditRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "x", "--project", "nope",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

// A plan never pulled from the workspace is hydrated on the way to reading
// anything at all, and a workspace that will not answer that pull fails the
// whole command before the slice itself is ever read.
func TestSliceEditReportsAFailedHydrate(t *testing.T) {
	boom := errors.New("notion is down")
	api := &fakeAPI{dataSourceErr: boom}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "x", "--project", "project-1",
	}, env)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

// The new brief is a local write before anything is pushed, and a plan that
// cannot make that write fails the command outright.
func TestSliceEditReportsAFailedLocalWrite(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE slices DROP COLUMN body`); err != nil {
			t.Fatalf("break the plan's body column: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "x", "--project", "project-1",
	}, env)

	if err == nil {
		t.Error("slice-edit over a plan that cannot write the brief: want an error")
	}
}

func TestSliceEditReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "x", "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

// A stdin that cannot be read fails before Notion is touched at all.
func TestSliceEditRefusesAnUnreadableStdin(t *testing.T) {
	env, _ := testEnv(testConfig(t), editableAPI())
	env.In = errReader{err: fmt.Errorf("boom")}

	err := Run(context.Background(), []string{
		"slice-edit", testSliceID, "--description", "-", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "read the description") {
		t.Errorf("err = %v, want the failed stdin read named", err)
	}
}
