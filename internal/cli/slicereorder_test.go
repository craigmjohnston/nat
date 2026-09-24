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

const otherSliceID = "4b738308f654815fa843dce9c020efb4"

// reorderableAPI is a plan of two slices: the first under M1 with the given
// status, the second under the given milestone.
func reorderableAPI(status, otherMilestone string) *fakeAPI {
	return &fakeAPI{
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client", "M2: Board"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePage(testSliceID, "Render the board", status, "M1: Client", "", ""),
				slicePage(otherSliceID, "Draw the rail", notion.SliceTodo, otherMilestone, "", ""),
			},
		},
	}
}

func reorder(t *testing.T, api *fakeAPI, args ...string) (string, int, error) {
	t.Helper()
	env, out := testEnv(testConfig(t), api)
	var nudges int
	env.Nudge = func() { nudges++ }
	err := Run(context.Background(), append([]string{"slice-reorder"}, append(args, "--project", "project-1")...), env)
	return out.String(), nudges, err
}

func TestSliceReorderWithinAMilestoneWritesNothingToNotion(t *testing.T) {
	api := reorderableAPI(notion.SliceTodo, "M1: Client")
	out, nudges, err := reorder(t, api, testSliceID, "--before", otherSliceID)
	if err != nil {
		t.Fatalf("slice-reorder: %v", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want a reorder within a milestone to send nothing", api.updates)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1", nudges)
	}
	if !strings.Contains(out, "Placed directly before Draw the rail.") || strings.Contains(out, "Refiled") {
		t.Errorf("output = %q", out)
	}
}

func TestSliceReorderAcrossMilestonesRefilesAndSaysSo(t *testing.T) {
	api := reorderableAPI(notion.SliceTodo, "M2: Board")
	out, _, err := reorder(t, api, testSliceID, "--after", otherSliceID)
	if err != nil {
		t.Fatalf("slice-reorder: %v", err)
	}
	if len(api.updates) != 1 || api.updates[0].props[notion.PropMilestone].Select.Name != "M2: Board" {
		t.Errorf("updates = %+v, want the refile to M2 pushed", api.updates)
	}
	if !strings.Contains(out, "Placed directly after Draw the rail. Refiled under M2: Board.") {
		t.Errorf("output = %q", out)
	}
}

func TestSliceReorderJSONSaysWhatMovedAndWhereItSits(t *testing.T) {
	api := reorderableAPI(notion.SliceTodo, "M2: Board")
	out, _, err := reorder(t, api, testSliceID, "--before", otherSliceID, "--json")
	if err != nil {
		t.Fatalf("slice-reorder --json: %v", err)
	}
	var got sliceReorderedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.ID != testSliceID || got.MilestoneID != "M2: Board" || !got.Refiled || got.Placement != "before" ||
		got.RelativeTo.ID != otherSliceID || got.RelativeTo.MilestoneID != "M2: Board" {
		t.Errorf("json = %+v", got)
	}
}

func TestSliceReorderUnderNoMilestoneSaysSo(t *testing.T) {
	// The file takes the refile; the workspace cannot hold "no milestone", which
	// is logged and left for a later sync rather than failing the command.
	api := reorderableAPI(notion.SliceTodo, "")
	out, _, err := reorder(t, api, testSliceID, "--after", otherSliceID)
	if err != nil {
		t.Fatalf("slice-reorder: %v", err)
	}
	if !strings.Contains(out, "Refiled under no milestone.") {
		t.Errorf("output = %q", out)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing sent for a milestone Notion cannot hold", api.updates)
	}
}

