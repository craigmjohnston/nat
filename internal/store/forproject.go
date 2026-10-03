package store

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/source"
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
//
// A project with no workspace behind it is the file alone, and remote may be
// nil: nothing here reads a token or makes a request for it.
//
// A source project is the file too, wrapped in [Sourced] over the plugin's
// client src, which is required for one — a source project opened with no
// plugin to tell would file tasks under containers nobody ever hears about.
// plugin is the project as the plugin is told of it, built by the caller since
// a [Project] deliberately carries no working directory. remote may be nil
// here as well.
func ForProject(ctx context.Context, p Project, remote *Notion, plugin source.Project, src source.Client) (Store, error) {
	if p.Source != "" && src == nil {
		return nil, fmt.Errorf("the %q project's plan is kept with the %s task source, and no client for it was given", p.Name, p.Source)
	}
	local, err := OpenProject(p)
	if err != nil {
		return nil, err
	}
	if p.Local {
		return local, nil
	}
	if p.Source != "" {
		return NewSourced(local, src, plugin, p.Source), nil
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

// OpenProject opens the plan file a project keeps its plan in — nat's own
// data directory, or the directory its config names.
func OpenProject(p Project) (*Local, error) {
	path, err := PlanPath(p)
	if err != nil {
		return nil, fmt.Errorf("resolve the plan file: %w", err)
	}
	return OpenLocal(path)
}

// NewProjectID mints the ID of a project of nat's own, shaped like a Notion page
// ID so that everything which carries one around — flags, the config's keys,
// prompts, the app — cannot tell the two apart.
func NewProjectID() string { return newLocalID() }

// CreateLocalProject lays down the plan of a project with no workspace behind
// it — a local project's, or a source project's, which is the same file: the
// file, with its project row and conventions. It touches Notion nowhere.
// It is written before the config entry that names it, since a config naming a
// project whose plan could not be laid down is one every later command fails on.
func CreateLocalProject(ctx context.Context, p Project, conventions string) error {
	local, err := OpenProject(p)
	if err != nil {
		return err
	}
	defer func() { _ = local.Close() }()
	return local.InitProject(ctx, p, conventions)
}
