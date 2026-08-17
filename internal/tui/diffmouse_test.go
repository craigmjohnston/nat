package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// diffMouseApp is the app the review screen's mouse tests drive: the diff of a
// handed-back branch on show, in a window with a band to draw it in.
func diffMouseApp(t *testing.T) *App {
	t.Helper()
	app, _, _ := diffApp(t)
	cursorOn(t, app, handedBack)
	app.Update(first[diffLoadedMsg](t, run(press(app, "v"))))
	if app.screen != screenDiff || len(app.diff.files) == 0 {
		t.Fatalf("screen = %v over %d files, want the diff up", app.screen, len(app.diff.files))
	}
	return app
}

// clickDiff sends a left click at a column of the diff and a line of the body,
// scrolled to wherever the viewport is.
func clickDiff(a *App, col, line int) {
	x, y := framePadX+diffListWidth+diffRuleWidth, a.headerBandHeight()+1
	a.Update(tea.MouseClickMsg(tea.Mouse{
		X: x + col, Y: y + line - a.diff.vp.YOffset(), Button: tea.MouseLeft,
	}))
}

// A click on a box's header row folds the file away, and the click after it
// opens the file again: the gesture GitHub's viewed checkbox is.
func TestDiffClickFoldsTheFile(t *testing.T) {
	app := diffMouseApp(t)

	clickDiff(app, 2, app.diff.tops[0])
	if !app.diff.viewedFile(0) {
		t.Error("a click on the header row should fold the file")
	}

	clickDiff(app, 2, app.diff.tops[0])
	if app.diff.viewedFile(0) {
		t.Error("a click on the header row again should open the file")
	}
}

// A click on the footer row folds it too — the row is the other end of the same
// box — and a click on a line of the diff folds nothing, since the line cursor
// is what a comment is left on and a stray click must not move it.
func TestDiffClickAwayFromABoxRow(t *testing.T) {
	app := diffMouseApp(t)
	line := app.diff.line

	clickDiff(app, 2, app.diff.offsets[0])
	if app.diff.viewedFile(0) || app.diff.line != line {
		t.Error("a click on a line of the diff should fold nothing and move nothing")
	}

	footer := app.diff.offsets[0] + len(app.diff.files[0].Lines)
	clickDiff(app, 2, footer)
	if !app.diff.viewedFile(0) {
		t.Error("a click on the footer row should fold the file")
	}
}

// A click outside the diff is nobody's: the file list beside it folds nothing,
// and neither does the header band above it.
func TestDiffClickOutsideTheDiff(t *testing.T) {
	app := diffMouseApp(t)
	for _, tt := range []struct{ x, y int }{
		{framePadX, app.headerBandHeight() + 1}, // the file list
		{framePadX, 0},                          // the header band
	} {
		app.Update(tea.MouseClickMsg(tea.Mouse{X: tt.x, Y: tt.y, Button: tea.MouseLeft}))
		if app.diff.viewedFile(0) {
			t.Errorf("a click at %d,%d folded a file, want it dropped", tt.x, tt.y)
		}
	}
}

// The wheel scrolls the diff, since asking for the mouse at all is what took
// the wheel off the terminal itself; the other buttons are dropped.
func TestDiffWheelScrollsTheDiff(t *testing.T) {
	app := diffMouseApp(t)
	x, y := framePadX+diffListWidth+diffRuleWidth+1, app.headerBandHeight()+2

	app.Update(tea.MouseWheelMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseWheelDown}))
	if app.diff.vp.YOffset() == 0 {
		t.Error("a wheel notch over the diff should scroll it")
	}
	app.Update(tea.MouseWheelMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseWheelUp}))
	if got := app.diff.vp.YOffset(); got != 0 {
		t.Errorf("YOffset = %d back at the top, want 0", got)
	}

	app.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseRight}))
	app.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y}))
	if app.diff.viewedFile(0) {
		t.Error("only a left click folds a file")
	}
}

// The mouse is reported to nat on the review screen and nowhere else it is not
// already routing it, since reporting takes the mouse off the terminal itself.
func TestDiffAsksForTheMouse(t *testing.T) {
	app := diffMouseApp(t)
	if got := app.mouseMode(); got != tea.MouseModeCellMotion {
		t.Errorf("mouseMode = %v on the diff, want the button events", got)
	}
	app.setScreen(screenBoard)
	if got := app.mouseMode(); got != tea.MouseModeNone {
		t.Errorf("mouseMode = %v on the board, want the mouse left where it was", got)
	}
}
