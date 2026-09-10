package tui

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// cyclicPlan makes the plan's two board slices wait on each other, which is the
// one wait no landing slice will ever end.
func cyclicPlan(app *App) {
	app.project.Slices[4].DependsOn = []string{"s4"} // Info view waits on Board screen
	app.project.Slices[3].DependsOn = []string{"s5"} // and Board screen waits back
	app.board.SetProject(app.project)
}

// A slice caught in a cycle reads as the cycle rather than as what it waits on:
// naming one unfinished slice would be true and no help at all, since nothing
// in a cycle can finish.
func TestStatusLineNamesACycleAsOne(t *testing.T) {
	app, _, _ := launchApp(t)
	cyclicPlan(app)
	app.board.cursor = rowTodoSlice

	want := "in a dependency cycle: Info view → Board screen → Info view"
	if got := bar(app); !strings.Contains(got, want) {
		t.Errorf("status line = %q, want %q on it", got, want)
	}
	if got := bar(app); strings.Contains(got, "blocked by") {
		t.Errorf("status line = %q, want the cycle instead of the list of blockers", got)
	}
}

// The cycle is read out from whichever row the cursor is on, so what the user is
// looking at is the first step round.
func TestStatusLineReadsTheCycleFromTheSelectedRow(t *testing.T) {
	app, _, _ := launchApp(t)
	cyclicPlan(app)
	app.board.cursor = rowClaimedSlice

	want := "in a dependency cycle: Board screen → Info view → Board screen"
	if got := bar(app); !strings.Contains(got, want) {
		t.Errorf("status line = %q, want %q on it", got, want)
	}
}

// A slice waiting on work that is merely unfinished is no cycle, and the board
// knows of none.
func TestBoardCycleOfASliceInNone(t *testing.T) {
	app, _, _ := launchApp(t)
	app.project.Slices[4].DependsOn = []string{"s4"}
	app.board.SetProject(app.project)

	if got := app.board.CycleOf(app.project.Slices[4]); len(got) != 0 {
		t.Errorf("CycleOf = %v, want nothing", domain.SliceNames(got))
	}
}

// The launch key refuses a cycle in its own words: what the slice waits on is
// itself, and the refusal has to say which dependency to drop or there is
// nothing the user can do about it.
func TestAppLaunchRefusesACycle(t *testing.T) {
	app, launcher, _ := launchApp(t)
	cyclicPlan(app)
	app.board.cursor = rowTodoSlice

	press(app, "l")

	want := `"Info view" is in a dependency cycle: Info view → Board screen → Info view — drop one of those dependencies to unblock it.`
	if app.toast != want {
		t.Errorf("toast = %q, want %q", app.toast, want)
	}
	if app.toastSev != sevWarning {
		t.Errorf("severity = %v, want a warning — nothing has gone wrong", app.toastSev)
	}
	if len(launcher.launches) != 0 {
		t.Errorf("launched %+v, want nothing", launcher.launches)
	}
	if client := app.client.(*fakeNotion); len(client.updated) != 0 {
		t.Errorf("writes = %+v, want the slice left exactly as it was", client.updated)
	}
}
