package tui

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/craigmjohnston/nat/internal/store"
)

// The board draws a project kept in a file exactly as it draws one kept in
// Notion. Nothing above the store knows which it is reading — that is what the
// seam is for — so the test is the two renders being the same bytes.
func TestBoardRendersAFileBackedProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	st, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	}()
	writeLocalPlan(t, path)

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

// writeLocalPlan writes testProject into a plan of its own, through a
// connection of its own: the write half of a local plan is the next slice's
// work, and a second process writing the file while the store holds it open is
// the access pattern the format was chosen for anyway.
func writeLocalPlan(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to write it: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the writer: %v", err)
		}
	}()

	p := testProject()
	for i, m := range p.Milestones {
		if _, err := db.Exec(`INSERT INTO milestones (name, position) VALUES (?, ?)`, m.Name, i); err != nil {
			t.Fatalf("write the milestone %q: %v", m.Name, err)
		}
	}
	for i, s := range p.Slices {
		status := s.StatusName
		if status == "" {
			status = string(s.Status)
		}
		if _, err := db.Exec(
			`INSERT INTO slices (id, title, status, milestone, position, assignee, pr) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.Name, status, s.MilestoneID, i, s.AssigneeName, s.PRURL); err != nil {
			t.Fatalf("write the slice %q: %v", s.Name, err)
		}
	}
}
