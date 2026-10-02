package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

// shot lays an empty file down under dir for a hand-in to name, and is its
// path.
func shot(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// The round an agent makes: images handed in, read back by slice-show in
// order, a relative path filed absolute and a URI as given; then a second
// hand-in that replaces the first.
func TestSliceVisualsAreFiledAndReadBack(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	dir := t.TempDir()
	dark := shot(t, dir, "dark.png")
	shot(t, dir, "light.png")
	stubGetwd(t, dir, nil)

	if err := fp.run("slice-visuals", sliceID,
		"--visual", "Settings pane, dark\n"+dark,
		"--visual", "Settings pane, light\nlight.png",
		"--visual", "The docs page\nhttps://example.com/docs.png"); err != nil {
		t.Fatalf("slice-visuals: %v", err)
	}
	wantFiled := `# Render the board

3 visual changes filed on the slice page for the user to review:

1. Settings pane, dark
2. Settings pane, light
3. The docs page

Carry on — the user's comments on them, if any, arrive as a message.
`
	if fp.out.String() != wantFiled {
		t.Errorf("slice-visuals printed\n%s\nwant\n%s", fp.out, wantFiled)
	}
	if fp.nudges != 1 {
		t.Errorf("nudges = %d, want one for the filing", fp.nudges)
	}
	want := []visualJSON{
		{1, "Settings pane, dark", dark},
		{2, "Settings pane, light", filepath.Join(dir, "light.png")},
		{3, "The docs page", "https://example.com/docs.png"},
	}
	if got := fp.shown(t).Visuals; !equalVisuals(got, want) {
		t.Errorf("slice-show visuals = %+v, want %+v", got, want)
	}

	if err := fp.run("slice-visuals", sliceID, "--visual", "Only this\nfile://"+dark); err != nil {
		t.Fatalf("slice-visuals again: %v", err)
	}
	if got := fp.shown(t).Visuals; !equalVisuals(got, []visualJSON{{1, "Only this", dark}}) {
		t.Errorf("slice-show after a second hand-in = %+v, want only its one", got)
	}
}

func equalVisuals(got, want []visualJSON) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// slice-show names its fields as the app decodes them, and leaves the list out
// where nothing was handed in.
func TestSliceShowVisualsJSONShape(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	if err := fp.run("slice-show", sliceID, "--json"); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	if strings.Contains(fp.out.String(), `"visuals"`) {
		t.Errorf("slice-show with nothing handed in carries visuals:\n%s", fp.out)
	}
	path := shot(t, t.TempDir(), "a.png")
	if err := fp.run("slice-visuals", sliceID, "--visual", "A\n"+path); err != nil {
		t.Fatalf("slice-visuals: %v", err)
	}
	if err := fp.run("slice-show", sliceID, "--json"); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	var raw struct {
		Visuals []map[string]any `json:"visuals"`
	}
	if err := json.Unmarshal(fp.out.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]any{"index": float64(1), "name": "A", "uri": path}
	if len(raw.Visuals) != 1 || len(raw.Visuals[0]) != 3 ||
		raw.Visuals[0]["index"] != want["index"] || raw.Visuals[0]["name"] != want["name"] || raw.Visuals[0]["uri"] != want["uri"] {
		t.Errorf("visuals = %v, want [%v]", raw.Visuals, want)
	}
}

func TestSliceVisualsRefusals(t *testing.T) {
	dir := t.TempDir()
	path := shot(t, dir, "a.png")
	tests := []struct {
		name   string
		status string
		args   []string
		want   string
	}{
		{"none", notion.SliceInProgress, nil, "no visual given"},
		{"no name", notion.SliceInProgress, []string{"--visual", "  \n "}, "has no name"},
		{"no image", notion.SliceInProgress, []string{"--visual", "Just a name"}, `"Just a name" has no image`},
		{"two images", notion.SliceInProgress, []string{"--visual", "A\n" + path + "\n\n" + path}, `"A" names more than one image`},
		{"twice", notion.SliceInProgress, []string{"--visual", "A\n" + path, "--visual", "A\n" + path}, `two visuals are named "A"`},
		{"missing file", notion.SliceInProgress, []string{"--visual", "A\n" + filepath.Join(dir, "gone.png")}, "no image at " + filepath.Join(dir, "gone.png")},
		{"not held", notion.SliceTodo, []string{"--visual", "A\n" + path}, "only a slice you claimed can be given visual changes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, tt.status, true, nil)
			err := fp.run(append([]string{"slice-visuals", sliceID}, tt.args...)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if body := fp.body(t); body != "Do the thing." {
				t.Errorf("body = %q, want nothing written", body)
			}
		})
	}
}

func TestSliceVisualsMisuseAndFailures(t *testing.T) {
	path := shot(t, t.TempDir(), "a.png")
	visual := "A\n" + path
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	for name, args := range map[string][]string{
		"no slice":    {"slice-visuals", "--visual", visual},
		"not a slice": {"slice-visuals", "the board", "--visual", visual},
		"bad flag":    {"slice-visuals", sliceID, "--nope"},
		"no project":  {"slice-visuals", sliceID, "--visual", visual},
	} {
		var err error
		if name == "no project" {
			err = Run(context.Background(), args, fp.env)
		} else {
			err = fp.run(args...)
		}
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	stubGetwd(t, "", errors.New("gone"))
	if err := fp.run("slice-visuals", sliceID, "--visual", "A\na.png"); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Errorf("an unreadable working directory: err = %v", err)
	}

	cfg := testConfig(t)
	env, _ := testEnv(cfg, &fakeAPI{})
	err := Run(context.Background(), []string{"slice-visuals", sliceID, "--visual", visual, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no assignee in the config") {
		t.Errorf("no assignee: err = %v", err)
	}

	broken := &fakeAPI{dataSourceErr: errors.New("boom")}
	env, _ = testEnv(testClaimConfig(t), broken)
	if err := Run(context.Background(), []string{"slice-visuals", sliceID, "--visual", visual, "--project", "project-1"}, env); err == nil {
		t.Error("an unreadable workspace: want an error")
	}
}

// A plan that cannot answer its shape, the slice, or the write each stops the
// command with nothing filed.
func TestSliceVisualsReadAndWriteFailures(t *testing.T) {
	path := shot(t, t.TempDir(), "a.png")
	for name, breakIt := range map[string]func(*sql.DB) error{
		"shape": func(db *sql.DB) error { _, err := db.Exec(`ALTER TABLE milestones RENAME TO gone`); return err },
		"slice": func(db *sql.DB) error { _, err := db.Exec(`DELETE FROM slices`); return err },
		"write": func(db *sql.DB) error {
			_, err := db.Exec(`CREATE TRIGGER no_writes BEFORE UPDATE ON slices BEGIN SELECT RAISE(FAIL, 'boom'); END`)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, notion.SliceInProgress, true, func(db *sql.DB) {
				if err := breakIt(db); err != nil {
					t.Fatalf("break the plan: %v", err)
				}
			})
			if err := fp.run("slice-visuals", sliceID, "--visual", "A\n"+path); err == nil {
				t.Error("want an error")
			}
			if fp.nudges != 0 {
				t.Errorf("nudges = %d, want none", fp.nudges)
			}
		})
	}
}

func TestSliceVisualsReportsAFailedWrite(t *testing.T) {
	path := shot(t, t.TempDir(), "a.png")
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	fp.env.Out = failingWriter{}
	if err := Run(context.Background(), []string{"slice-visuals", sliceID, "--visual", "A\n" + path,
		"--project", "project-1"}, fp.env); err == nil {
		t.Error("an unwritable output: want an error")
	}
}
