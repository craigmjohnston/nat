package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRViewer is what the pull request screen needs of the GitHub CLI: one pull
// request in full, read in the slice's repository. It is an interface for the
// reason [PRReader] and [PRCreator] are — the flow can then be driven without
// gh, without a network and without a GitHub account.
type PRViewer interface {
	ViewPR(dir, ref string) (gh.PR, error)
}

// The screen's edge, held as a variable so the tests can stand in for it: the
// real one shells out to gh.
var newPRViewer = defaultPRViewer

// defaultPRViewer is the real gh on PATH.
func defaultPRViewer() PRViewer { return gh.New() }

// newReviewReader is a fix launch's own edge onto gh — see
// [actions.PRReviewReader] — held apart from newPRViewer even though both
// drive the real gh, so a fake standing in for the PR screen's own reads is
// never asked to answer these two as well.
var newReviewReader = defaultReviewReader

// defaultReviewReader is the real gh on PATH.
func defaultReviewReader() actions.PRReviewReader { return gh.New() }

// prViewLoadedMsg carries the pull request that was read, or the failure that
// came instead. The screen reports its own failures, so they do not go through
// notionErrMsg — nothing about this one came from Notion.
type prViewLoadedMsg struct {
	pr  gh.PR
	err error
}

// viewPRFlow opens the pull request screen on the slice the cursor is on: what
// GitHub currently says about the pull request recorded on its page.
//
// Only a slice with one recorded has anything to show, which is every slice the
// approve key has been pressed on and no other: the key is refused on the rest
// rather than opening a screen that could only say there is nothing to read.
func (a *App) viewPRFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.prViewer == nil {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to read its pull request.", sevWarning)
	}
	if s.PRURL == "" {
		return a.showConfirm(fmt.Sprintf("%q has no pull request recorded yet.", s.Name), sevWarning)
	}
	dir := expandHome(strings.TrimSpace(workdirFor(s, project)))
	// The repository is checked here rather than left to gh, which would
	// otherwise fail deep inside a subprocess over something the board knows.
	if err := existingDir(dir); err != nil {
		return a.showConfirm(fmt.Sprintf("Cannot read the pull request for %q: %v.", s.Name, err), sevError)
	}
	a.prview.Start(s.ID, s.Name, s.PRURL, dir)
	a.setScreen(screenPR)
	// A new generation retires any timer still running from an earlier opening.
	a.prPollGen++
	return tea.Batch(a.spinner.Tick, readPR(a.prViewer, s.PRURL, dir), prPollTick(a.prPollGen))
}

// prPollInterval is how often the open screen reads the pull request again:
// often enough that checks finishing show up unprompted, rarely enough not to
// hammer gh.
var prPollInterval = 12 * time.Second

// prPollTick is the timer's edge, a variable so the tests can stand in for the
// real wait.
var prPollTick = defaultPRPollTick

func defaultPRPollTick(gen int) tea.Cmd {
	return tea.Tick(prPollInterval, func(time.Time) tea.Msg { return prPollTicked(gen) })
}

// prPollTicked turns the timer going off into the message for its opening.
func prPollTicked(gen int) tea.Msg { return prPollTickMsg{gen: gen} }

// prPollTickMsg is the timer going off; gen is the opening it belongs to.
type prPollTickMsg struct{ gen int }

// prBackgroundMsg is a timer-driven reading coming back. ref says which pull
// request it was of, so one landing after the screen moved on is dropped.
type prBackgroundMsg struct {
	ref string
	pr  gh.PR
	err error
}

// prPolled is one tick. The timer stops (is not rescheduled) once the screen
// is left or a later opening has taken over. While it runs the read is passed
// over — but the timer kept — when the merge prompt is up, a read is already in
// flight (manual or background), or the screen has nothing loaded to refresh.
func (a *App) prPolled(msg prPollTickMsg) tea.Cmd {
	if msg.gen != a.prPollGen || a.screen != screenPR {
		return nil
	}
	next := prPollTick(a.prPollGen)
	if a.prPollBusy || a.prview.Prompting() || a.prview.Busy() || a.prViewer == nil || !a.prview.Loadable() {
		return next
	}
	_, ref, dir := a.prview.Target()
	a.prPollBusy = true
	viewer := a.prViewer
	read := func() tea.Msg {
		pr, err := viewer.ViewPR(dir, ref)
		return prBackgroundMsg{ref: ref, pr: pr, err: err}
	}
	return tea.Batch(next, read)
}

// prBackgroundLoaded lands a timer-driven reading. A failure is logged and the
// last good reading stays (unlike the manual read, which drops it on an
// explicit ask). A result for another pull request, a screen since left, or a
// manual read now in flight is dropped.
func (a *App) prBackgroundLoaded(msg prBackgroundMsg) {
	a.prPollBusy = false
	_, ref, _ := a.prview.Target()
	if msg.ref != ref || a.screen != screenPR || a.prview.Busy() {
		return
	}
	if msg.err != nil {
		logging.Error("background pull request read failed", "ref", msg.ref, "error", msg.err)
		return
	}
	a.prview.Refresh(msg.pr)
}

// startPRLoad reads the pull request the screen is already showing again, which
// is what the refresh key does while it is up: a review left, a check that
// finished or a merge is exactly when a second read is wanted. It returns nil
// when the screen has never been pointed at one, and so has nothing to read
// again.
func (a *App) startPRLoad() tea.Cmd {
	if a.prViewer == nil || !a.prview.Loadable() {
		return nil
	}
	slice, ref, dir := a.prview.Target()
	a.prview.Start(a.prview.SliceID(), slice, ref, dir)
	return tea.Batch(a.spinner.Tick, readPR(a.prViewer, ref, dir))
}

// readPR runs gh in the slice's repository and comes back with the pull request
// it printed.
func readPR(viewer PRViewer, ref, dir string) tea.Cmd {
	return func() tea.Msg {
		pr, err := viewer.ViewPR(dir, ref)
		if err != nil {
			return prViewLoadedMsg{err: fmt.Errorf("read the pull request %s: %w", ref, err)}
		}
		return prViewLoadedMsg{pr: pr}
	}
}

// prViewLoaded shows the pull request that came back, or the failure that came
// instead. A failure is the screen's own state rather than the app's error: the
// slice and its pull request are still there, and the way out is the refresh
// key or esc.
func (a *App) prViewLoaded(msg prViewLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.prview.Fail(msg.err)
		return a, nil
	}
	a.prview.SetPR(msg.pr)
	return a, nil
}

// prHeading is what the layout's header calls the pull request screen: the
// number of the pull request on show, which is what names one. Before one has
// come back there is only the screen's own name — a read in flight has no
// number yet, and neither has one that failed.
func (a *App) prHeading() string {
	if n := a.prview.Number(); n > 0 {
		return fmt.Sprintf("PR #%d", n)
	}
	return "PR"
}
