package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// wishlistItems is a wishlist of n items, as the client reads them off the
// project page.
func wishlistItems(n int) []notion.WishlistItem {
	items := make([]notion.WishlistItem, n)
	for i := range items {
		items[i] = notion.WishlistItem{ID: string(rune('a'+i)) + "-block", Markdown: "- something"}
	}
	return items
}

// loadedApp is the app after a full load: the plan, and whatever the wishlist
// read answered with. The window is sized so the bar has a width to lay out to.
func loadedApp(t *testing.T, client *loadingClient) *App {
	t.Helper()
	a := NewApp(testConfig(), client)
	for _, msg := range run(a.Init()) {
		a.Update(msg)
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return a
}

// wishlistClient answers the load pipeline's queries and hands back items for
// the wishlist read, or err instead when it is set.
func wishlistClient(items []notion.WishlistItem, err error) *loadingClient {
	c := newLoadingClient()
	c.wishlist = func(string) ([]notion.WishlistItem, error) { return items, err }
	return c
}

func TestAppReadsTheWishlistOfTheActiveProjectPage(t *testing.T) {
	client := wishlistClient(wishlistItems(3), nil)
	a := loadedApp(t, client)

	if got := client.wishlistPages; len(got) != 1 || got[0] != testProjectID {
		t.Errorf("wishlist read for %v, want the active project page %q", got, testProjectID)
	}
	if len(a.wishlist) != 3 {
		t.Errorf("wishlist = %d, want the 3 items read", len(a.wishlist))
	}
}

func TestAppRefreshRereadsTheWishlist(t *testing.T) {
	client := wishlistClient(wishlistItems(1), nil)
	a := loadedApp(t, client)

	for _, msg := range run(press(a, "r")) {
		a.Update(msg)
	}
	if got := len(client.wishlistPages); got != 2 {
		t.Errorf("wishlist read %d times, want the refresh to have re-read it", got)
	}
	if len(a.wishlist) != 1 {
		t.Errorf("wishlist = %d, want the one item still pending", len(a.wishlist))
	}
}

func TestBarShowsTheWishlistCount(t *testing.T) {
	golden(t, "bar-wishlist", loadedApp(t, wishlistClient(wishlistItems(3), nil)).View().WindowTitle)
}

func TestBarWithoutPendingWishlistItemsSaysNothingAboutIt(t *testing.T) {
	golden(t, "bar-no-wishlist", loadedApp(t, wishlistClient(nil, nil)).View().WindowTitle)
}

func TestBarCountsOneWishlistItemInTheSingular(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(1), nil))

	if got := a.View().WindowTitle; !strings.Contains(got, "1 wishlist item ") {
		t.Errorf("bar = %q, want one item named in the singular", got)
	}
}

func TestAFailedWishlistReadLeavesTheBoardRendered(t *testing.T) {
	client := wishlistClient(nil, errors.New("boom"))
	a := loadedApp(t, client)

	if a.err != nil {
		t.Errorf("err = %v, want a wishlist failure kept off the screen", a.err)
	}
	if len(a.wishlist) != 0 {
		t.Errorf("wishlist = %d, want no count from a failed read", len(a.wishlist))
	}
	if got := a.View().WindowTitle; strings.Contains(got, "wishlist") || strings.Contains(got, "boom") {
		t.Errorf("bar = %q, want no indicator and no error on it", got)
	}
	// The plan the failure rode alongside is on the board all the same.
	view := stripANSI(a.View().Content)
	for _, want := range []string{"tracker", "1/2"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q, want the plan still drawn:\n%s", want, view)
		}
	}
}

func TestTheWishlistIndicatorGivesWayToTheStatusMessage(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(3), nil))
	a.Update(notionErrMsg{err: errors.New(strings.Repeat("wide ", 12))})

	if got := a.View().WindowTitle; strings.Contains(got, "wishlist") {
		t.Errorf("bar = %q, want the indicator dropped for want of room", got)
	}
	// It is back as soon as the message goes and the room comes back with it.
	a.Update(toastGoneMsg{id: a.toastID})
	if got := a.View().WindowTitle; !strings.Contains(got, "3 wishlist items") {
		t.Errorf("bar = %q, want the indicator back", got)
	}
}

func TestTheWishlistIndicatorNamesTheWorkshopKey(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(2), nil))

	help := a.keys.Workshop.Help()
	if got := a.View().WindowTitle; !strings.Contains(got, help.Key+" "+help.Desc) {
		t.Errorf("bar = %q, want the key that opens a workshop on the wishlist", got)
	}
}

func TestSwitchingProjectDropsTheWishlistCount(t *testing.T) {
	a := loadedApp(t, wishlistClient(wishlistItems(3), nil))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a.Update(projectSwitchedMsg{id: "other", name: "other"})

	if len(a.wishlist) != 0 {
		t.Errorf("wishlist = %d, want the last project's count dropped", len(a.wishlist))
	}
}

// workshopApp is the launch harness with a wishlist of n items already read,
// as a load would have left it.
func workshopApp(t *testing.T, n int) (*App, *fakeLauncher, string) {
	t.Helper()
	app, launcher, workdir := launchApp(t)
	app.wishlist = wishlistItems(n)
	return app, launcher, workdir
}

