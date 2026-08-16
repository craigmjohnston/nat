package cli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// dependentSlicePage is a slice waiting on the named ones, the relation written
// the way Notion returns it.
func dependentSlicePage(id, name, status, milestone string, dependsOn ...string) notion.Page {
	p := slicePage(id, name, status, milestone, "", "")
	p.Properties[notion.PropDependsOn] = notion.NewRelation(dependsOn...)
	return p
}

// dependencyIDsOf reads the relation a write put on a page, so a test asserts
// on what was recorded rather than on the shape it was recorded in.
func dependencyIDsOf(t *testing.T, u update) []string {
	t.Helper()
	v, ok := u.props[notion.PropDependsOn]
	if !ok {
		t.Fatalf("update %+v wrote no %s", u, notion.PropDependsOn)
	}
	return v.RelationIDs()
}

// The slices of the dependency plan, named by real Notion IDs because the
// commands that take a slice by name insist on one.
const (
	depDone    = "3b838308f654816da085f46dd135ade1"
	depWaiting = "3b838308f654816da085f46dd135ade2"
	depBlocker = "3b838308f654816da085f46dd135ade3"
	depSpare   = "3b838308f654816da085f46dd135ade4"
	depGone    = "3b838308f654816da085f46dd135ade9"
)

// dependsAPI is a plan of four slices whose second waits on its third, which is
// still Todo — so the plan's next workable slice is that third one.
func dependsAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		blocksByID: map[string][]notion.Block{
			"project-1": conventionBlocks(t),
			depWaiting:  briefBlocks(t, "Render the board, then stop."),
			depBlocker:  briefBlocks(t, "Style the board, then stop."),
			depSpare:    briefBlocks(t, "Queued work, then stop."),
		},
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("M1: Client", "M2: Board", "M3: Later"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePage(depDone, "Notion client", notion.SliceDone, "M1: Client", "Craig Johnston", ""),
				dependentSlicePage(depWaiting, "Render the board", notion.SliceTodo, "M2: Board", depBlocker),
				slicePage(depBlocker, "Style the board", notion.SliceTodo, "M2: Board", "", ""),
				slicePage(depSpare, "Queued work", notion.SliceTodo, "M3: Later", "", ""),
			},
		},
	}
}

// dependsOn rewrites one slice of the plan to wait on the named slices.
func dependsOn(api *fakeAPI, id string, on ...string) {
	pages := api.pages["slices-ds"]
	for i, p := range pages {
		if p.ID == id {
			pages[i].Properties[notion.PropDependsOn] = notion.NewRelation(on...)
		}
	}
}

// next-slice steps over a blocked slice rather than stopping at it: the work
// below it may well be ready, and a plan that halted at the first blocked slice
// would hand out nothing until somebody noticed.
func TestNextSliceSkipsABlockedSlice(t *testing.T) {
	api := dependsAPI(t)
	env, out := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != depBlocker {
		t.Fatalf("updates = %+v, want the unblocked slice claimed", api.updates)
	}
	if !strings.Contains(out.String(), "# Style the board\n") {
		t.Errorf("output =\n%s\nwant the unblocked slice's brief", out.String())
	}
}

// A dependency that is Done is no dependency at all, so the slice waiting on it
// is handed out exactly as it would be without one.
func TestNextSliceHandsOutASliceWhoseDependenciesAreDone(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depWaiting, depDone)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != depWaiting {
		t.Errorf("updates = %+v, want the waiting slice claimed", api.updates)
	}
}

// With every candidate waiting on something, there is nothing to hand out — and
// the refusal names the slices and what each of them waits on, which is the
// whole of what somebody has to look at to unblock the plan.
func TestNextSliceRefusesWhenEveryCandidateIsBlocked(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depBlocker, depSpare)
	dependsOn(api, depSpare, depWaiting)
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"next-slice"}, env)

	if err == nil || !strings.Contains(err.Error(), "is blocked") {
		t.Fatalf("err = %v, want the blocking said plainly", err)
	}
	for _, want := range []string{
		`"Render the board" waits on "Style the board" (Todo)`,
		`"Style the board" waits on "Queued work" (Todo)`,
		`"Queued work" waits on "Render the board" (Todo)`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to say %s", err, want)
		}
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing claimed", api.updates)
	}
}

