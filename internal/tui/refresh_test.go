package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/notion"
)

// fixClock stands a stopped clock in for the one the freshness indicator
// reads, so a reading in minutes is a fact rather than a race.
func fixClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := timeNow
	timeNow = func() time.Time { return at }
	t.Cleanup(func() { timeNow = prev })
}

// failAfterLoad makes the client's next query fail, so a refresh on an
// already-loaded board comes back with nothing but the error.
func failAfterLoad(c *loadingClient, err error) {
	c.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) { return nil, err }
}

// bar is the status line as the tmux bar draws it: one line, no styling.
func bar(a *App) string { return stripANSI(a.windowTitle()) }

func TestFreshnessIndicatorStates(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		setup func(a *App)
		want  string
	}{
		{"never loaded", func(a *App) { a.loading, a.syncedAt = false, time.Time{} }, ""},
		{"in flight", func(a *App) { a.loading = true }, "syncing…"},
		{"just synced", func(a *App) { a.syncedAt = now }, "synced just now"},
		{"minutes old", func(a *App) { a.syncedAt = now.Add(-5 * time.Minute) }, "synced 5m ago"},
		{"in flight over a stale plan", func(a *App) {
			a.loading, a.syncedAt = true, now.Add(-time.Hour)
		}, "syncing…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixClock(t, now)
			a := loadedApp(t, newLoadingClient())
			tt.setup(a)

			got := bar(a)
			if tt.want == "" {
				if strings.Contains(got, "synced") || strings.Contains(got, "syncing") {
					t.Errorf("status line = %q, want nothing about freshness", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("status line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefreshKeyShowsInFlightThenLastSynced(t *testing.T) {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	fixClock(t, start)
	client := newLoadingClient()
	a := loadedApp(t, client)
	if got := bar(a); !strings.Contains(got, "synced just now") {
		t.Fatalf("status line = %q, want the first load's timestamp", got)
	}

	// Ten minutes on, the same load reads as ten minutes old, and the refresh
	// key puts a fetch in flight.
	fixClock(t, start.Add(10*time.Minute))
	if got := bar(a); !strings.Contains(got, "synced 10m ago") {
		t.Fatalf("status line = %q, want the age of the plan on screen", got)
	}
	msgs := run(press(a, "r"))
	if got := bar(a); !strings.Contains(got, "syncing…") {
		t.Errorf("status line = %q, want the in-flight indicator", got)
	}

	a.Update(first[projectLoadedMsg](t, msgs))
	if !a.syncedAt.Equal(start.Add(10 * time.Minute)) {
		t.Errorf("syncedAt = %v, want the moment the refresh landed", a.syncedAt)
	}
	if got := bar(a); !strings.Contains(got, "synced just now") {
		t.Errorf("status line = %q, want the refresh's own timestamp", got)
	}
}

func TestFailedRefreshKeepsThePlanAndItsAge(t *testing.T) {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	fixClock(t, start)
	client := newLoadingClient()
	a := loadedApp(t, client)

	fixClock(t, start.Add(2*time.Minute))
	failAfterLoad(client, errors.New("boom"))
	for _, msg := range run(press(a, "r")) {
		a.Update(msg)
	}

	if a.project == nil || len(a.project.Slices) != 2 {
		t.Fatalf("project = %+v, want the plan that was already on screen", a.project)
	}
	// The Done milestone the fake plan is all of comes up collapsed, so what
	// the board draws of it is its summary row.
	if view := stripANSI(a.View().Content); !strings.Contains(view, "Done 1 milestone") {
		t.Errorf("view = %q, want the stale plan still drawn behind the failure", view)
	}
	if a.form != nil || a.screen != screenBoard {
		t.Errorf("form = %v, screen = %v, want the failure reported without a modal", a.form, a.screen)
	}
	if got := bar(a); !strings.Contains(got, "boom") || !strings.Contains(got, "synced 2m ago") {
		t.Errorf("status line = %q, want the error beside the age of what is on screen", got)
	}
	if !a.syncedAt.Equal(start) {
		t.Errorf("syncedAt = %v, want it left at the last load that landed", a.syncedAt)
	}
}

func TestErrorStandsUntilARefreshLands(t *testing.T) {
	fixClock(t, time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	client := newLoadingClient()
	a := loadedApp(t, client)
	a.Update(notionErrMsg{err: errors.New("boom")})

	msgs := run(press(a, "r"))
	if a.err == nil {
		t.Error("the failure should stand while the refresh that may fix it is in flight")
	}
	if got := bar(a); !strings.Contains(got, "boom") {
		t.Errorf("status line = %q, want the error still on it", got)
	}

	a.Update(first[projectLoadedMsg](t, msgs))
	if a.err != nil {
		t.Errorf("err = %v, want the landed refresh to have cleared it", a.err)
	}
	if got := bar(a); strings.Contains(got, "boom") {
		t.Errorf("status line = %q, want the error gone", got)
	}
}

func TestHelpListsTheRefreshKey(t *testing.T) {
	a := NewApp(testConfig(), newLoadingClient())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(a, "?")

	if view := stripANSI(a.View().Content); !strings.Contains(view, "r  refresh") {
		t.Errorf("help = %q, want the refresh key listed", view)
	}
}

func TestANarrowBarDropsTheWishlistCountBeforeTheFreshnessReading(t *testing.T) {
	fixClock(t, time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	a := loadedApp(t, wishlistClient(wishlistItems(3), nil))

	// Room for the chip and one indicator, but not for both.
	got := stripANSI(a.statusLeft(len(stripANSI(a.statusLeft(0))) - 10))
	if !strings.Contains(got, "synced") {
		t.Errorf("status line = %q, want the freshness reading kept", got)
	}
	if strings.Contains(got, "wishlist") {
		t.Errorf("status line = %q, want the wishlist count dropped", got)
	}
}

func TestAgoWords(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Minute, "just now"},
		{0, "just now"},
		{59 * time.Second, "just now"},
		{time.Minute, "1m ago"},
		{90 * time.Second, "1m ago"},
		{59 * time.Minute, "59m ago"},
		{time.Hour, "1h ago"},
		{23 * time.Hour, "23h ago"},
		{24 * time.Hour, "1d ago"},
		{50 * time.Hour, "2d ago"},
	}
	for _, tt := range tests {
		if got := ago(tt.d); got != tt.want {
			t.Errorf("ago(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
