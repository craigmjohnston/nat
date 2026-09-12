package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

// ForProject is the one place the backend is chosen, so the whole of what it
// has to get right is which kind of store comes back — and that a Notion
// project is refused rather than handed a nil client to fail on later.
func TestForProjectChoosesTheBackend(t *testing.T) {
	dir := t.TempDir()

	notionStore, err := ForProject("p1", config.ProjectConfig{Name: "nat"}, &fakeAPI{})
	if err != nil {
		t.Fatalf("open the Notion project: %v", err)
	}
	if _, ok := notionStore.(*Notion); !ok {
		t.Errorf("store = %T, want a Notion one", notionStore)
	}
	// Closing one gives nothing back, since it holds nothing open.
	if err := notionStore.Close(); err != nil {
		t.Errorf("close the Notion store: %v", err)
	}

	localStore, err := ForProject("p2", config.ProjectConfig{
		Name: "local", Backend: config.BackendLocal, PlanDir: dir,
	}, nil)
	if err != nil {
		t.Fatalf("open the local project: %v", err)
	}
	l, ok := localStore.(*Local)
	if !ok {
		t.Fatalf("store = %T, want a local one", localStore)
	}
	if want := filepath.Join(dir, "p2.db"); l.Path() != want {
		t.Errorf("path = %q, want %q", l.Path(), want)
	}
	if err := localStore.Close(); err != nil {
		t.Errorf("close the local store: %v", err)
	}
}

// A machine with no Notion connection is refused by name rather than at the
// first request, so the refusal says which project it is about.
func TestForProjectRefusesANotionProjectWithNoClient(t *testing.T) {
	_, err := ForProject("p1", config.ProjectConfig{Name: "nat"}, nil)
	if err == nil || !strings.Contains(err.Error(), "p1") {
		t.Fatalf("err = %v, want a refusal naming the project", err)
	}
}

// A plan file that cannot be laid down is the open's own failure, handed
// straight back — the directory here is a file.
func TestForProjectReportsAPlanItCannotOpen(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	_, err := ForProject("p2", config.ProjectConfig{Backend: config.BackendLocal, PlanDir: file}, nil)
	if err == nil {
		t.Fatal("want the open to fail")
	}
}

// With no directory named, a local plan goes to nat's own data directory —
// which is [LocalPath]'s answer and nothing this function invents.
func TestPlanPathFallsBackToNatsOwnDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	got, err := PlanPath("", "p1")
	if err != nil {
		t.Fatalf("PlanPath(): %v", err)
	}
	want, err := LocalPath("p1")
	if err != nil {
		t.Fatalf("LocalPath(): %v", err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

// A home directory that cannot be resolved is the only way the fallback fails,
// and it is handed back rather than guessed past.
func TestPlanPathReportsAnUnresolvableHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	if _, err := PlanPath("", "p1"); err == nil {
		t.Fatal("want the home resolution to fail")
	}
}

// Creating a local project is the whole of what stands where Notion's page
// create stands: a file with the project's own row in it, which every later
// read answers about.
func TestCreateLocalProjectWritesThePlanAndItsProject(t *testing.T) {
	dir := t.TempDir()
	id := NewProjectID()

	path, err := CreateLocalProject(dir, id, "nat", "Branch per slice.")
	if err != nil {
		t.Fatalf("create the project: %v", err)
	}
	if want := filepath.Join(dir, localSlug(id)+".db"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("reopen the plan: %v", err)
	}
	defer func() { _ = l.Close() }()

	ctx := context.Background()
	body, err := l.Body(ctx, id)
	if err != nil {
		t.Fatalf("read the conventions: %v", err)
	}
	if body != "Branch per slice." {
		t.Errorf("conventions = %q, want the ones written", body)
	}
	plan, err := l.Plan(ctx, Project{ID: id})
	if err != nil {
		t.Fatalf("read the plan: %v", err)
	}
	if plan.Project.Name != "nat" {
		t.Errorf("name = %q, want nat", plan.Project.Name)
	}
	if len(plan.Project.Slices) != 0 || len(plan.Project.Milestones) != 0 {
		t.Errorf("plan = %+v, want an empty one", plan.Project)
	}
}

// An ID nat makes for itself is shaped like a page ID, so that everything
// carrying a project ID around cannot tell the two apart.
func TestNewProjectIDLooksLikeAPageID(t *testing.T) {
	id := NewProjectID()
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Errorf("id = %q, want a dashed 32-hex-character ID", id)
	}
	if NewProjectID() == id {
		t.Error("two projects should not share an ID")
	}
}

