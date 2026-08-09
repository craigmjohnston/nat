package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/notion"
)

// The project the tests load, and the config that points at it.
const testProjectID = "proj-1"

func testConfig() config.Config {
	return config.Config{
		AssigneeUserName: "Craig Johnston",
		ActiveProjectID:  testProjectID,
		Projects: map[string]config.ProjectConfig{
			testProjectID: {
				Name:           "tracker",
				MilestonesDSID: "ms-ds",
				SlicesDSID:     "sl-ds",
			},
		},
	}
}

// pages builds the query results the load pipeline maps.
func milestonePage(id, name, status string, order float64) notion.Page {
	return notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
		notion.PropName:   notion.NewTitle(name),
		notion.PropStatus: notion.NewSelect(status),
		notion.PropOrder:  notion.NewNumber(order),
	}}
}

func slicePage(id, name, status, milestoneID string) notion.Page {
	return notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
		notion.PropName:      notion.NewTitle(name),
		notion.PropStatus:    notion.NewSelect(status),
		notion.PropMilestone: notion.NewRelation(milestoneID),
	}}
}

// loadingClient answers the two queries the load pipeline makes, recording the
// sorts it was asked for.
type loadingClient struct {
	fakeNotion
	sorts map[string][]notion.Sort
}

func newLoadingClient() *loadingClient {
	c := &loadingClient{sorts: map[string][]notion.Sort{}}
	c.query = func(id string, _ map[string]any, sorts []notion.Sort) ([]notion.Page, error) {
		c.sorts[id] = sorts
		switch id {
		case "ms-ds":
			return []notion.Page{milestonePage("m1", "M1", notion.MilestoneDone, 1)}, nil
		case "sl-ds":
			return []notion.Page{
				slicePage("s1", "First", notion.SliceDone, "m1"),
				slicePage("s2", "Second", notion.SliceTodo, "m1"),
			}, nil
		}
		return nil, nil
	}
	return c
}

// run executes a command the way the runtime would, flattening a batch into
// the messages its commands produce. Nil commands yield nothing.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, run(c)...)
		}
		return msgs
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// first returns the only message of type T among msgs.
func first[T tea.Msg](t *testing.T, msgs []tea.Msg) T {
	t.Helper()
	for _, m := range msgs {
		if got, ok := m.(T); ok {
			return got
		}
	}
	var zero T
	t.Fatalf("no %T among %v", zero, msgs)
	return zero
}

// press feeds a key press to the app.
func press(a *App, s string) tea.Cmd {
	var msg tea.KeyPressMsg
	switch s {
	case "esc":
		msg = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	case "ctrl+c":
		msg = tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	default:
		msg = tea.KeyPressMsg(tea.Key{Code: rune(s[0]), Text: s})
	}
	_, cmd := a.Update(msg)
	return cmd
}

// isQuitCmd reports whether cmd is tea.Quit, which is only identifiable by the
// message it produces.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestAppLoadsTheActiveProject(t *testing.T) {
	client := newLoadingClient()
	app := NewApp(testConfig(), client)

	msgs := run(app.Init())
	if !app.loading {
		t.Error("the app should be loading while the queries are in flight")
	}
	if !strings.Contains(app.View().Content, "Loading the plan") {
		t.Errorf("view = %q, want the loading state", app.View().Content)
	}

	loaded := first[projectLoadedMsg](t, msgs)
	app.Update(loaded)

	if app.loading {
		t.Error("still loading after the plan arrived")
	}
	if app.project == nil || app.project.ID != testProjectID || app.project.Name != "tracker" {
		t.Fatalf("project = %+v, want the configured project", app.project)
	}
	if got := len(app.project.Milestones); got != 1 {
		t.Errorf("milestones = %d, want 1", got)
	}
	view := app.View().Content
	for _, want := range []string{"tracker", "milestones: 1", "slices done: 1/2"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestAppQueriesBothDataSourcesInPlanOrder(t *testing.T) {
	client := newLoadingClient()
	run(NewApp(testConfig(), client).Init())

	want := map[string][]notion.Sort{
		"ms-ds": {{Property: notion.PropOrder, Direction: notion.SortAscending}},
		"sl-ds": {{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}},
	}
	for id, sorts := range want {
		got := client.sorts[id]
		if len(got) != 1 || got[0] != sorts[0] {
			t.Errorf("%s sorted by %+v, want %+v", id, got, sorts)
		}
	}
}

func TestAppReportsAFailedLoad(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		failDS string
		want   string
	}{
		{"milestones", "ms-ds", "load milestones: boom"},
		{"slices", "sl-ds", "load slices: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newLoadingClient()
			inner := client.query
			client.query = func(id string, f map[string]any, s []notion.Sort) ([]notion.Page, error) {
				if id == tt.failDS {
					return nil, boom
				}
				return inner(id, f, s)
			}
			app := NewApp(testConfig(), client)

			msgs := run(app.Init())
			app.Update(first[notionErrMsg](t, msgs))

			if app.loading {
				t.Error("a failed load should stop the spinner")
			}
			if app.err == nil || app.err.Error() != tt.want {
				t.Fatalf("err = %v, want %q", app.err, tt.want)
			}
			if !errors.Is(app.err, boom) {
				t.Error("the underlying error should be wrapped, not swallowed")
			}
			if view := app.View().Content; !strings.Contains(view, "boom") ||
				!strings.Contains(view, "esc to dismiss") {
				t.Errorf("view = %q, want the error status bar", view)
			}
		})
	}
}

