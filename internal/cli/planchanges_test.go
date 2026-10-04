package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

const depClaimed = "3b838308f654816da085f46dd135ade6"

// changesAPI is dependsAPI's board — "Render the board" waiting on "Style the
// board", "Queued work" alone under M3, "Notion client" Done — plus a slice in
// progress, with pages to hand out for however many creations a test expects.
func changesAPI(t *testing.T, creations int) *fakeAPI {
	t.Helper()
	api := dependsAPI(t)
	api.pages["slices-ds"] = append(api.pages["slices-ds"],
		slicePage(depClaimed, "Work in flight", notion.SliceInProgress, "M2: Board", "Craig Johnston", ""))
	api.createdPages = createdSeq(creations)
	return api
}

// assertNothingChanged fails unless the run wrote nothing at all to the
// workspace — no page, no property, no trash, no body.
func assertNothingChanged(t *testing.T, api *fakeAPI) {
	t.Helper()
	if len(api.creates) != 0 || len(api.updates) != 0 || len(api.trashes) != 0 || len(api.appends) != 0 ||
		len(api.deletes) != 0 || len(api.schemaUpdates) != 0 {
		t.Errorf("creates = %+v, updates = %+v, trashes = %v, appends = %+v, deletes = %v, schema = %+v; want nothing written",
			api.creates, api.updates, api.trashes, api.appends, api.deletes, api.schemaUpdates)
	}
}

// The superseding plan every happy-path test applies: it replaces "Style the
// board" with a new slice of the same title, refiles "Queued work" under the
// milestone it creates, and rewrites the brief of "Render the board" — which
// waited on the slice being removed.
const supersedingPlan = `{
  "milestones": [{"name": "M4: Polish"}],
  "slices": [{"title": "Style the board", "milestone": "M4: Polish", "description": "Style it anew."}],
  "remove": ["Style the board"],
  "move": [{"slice": "Queued work", "milestone": "M4: Polish"}],
  "edit": [{"slice": "Render the board", "description": "Render it, then stop."}]
}`

// One run removes, moves and edits slices already on the board and creates
// the replacement, dropping the wait on the removed slice and saying so.
func TestPlanApplyRemovesMovesAndEditsInOneRun(t *testing.T) {
	api := changesAPI(t, 1)

	out, err := runPlan(t, api, supersedingPlan)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if !equalLines(api.trashes, []string{depBlocker}) {
		t.Errorf("trashes = %v, want the removed slice alone", api.trashes)
	}
	if len(api.creates) != 1 || titleText(api.creates[0].props) != "Style the board" {
		t.Errorf("creates = %+v, want the replacement alone", api.creates)
	}
	var dropped, moved bool
	for _, u := range api.updates {
		switch u.id {
		case depWaiting:
			if got := dependencyIDsOf(t, u); len(got) != 0 {
				t.Errorf("depends_on = %v, want the wait on the removed slice dropped", got)
			}
			dropped = true
		case depSpare:
			if _, ok := u.props[notion.PropMilestone]; !ok {
				t.Errorf("update %+v, want the milestone written", u)
			}
			moved = true
		}
	}
	if !dropped || !moved {
		t.Errorf("updates = %+v, want the dropped wait and the move", api.updates)
	}
	var edited bool
	for _, a := range api.appends {
		if a.id == depWaiting && strings.Contains(fmt.Sprint(a.children), "Render it, then stop.") {
			edited = true
		}
	}
	if !edited {
		t.Errorf("appends = %+v, want the new brief written on the edited slice", api.appends)
	}
	for _, want := range []string{
		"Of the slices already there, 1 edited, 1 moved and 1 removed.\n",
		"## Edited\n\n- Render the board — brief replaced\n",
		"## Moved\n\n- Queued work — now under M4: Polish\n",
		"## Removed\n\n- Style the board — no longer waited on by \"Render the board\"\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output =\n%s\nwant %q", out, want)
		}
	}
}

// titleText reads a created page's title back as plain text.
func titleText(props map[string]notion.PropertyValue) string {
	v := props[notion.PropName]
	var b strings.Builder
	for _, t := range v.Title {
		b.WriteString(t.PlainText)
		if t.Text != nil && t.PlainText == "" {
			b.WriteString(t.Text.Content)
		}
	}
	return b.String()
}

