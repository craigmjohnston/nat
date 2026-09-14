package cli

import (
	"database/sql"
	"testing"

	"github.com/craigmjohnston/nat/internal/store"
)

// seedHydratedSlice writes a plan file directly, already marked as pulled
// from a workspace (synced_at set) — so a command run against it finds
// store.ForProject's own hydrate a no-op — with one slice in it. break, when
// given, runs after the slice is seeded, so a test can corrupt exactly the
// table or column its own write is meant to trip over, without that
// corruption also breaking the hydrate check itself.
//
// It exists because several commands' own local-write failure branches
// cannot be reached through the fakeAPI at all: by the time such a write
// runs, the slice is already in the local file (read there, or taken in
// from a first-time pull), so nothing about the workspace being unreachable
// touches it — only the file itself, underneath the store, can still fail.
func seedHydratedSlice(t *testing.T, projectID, sliceID, title, status string, breakIt func(db *sql.DB)) string {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	l, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close the plan after creating it: %v", err)
	}

	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to seed it: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the seeding connection: %v", err)
		}
	}()
	if _, err := db.Exec(`INSERT INTO project (id, name, synced_at, has_assignee, has_branch) VALUES (?, ?, ?, 1, 1)`,
		projectID, "nat", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed the project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO slices (id, title, status, position) VALUES (?, ?, ?, 0)`,
		sliceID, title, status); err != nil {
		t.Fatalf("seed the slice: %v", err)
	}
	if breakIt != nil {
		breakIt(db)
	}
	return path
}
