package tui

import (
	"context"
	"testing"

	"github.com/craigmjohnston/nat/internal/store"
)

// The board draws a project kept in a file exactly as it draws one kept in
// Notion. Nothing above the store knows which it is reading — that is what the
// seam is for — so the test is the two renders being the same bytes.
func TestBoardRendersAFileBackedProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
	seedLocalPlan(t, "local-1", testProject())

	path, err := store.LocalPath("local-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	st, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	}()

	plan, err := st.Plan(context.Background(), store.Project{ID: "local-1", Name: "tracker"})
	if err != nil {
		t.Fatalf("read the plan: %v", err)
	}

	local := NewBoard(DefaultStyles())
	local.hideDone = false
	local.SetWidth(60)
	local.SetProject(&plan.Project)

	if got, want := rowNames(&local), rowNames(newTestBoard()); len(got) != len(want) {
		t.Fatalf("rows = %v, want the same rows as the same plan read from Notion: %v", got, want)
	}
	if got, want := local.View(), newTestBoard().View(); got != want {
		t.Errorf("render =\n%s\nwant the render of the same plan read from Notion:\n%s", got, want)
	}
}
