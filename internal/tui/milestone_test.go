package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The milestone rows of the board testProject flattens to.
const (
	rowDoneSection     = 0 // the Done section, folding M1: Config
	rowQueuedMilestone = 5 // M3: Mutations
)

// answerConfirm answers the open confirm — "y" or "n" — and feeds the write
// that falls out back through the app, as the runtime would. Both keys submit
// the form, so there is nothing else to press.
func answerConfirm(t *testing.T, a *App, answer string) {
	t.Helper()
	finishForm(t, a, press(a, answer))
}

func TestNextMilestoneStatus(t *testing.T) {
	tests := []struct {
		from   domain.MilestoneStatus
		want   domain.MilestoneStatus
		wantOK bool
	}{
		{domain.MilestoneQueued, domain.MilestoneActive, true},
		{domain.MilestoneActive, domain.MilestoneDone, true},
		{domain.MilestoneDone, "", false},
		{"Unknown", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from), func(t *testing.T) {
			got, ok := nextMilestoneStatus(tt.from)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("nextMilestoneStatus(%q) = %q, %v, want %q, %v",
					tt.from, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSetMilestoneStatusWritesTheShapeItWasReadIn(t *testing.T) {
	tests := []struct {
		name       string
		statusType string
		want       notion.PropertyValue
	}{
		{"select", notion.TypeSelect, notion.NewSelect(notion.MilestoneActive)},
		{"status", notion.TypeStatus, notion.NewStatus(notion.MilestoneActive)},
		// A page whose property type did not come back is written the way this
		// app's own schemas are made.
		{"unknown", "", notion.NewSelect(notion.MilestoneActive)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeNotion{}

			msg := runMsg(t, setMilestoneStatus(client, "m3", "M3: Mutations", tt.statusType,
				domain.MilestoneActive))

			if got := msg.(milestoneSavedMsg); got.err != nil || got.note != `"M3: Mutations" is now Active.` {
				t.Errorf("msg = %+v, want the updated note", got)
			}
			if len(client.updated) != 1 {
				t.Fatalf("updated %d pages, want 1", len(client.updated))
			}
			call := client.updated[0]
			want := map[string]notion.PropertyValue{notion.PropStatus: tt.want}
			if call.pageID != "m3" || !reflect.DeepEqual(call.properties, want) {
				t.Errorf("update = %+v, want m3 with %+v", call, want)
			}
		})
	}
}

func TestSetMilestoneStatusReportsAFailure(t *testing.T) {
	client := &fakeNotion{
		updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
			return nil, errors.New("boom")
		},
	}

	msg := runMsg(t, setMilestoneStatus(client, "m3", "M3", "", domain.MilestoneDone))

	if got := msg.(milestoneSavedMsg); got.err == nil || got.err.Error() != "update milestone: boom" {
		t.Errorf("err = %v, want the wrapped failure", got.err)
	}
}

func TestAppQueueOpensTheConfirmOnTheSelectedMilestone(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		want   string
	}{
		{"queued", rowQueuedMilestone, "M3: Mutations — set Active?"},
		{"active", rowActiveMilestone, "M2: Board — set Done?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWriteApp(&fakeNotion{})
			app.board.cursor = tt.cursor

			feed(t, app, press(app, "Q"))

			if app.form == nil || app.screen != screenForm {
				t.Fatalf("screen = %v, form = %v, want the confirm on show", app.screen, app.form)
			}
			view := stripANSI(app.View().Content)
			if !strings.Contains(view, "Milestone") || !strings.Contains(view, tt.want) {
				t.Errorf("view is missing %q:\n%s", tt.want, view)
			}
		})
	}
}

func TestAppQueueWritesTheConfirmedStatus(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowQueuedMilestone

	feed(t, app, press(app, "Q"))
	answerConfirm(t, app, "y")

	if app.screen != screenBoard {
		t.Error("the board should be back once the confirm is answered")
	}
	if len(client.updated) != 1 || client.updated[0].pageID != "m3" {
		t.Fatalf("updated = %+v, want the milestone written", client.updated)
	}
	want := notion.NewSelect(notion.MilestoneActive)
	if got := client.updated[0].properties[notion.PropStatus]; !reflect.DeepEqual(got, want) {
		t.Errorf("status = %+v, want %+v", got, want)
	}
	if app.board.confirmText != `"M3: Mutations" is now Active.` {
		t.Errorf("confirm = %q, want the updated confirmation", app.board.confirmText)
	}
}

func TestAppQueueWritesNothingWhenTheAnswerIsNo(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowQueuedMilestone

	feed(t, app, press(app, "Q"))
	answerConfirm(t, app, "n")

	if len(client.updated) != 0 {
		t.Errorf("updated = %+v, want nothing written", client.updated)
	}
	if app.busy {
		t.Error("a confirm answered no leaves no write in flight")
	}
	if app.toast != "Cancelled." {
		t.Errorf("toast = %q, want the cancelled toast", app.toast)
	}
}

func TestAppQueueRefusesAMilestoneWithNowhereToGo(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.expanded[doneSectionKey] = true
	app.board.rebuild()
	app.board.cursor = 1 // M1: Config, revealed inside the Done section

	press(app, "Q")

	if app.form != nil {
		t.Error("a Done milestone should not open a confirm")
	}
	if want := `"M1: Config" is Done — there is nothing to move it to.`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
}

func TestAppQueueNeedsAMilestoneUnderTheCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
	}{
		{"on a slice", rowTodoSlice},
		{"on the unassigned group", rowUnassigned},
		{"on the Done section", rowDoneSection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWriteApp(&fakeNotion{})
			app.board.cursor = tt.cursor

			press(app, "Q")

			if app.form != nil {
				t.Error("a confirm was opened with no milestone to write to")
			}
			if !strings.Contains(app.board.confirmText, "Move to a milestone") {
				t.Errorf("confirm = %q, want the milestone hint", app.board.confirmText)
			}
		})
	}
}

func TestAppReloadsThePlanAfterAMilestoneWrite(t *testing.T) {
	client := newLoadingClient()
	app := NewApp(testConfig(), client)
	app.busy = true

	_, cmd := app.Update(milestoneSavedMsg{note: `"M3" is now Active.`})

	if app.busy || app.board.confirmText != `"M3" is now Active.` {
		t.Errorf("busy = %v, confirm = %q, want the write finished", app.busy, app.board.confirmText)
	}
	if got := first[projectLoadedMsg](t, run(cmd)); got.project.ID != testProjectID {
		t.Errorf("reloaded %q, want the active project", got.project.ID)
	}
}

func TestAppReportsAFailedMilestoneWrite(t *testing.T) {
	app := newWriteApp(&fakeNotion{})

	app.Update(milestoneSavedMsg{err: errors.New("update milestone: boom")})

	if app.err == nil || app.err.Error() != "update milestone: boom" {
		t.Errorf("err = %v, want the failure reported", app.err)
	}
}
