package tui

import (
	"errors"
	"strings"
	"testing"
)

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

func TestAppDeleteAsksAboutTheSelectedSlice(t *testing.T) {
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
			app := newSizedWriteApp(&fakeNotion{})
			app.board.cursor = tt.cursor

			feed(t, app, press(app, "d"))

			if app.prompt == nil || app.screen != screenBoard {
				t.Fatalf("screen = %v, prompt = %v, want the question asked over the board",
					app.screen, app.prompt)
			}
			// The board is still there behind the question, which is the point of
			// asking it on the bar.
			view := stripANSI(app.View().Content)
			for _, want := range []string{tt.want, "(y/n)", "1 ▸ Config"} {
				if !strings.Contains(view, want) {
					t.Errorf("view is missing %q:\n%s", want, view)
				}
			}
			if got := strings.Contains(view, "It is Done"); got != tt.wantWarned {
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
	answerPrompt(t, app, "y")

	if app.prompt != nil {
		t.Error("the question should be gone once it is answered")
	}
	if !equal(client.trashed, []string{"s5"}) {
		t.Fatalf("trashed = %v, want the slice's page", client.trashed)
	}
	if app.note != `Deleted "Info view".` {
		t.Errorf("note = %q, want the deleted note", app.note)
	}
}

func TestAppDeleteTrashesNothingWhenTheAnswerIsNo(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "d"))
	answerPrompt(t, app, "n")

	if app.prompt != nil {
		t.Error("the question should be gone once it is answered")
	}
	if len(client.trashed) != 0 {
		t.Errorf("trashed = %v, want nothing deleted", client.trashed)
	}
	if app.busy {
		t.Error("a question answered no leaves no write in flight")
	}
	if app.note != "Cancelled." {
		t.Errorf("note = %q, want the cancelled note", app.note)
	}
}

func TestAppDeleteRefusesAClaimedSlice(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowClaimedSlice

	press(app, "d")

	if app.prompt != nil {
		t.Error("a claimed slice should not be asked about")
	}
	if want := `"Board screen" is Claimed — work in flight cannot be deleted.`; app.note != want {
		t.Errorf("note = %q, want %q", app.note, want)
	}
}

func TestAppDeleteNeedsASliceUnderTheCursor(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone

	press(app, "d")

	if app.prompt != nil {
		t.Error("a question was asked with no slice to delete")
	}
	if !strings.Contains(app.note, "Move to a slice") {
		t.Errorf("note = %q, want the slice hint", app.note)
	}
}
