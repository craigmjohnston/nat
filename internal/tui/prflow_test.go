package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// viewCall is one pull request the screen asked gh for.
type viewCall struct{ dir, ref string }

// fakePRViewer stands in for the GitHub CLI: it records what it was asked to
// read and answers with the pull request — or the refusal — the test wants gh
// to have given.
type fakePRViewer struct {
	pr   gh.PR
	err  error
	made []viewCall
}

var _ PRViewer = (*fakePRViewer)(nil)

func (f *fakePRViewer) ViewPR(dir, ref string) (gh.PR, error) {
	f.made = append(f.made, viewCall{dir, ref})
	return f.pr, f.err
}

// The slices the pull request tests work on: one Done with a pull request
// recorded, and one still in progress with none.
const (
	withPR    = "pr"
	noPRSlice = "np"
)

// samplePR is what gh answers with unless a test says otherwise.
func samplePR() gh.PR {
	return gh.PR{
		Number:      12,
		Title:       "Open a PR screen over the board",
		Body:        "## What\n\nA screen over the board that draws a pull request.",
		State:       "OPEN",
		Author:      "craigmjohnston",
		BaseRefName: "main",
		HeadRefName: "slice/open-a-pr-screen-over-the-board",
		URL:         "https://github.test/craig/nat/pull/12",
	}
}

// prViewApp returns an app showing a plan with those two slices, a fake gh
// wired in and a real working directory for the flow to find, sized so the
// screen has a band to draw in.
func prViewApp(t *testing.T) (*App, *fakePRViewer, string) {
	t.Helper()
	workdir := t.TempDir()

	cfg := testConfig(t)
	project := cfg.Projects[testProjectID]
	project.WorkingDir = workdir
	cfg.Projects[testProjectID] = project

	app := NewApp(cfg, &fakeNotion{})
	p := domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: PR viewer"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: withPR, Name: "PR screen", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: PR viewer", PRURL: "https://github.test/craig/nat/pull/12"},
			{ID: noPRSlice, Name: "No pull request", Status: domain.SliceClaimed,
				StatusName: "In progress", MilestoneID: "M1: PR viewer"},
		})
	seedLocalPlan(t, testProjectID, p)
	app.project = &p
	app.board.hideDone = false // the slice with a pull request is a Done one
	app.board.SetProject(&p)
	viewer := &fakePRViewer{pr: samplePR()}
	app.prViewer = viewer
	// Nothing here asks gh what a repository has open; the screen's own read is
	// the whole of it.
	app.prReader = nil
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return app, viewer, workdir
}

// TestPRKeyOpensThePullRequestScreen is the whole of V on a slice with a pull
// request recorded: the screen comes up, gh is asked for that pull request in
// that repository, and what comes back is drawn.
func TestPRKeyOpensThePullRequestScreen(t *testing.T) {
	app, viewer, workdir := prViewApp(t)
	cursorOn(t, app, withPR)

	cmd := press(app, "V")
	if app.screen != screenPR {
		t.Fatalf("screen = %v, want the pull request screen", app.screen)
	}
	if !app.prview.Busy() {
		t.Error("the screen should show a read in flight")
	}
	if body := app.body(); !strings.Contains(body, "Reading the pull request of PR screen") {
		t.Errorf("body = %q, want the read reported", body)
	}
	msg := first[prViewLoadedMsg](t, run(cmd))
	want := viewCall{workdir, "https://github.test/craig/nat/pull/12"}
	if len(viewer.made) != 1 || viewer.made[0] != want {
		t.Fatalf("gh was asked for %+v, want %+v", viewer.made, want)
	}
	if msg.err != nil {
		t.Fatalf("read = %v, want the pull request", msg.err)
	}

	app.Update(msg)
	if app.prview.Busy() || app.prview.Number() != 12 {
		t.Errorf("screen shows #%d, want the pull request that came back", app.prview.Number())
	}
	if got := app.headerName(); got != "PR #12" {
		t.Errorf("header = %q, want the pull request named", got)
	}
	if got := app.chipText(); got != "pr" {
		t.Errorf("chip = %q, want the screen named", got)
	}
	body := app.body()
	for _, want := range []string{"open", "#12", "Open a PR screen over the board",
		"slice/open-a-pr-screen-over-the-board → main"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want it to name %q", body, want)
		}
	}
	// glamour styles a word at a time, so the description is looked for by one
	// of its words rather than by a phrase with escape sequences through it.
	if !strings.Contains(body, "draws") {
		t.Error("the body should render the description")
	}
}

