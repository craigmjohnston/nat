package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/nudge"
)

// nudgeInterval is how often the board stats the nudge marker — the file every
// mutating nat command touches after a successful Notion write. A stat is
// cheap enough to make every second, and a second is how quickly an agent's
// claim shows up on the board instead of waiting out a poll interval.
const nudgeInterval = time.Second

// The watcher's edges, held as variables so the tests can stand in for them:
// the real ones sleep for a second and stat a file in the home directory.
var (
	nudgeTick = defaultNudgeTick
	nudgeStat = nudge.Stat
)

// defaultNudgeTick schedules the next look at the marker.
func defaultNudgeTick() tea.Cmd { return tea.Tick(nudgeInterval, nudgeTicked) }

// nudgeTicked turns the timer going off into the prod to stat the marker.
func nudgeTicked(time.Time) tea.Msg { return nudgeTickMsg{} }

type (
	// nudgeTickMsg is the periodic prod to look at the marker.
	nudgeTickMsg struct{}
	// nudgeMsg carries the marker's reading: its mtime, or ok false when there
	// is no marker to read — nothing has ever touched it, or the stat failed.
	nudgeMsg struct {
		mtime time.Time
		ok    bool
	}
)

// checkNudge stats the marker off the update loop, the way every other I/O
// here runs: as a command coming back as a message.
func checkNudge() tea.Cmd {
	return func() tea.Msg {
		mtime, ok := nudgeStat()
		return nudgeMsg{mtime: mtime, ok: ok}
	}
}

// nudged takes a reading of the marker to the board, returning the reload a
// moved mtime calls for.
//
// The first reading is a baseline, not news: whatever commands ran before this
// board started are already in the load Init kicked off. And a nudge that
// arrives while a load is in flight is deliberately left unconsumed — the load
// may have started before the write landed — so the next tick sees the mtime
// still moved and refetches then, against a plan the write is certainly in.
func (a *App) nudged(msg nudgeMsg) tea.Cmd {
	if !msg.ok || a.nudgeSeen.Equal(msg.mtime) {
		return nil
	}
	if a.nudgeSeen.IsZero() {
		a.nudgeSeen = msg.mtime
		return nil
	}
	if a.loading {
		return nil
	}
	a.nudgeSeen = msg.mtime
	return a.startLoad()
}
