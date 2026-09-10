package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

// The plainest closing edge there is: the slice being written to is already
// waited on by the slice it is being made to wait on. Notion would record it
// happily, and what it would make is two slices neither of which can ever be
// handed out.
func TestSliceDependsRefusesADirectCycle(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depBlocker, "--on", depWaiting, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(),
		`"Style the board" would then wait on itself: "Style the board" → "Render the board" → "Style the board"`) {
		t.Fatalf("err = %v, want the cycle read out from the slice being written to", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A cycle closed through a third slice is refused the same way: the whole plan
// is read, so how far round the wait comes back makes no difference.
func TestSliceDependsRefusesATransitiveCycle(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depBlocker, depSpare)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depSpare, "--on", depWaiting, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(),
		`"Queued work" → "Render the board" → "Style the board" → "Queued work"`) {
		t.Fatalf("err = %v, want the whole way round named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// The check is against the graph the write would leave rather than the one
// there is: --clear may be dropping the very edge that closed the cycle, and a
// slice already caught in one has to be able to get out.
func TestSliceDependsAllowsAClearThatBreaksTheCycle(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depBlocker, depWaiting)
	env, _ := testEnv(testConfig(), api)

	if err := Run(context.Background(),
		[]string{"slice-depends", depWaiting, "--clear", "--on", depSpare, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want the one write", api.updates)
	}
	if got := dependencyIDsOf(t, api.updates[0]); len(got) != 1 || got[0] != depSpare {
		t.Errorf("depends_on = %v, want just the slice named", got)
	}
}

// Dropping dependencies cannot close a cycle, so the plan is not read to prove
// it: a Notion that will not answer the query is no obstacle to a --clear.
func TestSliceDependsClearAloneReadsNoPlan(t *testing.T) {
	api := dependsAPI(t)
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}
	env, _ := testEnv(testConfig(), api)

	if err := Run(context.Background(),
		[]string{"slice-depends", depWaiting, "--clear", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-depends: %v", err)
	}
	for _, q := range api.queries {
		if q.id == "slices-ds" {
			t.Errorf("queries = %+v, want the plan left unread", api.queries)
		}
	}
}

// A check that could not be made is no assurance at all, so a plan that cannot
// be read stops the write rather than being passed over.
func TestSliceDependsRefusesWhenThePlanCannotBeRead(t *testing.T) {
	api := dependsAPI(t)
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depSpare, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Fatalf("err = %v, want the failed read named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A slice the project's query does not return — one filed outside the project
// the flag named — still has the dependencies it is being given, and they still
// lead wherever they lead.
func TestSliceDependsRefusesACycleFromOutsideThePlan(t *testing.T) {
	const outside = "3b838308f654816da085f46dd135adf0"
	api := dependsAPI(t)
	api.pages["other-ds"] = []notion.Page{slicePage(outside, "Elsewhere", notion.SliceTodo, "", "", "")}
	dependsOn(api, depWaiting, outside)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", outside, "--on", depWaiting, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(),
		`"Elsewhere" would then wait on itself: "Elsewhere" → "Render the board" → "Elsewhere"`) {
		t.Fatalf("err = %v, want the cycle named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A cycle the plan already holds is one the document would leave behind, so it
// is refused too — adding work to a plan nobody can finish only buries the
// mistake deeper, and this is the last moment anybody is looking.
func TestPlanApplyRefusesACycleTheBoardAlreadyHas(t *testing.T) {
	api := planAPI(1)
	boardSlices(api)
	dependsOn(api, depBlocker, depSpare)
	dependsOn(api, depSpare, depBlocker)
	doc := `{
	  "slices": [{"title": "Frame the board", "milestone": "M2: Board", "depends_on": ["Style the board"]}]
	}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(),
		`"Style the board" → "Queued work" → "Style the board"`) {
		t.Fatalf("err = %v, want the cycle already on the board named", err)
	}
	if len(api.creates) != 0 || len(api.updates) != 0 {
		t.Errorf("creates = %+v, updates = %+v, want nothing written", api.creates, api.updates)
	}
}

// A document whose own slices wait on each other is refused whole, with both
// titles named and nothing created — a plan half applied is bad enough without
// the half that landed being unworkable.
func TestPlanApplyRefusesADirectCycle(t *testing.T) {
	api := planAPI(2)
	doc := `{"slices": [
	  {"title": "Frame the board", "milestone": "M2: Board", "depends_on": ["Draw the board"]},
	  {"title": "Draw the board", "milestone": "M2: Board", "depends_on": ["Frame the board"]}
	]}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(),
		`"Frame the board" → "Draw the board" → "Frame the board"`) {
		t.Fatalf("err = %v, want the cycle named in order", err)
	}
	if !strings.Contains(err.Error(), "no slice in a cycle can ever be unblocked") {
		t.Errorf("err = %v, want it to say what a cycle costs", err)
	}
	if len(api.creates) != 0 || len(api.updates) != 0 {
		t.Errorf("creates = %+v, updates = %+v, want nothing written", api.creates, api.updates)
	}
}

// However many slices the wait comes back through, it is the same refusal.
func TestPlanApplyRefusesATransitiveCycle(t *testing.T) {
	api := planAPI(3)
	doc := `{"slices": [
	  {"title": "One", "milestone": "M2: Board", "depends_on": ["Two"]},
	  {"title": "Two", "milestone": "M2: Board", "depends_on": ["Three"]},
	  {"title": "Three", "milestone": "M2: Board", "depends_on": ["One"]}
	]}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(), `"One" → "Two" → "Three" → "One"`) {
		t.Fatalf("err = %v, want the whole way round named", err)
	}
	if len(api.creates) != 0 {
		t.Errorf("creates = %+v, want nothing written", api.creates)
	}
}

// A plan closes a cycle through work already on the board just as easily as
// through its own, which is why the project's slices are part of the graph.
func TestPlanApplyRefusesACycleThroughTheBoard(t *testing.T) {
	api := planAPI(1)
	boardSlices(api)
	dependsOn(api, depBlocker, depSpare)
	doc := `{
	  "slices": [{"title": "Frame the board", "milestone": "M2: Board", "depends_on": ["Style the board"]}],
	  "dependencies": [{"slice": "Queued work", "on": ["Frame the board"]}]
	}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(),
		`"Frame the board" → "Style the board" → "Queued work" → "Frame the board"`) {
		t.Fatalf("err = %v, want the cycle through the board named", err)
	}
	if len(api.creates) != 0 || len(api.updates) != 0 {
		t.Errorf("creates = %+v, updates = %+v, want nothing written", api.creates, api.updates)
	}
}

// Two cycles are both named: a document with one mistake in it usually has the
// other, and being told one at a time is a run per edge.
func TestPlanApplyNamesEveryCycle(t *testing.T) {
	api := planAPI(4)
	doc := `{"slices": [
	  {"title": "One", "milestone": "M2: Board", "depends_on": ["Two"]},
	  {"title": "Two", "milestone": "M2: Board", "depends_on": ["One"]},
	  {"title": "Three", "milestone": "M2: Board", "depends_on": ["Four"]},
	  {"title": "Four", "milestone": "M2: Board", "depends_on": ["Three"]}
	]}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(), "would leave 2 cycles of dependencies") {
		t.Fatalf("err = %v, want both cycles counted", err)
	}
	for _, want := range []string{`"One" → "Two" → "One"`, `"Three" → "Four" → "Three"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to name %s", err, want)
		}
	}
}

// A dependency the plan hangs on work already filed is no cycle, and applies
// exactly as it did before there was any checking.
func TestPlanApplyAllowsADependencyOnTheBoard(t *testing.T) {
	api := planAPI(1)
	boardSlices(api)
	doc := `{
	  "slices": [{"title": "Frame the board", "milestone": "M2: Board", "depends_on": ["Style the board"]}]
	}`

	if _, err := runPlan(t, api, doc); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want the one relation written", api.updates)
	}
	if got := dependencyIDsOf(t, api.updates[0]); len(got) != 1 || got[0] != depBlocker {
		t.Errorf("depends_on = %v, want the board's slice", got)
	}
}

// A slice whose dependencies are all Done is workable, whatever the graph does
// further round — so next-slice hands it out rather than reporting the cycle it
// happens to be part of.
func TestNextSliceHandsOutASliceWhoseCycleIsFinished(t *testing.T) {
	api := dependsAPI(t)
	// depDone is Done and waits back on the slice waiting on it: a cycle, and
	// nothing is stuck, because a Done dependency is no dependency at all.
	dependsOn(api, depWaiting, depDone)
	dependsOn(api, depDone, depWaiting)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}
	if len(api.updates) != 1 || api.updates[0].id != depWaiting {
		t.Errorf("updates = %+v, want the slice claimed anyway", api.updates)
	}
}

// A cycle among the candidates is reported as one: naming an unfinished slice
// it waits on would be true and no help, since nothing in a cycle can finish.
func TestNextSliceReportsACycleAsOne(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depBlocker, depWaiting)
	dependsOn(api, depSpare, depWaiting)
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"next-slice", "--project", "project-1"}, env)

	if err == nil {
		t.Fatal("next-slice succeeded, want every candidate refused")
	}
	if !strings.Contains(err.Error(),
		`"Render the board" is in a dependency cycle: "Render the board" → "Style the board" → "Render the board"`) {
		t.Errorf("err = %v, want the cycle read out", err)
	}
	// The slice merely waiting on a member of the cycle is still reported for
	// what it is: what it waits on.
	if !strings.Contains(err.Error(), `"Queued work" waits on "Render the board" (Todo)`) {
		t.Errorf("err = %v, want the ordinary wait said plainly", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing claimed", api.updates)
	}
}

// A cycle among slices the plan cannot see is no cycle here: such a page has no
// dependencies to lead anywhere, exactly as Blockers passes over it.
func TestPlanApplyPassesOverAnUnreadableDependency(t *testing.T) {
	api := planAPI(1)
	boardSlices(api)
	dependsOn(api, depSpare, depGone)
	doc := `{
	  "slices": [{"title": "Frame the board", "milestone": "M2: Board", "depends_on": ["Queued work"]}]
	}`

	if _, err := runPlan(t, api, doc); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	if len(api.creates) != 1 {
		t.Errorf("creates = %+v, want the slice created", api.creates)
	}
}

// A milestone-only plan names no dependency, so the project's slices are never
// read and there is no graph to check.
func TestPlanApplyWithNoDependenciesChecksNothing(t *testing.T) {
	api := planAPI(0)
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}

	if _, err := runPlan(t, api, `{"milestones": [{"name": "M9: Later"}]}`); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	if len(api.schemaUpdates) != 1 {
		t.Errorf("schemaUpdates = %+v, want the milestone appended", api.schemaUpdates)
	}
}