// Recording the project twice records it once: the row is replaced, so a
// re-created project has its name and conventions rewritten and everything
// filed under it left alone.
func TestSetProjectReplacesTheProjectRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() { _ = l.Close() }()
	ctx := context.Background()

	if err := l.SetProject(ctx, "p1", "first", "one"); err != nil {
		t.Fatalf("record the project: %v", err)
	}
	if err := l.SetProject(ctx, "p1", "second", "two"); err != nil {
		t.Fatalf("record it again: %v", err)
	}

	plan, err := l.Plan(ctx, Project{ID: "p1"})
	if err != nil {
		t.Fatalf("read the plan: %v", err)
	}
	if plan.Project.Name != "second" {
		t.Errorf("name = %q, want the second one", plan.Project.Name)
	}
	body, err := l.Body(ctx, "p1")
	if err != nil {
		t.Fatalf("read the conventions: %v", err)
	}
	if body != "two" {
		t.Errorf("conventions = %q, want the second ones", body)
	}
}

// A plan is one project's. Recording another drops the one that was there,
// rather than leaving a file that answers about two.
func TestSetProjectLeavesOneProjectInThePlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() { _ = l.Close() }()
	ctx := context.Background()

	if err := l.SetProject(ctx, "p1", "first", "one"); err != nil {
		t.Fatalf("record the project: %v", err)
	}
	if err := l.SetProject(ctx, "p2", "second", "two"); err != nil {
		t.Fatalf("record another: %v", err)
	}

	if body, err := l.Body(ctx, "p1"); err != nil || body != "" {
		t.Errorf("p1 body = %q, %v, want it gone", body, err)
	}
	plan, err := l.Plan(ctx, Project{ID: "p2"})
	if err != nil {
		t.Fatalf("read the plan: %v", err)
	}
	if plan.Project.Name != "second" {
		t.Errorf("name = %q, want the project now recorded", plan.Project.Name)
	}
}

// A plan that has been closed refuses the write rather than panicking, which
// is the one failure SetProject has of its own.
func TestSetProjectReportsAPlanThatIsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close the plan: %v", err)
	}
	if err := l.SetProject(context.Background(), "p1", "nat", ""); err == nil {
		t.Fatal("want the write to fail")
	}
}

// The two remaining ways creating a local project fails: no directory to put
// the file in, and a path that is not a plan at all.
func TestCreateLocalProjectReportsWhatStoppedIt(t *testing.T) {
	t.Run("no directory to fall back to", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_DATA_HOME", "")
		if _, err := CreateLocalProject("", "p1", "nat", ""); err == nil {
			t.Fatal("want the home resolution to fail")
		}
		// The same failure stops ForProject before it opens anything.
		if _, err := ForProject("p1", config.ProjectConfig{Backend: config.BackendLocal}, nil); err == nil {
			t.Fatal("want the home resolution to fail there too")
		}
	})
	t.Run("a file that is not a plan", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "p1.db"), []byte("not a database"), 0o644); err != nil {
			t.Fatalf("write the file: %v", err)
		}
		if _, err := CreateLocalProject(dir, "p1", "nat", ""); err == nil {
			t.Fatal("want the open to fail")
		}
	})
	t.Run("a plan with no project table", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := CreateLocalProject(dir, "p1", "nat", ""); err != nil {
			t.Fatalf("create the plan: %v", err)
		}
		path := filepath.Join(dir, "p1.db")
		db, err := sql.Open("sqlite3", localDSN(path))
		if err != nil {
			t.Fatalf("open the plan directly: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE project`); err != nil {
			t.Fatalf("drop the table: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if _, err := CreateLocalProject(dir, "p1", "nat", ""); err == nil {
			t.Fatal("want the write to fail with nowhere to write")
		}
	})
}

// The one thing about how a store keeps its order that a caller cannot work
// out from the operations: whether a slice filed now reads back after the ones
// already there. A plan kept in a file does; Notion's does not, since a created
// row reads back newest first.
func TestAppendsSaysWhereANewSliceLands(t *testing.T) {
	if Over(&fakeAPI{}).Appends() {
		t.Error("a plan kept in Notion reads its newest slice first")
	}
	path := filepath.Join(t.TempDir(), "plan.db")
	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() { _ = l.Close() }()
	if !l.Appends() {
		t.Error("a plan kept in a file reads its slices in the order they were written")
	}
}
