package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// firingPollTick stands a tick that fires at once in for the quiet one TestMain
// pins, recording the interval it was asked to wait. It goes in after the app
// has loaded: a tick that fires as Init returns would have the board polling
// before the test says so.
func firingPollTick(t *testing.T) *time.Duration {
	t.Helper()
	var asked time.Duration
	prev := pollTick
	pollTick = func(d time.Duration) tea.Cmd {
		asked = d
		return func() tea.Msg { return pollTickMsg{} }
	}
	t.Cleanup(func() { pollTick = prev })
	return &asked
}

func TestPollTickedProdsThePoll(t *testing.T) {
	if _, ok := pollTicked(time.Time{}).(pollTickMsg); !ok {
		t.Error("pollTicked should produce the tick message")
	}
	if defaultPollTick(time.Second) == nil {
		t.Error("the real tick should schedule something")
	}
}

// The poll starts with the app, beside the nudge watcher, and at the interval
// the config asks for.
func TestInitSchedulesTheBackgroundPoll(t *testing.T) {
	asked := firingPollTick(t)
	cfg := testConfig()
	cfg.PollSeconds = 90
	app := NewApp(cfg, newLoadingClient())

	first[pollTickMsg](t, run(app.Init()))

	if *asked != 90*time.Second {
		t.Errorf("interval = %v, want the configured 90s", *asked)
	}
}

// A tick refetches the plan and schedules the next one.
func TestAPollTickRefetchesThePlanAndReschedules(t *testing.T) {
	app := loadedApp(t, newLoadingClient())
	asked := firingPollTick(t)

	_, cmd := app.Update(pollTickMsg{})

	if !app.loading {
		t.Error("a poll tick should start a refetch")
	}
	msgs := run(cmd)
	first[projectLoadedMsg](t, msgs)
	first[pollTickMsg](t, msgs)
	if *asked != config.DefaultPollSeconds*time.Second {
		t.Errorf("interval = %v, want the default", *asked)
	}
}

// What the poll brings back replaces the plan on the board in place: the row
// whose status changed in Notion is redrawn, and the cursor stays where the
// user left it.
func TestAPollMergesTheNewPlanIntoTheBoard(t *testing.T) {
	client := newLoadingClient()
	app := loadedApp(t, client)
	app.board.cursor = 1
	before := app.board.View()

	client.query = func(id string, _ map[string]any, _ []notion.Sort) ([]notion.Page, error) {
		if id != "sl-ds" {
			return nil, nil
		}
		return []notion.Page{
			slicePage("s1", "First", notion.SliceDone, "M1"),
			slicePage("s2", "Second", notion.SliceInProgress, "M1"),
		}, nil
	}
	_, cmd := app.Update(pollTickMsg{})
	for _, msg := range run(cmd) {
		app.Update(msg)
	}

	if app.loading {
		t.Error("the board should be idle once the poll has landed")
	}
	if got := app.board.View(); got == before {
		t.Error("the board should be redrawn with what the poll brought back")
	}
	if app.board.cursor != 1 {
		t.Errorf("cursor = %d, want it left at 1", app.board.cursor)
	}
	if s, ok := app.board.SelectedSlice(); !ok || s.Status != domain.SliceClaimed {
		t.Errorf("selected slice = %+v, want the status the poll brought back", s)
	}
}

// A poll that fails leaves the plan on the board as it was, and says so on the
// status line rather than blanking the board.
func TestAFailedPollKeepsThePlan(t *testing.T) {
	client := newLoadingClient()
	app := loadedApp(t, client)
	before := app.board.View()

	client.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
		return nil, errors.New("notion is down")
	}
	_, cmd := app.Update(pollTickMsg{})
	for _, msg := range run(cmd) {
		app.Update(msg)
	}

	if app.project == nil || app.board.View() != before {
		t.Error("a failed poll should leave the plan on the board")
	}
	if app.err != nil {
		t.Errorf("err = %v, want the failure passed as a toast instead", app.err)
	}
	if app.toast == "" || app.toastSev != sevError {
		t.Errorf("toast = %q (sev %v), want the failure reported", app.toast, app.toastSev)
	}
}

// The poll passes over a tick that would land on top of what the user is doing,
// and takes it up again on the next one.
func TestThePollIsSuspendedWhileWorkIsInFlight(t *testing.T) {
	suspended := map[string]func(a *App){
		"a form is open":     func(a *App) { a.form = addForm(a) },
		"a prompt is open":   func(a *App) { a.board.SetPrompt([]string{"yes", "no"}) },
		"a write is running": func(a *App) { a.busy = true },
		"a load is running":  func(a *App) { a.loading = true },
		"the wizard is up":   func(a *App) { a.onboarding = &Onboarding{} },
	}
	for name, suspend := range suspended {
		t.Run(name, func(t *testing.T) {
			app := loadedApp(t, newLoadingClient())
			firingPollTick(t)
			suspend(app)

			_, cmd := app.Update(pollTickMsg{})

			msgs := run(cmd)
			for _, msg := range msgs {
				if _, ok := msg.(projectLoadedMsg); ok {
					t.Error("the poll should start no refetch while work is in flight")
				}
			}
			// The tick still runs, so the poll resumes by itself.
			first[pollTickMsg](t, msgs)
		})
	}
}

// addForm is the add-slice form over the first milestone of the loaded plan:
// something for the suspend tests to have open.
func addForm(a *App) modal {
	return newAddSliceForm(a.styles.FormTheme, *a.project.Groups()[0].Milestone)
}

// Once what suspended it is out of the way, the next tick polls as usual.
func TestThePollResumesOnceTheFormIsClosed(t *testing.T) {
	app := loadedApp(t, newLoadingClient())
	firingPollTick(t)
	app.form = addForm(app)
	app.Update(pollTickMsg{})

	app.closeForm()
	_, cmd := app.Update(pollTickMsg{})

	if !app.loading {
		t.Error("the poll should resume once the form is closed")
	}
	first[projectLoadedMsg](t, run(cmd))
}
