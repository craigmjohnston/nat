package tui

import (
	"fmt"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/logging"
)

// wheelLines is how far one notch of the wheel scrolls the board — the three
// lines every other terminal program moves.
const wheelLines = 3

// mouseEvent routes one mouse event. The agent's terminal is asked first,
// because it is the half of the window that wants every kind of event; what it
// does not take is the board's.
//
// The mouse only ever arrives while a terminal is beside the board — see
// [App.mouseMode] — which is exactly when the outer terminal has stopped acting
// on clicks itself. So the board handles its own from here: without it, asking
// for the mouse would trade the agent's half of the window for the board's.
func (a *App) mouseEvent(msg tea.MouseMsg) tea.Cmd {
	if cmd, taken := a.viewerMouse(msg); taken {
		return cmd
	}
	if a.screen == screenDiff {
		return a.diffMouse(msg)
	}
	return a.boardMouse(msg)
}

// boardMouse is a mouse event over the plan: the wheel scrolls it, and a left
// click selects the row it lands on — and opens the PR chip when it lands on
// one of those.
//
// It is ignored whenever the keyboard is not the board's either: while the
// wizard, a modal form or an inline prompt is up, the board must not move out
// from under the question being asked.
func (a *App) boardMouse(msg tea.MouseMsg) tea.Cmd {
	if a.screen != screenBoard || a.project == nil ||
		a.onboarding != nil || a.form != nil || a.board.Prompting() {
		return nil
	}
	m := msg.Mouse()
	col, line, ok := a.boardCell(m.X, m.Y)
	if !ok {
		return nil
	}
	switch msg.(type) {
	case tea.MouseWheelMsg:
		a.scrollBoard(wheelDelta(m.Button))
	case tea.MouseClickMsg:
		if m.Button == tea.MouseLeft {
			return a.boardClick(col, line)
		}
	}
	return nil
}

// wheelDelta is the lines a wheel event scrolls by, and nothing at all for the
// sideways notches a tilting wheel sends: the board has no columns to scroll.
func wheelDelta(button tea.MouseButton) int {
	switch button {
	case tea.MouseWheelUp:
		return -wheelLines
	case tea.MouseWheelDown:
		return wheelLines
	}
	return 0
}

// scrollBoard moves the plan under the window. The cursor comes along only as
// far as it has to: it is dragged to the nearest row still on screen, because
// the layout re-syncs the board on every animation frame and would scroll
// straight back to a cursor left behind.
func (a *App) scrollBoard(delta int) {
	h := a.boardVP.Height()
	if delta == 0 || h <= 0 {
		return
	}
	a.boardVP.SetYOffset(a.boardVP.YOffset() + delta)
	a.board.CursorToVisible(a.boardVP.YOffset(), h)
	a.syncBoard()
}

// boardClick selects the row a click landed on, and opens the pull request when
// the cell it landed on is a PR chip. The chip is an OSC 8 hyperlink, which the
// terminal itself would have opened — but not while nat is holding the mouse,
// so the gesture is answered here instead.
func (a *App) boardClick(col, line int) tea.Cmd {
	i, ok := a.board.RowAtLine(line)
	if !ok {
		return nil
	}
	a.board.SelectRow(i)
	a.syncBoard()
	if url, ok := a.board.LinkAt(line, col); ok {
		return openLink(url)
	}
	return nil
}

// boardCell turns a cell of the window into a column of the board and a line of
// the whole plan it draws, and reports whether the event landed on the board at
// all.
func (a *App) boardCell(mx, my int) (col, line int, ok bool) {
	col, row, ok := a.bodyCell(mx, my)
	if !ok || col >= a.boardWidth() {
		return 0, 0, false
	}
	return col, row + a.boardTopLine(), true
}

// bodyCell turns a cell of the window into a column and a row of the body band,
// and reports whether the event landed on that band at all. The band's content
// starts two columns in from the window's edge either way the layout is drawn —
// a border and a column of padding when it is framed, the bare band's own indent
// when it is not — and the line it starts on is the first under the header, plus
// the box's top border where there is one.
func (a *App) bodyCell(mx, my int) (col, row int, ok bool) {
	top := a.headerBandHeight()
	if a.framed() {
		top++
	}
	col, row = mx-framePadX, my-top
	if col < 0 || row < 0 || row >= a.bodyHeight() {
		return 0, 0, false
	}
	return col, row, true
}

// boardTopLine is the line of the plan the body band starts at: whatever the
// viewport is scrolled to, or the plan's own first line when there is no band
// to scroll in and every row is drawn — the same two cases [App.boardView]
// draws.
func (a *App) boardTopLine() int {
	if a.boardVP.Width() <= 0 || a.boardVP.Height() <= 0 {
		return 0
	}
	return a.boardVP.YOffset()
}

// linkOpenedMsg reports a click on the PR chip that could not be handed to the
// desktop. A click that worked says nothing: the browser coming up is the
// answer.
type linkOpenedMsg struct {
	url string
	err error
}

// openLink hands a URL to the desktop, off the update loop: the opener is a
// process, and one that is slow to start would otherwise hold the board.
func openLink(url string) tea.Cmd {
	return func() tea.Msg {
		return linkOpenedMsg{url: url, err: openURL(url)}
	}
}

// openURL runs the opener and leaves it to it: nothing is waited on, since the
// browser it starts outlives the command that asked for it.
func openURL(url string) error { return openerCommand(url).Start() }

// openerCommand is the platform's opener as the process that will be run, held
// as a variable so the tests can stand a harmless one in — the real one opens a
// browser.
var openerCommand = func(url string) *exec.Cmd {
	return exec.Command(agent.URLOpener(), url) //nolint:gosec // the URL is a Notion property of the slice, passed as an argument
}

// linkOpened reports a failed open as a toast, since nothing else would say so.
func (a *App) linkOpened(msg linkOpenedMsg) tea.Cmd {
	if msg.err == nil {
		return nil
	}
	logging.Error("could not open the pull request", "url", msg.url, "error", msg.err)
	return a.showToast(fmt.Sprintf("Could not open %s: %v", msg.url, msg.err), sevError)
}