func TestPlanApplyPrintsTheChangesAsJSON(t *testing.T) {
	api := changesAPI(t, 1)

	out, err := runPlan(t, api, supersedingPlan, "--json")
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	var got planAppliedJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if want := []sliceEditedJSON{{ID: depWaiting, Name: "Render the board", URL: "https://notion.so/" + depWaiting,
		Brief: "Render it, then stop."}}; !reflect.DeepEqual(got.Edited, want) {
		t.Errorf("edited = %+v, want %+v", got.Edited, want)
	}
	if len(got.Moved) != 1 || got.Moved[0].ID != depSpare || got.Moved[0].MilestoneName != "M4: Polish" {
		t.Errorf("moved = %+v, want Queued work under M4: Polish", got.Moved)
	}
	want := []removedSliceJSON{{ID: depBlocker, Name: "Style the board", URL: "https://notion.so/" + depBlocker,
		Dependents: []namedSliceRef{{ID: depWaiting, Name: "Render the board"}}}}
	if !reflect.DeepEqual(got.Removed, want) {
		t.Errorf("removed = %+v, want %+v", got.Removed, want)
	}
}

// Each of these is refused whole, before the first write.
func TestPlanApplyRefusesABadChange(t *testing.T) {
	for _, tc := range []struct {
		name, doc, want string
		dup         bool
	}{
		{"removing a slice in progress", `{"remove": ["Work in flight"]}`,
			`remove 1 names "Work in flight", which is in progress`, false},
		{"moving a slice in progress", `{"move": [{"slice": "Work in flight", "milestone": "M3: Later"}]}`,
			`move 1 names "Work in flight", which is in progress`, false},
		{"editing a Done slice", `{"edit": [{"slice": "Notion client", "description": "x"}]}`,
			`edit 1 names "Notion client", which is already Done`, false},
		{"removing a Done slice", `{"remove": ["Notion client"]}`,
			`remove 1 names "Notion client", which is already Done`, false},
		{"an unknown title", `{"move": [{"slice": "Nothing like it", "milestone": "M3: Later"}]}`,
			`move 1 names "Nothing like it", which the project has no slice named`, false},
		{"an ambiguous title", `{"edit": [{"slice": "Queued work", "description": "x"}]}`,
			`edit 1 names "Queued work", which the project has 2 slices named`, true},
		{"an empty title", `{"remove": [" "]}`, `remove 1 names no slice`, false},
		{"a slice removed twice", `{"remove": ["Queued work", "queued work"]}`,
			`remove 2 names "Queued work", which the list already removes`, false},
		{"a removed slice moved", `{"remove": ["Queued work"], "move": [{"slice": "Queued work", "milestone": "M2: Board"}]}`,
			`move 1 names "Queued work", which the plan also removes`, false},
		{"a removed slice edited", `{"remove": ["Queued work"], "edit": [{"slice": "Queued work", "description": "x"}]}`,
			`edit 1 names "Queued work", which the plan also removes`, false},
		{"a slice moved twice", `{"move": [{"slice": "Queued work", "milestone": "M2: Board"}, {"slice": "Queued work", "milestone": "M1: Client"}]}`,
			`move 2 names "Queued work", which the list already moves`, false},
		{"a move with no milestone", `{"move": [{"slice": "Queued work"}]}`,
			`move 1 ("Queued work") names no milestone`, false},
		{"a move to an unknown milestone", `{"move": [{"slice": "Queued work", "milestone": "M9"}]}`,
			`move 1 ("Queued work"): no milestone named "M9"`, false},
		{"a move to where it already is", `{"move": [{"slice": "Queued work", "milestone": "M3: Later"}]}`,
			`move 1 ("Queued work"): it is already filed under M3: Later`, false},
		{"a slice edited twice", `{"edit": [{"slice": "Queued work", "description": "x"}, {"slice": "Queued work", "description": "y"}]}`,
			`edit 2 names "Queued work", which the list already edits`, false},
		{"an edit with no description", `{"edit": [{"slice": "Queued work", "description": "  "}]}`,
			`edit 1 ("Queued work") has no description`, false},
		{"a depends_on naming a removed slice", `{"remove": ["Style the board"],
			"slices": [{"title": "New", "milestone": "M2: Board", "depends_on": ["Style the board"]}]}`,
			`depends on "Style the board", which the plan removes`, false},
		{"a dependencies entry naming a removed slice", `{"remove": ["Queued work"],
			"dependencies": [{"slice": "Queued work", "on": ["Style the board"]}]}`,
			`dependencies 1: names "Queued work", which the plan removes`, false},
		{"a dependencies entry waiting on a removed slice", `{"remove": ["Queued work"],
			"dependencies": [{"slice": "Style the board", "on": ["Queued work"]}]}`,
			`depends on "Queued work", which the plan removes`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := changesAPI(t, 1)
			if tc.dup {
				api.pages["slices-ds"] = append(api.pages["slices-ds"],
					slicePage("3b838308f654816da085f46dd135ade7", "Queued work", notion.SliceTodo, "M1: Client", "", ""))
			}

			_, err := runPlan(t, api, tc.doc)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			assertNothingChanged(t, api)
		})
	}
}

