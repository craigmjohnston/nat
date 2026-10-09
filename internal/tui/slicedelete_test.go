package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// answerConfirm answers the open confirm — "y" or "n" — and feeds the write
// that falls out back through the app, as the runtime would. Both keys submit
// the form, so there is nothing else to press.
func answerConfirm(t *testing.T, a *App, answer string) {
	t.Helper()
	finishForm(t, a, press(a, answer))
}

// deleteOver is deleteSlice of a Todo slice through client, with tmux and git
// standing in.
func deleteOver(client *fakeNotion) tea.Cmd {
	return deleteSlice(store.Over(client), store.Project{}, config.ProjectConfig{}, &fakeLauncher{}, &fakeWorktrees{},
		domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceTodo})
}

func TestDeleteSliceTrashesThePage(t *testing.T) {
	client := &fakeNotion{}

	msg := runMsg(t, deleteOver(client))

	if got := msg.(sliceSavedMsg); got.err != nil || got.note != `Deleted "Info view".` {
		t.Errorf("msg = %+v, want the deleted note", got)
	}
	if !equal(client.trashed, []string{"s5"}) {
		t.Errorf("trashed = %v, want the slice's page", client.trashed)
	}
}

func TestDeleteSliceReportsAFailure(t *testing.T) {
	client := &fakeNotion{trashPage: func(string) error { return errors.New("boom") }}

	msg := runMsg(t, deleteOver(client))

	if got := msg.(sliceSavedMsg); got.err == nil || got.err.Error() != `delete "Info view": delete the slice: boom` {
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
			app := newWriteApp(t, &fakeNotion{})
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
	app := newWriteApp(t, client)
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
	app := newWriteApp(t, client)
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

// workingOn points the test project at a repository, so a slice's worktree
// has somewhere to be found.
func workingOn(a *App, dir string) {
	p := a.cfg.Projects[testProjectID]
	p.WorkingDir = dir
	a.cfg.Projects[testProjectID] = p
}

// A slice in progress opens the confirm, warned of what goes with it; a yes
// stops its agent, trashes the page and discards its worktree and branch.
func TestAppDeleteStopsAndDiscardsASliceInProgress(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(t, client)
	workingOn(app, "/repo")
	tmux := &fakeLauncher{live: map[string]string{"s4": "nat-s4"}}
	app.launcher = tmux
	trees := &fakeWorktrees{}
	newWorktrees = func() Worktrees { return trees }
	t.Cleanup(func() { newWorktrees = func() Worktrees { return &fakeWorktrees{} } })
	app.board.cursor = rowClaimedSlice

	feed(t, app, press(app, "d"))
	view := stripANSI(app.View().Content)
	if !strings.Contains(view, `Delete "Board screen"?`) || !strings.Contains(view, "Its agent is stopped") {
		t.Fatalf("view is missing the in-progress warning:\n%s", view)
	}
	answerConfirm(t, app, "y")

	if !equal(tmux.kills, []string{"nat-s4"}) {
		t.Errorf("kills = %v, want the slice's session", tmux.kills)
	}
	if !equal(client.trashed, []string{"s4"}) {
		t.Errorf("trashed = %v, want the slice's page", client.trashed)
	}
	if want := []worktreeCall{{dir: "/repo", branch: "slice/board-screen"}}; !slices.Equal(trees.discards, want) {
		t.Errorf("discards = %+v, want %+v", trees.discards, want)
	}
}

// An agent that cannot be stopped refuses the delete, before the page is
// touched.
func TestAppDeleteRefusesWhereTheAgentCannotBeStopped(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(t, client)
	app.launcher = &fakeLauncher{live: map[string]string{"s4": "nat-s4"}, killErr: errors.New("permission denied")}
	app.board.cursor = rowClaimedSlice

	feed(t, app, press(app, "d"))
	answerConfirm(t, app, "y")

	if len(client.trashed) != 0 {
		t.Errorf("trashed = %v, want nothing", client.trashed)
	}
	if app.err == nil || !strings.Contains(app.err.Error(), "stop its agent") {
		t.Errorf("err = %v, want the kill's failure", app.err)
	}
}

func TestAppDeleteNeedsASliceUnderTheCursor(t *testing.T) {
	app := newWriteApp(t, &fakeNotion{})
	app.board.cursor = rowActiveMilestone

	press(app, "d")

	if app.form != nil {
		t.Error("a confirm was opened with no slice to delete")
	}
	if !strings.Contains(app.board.confirmText, "Move to a slice") {
		t.Errorf("confirm = %q, want the slice hint", app.board.confirmText)
	}
}