// A dependency the plan does not hold cannot be read at all — trashed, or filed
// outside this project — and a slice waiting forever on one nobody can see is
// worse than one that goes ahead.
func TestNextSliceIgnoresADependencyThePlanDoesNotHold(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depWaiting, depGone)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"next-slice"}, env); err != nil {
		t.Fatalf("next-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != depWaiting {
		t.Errorf("updates = %+v, want the waiting slice claimed regardless", api.updates)
	}
}

// start-slice was pointed at the slice, so there is nothing to skip: it refuses,
// names what the slice waits on, and writes nothing.
func TestStartSliceRefusesABlockedSlice(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"start-slice", depWaiting}, env)

	if err == nil || !strings.Contains(err.Error(), `"Render the board" waits on 1 unfinished slice`) {
		t.Fatalf("err = %v, want the wait named", err)
	}
	if !strings.Contains(err.Error(), `"Style the board" (Todo)`) {
		t.Errorf("err = %v, want the blocking slice and its status", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

func TestStartSliceClaimsASliceWhoseDependenciesAreDone(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depWaiting, depDone)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"start-slice", depWaiting}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != depWaiting {
		t.Errorf("updates = %+v, want the waiting slice claimed", api.updates)
	}
}

// A dependency whose page cannot be fetched is logged and passed over, for the
// same reason one missing from the plan is.
func TestStartSliceIgnoresAnUnreadableDependency(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depWaiting, depGone)
	env, _ := testEnv(testClaimConfig(), api)

	if err := Run(context.Background(), []string{"start-slice", depWaiting}, env); err != nil {
		t.Fatalf("start-slice: %v", err)
	}

	if len(api.updates) != 1 || api.updates[0].id != depWaiting {
		t.Errorf("updates = %+v, want the waiting slice claimed regardless", api.updates)
	}
}

func TestSliceDependsRecordsWhatASliceWaitsOn(t *testing.T) {
	api := dependsAPI(t)
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depSpare}, env); err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want one write", api.updates)
	}
	// The one already recorded is kept: --on adds, and --clear is what drops.
	if got := dependencyIDsOf(t, api.updates[0]); !reflect.DeepEqual(got, []string{depBlocker, depSpare}) {
		t.Errorf("dependencies = %v, want the old one kept and the new one added", got)
	}
	for _, want := range []string{"# Render the board\n", "Blocked: 2 unfinished of the 2 slices",
		"- Style the board — Todo\n", "- Queued work — Todo\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output =\n%s\nwant %q", out.String(), want)
		}
	}
}

// A slice named twice, or named where it is already recorded, is recorded once:
// the relation is a set, and Notion would keep whatever it was sent.
func TestSliceDependsRecordsEachSliceOnce(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depBlocker, "--on", depSpare, "--on", depSpare}, env)
	if err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	if got := dependencyIDsOf(t, api.updates[0]); !reflect.DeepEqual(got, []string{depBlocker, depSpare}) {
		t.Errorf("dependencies = %v, want each slice once", got)
	}
}

func TestSliceDependsClears(t *testing.T) {
	api := dependsAPI(t)
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-depends", depWaiting, "--clear"}, env); err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	if got := dependencyIDsOf(t, api.updates[0]); len(got) != 0 {
		t.Errorf("dependencies = %v, want none", got)
	}
	if !strings.Contains(out.String(), "Waits on nothing.\n") {
		t.Errorf("output =\n%s\nwant it to say the slice waits on nothing", out.String())
	}
}

// --clear with --on is the list replaced outright, which is the only way to drop
// one dependency and keep another.
func TestSliceDependsClearsBeforeAdding(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--clear", "--on", depSpare}, env)
	if err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	if got := dependencyIDsOf(t, api.updates[0]); !reflect.DeepEqual(got, []string{depSpare}) {
		t.Errorf("dependencies = %v, want only the one just named", got)
	}
}