// TestPRKeyUsesTheSliceRepo covers a slice with a repository of its own, which
// is where its pull request is and so where gh is run.
func TestPRKeyUsesTheSliceRepo(t *testing.T) {
	app, viewer, _ := prViewApp(t)
	repo := t.TempDir()
	setSlice(app, withPR, func(s *domain.Slice) { s.Repo = repo })
	cursorOn(t, app, withPR)

	run(press(app, "V"))
	if len(viewer.made) != 1 || viewer.made[0].dir != repo {
		t.Errorf("gh ran in %+v, want the slice's own repository", viewer.made)
	}
}

// TestPRKeyNeedsAPullRequest covers the refusals: a row that is no slice, and a
// slice with nothing recorded on it. Neither opens the screen.
func TestPRKeyNeedsAPullRequest(t *testing.T) {
	app, viewer, _ := prViewApp(t)

	cursorOnMilestone(t, app)
	run(press(app, "V"))
	if app.screen != screenBoard || !strings.Contains(app.board.confirmText, "Move to a slice") {
		t.Errorf("screen = %v, said %q; want the board and a refusal", app.screen, app.board.confirmText)
	}

	cursorOn(t, app, noPRSlice)
	run(press(app, "V"))
	if app.screen != screenBoard {
		t.Fatalf("screen = %v, want the board", app.screen)
	}
	if !strings.Contains(app.board.confirmText, "no pull request recorded") {
		t.Errorf("said %q, want the slice named as having none", app.board.confirmText)
	}
	if len(viewer.made) != 0 {
		t.Errorf("gh ran %+v, want nothing asked of it", viewer.made)
	}
}

// TestPRKeyNeedsARepository covers a slice whose working directory is not
// there: the board knows that before gh does, so it says so rather than letting
// gh fail deep inside a subprocess.
func TestPRKeyNeedsARepository(t *testing.T) {
	app, viewer, _ := prViewApp(t)
	setSlice(app, withPR, func(s *domain.Slice) { s.Repo = t.TempDir() + "/gone" })
	cursorOn(t, app, withPR)

	run(press(app, "V"))
	if app.screen != screenBoard || !strings.Contains(app.board.confirmText, "Cannot read the pull request") {
		t.Errorf("screen = %v, said %q; want the board and a refusal", app.screen, app.board.confirmText)
	}
	if len(viewer.made) != 0 {
		t.Errorf("gh ran %+v, want nothing asked of it", viewer.made)
	}
}

// TestPRKeyDoesNothingWithoutAProjectOrGh covers the two ways the flow has
// nowhere to run: no configured project, and no GitHub CLI behind it.
func TestPRKeyDoesNothingWithoutAProjectOrGh(t *testing.T) {
	app, _, _ := prViewApp(t)
	cursorOn(t, app, withPR)
	app.prViewer = nil
	if cmd := app.viewPRFlow(); cmd != nil {
		t.Error("a board with no gh should ask for nothing")
	}

	app, _, _ = prViewApp(t)
	cursorOn(t, app, withPR)
	app.cfg.ActiveProjectID = ""
	if cmd := app.viewPRFlow(); cmd != nil {
		t.Error("a board with no project should ask for nothing")
	}
}

// TestPRScreenReportsAFailedRead covers a gh that refused: the screen says so
// and the board is still there behind it, rather than an error banner over the
// app.
func TestPRScreenReportsAFailedRead(t *testing.T) {
	app, viewer, _ := prViewApp(t)
	viewer.err = errors.New("no pull requests found for branch slice/x")
	cursorOn(t, app, withPR)

	app.Update(first[prViewLoadedMsg](t, run(press(app, "V"))))
	if app.err != nil {
		t.Errorf("app error = %v, want the failure kept on the screen", app.err)
	}
	if app.screen != screenPR {
		t.Fatalf("screen = %v, want the pull request screen", app.screen)
	}
	if body := app.body(); !strings.Contains(body, "no pull requests found") {
		t.Errorf("body = %q, want gh's own refusal", body)
	}
	if got := app.headerName(); got != "PR" {
		t.Errorf("header = %q, want the screen's own name with no number to give", got)
	}
}

