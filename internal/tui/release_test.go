package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The plan the release tests work on: one milestone holding a slice of each
// state the key has an answer for.
const (
	abandoned  = "ab"
	notStarted = "td"
	allDone   = "dn"
)

// releasePlan is that plan. The abandoned slice carries a branch, so the tests
// can say the release leaves one alone rather than only that it writes no PR.
func releasePlan() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Recovery"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: abandoned, Name: "Release action", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M1: Recovery", AssigneeName: "Craig Johnston", Branch: "slice/release"},
			{ID: notStarted, Name: "Startup checks", Status: domain.SliceTodo, MilestoneID: "M1: Recovery"},
			{ID: allDone, Name: "Hand back", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Recovery"},
		})
}

// releaseApp returns an app showing that plan, with the Notion fake behind it.
func releaseApp(t *testing.T) (*App, *fakeNotion) {
	t.Helper()
	client := &fakeNotion{
		// A page as Notion hands it back: every column of the data source,
		// including the Assignee the release clears.
		getPage: func(id string) (*notion.Page, error) {
			return &notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
				notion.PropStatus:   notion.NewSelect(notion.SliceInProgress),
				notion.PropAssignee: notion.NewPeople("u1"),
			}}, nil
		},
	}
	app := NewApp(testConfig(), client)
	p := releasePlan()
	app.project = &p
	app.board.hideDone = false // the Done slice is a row the refusals need
	app.board.SetProject(&p)
	return app, client
}

// release presses R on the row the cursor is on and takes the prompt's first
// choice, which is the one that hands the slice back.
func release(t *testing.T, a *App) {
	t.Helper()
	feed(t, a, press(a, "R"))
	if !a.board.Prompting() {
		t.Fatalf("no release prompt opened: %s", a.board.confirmText)
	}
	drive(t, a, press(a, "enter"))
}

// TestReleaseHandsTheSliceBack is the whole action: one line on the page, the
// status back to Todo, the assignee cleared, and nothing else written.
func TestReleaseHandsTheSliceBack(t *testing.T) {
	app, client := releaseApp(t)
	cursorOn(t, app, abandoned)

	release(t, app)

	if len(client.appended) != 1 || client.appended[0].pageID != abandoned {
		t.Fatalf("appended %+v, want one note on the slice", client.appended)
	}
	if got := blockText(t, client.appended[0].children); !strings.Contains(got, "Released back to Todo by Craig Johnston") {
		t.Errorf("note = %q, want it to say who released it", got)
	}
	if len(client.updated) != 1 || client.updated[0].pageID != abandoned {
		t.Fatalf("wrote %+v, want exactly the slice", client.updated)
	}
	props := client.updated[0].properties
	if status := props[notion.PropStatus]; status.Select == nil || status.Select.Name != notion.SliceTodo {
		t.Errorf("Status = %+v, want the select shape saying Todo", status)
	}
	assignee, cleared := props[notion.PropAssignee]
	if !cleared || assignee.People == nil || len(*assignee.People) != 0 {
		t.Errorf("Assignee = %+v, want the empty list that clears it", assignee)
	}
	// The brief, the branch and the dependencies are what the next session
	// picks the slice up with, so the write says nothing about any of them.
	for _, left := range []string{notion.PropBranch, notion.PropPR, notion.PropDependsOn, notion.PropRepo} {
		if _, wrote := props[left]; wrote {
			t.Errorf("props = %+v, want %s left alone", props, left)
		}
	}
	if !strings.Contains(app.board.confirmText, "Release action") {
		t.Errorf("confirmation = %q, want it to name the slice", app.board.confirmText)
	}
	if app.busy {
		t.Error("the board is still busy after the release landed")
	}
}

// A Status column converted to Notion's own status type in the UI is written
// back in that shape, which is why the page is read before the write.
func TestReleaseWritesTheStatusShapeItRead(t *testing.T) {
	app, client := releaseApp(t)
	client.getPage = func(id string) (*notion.Page, error) {
		return &notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
			notion.PropStatus: {Type: notion.TypeStatus, Status: &notion.SelectOption{Name: notion.SliceInProgress}},
		}}, nil
	}
	cursorOn(t, app, abandoned)

	release(t, app)

	status := client.updated[0].properties[notion.PropStatus]
	if status.Status == nil || status.Status.Name != notion.SliceTodo || status.Select != nil {
		t.Errorf("Status = %+v, want the status shape saying Todo", status)
	}
}

// A project whose Slices table has no Assignee column has nobody to clear, and
// a write naming a column that is not there is one Notion refuses outright.
func TestReleaseWithoutAnAssigneeColumn(t *testing.T) {
	app, client := releaseApp(t)
	client.getPage = func(id string) (*notion.Page, error) {
		return &notion.Page{ID: id, Properties: map[string]notion.PropertyValue{
			notion.PropStatus: notion.NewSelect(notion.SliceInProgress),
		}}, nil
	}
	cursorOn(t, app, abandoned)

	release(t, app)

	if _, wrote := client.updated[0].properties[notion.PropAssignee]; wrote {
		t.Errorf("props = %+v, want no assignee written at all", client.updated[0].properties)
	}
}