// W launches the planning agent straight on the wishlist: no form between the
// key and the session, because the items are the request.
func TestAppWorkshopKeyLaunchesAPlanningAgentOnTheWishlist(t *testing.T) {
	app, launcher, workdir := workshopApp(t, 2)

	drive(t, app, press(app, "W"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	got := launcher.launches[0]
	if got.session != agent.PlanSession {
		t.Errorf("session = %q, want %q", got.session, agent.PlanSession)
	}
	if got.workdir != workdir {
		t.Errorf("workdir = %q, want the project default %q", got.workdir, workdir)
	}
	// The sentinel again: one planning agent, however it was launched.
	if got.sliceID != agent.PlanSentinel {
		t.Errorf("tag = %q, want %q", got.sliceID, agent.PlanSentinel)
	}
	if app.form != nil {
		t.Fatalf("form = %T, want no question asked", app.form)
	}

	prompt, err := os.ReadFile(got.promptFile)
	if err != nil {
		t.Fatalf("read the prompt file: %v", err)
	}
	for _, want := range []string{"/queue-work", "## The request",
		"nat wishlist-clear a-block b-block"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("the prompt is missing %q:\n%s", want, prompt)
		}
	}

	// The agent is shown on launch, as a typed planning launch is.
	if want := []string{agent.PlanSession}; !equal(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
	if app.busy {
		t.Error("the launch is over; the board is live again")
	}
}

// Nothing pending, nothing to workshop: the key is not even named on the bar
// when the wishlist is empty, and pressing it anyway does nothing.
// W asks nothing, so the workshop pair is the whole answer: the same setting
// the typed planning launch prefills, used as it stands.
func TestAppWorkshopKeyCarriesTheConfiguredWorkshopModel(t *testing.T) {
	app, launcher, _ := workshopApp(t, 2)
	app.cfg.WorkshopAgent = config.AgentModel{Model: "haiku", Effort: "low"}
	app.cfg.SliceAgent = config.AgentModel{Model: "opus", Effort: "high"}

	drive(t, app, press(app, "W"))

	if len(launcher.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one", launcher.launches)
	}
	want := config.AgentModel{Model: "haiku", Effort: "low"}
	if got := launcher.launches[0].model; got != want {
		t.Errorf("model = %+v, want the configured %+v", got, want)
	}
}

func TestAppWorkshopKeyDoesNothingWithAnEmptyWishlist(t *testing.T) {
	app, launcher, _ := workshopApp(t, 0)

	if cmd := press(app, "W"); cmd != nil {
		t.Error("there is nothing to workshop")
	}
	if len(launcher.launches) != 0 {
		t.Errorf("launches = %+v, want none", launcher.launches)
	}
	if app.form != nil {
		t.Fatalf("form = %T, want the key ignored", app.form)
	}
}

// One planning agent at a time. With one already running, W neither starts a
// second nor toggles the first — that is w's job, and the running agent is
// already holding the plan in its head.
func TestAppWorkshopKeyDoesNothingWithAPlanningAgentRunning(t *testing.T) {
	app, launcher, _ := workshopApp(t, 2)
	app.live = map[string]string{agent.PlanSentinel: agent.PlanSession}

	if cmd := press(app, "W"); cmd != nil {
		t.Error("the running planning agent should be left alone")
	}
	if len(launcher.launches) != 0 {
		t.Errorf("launches = %+v, want no second planning agent", launcher.launches)
	}
	if len(launcher.attached) != 0 || len(launcher.clients) != 0 {
		t.Errorf("attached = %v, clients = %v, want the agent untouched", launcher.attached, launcher.clients)
	}
}

func TestAppWorkshopKeyIsRefusedWithNothingToLaunchWith(t *testing.T) {
	tests := []struct {
		name    string
		disable func(*App)
	}{
		{"no project", func(a *App) { a.cfg.ActiveProjectID = "" }},
		{"no launcher", func(a *App) { a.launcher = nil }},
		{"a write already in flight", func(a *App) { a.busy = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _ := workshopApp(t, 2)
			tt.disable(app)

			if cmd := press(app, "W"); cmd != nil {
				t.Error("there is nothing to launch with")
			}
			if app.form != nil {
				t.Errorf("form = %T, want the key ignored", app.form)
			}
		})
	}
}

func TestLaunchWishlistAgentReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	launcher := &fakeLauncher{}

	msg := runMsg(t, launchWishlistAgent(launcher, "tracker", "/tmp", wishlistItems(1), config.AgentModel{})).(agentLaunchedMsg)

	if msg.err == nil || !strings.Contains(msg.err.Error(), "launch planning agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file", msg.err)
	}
	if len(launcher.launches) != 0 {
		t.Error("no session should start without a prompt to seed it")
	}
}

func TestAppWorkshopLaunchReportsAFailedLaunch(t *testing.T) {
	app, launcher, _ := workshopApp(t, 2)
	launcher.launchErr = errors.New("duplicate session")

	drive(t, app, press(app, "W"))

	if app.err == nil || !strings.Contains(app.err.Error(), "duplicate session") {
		t.Errorf("err = %v, want the failed launch", app.err)
	}
	if app.busy {
		t.Error("a failed launch should leave nothing in flight")
	}
}

// The key is out of the hints row — the indicator names it when it does
// anything — but the help screen lists it, which is where a key nobody has
// seen hinted is still findable.
func TestTheHelpScreenListsTheWorkshopKey(t *testing.T) {
	app, _, _ := launchApp(t)

	help := app.keys.Workshop.Help()
	if got := stripANSI(app.helpBody()); !strings.Contains(got, help.Key+"  "+help.Desc) {
		t.Errorf("help does not list the workshop key:\n%s", got)
	}
	for _, line := range app.wrapHints(app.contextHints(), 200, 3) {
		if strings.Contains(stripANSI(line), help.Desc) {
			t.Errorf("hints row = %q, want the workshop key left to the indicator", line)
		}
	}
}
