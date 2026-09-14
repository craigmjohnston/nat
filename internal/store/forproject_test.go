package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

// isolatedHome points HOME (and XDG_DATA_HOME, for a non-darwin run of this
// same test) at a directory of the test's own, so [ForProject] — which opens
// a file under [LocalDir] rather than a path the test hands it directly —
// never comes near this machine's real plans.
func isolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
}

// TestForProjectHydratesAnUnpulledPlan checks that a project named for the
// first time — no plan file at all yet — comes back already filled from the
// workspace, rather than empty until something else thinks to pull it.
func TestForProjectHydratesAnUnpulledPlan(t *testing.T) {
	isolatedHome(t)
	api := fullPlanAPI()

	st, err := ForProject(context.Background(), project(), Over(api))
	if err != nil {
		t.Fatalf("ForProject: %v", err)
	}

	plan, err := st.Plan(context.Background(), project())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Project.Slices) != 4 {
		t.Errorf("slices = %d, want the 4 the workspace holds", len(plan.Project.Slices))
	}
	if got := len(api.calls); got == 0 {
		t.Errorf("calls = %d, want the hydrate to have read the workspace", got)
	}
}

// TestForProjectLeavesAnAlreadyHydratedPlanAlone checks that a second run
// against the same plan file does not pull again — the file already answers
// for itself, and pulling on every command would be a request nobody asked
// for.
func TestForProjectLeavesAnAlreadyHydratedPlanAlone(t *testing.T) {
	isolatedHome(t)
	api := fullPlanAPI()
	ctx := context.Background()

	if _, err := ForProject(ctx, project(), Over(api)); err != nil {
		t.Fatalf("ForProject (first): %v", err)
	}
	calledAfterFirst := len(api.calls)

	st, err := ForProject(ctx, project(), Over(api))
	if err != nil {
		t.Fatalf("ForProject (second): %v", err)
	}
	if got := len(api.calls); got != calledAfterFirst {
		t.Errorf("calls after the second open = %d, want no more than the first open's %d", got, calledAfterFirst)
	}

	plan, err := st.Plan(ctx, project())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Project.Slices) != 4 {
		t.Errorf("slices = %d, want the plan the first open pulled", len(plan.Project.Slices))
	}
}

// TestForProjectReportsAFailedPull checks that a workspace that will not
// answer the hydrate fails the whole call — a store nobody could fill in is
// not one to hand back regardless.
func TestForProjectReportsAFailedPull(t *testing.T) {
	isolatedHome(t)
	boom := errors.New("notion down")
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, boom }}

	if _, err := ForProject(context.Background(), project(), Over(api)); !errors.Is(err, boom) {
		t.Errorf("ForProject err = %v, want %v", err, boom)
	}
}

// TestForProjectReportsAnUnresolvableHome checks that a machine with no home
// directory to keep a plan under fails before ever reaching the workspace —
// the same refusal [LocalPath] itself gives, not swallowed here.
func TestForProjectReportsAnUnresolvableHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	if _, err := ForProject(context.Background(), project(), Over(&fakeAPI{})); err == nil {
		t.Error("ForProject with no home: want an error")
	}
}

// TestForProjectReportsAFileWhereItsPlanDirectoryBelongs checks that a plan
// directory [OpenLocal] cannot make — something else is sitting where it
// needs to go — fails the call rather than reading it as an empty plan.
func TestForProjectReportsAFileWhereItsPlanDirectoryBelongs(t *testing.T) {
	isolatedHome(t)
	dir, err := LocalDir()
	if err != nil {
		t.Fatalf("LocalDir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ForProject(context.Background(), project(), Over(&fakeAPI{})); err == nil {
		t.Error("ForProject over a blocked plan directory: want an error")
	}
}

// TestForProjectReportsAFailedHydratedCheck checks that a plan file this
// build can no longer make sense of — its project row gone, in this case —
// fails the call rather than silently pulling on top of whatever is left.
func TestForProjectReportsAFailedHydratedCheck(t *testing.T) {
	isolatedHome(t)
	api := fullPlanAPI()
	ctx := context.Background()
	if _, err := ForProject(ctx, project(), Over(api)); err != nil {
		t.Fatalf("ForProject (first): %v", err)
	}

	path, err := LocalPath(project().ID)
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	l2, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("reopen the plan: %v", err)
	}
	write(t, l2, `DROP TABLE project`)
	if err := l2.Close(); err != nil {
		t.Fatalf("close the plan: %v", err)
	}

	if _, err := ForProject(ctx, project(), Over(api)); err == nil {
		t.Error("ForProject over a plan with no project table: want an error")
	}
}
