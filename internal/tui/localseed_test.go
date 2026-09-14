package tui

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// seedLocalPlan writes p into the project's own plan file directly, through a
// database connection of its own — the same seam internal/cli's tests use
// (seedHydratedSlice) — so a store opened against it, which every board write
// now goes through (see [App.storeFor]), finds every row a test set only on
// [App].project and [App].board directly rather than through a real load. A
// test that seeds only the board leaves every write and every re-read looking
// for a slice the store has never heard of.
//
// The project row is stamped hydrated with the real clock, never a test's own
// frozen timeNow: stamping it with a frozen one would read the plan as stale
// on the very next load and send [store.Mirrored.Plan] pulling against
// whatever client the test wired in, which is not what seeding the file
// directly is for. Both Assignee and Branch are recorded as present — every
// project these fixtures model carries both columns, the same assumption the
// fixtures already made through the fake Notion schema before routing through
// the file.
//
// Calling it more than once for the same project — a test landing a second
// plan, to watch a reading the first one settled not being repeated —
// replaces the file's rows rather than adding to them: every milestone,
// slice and dependency is cleared first, so a second seed is the plan as it
// now stands rather than a duplicate of the first.
func seedLocalPlan(t *testing.T, projectID string, p domain.Project) {
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

	for _, stmt := range []string{
		`DELETE FROM slice_deps`, `DELETE FROM sync`, `DELETE FROM slices`, `DELETE FROM milestones`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("clear the plan before reseeding (%s): %v", stmt, err)
		}
	}

	if _, err := db.Exec(
		`INSERT INTO project (id, name, synced_at, has_assignee, has_branch) VALUES (?, ?, ?, 1, 1)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, synced_at = excluded.synced_at,
		   has_assignee = excluded.has_assignee, has_branch = excluded.has_branch`,
		projectID, p.Name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed the project: %v", err)
	}
	for i, m := range p.Milestones {
		if _, err := db.Exec(`INSERT INTO milestones (name, position, select_type) VALUES (?, ?, ?)`,
			m.Name, i, m.SelectType); err != nil {
			t.Fatalf("seed the milestone %q: %v", m.Name, err)
		}
	}
	for i, s := range p.Slices {
		status := s.StatusName
		if status == "" {
			status = string(s.Status)
		}
		// A local plan has no directory of users: the assignee identity is the
		// name itself, the same fallback scanLocalSlice reads back with —
		// unless the fixture named an identity of its own.
		assignee := s.AssigneeName
		if len(s.AssigneeIDs) > 0 {
			assignee = s.AssigneeIDs[0]
		}
		if _, err := db.Exec(
			`INSERT INTO slices (id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, url)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.Name, status, s.MilestoneID, i, assignee, s.AssigneeName, s.Repo, s.Branch, s.PRURL, s.URL); err != nil {
			t.Fatalf("seed the slice %q: %v", s.Name, err)
		}
	}
	for _, s := range p.Slices {
		for j, dep := range s.DependsOn {
			if _, err := db.Exec(`INSERT INTO slice_deps (slice_id, depends_on, position) VALUES (?, ?, ?)`,
				s.ID, dep, j); err != nil {
				t.Fatalf("seed the dependency %s -> %s: %v", s.ID, dep, err)
			}
		}
	}
}

// makeLocalPlanStale backdates a seeded plan's own synced_at past
// store.planStaleAfter, so a test can watch an ordinary (non-forced) read
// pull for itself — see store.Mirrored.Plan — rather than serving straight
// from the fresh-stamped file [seedLocalPlan] otherwise leaves behind.
func makeLocalPlanStale(t *testing.T, projectID string) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to backdate it: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the backdating connection: %v", err)
		}
	}()
	if _, err := db.Exec(`UPDATE project SET synced_at = ? WHERE id = ?`,
		time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), projectID); err != nil {
		t.Fatalf("backdate the plan: %v", err)
	}
}

// dropLocalSlice deletes a slice's row from the project's plan file directly,
// so a write to it fails the way [store.Local]'s own "no such slice" refusal
// does — the local write half of a mirrored write, which a broken remote
// client alone can no longer exercise: a push that fails is logged and
// swallowed rather than failing the write, since the local half already
// landed. This is what still can fail it.
func dropLocalSlice(t *testing.T, projectID, sliceID string) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to drop the slice: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the dropping connection: %v", err)
		}
	}()
	if _, err := db.Exec(`DELETE FROM slice_deps WHERE slice_id = ? OR depends_on = ?`, sliceID, sliceID); err != nil {
		t.Fatalf("drop the slice's dependencies: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM slices WHERE id = ?`, sliceID); err != nil {
		t.Fatalf("drop the slice: %v", err)
	}
}

// breakLocalColumn drops a column from the project's plan file directly, so a
// local write or read that touches it fails the way a corrupted file would —
// [internal/cli]'s own seedHydratedSlice uses the same trick for exactly the
// same reason: some local-write failure branches have no other way in, since
// the fakeAPI's own failures are the workspace's, and a write that lands in
// the file first no longer depends on the workspace at all to succeed.
func breakLocalColumn(t *testing.T, projectID, table, column string) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to break it: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the breaking connection: %v", err)
		}
	}()
	if _, err := db.Exec(`ALTER TABLE ` + table + ` DROP COLUMN ` + column); err != nil {
		t.Fatalf("drop %s.%s: %v", table, column, err)
	}
}

// setLocalShape overrides whether the project's own file records ownership
// and a branch at all, past whatever [seedLocalPlan] stamped it — a test
// whose fixture models a project with neither column has to say so on the
// file's own copy too, now that a release, a claim or a hand-back reads the
// shape it writes in from there first, rather than from whatever page the
// fakeNotion answers with.
func setLocalShape(t *testing.T, projectID string, hasAssignee, hasBranch bool) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to set its shape: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the shape-setting connection: %v", err)
		}
	}()
	if _, err := db.Exec(`UPDATE project SET has_assignee = ?, has_branch = ? WHERE id = ?`,
		boolInt(hasAssignee), boolInt(hasBranch), projectID); err != nil {
		t.Fatalf("set the plan's shape: %v", err)
	}
}

// boolInt is a bool as the has_assignee/has_branch columns hold it.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// setLocalSliceStatus writes a slice's status directly into the project's
// plan file, the way a headless command running against the very same file
// would — so a test can watch a re-read pick up a change nothing on [App]
// itself made.
func setLocalSliceStatus(t *testing.T, projectID, sliceID, status string) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the plan to update it: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the updating connection: %v", err)
		}
	}()
	if _, err := db.Exec(`UPDATE slices SET status = ? WHERE id = ?`, status, sliceID); err != nil {
		t.Fatalf("update the slice: %v", err)
	}
}

// breakLocalPlanFile occupies the project's plan file's own path with a
// directory, so store.OpenLocal — and so App.storeFor, since [App].stores
// starts out empty every time a test constructs one — fails to open it at
// all: a directory is not a database file. It has to run after testConfig,
// which is what pins HOME to begin with, and before anything has opened the
// store for real — a store already cached in App.stores is unaffected,
// since storeFor never reopens one it already holds.
func breakLocalPlanFile(t *testing.T, projectID string) {
	t.Helper()
	path, err := store.LocalPath(projectID)
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	// Whatever a prior seed left at the path — a real plan file — has to go
	// first, or occupying it with a directory fails the same way opening it
	// as a database would.
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("clear the plan path: %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("occupy the plan path with a directory: %v", err)
	}
}
