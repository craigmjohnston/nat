package plugins

import (
	"context"
	"path/filepath"

	"github.com/craigmjohnston/nat/internal/source"
)

// How an installed plugin got where it is.
const (
	// KindManaged is one nat installed, with a record beside it.
	KindManaged = "managed"
	// KindManual is one in nat's plugins directory that nat did not put there.
	KindManual = "manual"
	// KindPath is one found on PATH.
	KindPath = "path"
)

// SourceStatus is one plugin source as last read: its latest release's
// version, or why it could not be read.
type SourceStatus struct {
	Repo    string `json:"repo"`
	Version string `json:"version"`
	Error   string `json:"error"`
	Default bool   `json:"default"`
}

// Installed is one installed plugin. Source and Version are a managed
// install's alone; Update is the newer version its source now offers, empty
// where there is none or nothing to compare.
type Installed struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Update  string `json:"update"`
}

// Available is one plugin a source offers, at that source's latest version.
type Available struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Version     string `json:"version"`
	Installed   bool   `json:"installed"`
}

// Listing is everything there is to say about plugins on this machine: the
// sources, what is installed, and what could be.
type Listing struct {
	Sources   []SourceStatus `json:"sources"`
	Installed []Installed    `json:"installed"`
	Available []Available    `json:"available"`
}

// List reads every source and every installed plugin. A source that could
// not be read is listed with its error and offers nothing — never read as a
// source with no plugins, and never stopping the others being read. Only a
// plugins directory that cannot be read at all fails the listing.
func (m *Manager) List(ctx context.Context, sources []string) (Listing, error) {
	found, err := source.Discover(m.ConfigDir)
	if err != nil {
		return Listing{}, err
	}
	out := Listing{Sources: []SourceStatus{}, Installed: []Installed{}, Available: []Available{}}
	installed := map[string]bool{}
	for _, p := range found {
		installed[p.Name] = true
	}
	latest := map[string]Manifest{}
	for _, repo := range sources {
		st := SourceStatus{Repo: repo, Default: repo == DefaultSource}
		man, err := m.ReadSource(ctx, repo, "")
		if err != nil {
			st.Error = err.Error()
			out.Sources = append(out.Sources, st)
			continue
		}
		st.Version = man.Version
		out.Sources = append(out.Sources, st)
		latest[repo] = man
		for _, e := range man.Plugins {
			out.Available = append(out.Available, Available{
				Name: e.Name, Title: e.Title, Description: e.Description,
				Source: repo, Version: man.Version, Installed: installed[e.Name],
			})
		}
	}
	for _, p := range found {
		out.Installed = append(out.Installed, m.installed(p, latest))
	}
	return out, nil
}

// installed says how one discovered plugin was installed and, for a managed
// one, whether its source has a newer release of it.
func (m *Manager) installed(p source.Plugin, latest map[string]Manifest) Installed {
	in := Installed{Name: p.Name, Path: p.Path, Kind: KindPath}
	if filepath.Dir(p.Path) != m.pluginDir(p.Name) {
		return in
	}
	rec, managed, _ := m.readRecord(p.Name)
	if !managed {
		in.Kind = KindManual
		return in
	}
	in.Kind, in.Source, in.Version = KindManaged, rec.Source, rec.Version
	man, ok := latest[rec.Source]
	if !ok || !Newer(man.Version, rec.Version) {
		return in
	}
	for _, e := range man.Plugins {
		if e.Name == p.Name {
			in.Update = man.Version
		}
	}
	return in
}
