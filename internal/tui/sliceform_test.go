package tui

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// The rows of the board testProject flattens to, named so the tests read.
// They are the rows with the hide-done toggle off, which is how the fixtures
// below set the board up: a mutation has to be able to address a Done slice.
const (
	rowActiveMilestone = 1 // M2: Board, expanded because it is active
	rowClaimedSlice    = 3 // Board screen
	rowTodoSlice       = 4 // Info view
	rowUnassigned      = 6 // the implicit group, which is not a milestone
)

// newWriteApp returns an app showing testProject, ready for a mutation. The
// plan is set straight rather than loaded, so the tests start on a board with a
// slice of every status on it — the toggle that would hide the Done one off,
// since the mutations have to be able to reach it.
func newWriteApp(client NotionAPI) *App { return newWriteAppOn(client, testProject()) }

// newWriteAppOn is the app both fixtures are built from.
func newWriteAppOn(client NotionAPI, p domain.Project) *App {
	a := NewApp(testConfig(), client)
	a.project = &p
	a.board.hideDone = false
	a.board.SetProject(&p)
	return a
}

// sliceFormOf is the add/edit form on show, failing the test when the modal
// open over the board is something else.
func sliceFormOf(t *testing.T, a *App) *SliceForm {
	t.Helper()
	f, ok := a.form.(*SliceForm)
	if !ok {
		t.Fatalf("form = %T, want the slice form", a.form)
	}
	return f
}

// runMsg runs a command the way the runtime would and returns the single
// message it produced.
func runMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	msgs := run(cmd)
	if len(msgs) != 1 {
		t.Fatalf("command produced %d messages, want 1: %v", len(msgs), msgs)
	}
	return msgs[0]
}

// fillForm types the three answers into the open form and submits it, then
// feeds the write that falls out back through the app, as the runtime would.
// What is typed is appended to whatever the field was opened with, so "" leaves
// a field as it stands.
//
// Fields are stepped over with the keys that do it — enter off the title, tab
// off the multi-line brief where enter is a newline, enter on the last field to
// submit — because a field only writes its answer back to the model when it is
// left that way.
func fillForm(t *testing.T, a *App, title, brief, repo string) {
	t.Helper()
	typeText(a, title)
	feed(t, a, press(a, "enter"))
	typeText(a, brief)
	feed(t, a, press(a, "tab"))
	typeText(a, repo)

	// Submitting runs through messages of huh's own — off the field, then off
	// the group — before the form completes and the write falls out of it.
	finishForm(t, a, press(a, "enter"))
}

// finishForm drives a form that has just been submitted to completion, running
// what it returns and threading the messages back through the app the way the
// runtime does, then dispatching the write that falls out of it.
func finishForm(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	for range 4 {
		if a.form == nil {
			break
		}
		var next []tea.Cmd
		for _, msg := range run(cmd) {
			_, c := a.Update(msg)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
	if a.form != nil {
		t.Fatalf("the form did not finish:\n%s", a.View().Content)
	}
	feed(t, a, cmd)
}

// feed runs a command and threads the messages it produced back through the
// app, the way the runtime does. Starting a form is one of these: its first
// field is focused by a message of its own, and nothing can be typed until it
// has been.
func feed(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	for _, msg := range run(cmd) {
		a.Update(msg)
	}
}

// typeText types into the focused field, one key press at a time. Typing needs
// no follow-up messages, so the commands it returns are dropped.
func typeText(a *App, s string) {
	for _, r := range s {
		press(a, string(r))
	}
}

// paragraphTexts is the text of each paragraph block of a body write.
func paragraphTexts(t *testing.T, blocks []map[string]any) []string {
	t.Helper()
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		encoded, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal block: %v", err)
		}
		var decoded struct {
			Object    string `json:"object"`
			Type      string `json:"type"`
			Paragraph struct {
				RichText []notion.RichText `json:"rich_text"`
			} `json:"paragraph"`
		}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decode block: %v", err)
		}
		if decoded.Object != "block" || decoded.Type != "paragraph" {
			t.Errorf("block %d = %s/%s, want block/paragraph", i, decoded.Object, decoded.Type)
		}
		if len(decoded.Paragraph.RichText) != 1 {
			t.Fatalf("block %d has %d spans, want 1", i, len(decoded.Paragraph.RichText))
		}
		texts[i] = decoded.Paragraph.RichText[0].Text.Content
	}
	return texts
}

