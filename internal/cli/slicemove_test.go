package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
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
	env, out := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), api)

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
	env, _ := testEnv(testConfig(), movableAPI(notion.SliceTodo))

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M3: Nope", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), `no milestone named "M3: Nope"`) {
		t.Errorf("err = %v, want the unknown milestone named", err)
	}
}

func TestSliceMoveRefusesNoMilestone(t *testing.T) {
	env, _ := testEnv(testConfig(), movableAPI(notion.SliceTodo))

	err := Run(context.Background(), []string{"slice-move", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no milestone given") {
		t.Errorf("err = %v, want 'no milestone given'", err)
	}
}

func TestSliceMoveRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestSliceMoveRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--bogus", "--project", "project-1",
	}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceMoveRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", "not-a-uuid", "--milestone", "M2: Board", "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceMoveRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "nope",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceMoveReportsAFailedSliceRead(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	api.getErr = errors.New("notion is down")
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestSliceMoveReportsAFailedSchemaRead(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	api.dataSourceErr = errors.New("notion is down")
	env, _ := testEnv(testConfig(), api)

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

func TestSliceMoveReportsAFailedWrite(t *testing.T) {
	api := movableAPI(notion.SliceTodo)
	api.updateErr = errors.New("notion refused")
	env, _ := testEnv(testConfig(), api)
	var nudges int
	env.Nudge = func() { nudges++ }

	err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "move the slice") {
		t.Errorf("err = %v, want the failed write named", err)
	}
	if nudges != 0 {
		t.Errorf("nudges = %d, want none for a failed move", nudges)
	}
}
