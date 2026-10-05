package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// prunePlan is a local project with one slice under Lone, two under Pair, and
// Empty holding nothing — the milestone milestone-add makes, which no move or
// delete elsewhere may take with it.
type prunePlan struct {
	env Env
	out interface {
		String() string
		Reset()
	}
	id   string
	st   store.Store
	sp   store.Project
	lone domain.Slice
	pair [2]domain.Slice
}

func newPrunePlan(t *testing.T) prunePlan {
	t.Helper()
	ctx := context.Background()
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	if err := Run(ctx, []string{"scratch-open"}, env); err != nil {
		t.Fatal(err)
	}
	id := saved.ScratchProject
	entry := saved.Projects[id]
	st, err := env.storeFor(ctx, id, entry)
	if err != nil {
		t.Fatal(err)
	}
	sp := storeProject(id, entry)
	sh, err := st.Shape(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := st.AddMilestones(ctx, sp, sh, []string{"Lone", "Pair", "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(title string, m domain.Milestone) domain.Slice {
		s, err := st.AddSlice(ctx, sp, store.NewSlice{Title: title, Milestone: m})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	p := prunePlan{env: env, out: out, id: id, st: st, sp: sp}
	p.lone = add("lone", ms[0])
	p.pair = [2]domain.Slice{add("first", ms[1]), add("second", ms[1])}
	out.Reset()
	return p
}

// run runs one command against the plan and answers with what it printed.
func (p prunePlan) run(t *testing.T, args ...string) string {
	t.Helper()
	p.out.Reset()
	if err := Run(context.Background(), append(args, "--project", p.id), p.env); err != nil {
		t.Fatalf("%s: %v", args[0], err)
	}
	return p.out.String()
}

// milestones is the plan's milestones by name, as they now stand.
func (p prunePlan) milestones(t *testing.T) string {
	t.Helper()
	plan, err := p.st.Plan(context.Background(), p.sp)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(plan.Project.Milestones))
	for i, m := range plan.Project.Milestones {
		names[i] = m.Name
	}
	return strings.Join(names, ",")
}

func TestSliceMoveRemovesTheMilestoneItEmptied(t *testing.T) {
	p := newPrunePlan(t)
	out := p.run(t, "slice-move", p.lone.ID, "--milestone", "Pair", "--json")
	var got sliceMovedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.RemovedMilestone != "Lone" {
		t.Errorf("removed_milestone = %q, want Lone", got.RemovedMilestone)
	}
	if ms := p.milestones(t); ms != "Pair,Empty" {
		t.Errorf("milestones = %s, want Lone gone and Empty kept", ms)
	}
}

func TestSliceMoveKeepsAMilestoneStillHoldingASlice(t *testing.T) {
	p := newPrunePlan(t)
	out := p.run(t, "slice-move", p.pair[0].ID, "--milestone", "Lone", "--json")
	if strings.Contains(out, "removed_milestone") {
		t.Errorf("json = %s, want no removed_milestone", out)
	}
	out = p.run(t, "slice-move", p.pair[1].ID, "--milestone", "Lone")
	if !strings.Contains(out, "Removed Pair, which no slice is filed under any more.") {
		t.Errorf("output = %q, want the removal named", out)
	}
	if ms := p.milestones(t); ms != "Lone,Empty" {
		t.Errorf("milestones = %s, want Pair gone once its last slice left", ms)
	}
}

func TestSliceDeleteRemovesTheMilestoneItEmptied(t *testing.T) {
	p := newPrunePlan(t)
	out := p.run(t, "slice-delete", p.pair[0].ID)
	if strings.Contains(out, "Removed") {
		t.Errorf("output = %q, want no removal: Pair still holds a slice", out)
	}
	out = p.run(t, "slice-delete", p.lone.ID, "--json")
	var got sliceDeletedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.RemovedMilestone != "Lone" {
		t.Errorf("removed_milestone = %q, want Lone", got.RemovedMilestone)
	}
	out = p.run(t, "slice-delete", p.pair[1].ID)
	if !strings.Contains(out, "Removed Pair") {
		t.Errorf("output = %q, want the removal named", out)
	}
	if ms := p.milestones(t); ms != "Empty" {
		t.Errorf("milestones = %s, want only the already-empty one left", ms)
	}
}

func TestSliceReorderRemovesTheMilestoneARefileEmptied(t *testing.T) {
	p := newPrunePlan(t)
	// Within one milestone, nothing is left anywhere.
	out := p.run(t, "slice-reorder", p.pair[1].ID, "--before", p.pair[0].ID, "--json")
	if strings.Contains(out, "removed_milestone") {
		t.Errorf("json = %s, want no removed_milestone", out)
	}
	out = p.run(t, "slice-reorder", p.lone.ID, "--after", p.pair[1].ID, "--json")
	var got sliceReorderedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if !got.Refiled || got.RemovedMilestone != "Lone" {
		t.Errorf("json = %+v, want refiled with Lone removed", got)
	}
	if ms := p.milestones(t); ms != "Pair,Empty" {
		t.Errorf("milestones = %s, want Lone gone", ms)
	}
	out = p.run(t, "slice-reorder", p.lone.ID, "--before", p.pair[0].ID)
	if strings.Contains(out, "Removed") {
		t.Errorf("output = %q, want no removal", out)
	}
}

func TestSliceReorderTextNamesTheRemovedMilestone(t *testing.T) {
	p := newPrunePlan(t)
	out := p.run(t, "slice-reorder", p.lone.ID, "--before", p.pair[0].ID)
	if !strings.Contains(out, "Refiled under Pair.\n\nRemoved Lone") {
		t.Errorf("output = %q, want the refile then the removal", out)
	}
}

func TestPlanApplyRemovesTheMilestonesItsMovesAndRemovalsEmptied(t *testing.T) {
	p := newPrunePlan(t)
	p.env.In = strings.NewReader(`{"move": [{"slice": "first", "milestone": "Lone"}, {"slice": "second", "milestone": "Lone"}],
		"remove": ["lone"]}`)
	out := p.run(t, "plan-apply", "--json")
	var got planAppliedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	// Lone lost its own slice but gained two: only Pair is emptied.
	if strings.Join(got.MilestonesRemoved, ",") != "Pair" {
		t.Errorf("milestones_removed = %v, want [Pair]", got.MilestonesRemoved)
	}
	if ms := p.milestones(t); ms != "Lone,Empty" {
		t.Errorf("milestones = %s, want Pair gone", ms)
	}
}

func TestPlanApplyKeepsAMilestoneItAlsoCreatesASliceUnder(t *testing.T) {
	p := newPrunePlan(t)
	p.env.In = strings.NewReader(`{"remove": ["lone"],
		"slices": [{"title": "replacement", "milestone": "Lone"}]}`)
	out := p.run(t, "plan-apply")
	if strings.Contains(out, "Milestones removed") {
		t.Errorf("output = %q, want nothing removed", out)
	}
	if ms := p.milestones(t); ms != "Lone,Pair,Empty" {
		t.Errorf("milestones = %s, want every one kept", ms)
	}

	p.env.In = strings.NewReader(`{"remove": ["replacement"]}`)
	out = p.run(t, "plan-apply")
	if !strings.Contains(out, "## Milestones removed\n\n- Lone — no slice is filed under it any more") {
		t.Errorf("output = %q, want Lone's removal listed", out)
	}
}

// pruneMirroredAPI is a Notion workspace whose pages follow the writes made to
// them, so the workspace's own refusal of a removal sees the slice gone.
func pruneMirroredAPI() *fakeAPI {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client", "M2: Board"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Render the board", notion.SliceTodo, "M1: Client", "", "")},
		},
	}
	api.onUpdate = func() {
		u := api.updates[len(api.updates)-1]
		for i, page := range api.pages["slices-ds"] {
			if page.ID == u.id {
				for k, v := range u.props {
					api.pages["slices-ds"][i].Properties[k] = v
				}
			}
		}
	}
	return api
}

