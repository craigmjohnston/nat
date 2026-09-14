package store

import (
	"context"
	"fmt"
)

// ForProject is the one place a project's store is put together: it always
// opens the project's plan file, and wraps it in [Mirror] where the project
// has a workspace behind it — every project this build knows how to track,
// for now, which is why remote is required rather than optional. A caller
// builds one instead of calling [Over] and [OpenLocal] for itself, so every
// headless command reads and writes a plan the same way the board will once
// it is routed through here too.
//
// A file that has never been pulled from the workspace — no project row at
// all, which is exactly what a first run looks like — is hydrated from it
// before either is handed back, so the very first command run against a
// project sees the plan rather than an empty one.
func ForProject(ctx context.Context, p Project, remote *Notion) (Store, error) {
	path, err := LocalPath(p.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve the plan file: %w", err)
	}
	local, err := OpenLocal(path)
	if err != nil {
		return nil, err
	}
	m := Mirror(local, remote, p)
	// Unordered: a headless command has no board to read a view order for,
	// and the board's own domain rule about it is [Mirrored.ensureHydrated]'s
	// to apply, not this one's.
	if err := m.ensureHydrated(ctx, false); err != nil {
		return nil, err
	}
	return m, nil
}
