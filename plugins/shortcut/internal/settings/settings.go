// Package settings is the plugin's own per-project configuration, keyed by
// nat's project id — nat stores nothing for a plugin — and the directories
// the plugin keeps its config and cache in.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Dirs resolves where the plugin keeps things, from the environment it was
// run in.
type Dirs struct {
	Getenv func(string) string
}

func (d Dirs) home() string {
	if h := d.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// ConfigFile is $XDG_CONFIG_HOME/nat-source-shortcut/config.json, else
// ~/.config/nat-source-shortcut/config.json — the same XDG rule nat uses for
// its own config, so a scratch XDG_CONFIG_HOME keeps a test run off the real
// one.
func (d Dirs) ConfigFile() string {
	base := d.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(d.home(), ".config")
	}
	return filepath.Join(base, "nat-source-shortcut", "config.json")
}

// CacheDir is $XDG_CACHE_HOME/nat-source-shortcut, else
// ~/Library/Caches/nat-source-shortcut.
func (d Dirs) CacheDir() string {
	if base := d.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "nat-source-shortcut")
	}
	return filepath.Join(d.home(), "Library", "Caches", "nat-source-shortcut")
}

// Segment is one saved search under Ready. ID is fixed when the segment is
// made and never re-derived from Name, since nat and gnat remember a group by
// its id (`ready/<id>`).
type Segment struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

// Project is one nat project's settings.
type Project struct {
	// Segments is nil for "never configured" (the default, one Mine
	// segment), and empty once the user has removed every one.
	Segments []Segment `json:"segments"`
	// StartedState and DoneState name the state a claim and a last merge
	// move a story to — a state name or numeric id within the story's
	// workflow. Empty means the first state of that type by position.
	StartedState string `json:"started_state,omitempty"`
	DoneState    string `json:"done_state,omitempty"`
	// Team, when set, restricts every search to one team (its mention name).
	Team string `json:"team,omitempty"`
}

// DefaultSegments is what a project starts with.
func DefaultSegments() []Segment {
	return []Segment{{ID: "mine", Name: "Mine", Query: "owner:me"}}
}

// File is the whole config file.
type File struct {
	Projects map[string]*Project `json:"projects"`
}

// Load reads path; a missing file is an empty config.
func Load(path string) (*File, error) {
	f := &File{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		f.Projects = map[string]*Project{}
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("shortcut: can't read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, f); err != nil {
		return nil, fmt.Errorf("shortcut: %s is not valid JSON", path)
	}
	if f.Projects == nil {
		f.Projects = map[string]*Project{}
	}
	return f, nil
}

// Project is id's settings with the defaults filled in. It is the entry
// itself when there is one, so a change made to it is saved by Save.
func (f *File) Project(id string) *Project {
	p := f.Projects[id]
	if p == nil {
		p = &Project{}
		f.Projects[id] = p
	}
	if p.Segments == nil {
		p.Segments = DefaultSegments()
	}
	return p
}

// Save writes f to path atomically, creating its directory.
func (f *File) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("shortcut: can't save settings: %w", err)
	}
	// A map of plain structs: Marshal cannot fail on it.
	b, _ := json.MarshalIndent(f, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("shortcut: can't save settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("shortcut: can't save settings: %w", err)
	}
	return nil
}

// GroupID is a segment's sidebar group id.
func (s Segment) GroupID() string { return "ready/" + s.ID }

// Find is the segment whose group id is groupID, and its index; -1 when none.
func (p *Project) Find(groupID string) int {
	for i, s := range p.Segments {
		if s.GroupID() == groupID {
			return i
		}
	}
	return -1
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// NewSegmentID is a slug of name not already used by a segment of p.
func (p *Project) NewSegmentID(name string) string {
	base := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "segment"
	}
	id := base
	for n := 2; p.Find("ready/"+id) >= 0; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}