func TestParagraphBlocksSplitsOnBlankLines(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        []string
	}{
		{"empty", "", nil},
		{"blank", "  \n\n\t", nil},
		{"one paragraph", "Just the one.", []string{"Just the one."}},
		{"two paragraphs", "First.\n\nSecond.", []string{"First.", "Second."}},
		{"blank runs collapse", "First.\n\n\n\nSecond.", []string{"First.", "Second."}},
		{"lines within a paragraph are kept", "- a\n- b", []string{"- a\n- b"}},
		{"crlf", "First.\r\n\r\nSecond.", []string{"First.", "Second."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paragraphTexts(t, paragraphBlocks(tt.description))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("paragraphs = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateSliceFilesANewTodoSlice(t *testing.T) {
	client := &fakeNotion{}

	msg := runMsg(t, createSlice(client, "sl-ds", domain.Milestone{
		ID: "M2: Board", Name: "M2: Board", SelectType: notion.TypeSelect},
		"  New slice  ", "First.\n\nSecond.", " /tmp/repo "))

	if got := msg.(sliceSavedMsg); got.err != nil || got.note != `Added "New slice".` {
		t.Errorf("msg = %+v, want the created note", got)
	}
	if len(client.created) != 1 {
		t.Fatalf("created %d pages, want 1", len(client.created))
	}
	call := client.created[0]
	if want := notion.DataSourceParent("sl-ds"); call.parent != want {
		t.Errorf("parent = %+v, want %+v", call.parent, want)
	}
	want := map[string]notion.PropertyValue{
		notion.PropName:      notion.NewTitle("New slice"),
		notion.PropStatus:    notion.NewSelect(notion.SliceTodo),
		notion.PropMilestone: notion.NewSelect("M2: Board"),
		notion.PropRepo:      notion.NewRichText("/tmp/repo"),
	}
	if !reflect.DeepEqual(call.properties, want) {
		t.Errorf("properties = %+v, want %+v", call.properties, want)
	}
	if got := paragraphTexts(t, call.children); !reflect.DeepEqual(got, []string{"First.", "Second."}) {
		t.Errorf("body = %q, want the brief as two paragraphs", got)
	}
}

func TestCreateSliceReportsAFailure(t *testing.T) {
	client := &fakeNotion{createPage: func(notion.Parent, map[string]notion.PropertyValue, []map[string]any) (*notion.Page, error) {
		return nil, errors.New("boom")
	}}

	msg := runMsg(t, createSlice(client, "sl-ds", domain.Milestone{ID: "M2: Board", Name: "M2: Board"}, "New slice", "", ""))

	if got := msg.(sliceSavedMsg); got.err == nil || got.err.Error() != "create slice: boom" {
		t.Errorf("err = %v, want the wrapped failure", got.err)
	}
}

func TestEditSliceRewritesThePropertiesAndTheBody(t *testing.T) {
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) {
		return []notion.Block{{ID: "b1"}, {ID: "b2"}}, nil
	}}

	msg := runMsg(t, editSlice(client, "s5", " Renamed ", "Rewritten.", " /tmp/other "))

	if got := msg.(sliceSavedMsg); got.err != nil || got.note != `Updated "Renamed".` {
		t.Errorf("msg = %+v, want the updated note", got)
	}
	if len(client.updated) != 1 {
		t.Fatalf("updated %d pages, want 1", len(client.updated))
	}
	call := client.updated[0]
	want := map[string]notion.PropertyValue{
		notion.PropName: notion.NewTitle("Renamed"),
		notion.PropRepo: notion.NewRichText("/tmp/other"),
	}
	if call.pageID != "s5" || !reflect.DeepEqual(call.properties, want) {
		t.Errorf("update = %+v, want s5 with %+v", call, want)
	}
	if !reflect.DeepEqual(client.deleted, []string{"b1", "b2"}) {
		t.Errorf("deleted = %v, want every existing block", client.deleted)
	}
	if len(client.appended) != 1 || client.appended[0].pageID != "s5" {
		t.Fatalf("appended = %+v, want one write to s5", client.appended)
	}
	if got := paragraphTexts(t, client.appended[0].children); !reflect.DeepEqual(got, []string{"Rewritten."}) {
		t.Errorf("body = %q, want the new brief", got)
	}
}

func TestEditSliceWithABlankBriefLeavesTheBodyEmpty(t *testing.T) {
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) {
		return []notion.Block{{ID: "b1"}}, nil
	}}

	runMsg(t, editSlice(client, "s5", "Renamed", "   ", ""))

	if len(client.deleted) != 1 {
		t.Errorf("deleted = %v, want the old body cleared", client.deleted)
	}
	if len(client.appended) != 0 {
		t.Errorf("appended = %+v, want nothing written back", client.appended)
	}
}

