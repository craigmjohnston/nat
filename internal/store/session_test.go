package store

import (
	"context"
	"strings"
	"testing"
)

func TestLocalSessionRoundTrip(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()

	added, err := l.AddSession(ctx, Project{ID: "proj"}, NewSession{ID: "s1", Dir: "/repo", Branch: "session/abcd1234"})
	if err != nil {
		t.Fatalf("AddSession: %v", err)
	}
	if added.ID != "s1" || added.Dir != "/repo" || added.Branch != "session/abcd1234" {
		t.Errorf("AddSession result = %+v, want the session as filed", added)
	}
	if added.StartedAt.IsZero() {
		t.Errorf("AddSession result has no StartedAt")
	}
	if added.Ended() {
		t.Errorf("a freshly added session reads as ended")
	}

	sessions, err := l.Sessions(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("Sessions = %+v, want exactly the one session", sessions)
	}

	if err := l.EndSession(ctx, "s1"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	sessions, err = l.Sessions(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Sessions after end: %v", err)
	}
	if !sessions[0].Ended() {
		t.Errorf("session after EndSession reads as not ended")
	}

	if err := l.DeleteSession(ctx, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	sessions, err = l.Sessions(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Sessions after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Sessions after delete = %+v, want none", sessions)
	}
}

func TestLocalSessionEndAndDeleteRefuseAnUnknownID(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()

	if err := l.EndSession(ctx, "nope"); err == nil {
		t.Error("EndSession on an unknown ID: want an error")
	}
	if err := l.DeleteSession(ctx, "nope"); err == nil {
		t.Error("DeleteSession on an unknown ID: want an error")
	}
}

func TestNewSessionIDsAreDistinct(t *testing.T) {
	a, b := NewSessionID(), NewSessionID()
	if a == b {
		t.Errorf("NewSessionID produced the same ID twice: %q", a)
	}
	if a == "" {
		t.Error("NewSessionID produced an empty ID")
	}
}

func TestNotionSessionMethodsRefuse(t *testing.T) {
	n := Over(&fakeAPI{})
	ctx := context.Background()

	if _, err := n.AddSession(ctx, Project{ID: "proj"}, NewSession{ID: "s1"}); err == nil {
		t.Error("Notion.AddSession: want a refusal")
	}
	if _, err := n.Sessions(ctx, Project{ID: "proj"}); err == nil {
		t.Error("Notion.Sessions: want a refusal")
	}
	if err := n.EndSession(ctx, "s1"); err == nil {
		t.Error("Notion.EndSession: want a refusal")
	}
	if err := n.DeleteSession(ctx, "s1"); err == nil {
		t.Error("Notion.DeleteSession: want a refusal")
	}
}

// TestMirroredSessionsNeverReachTheWorkspace is this slice's own acceptance
// criterion: a Notion-backed project's session rows never reach Notion. The
// fake API answers nothing at all, so any request a session method made
// would fail the read/write it tried and, either way, would show up in
// api.calls — which stays empty throughout.
func TestMirroredSessionsNeverReachTheWorkspace(t *testing.T) {
	api := &fakeAPI{}
	m, _ := mirroredPlan(t, api)
	ctx := context.Background()

	added, err := m.AddSession(ctx, Project{ID: "proj"}, NewSession{ID: "s1", Dir: "/repo", Branch: "session/abcd1234"})
	if err != nil {
		t.Fatalf("AddSession: %v", err)
	}
	if _, err := m.Sessions(ctx, Project{ID: "proj"}); err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if err := m.EndSession(ctx, added.ID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if err := m.DeleteSession(ctx, added.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if len(api.calls) != 0 {
		t.Errorf("calls to the workspace = %v, want none: sessions are local-only", api.calls)
	}
}

// A closed database is a cheap way of failing every read of the sessions
// table, the same trick TestLocalReadsNameTheirFileWhenTheyFail plays for
// every other reader in this file.
func TestLocalSessionsReportsAFailedRead(t *testing.T) {
	l, path := openPlan(t)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := l.Sessions(context.Background(), Project{ID: "proj"}); err == nil {
		t.Fatal("Sessions on a closed plan: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A row that will not scan is a corrupt plan: a hand-edited view standing
// in for the table can answer a NULL where a plain string is expected.
func TestLocalSessionsReportsARowThatWillNotScan(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `DROP TABLE sessions`)
	write(t, l, `CREATE VIEW sessions (id, started_at, dir, branch, ended_at) AS
		SELECT NULL, '2024-01-01T00:00:00Z', '', '', NULL`)
	if _, err := l.Sessions(context.Background(), Project{ID: "proj"}); err == nil {
		t.Fatal("Sessions with an unscannable row: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A read that fails part way through its rows is reported rather than
// treated as a short list — the same integer-overflow trick
// TestLocalNamesItsFileWhenAReadFailsPartWayThrough plays.
func TestLocalSessionsReportsAReadFailingPartWayThrough(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `DROP TABLE sessions`)
	write(t, l, `CREATE VIEW sessions (id, started_at, dir, branch, ended_at) AS
		SELECT 'a', '2024-01-01T00:00:00Z', '', '', NULL
		UNION ALL SELECT 'b', abs(-9223372036854775808), '', '', NULL`)
	if _, err := l.Sessions(context.Background(), Project{ID: "proj"}); err == nil {
		t.Fatal("Sessions failing part way through: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A started_at or ended_at stamp that will not parse as RFC3339Nano is
// exactly the corruption a hand-edited plan could leave, and each is its
// own read.
func TestLocalSessionsReportsAnUnparseableTimestamp(t *testing.T) {
	l, _ := openPlan(t)
	write(t, l, `INSERT INTO sessions (id, started_at, dir, branch) VALUES ('s1', 'not-a-time', '', '')`)
	if _, err := l.Sessions(context.Background(), Project{ID: "proj"}); err == nil {
		t.Fatal("Sessions with an unparseable start time: want an error")
	}

	l2, _ := openPlan(t)
	write(t, l2, `INSERT INTO sessions (id, started_at, dir, branch, ended_at)
		VALUES ('s2', '2024-01-01T00:00:00Z', '', '', 'not-a-time')`)
	if _, err := l2.Sessions(context.Background(), Project{ID: "proj"}); err == nil {
		t.Fatal("Sessions with an unparseable end time: want an error")
	}
}

// AddSession, EndSession and DeleteSession each report a write that fails
// underneath them, named with the file, exactly as every other writer in
// this package does.
func TestLocalSessionWritesReportAFailedWrite(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `DROP TABLE sessions`)
	ctx := context.Background()

	if _, err := l.AddSession(ctx, Project{ID: "proj"}, NewSession{ID: "s1"}); err == nil {
		t.Error("AddSession with no sessions table: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("AddSession error = %q, want the path named", err)
	}
	if err := l.EndSession(ctx, "s1"); err == nil {
		t.Error("EndSession with no sessions table: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("EndSession error = %q, want the path named", err)
	}
	if err := l.DeleteSession(ctx, "s1"); err == nil {
		t.Error("DeleteSession with no sessions table: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("DeleteSession error = %q, want the path named", err)
	}
}