func TestSliceReorderRefusals(t *testing.T) {
	cases := map[string]struct {
		status string
		args   []string
		want   string
	}{
		"no slice":       {notion.SliceTodo, []string{"--before", otherSliceID}, "want exactly one slice"},
		"not a slice":    {notion.SliceTodo, []string{"nope", "--before", otherSliceID}, "is not a slice"},
		"no flag":        {notion.SliceTodo, []string{testSliceID}, "no destination given"},
		"both flags":     {notion.SliceTodo, []string{testSliceID, "--before", otherSliceID, "--after", otherSliceID}, "two places at once"},
		"bad target":     {notion.SliceTodo, []string{testSliceID, "--after", "nope"}, "is not a slice"},
		"self":           {notion.SliceTodo, []string{testSliceID, "--before", testSliceID}, "beside itself"},
		"unknown slice":  {notion.SliceTodo, []string{"5b738308f654815fa843dce9c020efb4", "--before", otherSliceID}, "load the slice"},
		"unknown target": {notion.SliceTodo, []string{testSliceID, "--before", "5b738308f654815fa843dce9c020efb4"}, "load the slice"},
		"bad flag":       {notion.SliceTodo, []string{testSliceID, "--nope"}, "nope"},
	}
	for name, tc := range cases {
		api := reorderableAPI(tc.status, "M2: Board")
		_, nudges, err := reorder(t, api, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if len(api.updates) != 0 || nudges != 0 {
			t.Errorf("%s: refused reorder still wrote %+v / nudged %d", name, api.updates, nudges)
		}
	}
}

// Work in flight is not refiled under its agent — but where it sits within its
// own milestone is the plan's to arrange.
func TestSliceReorderRefusesRefilingInProgressButNotReordering(t *testing.T) {
	api := reorderableAPI(notion.SliceInProgress, "M2: Board")
	if _, _, err := reorder(t, api, testSliceID, "--before", otherSliceID); err == nil ||
		!strings.Contains(err.Error(), "in progress") {
		t.Errorf("err = %v, want the refile refused", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("refused reorder wrote %+v", api.updates)
	}

	api = reorderableAPI(notion.SliceInProgress, "M1: Client")
	if _, _, err := reorder(t, api, testSliceID, "--before", otherSliceID); err != nil {
		t.Errorf("reorder within a milestone: %v", err)
	}
}

func TestSliceReorderSucceedsThoughTheWorkspaceWriteFails(t *testing.T) {
	api := reorderableAPI(notion.SliceTodo, "M2: Board")
	api.updateErr = errors.New("notion refused")
	if _, nudges, err := reorder(t, api, testSliceID, "--before", otherSliceID); err != nil || nudges != 1 {
		t.Errorf("err = %v, nudges = %d, want the file's own write to stand", err, nudges)
	}
}

func TestSliceReorderRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), []string{
		"slice-reorder", testSliceID, "--before", otherSliceID, "--project", "nope",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceReorderReportsAFailedSchemaRead(t *testing.T) {
	api := reorderableAPI(notion.SliceTodo, "M2: Board")
	api.dataSourceErr = errors.New("notion is down")
	if _, _, err := reorder(t, api, testSliceID, "--before", otherSliceID); err == nil {
		t.Error("err = nil, want the failed schema read reported")
	}
}

// The reorder is a local write before anything is pushed, and a plan that
// cannot make it fails the command outright.
func TestSliceReorderReportsAFailedLocalWrite(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`INSERT INTO slices (id, title, status, position) VALUES (?, 'Draw the rail', 'Todo', 1)`, otherSliceID); err != nil {
			t.Fatalf("seed the target: %v", err)
		}
		if _, err := db.Exec(`CREATE TRIGGER block_reorder BEFORE UPDATE OF position ON slices
			BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
			t.Fatalf("install the blocking trigger: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	err := Run(context.Background(), []string{
		"slice-reorder", testSliceID, "--after", otherSliceID, "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "reorder the slice") {
		t.Errorf("err = %v, want the failed write named", err)
	}
}

// A plan file that cannot answer for its own shape fails the command before
// either slice is read.
func TestSliceReorderReportsAFailedLocalShapeRead(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Render the board", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`ALTER TABLE project DROP COLUMN has_assignee`); err != nil {
			t.Fatalf("break the plan's has_assignee column: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	err := Run(context.Background(), []string{
		"slice-reorder", testSliceID, "--before", otherSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Error("err = nil, want the plan's own shape failure reported")
	}
}
