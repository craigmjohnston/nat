package tui

import (
	"errors"
	"reflect"
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
	// donePR is a slice already marked Done whose pull request is still open,
	// and mergedPR one whose pull request has landed.
	donePR   = "dn"
	mergedPR = "mg"
)

// The repositories that plan spans: the project's own default, and the one
// repo override on it.
const (
	natRepo   = "/repos/nat"
	otherRepo = "/repos/other"
)

// fakePRReader stands in for the GitHub CLI, recording which repositories it
// was asked to list and answering with whatever the test wants GitHub to have
// open in each.
type fakePRReader struct {
	open  map[string]map[string]gh.PRStatus
	err   error
	asked []string
}

var _ PRReader = (*fakePRReader)(nil)

func (f *fakePRReader) OpenPRs(dir string) (map[string]gh.PRStatus, error) {
	f.asked = append(f.asked, dir)
	if f.err != nil {
		return nil, f.err
	}
	return f.open[dir], nil
}

// prStatePlan is a plan with a pull request in each state worth reading, plus
// the slices there is nothing to read about: one handed back on a branch alone,
// one still Todo, and one Todo carrying the pull request of a round it already
// went.
func prStatePlan() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Review"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: approvedPR, Name: "Approved", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/1"},
			{ID: unreviewedPR, Name: "Unreviewed", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/2"},
			{ID: ownRepoPR, Name: "Own repo", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/3", Repo: otherRepo},
			{ID: "hb", Name: "Handed back", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Review", Branch: "slice/handed-back"},
			{ID: "td", Name: "Not started", Status: domain.SliceTodo, MilestoneID: "M1: Review"},
			{ID: "tp", Name: "Round again", Status: domain.SliceTodo, MilestoneID: "M1: Review",
				PRURL: "https://github.test/pr/6"},
			{ID: donePR, Name: "Awaiting merge", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/4"},
			{ID: mergedPR, Name: "Landed", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/5"},
		})
}

// sliceByID is a slice of the plan by the ID the test names it with, so a test
// about one slice does not depend on where in the plan it sits.
func sliceByID(t *testing.T, p domain.Project, id string) domain.Slice {
	t.Helper()
	for _, s := range p.Slices {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("the plan holds no slice %q", id)
	return domain.Slice{}
}

// prStateApp returns an app showing that plan with a fake gh behind it. GitHub
// has four of the plan's five pull requests open — the first approved and
// mergeable, the rest still waiting — and the fifth, the one on the Landed
// slice, is not open at all.
func prStateApp() (*App, *fakePRReader) {
	cfg := testConfig()
	project := cfg.Projects[testProjectID]
	project.WorkingDir = natRepo
	cfg.Projects[testProjectID] = project

	app := NewApp(cfg, &fakeNotion{})
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		natRepo: {
			"https://github.test/pr/1": {Approved: true, Mergeable: true},
			"https://github.test/pr/2": {Mergeable: true},
			"https://github.test/pr/4": {Mergeable: true},
		},
		otherRepo: {
			"https://github.test/pr/3": {Approved: true, Mergeable: true},
		},
	}}
	app.prReader = reader
	return app, reader
}

// runPRRead takes the reading the app started and hands the result back to it,
// which is the round trip a landed plan makes through the event loop. A plan
// landing does more than read gh — it sweeps up the worktrees of the pull
// requests that have already landed — so everything the load produced is
// threaded back, and so is everything the reading itself sets off.
func runPRRead(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no reading was started")
	}
	msgs := run(cmd)
	first[prStateMsg](t, msgs)
	for _, msg := range msgs {
		_, next := a.Update(msg)
		drive(t, a, next)
	}
}

// activeSection is the Active panel's own lines: the section is a panel beside
// the plan rather than rows of it, so what it holds is read from here.
func activeSection(a *App) string { return strings.Join(a.board.ActiveLines(), "\n") }

