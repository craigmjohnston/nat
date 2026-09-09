package cli

import (
	"context"
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
	env, out := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestSliceDeleteRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--bogus", "--project", "project-1",
	}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceDeleteRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceDeleteRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceDeleteReportsAFailedRead(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	api.getErr = errors.New("notion is down")
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
	if len(api.trashes) != 0 {
		t.Errorf("failed read still trashed: %v", api.trashes)
	}
}

func TestSliceDeleteReportsAFailedTrash(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	api.trashErr = errors.New("notion refused")
	env, _ := testEnv(testConfig(), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	err := Run(context.Background(), []string{
		"slice-delete", testSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "delete the slice") {
		t.Errorf("err = %v, want the failed trash named", err)
	}
	if nudges != 0 {
		t.Errorf("nudges = %d, want none for a failed delete", nudges)
	}
}
