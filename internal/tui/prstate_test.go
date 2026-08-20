package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The slices of the plan below, by the state their pull request is in.
const (
	approvedPR   = "ap"
	unreviewedPR = "un"
	ownRepoPR    = "or"
)

// fakePRReader stands in for the GitHub CLI, recording what it was asked and
// answering with whatever the test wants GitHub to say about each pull request.
type fakePRReader struct {
	status map[string]gh.PRStatus
	err    error
	asked  []prAsk
}

// prAsk is one reading the board asked for: the repository it was taken in and
// the pull request it was about.
type prAsk struct {
	dir string
	url string
}

var _ PRReader = (*fakePRReader)(nil)

func (f *fakePRReader) PRStatus(dir, url string) (gh.PRStatus, error) {
	f.asked = append(f.asked, prAsk{dir: dir, url: url})
	if f.err != nil {
		return gh.PRStatus{}, f.err
	}
	return f.status[url], nil
}

// prStatePlan is a plan with a pull request in each state worth reading, plus
// the slices there is nothing to read about: one handed back on a branch alone,
// one still Todo, and one already Done with its pull request recorded.
func prStatePlan() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Review"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: approvedPR, Name: "Approved", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/1"},
			{ID: unreviewedPR, Name: "Unreviewed", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/2"},
			{ID: ownRepoPR, Name: "Own repo", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/3", Repo: "/repos/other"},
			{ID: "hb", Name: "Handed back", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", Branch: "slice/handed-back"},
			{ID: "td", Name: "Not started", Status: domain.SliceTodo, MilestoneID: "M1: Review"},
			{ID: "dn", Name: "Finished", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/4"},
		})
}

// prStateApp returns an app showing that plan with a fake gh behind it, which
// reads the first slice's pull request as approved and mergeable and the rest
// as still waiting.
func prStateApp() (*App, *fakePRReader) {
	cfg := testConfig()
	project := cfg.Projects[testProjectID]
	project.WorkingDir = "/repos/nat"
	cfg.Projects[testProjectID] = project

	app := NewApp(cfg, &fakeNotion{})
	reader := &fakePRReader{status: map[string]gh.PRStatus{
		"https://github.test/pr/1": {Approved: true, Mergeable: true},
		"https://github.test/pr/2": {Mergeable: true},
		"https://github.test/pr/3": {Approved: true, Mergeable: true},
	}}
	app.prReader = reader
	return app, reader
}

// runPRRead takes the reading the app started and hands the result back to it,
// which is the round trip a landed plan makes through the event loop.
func runPRRead(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no reading was started")
	}
	msg, ok := cmd().(prStateMsg)
	if !ok {
		t.Fatalf("the reading came back as %T, want a prStateMsg", msg)
	}
	a.Update(msg)
}

// TestPRStatesReadOnEveryPlanThatLands is the whole flow: a plan landing is
// what takes the reading — the board has no timer of its own for it — and every
// slice whose work is out is asked about, in its own repository, by the URL
// recorded on it.
func TestPRStatesReadOnEveryPlanThatLands(t *testing.T) {
	app, reader := prStateApp()
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	want := []prAsk{
		{dir: "/repos/nat", url: "https://github.test/pr/1"},
		{dir: "/repos/nat", url: "https://github.test/pr/2"},
		// The slice's own repo override, not the project's default.
		{dir: "/repos/other", url: "https://github.test/pr/3"},
	}
	if len(reader.asked) != len(want) {
		t.Fatalf("gh was asked %+v, want %+v", reader.asked, want)
	}
	for i, ask := range want {
		if reader.asked[i] != ask {
			t.Errorf("reading %d = %+v, want %+v", i, reader.asked[i], ask)
		}
	}

	// An approved and mergeable pull request is a review that is over; one
	// nobody has approved is one still to come, exactly as it read before.
	if got := app.board.state(p.Slices[0]); got != domain.SliceStateReadyToMerge {
		t.Errorf("the approved slice is %v, want ready to merge", got)
	}
	if got := app.board.state(p.Slices[1]); got != domain.SliceStateAwaitingReview {
		t.Errorf("the unreviewed slice is %v, want awaiting review", got)
	}
	if !strings.Contains(app.board.View(), domain.SliceStateReadyToMerge.String()) {
		t.Errorf("the section says nothing about the review being over:\n%s", app.board.View())
	}
}