// TestPRScreenRefreshesInPlace covers r while the screen is up: gh is asked
// again for the same pull request, and what comes back replaces what was there.
func TestPRScreenRefreshesInPlace(t *testing.T) {
	app, viewer, workdir := prViewApp(t)
	cursorOn(t, app, withPR)
	app.Update(first[prViewLoadedMsg](t, run(press(app, "V"))))

	merged := samplePR()
	merged.State = "MERGED"
	merged.Title = "Merged by now"
	viewer.pr = merged
	cmd := press(app, "r")
	if !app.prview.Busy() {
		t.Error("the refresh should show a read in flight")
	}
	app.Update(first[prViewLoadedMsg](t, run(cmd)))

	if len(viewer.made) != 2 || viewer.made[1] != (viewCall{workdir, samplePR().URL}) {
		t.Fatalf("gh was asked %+v, want the same pull request a second time", viewer.made)
	}
	body := app.body()
	if !strings.Contains(body, "merged") || !strings.Contains(body, "Merged by now") {
		t.Errorf("body = %q, want the fresh reading", body)
	}
}

// TestPRScreenRefreshNeedsATarget covers the refresh key on a screen that has
// never been pointed at a pull request, and on a board with no gh: neither has
// anything to read again.
func TestPRScreenRefreshNeedsATarget(t *testing.T) {
	app, _, _ := prViewApp(t)
	if cmd := app.startPRLoad(); cmd != nil {
		t.Error("a screen pointed at nothing should read nothing")
	}
	cursorOn(t, app, withPR)
	app.Update(first[prViewLoadedMsg](t, run(press(app, "V"))))
	app.prViewer = nil
	if cmd := app.startPRLoad(); cmd != nil {
		t.Error("a board with no gh should read nothing")
	}
}

// TestPRScreenScrollsAndCloses covers the two things left: the screen's keys
// are the viewport's, and esc goes back to the board.
func TestPRScreenScrollsAndCloses(t *testing.T) {
	app, viewer, _ := prViewApp(t)
	long := samplePR()
	long.Body = strings.Repeat("a paragraph of the description\n\n", 60)
	viewer.pr = long
	cursorOn(t, app, withPR)
	app.Update(first[prViewLoadedMsg](t, run(press(app, "V"))))

	press(app, "down")
	if app.prview.vp.YOffset() == 0 {
		t.Error("the screen should scroll its description")
	}
	press(app, "esc")
	if app.screen != screenBoard {
		t.Errorf("screen = %v, want the board back", app.screen)
	}
}

// TestPRScreenIsDroppedByAProjectSwitch covers the reset: a pull request read
// on one project is not what the refresh key reads again on the next.
func TestPRScreenIsDroppedByAProjectSwitch(t *testing.T) {
	app, _, _ := prViewApp(t)
	cursorOn(t, app, withPR)
	app.Update(first[prViewLoadedMsg](t, run(press(app, "V"))))
	app.setScreen(screenBoard)

	app.showActiveProject()
	if app.prview.Loadable() || app.prview.Number() != 0 {
		t.Error("the switch should leave nothing of the last project's pull request")
	}
}

// TestPRKeyIsInTheHelp covers the one place the key is named: the help screen,
// since the hints row has no room for a key that acts on so few rows.
func TestPRKeyIsInTheHelp(t *testing.T) {
	app, _, _ := prViewApp(t)
	if !strings.Contains(app.helpBody(), "view pull request") {
		t.Error("the help screen should name the pull request key")
	}
}

// pollingApp is the pull request screen up and loaded, with the re-read timer
// stubbed to a message the test feeds itself.
func pollingApp(t *testing.T) (*App, *fakePRViewer) {
	t.Helper()
	app, viewer, _ := prViewApp(t)
	cursorOn(t, app, withPR)
	cmd := press(app, "V")
	app.Update(first[prViewLoadedMsg](t, run(cmd)))
	viewer.made = nil
	return app, viewer
}

func TestPRPollTickIsScheduledOnOpen(t *testing.T) {
	prev := prPollTick
	t.Cleanup(func() { prPollTick = prev })
	var gens []int
	prPollTick = func(gen int) tea.Cmd { gens = append(gens, gen); return nil }
	app, _, _ := prViewApp(t)
	cursorOn(t, app, withPR)
	press(app, "V")
	if len(gens) != 1 || gens[0] != app.prPollGen {
		t.Errorf("scheduled %v, want one tick for generation %d", gens, app.prPollGen)
	}
	prevInterval := prPollInterval
	t.Cleanup(func() { prPollInterval = prevInterval })
	prPollInterval = time.Millisecond
	if got := defaultPRPollTick(3)(); got != (prPollTickMsg{gen: 3}) {
		t.Errorf("tick = %v, want one for generation 3", got)
	}
}