// TestPRStatesReadOnEveryPlanThatLands is the whole flow: a plan landing is
// what takes the reading — the board has no timer of its own for it — and it is
// one listing per repository the plan spans rather than one reading per slice.
func TestPRStatesReadOnEveryPlanThatLands(t *testing.T) {
	app, reader := prStateApp()
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	// The project's default repository, then the one slice's own override —
	// once each, however many of the plan's pull requests are in them.
	want := []string{natRepo, otherRepo}
	if !reflect.DeepEqual(reader.asked, want) {
		t.Errorf("gh listed %v, want %v", reader.asked, want)
	}

	// An approved and mergeable pull request is a review that is over; one
	// nobody has approved is one still to come, exactly as it read before.
	if got := app.board.state(sliceByID(t, p, approvedPR)); got != domain.SliceStateReadyToMerge {
		t.Errorf("the approved slice is %v, want ready to merge", got)
	}
	if got := app.board.state(sliceByID(t, p, unreviewedPR)); got != domain.SliceStateAwaitingReview {
		t.Errorf("the unreviewed slice is %v, want awaiting review", got)
	}
	if got := app.board.state(sliceByID(t, p, ownRepoPR)); got != domain.SliceStateReadyToMerge {
		t.Errorf("the slice in its own repo is %v, want ready to merge", got)
	}
	if section := activeSection(app); !strings.Contains(section, domain.SliceStateReadyToMerge.String()) {
		t.Errorf("the section says nothing about the review being over:\n%s", section)
	}
}

// A slice marked Done stays in the Active section for as long as gh says its
// pull request is open: the board marks a slice Done as it opens the pull
// request, and the work is not on main until that merges.
func TestADoneSliceStaysActiveWhileItsPRIsOpen(t *testing.T) {
	app, _ := prStateApp()
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	if got := app.board.state(sliceByID(t, p, donePR)); got != domain.SliceStateAwaitingReview {
		t.Errorf("the Done slice with an open pull request is %v, want awaiting review", got)
	}
	if section := activeSection(app); !strings.Contains(section, "Awaiting merge") {
		t.Errorf("the section left out a Done slice whose pull request is open:\n%s", section)
	}
}

// And drops out the moment that pull request has landed — which is the listing
// not naming it — never to be asked about again, since a merged pull request
// does not unmerge.
func TestADoneSliceDropsOutOnceItsPRHasLanded(t *testing.T) {
	app, reader := prStateApp()
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	if got := app.board.state(sliceByID(t, p, mergedPR)); got != domain.SliceStateNone {
		t.Errorf("the merged slice is %v, want no state at all", got)
	}
	if section := activeSection(app); strings.Contains(section, "Landed") {
		t.Errorf("the section kept a slice whose pull request has merged:\n%s", section)
	}
	if !app.prSettled[mergedPR] {
		t.Error("the merged pull request was not remembered as settled")
	}

	// The next plan to land asks about the repositories again, but no longer
	// about that slice: its answer cannot change.
	reader.open[natRepo]["https://github.test/pr/5"] = gh.PRStatus{}
	_, cmd = app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)
	if got := app.board.state(sliceByID(t, p, mergedPR)); got != domain.SliceStateNone {
		t.Errorf("the settled slice is %v, want it left out for good", got)
	}
}

// With every pull request settled there is nothing left to ask, and the whole
// reading is skipped — which is what a mature plan costs once its work is in.
func TestNothingLeftToAskTakesNoReading(t *testing.T) {
	app, reader := prStateApp()
	p := domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Review"}, notion.TypeSelect),
		[]domain.Slice{{ID: mergedPR, Name: "Landed", Status: domain.SliceDone, StatusName: "Done",
			MilestoneID: "M1: Review", PRURL: "https://github.test/pr/5"}})

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)
	if len(reader.asked) != 1 {
		t.Fatalf("gh listed %v, want the one repository read once", reader.asked)
	}
	if cmd := app.refreshPRStates(); cmd != nil {
		t.Error("refreshPRStates() = a command, want nothing left to ask about")
	}
}