func TestEditSliceReportsFailures(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		client *fakeNotion
		want   string
	}{
		{"properties", &fakeNotion{
			updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) { return nil, boom },
		}, "update slice: boom"},
		{"read", &fakeNotion{
			blocks: func(string) ([]notion.Block, error) { return nil, boom },
		}, "read slice body: boom"},
		{"clear", &fakeNotion{
			blocks:      func(string) ([]notion.Block, error) { return []notion.Block{{ID: "b1"}}, nil },
			deleteBlock: func(string) error { return boom },
		}, "clear slice body: boom"},
		{"write", &fakeNotion{
			appendBlock: func(string, []map[string]any) ([]notion.Block, error) { return nil, boom },
		}, "write slice body: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := runMsg(t, editSlice(tt.client, "s5", "Renamed", "Brief.", ""))

			got := msg.(sliceSavedMsg)
			if got.err == nil || got.err.Error() != tt.want {
				t.Errorf("err = %v, want %q", got.err, tt.want)
			}
		})
	}
}

func TestLoadSliceBodyConvertsThePageToMarkdown(t *testing.T) {
	client := &fakeNotion{blocks: func(id string) ([]notion.Block, error) {
		if id != "s5" {
			t.Errorf("fetched %q, want the selected slice", id)
		}
		return []notion.Block{block(t, "paragraph", "The brief.")}, nil
	}}

	msg := runMsg(t, loadSliceBody(client, domain.Slice{ID: "s5"})).(sliceBodyMsg)

	if msg.err != nil || msg.markdown != "The brief." {
		t.Errorf("msg = %+v, want the body as markdown", msg)
	}
}

func TestLoadSliceBodyReportsAFailure(t *testing.T) {
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) { return nil, errors.New("boom") }}

	msg := runMsg(t, loadSliceBody(client, domain.Slice{ID: "s5"})).(sliceBodyMsg)

	if msg.err == nil || msg.err.Error() != "load slice body: boom" {
		t.Errorf("err = %v, want the wrapped failure", msg.err)
	}
}

// block builds a paragraph-shaped block the way the API returns one.
func block(t *testing.T, blockType, text string) notion.Block {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"id":   "b1",
		"type": blockType,
		blockType: map[string]any{
			"rich_text": []map[string]any{{"type": "text", "plain_text": text}},
		},
	})
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	var b notion.Block
	if err := json.Unmarshal(encoded, &b); err != nil {
		t.Fatalf("decode block: %v", err)
	}
	return b
}

func TestAppAddOpensTheFormOnTheSelectedMilestone(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone

	press(app, "a")

	if app.form == nil || app.screen != screenForm {
		t.Fatalf("screen = %v, form = %v, want the add form on show", app.screen, app.form)
	}
	if f := sliceFormOf(t, app); f.mode != sliceFormAdd || f.milestone.ID != "M2: Board" {
		t.Errorf("form = %+v, want an add form for m2", f)
	}
	view := app.View().Content
	if !strings.Contains(view, "Add a slice to M2: Board") {
		t.Errorf("view is missing the heading:\n%s", view)
	}
	// The form's own way out is on the status line, and no other key is offered.
	if line := app.windowTitle(); !strings.Contains(line, "cancel") {
		t.Errorf("status line = %q, want the form's way out", line)
	}
	if strings.Contains(view, "refresh") {
		t.Errorf("view should offer only the form's own way out:\n%s", view)
	}
}

func TestAppAddNeedsAMilestoneToFileUnder(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
	}{
		{"on a slice", rowTodoSlice},
		{"on the unassigned group", rowUnassigned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWriteApp(&fakeNotion{})
			app.board.cursor = tt.cursor

			press(app, "a")

			if app.form != nil {
				t.Error("a form was opened with no milestone to file under")
			}
			if !strings.Contains(app.board.confirmText, "Move to a milestone") {
				t.Errorf("confirm = %q, want the milestone hint", app.board.confirmText)
			}
		})
	}
}

