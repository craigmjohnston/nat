package tui

import (
	"errors"
	"strings"
	"testing"
)

// answerConfirm answers the open confirm — "y" or "n" — and feeds the write
// that falls out back through the app, as the runtime would. Both keys submit
// the form, so there is nothing else to press.
func answerConfirm(t *testing.T, a *App, answer string) {
	t.Helper()
	finishForm(t, a, press(a, answer))
}

func TestDeleteSliceTrashesThePage(t *testing.T) {
	client := &fakeNotion{}

	msg := runMsg(t, deleteSlice(client, "s5", "Info view"))

	if got := msg.(sliceSavedMsg); got.err != nil || got.note != `Deleted "Info view".` {
		t.Errorf("msg = %+v, want the deleted note", got)
	}
	if !equal(client.trashed, []string{"s5"}) {
		t.Errorf("trashed = %v, want the slice's page", client.trashed)
	}
}

func TestDeleteSliceReportsAFailure(t *testing.T) {
	client := &fakeNotion{trashPage: func(string) error { return errors.New("boom") }}

	msg := runMsg(t, deleteSlice(client, "s5", "Info view"))

	if got := msg.(sliceSavedMsg); got.err == nil || got.err.Error() != "delete slice: boom" {
		t.Errorf("err = %v, want the wrapped failure", got.err)
	}
}

func TestAppDeleteOpensTheConfirmOnTheSelectedSlice(t *testing.T) {
	tests := []struct {
		name       string
		cursor     int
		want       string
		wantWarned bool
	}{
		{"todo", rowTodoSlice, `Delete "Info view"?`, false},
		// Finished work can be deleted, but not without being told what it is.
		{"done", rowDoneSlice, `Delete "Domain model"?`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWriteApp(&fakeNotion{})
			app.board.cursor = tt.cursor

			feed(t, app, press(app, "d"))

			if app.form == nil || app.screen != screenForm {
				t.Fatalf("screen = %v, form = %v, want the confirm on show", app.screen, app.form)
			}
			view := stripANSI(app.View().Content)
			if !strings.Contains(view, "Delete a slice") || !strings.Contains(view, tt.want) {
				t.Errorf("view is missing %q:\n%s", tt.want, view)
			}
			if got := strings.Contains(view, "WARNING"); got != tt.wantWarned {
				t.Errorf("warned = %v, want %v:\n%s", got, tt.wantWarned, view)
			}
		})
	}
}

func TestAppDeleteTrashesTheConfirmedSlice(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "d"))
	answerConfirm(t, app, "y")

	if app.screen != screenBoard {
		t.Error("the board should be back once the confirm is answered")
	}
	if !equal(client.trashed, []string{"s5"}) {
		t.Fatalf("trashed = %v, want the slice's page", client.trashed)
	}
	if app.board.confirmText != `Deleted "Info view".` {
		t.Errorf("confirm = %q, want the deleted confirmation", app.board.confirmText)
	}
}

func TestAppDeleteTrashesNothingWhenTheAnswerIsNo(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "d"))
	answerConfirm(t, app, "n")

	if len(client.trashed) != 0 {
		t.Errorf("trashed = %v, want nothing deleted", client.trashed)
	}
	if app.busy {
		t.Error("a confirm answered no leaves no write in flight")
	}
	if app.toast != "Cancelled." {
		t.Errorf("toast = %q, want the cancelled toast", app.toast)
	}
}

func TestAppDeleteRefusesAClaimedSlice(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowClaimedSlice

	press(app, "d")

	if app.form != nil {
		t.Error("a claimed slice should not be opened for deletion")
	}
	if want := `"Board screen" is In progress — work in flight cannot be deleted.`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
}

func TestAppDeleteNeedsASliceUnderTheCursor(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone

	press(app, "d")

	if app.form != nil {
		t.Error("a confirm was opened with no slice to delete")
	}
	if !strings.Contains(app.board.confirmText, "Move to a slice") {
		t.Errorf("confirm = %q, want the slice hint", app.board.confirmText)
	}
}
