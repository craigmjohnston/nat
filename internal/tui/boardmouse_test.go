package tui

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
)

// boardMouseApp is the app the board's mouse tests drive: a terminal beside the
// board — the only state the mouse is reported in — in a window short enough
// that the plan overflows the band and there is something to scroll.
func boardMouseApp(t *testing.T) *App {
	t.Helper()
	app, _, _ := viewerApp(t)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	if total := boardLines(app); total <= app.boardVP.Height() {
		t.Fatalf("the plan is %d lines in a band of %d, want one that overflows",
			total, app.boardVP.Height())
	}
	return app
}

// boardLines is how many lines the whole plan is drawn on.
func boardLines(a *App) int {
	total := 0
	for _, lines := range a.board.rowLines() {
		total += len(lines)
	}
	return total
}

// boardOrigin is the window cell the board's own first line and column are
// drawn at, measured the way the test can see it rather than from the code
// under test: the layout's own indent, and the line under the header's box.
func boardOrigin(a *App) (x, y int) { return framePadX, a.headerBandHeight() + 1 }

// lineOfRow is the line of the whole plan a row starts on.
func lineOfRow(b *Board, row int) int {
	at := 0
	for i, lines := range b.rowLines() {
		if i == row {
			return at
		}
		at += len(lines)
	}
	return at
}

// clickBoard sends a left click at a column and a line of the plan, scrolled to
// wherever the band is, and returns whatever the click asked for.
func clickBoard(a *App, col, line int) tea.Cmd {
	x, y := boardOrigin(a)
	_, cmd := a.Update(tea.MouseClickMsg(tea.Mouse{
		X: x + col, Y: y + line - a.boardVP.YOffset(), Button: tea.MouseLeft,
	}))
	return cmd
}

// wheelBoard sends one notch of the wheel over the middle of the board.
func wheelBoard(a *App, button tea.MouseButton) {
	x, y := boardOrigin(a)
	a.Update(tea.MouseWheelMsg(tea.Mouse{X: x + 1, Y: y + 1, Button: button}))
}

// A click on the board selects the row it landed on, wherever on the row it
// landed: a wrapped row is one row on all of its lines.
func TestBoardClickSelectsTheRow(t *testing.T) {
	app := boardMouseApp(t)
	const row = 3 // Board screen, which wraps onto a second line

	clickBoard(app, 4, lineOfRow(&app.board, row)+1)

	if got := app.board.Cursor(); got != row {
		t.Errorf("cursor = %d, want row %d, the one the second line belongs to", got, row)
	}
}

// A click off the plan is no selection: past its last line, and either side of
// the columns the board is drawn in.
func TestBoardClickOffThePlanChangesNothing(t *testing.T) {
	app := boardMouseApp(t)
	was := app.board.Cursor()

	for _, at := range [][2]int{
		{4, boardLines(app)},  // under the last row
		{-1, 0},               // the box's left border
		{app.boardWidth(), 0}, // past the board's own columns
		{4, -1},               // above the band, on the header's box
		{4, app.bodyHeight()}, // under the band, on the box's bottom border
	} {
		clickBoard(app, at[0], at[1]+app.boardVP.YOffset())
	}

	if got := app.board.Cursor(); got != was {
		t.Errorf("cursor = %d, want it left on %d", got, was)
	}
}

