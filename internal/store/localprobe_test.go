package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

func stampedDB(t *testing.T, path, stmts string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(stmts); err != nil {
		t.Fatal(err)
	}
}

func TestReadLocalPlanReadsAPlanAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	p := ProjectOf("3b738308-f654-811c-948d-e1fb36f71df3",
		config.ProjectConfig{Name: "Mine", Backend: config.BackendLocal, PlanDir: dir})
	if err := CreateLocalProject(context.Background(), p, ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, PlanFileName(p.ID))
	id, name, err := ReadLocalPlan(path)
	if err != nil || id != p.ID || name != "Mine" {
		t.Errorf("ReadLocalPlan = %q, %q, %v", id, name, err)
	}
}

func TestReadLocalPlanRefusals(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		stmts string
		want  string
		isNot bool
	}{
		{"no plan schema", `CREATE TABLE t (x);`, "no plan schema", true},
		{"no project table", `CREATE TABLE t (x); PRAGMA user_version = 1;`, "no project row", true},
		{"no project row", `CREATE TABLE project (id TEXT, name TEXT); PRAGMA user_version = 1;`, "no project row", true},
		{"a newer plan", `PRAGMA user_version = 99;`, "newer nat", false},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, string(rune('a'+i))+".db")
			stampedDB(t, path, c.stmts)
			_, _, err := ReadLocalPlan(path)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if errors.Is(err, ErrNotAPlan) != c.isNot {
				t.Errorf("ErrNotAPlan = %v, want %v", !c.isNot, c.isNot)
			}
		})
	}

	if _, _, err := ReadLocalPlan(filepath.Join(dir, "missing.db")); !errors.Is(err, ErrNotAPlan) {
		t.Errorf("a missing file: %v", err)
	}
}
