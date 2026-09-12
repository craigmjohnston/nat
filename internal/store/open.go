package store

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/craigmjohnston/nat/internal/config"
)

// ForProject opens the store a project's plan is kept in, which is the one
// place in the tree that choice is made: a caller names a project and is handed
// something that answers [Store], never having to learn which backend answered.
//
// The Notion half is [Over] on the client the caller already holds, since a
// board and a headless command each make one client for everything they do; the
// local half opens the project's own database, creating it where there is none,
// so a project recorded as local is a plan the first read can already answer
// about rather than a file somebody has to lay down first.
//
// The client may be nil for a local project and must not be for a Notion one:
// that is what makes a machine with no Notion credential able to work a local
// project at all.
func ForProject(projectID string, p config.ProjectConfig, api API) (Store, error) {
	if !p.IsLocal() {
		if api == nil {
			return nil, fmt.Errorf("the project %s is kept in Notion and there is no Notion connection", projectID)
		}
		return Over(api), nil
	}
	path, err := PlanPath(p.PlanDir, projectID)
	if err != nil {
		return nil, err
	}
	return OpenLocal(path)
}

// PlanPath is the file a local project's plan is kept in: [LocalPath] under
// nat's own data directory, or the same filename under the directory the
// project names when it names one — a plan the user would rather keep where
// their own backups reach.
func PlanPath(dir, projectID string) (string, error) {
	if dir == "" {
		return LocalPath(projectID)
	}
	return filepath.Join(dir, localSlug(projectID)+".db"), nil
}

// CreateLocalProject lays down a project's plan as a file and records what the
// plan is a plan of. It is the local half of creating a project — the half that
// stands where Notion's page create stands — and it is deliberately a function
// rather than a method: the caller has no store yet, because making one is the
// thing it is asking for.
//
// It answers with the file it wrote, since that is the whole of where a local
// project lives and is what a command prints for somebody to go and look at.
func CreateLocalProject(dir, projectID, name, conventions string) (string, error) {
	path, err := PlanPath(dir, projectID)
	if err != nil {
		return "", err
	}
	l, err := OpenLocal(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = l.Close() }()
	if err := l.SetProject(context.Background(), projectID, name, conventions); err != nil {
		return "", err
	}
	return path, nil
}

// NewProjectID is the identity a local project is given, since nothing hands
// one back the way creating a Notion page does. It is the same shape a Notion
// page ID has — thirty-two hex characters, dashed — so that everything that
// carries a project ID around, `--project` included, cannot tell the two apart.
func NewProjectID() string { return newLocalID() }