func TestSliceMoveRemovesTheEmptiedMilestoneFromAMirroredWorkspace(t *testing.T) {
	api := pruneMirroredAPI()
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("slice-move: %v", err)
	}
	var got sliceMovedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if got.RemovedMilestone != "M1: Client" {
		t.Errorf("removed_milestone = %q, want M1: Client", got.RemovedMilestone)
	}
	if len(api.schemaUpdates) != 1 {
		t.Fatalf("schema writes = %+v, want the option dropped", api.schemaUpdates)
	}
	for _, o := range api.schemaUpdates[0].props[notion.PropMilestone].Select.Options {
		if o.Name == "M1: Client" {
			t.Errorf("options written = %+v, want M1: Client dropped", api.schemaUpdates[0].props)
		}
	}
}

// A removal the workspace refuses is logged, and the move it followed still
// succeeds, reporting nothing removed.
func TestSliceMoveSucceedsThoughTheRemovalFails(t *testing.T) {
	api := pruneMirroredAPI()
	api.schemaUpdateErr = errors.New("notion refused")
	env, out := testEnv(testConfig(t), api)

	if err := Run(context.Background(), []string{
		"slice-move", testSliceID, "--milestone", "M2: Board", "--json", "--project", "project-1",
	}, env); err != nil {
		t.Fatalf("slice-move: %v", err)
	}
	if strings.Contains(out.String(), "removed_milestone") {
		t.Errorf("json = %s, want no removed_milestone", out.String())
	}
}
