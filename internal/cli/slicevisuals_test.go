package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

// shot lays a file down under dir for a hand-in to name, and is its path.
func shot(t *testing.T, dir, name string) string {
	t.Helper()
	return shotOf(t, dir, name, "png")
}

// shotOf lays a file of the given bytes down under dir, and is its path.
func shotOf(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// sum is the sha256 a hand-in files for a file of these bytes.
func sum(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// The round an agent makes: images handed in, read back by slice-show in
// order, a relative path filed absolute with its hash and a URI as given with
// none; then a second hand-in that re-renders one at the same path, gives
// another a before and removes the third; then one that changes nothing; then
// one that removes the rest.
func TestSliceVisualsAreFiledAndReadBack(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	dir := t.TempDir()
	dark := shot(t, dir, "dark.png")
	light := shotOf(t, dir, "light.png", "light")
	stubGetwd(t, dir, nil)

	if err := fp.run("slice-visuals", sliceID,
		"--visual", "Settings pane, dark\n"+dark,
		"--visual", "Settings pane, light\nlight.png",
		"--visual", "The docs page\nhttps://example.com/docs.png"); err != nil {
		t.Fatalf("slice-visuals: %v", err)
	}
	wantFiled := `# Render the board

Visual changes filed on the slice page for the user to review.

Added: "Settings pane, dark", "Settings pane, light", "The docs page"

As it now stands, 3 visual changes:

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
		{Index: 1, Name: "Settings pane, dark", URI: dark, Hash: sum("png"), Changed: true},
		{Index: 2, Name: "Settings pane, light", URI: light, Hash: sum("light"), Changed: true},
		{Index: 3, Name: "The docs page", URI: "https://example.com/docs.png", Changed: true},
	}
	if got := fp.shown(t).Visuals; !slices.Equal(got, want) {
		t.Errorf("slice-show visuals = %+v, want %+v", got, want)
	}

	shotOf(t, dir, "dark.png", "re-rendered")
	before := shotOf(t, dir, "light-before.png", "old light")
	if err := fp.run("slice-visuals", sliceID, "--visual", "Settings pane, dark\nfile://"+dark,
		"--before", "Settings pane, light\n"+before, "--remove", "The docs page"); err != nil {
		t.Fatalf("slice-visuals again: %v", err)
	}
	wantFiled = `# Render the board

Visual changes filed on the slice page for the user to review.

Updated: "Settings pane, dark", "Settings pane, light"
Removed: "The docs page"

As it now stands, 2 visual changes:

1. Settings pane, dark
2. Settings pane, light, with a before

Carry on — the user's comments on them, if any, arrive as a message.
`
	if fp.out.String() != wantFiled {
		t.Errorf("slice-visuals printed\n%s\nwant\n%s", fp.out, wantFiled)
	}
	want = []visualJSON{
		{Index: 1, Name: "Settings pane, dark", URI: dark, Hash: sum("re-rendered"), Changed: true},
		{Index: 2, Name: "Settings pane, light", URI: light, Hash: sum("light"),
			Before: visualBeforeJSON{URI: before, Hash: sum("old light")}, Changed: true},
	}
	if got := fp.shown(t).Visuals; !slices.Equal(got, want) {
		t.Errorf("slice-show after a second hand-in = %+v, want %+v", got, want)
	}

	// A re-render with the same bytes keeps the before it has, and leaves both
	// unchanged since the last hand-in.
	if err := fp.run("slice-visuals", sliceID, "--visual", "Settings pane, light\n"+light); err != nil {
		t.Fatalf("slice-visuals a third time: %v", err)
	}
	want[0].Changed, want[1].Changed = false, false
	if got := fp.shown(t).Visuals; !slices.Equal(got, want) {
		t.Errorf("slice-show after an unchanged hand-in = %+v, want %+v", got, want)
	}

	if err := fp.run("slice-visuals", sliceID, "--remove", "Settings pane, dark", "--remove", "Settings pane, light"); err != nil {
		t.Fatalf("slice-visuals removing the rest: %v", err)
	}
	if !strings.Contains(fp.out.String(), "No visual changes are filed now.") {
		t.Errorf("slice-visuals removing the rest printed\n%s", fp.out)
	}
	if got := fp.shown(t).Visuals; len(got) != 0 {
		t.Errorf("slice-show after removing every one = %+v, want none", got)
	}
}

// slice-show names its fields as the app decodes them, leaves the list out
// where nothing was handed in, and leaves an item's hash and before out where
// it has none.
func TestSliceShowVisualsJSONShape(t *testing.T) {
	fp := newFollowUpsPlan(t, notion.SliceInProgress, true, nil)
	if err := fp.run("slice-show", sliceID, "--json"); err != nil {
		t.Fatalf("slice-show: %v", err)
	}
	if strings.Contains(fp.out.String(), `"visuals"`) {
		t.Errorf("slice-show with nothing handed in carries visuals:\n%s", fp.out)
	}
	dir := t.TempDir()
	path, before := shot(t, dir, "a.png"), shotOf(t, dir, "b.png", "before")
	if err := fp.run("slice-visuals", sliceID, "--visual", "A\n"+path, "--before", "A\n"+before,
		"--visual", "B\nhttps://example.com/b.png"); err != nil {
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
	want := []map[string]any{
		{"index": float64(1), "name": "A", "uri": path, "hash": sum("png"),
			"before": map[string]any{"uri": before, "hash": sum("before")}, "changed": true},
		{"index": float64(2), "name": "B", "uri": "https://example.com/b.png", "changed": true},
	}
	if !reflect.DeepEqual(raw.Visuals, want) {
		t.Errorf("visuals = %v, want %v", raw.Visuals, want)
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
		{"no name", notion.SliceInProgress, []string{"--visual", "  \n "}, "a --visual has no name"},
		{"a before with no name", notion.SliceInProgress, []string{"--before", "  \n "}, "a --before has no name"},
		{"no image", notion.SliceInProgress, []string{"--visual", "Just a name"}, `"Just a name" has no image`},
		{"two images", notion.SliceInProgress, []string{"--visual", "A\n" + path + "\n\n" + path}, `"A" names more than one image`},
		{"twice", notion.SliceInProgress, []string{"--visual", "A\n" + path, "--visual", "A\n" + path}, `two visuals are named "A"`},
		{"a before twice", notion.SliceInProgress, []string{"--visual", "A\n" + path, "--before", "A\n" + path, "--before", "A\n" + path},
			`two befores are named "A"`},
		{"a remove with no name", notion.SliceInProgress, []string{"--remove", " "}, "a --remove names nothing"},
		{"a remove twice", notion.SliceInProgress, []string{"--remove", "A", "--remove", " A "}, `"A" is removed twice`},
		{"removed and given", notion.SliceInProgress, []string{"--visual", "A\n" + path, "--remove", "A"}, `"A" is both removed and given`},
		{"removed and given a before", notion.SliceInProgress, []string{"--before", "A\n" + path, "--remove", "A"}, `"A" is both removed and given`},
		{"a remove naming nothing", notion.SliceInProgress, []string{"--remove", "A"}, `--remove "A" names no visual filed: none is filed`},
		{"a before naming nothing", notion.SliceInProgress, []string{"--visual", "B\n" + path, "--before", "A\n" + path},
			`--before "A" names no visual: filed are "B", or give it with --visual in the same command`},
		{"a missing before", notion.SliceInProgress, []string{"--visual", "A\n" + path, "--before", "A\n" + filepath.Join(dir, "gone.png")},
			"no image at " + filepath.Join(dir, "gone.png")},
		{"missing file", notion.SliceInProgress, []string{"--visual", "A\n" + filepath.Join(dir, "gone.png")}, "no image at " + filepath.Join(dir, "gone.png")},
		{"not held", notion.SliceTodo, []string{"--visual", "A\n" + path},
			`"Render the board" is Todo: visual changes can be given only to a slice you hold`},
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

// A Done slice is never held — its work is merged and no session is launched
// on it — so it takes no visual changes, a pull request recorded or not,
// whoever it was assigned to.
func TestSliceVisualsRefusesADoneSlice(t *testing.T) {
	path := shot(t, t.TempDir(), "a.png")
	const want = ": visual changes can be given only to a slice you hold"
	tests := []struct {
		name   string
		update string
		refuse string
	}{
		{"mine with a PR", `UPDATE slices SET pr = 'https://github.com/o/r/pull/9'`, `"Render the board" is Done` + want},
		{"someone else's", `UPDATE slices SET pr = 'https://github.com/o/r/pull/9', assignee = 'Someone Else',
			assignee_name = 'Someone Else'`, `"Render the board" is Done, held by Someone Else` + want},
		{"nobody's", `UPDATE slices SET pr = 'https://github.com/o/r/pull/9', assignee = '', assignee_name = ''`,
			`"Render the board" is Done, held by nobody` + want},
		{"no PR", `UPDATE slices SET pr = ''`, `"Render the board" is Done` + want},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newFollowUpsPlan(t, notion.SliceDone, false, func(db *sql.DB) {
				if _, err := db.Exec(tt.update); err != nil {
					t.Fatalf("set the slice up: %v", err)
				}
			})
			err := fp.run("slice-visuals", sliceID, "--visual", "A\n"+path)
			if err == nil || err.Error() != tt.refuse {
				t.Errorf("err = %v, want %q", err, tt.refuse)
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
		"body":  func(db *sql.DB) error { _, err := db.Exec(`ALTER TABLE slices DROP COLUMN body`); return err },
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
