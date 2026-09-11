package cli

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/craigmjohnston/nat/internal/store"
)

// A plan kept in a file prints exactly as the same plan kept in Notion does.
// That is the whole claim of the store seam, and the cheapest way to hold it to
// account is to assert the very output TestInfoPrintsTheProjectAsMarkdown
// asserts, built from a local plan instead of from a Notion fake.
func TestInfoPrintsAPlanKeptLocally(t *testing.T) {
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
	fillLocalPlan(t, path)

	ctx := context.Background()
	conventions, err := st.Body(ctx, "project-1")
	if err != nil {
		t.Fatalf("read the conventions: %v", err)
	}
	plan, err := st.Plan(ctx, store.Project{ID: "project-1", Name: "nat"})
	if err != nil {
		t.Fatalf("read the plan: %v", err)
	}

	want := `# nat

Branch per slice.

## Milestones

- 0. M1: Client — Done
- 1. M2: Board — Queued

## Slices

### M1: Client

- Notion client — Done · Craig Johnston · PR https://github.com/nat/pull/1

### M2: Board

- Render the board — Todo

### Unassigned

- Stray idea — (no status)
`
	if got := infoMarkdown(plan.Project, conventions); got != want {
		t.Errorf("output =\n%s\nwant:\n%s", got, want)
	}
}

// fillLocalPlan writes that plan, through a connection of its own rather than
// through the store: the write half of a local plan is the next slice's work,
// and a second process writing the file while the store holds it open is the
// access pattern the format was chosen for anyway.
func fillLocalPlan(t *testing.T, path string) {
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

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO project (id, name, conventions) VALUES (?, ?, ?)`,
			[]any{"project-1", "nat", "Branch per slice."}},
		{`INSERT INTO milestones (name, position) VALUES (?, ?), (?, ?)`,
			[]any{"M1: Client", 0, "M2: Board", 1}},
		{`INSERT INTO slices (id, title, status, milestone, position, assignee, pr) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			[]any{"s1", "Notion client", "Done", "M1: Client", 0, "Craig Johnston", "https://github.com/nat/pull/1"}},
		{`INSERT INTO slices (id, title, status, milestone, position) VALUES (?, ?, ?, ?, ?)`,
			[]any{"s2", "Render the board", "Todo", "M2: Board", 0}},
		{`INSERT INTO slices (id, title, status, position) VALUES (?, ?, ?, ?)`,
			[]any{"s3", "Stray idea", "", 1}},
	}
	for _, s := range statements {
		if _, err := db.Exec(s.query, s.args...); err != nil {
			t.Fatalf("write %q: %v", s.query, err)
		}
	}
}
