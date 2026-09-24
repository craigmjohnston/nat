package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/store"
)

// makePlanFolder lays down a local plan in a fresh folder the way
// project-create --local does, and returns the folder and the plan's ID.
func makePlanFolder(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "plans")
	id := store.NewProjectID()
	entry := config.ProjectConfig{Name: name, Backend: config.BackendLocal, PlanDir: dir}
	if err := store.CreateLocalProject(context.Background(), store.ProjectOf(id, entry), "Be small."); err != nil {
		t.Fatalf("lay down the plan: %v", err)
	}
	return dir, id
}

func TestProjectOpenFolderRecordsThePlan(t *testing.T) {
	env, out, saved := noNotionEnv(t, config.Config{}, false)
	dir, id := makePlanFolder(t, "Found")

	if err := Run(context.Background(), []string{"project-open-folder", dir, "--json"}, env); err != nil {
		t.Fatalf("project-open-folder: %v", err)
	}
	var got projectCreatedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if got.Project.ID != id || got.Project.Name != "Found" || got.Project.Backend != "local" || got.Project.PlanDir != dir {
		t.Errorf("reported %+v", got.Project)
	}
	want := config.ProjectConfig{Name: "Found", Backend: "local", PlanDir: dir}
	if saved.Projects[id] != want {
		t.Errorf("config entry = %+v, want %+v", saved.Projects[id], want)
	}

	// Opened again, the entry that is there answers, and config is not rewritten.
	saved.Projects[id] = config.ProjectConfig{Name: "Renamed", WorkingDir: "/src", Backend: "local", PlanDir: dir}
	out.Reset()
	if err := Run(context.Background(), []string{"project-open-folder", dir}, env); err != nil {
		t.Fatalf("second open: %v", err)
	}
	if saved.Projects[id].Name != "Renamed" {
		t.Errorf("a known project's entry was rewritten: %+v", saved.Projects[id])
	}
	if s := out.String(); !strings.Contains(s, "# Renamed") || !strings.Contains(s, "Working directory: /src") {
		t.Errorf("markdown = %q", s)
	}
}

func TestProjectOpenFolderRefusals(t *testing.T) {
	ctx := context.Background()
	empty := t.TempDir()

	// A stray SQLite file that is not a plan, and must be left untouched.
	other := t.TempDir()
	otherPath := filepath.Join(other, "notes.db")
	db, err := sql.Open("sqlite3", "file:"+otherPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE notes (body TEXT)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	before, _ := os.ReadFile(otherPath)

	// A plan file under a name that is not its project's, and a text file
	// that is no database at all.
	misnamed, _ := makePlanFolder(t, "Misnamed")
	entries, _ := os.ReadDir(misnamed)
	if err := os.Rename(filepath.Join(misnamed, entries[0].Name()), filepath.Join(misnamed, "other.db")); err != nil {
		t.Fatal(err)
	}
	garbage := t.TempDir()
	if err := os.WriteFile(filepath.Join(garbage, "x.db"), []byte("not sqlite at all, just text"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	newer := t.TempDir()
	db, err = sql.Open("sqlite3", "file:"+filepath.Join(newer, "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99;`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	two, _ := makePlanFolder(t, "One")
	second := store.NewProjectID()
	if err := store.CreateLocalProject(ctx, store.ProjectOf(second, config.ProjectConfig{Name: "Two", Backend: "local", PlanDir: two}), ""); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"an empty folder", []string{empty}, "looked for a nat plan file"},
		{"a foreign database", []string{other}, "looked for a nat plan file"},
		{"a misnamed plan", []string{misnamed}, "looked for a nat plan file"},
		{"not a database", []string{garbage}, "looked for a nat plan file"},
		{"no such folder", []string{filepath.Join(empty, "nope")}, "is not a folder"},
		{"a file", []string{file}, "is not a folder"},
		{"a newer plan", []string{newer}, "newer nat"},
		{"two plans", []string{two}, "2 plans found"},
		{"no folder", nil, "want exactly one folder"},
		{"a blank folder", []string{"  "}, "the folder is empty"},
		{"a bad flag", []string{"--nope", empty}, "project-open-folder"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, _, saved := noNotionEnv(t, config.Config{}, false)
			err := Run(ctx, append([]string{"project-open-folder"}, c.args...), env)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to say %q", err, c.want)
			}
			if len(saved.Projects) != 0 {
				t.Errorf("a refusal wrote config: %+v", saved.Projects)
			}
		})
	}
	if after, _ := os.ReadFile(otherPath); string(after) != string(before) {
		t.Error("the foreign database was modified")
	}
}

func TestProjectOpenFolderNeedsAWorkingDirectoryForARelativePath(t *testing.T) {
	env, _, _ := noNotionEnv(t, config.Config{}, false)
	stubGetwd(t, "", os.ErrNotExist)
	if err := Run(context.Background(), []string{"project-open-folder", "rel"}, env); err == nil {
		t.Error("a relative folder with no working directory was accepted")
	}
}

func TestProjectOpenFolderConfigFailures(t *testing.T) {
	dir, _ := makePlanFolder(t, "P")
	env, _, _ := noNotionEnv(t, config.Config{}, false)
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, os.ErrPermission }
	if err := Run(context.Background(), []string{"project-open-folder", dir}, env); err == nil {
		t.Error("an unreadable config was not reported")
	}

	env, _, _ = noNotionEnv(t, config.Config{}, false)
	env.Save = func(config.Config) error { return os.ErrPermission }
	err := Run(context.Background(), []string{"project-open-folder", dir}, env)
	if err == nil || !strings.Contains(err.Error(), "save config") {
		t.Errorf("err = %v", err)
	}
}
