package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// activePanelApp is an app of a given window size showing a plan with work in
// flight, which is what the body band splits in two for.
func activePanelApp(width, height int, p domain.Project) *App {
	a := NewApp(testConfig(), newLoadingClient())
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	a.Update(projectLoadedMsg{project: p})
	return a
}

// bodyLines is the body band as bare text: the lines of the window between the
// header's band and the hints row, which is where the panels are drawn.
func bodyLines(a *App) []string {
	lines := strings.Split(stripANSI(a.View().Content), "\n")
	top := a.headerBandHeight()
	return lines[top : top+a.bodyBoxHeight()]
}

// TestAppDrawsTheActiveSectionAsASiblingBox pins the shape: with work in flight
// the body band is two boxes, the Active panel above and the plan below, each
// framed at the window's own edges so no border of the layout sits inside
// another.
func TestAppDrawsTheActiveSectionAsASiblingBox(t *testing.T) {
	a := activePanelApp(80, 24, testProject())

	golden(t, "app-active-panel", a.View().Content)

	lines := bodyLines(a)
	n := a.activeBandHeight()
	if n <= 0 {
		t.Fatalf("the band gave the section %d lines, want a panel of its own", n)
	}
	for i, want := range map[int]string{
		0:              "╭─ Active ",
		n - 1:          "╰",
		n:              "╭",
		len(lines) - 1: "╰",
		len(lines) - 2: "│",
	} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("body line %d = %q, want it to start %q", i, lines[i], want)
		}
	}
	// Every line of the band is one box or the other, drawn at the band's own
	// edges: a border inside another would put one of these characters further in.
	for i, line := range lines {
		if edge := []rune(line)[0]; !strings.ContainsRune("╭│╰", edge) {
			t.Errorf("body line %d = %q, want a box edge at the band's own left", i, line)
		}
	}
	if !strings.Contains(lines[1], "● Board screen") {
		t.Errorf("panel line = %q, want the slice in flight as its entry", lines[1])
	}
	if !strings.Contains(lines[n+1], "Board") {
		t.Errorf("plan line = %q, want the plan's own first row under the panel", lines[n+1])
	}
}

// A plan with nothing in flight has no section to lift out, so the body band is
// one box — exactly the layout there was before there was a panel.
func TestAppWithNothingInFlightDrawsOneBodyBox(t *testing.T) {
	a := activePanelApp(80, 24, windowProject())

	if got := a.activeBandHeight(); got != 0 {
		t.Errorf("the section took %d lines of the band, want none", got)
	}
	lines := bodyLines(a)
	if strings.Contains(strings.Join(lines, "\n"), "Active ─") {
		t.Errorf("the band drew a panel with nothing in flight:\n%s", strings.Join(lines, "\n"))
	}
	tops := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "╭") {
			tops++
		}
	}
	if tops != 1 {
		t.Errorf("the band is %d boxes, want the plan's alone", tops)
	}
}

