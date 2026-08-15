package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// steppingClock stands a clock in that moves step forward per reading, so the
// tests can tell the stamp a sync counts from — its first reading, when the
// query went out — from any reading taken later.
func steppingClock(t *testing.T, start time.Time, step time.Duration) {
	t.Helper()
	prev := timeNow
	current := start
	timeNow = func() time.Time {
		c := current
		current = current.Add(step)
		return c
	}
	t.Cleanup(func() { timeNow = prev })
}

// syncPage is slicePage in the shape a query response takes: the title
// carrying its plain_text, which is the field a read decodes — slicePage
// builds the write shape, whose title reads back empty.
func syncPage(id, name, status, milestone string) notion.Page {
	p := slicePage(id, name, status, milestone)
	p.Properties[notion.PropName] = notion.PropertyValue{Title: []notion.RichText{{PlainText: name}}}
	return p
}

// syncClient answers every query with the given pages, recording the filter
// and sorts each was asked with — which is what the selective load is about.
type syncClient struct {
	fakeNotion
	filters []map[string]any
	sorts   [][]notion.Sort
}

func newSyncClient(pages ...notion.Page) *syncClient {
	c := &syncClient{}
	c.query = func(_ string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error) {
		c.filters = append(c.filters, filter)
		c.sorts = append(c.sorts, sorts)
		return pages, nil
	}
	return c
}

// newSyncedApp is an app showing testProject with a landed load's stamp on it,
// which is what a selective run counts from.
func newSyncedApp(client NotionAPI, at time.Time) *App {
	a := newWriteApp(client)
	a.syncedAt = at
	return a
}

func TestAppSelectiveLoadQueriesOnlyWhatChanged(t *testing.T) {
	fixClock(t, refreshedAt)
	client := newSyncClient()
	since := refreshedAt.Add(-5 * time.Minute)
	app := newSyncedApp(client, since)

	feed(t, app, app.startSelectiveLoad())

	// One filtered query and nothing else: no schema read — the milestones in
	// the model are reused — no view order, and no page fetches.
	if !equal(client.queriedDSIDs, []string{"sl-ds"}) {
		t.Errorf("queried %v, want the one slices query", client.queriedDSIDs)
	}
	if client.fetchedDSs != nil || client.orderedDSIDs != nil || client.fetchedPages != nil {
		t.Errorf("schema %v, order %v, pages %v — want the filtered query alone",
			client.fetchedDSs, client.orderedDSIDs, client.fetchedPages)
	}
	if want := notion.EditedOnOrAfter(since); !reflect.DeepEqual(client.filters, []map[string]any{want}) {
		t.Errorf("filters = %v, want %v", client.filters, want)
	}
	if client.sorts[0] != nil {
		t.Errorf("sorts = %v, want none — the merge keeps the plan's own order", client.sorts[0])
	}
	if app.loading {
		t.Error("a selective run is not a full load, and must not blank the board")
	}
}

func TestAppSelectiveSyncMergesAMovedSlice(t *testing.T) {
	start := refreshedAt
	steppingClock(t, start, time.Minute)
	// s5 comes back claimed and refiled from M2: Board to M1: Config.
	client := newSyncClient(syncPage("s5", "Info view", notion.SliceInProgress, "M1: Config"))
	app := newSyncedApp(client, start.Add(-time.Hour))

	feed(t, app, app.startSelectiveLoad())

	if i := sliceIndex(app.project.Slices, "s5"); i != 4 {
		t.Errorf("s5 at %d, want overwritten where it sat", i)
	}
	moved := app.project.Slices[4]
	if moved.MilestoneID != "M1: Config" || moved.Status != domain.SliceClaimed {
		t.Errorf("merged = %+v, want the slice refiled and claimed", moved)
	}
	// The milestone statuses are re-derived from the merged plan: a slice in
	// progress under the finished M1 puts it back in flight.
	if got := milestoneStatus(t, app, "M1: Config"); got != domain.MilestoneActive {
		t.Errorf("M1 = %v, want %v recomputed from the merged plan", got, domain.MilestoneActive)
	}
	// The stamp counts from when the query went out — the clock's first
	// reading — not from the later reading the rebuild took.
	if app.syncedAt != start {
		t.Errorf("syncedAt = %v, want the dispatch stamp %v", app.syncedAt, start)
	}
}

func TestAppSelectiveSyncAppendsAnUnseenSlice(t *testing.T) {
	fixClock(t, refreshedAt)
	client := newSyncClient(syncPage("s9", "From the CLI", notion.SliceTodo, "M2: Board"))
	app := newSyncedApp(client, refreshedAt.Add(-time.Hour))

	feed(t, app, app.startSelectiveLoad())

	if got := len(app.project.Slices); got != 7 {
		t.Fatalf("plan holds %d slices, want the new one appended", got)
	}
	added := app.project.Slices[6]
	if added.ID != "s9" || added.MilestoneID != "M2: Board" {
		t.Errorf("added = %+v, want the unseen slice under its milestone", added)
	}
	if view := stripANSI(app.View().Content); !strings.Contains(view, "From the CLI") {
		t.Errorf("the new row is not drawn:\n%s", view)
	}
}