// Every dependency that is Done is worth saying so: the slice is recorded as
// waiting on them and is not blocked by any of it.
func TestSliceDependsSaysWhenNothingIsOutstanding(t *testing.T) {
	api := dependsAPI(t)
	env, out := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--clear", "--on", depDone}, env)
	if err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	if !strings.Contains(out.String(), "Waits on 1 slice, all Done — it is not blocked.") {
		t.Errorf("output =\n%s\nwant it to say the slice is not blocked", out.String())
	}
}

// A dependency that could not be read is still on the slice in Notion, so it is
// shown as being there rather than quietly dropped from the output.
func TestSliceDependsShowsADependencyItCannotRead(t *testing.T) {
	api := dependsAPI(t)
	dependsOn(api, depWaiting, depGone)
	env, out := testEnv(testConfig(), api)

	if err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depSpare}, env); err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	// The unreadable one is kept rather than quietly dropped — only --on's own
	// arguments are checked — so the output has to account for it.
	if !strings.Contains(out.String(), "- "+depGone+" — could not be read\n") {
		t.Errorf("output =\n%s\nwant the unreadable dependency shown", out.String())
	}
}

func TestSliceDependsJSON(t *testing.T) {
	env, out := testEnv(testConfig(), dependsAPI(t))

	if err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depSpare, "--json"}, env); err != nil {
		t.Fatalf("slice-depends: %v", err)
	}

	var doc dependsJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decoding %s: %v", out.String(), err)
	}
	if doc.Slice.ID != depWaiting || doc.Slice.Name != "Render the board" {
		t.Errorf("slice = %+v, want the slice written to", doc.Slice)
	}
	want := []dependencyJSON{
		{ID: depBlocker, Name: "Style the board", Status: notion.SliceTodo, URL: "https://notion.so/" + depBlocker},
		{ID: depSpare, Name: "Queued work", Status: notion.SliceTodo, URL: "https://notion.so/" + depSpare},
	}
	if !reflect.DeepEqual(doc.Slice.DependsOn, want) {
		t.Errorf("depends_on = %+v, want %+v", doc.Slice.DependsOn, want)
	}
}

// A slice waiting on itself could never be unblocked, and Notion would record
// it happily — so it is refused here, before anything is written.
func TestSliceDependsRefusesSelfDependency(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depWaiting}, env)

	if err == nil || !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Fatalf("err = %v, want the self-dependency refused", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

// A dependency nobody can fetch would be a wait with no end, so naming one is
// refused rather than recorded.
func TestSliceDependsRefusesAnUnreadableSlice(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--on", depGone}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Fatalf("err = %v, want the unreadable dependency named", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written", api.updates)
	}
}

func TestSliceDependsMisuse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no slice", []string{"slice-depends", "--on", depBlocker}, "want exactly one slice"},
		{"two slices", []string{"slice-depends", depWaiting, depBlocker, "--on", depSpare}, "want exactly one slice"},
		{"nothing to record", []string{"slice-depends", depWaiting}, "nothing to record"},
		{"a slice that is not one", []string{"slice-depends", "nope", "--clear"}, "is not a slice"},
		{"a dependency that is not one", []string{"slice-depends", depWaiting, "--on", "nope"}, "is not a slice"},
		{"an unknown flag", []string{"slice-depends", depWaiting, "--nope"}, "slice-depends:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := dependsAPI(t)
			env, _ := testEnv(testConfig(), api)

			err := Run(context.Background(), tt.args, env)

			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("err = %v (%T), want a *UsageError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to say %q", err, tt.want)
			}
			if len(api.updates) != 0 {
				t.Errorf("updates = %+v, want nothing written", api.updates)
			}
		})
	}
}

func TestSliceDependsReportsAFailedWrite(t *testing.T) {
	api := dependsAPI(t)
	api.updateErr = errors.New("notion is down")
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--clear"}, env)

	if err == nil || !strings.Contains(err.Error(), "record the dependencies") {
		t.Errorf("err = %v, want the failing step named", err)
	}
}

