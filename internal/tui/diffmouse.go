package tui

import (
	tea "charm.land/bubbletea/v2"
)

// diffMouse is a mouse event over the review screen: a left click on a box's
// own header or footer row folds that file away, or unfolds it again, which is
// the gesture GitHub's viewed checkbox is — and the wheel scrolls the diff,
// since asking for the mouse at all is what took that off the terminal itself.
//
// Everything else is dropped. A click on a line of the diff moves nothing: the
// line cursor is what a comment is left on, and a stray click landing in a file
// the user is part way through reading would take their place in it.
func (a *App) diffMouse(msg tea.MouseMsg) tea.Cmd {
	m := msg.Mouse()
	switch msg.(type) {
	case tea.MouseWheelMsg:
		return a.diff.Update(msg)
	case tea.MouseClickMsg:
		if m.Button != tea.MouseLeft {
			return nil
		}
		if line, ok := a.diffCell(m.X, m.Y); ok {
			a.diff.ToggleViewedAt(line)
		}
	}
	return nil
}

// diffCell turns a cell of the window into a line of the rendered diff, and
// reports whether the event landed on the diff at all — the file list beside it
// and the bands around it are not it.
func (a *App) diffCell(mx, my int) (line int, ok bool) {
	col, row, ok := a.bodyCell(mx, my)
	if !ok {
		return 0, false
	}
	return a.diff.LineAt(col, row)
}
