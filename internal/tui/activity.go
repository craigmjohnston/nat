package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/logging"
)

// activityInterval is how often the board re-reads how the running agents are
// getting on. It is far shorter than the live read's half a minute because this
// is the reading a star on a row moves with: an agent that has stopped for
// input is asking for the user, and a mark that takes half a minute to settle
// would be answering after they had gone. One scan of the panes plus a screen
// read per agent, all of them local socket calls, is cheap enough to pay for
// twice a second's worth of freshness.
const activityInterval = 2 * time.Second

// activityTick is held as a variable so the tests can pin the watcher quiet
// rather than wait out real intervals.
var activityTick = defaultActivityTick

// defaultActivityTick schedules the next reading.
func defaultActivityTick() tea.Cmd { return tea.Tick(activityInterval, activityTicked) }

// activityTicked turns the timer going off into the prod to take one.
func activityTicked(time.Time) tea.Msg { return activityTickMsg{} }

// The messages the watcher comes back as.
type (
	// activityTickMsg is the periodic prod to re-read the agents' states.
	activityTickMsg struct{}
	// agentActivityMsg carries one reading: how each slice's agent is getting
	// on, as the board draws it.
	agentActivityMsg struct {
		activity map[string]Presence
		err      error
	}
)

// presenceOf turns a reading of the panes into what the board draws from.
//
// An agent read as gone is dropped rather than carried as a state of its own:
// whether there is an agent at all is the live map's answer, and the next live
// read is what takes its star away. Carrying it here would only put a second,
// staler opinion beside that one.
func presenceOf(activity map[string]agent.Activity) map[string]Presence {
	presence := make(map[string]Presence, len(activity))
	for id, a := range activity {
		switch a {
		case agent.ActivityWorking:
			presence[id] = PresenceWorking
		case agent.ActivityWaiting:
			presence[id] = PresenceWaiting
		case agent.ActivityUnknown:
			presence[id] = PresenceUnknown
		}
	}
	return presence
}

// refreshActivity kicks off a reading of the running agents. It is skipped when
// nothing is running: the reading would be empty, and the pane scan behind it
// is not worth taking to find that out.
func (a *App) refreshActivity() tea.Cmd {
	if a.launcher == nil || len(a.live) == 0 {
		return nil
	}
	l := a.launcher
	return func() tea.Msg {
		activity, err := l.Activity()
		if err != nil {
			return agentActivityMsg{err: err}
		}
		return agentActivityMsg{activity: presenceOf(activity)}
	}
}

// startWatch arms the watcher's timer, if there are agents to watch and no
// timer is running already. Exactly one runs at a time, however many agents
// there are and however often the live read lands; it stops itself as soon as
// there is nothing left running, and the next live read to find an agent arms
// it again.
func (a *App) startWatch() tea.Cmd {
	if a.watching || len(a.live) == 0 {
		return nil
	}
	a.watching = true
	return tea.Batch(a.refreshActivity(), activityTick())
}

// activityTicked takes a reading and schedules the next, or lets the timer stop
// where there is no agent left to read.
func (a *App) activityTicked() tea.Cmd {
	if len(a.live) == 0 {
		a.watching = false
		return nil
	}
	return tea.Batch(a.refreshActivity(), activityTick())
}

// activityLoaded takes a reading to the board. A failure is logged and left
// there: it is a background poll taken twice a second, and a toast per failed
// reading would bury the board — the stars simply go on saying what they last
// said, which for an agent still running is true for a while yet.
func (a *App) activityLoaded(msg agentActivityMsg) tea.Cmd {
	if msg.err != nil {
		logging.Error("could not read how the agents are getting on", "error", msg.err)
		return nil
	}
	a.activity = msg.activity
	a.board.SetActivity(a.activity)
	cmd := a.startPulse()
	// The board's rows are drawn into a viewport and cached there, so a reading
	// that is not synced never reaches the screen.
	a.syncBoard()
	return cmd
}
