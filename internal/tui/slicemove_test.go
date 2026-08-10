package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The slice rows the move and delete flows act on, beyond those sliceform_test
// already names.
const rowDoneSlice = 2 // Domain model

// milestoneNames is the names of a set of milestones, which is what the picker
// tests assert the order of.
func milestoneNames(ms []domain.Milestone) []string {
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name
	}
	return names
}

// chooseOption moves the open picker down by n options and submits it, feeding
// the write that falls out back through the app.
func chooseOption(t *testing.T, a *App, down int) {
	t.Helper()
	for range down {
		press(a, "j")
	}
	finishForm(t, a, press(a, "enter"))
}

func TestMoveTargetsAreEveryOtherMilestoneInPlanOrder(t *testing.T) {
	p := testProject()
	tests := []struct {
		name  string
		slice domain.Slice
		want  []string
	}{
		{"from a milestone", domain.Slice{MilestoneID: "m2"},
			[]string{"M1: Config", "M3: Mutations"}},
		{"from a milestone not in the plan", domain.Slice{MilestoneID: "gone"},
			[]string{"M1: Config", "M2: Board", "M3: Mutations"}},
		{"from no milestone at all", domain.Slice{},
			[]string{"M1: Config", "M2: Board", "M3: Mutations"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := milestoneNames(moveTargets(&p, tt.slice)); !equal(got, tt.want) {
				t.Errorf("targets = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMoveTargetsOfNoPlanAtAll(t *testing.T) {
	if got := moveTargets(nil, domain.Slice{}); got != nil {
		t.Errorf("targets = %v, want none without a plan", got)
	}
}

func TestMoveSliceWritesOnlyTheRelation(t *testing.T) {
	client := &fakeNotion{}

	msg := runMsg(t, moveSlice(client, "s5", "Info view", "m3", "M3: Mutations"))

	if got := msg.(sliceSavedMsg); got.err != nil || got.note != `Moved "Info view" to M3: Mutations.` {
		t.Errorf("msg = %+v, want the moved note", got)
	}
	if len(client.updated) != 1 {
		t.Fatalf("updated %d pages, want 1", len(client.updated))
	}
	call := client.updated[0]
	want := map[string]notion.PropertyValue{notion.PropMilestone: notion.NewRelation("m3")}
	if call.pageID != "s5" || !reflect.DeepEqual(call.properties, want) {
		t.Errorf("update = %+v, want s5 with %+v", call, want)
	}
}

func TestMoveSliceReportsAFailure(t *testing.T) {
	client := &fakeNotion{
		updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
			return nil, errors.New("boom")
		},
	}

	msg := runMsg(t, moveSlice(client, "s5", "Info view", "m3", "M3: Mutations"))

	if got := msg.(sliceSavedMsg); got.err == nil || got.err.Error() != "move slice: boom" {
		t.Errorf("err = %v, want the wrapped failure", got.err)
	}
}

func TestAppMoveOpensThePickerOnTheSelectedSlice(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		want   string
	}{
		{"todo", rowTodoSlice, "Move Info view"},
		// Finished work still sits somewhere in the plan, so it can be refiled.
		{"done", rowDoneSlice, "Move Domain model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWriteApp(&fakeNotion{})
			app.board.cursor = tt.cursor

			feed(t, app, press(app, "m"))

			if app.form == nil || app.screen != screenForm {
				t.Fatalf("screen = %v, form = %v, want the picker on show", app.screen, app.form)
			}
			view := stripANSI(app.View().Content)
			// Every milestone but the one the slice is already under.
			for _, want := range []string{tt.want, "M1: Config", "M3: Mutations"} {
				if !strings.Contains(view, want) {
					t.Errorf("view is missing %q:\n%s", want, view)
				}
			}
			if strings.Contains(view, "M2: Board") {
				t.Errorf("the picker offers the milestone the slice is already under:\n%s", view)
			}
		})
	}
}

func TestAppMoveWritesThePickedMilestone(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "m"))
	// Past M1: Config, which the picker opens on, to M3: Mutations.
	chooseOption(t, app, 1)

	if app.screen != screenBoard {
		t.Error("the board should be back once the picker is answered")
	}
	if len(client.updated) != 1 || client.updated[0].pageID != "s5" {
		t.Fatalf("updated = %+v, want the slice written", client.updated)
	}
	want := notion.NewRelation("m3")
	if got := client.updated[0].properties[notion.PropMilestone]; !reflect.DeepEqual(got, want) {
		t.Errorf("milestone = %+v, want %+v", got, want)
	}
	if app.note != `Moved "Info view" to M3: Mutations.` {
		t.Errorf("note = %q, want the moved note", app.note)
	}
}

func TestAppMoveRefusesAClaimedSlice(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowClaimedSlice

	press(app, "m")

	if app.form != nil {
		t.Error("a claimed slice should not be opened for moving")
	}
	if want := `"Board screen" is Claimed — work in flight cannot be moved.`; app.note != want {
		t.Errorf("note = %q, want %q", app.note, want)
	}
}

func TestAppMoveNeedsASliceUnderTheCursor(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone

	press(app, "m")

	if app.form != nil {
		t.Error("a picker was opened with no slice to move")
	}
	if !strings.Contains(app.note, "Move to a slice") {
		t.Errorf("note = %q, want the slice hint", app.note)
	}
}

func TestAppMoveNeedsSomewhereToMoveTo(t *testing.T) {
	// A plan of one milestone holding one slice: there is nowhere else to file
	// it.
	p := domain.Project{
		ID:         testProjectID,
		Milestones: []domain.Milestone{{ID: "m1", Name: "M1", Status: domain.MilestoneActive}},
		Slices:     []domain.Slice{{ID: "s1", Name: "Only", Status: domain.SliceTodo, MilestoneID: "m1"}},
	}
	app := NewApp(testConfig(), &fakeNotion{})
	app.project = &p
	app.board.SetProject(&p)
	app.board.cursor = 1

	press(app, "m")

	if app.form != nil {
		t.Error("a picker was opened with nowhere to move to")
	}
	if want := `There is no other milestone to move "Only" to.`; app.note != want {
		t.Errorf("note = %q, want %q", app.note, want)
	}
}
