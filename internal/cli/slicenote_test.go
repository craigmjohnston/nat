package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// The slices notePlan seeds beside followUpsPlan's own, which is In progress
// as "Render the board" under M1: two slices sharing a name under M1 and M2,
// a Done one, and one under no milestone.
const (
	drawM1ID  = "3b738308f654815fa843dce9c020e001"
	drawM2ID  = "3b738308f654815fa843dce9c020e002"
	shippedID = "3b738308f654815fa843dce9c020e003"
	looseID   = "3b738308f654815fa843dce9c020e004"
)

func newNotePlan(t *testing.T, breakIt func(*sql.DB)) *followUpsPlan {
	t.Helper()
	return newFollowUpsPlan(t, notion.SliceInProgress, false, func(db *sql.DB) {
		for _, q := range []string{
			`INSERT INTO milestones (name, position) VALUES ('M2', 1)`,
			`INSERT INTO slices (id, title, status, milestone, position, body) VALUES
			 ('` + drawM1ID + `', 'Draw it', 'Todo', 'M1', 1, 'Draw the board.'),
			 ('` + drawM2ID + `', 'Draw it', 'Todo', 'M2', 0, 'Draw the menu.'),
			 ('` + shippedID + `', 'Ship it', 'Done', 'M2', 1, 'Shipped.')`,
			`INSERT INTO slices (id, title, status, position, body) VALUES ('` + looseID + `', 'Loose end', 'Todo', 2, 'Tidy.')`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("seed %q: %v", q, err)
			}
		}
		if breakIt != nil {
			breakIt(db)
		}
	})
}

// bodyOf is a slice's body as the plan holds it, its stamps read as
// "<stamp>" (see [stampless]).
func (fp *followUpsPlan) bodyOf(t *testing.T, id string) string {
	t.Helper()
	body, err := fp.local(t).Body(context.Background(), id)
	if err != nil {
		t.Fatalf("read the body: %v", err)
	}
	return stampless(body)
}

// A note from the agent's own slice, on another named by name and milestone,
// ends that slice's brief with nat's provenance — the slice and milestone by
// name, no ID — and reads back as a note event from slice-show.
func TestSliceNoteByNameFromASlice(t *testing.T) {
	fp := newNotePlan(t, nil)
	if err := fp.run("slice-note", " draw IT ", "--milestone", "m2", "--from", sliceID,
		"--note", "The menu moved to the toolbar."); err != nil {
		t.Fatalf("slice-note: %v", err)
	}
	want := "Draw the menu.\n\n### Note\n\nAt <stamp>\n\nFrom \"Render the board\" (M1)\n\nThe menu moved to the toolbar."
	if got := fp.bodyOf(t, drawM2ID); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := fp.bodyOf(t, drawM1ID); got != "Draw the board." {
		t.Errorf("the other Draw it = %q, want it untouched", got)
	}
	if fp.nudges != 1 {
		t.Errorf("nudges = %d, want one", fp.nudges)
	}
	if want := "# Draw it\n\nNote filed at the end of its brief: From \"Render the board\" (M1).\n"; fp.out.String() != want {
		t.Errorf("output = %q, want %q", fp.out.String(), want)
	}

	if err := fp.run("slice-show", drawM2ID, "--json"); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var shown sliceShowJSON
	if err := json.Unmarshal(fp.out.Bytes(), &shown); err != nil {
		t.Fatalf("decode slice-show: %v", err)
	}
	// The note names the slice it came from by name and milestone, and when it
	// was left, which is a time of the wall clock's own here.
	want2 := []taskEventJSON{{
		Kind: "note", Note: "The menu moved to the toolbar.", By: `"Render the board" (M1)`,
		FromSlice: &noteSourceJSON{Name: "Render the board", Milestone: "M1"}, At: "<stamp>",
	}}
	for i := range shown.Events {
		shown.Events[i].At = stampless(shown.Events[i].At)
	}
	if !reflect.DeepEqual(shown.Events, want2) {
		t.Errorf("events = %+v, want %+v", shown.Events, want2)
	}
}