func TestPRPollTickRereadsAndKeepsScroll(t *testing.T) {
	app, viewer := pollingApp(t)
	long := samplePR()
	long.Body = strings.Repeat("line\n\n", 80)
	app.prview.SetPR(long)
	app.prview.vp.SetYOffset(5)

	updated := samplePR()
	updated.Body = long.Body
	updated.Title = "Retitled"
	viewer.pr = updated

	_, cmd := app.Update(prPollTickMsg{gen: app.prPollGen})
	if !app.prPollBusy {
		t.Fatal("a read should be in flight")
	}
	msgs := run(cmd)
	// One read at a time: a tick while one is in flight is passed over.
	if got := app.prPolled(prPollTickMsg{gen: app.prPollGen}); run(got) != nil && len(viewer.made) != 1 {
		t.Error("a second read started while one was in flight")
	}
	app.Update(first[prBackgroundMsg](t, msgs))
	if app.prPollBusy {
		t.Error("read should be finished")
	}
	if !strings.Contains(app.body(), "Retitled") {
		t.Error("the new reading should be on screen")
	}
	if got := app.prview.vp.YOffset(); got != 5 {
		t.Errorf("scroll = %d, want it kept at 5", got)
	}
	if !app.prview.Ready() {
		t.Error("screen should stay ready")
	}
}

func TestPRPollFailureKeepsReading(t *testing.T) {
	app, viewer := pollingApp(t)
	viewer.err = errors.New("gh down")
	_, cmd := app.Update(prPollTickMsg{gen: app.prPollGen})
	app.Update(first[prBackgroundMsg](t, run(cmd)))
	if !app.prview.Ready() || app.prview.Number() != 12 {
		t.Error("the last good reading should stay on screen")
	}
	if app.prPollBusy {
		t.Error("busy should clear after a failure")
	}
}

func TestPRPollPassedOver(t *testing.T) {
	app, viewer := pollingApp(t)
	gen := app.prPollGen

	// Merge prompt up.
	app.prview.SetPrompt([]string{"yes", "no"})
	app.Update(prPollTickMsg{gen: gen})
	app.prview.ClearPrompt()
	// A manual read in flight.
	app.prview.Start("s", "n", "ref", "dir")
	app.Update(prPollTickMsg{gen: gen})
	// No viewer.
	app.prview.SetPR(samplePR())
	pv := app.prViewer
	app.prViewer = nil
	app.Update(prPollTickMsg{gen: gen})
	app.prViewer = pv
	if len(viewer.made) != 0 || app.prPollBusy {
		t.Errorf("reads made %v, want every tick passed over", viewer.made)
	}
}

func TestPRPollStopsWithTheScreenAndGeneration(t *testing.T) {
	prev := prPollTick
	t.Cleanup(func() { prPollTick = prev })
	n := 0
	prPollTick = func(int) tea.Cmd { n++; return nil }
	app, viewer := pollingApp(t)
	n = 0
	// A stale generation is dropped.
	if cmd := app.prPolled(prPollTickMsg{gen: app.prPollGen - 1}); cmd != nil {
		t.Error("stale generation should stop")
	}
	// Leaving the screen stops it.
	press(app, "esc")
	if app.screen == screenPR {
		t.Fatal("esc should leave the screen")
	}
	if cmd := app.prPolled(prPollTickMsg{gen: app.prPollGen}); cmd != nil || n != 0 || len(viewer.made) != 0 {
		t.Error("the timer should stop once the screen is left")
	}
}

func TestPRBackgroundResultDropped(t *testing.T) {
	app, _ := pollingApp(t)
	other := samplePR()
	other.Title = "Other"
	// Different ref.
	app.prPollBusy = true
	app.Update(prBackgroundMsg{ref: "elsewhere", pr: other})
	// Manual read in flight.
	app.prview.Start("s", "n", app.prview.ref, app.prview.dir)
	app.Update(prBackgroundMsg{ref: app.prview.ref, pr: other})
	if app.prPollBusy || strings.Contains(app.body(), "Other") {
		t.Error("stale results should be dropped")
	}
	// Screen left.
	app.prview.SetPR(samplePR())
	press(app, "esc")
	app.Update(prBackgroundMsg{ref: app.prview.ref, pr: other})
	if app.prview.pr.Title == "Other" {
		t.Error("a result after leaving should be dropped")
	}
}