func TestAppDismissesAnErrorWithoutQuitting(t *testing.T) {
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(notionErrMsg{err: errors.New("boom")})

	if cmd := press(app, "esc"); isQuitCmd(cmd) {
		t.Fatal("esc should dismiss the error, not quit")
	}
	if app.err != nil {
		t.Errorf("err = %v, want it dismissed", app.err)
	}
	// The next esc has nothing left to dismiss, so it quits.
	if !isQuitCmd(press(app, "esc")) {
		t.Error("esc should quit once the error is gone")
	}
}

func TestAppShowsNothingToLoadWithoutAProject(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"nothing selected", config.Config{}, "No project selected"},
		{"selection not in config", config.Config{ActiveProjectID: "gone"}, "not in the config file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newLoadingClient()
			app := NewApp(tt.cfg, client)

			if cmd := app.Init(); cmd != nil {
				t.Error("there is nothing to load")
			}
			if len(client.queriedDSIDs) != 0 {
				t.Errorf("queried %v, want no Notion calls", client.queriedDSIDs)
			}
			if view := app.View().Content; !strings.Contains(view, tt.want) {
				t.Errorf("view = %q, want %q", view, tt.want)
			}
		})
	}
}

func TestAppWithoutAClientDoesNotLoad(t *testing.T) {
	if cmd := NewApp(testConfig(), nil).Init(); cmd != nil {
		t.Error("there is no client to load with")
	}
}

func TestAppRefreshReloads(t *testing.T) {
	client := newLoadingClient()
	app := NewApp(testConfig(), client)
	run(app.Init())
	app.Update(first[projectLoadedMsg](t, run(app.Init())))
	before := len(client.queriedDSIDs)
	app.note = "Setup complete."

	run(press(app, "r"))

	if len(client.queriedDSIDs) != before+2 {
		t.Errorf("queried %d data sources, want %d more", len(client.queriedDSIDs), before+2)
	}
	if app.note != "" {
		t.Errorf("note = %q, want the stale note cleared on refresh", app.note)
	}
}

func TestAppRefreshWithNothingToLoad(t *testing.T) {
	app := NewApp(config.Config{}, newLoadingClient())
	if cmd := press(app, "r"); cmd != nil {
		t.Error("there is nothing to refresh")
	}
}

func TestAppScreenToggles(t *testing.T) {
	tests := []struct {
		key    string
		want   screen
		marker string
	}{
		{"?", screenHelp, "Keys"},
		{"i", screenInfo, "Info"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			app := NewApp(config.Config{}, nil)

			press(app, tt.key)
			if app.screen != tt.want {
				t.Fatalf("screen = %v, want %v", app.screen, tt.want)
			}
			if view := app.View().Content; !strings.Contains(view, tt.marker) {
				t.Errorf("view = %q, want the %s screen", view, tt.key)
			}
			// The same key toggles back, and so does esc.
			press(app, tt.key)
			if app.screen != screenBoard {
				t.Errorf("screen = %v, want the board back", app.screen)
			}
			press(app, tt.key)
			if cmd := press(app, "esc"); isQuitCmd(cmd) {
				t.Fatal("esc should leave the screen, not quit")
			}
			if app.screen != screenBoard {
				t.Errorf("screen = %v, want the board back", app.screen)
			}
		})
	}
}

func TestAppQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		if !isQuitCmd(press(NewApp(config.Config{}, nil), k)) {
			t.Errorf("%q should quit", k)
		}
	}
}

func TestAppIgnoresUnboundKeys(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	if cmd := press(app, "x"); cmd != nil {
		t.Error("x is not bound to anything")
	}
	if app.screen != screenBoard {
		t.Errorf("screen = %v, want the board", app.screen)
	}
}

