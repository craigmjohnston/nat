package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/actions"
)

// severity is how loudly a confirmation or toast speaks: an action that
// worked, one that was refused, and one that failed. Each maps to the theme
// token of the same name. It is [actions.Severity] under this package's own
// name, since the launch and approve flows report their toasts in it too.
type severity = actions.Severity

const (
	sevSuccess = actions.SevSuccess
	sevWarning = actions.SevWarning
	sevError   = actions.SevError
)

// The dismissal timers' messages. Each carries the id its toast or confirm was
// shown with, so a timer that outlives its message — a newer one has replaced
// it — dismisses nothing.
type (
	toastGoneMsg   struct{ id int }
	confirmGoneMsg struct{ id int }
)

// dismissDuration is how long a toast or an inline confirmation stays up. Long
// enough to read, short enough that the interface does not feel littered. A
// variable so the test of the timer itself need not sit the whole wait out.
var dismissDuration = 4 * time.Second

// dismissAfter schedules a dismissal message, held as a variable so the tests
// can fire it without the wait.
var dismissAfter = func(msg tea.Msg) tea.Cmd {
	return tea.Tick(dismissDuration, func(time.Time) tea.Msg { return msg })
}

// showToast puts a message on the status bar for events not scoped to a row —
// project switches, background failures — returning the timer that takes it
// down again.
func (a *App) showToast(text string, sev severity) tea.Cmd {
	a.toastID++
	a.toast, a.toastSev = text, sev
	return dismissAfter(toastGoneMsg{id: a.toastID})
}

// showConfirm anchors a confirmation to the board row the cursor is on — the
// row the action it reports was about — returning the timer that takes it down
// again.
func (a *App) showConfirm(text string, sev severity) tea.Cmd {
	a.confirmID++
	a.board.SetConfirm(text, sev)
	a.syncBoard()
	return dismissAfter(confirmGoneMsg{id: a.confirmID})
}

// toastGone dismisses the toast a timer was started for, unless a newer one
// has taken its place.
func (a *App) toastGone(msg toastGoneMsg) {
	if msg.id == a.toastID {
		a.toast = ""
	}
}

// confirmGone dismisses the inline confirmation the same way.
func (a *App) confirmGone(msg confirmGoneMsg) {
	if msg.id == a.confirmID {
		a.board.ClearConfirm()
		a.syncBoard()
	}
}