// A gh that fails changes nothing: the slices keep the states they had, the
// board is not put into an error and nothing is toasted — the failure is in the
// log and nowhere else. Above all it settles nothing, since a listing that
// never happened says nothing about what has landed.
func TestPRStateFailureLeavesTheBoardAsItWas(t *testing.T) {
	app, reader := prStateApp()
	reader.err = errors.New("gh: not authenticated")
	p := prStatePlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	if got := app.board.state(sliceByID(t, p, approvedPR)); got != domain.SliceStateAwaitingReview {
		t.Errorf("the slice is %v, want it left at awaiting review", got)
	}
	if got := app.board.state(sliceByID(t, p, donePR)); got != domain.SliceStateNone {
		t.Errorf("the Done slice is %v, want it out with nothing read of it", got)
	}
	if len(app.prSettled) != 0 {
		t.Errorf("prSettled = %v, want a failed listing to settle nothing", app.prSettled)
	}
	if app.err != nil || app.toast != "" {
		t.Errorf("err = %v, toast = %q, want the failure kept to the log", app.err, app.toast)
	}
	if !reflect.DeepEqual(reader.asked, []string{natRepo, otherRepo}) {
		t.Errorf("gh listed %v, want every repository asked about regardless", reader.asked)
	}
}

// A reading that comes back about a slice the plan has since taken off the
// board says nothing about it: there is nothing on the board for it to refine.
func TestPRStateOfASliceNoLongerOnThePlan(t *testing.T) {
	app, _ := prStateApp()
	p := prStatePlan()
	app.Update(prStateMsg{state: map[string]domain.PRReadiness{"gone": domain.PRReadyToMerge}})
	app.board.SetProject(&p)

	if got := app.board.state(sliceByID(t, p, mergedPR)); got != domain.SliceStateNone {
		t.Errorf("the finished slice is %v, want no state at all", got)
	}
}

// A reading that changes what the Active section holds rebuilds the rows under
// the cursor, so it is put back on whatever it was on: the slice, for an entry
// of the section, and the row itself for anything in the plan below it.
func TestPRStateReadingKeepsTheCursorWhereItWas(t *testing.T) {
	p := prStatePlan()
	b := NewBoard(DefaultStyles())
	b.SetProject(&p)
	open := map[string]domain.PRReadiness{donePR: domain.PRAwaitingReview}

	// On an entry of the section: the entry it is on outlives the reading, so
	// the cursor is on the same slice however the rows moved.
	b.cursorTo(func(r row) bool { return r.kind == rowActive && b.active[r.slice].ID == "hb" })
	b.SetPRState(open)
	if s, ok := b.SelectedActive(); !ok || s.ID != "hb" {
		t.Errorf("the cursor is on %+v, want it left on the slice it was on", b.rows[b.cursor])
	}

	// On a row of the plan, with an entry appearing above it: the row moves down
	// the board and the cursor moves with it.
	b.cursorTo(func(r row) bool { return r.kind == rowMilestone })
	was := b.cursor
	b.SetPRState(nil)
	if r := b.rows[b.cursor]; r.kind != rowMilestone {
		t.Errorf("the cursor is on %+v, want it back on the milestone's own row", r)
	}
	if b.cursor == was {
		t.Error("the milestone's row did not move, want the reading to have taken an entry away")
	}

	// And on an entry whose slice has left the section — its pull request has
	// landed — the cursor stays where that entry was.
	b.SetPRState(open)
	b.cursorTo(func(r row) bool { return r.kind == rowActive && b.active[r.slice].ID == donePR })
	was = b.cursor
	b.SetPRState(nil)
	if b.cursor != was {
		t.Errorf("the cursor moved to %d, want it left at %d where the entry was", b.cursor, was)
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
				t.Errorf("gh listed %v, want nothing asked", reader.asked)
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
		t.Error("gh listed nothing for a migrated project")
	}
}

// worthReading is which slices have a pull request anything might still be
// waiting on.
func TestWorthReading(t *testing.T) {
	tests := []struct {
		name  string
		slice domain.Slice
		want  bool
	}{
		{"in progress with a PR", domain.Slice{Status: domain.SliceClaimed, PRURL: "u"}, true},
		{"done with a PR", domain.Slice{Status: domain.SliceDone, PRURL: "u"}, true},
		{"todo with a PR of a round it already went",
			domain.Slice{Status: domain.SliceTodo, PRURL: "u"}, false},
		{"in progress with no PR", domain.Slice{Status: domain.SliceClaimed}, false},
		{"done with no PR", domain.Slice{Status: domain.SliceDone}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worthReading(tt.slice); got != tt.want {
				t.Errorf("worthReading(%+v) = %v, want %v", tt.slice, got, tt.want)
			}
		})
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