// With no --from the note is from the person at the keyboard; a slice may be
// named by ID, the note piped in, and the agent's own slice noted too.
func TestSliceNoteFromThePersonByID(t *testing.T) {
	fp := newNotePlan(t, nil)
	fp.env.In = strings.NewReader("  Mind the cache.\n")
	if err := fp.run("slice-note", sliceID, "--note", "-"); err != nil {
		t.Fatalf("slice-note: %v", err)
	}
	if got, want := fp.bodyOf(t, sliceID), "Do the thing.\n\n### Note\n\nAt <stamp>\n\nFrom Craig Johnston\n\nMind the cache."; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// A --from slice under no milestone is named without one.
func TestSliceNoteFromASliceWithNoMilestone(t *testing.T) {
	fp := newNotePlan(t, nil)
	if err := fp.run("slice-note", "Loose end", "--from", "Loose end", "--note", "Self."); err != nil {
		t.Fatalf("slice-note: %v", err)
	}
	if got := fp.bodyOf(t, looseID); !strings.HasSuffix(got, "### Note\n\nAt <stamp>\n\nFrom \"Loose end\"\n\nSelf.") {
		t.Errorf("body = %q, want the provenance without a milestone", got)
	}
}

func TestSliceNoteRefusals(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty note", []string{"Ship it", "--note", "  "}, "no note given"},
		{"no note", []string{"Ship it"}, "no note given"},
		{"done", []string{"Ship it", "--note", "x"}, `"Ship it" is Done: a finished slice is a record of what happened`},
		{"unknown", []string{"Paint it", "--note", "x"}, `no slice in the project is named "Paint it"`},
		{"unknown under milestone", []string{"Ship it", "--milestone", "M1", "--note", "x"}, `no slice named "Ship it" is filed under "M1"`},
		{"ambiguous", []string{"Draw it", "--note", "x"},
			`2 slices are named "Draw it": "Draw it" (M2), "Draw it" (M1) — pass --milestone to say which`},
		{"milestone with an ID", []string{drawM1ID, "--milestone", "M1", "--note", "x"}, "--milestone narrows a slice named by name"},
		{"unknown from", []string{drawM1ID, "--from", "Paint it", "--note", "x"}, `--from: no slice in the project is named "Paint it"`},
		{"unreadable from", []string{drawM1ID, "--from", "3b738308f654815fa843dce9c020efff", "--note", "x"}, "--from: load the slice"},
		{"stdin with nothing to read", []string{drawM1ID, "--note", "-"}, "--note - was given but there is nothing to read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newNotePlan(t, nil)
			fp.env.In = nil
			err := fp.run(append([]string{"slice-note"}, tt.args...)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if got := fp.bodyOf(t, drawM1ID); got != "Draw the board." {
				t.Errorf("body = %q, want nothing written", got)
			}
			if fp.nudges != 0 {
				t.Errorf("nudges = %d, want none", fp.nudges)
			}
		})
	}
}

func TestSliceNoteMisuse(t *testing.T) {
	fp := newNotePlan(t, nil)
	if err := fp.run("slice-note", "--note", "x"); err == nil || !strings.Contains(err.Error(), "want exactly one slice") {
		t.Errorf("no slice: err = %v", err)
	}
	if err := fp.run("slice-note", drawM1ID, "--nope"); err == nil {
		t.Error("bad flag: want an error")
	}
	if err := Run(context.Background(), []string{"slice-note", drawM1ID, "--note", "x"}, fp.env); err == nil {
		t.Error("no project: want an error")
	}

	fp.env.In = failingReader{}
	if err := fp.run("slice-note", drawM1ID, "--note", "-"); err == nil {
		t.Error("an unreadable stdin: want an error")
	}
}

// A note with no --from needs somebody to be from: a project in a workspace
// whose config names nobody has nobody to name.
func TestSliceNoteWithNoAssigneeNeedsFrom(t *testing.T) {
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
	}}
	env, _ := testEnv(testConfig(t), api)
	err := Run(context.Background(), []string{"slice-note", testSliceID, "--note", "x", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no assignee in the config to say the note is from") {
		t.Errorf("err = %v", err)
	}
	if len(api.appends) != 0 {
		t.Errorf("appends = %d, want nothing written", len(api.appends))
	}
}

// A workspace that cannot be read stops the command before anything is.
func TestSliceNoteOverAnUnreadableWorkspace(t *testing.T) {
	api := &fakeAPI{dataSourceErr: errors.New("boom")}
	env, _ := testEnv(testClaimConfig(t), api)
	if err := Run(context.Background(), []string{"slice-note", testSliceID, "--note", "x", "--project", "project-1"}, env); err == nil {
		t.Error("an unreadable workspace: want an error")
	}
}

// A plan that cannot answer its shape, its slices, a body, or the write each
// stops the command with nothing filed.
func TestSliceNoteReadAndWriteFailures(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		breakIt string
		want    string
	}{
		{"shape", drawM1ID, `ALTER TABLE milestones RENAME TO gone`, ""},
		{"plan", "Ship it", `ALTER TABLE slice_deps RENAME TO gone`, "read the plan to find"},
		{"body", drawM1ID, `ALTER TABLE slices DROP COLUMN body`, `"Draw it" has no readable brief`},
		{"write", drawM1ID, `CREATE TRIGGER no_writes BEFORE UPDATE ON slices BEGIN SELECT RAISE(FAIL, 'boom'); END`, "file the note"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newNotePlan(t, func(db *sql.DB) {
				if _, err := db.Exec(tt.breakIt); err != nil {
					t.Fatalf("break the plan: %v", err)
				}
			})
			err := fp.run("slice-note", tt.ref, "--note", "x")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if fp.nudges != 0 {
				t.Errorf("nudges = %d, want none", fp.nudges)
			}
		})
	}
}

func TestSliceNoteReportsAFailedOutput(t *testing.T) {
	fp := newNotePlan(t, nil)
	fp.env.Out = failingWriter{}
	if err := Run(context.Background(), []string{"slice-note", drawM1ID, "--note", "x", "--project", "project-1"}, fp.env); err == nil {
		t.Error("an unwritable output: want an error")
	}
}