func TestSliceDependsReportsAFailedRead(t *testing.T) {
	api := dependsAPI(t)
	api.getErr = errors.New("notion is down")
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--clear"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failing step named", err)
	}
}

// slice-add files the dependency with the slice, so a slice queued behind
// another never exists unblocked even for a moment.
func TestSliceAddRecordsDependencies(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-add", "Polish the board",
		"--milestone", "M2: Board", "--depends-on", depWaiting, "--depends-on", "https://notion.so/Style-the-board"}, env)
	if err == nil || !strings.Contains(err.Error(), "is not a slice") {
		t.Fatalf("err = %v, want the slug-only URL refused", err)
	}

	api = dependsAPI(t)
	env, _ = testEnv(testConfig(), api)
	err = Run(context.Background(), []string{"slice-add", "Polish the board",
		"--milestone", "M2: Board", "--depends-on", "3be38308-f654-81dc-962c-c60836e92992"}, env)
	if err != nil {
		t.Fatalf("slice-add: %v", err)
	}

	if len(api.creates) != 1 {
		t.Fatalf("creates = %+v, want one", api.creates)
	}
	got := api.creates[0].props[notion.PropDependsOn].RelationIDs()
	if !reflect.DeepEqual(got, []string{"3be38308-f654-81dc-962c-c60836e92992"}) {
		t.Errorf("dependencies = %v, want the one named", got)
	}
}

// A slice-add naming no dependency writes no relation at all, so a project
// whose table has no such column takes slices exactly as it always did.
func TestSliceAddWithoutDependenciesWritesNoRelation(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"slice-add", "Polish the board", "--milestone", "M2: Board"}, env)
	if err != nil {
		t.Fatalf("slice-add: %v", err)
	}

	if _, ok := api.creates[0].props[notion.PropDependsOn]; ok {
		t.Errorf("properties = %+v, want no dependency relation", api.creates[0].props)
	}
}

// A plan may name a dependency it creates further down the document, so the
// relations are written after every slice exists to be pointed at.
func TestPlanApplyRecordsDependenciesBetweenNewSlices(t *testing.T) {
	api := planAPI(2)
	doc := `{
	  "milestones": [{"name": "M4: Polish"}],
	  "slices": [
	    {"title": "Frame the board", "milestone": "M4: Polish", "depends_on": ["Colour the chips"]},
	    {"title": "Colour the chips", "milestone": "M4: Polish"}
	  ]
	}`

	if _, err := runPlan(t, api, doc); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	// The creations carry no relation at all: there was no page to point at yet.
	for i, c := range api.creates {
		if _, ok := c.props[notion.PropDependsOn]; ok {
			t.Errorf("creation %d wrote a relation: %+v", i, c.props)
		}
	}
	if len(api.updates) != 1 || api.updates[0].id != "new-1" {
		t.Fatalf("updates = %+v, want the waiting slice written once", api.updates)
	}
	if got := dependencyIDsOf(t, api.updates[0]); !reflect.DeepEqual(got, []string{"new-2"}) {
		t.Errorf("dependencies = %v, want the slice created after it", got)
	}
}

// A dependency may name a slice the project already has, which is what lets a
// plan hang new work off work already queued.
func TestPlanApplyRecordsADependencyOnAnExistingSlice(t *testing.T) {
	api := planAPI(1)
	api.pages = map[string][]notion.Page{
		"slices-ds": {slicePage(depBlocker, "Style the board", notion.SliceTodo, "M2: Board", "", "")},
	}
	doc := `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
		"depends_on": ["style the board"]}]}`

	if _, err := runPlan(t, api, doc); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if got := dependencyIDsOf(t, api.updates[0]); !reflect.DeepEqual(got, []string{depBlocker}) {
		t.Errorf("dependencies = %v, want the existing slice", got)
	}
}

// A plan declaring no dependency reads no slices at all: nothing has to be
// resolved, and a project whose table has no such column applies it as ever.
func TestPlanApplyReadsNoSlicesWithoutDependencies(t *testing.T) {
	api := planAPI(3)

	if _, err := runPlan(t, api, samplePlan); err != nil {
		t.Fatalf("plan-apply: %v", err)
	}

	if len(api.queries) != 0 {
		t.Errorf("queries = %+v, want the slices left unread", api.queries)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want no relation written", api.updates)
	}
}

