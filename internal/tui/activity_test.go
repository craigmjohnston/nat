package tui

import (
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
)

// TestPresenceOfAReading pins how a reading of the panes reaches the board: an
// agent that is busy or has stopped says so, one nothing could be read of draws
// as working, and one already gone is left out — liveness is the live map's
// answer, and a second opinion here would only be a staler one.
func TestPresenceOfAReading(t *testing.T) {
	got := presenceOf(map[string]agent.Activity{
		"working": agent.ActivityWorking,
		"waiting": agent.ActivityWaiting,
		"unread":  agent.ActivityUnknown,
		"gone":    agent.ActivityGone,
	})
	want := map[string]Presence{
		"working": PresenceWorking,
		"waiting": PresenceWaiting,
		"unread":  PresenceUnknown,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("presence = %v, want %v", got, want)
	}
}

// A reading reaches the board, so the star on the row it was about settles.
func TestActivityReadingReachesTheBoard(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}
	launcher.activity = map[string]agent.Activity{id: agent.ActivityWaiting}

	drive(t, app, app.refreshLive())

	if got := app.activity[id]; got != PresenceWaiting {
		t.Errorf("activity[%q] = %v, want %v", id, got, PresenceWaiting)
	}
	if got := app.board.activity[id]; got != PresenceWaiting {
		t.Errorf("the board was not told: %v", app.board.activity)
	}
	// A board with nothing moving on it needs no frame timer.
	if app.board.Pulsing() {
		t.Error("a waiting agent should leave nothing pulsing")
	}
}

// The watcher is armed by the live read finding an agent, and only once however
// many readings land.
func TestWatcherArmedOnceByTheLiveRead(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}

	ticks := 0
	restore := activityTick
	activityTick = func() tea.Cmd {
		ticks++
		return nil
	}
	t.Cleanup(func() { activityTick = restore })

	drive(t, app, app.refreshLive())
	if !app.watching {
		t.Fatal("the live read found an agent and did not arm the watcher")
	}
	drive(t, app, app.refreshLive())
	if ticks != 1 {
		t.Errorf("armed the timer %d times, want exactly 1", ticks)
	}
}

// With nothing running there is nothing to watch: no timer is armed, and no
// pane scan is taken to find that out.
func TestWatcherNotArmedWithNoAgents(t *testing.T) {
	app, launcher, _ := launchApp(t)

	drive(t, app, app.refreshLive())

	if app.watching {
		t.Error("armed the watcher with no agent running")
	}
	if launcher.reads != 0 {
		t.Errorf("took %d readings with no agent running, want none", launcher.reads)
	}
}

// The timer stops itself once the last agent has gone, and the next live read
// to find one arms it again.
func TestWatcherStopsWithTheLastAgent(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}
	drive(t, app, app.refreshLive())

	launcher.live = nil
	drive(t, app, app.refreshLive())
	if cmd := app.activityTicked(); cmd != nil {
		t.Error("the tick scheduled another with no agent left")
	}
	if app.watching {
		t.Error("the watcher is still marked running")
	}

	launcher.live = map[string]string{id: session}
	drive(t, app, app.refreshLive())
	if !app.watching {
		t.Error("an agent found again did not re-arm the watcher")
	}
}

// A tick with an agent still running takes a reading and schedules the next.
// The tick is fed in as the message the timer really sends, so the route
// through Update is exercised too and the timer is wired to the loop rather
// than only to a handler a test can reach.
func TestWatcherTickReadsAndReschedules(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}
	launcher.activity = map[string]agent.Activity{id: agent.ActivityWorking}
	drive(t, app, app.refreshLive())

	before := launcher.reads
	ticked := false
	restore := activityTick
	activityTick = func() tea.Cmd {
		ticked = true
		return nil
	}
	t.Cleanup(func() { activityTick = restore })

	_, cmd := app.Update(activityTickMsg{})
	drive(t, app, cmd)

	if launcher.reads != before+1 {
		t.Errorf("readings = %d, want one more than %d", launcher.reads, before)
	}
	if !ticked {
		t.Error("the tick did not schedule the next one")
	}
	if got := app.activity[id]; got != PresenceWorking {
		t.Errorf("activity[%q] = %v, want %v", id, got, PresenceWorking)
	}
}

// A failed reading is kept off the board: the stars go on saying what they last
// said rather than every agent reading as stopped, and nothing is toasted for a
// poll taken twice a second.
func TestActivityReadFailureLeavesTheBoardAlone(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	launcher.live = map[string]string{id: session}
	launcher.activity = map[string]agent.Activity{id: agent.ActivityWaiting}
	drive(t, app, app.refreshLive())

	launcher.activityErr = errors.New("boom")
	drive(t, app, app.activityTicked())

	if got := app.activity[id]; got != PresenceWaiting {
		t.Errorf("activity[%q] = %v, want the last good reading %v", id, got, PresenceWaiting)
	}
	if app.toast != "" {
		t.Errorf("toasted %q for a background reading", app.toast)
	}
}

// An app with no launcher at all — one that never got as far as tmux — takes no
// reading rather than panicking on it.
func TestActivityWithNoLauncher(t *testing.T) {
	app, _, _ := launchApp(t)
	app.live = map[string]string{"s4": "nat-1"}
	app.launcher = nil
	if cmd := app.refreshActivity(); cmd != nil {
		t.Error("read the agents with no launcher to read them from")
	}
}

// The real timer fires the message the tick handler is routed by, which is what
// pins the interval to the loop rather than to a test's stand-in.
func TestDefaultActivityTick(t *testing.T) {
	if _, ok := run(defaultActivityTick())[0].(activityTickMsg); !ok {
		t.Error("the activity tick does not produce its own message")
	}
}
