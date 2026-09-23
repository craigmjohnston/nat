package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

func TestProjectOfNarrowsAConfigEntry(t *testing.T) {
	p := ProjectOf("id", config.ProjectConfig{Name: "n", SlicesDSID: "ds", Backend: "local", PlanDir: "/d"})
	want := Project{ID: "id", Name: "n", SlicesID: "ds", Local: true, PlanDir: "/d"}
	if p != want {
		t.Errorf("got %+v, want %+v", p, want)
	}
	if ProjectOf("id", config.ProjectConfig{Backend: "elsewhere"}).Local {
		t.Errorf("a backend this build does not know must read as Notion")
	}
}

func TestPlanPathHonoursThePlanDir(t *testing.T) {
	isolatedHome(t)
	in, err := PlanPath(Project{ID: "Abc 1", PlanDir: "/plans"})
	if err != nil || in != filepath.Join("/plans", "abc-1.db") {
		t.Errorf("PlanPath with a dir = %q, %v", in, err)
	}
	def, err := PlanPath(Project{ID: "Abc 1"})
	want, _ := LocalPath("Abc 1")
	if err != nil || def != want {
		t.Errorf("PlanPath default = %q, %v; want %q", def, err, want)
	}
}

func TestNewProjectIDLooksLikeAPageID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	a, b := NewProjectID(), NewProjectID()
	if !re.MatchString(a) || a == b {
		t.Errorf("ids %q, %q", a, b)
	}
}

// A local project is a file and nothing else: no remote is needed to open,
// create, or read it, and its conventions are what an agent reads back.
func TestCreateLocalProjectNeedsNoRemote(t *testing.T) {
	isolatedHome(t)
	dir := filepath.Join(t.TempDir(), "plans")
	p := Project{ID: NewProjectID(), Name: "Mine", Local: true, PlanDir: dir}
	ctx := context.Background()
	if err := CreateLocalProject(ctx, p, "# conventions"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, localSlug(p.ID)+".db")); err != nil {
		t.Fatalf("the plan file is not where the config will say: %v", err)
	}
	st, err := ForProject(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := st.Plan(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Project.Name != "Mine" || !plan.Shape.HasAssignee || !plan.Shape.HasBranch {
		t.Errorf("plan = %+v", plan.Project)
	}
	if body, _ := st.Body(ctx, p.ID); body != "# conventions" {
		t.Errorf("conventions = %q", body)
	}
	// Creating again renames and replaces, and is not an error.
	p.Name = "Renamed"
	if err := CreateLocalProject(ctx, p, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLocalProjectFailsWhereThePlanCannotBeLaid(t *testing.T) {
	isolatedHome(t)
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A plan directory that sits under a regular file can never be made.
	err := CreateLocalProject(context.Background(), Project{ID: "x", Local: true, PlanDir: filepath.Join(file, "sub")}, "")
	if err == nil {
		t.Fatal("want an error")
	}
	if _, err := ForProject(context.Background(), Project{ID: "x", Local: true, PlanDir: filepath.Join(file, "sub")}, nil); err == nil {
		t.Fatal("ForProject: want an error")
	}
}

func TestOpenProjectResolveError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if _, err := OpenProject(Project{ID: "x"}); err == nil {
		t.Skip("home dir resolved on this platform")
	}
}