// The wheel scrolls the plan under the window, three lines a notch, and stops
// at either end rather than running off.
func TestBoardWheelScrollsThePlan(t *testing.T) {
	app := boardMouseApp(t)
	// Where the layout has scrolled the board to bring the cursor's row on
	// screen, which is where the wheel starts from.
	start := app.boardVP.YOffset()

	wheelBoard(app, tea.MouseWheelDown)
	if got := app.boardVP.YOffset(); got != start+wheelLines {
		t.Errorf("offset = %d, want the board scrolled %d lines down from %d", got, wheelLines, start)
	}
	wheelBoard(app, tea.MouseWheelUp)
	if got := app.boardVP.YOffset(); got != start {
		t.Errorf("offset = %d, want the board back at %d", got, start)
	}
	// Far enough up to run out of plan, which stops at the top rather than
	// scrolling past it.
	for range boardLines(app) {
		wheelBoard(app, tea.MouseWheelUp)
	}
	if got := app.boardVP.YOffset(); got != 0 {
		t.Errorf("offset = %d, want the top to be as far up as it goes", got)
	}
	// A tilting wheel's sideways notches are nothing to a board with no columns
	// to scroll.
	for _, button := range []tea.MouseButton{tea.MouseWheelLeft, tea.MouseWheelRight} {
		wheelBoard(app, button)
		if got := app.boardVP.YOffset(); got != 0 {
			t.Errorf("offset = %d after a sideways notch, want it unmoved", got)
		}
	}
}

// The cursor comes along with a scroll as far as it has to: the layout brings
// the cursor's row back on screen every time it re-syncs, so a cursor left
// behind would drag the board back where it was.
func TestBoardWheelKeepsTheCursorOnScreen(t *testing.T) {
	app := boardMouseApp(t)
	app.board.SelectRow(0)
	app.syncBoard()

	wheelBoard(app, tea.MouseWheelDown)
	offset := app.boardVP.YOffset()
	if offset == 0 {
		t.Fatal("the wheel should have scrolled the board")
	}
	top, height := app.board.CursorSpan()
	if top < offset || top+height > offset+app.boardVP.Height() {
		t.Errorf("the cursor's row is lines %d..%d, outside the band %d..%d",
			top, top+height, offset, offset+app.boardVP.Height())
	}
	// The re-sync agrees with where the wheel left the board, so the next
	// animation frame does not scroll it back.
	app.syncBoard()
	if got := app.boardVP.YOffset(); got != offset {
		t.Errorf("offset = %d after a re-sync, want the scroll left at %d", got, offset)
	}
}

// prChipCell is where the PR chip is drawn on the board, found in the plain
// text of the rows rather than through the code that answers the click.
func prChipCell(t *testing.T, b *Board, label string) (col, line int) {
	t.Helper()
	at := 0
	for _, lines := range b.rowLines() {
		for _, l := range lines {
			if i := strings.Index(stripANSI(l), label); i >= 0 {
				return i, at
			}
			at++
		}
	}
	t.Fatalf("no %q chip on the board", label)
	return 0, 0
}

// A click on the PR chip opens the pull request: with nat holding the mouse for
// the agent's terminal, the terminal's own hyperlink handling is gone and the
// app answers the gesture itself. The row is selected too — it is still a click
// on a row.
func TestBoardClickOpensThePRChip(t *testing.T) {
	app := boardMouseApp(t)
	opened := stubOpener(t)
	col, line := prChipCell(t, &app.board, "#1")

	drive(t, app, clickBoard(app, col, line))

	if want := []string{"https://example.test/pr/1"}; !reflect.DeepEqual(*opened, want) {
		t.Errorf("opened %v, want the slice's PR", *opened)
	}
	if s, ok := app.board.SelectedSlice(); !ok || s.Name != "Domain model" {
		t.Errorf("selected %+v, want the clicked row's slice", s)
	}
}

// A click on the same row away from the chip opens nothing: the link is the
// cells it is drawn on, not the row it is on.
func TestBoardClickBesideThePRChipOpensNothing(t *testing.T) {
	app := boardMouseApp(t)
	opened := stubOpener(t)
	col, line := prChipCell(t, &app.board, "#1")

	for _, at := range []int{col - 1, col + 2} {
		if cmd := clickBoard(app, at, line); cmd != nil {
			drive(t, app, cmd)
		}
	}

	if len(*opened) != 0 {
		t.Errorf("opened %v, want nothing beside the chip", *opened)
	}
}