// bodyAtSendRunner reads the target's body the moment a prompt is pasted, so a
// test can say the note was on the page before the agent was told.
type bodyAtSendRunner struct {
	*agentTestRunner
	read func() string
	seen []string
}

func (r *bodyAtSendRunner) Run(name string, args ...string) (string, error) {
	if len(args) > 1 && args[1] == "paste-buffer" {
		r.seen = append(r.seen, r.read())
	}
	return r.agentTestRunner.Run(name, args...)
}

// A note on a slice whose agent is live is filed, then sent to that agent as
// one turn, and the output says so.
func TestSliceNoteTellsALiveAgent(t *testing.T) {
	fp := newNotePlan(t, nil)
	fp.runner.liveSessions[drawM2ID] = "nat-draw"
	r := &bodyAtSendRunner{agentTestRunner: fp.runner, read: func() string { return fp.bodyOf(t, drawM2ID) }}
	fp.env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(r) }
	if err := fp.run("slice-note", "Draw it", "--milestone", "M2", "--from", sliceID,
		"--note", "The menu moved to the toolbar."); err != nil {
		t.Fatalf("slice-note: %v", err)
	}
	want := agent.NoteArrivedPrompt(`From "Render the board" (M1)`, "The menu moved to the toolbar.")
	if len(fp.runner.sends) != 1 || fp.runner.sends[0].session != "nat-draw" || fp.runner.sends[0].prompt != want {
		t.Errorf("sends = %+v, want one to nat-draw of\n%s", fp.runner.sends, want)
	}
	if len(r.seen) != 1 || !strings.HasSuffix(r.seen[0], "The menu moved to the toolbar.") {
		t.Errorf("body at send = %q, want the note already on it", r.seen)
	}
	if want := "# Draw it\n\nNote filed at the end of its brief: From \"Render the board\" (M1). Its live agent was told.\n"; fp.out.String() != want {
		t.Errorf("output = %q, want %q", fp.out.String(), want)
	}
}

// No live session, and an agent noting its own slice, each send nothing and
// print the plain line.
func TestSliceNoteTellsNobody(t *testing.T) {
	for name, args := range map[string][]string{
		"no live agent": {"slice-note", drawM1ID, "--note", "x"},
		"self note":     {"slice-note", sliceID, "--from", sliceID, "--note", "x"},
	} {
		t.Run(name, func(t *testing.T) {
			fp := newNotePlan(t, nil)
			fp.runner.liveSessions[sliceID] = "nat-render"
			if err := fp.run(args...); err != nil {
				t.Fatalf("slice-note: %v", err)
			}
			if len(fp.runner.sends) != 0 {
				t.Errorf("sends = %+v, want none", fp.runner.sends)
			}
			if strings.Contains(fp.out.String(), "told") {
				t.Errorf("output = %q, want the plain line", fp.out.String())
			}
		})
	}
}

// A tmux that cannot be listed concludes nothing: the note is filed and nobody
// told.
func TestSliceNoteOverAnUnreadableTmux(t *testing.T) {
	fp := newNotePlan(t, nil)
	fp.runner.liveFatalErr = "tmux broke"
	if err := fp.run("slice-note", drawM1ID, "--note", "x"); err != nil {
		t.Fatalf("slice-note: %v, want it to go ahead", err)
	}
	if got := fp.bodyOf(t, drawM1ID); !strings.HasSuffix(got, "\n\nx") {
		t.Errorf("body = %q, want the note filed", got)
	}
	if want := "# Draw it\n\nNote filed at the end of its brief: From Craig Johnston.\n"; fp.out.String() != want {
		t.Errorf("output = %q, want %q", fp.out.String(), want)
	}
}

// A send that fails leaves the note on the page and says the agent was not
// told.
func TestSliceNoteFailedSendLeavesTheNote(t *testing.T) {
	fp := newNotePlan(t, nil)
	fp.runner.liveSessions[drawM1ID] = "nat-draw"
	fp.runner.sendErr = "pane gone"
	err := fp.run("slice-note", drawM1ID, "--note", "x")
	if err == nil || !strings.Contains(err.Error(), "the note is filed, but telling the agent failed") {
		t.Fatalf("err = %v, want the failed send named", err)
	}
	if got := fp.bodyOf(t, drawM1ID); !strings.HasSuffix(got, "\n\nx") {
		t.Errorf("body = %q, want the note to stand", got)
	}
}

// The provenance names the slice by name, with its milestone where it has one,
// and never an ID or URL.
func TestFromSlice(t *testing.T) {
	fp := newNotePlan(t, nil)
	l := fp.local(t)
	sh, err := l.Shape(context.Background(), store.Project{ID: "project-1"})
	if err != nil {
		t.Fatalf("read the shape: %v", err)
	}
	s, _, _ := l.Slice(context.Background(), drawM2ID)
	if got := fromSlice(s, sh.Milestones); got != `From "Draw it" (M2)` {
		t.Errorf("fromSlice = %q", got)
	}
}