// Below the framed threshold the section follows every other band and draws
// bare: its heading on a line of its own, its entries under it, and no border
// anywhere.
func TestAppDrawsTheActiveSectionBareBelowTheFramedThreshold(t *testing.T) {
	a := activePanelApp(80, 9, testProject())

	if a.framed() {
		t.Fatal("the window is framed, want one below the threshold")
	}
	view := stripANSI(a.View().Content)
	if strings.ContainsAny(view, "╭╮╰╯") {
		t.Errorf("a bare window drew a border:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	top := a.headerBandHeight()
	if got := strings.TrimSpace(lines[top]); got != activeTitle {
		t.Errorf("line %d = %q, want the section's bare heading", top, got)
	}
	if !strings.Contains(lines[top+1], "● Board screen") {
		t.Errorf("line %d = %q, want the entry under the heading", top+1, lines[top+1])
	}
}

// A band with no room for a second panel draws none, and the board's rows say
// so too: the cursor is never left on an entry nothing draws.
func TestAppDropsTheActiveSectionWithNoRoomForIt(t *testing.T) {
	a := activePanelApp(80, 10, testProject())

	if !a.framed() {
		t.Fatal("the window is not framed, want the smallest framed one")
	}
	if got := a.activeBandHeight(); got != 0 {
		t.Errorf("the section took %d lines, want none in a band this short", got)
	}
	if a.board.ActiveCount() == 0 {
		t.Fatal("nothing is in flight, want a plan the section would have drawn")
	}
	if got := a.board.rows[0].kind; got == rowActive {
		t.Errorf("the board's first row is kind %v, want the plan's own", got)
	}
	if got := stripANSI(a.View().Content); strings.Contains(got, activeDot) {
		t.Errorf("the section was drawn with no room for it:\n%s", got)
	}
}

// The panel scrolls to the entry under the cursor, the way the plan's own box
// scrolls to its row: a plan with more in flight than the band can show still
// puts the cursor where it can be seen.
func TestAppScrollsTheActivePanelToTheCursor(t *testing.T) {
	p := activeProject()
	a := activePanelApp(80, 20, p)
	if a.activeHeight() >= a.board.ActiveHeight() {
		t.Fatalf("the panel holds all %d lines, want one the entries overflow",
			a.board.ActiveHeight())
	}

	// Down to the last entry of the section.
	for range a.board.ActiveCount() - 1 {
		press(a, "j")
	}

	if !strings.Contains(stripANSI(a.activeView()), "Awaiting review") {
		t.Errorf("the panel does not show the entry under the cursor:\n%s",
			stripANSI(a.activeView()))
	}
	if got := lipgloss.Height(a.activeView()); got != a.activeHeight() {
		t.Errorf("the panel is %d lines, want the %d its box has", got, a.activeHeight())
	}
	// And back up to the first, which scrolls it the other way.
	for range a.board.ActiveCount() - 1 {
		press(a, "k")
	}
	if a.activeOffset != 0 {
		t.Errorf("offset = %d, want the panel back at its first entry", a.activeOffset)
	}
}

// A click in the panel selects the entry it landed on, and one in the plan the
// row it landed on: the two panels are one board, and a click on either is a
// click on a row of it.
func TestBoardClickSelectsAnActiveEntry(t *testing.T) {
	app := boardMouseApp(t)
	// The plan's first row, so the click has somewhere to move the cursor from.
	app.board.SelectRow(app.board.activeRowCount())
	app.syncBoard()

	x, y := framePadX, app.headerBandHeight()+1
	app.Update(tea.MouseClickMsg(tea.Mouse{X: x + 1, Y: y + 1, Button: tea.MouseLeft}))

	if got := app.board.Cursor(); got != 0 {
		t.Errorf("cursor = %d, want the entry the click landed on", got)
	}
	if s, ok := app.board.SelectedActive(); !ok || s.Name != "Board screen" {
		t.Errorf("selected %+v (ok=%v), want the entry's own slice", s, ok)
	}
}

// A click on the border between the panels is a click on no row at all.
func TestBoardClickBetweenThePanelsChangesNothing(t *testing.T) {
	app := boardMouseApp(t)
	was := app.board.Cursor()

	x, y := framePadX, app.headerBandHeight()+1
	for _, row := range []int{app.activeHeight(), app.activeBandHeight() - 1} {
		app.Update(tea.MouseClickMsg(tea.Mouse{X: x + 1, Y: y + row, Button: tea.MouseLeft}))
		if got := app.board.Cursor(); got != was {
			t.Errorf("a click on band row %d moved the cursor to %d", row, got)
		}
	}
}

// The wheel is the plan's wherever it lands, and a cursor up in the panel is
// left where it is: that box does not scroll with the plan, so nothing has
// moved out from under it.
func TestBoardWheelOverThePanelLeavesTheCursorAlone(t *testing.T) {
	app := boardMouseApp(t)
	app.board.SelectRow(0)
	app.syncBoard()
	offset := app.boardVP.YOffset()

	x, y := framePadX, app.headerBandHeight()+1
	app.Update(tea.MouseWheelMsg(tea.Mouse{X: x + 1, Y: y + 1, Button: tea.MouseWheelDown}))

	if got := app.board.Cursor(); got != 0 {
		t.Errorf("cursor = %d, want it left on the entry it was on", got)
	}
	if got := app.boardVP.YOffset(); got != offset+wheelLines {
		t.Errorf("offset = %d, want the plan scrolled %d from %d", got, wheelLines, offset)
	}
}

// A screen over the board takes the whole band, panel and all — and putting the
// board back puts the section back, with the cursor exactly where it was left.
func TestAppKeepsTheActiveSectionAcrossAScreenOverTheBoard(t *testing.T) {
	a := activePanelApp(80, 24, testProject())
	was := a.board.Cursor()

	press(a, "?")
	if got := a.activeBandHeight(); got != 0 {
		t.Errorf("the section took %d lines under the help screen, want none", got)
	}
	if got := a.board.rows[0].kind; got != rowActive {
		t.Errorf("the board's first row is kind %v, want the section's rows kept", got)
	}

	press(a, "esc")
	if got := a.board.Cursor(); got != was {
		t.Errorf("cursor = %d, want it back where it was on %d", got, was)
	}
	if a.activeBandHeight() <= 0 {
		t.Error("the section did not come back with the board")
	}
}

// Scrolling the plan away from a cursor below the band drags the cursor up with
// it, the same way scrolling past one above drags it down: the layout re-syncs
// to the cursor's row on every frame, so a cursor left behind would scroll the
// board straight back.
func TestBoardWheelUpDragsTheCursorWithIt(t *testing.T) {
	app := boardMouseApp(t)
	app.board.SelectRow(len(app.board.rows) - 1)
	app.syncBoard()
	was := app.board.Cursor()

	wheelBoard(app, tea.MouseWheelUp)

	if got := app.board.Cursor(); got >= was {
		t.Errorf("cursor = %d, want it dragged up from %d", got, was)
	}
	top, height := app.board.CursorSpan()
	offset := app.boardVP.YOffset()
	if top < offset || top+height > offset+app.boardVP.Height() {
		t.Errorf("the cursor's row is lines %d..%d, outside the band %d..%d",
			top, top+height, offset, offset+app.boardVP.Height())
	}
}

// A bare window reports no mouse of its own — nat only asks for it with an
// agent terminal on show, which wants a framed one — but the section's heading
// is a line of the band there rather than a title in a border, so a click that
// does arrive is read past it.
func TestBoardClickOnABareActiveSection(t *testing.T) {
	a := activePanelApp(80, 9, testProject())
	a.board.SelectRow(a.board.activeRowCount())
	if a.framed() {
		t.Fatal("the window is framed, want one below the threshold")
	}

	a.Update(tea.MouseClickMsg(tea.Mouse{
		X: framePadX, Y: a.headerBandHeight() + 1, Button: tea.MouseLeft,
	}))

	if s, ok := a.board.SelectedActive(); !ok || s.Name != "Board screen" {
		t.Errorf("selected %+v (ok=%v), want the entry under the bare heading", s, ok)
	}
	// The heading itself is no entry, and neither is the line under the last.
	for _, row := range []int{0, a.activeBandHeight()} {
		if got := a.activeLineAt(row); got >= 0 && got < a.board.ActiveHeight() {
			t.Errorf("band row %d is entry line %d, want none", row, got)
		}
	}
}
