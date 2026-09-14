package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// movableAPI answers with one Todo slice filed under the first of two
// milestones, which is what slice-move refiles under the second.
func movableAPI(status string) *fakeAPI {
	return &fakeAPI{
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client", "M2: Board"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Render the board", status, "M1: Client", "", "")},
		},
	}
}

func TestSliceMoveRefilesTheSlice(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-move: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != testSliceID {
		t.Fatalf("updates = %+v, want exactly one, of the slice", api.updates)
	}
	props := api.updates[0].props
	if len(props) != 1 {
		t.Errorf("props = %v, want the Milestone column alone", props)
	}
	milestone := props[notion.PropMilestone]
	if milestone.Select == nil || milestone.Select.Name != "M2: Board" {
		t.Errorf("milestone written = %+v, want the option naming M2: Board", milestone)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1", nudges)
	}
	if !strings.Contains(out.String(), "Moved to M2: Board") {
		t.Errorf("output missing the destination:\n%s", out.String())
	}
}

// A Done slice may be refiled: where finished work is recorded is still the
// plan's to arrange.
func TestSliceMoveAllowsDone(t *testing.T) {
	api := movableAPI(notion.SliceDone)
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-move: %v", err)
	}
	if len(api.updates) != 1 {
		t.Errorf("updates = %+v, want the move written", api.updates)
	}
}

func TestSliceMoveJSON(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	env, out := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-move --json: %v", err)
	}
	var got sliceMovedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.ID != testSliceID || got.MilestoneName != "M2: Board" {
		t.Errorf("json = %+v", got)
	}
}

func TestSliceMoveRefusesInProgress(t *testing.T) {
	api := movableAPI(notion.SliceInProgress)
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("err = %v, want 'in progress'", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("refused move still wrote: %+v", api.updates)
	}
}

func TestSliceMoveRefusesTheMilestoneItIsAlreadyUnder(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M1: Client", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "already filed under") {
		t.Errorf("err = %v, want 'already filed under'", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("refused move still wrote: %+v", api.updates)
	}
}

func TestSliceMoveRefusesAnUnknownMilestone(t *testing.T) {
	env, _ := testEnv(testConfig(t), movableAPI(notion.SliceTodo))

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M3: Nope", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), `no milestone named "M3: Nope"`) {
		t.Errorf("err = %v, want the unknown milestone named", err)
	}
}

func TestSliceMoveRefusesNoMilestone(t *testing.T) {
	env, _ := testEnv(testConfig(t), movableAPI(notion.SliceTodo))

	err := Run(context.Background(), []string{"slice-move", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no milestone given") {
		t.Errorf("err = %v, want 'no milestone given'", err)
	}
}

func TestSliceMoveRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestSliceMoveRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--bogus", "--project", "project-1",
	}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceMoveRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", "not-a-uuid", "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceMoveRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "nope",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

// The slice is already hydrated into the local plan by the time it is read,
// so a failed read now is the plan file's own initial pull, not a page fetch
// by ID — api.queryErr stands in for that where api.getErr used to.
func TestSliceMoveReportsAFailedSliceRead(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestSliceMoveReportsAFailedSchemaRead(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	api.dataSourceErr = errors.New("notion is down")
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err == nil {
		t.Fatal("err = nil, want the failed schema read reported")
	}
	if len(api.updates) != 0 {
		t.Errorf("failed schema read still wrote: %+v", api.updates)
	}
}

// The project's shape is read from the local file once the plan has been
// pulled — no request of its own — and a file that cannot even answer that
// fails the command before any milestone is resolved.
func TestSliceMoveReportsAFailedLocalShapeRead(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE project DROP COLUMN has_assignee`); err != nil {
			t.Fatalf("break the plan's has_assignee column: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil {
		t.Error("slice-move over a plan that cannot read its own shape: want an error")
	}
}

// A slice named that the file has never met, and that the workspace cannot
// answer for either, fails the load outright.
func TestSliceMoveReportsAFailedLoad(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", "other-slice", "Somebody else", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`INSERT INTO milestones (name, position) VALUES (?, 0)`, "M2: Board"); err != nil {
			t.Fatalf("seed the milestone: %v", err)
		}
	})
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(cfg, api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil {
		t.Error("slice-move over a slice neither the file nor the workspace has: want an error")
	}
}

// The move itself is a local write before anything is pushed, and a plan
// that cannot make that write fails the command outright.
func TestSliceMoveReportsAFailedLocalWrite(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`INSERT INTO milestones (name, position) VALUES (?, 0)`, "M2: Board"); err != nil {
			t.Fatalf("seed the milestone: %v", err)
		}
		// A trigger, rather than dropping a column the read needs too: this
		// breaks only the write the move itself makes, not the reads that
		// lead up to it.
		if _, err := db.Exec(`CREATE TRIGGER block_move BEFORE UPDATE OF milestone ON slices
			WHEN NEW.milestone = 'M2: Board' BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
			t.Fatalf("install the blocking trigger: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil {
		t.Error("slice-move over a plan that cannot move the slice: want an error")
	}
}

// A push to the workspace that fails does not fail slice-move: the file
// already holds the new milestone, so the command succeeds, still nudges,
// and leaves the slice dirty for the next sync to resend.
func TestSliceMoveSucceedsThoughTheWorkspaceWriteFails(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	api.updateErr = errors.New("notion refused")
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	if err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("slice-move: %v", err)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1: the file's own move landed", nudges)
	}
	if out.Len() == 0 {
		t.Errorf("output = %q, want the move reported despite the failed push", out.String())
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
}