// A gh that fails changes nothing: the slice keeps the state it had, the board
// is not put into an error and nothing is toasted — the failure is in the log
// and nowhere else.
func TestPRStateFailureLeavesTheBoardAsItWas(t *testing.T) {
	app, reader := prStateApp()
	reader.err = errors.New("gh: not authenticated")
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	if got := app.board.state(p.Slices[0]); got != domain.SliceStateAwaitingReview {
		t.Errorf("the slice is %v, want it left at awaiting review", got)
	}
	if app.err != nil || app.toast != "" {
		t.Errorf("err = %v, toast = %q, want the failure kept to the log", app.err, app.toast)
	}
	if len(reader.asked) != 3 {
		t.Errorf("gh was asked %d times, want every slice asked about regardless", len(reader.asked))
	}
}

// A reading that comes back after the pull request has been merged and the
// slice moved on says nothing about it: the classifier ignores a slice that is
// no longer in flight, whatever GitHub last said.
func TestPRStateOfASliceNoLongerInFlight(t *testing.T) {
	app, _ := prStateApp()
	p := prStatePlan()
	app.Update(prStateMsg{state: map[string]domain.PRReadiness{"dn": domain.PRReadyToMerge}})
	app.board.SetProject(&p)

	if got := app.board.state(p.Slices[5]); got != domain.SliceStateNone {
		t.Errorf("the finished slice is %v, want no state at all", got)
	}
}

// The readings that are not worth taking: there is no gh to ask, no plan to ask
// about, no project to resolve a repository from, nothing on the plan with a
// pull request on it, or a reading already in flight — which is what keeps a
// slow gh from being started again by the next plan that lands.
func TestPRStatesNotRead(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(*App)
	}{
		{"no gh to run", func(a *App) { a.prReader = nil }},
		{"no plan", func(a *App) { a.project = nil }},
		{"no active project", func(a *App) { a.cfg = config.Config{} }},
		{"nothing with a pull request", func(a *App) {
			p := domain.NewProject(testProjectID, "tracker",
				domain.MilestonesFromOptions([]string{"M1: Review"}, notion.TypeSelect),
				[]domain.Slice{{ID: "td", Name: "Not started", Status: domain.SliceTodo,
					MilestoneID: "M1: Review"}})
			a.project = &p
		}},
		{"a reading already in flight", func(a *App) { a.prReading = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, reader := prStateApp()
			p := prStatePlan()
			app.project = &p
			tt.setUp(app)

			if cmd := app.refreshPRStates(); cmd != nil {
				t.Errorf("refreshPRStates() = a command, want no reading taken")
			}
			if len(reader.asked) != 0 {
				t.Errorf("gh was asked %+v, want nothing asked", reader.asked)
			}
		})
	}
}

// One reading at a time, and the next plan to land after it takes another: the
// bit that holds a second off is dropped by the reading coming back.
func TestPRStateReadingRunsOneAtATime(t *testing.T) {
	app, _ := prStateApp()
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	if !app.prReading {
		t.Fatal("a reading is in flight, want it marked as such")
	}
	if _, second := app.Update(projectLoadedMsg{project: p}); second != nil {
		t.Error("a second plan started another reading over the first")
	}
	runPRRead(t, app, cmd)
	if app.prReading {
		t.Error("the reading came back, want the way clear for the next")
	}
	if _, third := app.Update(projectLoadedMsg{project: p}); third == nil {
		t.Error("the next plan to land took no reading")
	}
}

// A migrated project says so and reads its pull requests: the toast and the
// reading are both what that one landing is worth.
func TestPRStatesReadAlongsideAMigrationToast(t *testing.T) {
	app, reader := prStateApp()
	_, cmd := app.Update(projectLoadedMsg{project: prStatePlan(),
		migration: notion.Migration{StatusRenamed: true}})
	if cmd == nil {
		t.Fatal("a migrated project took no reading")
	}
	cmd()
	if len(reader.asked) == 0 {
		t.Error("gh was asked nothing about a migrated project's pull requests")
	}
}

// readinessOf is the one place GitHub's own answer becomes the rule's: only
// approved and mergeable is a review that is over.
func TestReadinessOf(t *testing.T) {
	tests := []struct {
		status gh.PRStatus
		want   domain.PRReadiness
	}{
		{gh.PRStatus{Approved: true, Mergeable: true}, domain.PRReadyToMerge},
		{gh.PRStatus{Approved: true}, domain.PRAwaitingReview},
		{gh.PRStatus{Mergeable: true}, domain.PRAwaitingReview},
		{gh.PRStatus{}, domain.PRAwaitingReview},
	}
	for _, tt := range tests {
		if got := readinessOf(tt.status); got != tt.want {
			t.Errorf("readinessOf(%+v) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// The real reader is the gh on PATH, which is what the app is built with.
func TestDefaultPRReader(t *testing.T) {
	if _, ok := defaultPRReader().(gh.CLI); !ok {
		t.Errorf("defaultPRReader() = %T, want the gh CLI", defaultPRReader())
	}
}
