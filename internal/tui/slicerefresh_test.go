package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// refreshedAt is the moment the tests pin the clock to, so the freshness stamp
// a patch moves is a fact rather than a race.
var refreshedAt = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

// submitForm fills the slice form the way fillForm does, but finishes it with
// drive rather than feed, so the single-page refetch the finished write kicks
// off lands the way it would under the runtime.
func submitForm(t *testing.T, a *App, title, brief, repo string) {
	t.Helper()
	typeText(a, title)
	feed(t, a, press(a, "enter"))
	typeText(a, brief)
	feed(t, a, press(a, "tab"))
	typeText(a, repo)
	drive(t, a, press(a, "enter"))
	if a.form != nil {
		t.Fatalf("the form did not finish:\n%s", a.View().Content)
	}
}

// pageFor is a slicePage as GetPage returns one: by pointer, which is what the
// single-slice refresh maps its row from.
func pageFor(id, name, status, milestone string) func(string) (*notion.Page, error) {
	return func(string) (*notion.Page, error) {
		p := slicePage(id, name, status, milestone)
		return &p, nil
	}
}

// milestoneStatus is the status the patched plan computes for a milestone.
func milestoneStatus(t *testing.T, a *App, id string) domain.MilestoneStatus {
	t.Helper()
	for _, m := range a.project.Milestones {
		if m.ID == id {
			return m.Status
		}
	}
	t.Fatalf("no milestone %q in the plan", id)
	return ""
}

func TestAppEditRefreshesJustTheSlice(t *testing.T) {
	fixClock(t, refreshedAt)
	client := &fakeNotion{getPage: pageFor("s5", "Info view, renamed", notion.SliceTodo, "M2: Board")}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	_, opened := app.Update(runMsg(t, press(app, "e")))
	feed(t, app, opened)
	submitForm(t, app, ", renamed", "", "")

	// The row is redrawn from the one refetched page, with no full load: the
	// plan query is never made and the board never goes back to loading.
	if !equal(client.fetchedPages, []string{"s5"}) {
		t.Errorf("fetched %v, want just the edited page", client.fetchedPages)
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want no full reload", client.queriedDSIDs)
	}
	if app.loading {
		t.Error("a patched row is not a plan load")
	}
	if view := stripANSI(app.View().Content); !strings.Contains(view, "Info view, renamed") {
		t.Errorf("the row was not redrawn:\n%s", view)
	}
	if app.board.confirmText != `Updated "Info view, renamed".` {
		t.Errorf("confirm = %q, want the updated confirmation", app.board.confirmText)
	}
	// The board is as current about this page as a reload would have made it.
	if app.syncedAt != refreshedAt {
		t.Errorf("syncedAt = %v, want the stamp moved to %v", app.syncedAt, refreshedAt)
	}
	if got := bar(app); !strings.Contains(got, "synced just now") {
		t.Errorf("status line = %q, want the freshness stamp on it", got)
	}
}

func TestAppAddPatchesTheNewSliceIn(t *testing.T) {
	fixClock(t, refreshedAt)
	// CreatePage answers with the page "new-page", which is the one the patch
	// refetches: a created slice has an ID only Notion knows.
	client := &fakeNotion{getPage: pageFor("new-page", "Brand new", notion.SliceTodo, "M2: Board")}
	app := newWriteApp(client)
	app.board.cursor = rowActiveMilestone

	feed(t, app, press(app, "a"))
	submitForm(t, app, "Brand new", "The brief.", "")

	if !equal(client.fetchedPages, []string{"new-page"}) {
		t.Errorf("fetched %v, want just the created page", client.fetchedPages)
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want no full reload", client.queriedDSIDs)
	}
	if got := len(app.project.Slices); got != 7 {
		t.Fatalf("plan holds %d slices, want the new one patched in", got)
	}
	added := app.project.Slices[6]
	if added.ID != "new-page" || added.MilestoneID != "M2: Board" {
		t.Errorf("added = %+v, want the refetched slice under its milestone", added)
	}
	if view := stripANSI(app.View().Content); !strings.Contains(view, "Brand new") {
		t.Errorf("the new row is not drawn:\n%s", view)
	}
}

func TestAppMoveRefilesTheRowWithoutAFullLoad(t *testing.T) {
	fixClock(t, refreshedAt)
	client := &fakeNotion{getPage: pageFor("s5", "Info view", notion.SliceTodo, "M1: Config")}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "m"))
	// The picker opens on its first target, M1: Config, which the refetched
	// page then names; drive lands the refetch the submitted move kicks off.
	drive(t, app, press(app, "enter"))

	if !equal(client.fetchedPages, []string{"s5"}) {
		t.Errorf("fetched %v, want just the moved page", client.fetchedPages)
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want no full reload", client.queriedDSIDs)
	}
	moved := app.project.Slices[sliceIndex(app.project.Slices, "s5")]
	if moved.MilestoneID != "M1: Config" {
		t.Errorf("milestone = %q, want the slice refiled", moved.MilestoneID)
	}
	// The milestone statuses are re-derived around the patch: a Todo slice
	// under the finished M1 puts it back in flight.
	if got := milestoneStatus(t, app, "M1: Config"); got != domain.MilestoneActive {
		t.Errorf("M1 = %v, want %v recomputed from the patched plan", got, domain.MilestoneActive)
	}
}

