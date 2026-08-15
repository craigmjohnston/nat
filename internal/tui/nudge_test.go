package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// stubNudgeStat pins the marker's reading for a test, putting the quiet
// TestMain stub back afterwards.
func stubNudgeStat(t *testing.T, mtime time.Time, ok bool) {
	t.Helper()
	prev := nudgeStat
	nudgeStat = func() (time.Time, bool) { return mtime, ok }
	t.Cleanup(func() { nudgeStat = prev })
}

// nudgeApp is an app with its first plan landed: the state a board spends its
// life in, and the one a nudge finds it in.
func nudgeApp(t *testing.T) *App {
	t.Helper()
	app := loadedApp(t, newLoadingClient())
	if app.loading {
		t.Fatal("the app should not be loading once the plan has landed")
	}
	return app
}

func TestNudgeTickedProdsTheWatcher(t *testing.T) {
	if _, ok := nudgeTicked(time.Time{}).(nudgeTickMsg); !ok {
		t.Error("nudgeTicked should produce the tick message")
	}
	if defaultNudgeTick() == nil {
		t.Error("the real tick should schedule something")
	}
}

// The tick stats the marker and schedules the next look, the same rhythm the
// live-session poll runs on.
func TestNudgeTickStatsTheMarkerAndReschedules(t *testing.T) {
	prev := nudgeTick
	nudgeTick = func() tea.Cmd { return func() tea.Msg { return nudgeTickMsg{} } }
	t.Cleanup(func() { nudgeTick = prev })
	at := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	stubNudgeStat(t, at, true)
	app := nudgeApp(t)

	_, cmd := app.Update(nudgeTickMsg{})

	msgs := run(cmd)
	if got := first[nudgeMsg](t, msgs); !got.ok || !got.mtime.Equal(at) {
		t.Errorf("reading = %+v, want the stubbed marker", got)
	}
	first[nudgeTickMsg](t, msgs)
}

// The first reading is a baseline, not news: whatever ran before the board
// started is already in the load Init kicked off.
func TestTheFirstNudgeReadingIsABaseline(t *testing.T) {
	app := nudgeApp(t)
	at := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	_, cmd := app.Update(nudgeMsg{mtime: at, ok: true})

	if cmd != nil {
		t.Error("the first reading should start no load")
	}
	if !app.nudgeSeen.Equal(at) {
		t.Errorf("nudgeSeen = %v, want the baseline %v", app.nudgeSeen, at)
	}
}

// A moved mtime is a command having written: the plan on show is out of date,
// and the board refetches it at once.
func TestAMovedMarkerReloadsThePlan(t *testing.T) {
	app := nudgeApp(t)
	base := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	app.Update(nudgeMsg{mtime: base, ok: true})

	moved := base.Add(time.Second)
	_, cmd := app.Update(nudgeMsg{mtime: moved, ok: true})

	if !app.loading {
		t.Error("a moved marker should start a reload")
	}
	first[projectLoadedMsg](t, run(cmd))
	if !app.nudgeSeen.Equal(moved) {
		t.Errorf("nudgeSeen = %v, want the moved mtime %v", app.nudgeSeen, moved)
	}
}

// An unmoved marker, and no marker at all, are both nothing to act on.
func TestAQuietMarkerStartsNothing(t *testing.T) {
	app := nudgeApp(t)
	base := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	app.Update(nudgeMsg{mtime: base, ok: true})

	if _, cmd := app.Update(nudgeMsg{mtime: base, ok: true}); cmd != nil {
		t.Error("an unmoved mtime should start no load")
	}
	if _, cmd := app.Update(nudgeMsg{ok: false}); cmd != nil {
		t.Error("a missing marker should start no load")
	}
	if !app.nudgeSeen.Equal(base) {
		t.Errorf("nudgeSeen = %v, want it untouched at %v", app.nudgeSeen, base)
	}
}

// A nudge that lands while a load is in flight is left for the next tick: the
// load may have started before the write did, so it cannot be trusted to carry
// it — the mtime stays unconsumed and triggers the refetch once the board is
// idle again.
func TestANudgeDuringALoadWaitsForTheNextTick(t *testing.T) {
	app := nudgeApp(t)
	base := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	app.Update(nudgeMsg{mtime: base, ok: true})
	loadCmd := press(app, "r")
	if !app.loading {
		t.Fatal("the refresh should have a load in flight")
	}

	moved := base.Add(time.Second)
	if _, cmd := app.Update(nudgeMsg{mtime: moved, ok: true}); cmd != nil {
		t.Error("a nudge racing a load should start nothing")
	}
	if !app.nudgeSeen.Equal(base) {
		t.Errorf("nudgeSeen = %v, want %v: the nudge is left for the next tick", app.nudgeSeen, base)
	}

	// The load lands, and the next tick's reading finds the mtime still moved.
	app.Update(first[projectLoadedMsg](t, run(loadCmd)))
	_, cmd := app.Update(nudgeMsg{mtime: moved, ok: true})
	if !app.loading {
		t.Error("the reading after the load should start the reload the nudge asked for")
	}
	first[projectLoadedMsg](t, run(cmd))
}

// The watcher starts with the app, beside the live-session poll.
func TestInitSchedulesTheNudgeWatcher(t *testing.T) {
	prev := nudgeTick
	nudgeTick = func() tea.Cmd { return func() tea.Msg { return nudgeTickMsg{} } }
	t.Cleanup(func() { nudgeTick = prev })
	app := NewApp(testConfig(), newLoadingClient())

	first[nudgeTickMsg](t, run(app.Init()))
}
