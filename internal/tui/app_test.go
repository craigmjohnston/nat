package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
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
		// tea.sequenceMsg is unexported, but it is a []tea.Cmd underneath — the
		// runtime runs its commands in order, and so do we. huh moves between
		// fields with one, so a form cannot be driven without this.
		if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
			var msgs []tea.Msg
			for i := range v.Len() {
				msgs = append(msgs, run(v.Index(i).Interface().(tea.Cmd))...)
			}
			return msgs
		}
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
		msg = keyPress(s)
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
	// A window to size the bar to: the plan's progress is the segmented bar
	// over the board, its label naming the tally and the milestone in play.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := stripANSI(app.View().Content)
	for _, want := range []string{"tracker", barCell, "1/2"} {
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
			if line := app.windowTitle(); !strings.Contains(line, "boom") ||
				!strings.Contains(line, "esc to dismiss") {
				t.Errorf("status line = %q, want the error and the way out", line)
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

			// Init still queries the terminal's background colour; what it must
			// not do is call Notion, which the queriedDSIDs check below covers.
			if cmd := app.Init(); cmd == nil {
				t.Error("the background query should still go out")
			}
			if len(client.queriedDSIDs) != 0 {
				t.Errorf("queried %v, want no Notion calls", client.queriedDSIDs)
			}
			// The board's own note goes out on the status line; the plan it loads
			// is drawn in the window.
			if got := app.View().Content + app.windowTitle(); !strings.Contains(got, tt.want) {
				t.Errorf("view and status line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppWithoutAClientDoesNotLoad(t *testing.T) {
	// Init still polls for the agents' sessions, which do not go through Notion.
	if cmd := NewApp(testConfig(), nil).startLoad(); cmd != nil {
		t.Error("there is no client to load with")
	}
}

func TestAppRefreshReloads(t *testing.T) {
	client := newLoadingClient()
	app := NewApp(testConfig(), client)
	run(app.Init())
	app.Update(first[projectLoadedMsg](t, run(app.Init())))
	before := len(client.queriedDSIDs)
	app.note, app.toast = "Setup complete.", "Refreshed."
	app.board.SetConfirm("Saved.", sevSuccess)

	run(press(app, "r"))

	if len(client.queriedDSIDs) != before+2 {
		t.Errorf("queried %d data sources, want %d more", len(client.queriedDSIDs), before+2)
	}
	if app.note != "" || app.toast != "" || app.board.confirmText != "" {
		t.Errorf("note = %q, toast = %q, confirm = %q, want them all cleared on refresh",
			app.note, app.toast, app.board.confirmText)
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
	// The bands are laid out to the bottom of the window it now knows about.
	if got := strings.Count(app.View().Content, "\n") + 1; got != 20 {
		t.Errorf("rendered %d lines, want the full window height of 20", got)
	}
}

func TestAppFitsTheHintsUnderTallScreens(t *testing.T) {
	// Too short a window to pad: the hints row simply follows the body.
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(tea.WindowSizeMsg{Width: 60, Height: 3})

	if !strings.Contains(app.View().Content, "quit") {
		t.Error("the hints row should still be rendered")
	}
}

func TestAppViewTakesTheWholeWindow(t *testing.T) {
	if !NewApp(config.Config{}, nil).View().AltScreen {
		t.Error("the app should run in the alternate screen buffer")
	}
}

func TestAppSetsTheNativeProgress(t *testing.T) {
	// The board's own plan: 6 slices, 3 of them Done.
	p := testProject()
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(projectLoadedMsg{project: p})

	v := app.View()
	if v.ProgressBar == nil {
		t.Fatal("a loaded plan should set the terminal progress bar")
	}
	if got := *v.ProgressBar; got != (tea.ProgressBar{State: tea.ProgressBarDefault, Value: 50}) {
		t.Errorf("progress bar = %+v, want half done", got)
	}

	// Moving a slice on moves the bar, so the terminal tracks the plan.
	p.Slices[4].Status = domain.SliceDone
	app.Update(projectLoadedMsg{project: p})

	v = app.View()
	if v.ProgressBar == nil || v.ProgressBar.Value != 66 {
		t.Errorf("progress bar = %+v, want two thirds done", v.ProgressBar)
	}
}

func TestAppClearsTheNativeProgressWithNoPlan(t *testing.T) {
	tests := []struct {
		name    string
		project *domain.Project
	}{
		{name: "no project at all"},
		{
			name:    "a project with nothing planned yet",
			project: &domain.Project{ID: testProjectID, Name: "tracker"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp(testConfig(), newLoadingClient())
			app.project = tt.project

			if v := app.View(); v.ProgressBar != nil {
				t.Errorf("progress bar = %+v, want it cleared", v.ProgressBar)
			}
		})
	}
}

func TestAppPutsANoteOnTheStatusLine(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	app.note = "Setup complete."

	if got := app.View().WindowTitle; !strings.Contains(got, "Setup complete.") {
		t.Errorf("window title = %q, want the note", got)
	}
}

func TestAppStatusBarChipNamesTheScreen(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	// No window yet and no project loaded: the chip's fallback, uncut.
	if got := app.chipText(); got != "nat" {
		t.Errorf("chip = %q, want the app's own name with no project", got)
	}

	// The board's chip stays the app's own name even with a project loaded:
	// the heading names the project already.
	p := testProject()
	app.project = &p
	tests := map[screen]string{
		screenBoard: "nat",
		screenHelp:  "help",
		screenInfo:  "info",
		screenForm:  "edit",
	}
	for scr, want := range tests {
		app.screen = scr
		if got := app.chipText(); got != want {
			t.Errorf("chip on screen %d = %q, want %q", scr, got, want)
		}
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
	o.fail(errNoOwner)
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
			// The board's own note goes out on the status line; the plan it loads
			// is drawn in the window.
			if got := app.View().Content + app.windowTitle(); !strings.Contains(got, tt.want) {
				t.Errorf("view and status line = %q, want %q", got, tt.want)
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

// pageBlocks is the project page body the info tests fetch.
func pageBlocks(t *testing.T) []notion.Block {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "info-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var blocks []notion.Block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

// infoApp returns an app sized like a terminal, with the fixture page waiting
// behind the info key.
func infoApp(t *testing.T) (*App, *loadingClient) {
	t.Helper()
	client := newLoadingClient()
	client.blocks = func(string) ([]notion.Block, error) { return pageBlocks(t), nil }
	app := NewApp(testConfig(), client)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return app, client
}

// openInfo presses "i" and delivers whatever the fetch came back with.
func openInfo(t *testing.T, app *App) {
	t.Helper()
	for _, msg := range run(press(app, "i")) {
		app.Update(msg)
	}
}

func TestAppLoadsTheProjectPageOnDemand(t *testing.T) {
	app, client := infoApp(t)

	openInfo(t, app)

	if want := []string{testProjectID}; !equal(client.blockParents, want) {
		t.Errorf("fetched %v, want the project page %v", client.blockParents, want)
	}
	view := app.View().Content
	for _, want := range []string{"Info", "Conventions", "Never push to main"} {
		if !strings.Contains(stripANSI(view), want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestAppFetchesTheProjectPageOnlyOnce(t *testing.T) {
	app, client := infoApp(t)

	openInfo(t, app)
	press(app, "i") // back to the board
	openInfo(t, app)

	if len(client.blockParents) != 1 {
		t.Errorf("fetched %d times, want the page cached", len(client.blockParents))
	}
}

func TestAppReportsAFailedProjectPageOnTheInfoScreen(t *testing.T) {
	boom := errors.New("boom")
	app, client := infoApp(t)
	client.blocks = func(string) ([]notion.Block, error) { return nil, boom }

	openInfo(t, app)

	if app.err != nil {
		t.Errorf("err = %v, want the info screen to own its own failure", app.err)
	}
	if view := stripANSI(app.View().Content); !strings.Contains(view, "load project page: boom") {
		t.Errorf("view = %q, want the failure on the info screen", view)
	}
	// A failed fetch leaves the screen idle, so leaving and returning retries.
	press(app, "i")
	openInfo(t, app)
	if len(client.blockParents) != 2 {
		t.Errorf("fetched %d times, want a failed fetch to be retried", len(client.blockParents))
	}
}

func TestAppWithNothingToShowDoesNotFetchTheProjectPage(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.Config
		client NotionAPI
	}{
		{"no project", config.Config{}, newLoadingClient()},
		{"selection not in config", config.Config{ActiveProjectID: "gone"}, newLoadingClient()},
		{"no client", testConfig(), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp(tt.cfg, tt.client)
			if cmd := press(app, "i"); cmd != nil {
				t.Error("there is no project page to fetch")
			}
			if view := app.View().Content; !strings.Contains(view, "Info") {
				t.Errorf("view = %q, want the info screen", view)
			}
		})
	}
}

func TestAppRoutesScrollingToTheInfoScreen(t *testing.T) {
	app, _ := infoApp(t)
	// A viewport tall enough to hold the whole page has nothing to scroll.
	app.info.SetSize(40, 3)
	openInfo(t, app)

	press(app, "j")

	if app.info.vp.YOffset() != 1 {
		t.Errorf("offset = %d, want the key routed to the info screen", app.info.vp.YOffset())
	}
}

func TestAppSizesTheInfoViewport(t *testing.T) {
	app, _ := infoApp(t)

	// The info screen is the body box's interior: the window less the box's
	// border and padding, and less the boxed header — no plan is loaded, so it
	// holds the heading alone — and the hints row.
	wantW := 80 - 2*framePadX
	wantH := 24 - (headerHeight + 2) - hintsHeight - 2
	if got := app.info.vp.Width(); got != wantW {
		t.Errorf("viewport width = %d, want %d", got, wantW)
	}
	if got := app.info.vp.Height(); got != wantH {
		t.Errorf("viewport height = %d, want %d", got, wantH)
	}

	// The whole window is still filled once the info screen is what is on show.
	openInfo(t, app)
	if got := strings.Count(app.View().Content, "\n") + 1; got != 24 {
		t.Errorf("rendered %d lines, want the full window height of 24", got)
	}
}

func TestAppRefreshReloadsTheProjectPage(t *testing.T) {
	t.Run("lazily from the board", func(t *testing.T) {
		app, client := infoApp(t)
		openInfo(t, app)
		press(app, "i") // back to the board

		run(press(app, "r"))
		if len(client.blockParents) != 1 {
			t.Error("a refresh from the board should not fetch the page it is not showing")
		}

		openInfo(t, app)
		if len(client.blockParents) != 2 {
			t.Error("the next visit to the info screen should re-fetch")
		}
	})

	t.Run("at once from the info screen", func(t *testing.T) {
		app, client := infoApp(t)
		openInfo(t, app)

		for _, msg := range run(press(app, "r")) {
			app.Update(msg)
		}

		if len(client.blockParents) != 2 {
			t.Errorf("fetched %d times, want the page the user is reading refreshed",
				len(client.blockParents))
		}
		if !strings.Contains(stripANSI(app.View().Content), "Conventions") {
			t.Error("the refreshed page should be back on show")
		}
	})
}

func TestAppSpinnerTurnsWhileTheProjectPageLoads(t *testing.T) {
	app, _ := infoApp(t)
	press(app, "i")

	tick := spinner.TickMsg{Time: time.Now(), ID: app.spinner.ID()}
	if _, cmd := app.Update(tick); cmd == nil {
		t.Error("the spinner should keep turning while the page is in flight")
	}
	if view := stripANSI(app.View().Content); !strings.Contains(view, "Loading the project page") {
		t.Errorf("view = %q, want the loading state", view)
	}
}
