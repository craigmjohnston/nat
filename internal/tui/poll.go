package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// The poll's edge, held as a variable so the tests can stand in for it: the
// real one sleeps for the configured interval.
var pollTick = defaultPollTick

// defaultPollTick schedules the next refetch of the plan.
func defaultPollTick(d time.Duration) tea.Cmd { return tea.Tick(d, pollTicked) }

// pollTicked turns the timer going off into the prod to refetch.
func pollTicked(time.Time) tea.Msg { return pollTickMsg{} }

// pollTickMsg is the periodic prod to refetch the plan.
type pollTickMsg struct{}

// polled is what a tick of the background poll does: refetch the plan, so that
// a change made in Notion itself — where no nat command ran to touch the nudge
// marker — reaches the board within an interval rather than waiting on the
// refresh key.
//
// It is the same load the refresh key starts, so it keeps the plan on screen
// while it is in flight and a failure leaves the board as it was, with the
// failure passing as a toast. Nothing is polled while the board is not the
// user's to reload out from under them — see [App.pollSuspended]; the tick
// still runs, so the poll resumes by itself on the first tick after.
func (a *App) polled() tea.Cmd {
	if a.pollSuspended() {
		return nil
	}
	return a.startLoad()
}

// pollSuspended reports whether this tick should pass without a refetch: while
// the wizard is up there is no board to refresh, and while a form, a row prompt
// or a write is in flight the reload would land on top of it — a completed form
// writes against the slice the plan held when it opened, and a landing plan
// closes the prompt anchored to a row it may no longer have. A load already in
// flight is left to finish rather than raced by a second.
func (a *App) pollSuspended() bool {
	return a.onboarding != nil || a.form != nil || a.busy || a.loading || a.board.Prompting()
}