func TestAppAddWritesTheCompletedForm(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowActiveMilestone

	feed(t, app, press(app, "a"))
	fillForm(t, app, "New slice", "The brief.", "/tmp/repo")

	if app.screen != screenBoard {
		t.Error("the board should be back once the form is submitted")
	}
	if len(client.created) != 1 {
		t.Fatalf("created %d pages, want the slice written", len(client.created))
	}
	props := client.created[0].properties
	if got := props[notion.PropName]; !reflect.DeepEqual(got, notion.NewTitle("New slice")) {
		t.Errorf("title = %+v, want the form's answer", got)
	}
	if got := props[notion.PropRepo]; !reflect.DeepEqual(got, notion.NewRichText("/tmp/repo")) {
		t.Errorf("repo = %+v, want the form's answer", got)
	}
	if got := paragraphTexts(t, client.created[0].children); !reflect.DeepEqual(got, []string{"The brief."}) {
		t.Errorf("body = %q, want the brief that was typed", got)
	}
	if app.board.confirmText != `Added "New slice".` {
		t.Errorf("confirm = %q, want the created confirmation", app.board.confirmText)
	}
}

func TestAppEditLoadsTheBodyThenOpensTheForm(t *testing.T) {
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) {
		return []notion.Block{block(t, "paragraph", "The brief.")}, nil
	}}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	cmd := press(app, "e")
	if !app.busy || app.form != nil {
		t.Fatalf("busy = %v, form = %v, want the fetch in flight", app.busy, app.form)
	}
	app.Update(runMsg(t, cmd))

	if app.busy || app.form == nil || app.screen != screenForm {
		t.Fatalf("screen = %v, form = %v, want the edit form on show", app.screen, app.form)
	}
	f := sliceFormOf(t, app)
	if f.mode != sliceFormEdit || f.sliceID != "s5" || f.title != "Info view" || f.description != "The brief." {
		t.Errorf("form = %+v, want it filled in from the slice", f)
	}
}

func TestAppEditRefusesSlicesThatAreNotTodo(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowClaimedSlice

	press(app, "e")

	if app.busy || app.form != nil {
		t.Error("a claimed slice should not be opened for editing")
	}
	if want := `"Board screen" is In progress — only Todo slices can be edited.`; app.board.confirmText != want {
		t.Errorf("confirm = %q, want %q", app.board.confirmText, want)
	}
}

func TestAppEditNeedsASliceUnderTheCursor(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone

	press(app, "e")

	if !strings.Contains(app.board.confirmText, "Move to a slice") {
		t.Errorf("confirm = %q, want the slice hint", app.board.confirmText)
	}
}

func TestAppEditReportsAFailedLoad(t *testing.T) {
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) { return nil, errors.New("boom") }}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	app.Update(runMsg(t, press(app, "e")))

	if app.busy || app.form != nil {
		t.Error("a failed fetch should leave nothing in flight and no form")
	}
	if app.err == nil || app.err.Error() != "load slice body: boom" {
		t.Errorf("err = %v, want the failure reported", app.err)
	}
}

func TestAppEditWritesTheCompletedForm(t *testing.T) {
	client := &fakeNotion{blocks: func(string) ([]notion.Block, error) {
		return []notion.Block{block(t, "paragraph", "The brief.")}, nil
	}}
	app := newWriteApp(client)
	app.board.cursor = rowTodoSlice

	_, opened := app.Update(runMsg(t, press(app, "e")))
	feed(t, app, opened)
	// Nothing is typed, so the form writes back what it was opened with: the
	// slice's own title and body, which is the round trip that must not lose
	// anything.
	fillForm(t, app, "", "", "")

	if len(client.updated) != 1 || client.updated[0].pageID != "s5" {
		t.Fatalf("updated = %+v, want the slice written", client.updated)
	}
	if got := client.updated[0].properties[notion.PropName]; !reflect.DeepEqual(got, notion.NewTitle("Info view")) {
		t.Errorf("title = %+v, want the slice's own", got)
	}
	if len(client.appended) != 1 {
		t.Fatalf("appended = %+v, want the body rewritten", client.appended)
	}
	if got := paragraphTexts(t, client.appended[0].children); !reflect.DeepEqual(got, []string{"The brief."}) {
		t.Errorf("body = %q, want the body it was opened with", got)
	}
	if app.board.confirmText != `Updated "Info view".` {
		t.Errorf("confirm = %q, want the updated confirmation", app.board.confirmText)
	}
}

func TestAppFormKeysDoNotReachTheBoard(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone
	press(app, "a")

	// q quits the app from the board; inside a form it is just a character.
	if cmd := press(app, "q"); isQuitCmd(cmd) {
		t.Error("q inside the form quit the app")
	}
	if app.form == nil {
		t.Error("the form was dismissed by a key meant for it")
	}
}

func TestAppFormIsCancelledWithEsc(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone
	press(app, "a")

	press(app, "esc")

	if app.form != nil || app.screen != screenBoard {
		t.Errorf("screen = %v, form = %v, want the board back", app.screen, app.form)
	}
	if app.toast != "Cancelled." {
		t.Errorf("toast = %q, want the cancelled toast", app.toast)
	}
}