// stubOpener stands a harmless command in for the desktop's opener and returns
// the URLs it is asked for.
func stubOpener(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	was := openerCommand
	openerCommand = func(url string) *exec.Cmd {
		opened = append(opened, url)
		return exec.Command("true")
	}
	t.Cleanup(func() { openerCommand = was })
	return &opened
}

// An open that fails says so on the status line: nothing else would, since a
// click that works is answered by the browser coming up.
func TestBoardReportsAFailedOpen(t *testing.T) {
	app := boardMouseApp(t)
	was := openerCommand
	openerCommand = func(string) *exec.Cmd { return exec.Command("nat-no-such-opener") }
	t.Cleanup(func() { openerCommand = was })

	drive(t, app, openLink("https://example.test/pr/1"))

	if !strings.Contains(app.toast, "Could not open https://example.test/pr/1") {
		t.Errorf("toast = %q, want the failed open reported", app.toast)
	}
	if app.toastSev != sevError {
		t.Errorf("severity = %v, want an error", app.toastSev)
	}
}

// The real opener is the platform's, with the URL as its one argument. It is
// built rather than run: running it would open a browser.
func TestTheRealOpenerIsThePlatformOpener(t *testing.T) {
	cmd := openerCommand("https://example.test/pr/1")
	want := []string{agent.URLOpener(), "https://example.test/pr/1"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
}

// The mouse is ignored wherever the keyboard would be: the board must not move
// out from under an open form, an inline prompt or a screen over it.
func TestBoardMouseIgnoredWhileSomethingElseIsUp(t *testing.T) {
	for name, set := range map[string]func(*App){
		"a screen over the board": func(a *App) { a.screen = screenHelp },
		"a modal form":            func(a *App) { a.form = &LaunchForm{} },
		"an inline prompt":        func(a *App) { a.board.SetPrompt([]string{"launch"}) },
		"the wizard":              func(a *App) { a.onboarding = &Onboarding{} },
		"no plan at all":          func(a *App) { a.project = nil },
	} {
		t.Run(name, func(t *testing.T) {
			app := boardMouseApp(t)
			app.board.SelectRow(0)
			app.syncBoard()
			set(app)

			clickBoard(app, 4, lineOfRow(&app.board, 3))
			wheelBoard(app, tea.MouseWheelDown)

			if got := app.board.Cursor(); got != 0 {
				t.Errorf("cursor = %d, want it left where it was", got)
			}
			if got := app.boardVP.YOffset(); got != 0 {
				t.Errorf("offset = %d, want the board unscrolled", got)
			}
		})
	}
}

// With no band to scroll in — a window too small to have been measured — the
// board draws every row, and a click is counted from its first line.
func TestBoardClickWithNoViewport(t *testing.T) {
	app := boardMouseApp(t)
	app.boardVP.SetHeight(0)

	if got := app.boardTopLine(); got != 0 {
		t.Errorf("top line = %d, want the plan's first", got)
	}
	// Nothing is drawn to click on either, so the event is dropped.
	if cmd := app.boardMouse(tea.MouseClickMsg(tea.Mouse{X: framePadX, Y: 6, Button: tea.MouseLeft})); cmd != nil {
		t.Errorf("cmd = %v, want the click dropped", cmd)
	}
}

// Buttons the board has no use for — the middle and the right — are dropped
// rather than moving the cursor.
func TestBoardIgnoresOtherButtons(t *testing.T) {
	app := boardMouseApp(t)
	app.board.SelectRow(0)
	x, y := boardOrigin(app)

	app.Update(tea.MouseClickMsg(tea.Mouse{
		X: x + 4, Y: y + lineOfRow(&app.board, 3), Button: tea.MouseRight,
	}))

	if got := app.board.Cursor(); got != 0 {
		t.Errorf("cursor = %d, want a right click to have done nothing", got)
	}
}

// A scroll with no band to scroll in, and one of no lines, both do nothing.
func TestScrollBoardWithNothingToScroll(t *testing.T) {
	app := boardMouseApp(t)
	start := app.boardVP.YOffset()

	app.scrollBoard(0)
	if got := app.boardVP.YOffset(); got != start {
		t.Errorf("offset = %d, want no scroll for no notch", got)
	}
	app.boardVP.SetHeight(0)
	app.scrollBoard(wheelLines)
	if got := app.boardVP.YOffset(); got != start {
		t.Errorf("offset = %d, want no scroll in a band of no lines", got)
	}
}

// The lookups the mouse maps a window cell back through, at their edges: a line
// before the plan starts and one after it belong to no row and carry no link.
func TestBoardRowAndLinkOffThePlan(t *testing.T) {
	b := newTestBoard()
	last := 0
	for _, lines := range b.rowLines() {
		last += len(lines)
	}

	for _, line := range []int{-1, last, last + 5} {
		if i, ok := b.RowAtLine(line); ok {
			t.Errorf("RowAtLine(%d) = %d, want no row", line, i)
		}
		if url, ok := b.LinkAt(line, 0); ok {
			t.Errorf("LinkAt(%d, 0) = %q, want no link", line, url)
		}
	}
	// The chip's own line, away from the chip.
	if url, ok := b.LinkAt(0, 0); ok {
		t.Errorf("LinkAt(0, 0) = %q, want no link on the first row", url)
	}
}

// A hyperlink left open runs to the end of the line, and a line with no link on
// it has none anywhere.
func TestHyperlinkAtTheEdges(t *testing.T) {
	const open = "\x1b]8;;https://example.test/pr/1\a"
	for _, tc := range []struct {
		name string
		line string
		col  int
		want string
	}{
		{"unterminated", "ab" + open + "#1", 3, "https://example.test/pr/1"},
		{"before an unterminated link", "ab" + open + "#1", 1, ""},
		{"no link at all", "\x1b[1mplain\x1b[m", 2, ""},
		{"past the line", open + "#1\x1b]8;;\a", 9, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url, ok := hyperlinkAt(tc.line, tc.col)
			if tc.want == "" {
				if ok {
					t.Errorf("hyperlinkAt = %q, want no link", url)
				}
				return
			}
			if !ok || url != tc.want {
				t.Errorf("hyperlinkAt = %q/%v, want %q", url, ok, tc.want)
			}
		})
	}
}