func TestAppDeleteRemovesTheRowOutright(t *testing.T) {
	fixClock(t, refreshedAt)
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	feed(t, app, press(app, "d"))
	answerConfirm(t, app, "y")

	// The page is in the trash: nothing is refetched and nothing reloaded.
	if client.fetchedPages != nil || client.queriedDSIDs != nil {
		t.Errorf("fetched %v, queried %v, want the row simply removed", client.fetchedPages, client.queriedDSIDs)
	}
	if i := sliceIndex(app.project.Slices, "s5"); i >= 0 || len(app.project.Slices) != 5 {
		t.Errorf("slices = %d with s5 at %d, want the row gone", len(app.project.Slices), i)
	}
	// The rows are checked by name rather than through the view, whose inline
	// confirmation names the deleted slice itself.
	if rows := strings.Join(rowNames(&app.board), "\n"); strings.Contains(rows, "Info view") {
		t.Errorf("the deleted row is still drawn:\n%s", rows)
	}
	if app.board.confirmText != `Deleted "Info view".` {
		t.Errorf("confirm = %q, want the deleted confirmation", app.board.confirmText)
	}
	if app.syncedAt != refreshedAt {
		t.Errorf("syncedAt = %v, want the stamp moved to %v", app.syncedAt, refreshedAt)
	}
}

func TestAppDeleteOfARowThePlanDoesNotHold(t *testing.T) {
	app := newWriteApp(&fakeNotion{})

	app.Update(sliceSavedMsg{note: "Deleted.", sliceID: "not-there", deleted: true})

	if got := len(app.project.Slices); got != 6 {
		t.Errorf("slices = %d, want the plan untouched", got)
	}
	if !app.syncedAt.IsZero() {
		t.Error("nothing was patched, so the stamp should not move")
	}
}

func TestAppDeleteBeforeTheFirstPlanChangesNothing(t *testing.T) {
	app := NewApp(testConfig(), &fakeNotion{})

	app.Update(sliceSavedMsg{note: "Deleted.", sliceID: "s5", deleted: true})

	if app.project != nil || !app.syncedAt.IsZero() {
		t.Errorf("project = %v, syncedAt = %v, want nothing to take a row off", app.project, app.syncedAt)
	}
}

func TestAppFallsBackToAFullLoadBeforeTheFirstPlan(t *testing.T) {
	// A write can land before the first load has — there is no plan to patch,
	// so the whole of it is fetched instead.
	client := newLoadingClient()
	app := NewApp(testConfig(), client)

	_, cmd := app.Update(sliceSavedMsg{note: "Updated.", sliceID: "s5"})
	run(cmd)

	if client.fetchedPages != nil {
		t.Errorf("fetched %v, want no single-page refetch without a plan", client.fetchedPages)
	}
	if len(client.queriedDSIDs) != 1 {
		t.Errorf("queried %v, want the full load", client.queriedDSIDs)
	}
}

func TestAppReportsAFailedSliceRefresh(t *testing.T) {
	client := &fakeNotion{getPage: func(string) (*notion.Page, error) { return nil, errors.New("boom") }}
	app := newWriteApp(client)

	_, cmd := app.Update(sliceSavedMsg{note: "Updated.", sliceID: "s5"})
	feed(t, app, cmd)

	if app.err == nil || app.err.Error() != "refresh slice: boom" {
		t.Errorf("err = %v, want the wrapped failure", app.err)
	}
	if !app.syncedAt.IsZero() {
		t.Error("a failed refetch left the board as old as it was")
	}
}

func TestAppSliceRefreshWithoutAPlanChangesNothing(t *testing.T) {
	app := NewApp(testConfig(), &fakeNotion{})

	app.Update(sliceRefreshedMsg{slice: domain.Slice{ID: "s5"}})

	if app.project != nil || !app.syncedAt.IsZero() {
		t.Errorf("project = %v, syncedAt = %v, want nothing to patch into", app.project, app.syncedAt)
	}
}

func TestAppSliceRefreshClosesAnOpenPrompt(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowTodoSlice
	app.openPrompt([]string{"yes", "no"}, func(int) tea.Cmd { return nil })

	app.Update(sliceRefreshedMsg{slice: domain.Slice{ID: "s5", Name: "Info view",
		Status: domain.SliceTodo, MilestoneID: "M2: Board"}})

	// The patch may have moved the rows under the prompt, exactly as a landed
	// load may have.
	if app.prompt != nil || app.board.Prompting() {
		t.Error("the prompt should close when the plan is patched under it")
	}
}

func TestAppRefreshesTheSliceTheAgentPaneWasAbout(t *testing.T) {
	// Hiding an agent's pane is the moment the user looks back at the board,
	// and the agent has been working exactly one page: that one is refetched.
	client := &fakeNotion{getPage: pageFor("s5", "Info view", notion.SliceInProgress, "M2: Board")}
	app := newWriteApp(client)
	app.busy = true

	_, cmd := app.Update(agentAttachedMsg{note: "Sent the agent back.", slice: "s5"})
	feed(t, app, cmd)

	if !equal(client.fetchedPages, []string{"s5"}) {
		t.Errorf("fetched %v, want the agent's slice", client.fetchedPages)
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want no full reload", client.queriedDSIDs)
	}
	patched := app.project.Slices[sliceIndex(app.project.Slices, "s5")]
	if patched.Status != domain.SliceClaimed {
		t.Errorf("status = %v, want the claim the agent made picked up", patched.Status)
	}
}
