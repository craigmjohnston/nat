package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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

func TestSliceDeleteRefusesInProgress(t *testing.T) {
	api := deletableAPI(notion.SliceInProgress)
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("err = %v, want 'in progress'", err)
	}
	if len(api.trashes) != 0 {
		t.Errorf("refused delete still trashed: %v", api.trashes)
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
