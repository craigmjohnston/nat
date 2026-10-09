package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// cancelApp is a write app with tmux and git standing in, on a project with a
// repository, its cursor on the slice in progress.
func cancelApp(t *testing.T, tmux *fakeLauncher) (*App, *fakeNotion, *fakeWorktrees) {
	t.Helper()
	client := &fakeNotion{}
	app := newWriteApp(t, client)
	workingOn(app, "/repo")
	app.launcher = tmux
	trees := &fakeWorktrees{}
	newWorktrees = func() Worktrees { return trees }
	t.Cleanup(func() { newWorktrees = func() Worktrees { return &fakeWorktrees{} } })
	app.board.cursor = rowClaimedSlice
	return app, client, trees
}

// X on a slice in progress asks first, saying what goes; a yes stops the
// agent, takes the slice back to Todo and discards its worktree and branch.
func TestAppCancelStopsTheAgentAndDiscardsTheWork(t *testing.T) {
	tmux := &fakeLauncher{live: map[string]string{"s4": "nat-s4"}}
	app, _, trees := cancelApp(t, tmux)

	feed(t, app, press(app, "X"))
	if _, ok := app.form.(*CancelSliceForm); !ok || app.screen != screenForm {
		t.Fatalf("form = %T, want the cancel confirm on show", app.form)
	}
	view := stripANSI(app.View().Content)
	for _, want := range []string{"Cancel a slice", `Cancel "Board screen" and discard its work?`, "its agent is stopped"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	answerConfirm(t, app, "y")

	if !equal(tmux.kills, []string{"nat-s4"}) {
		t.Errorf("kills = %v, want the slice's session", tmux.kills)
	}
	if want := []worktreeCall{{dir: "/repo", branch: "slice/board-screen"}}; !slices.Equal(trees.discards, want) {
		t.Errorf("discards = %+v, want %+v", trees.discards, want)
	}
	if want := `Cancelled "Board screen": back to Todo, its work discarded.`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
	if s, ok := app.board.SelectedSlice(); ok && s.ID == "s4" && s.Status != domain.SliceTodo {
		t.Errorf("slice = %+v, want it back at Todo on the board", s)
	}
}

// A no cancels nothing, and stops nothing.
func TestAppCancelDoesNothingWhenTheAnswerIsNo(t *testing.T) {
	tmux := &fakeLauncher{live: map[string]string{"s4": "nat-s4"}}
	app, _, trees := cancelApp(t, tmux)

	feed(t, app, press(app, "X"))
	answerConfirm(t, app, "n")

	if len(tmux.kills)+len(trees.discards) != 0 || app.busy {
		t.Errorf("kills = %v, discards = %+v, busy = %v, want nothing done", tmux.kills, trees.discards, app.busy)
	}
}

// An agent that cannot be stopped refuses the cancel, before anything is
// written or discarded.
func TestAppCancelRefusesWhereTheAgentCannotBeStopped(t *testing.T) {
	tmux := &fakeLauncher{liveErr: errors.New("no server")}
	app, _, trees := cancelApp(t, tmux)

	feed(t, app, press(app, "X"))
	answerConfirm(t, app, "y")

	if app.err == nil || !strings.Contains(app.err.Error(), "could not read live sessions") {
		t.Errorf("err = %v, want tmux's failure", app.err)
	}
	if len(trees.discards) != 0 {
		t.Errorf("discards = %+v, want none", trees.discards)
	}
}

// Only a slice in progress has anything to cancel; the rest are refused with
// a note, and so is a row that is no slice at all.
func TestAppCancelRefusals(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		want   string
	}{
		{"todo", rowTodoSlice, `"Info view" is Todo — only a slice in progress can be cancelled.`},
		{"done", rowDoneSlice, `"Domain model" is Done — only a slice in progress can be cancelled.`},
		{"no slice", rowActiveMilestone, "Move to a slice to cancel it."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWriteApp(t, &fakeNotion{})
			app.board.cursor = tt.cursor

			press(app, "X")

			if app.form != nil {
				t.Error("a confirm was opened for a slice that cannot be cancelled")
			}
			if app.board.confirmText != tt.want {
				t.Errorf("confirm = %q, want %q", app.board.confirmText, tt.want)
			}
		})
	}
}

// A write already in flight opens nothing.
func TestAppCancelWaitsOnAWriteInFlight(t *testing.T) {
	app := newWriteApp(t, &fakeNotion{})
	app.board.cursor = rowClaimedSlice
	app.busy = true

	if cmd := app.cancelSliceFlow(); cmd != nil || app.form != nil {
		t.Error("a cancel was started over a write already in flight")
	}
}

func TestCancelSliceFormSaveRefusedWhenTheStoreCannotOpen(t *testing.T) {
	a := storeForBrokenApp(t, &fakeNotion{})
	f := &CancelSliceForm{slice: domain.Slice{ID: "s4", Name: "Board screen"}, confirmed: true}
	if cmd := f.save(a); cmd != nil {
		t.Error("want nothing dispatched against a store that cannot open")
	}
}
