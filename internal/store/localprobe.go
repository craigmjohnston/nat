package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotAPlan is what [ReadLocalPlan] wraps for a file that opens as a
// database and is not one of nat's plans — the one refusal a caller shows as
// "not what was looked for" rather than as a fault.
var ErrNotAPlan = errors.New("not a nat plan")

// ReadLocalPlan reports whose plan a file is — the project ID and name its
// project row holds — without changing it. [OpenLocal] is the wrong way to
// ask: it creates and migrates, so pointed at some other SQLite file it would
// lay a plan's tables into it. This opens read-only and refuses anything that
// is not already a plan of a schema this build reads.
func ReadLocalPlan(path string) (id, name string, err error) {
	// sql.Open only parses the DSN here; the file is first touched by the query.
	db, _ := sql.Open("sqlite3", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	defer func() { _ = db.Close() }()

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return "", "", fmt.Errorf("%s: %w: %v", path, ErrNotAPlan, err)
	}
	if version < 1 {
		return "", "", fmt.Errorf("%s: %w: it carries no plan schema", path, ErrNotAPlan)
	}
	if version > localSchemaVersion {
		return "", "", fmt.Errorf("the plan at %s was written by a newer nat (schema %d, this build reads %d)",
			path, version, localSchemaVersion)
	}
	err = db.QueryRow(`SELECT id, name FROM project LIMIT 1`).Scan(&id, &name)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w: it holds no project row", path, ErrNotAPlan)
	}
	return id, name, nil
}

// PlanFileName is the name a project's plan file has inside its plan directory.
func PlanFileName(projectID string) string { return localSlug(projectID) + ".db" }