func TestAppSelectiveSyncWithNothingChangedMovesTheStampAlone(t *testing.T) {
	fixClock(t, refreshedAt)
	client := newSyncClient()
	app := newSyncedApp(client, refreshedAt.Add(-time.Hour))
	app.board.cursor = rowTodoSlice
	app.openPrompt([]string{"yes", "no"}, func(int) tea.Cmd { return nil })

	feed(t, app, app.startSelectiveLoad())

	if app.syncedAt != refreshedAt {
		t.Errorf("syncedAt = %v, want the board confirmed current", app.syncedAt)
	}
	// Nothing changed, so nothing is rebuilt: the prompt stays up and the plan
	// is what it was.
	if !app.board.Prompting() {
		t.Error("an empty sync should not close the prompt under the user")
	}
	if got := len(app.project.Slices); got != 6 {
		t.Errorf("plan holds %d slices, want it untouched", got)
	}
}

func TestAppSelectiveSyncClosesAnOpenPromptWhenRowsMove(t *testing.T) {
	fixClock(t, refreshedAt)
	client := newSyncClient(syncPage("s5", "Info view", notion.SliceDone, "M2: Board"))
	app := newSyncedApp(client, refreshedAt.Add(-time.Hour))
	app.board.cursor = rowTodoSlice
	app.openPrompt([]string{"yes", "no"}, func(int) tea.Cmd { return nil })

	feed(t, app, app.startSelectiveLoad())

	// The merge may have moved or taken away the row the prompt was about,
	// exactly as a landed load may have.
	if app.prompt != nil || app.board.Prompting() {
		t.Error("the prompt should close when the plan is rebuilt under it")
	}
}

func TestAppSelectiveSyncFailureKeepsTheBoardAsItWas(t *testing.T) {
	since := refreshedAt.Add(-time.Hour)
	client := &syncClient{}
	client.query = func(string, map[string]any, []notion.Sort) ([]notion.Page, error) {
		return nil, errors.New("boom")
	}
	app := newSyncedApp(client, since)

	feed(t, app, app.startSelectiveLoad())

	if app.err == nil || app.err.Error() != "sync slices: boom" {
		t.Errorf("err = %v, want the wrapped failure", app.err)
	}
	if app.syncedAt != since {
		t.Errorf("syncedAt = %v, want it left at %v — the board is as old as it was", app.syncedAt, since)
	}
}

func TestAppSelectiveLoadFallsBackToTheFullLoadFirst(t *testing.T) {
	// Before the first load lands there is no stamp to filter from and no plan
	// to merge into, so the full load runs — a selective run can never be the
	// only thing that ever reads the board.
	client := newLoadingClient()
	app := NewApp(testConfig(), client)

	cmd := app.startSelectiveLoad()
	if !app.loading {
		t.Error("the fallback is the full load, which the board reports")
	}
	run(cmd)

	if client.fetchedDSs == nil {
		t.Error("the full load reads the schema; the fallback did not")
	}
}

func TestAppSelectiveLoadYieldsToAFullLoadInFlight(t *testing.T) {
	client := newSyncClient()
	app := newSyncedApp(client, refreshedAt)
	app.loading = true

	if cmd := app.startSelectiveLoad(); cmd != nil {
		t.Error("a full load in flight supersedes a selective run")
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want nothing", client.queriedDSIDs)
	}
}

func TestAppSelectiveLoadNeedsAProjectAndAClient(t *testing.T) {
	// A plan and a stamp but no configured project — switched away mid-flight —
	// leaves nothing to query.
	app := newSyncedApp(&fakeNotion{}, refreshedAt)
	app.cfg.ActiveProjectID = ""
	if cmd := app.startSelectiveLoad(); cmd != nil {
		t.Error("no active project, so nothing to sync")
	}

	app = newSyncedApp(&fakeNotion{}, refreshedAt)
	app.client = nil
	if cmd := app.startSelectiveLoad(); cmd != nil {
		t.Error("no client, so nothing to sync with")
	}
}

func TestAppSelectiveSyncWithoutAPlanChangesNothing(t *testing.T) {
	// The landing can outlive the plan it was for: a project switch clears the
	// board while the query is in flight.
	app := NewApp(testConfig(), &fakeNotion{})

	app.Update(slicesSyncedMsg{slices: []domain.Slice{{ID: "s5"}}, syncedAt: refreshedAt})

	if app.project != nil || !app.syncedAt.IsZero() {
		t.Errorf("project = %v, syncedAt = %v, want nothing to merge into", app.project, app.syncedAt)
	}
}