func TestAppFormReceivesNonKeyMessages(t *testing.T) {
	app := newWriteApp(&fakeNotion{})
	app.board.cursor = rowActiveMilestone
	press(app, "a")

	// Anything the app does not handle itself belongs to the open form.
	app.Update(struct{ tea.Msg }{})

	if app.form == nil {
		t.Error("the form should still be on show")
	}
}

func TestAppReloadsThePlanAfterAWrite(t *testing.T) {
	client := newLoadingClient()
	app := NewApp(testConfig(), client)

	_, cmd := app.Update(sliceSavedMsg{note: "Added."})

	if app.busy {
		t.Error("still busy after the write came back")
	}
	if app.board.confirmText != "Added." {
		t.Errorf("confirm = %q, want the write's confirmation", app.board.confirmText)
	}
	if !app.loading {
		t.Error("the plan should be reloading")
	}
	if got := first[projectLoadedMsg](t, run(cmd)); got.project.ID != testProjectID {
		t.Errorf("reloaded %q, want the active project", got.project.ID)
	}
}

func TestAppReportsAFailedWrite(t *testing.T) {
	app := newWriteApp(&fakeNotion{})

	app.Update(sliceSavedMsg{err: errors.New("create slice: boom")})

	if app.busy || app.note != "" || app.board.confirmText != "" {
		t.Errorf("busy = %v, note = %q, confirm = %q, want the failure to clear them", app.busy, app.note, app.board.confirmText)
	}
	if app.err == nil || app.err.Error() != "create slice: boom" {
		t.Errorf("err = %v, want the failure reported", app.err)
	}
}

func TestAppRefusesWritesItCannotMake(t *testing.T) {
	tests := []struct {
		name string
		app  func() *App
	}{
		{"no client", func() *App { return newWriteApp(nil) }},
		{"busy", func() *App {
			a := newWriteApp(&fakeNotion{})
			a.busy = true
			return a
		}},
		{"no project", func() *App {
			a := NewApp(config.Config{}, &fakeNotion{})
			p := testProject()
			a.board.SetProject(&p)
			return a
		}},
	}
	for _, tt := range tests {
		for _, k := range []string{"a", "e", "m", "d", "Q"} {
			t.Run(tt.name+"/"+k, func(t *testing.T) {
				app := tt.app()
				app.board.cursor = rowActiveMilestone

				press(app, k)

				if app.form != nil || app.board.confirmText != "" {
					t.Errorf("form = %v, confirm = %q, want the key ignored", app.form, app.board.confirmText)
				}
			})
		}
	}
}

// Filing a slice under a milestone names the option, written back as the type
// the Milestone column was read as — a select, or Notion's own status type
// where the column was converted in the UI.
func TestCreateSliceNamesTheMilestoneOption(t *testing.T) {
	tests := []struct {
		name         string
		propertyType string
		want         notion.PropertyValue
	}{
		{"select", notion.TypeSelect, notion.NewSelect("M3: Mutations")},
		{"status", notion.TypeStatus, notion.NewStatus("M3: Mutations")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeNotion{}
			m := domain.Milestone{
				ID: "M3: Mutations", Name: "M3: Mutations", SelectType: tt.propertyType,
			}

			runMsg(t, createSlice(client, "sl-ds", m, "New slice", "", ""))

			if len(client.created) != 1 {
				t.Fatalf("created %d pages, want 1", len(client.created))
			}
			if got := client.created[0].properties[notion.PropMilestone]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("milestone = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestAppAddWritesTheMilestoneOption drives the whole key: a is pressed on a
// milestone and the slice that falls out names it the way the project's slices
// do.
func TestAppAddWritesTheMilestoneOption(t *testing.T) {
	client := &fakeNotion{}
	app := newWriteApp(client)
	app.board.cursor = rowActiveMilestone

	feed(t, app, press(app, "a"))
	fillForm(t, app, "New slice", "The brief.", "")

	if len(client.created) != 1 {
		t.Fatalf("created %d pages, want the slice written", len(client.created))
	}
	want := notion.NewSelect("M2: Board")
	if got := client.created[0].properties[notion.PropMilestone]; !reflect.DeepEqual(got, want) {
		t.Errorf("milestone = %+v, want %+v", got, want)
	}
	if app.board.confirmText != `Added "New slice".` {
		t.Errorf("confirm = %q, want the created confirmation", app.board.confirmText)
	}
}