func TestAppSpinnerOnlyTicksWhileLoading(t *testing.T) {
	app := NewApp(testConfig(), newLoadingClient())
	tick := spinner.TickMsg{Time: time.Now(), ID: app.spinner.ID()}

	if _, cmd := app.Update(tick); cmd != nil {
		t.Error("an idle app should not schedule another frame")
	}

	app.Init()
	if _, cmd := app.Update(tick); cmd == nil {
		t.Error("a loading app should keep the spinner turning")
	}
}

func TestAppRecordsTheWindowSize(t *testing.T) {
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(tea.WindowSizeMsg{Width: 60, Height: 20})

	if app.width != 60 || app.height != 20 {
		t.Fatalf("size = %dx%d, want 60x20", app.width, app.height)
	}
	// The status bar is pushed to the bottom of the window it now knows about.
	if got := strings.Count(app.View().Content, "\n") + 1; got != 20 {
		t.Errorf("rendered %d lines, want the full window height of 20", got)
	}
}

func TestAppFitsTheStatusBarUnderTallScreens(t *testing.T) {
	// Too short a window to pad: the status bar simply follows the body.
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(tea.WindowSizeMsg{Width: 60, Height: 3})

	if !strings.Contains(app.View().Content, "quit") {
		t.Error("the status bar should still be rendered")
	}
}

func TestAppViewTakesTheWholeWindow(t *testing.T) {
	if !NewApp(config.Config{}, nil).View().AltScreen {
		t.Error("the app should run in the alternate screen buffer")
	}
}

func TestAppShowsANoteOverTheKeyHints(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	app.note = "Setup complete."

	if view := app.View().Content; !strings.Contains(view, "Setup complete.") {
		t.Errorf("view = %q, want the note", view)
	}
}

func TestAppRoutesToOnboarding(t *testing.T) {
	client := &fakeNotion{}
	o := NewOnboarding(config.Config{}, client, func(config.Config) error { return nil })
	app := NewAppWithOnboarding(config.Config{}, client, o)

	if cmd := app.Init(); cmd == nil {
		t.Error("onboarding should start its first Notion call")
	}
	if got := app.View().Content; got != o.View() {
		t.Errorf("view = %q, want the wizard's own view", got)
	}
	// The wizard sizes its own forms, so it must see the resize too.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if app.width != 80 {
		t.Error("the root model should still record the size")
	}

	// While the wizard is on show, "q" belongs to it — the program must not
	// quit out from under a form the user is filling in.
	cmd := press(app, "q")
	if app.onboarding == nil {
		t.Fatal("the wizard should still be on show")
	}
	if isQuitCmd(cmd) {
		t.Error(`"q" quit the program while the wizard was on show`)
	}
}

func TestAppQuitsAFailedWizard(t *testing.T) {
	// A wizard that failed has no form left to handle ctrl+c itself.
	o := NewOnboarding(config.Config{}, &fakeNotion{}, func(config.Config) error { return nil })
	o.fail(errNoPeople)
	app := NewAppWithOnboarding(config.Config{}, &fakeNotion{}, o)

	if !isQuitCmd(press(app, "ctrl+c")) {
		t.Error("ctrl+c should quit even when the wizard has failed")
	}
}

func TestAppTakesOverWhenOnboardingFinishes(t *testing.T) {
	tests := []struct {
		name      string
		msg       OnboardingDoneMsg
		want      string
		wantLoads int
	}{
		{"with a project", OnboardingDoneMsg{Config: testConfig()}, "Setup complete.", 2},
		{"without a project", OnboardingDoneMsg{Config: config.Config{}, NeedsProject: true}, "No projects yet", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newLoadingClient()
			o := NewOnboarding(config.Config{}, client, func(config.Config) error { return nil })
			app := NewAppWithOnboarding(config.Config{}, client, o)

			_, cmd := app.Update(tt.msg)
			run(cmd)

			if app.onboarding != nil {
				t.Error("the wizard should be gone")
			}
			if len(client.queriedDSIDs) != tt.wantLoads {
				t.Errorf("queried %v, want %d loads", client.queriedDSIDs, tt.wantLoads)
			}
			if view := app.View().Content; !strings.Contains(view, tt.want) {
				t.Errorf("view = %q, want %q", view, tt.want)
			}
		})
	}
}

func TestDefaultStylesAreDistinct(t *testing.T) {
	s := DefaultStyles()
	if s.Title.Render("x") == s.Faint.Render("x") {
		t.Error("the title and faint styles should differ")
	}
}