// A band with no lines, and a board with no rows, leave the cursor alone; a
// band too short for the row it lands on takes whichever row its top line
// belongs to, since there is no whole row to offer.
func TestCursorToVisibleEdges(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(20) // narrow enough that rows wrap onto more than one line
	b.SelectRow(3)

	b.CursorToVisible(0, 0)
	if got := b.Cursor(); got != 3 {
		t.Errorf("cursor = %d, want it left alone by a band of no lines", got)
	}

	empty := NewBoard(DefaultStyles())
	empty.CursorToVisible(0, 4)
	if got := empty.Cursor(); got != 0 {
		t.Errorf("cursor = %d, want nothing to move on an empty board", got)
	}

	line := lineOfRow(b, 2) + 1
	b.CursorToVisible(line, 1)
	want, _ := b.RowAtLine(line)
	if got := b.Cursor(); got != want {
		t.Errorf("cursor = %d, want row %d, the one the top line belongs to", got, want)
	}
}

// A band taller than the plan has lines under the last row, and a click on one
// of those points at no row at all.
func TestBoardClickUnderAShortPlan(t *testing.T) {
	app, _, _ := viewerApp(t)
	// Taller than the window the viewer opens in, so the band outruns the plan
	// — the Active section and every row of it, with lines to spare under them.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 36})
	app.board.SelectRow(1)
	app.syncBoard()
	line := boardLines(app)
	if line >= app.bodyHeight() {
		t.Fatalf("the plan fills the band at %d lines of %d, want room under it",
			line, app.bodyHeight())
	}

	clickBoard(app, 4, line)

	if got := app.board.Cursor(); got != 1 {
		t.Errorf("cursor = %d, want it left on the row it was on", got)
	}
}