// TestReleaseCancelled takes the prompt's other choice: nothing is written.
func TestReleaseCancelled(t *testing.T) {
	app, client := releaseApp(t)
	cursorOn(t, app, abandoned)

	feed(t, app, press(app, "R"))
	feed(t, app, press(app, "right"))
	drive(t, app, press(app, "enter"))

	if len(client.updated) != 0 || len(client.appended) != 0 {
		t.Errorf("cancelling wrote %+v and %+v", client.updated, client.appended)
	}
	if app.board.Prompting() {
		t.Error("the prompt is still up after it was answered")
	}
}

// TestReleaseAbandoned covers esc, which leaves without an answer at all.
func TestReleaseAbandoned(t *testing.T) {
	app, client := releaseApp(t)
	cursorOn(t, app, abandoned)

	feed(t, app, press(app, "R"))
	feed(t, app, press(app, "esc"))

	if app.board.Prompting() {
		t.Error("esc left the prompt up")
	}
	if len(client.updated) != 0 {
		t.Errorf("esc wrote %+v", client.updated)
	}
}

// TestReleaseRefusals covers the rows the key has nothing to do on: it says why
// on the row and opens no prompt, so nothing is released by mistake.
func TestReleaseRefusals(t *testing.T) {
	tests := []struct {
		name   string
		set    func(a *App)
		reason string
	}{
		{"on a milestone", func(a *App) { cursorOnMilestone(t, a) }, "Move to a slice"},
		{"a Todo slice", func(a *App) { cursorOn(t, a, notStarted) }, "only a slice in progress"},
		{"a Done slice", func(a *App) { cursorOn(t, a, allDone) }, "only a slice in progress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, client := releaseApp(t)
			tt.set(app)

			feed(t, app, press(app, "R"))

			if app.board.Prompting() {
				t.Fatal("a prompt opened on a row that cannot be released")
			}
			if !strings.Contains(app.board.confirmText, tt.reason) {
				t.Errorf("said %q, want it to mention %q", app.board.confirmText, tt.reason)
			}
			if len(client.updated) != 0 {
				t.Errorf("wrote %+v", client.updated)
			}
		})
	}
}

// A slice with an agent still running on it is refused: releasing one out from
// under a working session is how two sessions end up on one branch. The board
// says so in a toast, since nothing has gone wrong — the way out is to stop the
// agent, and the slice is still there to release afterwards.
func TestReleaseRefusesASliceWithALiveAgent(t *testing.T) {
	app, client := releaseApp(t)
	app.live = map[string]string{abandoned: "nat-12345678"}
	cursorOn(t, app, abandoned)

	feed(t, app, press(app, "R"))

	if app.board.Prompting() {
		t.Fatal("a prompt opened on a slice an agent is still running on")
	}
	if !strings.Contains(app.toast, "An agent is still running") {
		t.Errorf("toast = %q, want it to say an agent is running", app.toast)
	}
	if len(client.updated) != 0 || len(client.appended) != 0 {
		t.Errorf("wrote %+v and %+v", client.updated, client.appended)
	}
}

// A write already in flight owns the board: the key does nothing at all rather
// than starting a second one over the top of it.
func TestReleaseWhileBusy(t *testing.T) {
	app, client := releaseApp(t)
	app.busy = true
	cursorOn(t, app, abandoned)

	feed(t, app, press(app, "R"))

	if app.board.Prompting() {
		t.Fatal("a prompt opened while a write was in flight")
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %+v", client.updated)
	}
}

// Each of the three calls the release makes can fail, and the failure names the
// slice rather than the step, since the board's error banner is what shows it.
func TestReleaseFailures(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		set  func(*fakeNotion)
	}{
		{"the page", func(f *fakeNotion) {
			f.getPage = func(string) (*notion.Page, error) { return nil, boom }
		}},
		{"the note", func(f *fakeNotion) {
			f.appendBlock = func(string, []map[string]any) ([]notion.Block, error) { return nil, boom }
		}},
		{"the write", func(f *fakeNotion) {
			f.updatePage = func(string, map[string]notion.PropertyValue) (*notion.Page, error) { return nil, boom }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, client := releaseApp(t)
			tt.set(client)
			cursorOn(t, app, abandoned)

			release(t, app)

			if app.err == nil || !strings.Contains(app.err.Error(), `release "Release action": boom`) {
				t.Fatalf("error = %v, want it to name the slice and the failure", app.err)
			}
			if app.busy {
				t.Error("the board is still busy after the release failed")
			}
		})
	}
}

// blockText is the plain text of a single appended block, which is the whole of
// what a release writes onto a page.
func blockText(t *testing.T, children []map[string]any) string {
	t.Helper()
	if len(children) != 1 {
		t.Fatalf("appended %d blocks, want exactly one", len(children))
	}
	payload, ok := children[0]["paragraph"].(map[string]any)
	if !ok {
		t.Fatalf("block %+v is not a paragraph", children[0])
	}
	spans, ok := payload["rich_text"].([]map[string]any)
	if !ok || len(spans) != 1 {
		t.Fatalf("block %+v has no single rich text span", children[0])
	}
	text, ok := spans[0]["text"].(map[string]any)["content"].(string)
	if !ok {
		t.Fatalf("block %+v has no text content", children[0])
	}
	return text
}