// Every dependency is resolved before the first write, so a plan naming one
// that does not exist lands nothing at all.
func TestPlanApplyRefusesAnUnresolvableDependency(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "a title nobody has",
			doc: `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
				"depends_on": ["Nothing at all"]}]}`,
			want: `depends on "Nothing at all", which is neither in the plan nor in the project`,
		},
		{
			name: "a title the plan creates twice",
			doc: `{"slices": [
				{"title": "Frame the board", "milestone": "M2: Board", "depends_on": ["Twice"]},
				{"title": "Twice", "milestone": "M2: Board"},
				{"title": "Twice", "milestone": "M2: Board"}
			]}`,
			want: `which the plan creates 2 times`,
		},
		{
			name: "a slice waiting on itself",
			doc: `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
				"depends_on": ["Frame the board"]}]}`,
			want: "depends on itself",
		},
		{
			name: "an empty title",
			doc: `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
				"depends_on": ["  "]}]}`,
			want: "names an empty dependency",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := planAPI(3)

			_, err := runPlan(t, api, tt.doc)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to say %q", err, tt.want)
			}
			if len(api.creates) != 0 {
				t.Errorf("creates = %+v, want nothing written", api.creates)
			}
		})
	}
}

// Two slices of the project sharing a title cannot be told apart by one, so the
// plan is refused with the one edit to make rather than guessed at.
func TestPlanApplyRefusesAnAmbiguousExistingDependency(t *testing.T) {
	api := planAPI(1)
	api.pages = map[string][]notion.Page{
		"slices-ds": {
			slicePage(depBlocker, "Style the board", notion.SliceTodo, "M2: Board", "", ""),
			slicePage(depSpare, "Style the board", notion.SliceTodo, "M2: Board", "", ""),
		},
	}
	doc := `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
		"depends_on": ["Style the board"]}]}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(), "already has 2 slices named") {
		t.Fatalf("err = %v, want the ambiguity refused", err)
	}
	if len(api.creates) != 0 {
		t.Errorf("creates = %+v, want nothing written", api.creates)
	}
}

func TestPlanApplyReportsAFailedDependencyWrite(t *testing.T) {
	api := planAPI(2)
	api.pages = map[string][]notion.Page{
		"slices-ds": {slicePage(depBlocker, "Style the board", notion.SliceTodo, "M2: Board", "", "")},
	}
	api.updateErr = errors.New("notion is down")
	doc := `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
		"depends_on": ["Style the board"]}]}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(), `record what "Frame the board" waits on`) {
		t.Fatalf("err = %v, want the failing step named", err)
	}
	// The slice itself was created, and the error has to say so.
	if !strings.Contains(err.Error(), "still in Notion") {
		t.Errorf("err = %v, want it to say what was already written", err)
	}
}

func TestPlanApplyReportsAFailedSliceRead(t *testing.T) {
	api := planAPI(1)
	api.queryErr = map[string]error{"slices-ds": errors.New("notion is down")}
	doc := `{"slices": [{"title": "Frame the board", "milestone": "M2: Board",
		"depends_on": ["Style the board"]}]}`

	_, err := runPlan(t, api, doc)

	if err == nil || !strings.Contains(err.Error(), "load slices") {
		t.Fatalf("err = %v, want the failing read named", err)
	}
}

// The active project is resolved before anything is read, so a machine that has
// not been set up says so rather than failing on a page fetch.
func TestSliceDependsNeedsAnActiveProject(t *testing.T) {
	api := dependsAPI(t)
	env, _ := testEnv(testConfig(), api)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"slice-depends", depWaiting, "--clear"}, env)

	if err == nil || !strings.Contains(err.Error(), "run `nat` once to set it up") {
		t.Errorf("err = %v, want the unfinished setup named", err)
	}
	if len(api.gets) != 0 {
		t.Errorf("gets = %v, want nothing read", api.gets)
	}
}