// A cycle already on the board that a removal breaks is no cycle once the plan
// is in, so the plan applies — the wait on the removed slice dropped with it.
func TestPlanApplyAllowsACycleARemovalBreaks(t *testing.T) {
	api := changesAPI(t, 0)
	dependsOn(api, depBlocker, depSpare)
	dependsOn(api, depSpare, depBlocker)

	if _, err := runPlan(t, api, `{"remove": ["Queued work"]}`); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	if !equalLines(api.trashes, []string{depSpare}) {
		t.Errorf("trashes = %v, want the removed slice", api.trashes)
	}
	if len(api.updates) != 1 || api.updates[0].id != depBlocker || len(dependencyIDsOf(t, api.updates[0])) != 0 {
		t.Errorf("updates = %+v, want Style the board's wait on it dropped", api.updates)
	}
}

// A cycle the plan closes through the replacement of a removed slice is still
// refused: the check walks the board as the plan leaves it.
func TestPlanApplyRefusesACycleThroughAReplacement(t *testing.T) {
	api := changesAPI(t, 1)
	doc := `{
	  "remove": ["Render the board"],
	  "slices": [{"title": "Render the board", "milestone": "M2: Board", "depends_on": ["Style the board"]}],
	  "dependencies": [{"slice": "Style the board", "on": ["Render the board"]}]
	}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(), `"Render the board" → "Style the board" → "Render the board"`) {
		t.Fatalf("err = %v, want the cycle through the replacement named", err)
	}
	assertNothingChanged(t, api)
}

// A plan of nothing but changes is not an empty plan.
func TestPlanApplyAppliesAPlanOfChangesAlone(t *testing.T) {
	api := changesAPI(t, 0)

	out, err := runPlan(t, api, `{"edit": [{"slice": "Queued work", "description": "Queue it."}]}`)
	if err != nil {
		t.Fatalf("plan-apply: %v", err)
	}
	if len(api.schemaUpdates) != 0 || len(api.creates) != 0 {
		t.Errorf("schema = %+v, creates = %+v, want nothing created", api.schemaUpdates, api.creates)
	}
	if !strings.Contains(out, "## Edited\n\n- Queued work — brief replaced\n") {
		t.Errorf("output =\n%s\nwant the edit reported", out)
	}
}

// A source project's tasks stay under the container they were filed under, so
// a move is refused before anything is read.
func TestValidateAgainstProjectRefusesAMoveOnASourceProject(t *testing.T) {
	p := plan{Move: []planMove{{Slice: "x", Milestone: "y"}}}

	_, _, err := validateAgainstProject(context.Background(), nil, store.Project{Source: "shortcut"}, p)

	if err == nil || !strings.Contains(err.Error(), "the plan moves 1 slice, but this project's milestones are shortcut's containers") {
		t.Errorf("err = %v, want the move refused by the source", err)
	}
}

// changesStub records every write applyPlan makes, in order, and fails the
// one named by failOn.
type changesStub struct {
	applyPlanStub
	calls  []string
	failOn string
}

func (s *changesStub) call(name string) error {
	s.calls = append(s.calls, name)
	if name == s.failOn {
		return errors.New("notion is down")
	}
	return nil
}

func (s *changesStub) SetSliceBrief(_ context.Context, id, _ string) error { return s.call("edit " + id) }

func (s *changesStub) MoveSlice(_ context.Context, id string, _ domain.Milestone) error {
	return s.call("move " + id)
}

func (s *changesStub) DeleteSlice(_ context.Context, id string) error { return s.call("delete " + id) }

func (s *changesStub) SetDependencies(_ context.Context, id string, _ []string) (domain.Slice, error) {
	return domain.Slice{}, s.call("depends " + id)
}

func (s *changesStub) AddMilestones(ctx context.Context, p store.Project, sh store.Shape, names []string) ([]domain.Milestone, error) {
	if err := s.call("milestones"); err != nil {
		return nil, err
	}
	return s.applyPlanStub.AddMilestones(ctx, p, sh, names)
}

func (s *changesStub) AddSlice(ctx context.Context, p store.Project, n store.NewSlice) (domain.Slice, error) {
	if err := s.call("create " + n.Title); err != nil {
		return domain.Slice{}, err
	}
	return s.applyPlanStub.AddSlice(ctx, p, n)
}

// stubChanges is a document of one change of each kind, resolved: the edit,
// the move into the plan's own new milestone, the removal with one dependent
// to unhook, and the replacement.
func stubChanges() (plan, planTargets) {
	doc := plan{Milestones: []planMilestone{{Name: "M4"}}, Slices: []planSlice{{Title: "Gone"}}}
	gone := domain.Slice{ID: "gone", Name: "Gone"}
	waiting := domain.Slice{ID: "waiting", Name: "Waiting"}
	targets := planTargets{
		slices: []sliceTarget{{newIndex: 0}},
		changes: planChanges{
			edits:    []resolvedEdit{{slice: domain.Slice{ID: "edited", Name: "Edited"}, brief: "b"}},
			moves:    []resolvedMove{{slice: domain.Slice{ID: "moved", Name: "Moved"}, newIndex: 0}},
			removals: []resolvedRemoval{{slice: gone, dependents: []domain.Slice{waiting}}},
			unhooked: []domain.Slice{waiting},
		},
	}
	return doc, targets
}

// Edits, then the milestones a move may name, then moves, then the dropped
// waits and the removals, and only then the creations.
func TestApplyPlanWritesTheChangesBeforeCreating(t *testing.T) {
	st := &changesStub{}
	doc, targets := stubChanges()

	applied, err := applyPlan(context.Background(), st, store.Project{}, store.Shape{}, doc, targets, nil)
	if err != nil {
		t.Fatalf("applyPlan: %v", err)
	}

	want := []string{"edit edited", "milestones", "move moved", "depends waiting", "delete gone", "create Gone"}
	if !reflect.DeepEqual(st.calls, want) {
		t.Errorf("calls = %v, want %v", st.calls, want)
	}
	if len(applied.Moved) != 1 || applied.Moved[0].Milestone.Name != "M4" {
		t.Errorf("moved = %+v, want the move into the new milestone", applied.Moved)
	}
}

// A failure partway through the changes stops the run and says what already
// stands.
func TestApplyPlanReportsAFailedChange(t *testing.T) {
	for _, tc := range []struct{ failOn, want string }{
		{"edit edited", `edit "Edited": notion is down`},
		{"move moved", `move "Moved": notion is down — of the slices already on the board, 1 edited, 0 moved and 0 removed before this failed, and that stands — 1 milestone and 0 slices were created`},
		{"depends waiting", `drop what "Waiting" waited on of the slices the plan removes: notion is down`},
		{"delete gone", `remove "Gone": notion is down — of the slices already on the board, 1 edited, 1 moved and 0 removed`},
		{"create Gone", `notion is down — of the slices already on the board, 1 edited, 1 moved and 1 removed`},
	} {
		t.Run(tc.failOn, func(t *testing.T) {
			st := &changesStub{failOn: tc.failOn}
			doc, targets := stubChanges()

			_, err := applyPlan(context.Background(), st, store.Project{}, store.Shape{}, doc, targets, nil)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// A failed milestone write, after the edits, says the edits stand.
func TestApplyPlanReportsAFailedMilestoneWriteAfterEdits(t *testing.T) {
	st := &changesStub{failOn: "milestones"}
	doc, targets := stubChanges()

	_, err := applyPlan(context.Background(), st, store.Project{}, store.Shape{}, doc, targets, nil)

	if err == nil || !strings.Contains(err.Error(), "1 edited, 0 moved and 0 removed before this failed") {
		t.Errorf("err = %v, want the edit already made named", err)
	}
}

// A new project has nothing on its board to change.
func TestPlanProposeRefusesChangesForAWorkspace(t *testing.T) {
	for _, doc := range []string{
		`{"milestones": [{"name": "M1"}], "remove": ["x"]}`,
		`{"milestones": [{"name": "M1"}], "move": [{"slice": "x", "milestone": "M1"}]}`,
		`{"milestones": [{"name": "M1"}], "edit": [{"slice": "x", "description": "y"}]}`,
	} {
		_, _, err := runPropose(t, doc, "--workspace", "ws-1", "--name", "importer")
		if err == nil || !strings.Contains(err.Error(), "there is no project yet for it to change") {
			t.Errorf("doc %s: err = %v, want the lists refused", doc, err)
		}
		assertNothingWritten(t, "ws-1")
	}
}

// A project's proposal carries the three lists through plan-proposal, and
// plan-accept applies them against the plan as it then stands.
func TestPlanProposeRoundTripsChangesThroughAccept(t *testing.T) {
	env, out, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	env.In = strings.NewReader(`{
	  "milestones": [{"name": "M1"}, {"name": "M2"}],
	  "slices": [
	    {"title": "Old", "milestone": "M1"},
	    {"title": "Keep", "milestone": "M1", "depends_on": ["Old"]},
	    {"title": "Wander", "milestone": "M1"}
	  ]
	}`)
	if err := Run(context.Background(), []string{"plan-apply", "--project", id}, env); err != nil {
		t.Fatalf("seed the plan: %v", err)
	}
	doc := `{
	  "slices": [{"title": "Old", "milestone": "M2", "description": "Its replacement."}],
	  "remove": ["Old"],
	  "move": [{"slice": "Wander", "milestone": "M2"}],
	  "edit": [{"slice": "Keep", "description": "Kept, rewritten."}]
	}`
	out.Reset()
	env.In = strings.NewReader(doc)
	if err := Run(context.Background(), []string{"plan-propose", "--project", id}, env); err != nil {
		t.Fatalf("plan-propose --project: %v", err)
	}
	if !strings.Contains(out.String(), "; of the slices already there, 1 edited, 1 moved and 1 removed.") {
		t.Errorf("propose output = %q, want the changes counted", out.String())
	}

	out.Reset()
	if err := Run(context.Background(), []string{"plan-proposal", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("plan-proposal: %v", err)
	}
	var answer proposalAnswer
	if err := json.Unmarshal([]byte(out.String()), &answer); err != nil || answer.Proposal == nil {
		t.Fatalf("answer = %q (%v)", out.String(), err)
	}
	got := answer.Proposal.Plan
	if !reflect.DeepEqual(got.Remove, []string{"Old"}) ||
		!reflect.DeepEqual(got.Move, []planMove{{Slice: "Wander", Milestone: "M2"}}) ||
		!reflect.DeepEqual(got.Edit, []planEdit{{Slice: "Keep", Description: "Kept, rewritten."}}) {
		t.Errorf("proposal plan = %+v, want the three lists as proposed", got)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"plan-accept", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("plan-accept --project: %v", err)
	}
	var accepted planAcceptedJSON
	if err := json.Unmarshal([]byte(out.String()), &accepted); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if accepted.Slices != 1 || accepted.Edited != 1 || accepted.Moved != 1 || accepted.Removed != 1 {
		t.Errorf("accepted = %+v, want one of each", accepted)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"info", "--project", id, "--json"}, env); err != nil {
		t.Fatalf("info: %v", err)
	}
	var info struct {
		Slices []struct {
			Name      string   `json:"name"`
			Milestone string   `json:"milestone_name"`
			DependsOn []string `json:"depends_on"`
		} `json:"slices"`
	}
	if err := json.Unmarshal([]byte(out.String()), &info); err != nil {
		t.Fatalf("info not JSON: %v", err)
	}
	var names []string
	for _, s := range info.Slices {
		names = append(names, s.Name)
		if s.Name == "Keep" && len(s.DependsOn) != 0 {
			t.Errorf("Keep depends on %v, want the wait on the removed slice gone", s.DependsOn)
		}
	}
	if len(names) != 3 {
		t.Errorf("slices = %v, want Keep, Wander and the new Old", names)
	}
}

// The text form of an accept with changes says what it did to the board.
func TestPlanAcceptReportsChangesInText(t *testing.T) {
	env, out, _ := acceptEnv(t)
	id := makeLocalProject(t, env)
	env.In = strings.NewReader(`{"milestones": [{"name": "M1"}], "slices": [{"title": "Old", "milestone": "M1"}]}`)
	if err := Run(context.Background(), []string{"plan-apply", "--project", id}, env); err != nil {
		t.Fatalf("seed the plan: %v", err)
	}
	proposeToProject(t, env, id, `{"remove": ["Old"]}`)
	out.Reset()

	if err := Run(context.Background(), []string{"plan-accept", "--project", id}, env); err != nil {
		t.Fatalf("plan-accept --project: %v", err)
	}
	if !strings.Contains(out.String(), "; of the slices already there, 0 edited, 0 moved and 1 removed.") {
		t.Errorf("output = %q, want the removal counted", out.String())
	}
	dir, _ := stateDir()
	if _, err := os.Stat(filepath.Join(dir, "proposals", id+".json")); !os.IsNotExist(err) {
		t.Errorf("the proposal file should be gone, stat err = %v", err)
	}
}
