package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDirs(t *testing.T) {
	d := Dirs{Getenv: env(map[string]string{"HOME": "/h"})}
	if got := d.ConfigFile(); got != "/h/.config/nat-source-shortcut/config.json" {
		t.Errorf("ConfigFile = %q", got)
	}
	if got := d.CacheDir(); got != "/h/Library/Caches/nat-source-shortcut" {
		t.Errorf("CacheDir = %q", got)
	}
	d = Dirs{Getenv: env(map[string]string{"HOME": "/h", "XDG_CONFIG_HOME": "/x/c", "XDG_CACHE_HOME": "/x/k"})}
	if got := d.ConfigFile(); got != "/x/c/nat-source-shortcut/config.json" {
		t.Errorf("ConfigFile = %q", got)
	}
	if got := d.CacheDir(); got != "/x/k/nat-source-shortcut" {
		t.Errorf("CacheDir = %q", got)
	}
	home, _ := os.UserHomeDir()
	d = Dirs{Getenv: env(nil)}
	if got := d.ConfigFile(); got != filepath.Join(home, ".config/nat-source-shortcut/config.json") {
		t.Errorf("ConfigFile with no HOME = %q", got)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	f, err := Load(path)
	if err != nil || len(f.Projects) != 0 {
		t.Fatalf("Load missing = %v, %v", f, err)
	}
	p := f.Project("p1")
	if !reflect.DeepEqual(p.Segments, DefaultSegments()) {
		t.Errorf("default segments = %v", p.Segments)
	}
	p.Team = "board"
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	g, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Project("p1"); got.Team != "board" || len(got.Segments) != 1 {
		t.Errorf("round trip = %+v", got)
	}
	// Every segment removed stays removed: an empty list isn't the default.
	g.Project("p1").Segments = []Segment{}
	if err := g.Save(path); err != nil {
		t.Fatal(err)
	}
	h, _ := Load(path)
	if got := h.Project("p1").Segments; len(got) != 0 {
		t.Errorf("emptied segments came back as %v", got)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte("{"), 0o600)
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("bad JSON: %v", err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "can't read") {
		t.Errorf("directory: %v", err)
	}
	null := filepath.Join(dir, "null.json")
	_ = os.WriteFile(null, []byte("{}"), 0o600)
	if f, err := Load(null); err != nil || f.Projects == nil {
		t.Errorf("empty object: %v, %v", f, err)
	}
}

func TestSaveErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	_ = os.WriteFile(blocker, nil, 0o600)
	f := &File{Projects: map[string]*Project{}}
	if err := f.Save(filepath.Join(blocker, "x", "config.json")); err == nil {
		t.Error("save under a file succeeded")
	}
	// The temp file can't be written where a directory sits on its name.
	path := filepath.Join(dir, "c.json")
	_ = os.Mkdir(path+".tmp", 0o700)
	if err := f.Save(path); err == nil {
		t.Error("save over a directory tmp succeeded")
	}
	// The rename can't land on a non-empty directory.
	path2 := filepath.Join(dir, "d.json")
	_ = os.MkdirAll(filepath.Join(path2, "x"), 0o700)
	if err := f.Save(path2); err == nil {
		t.Error("rename onto a directory succeeded")
	}
}

func TestSegments(t *testing.T) {
	p := &Project{Segments: []Segment{{ID: "mine", Name: "Mine"}, {ID: "bugs", Name: "Bugs"}}}
	if i := p.Find("ready/bugs"); i != 1 {
		t.Errorf("Find = %d", i)
	}
	if i := p.Find("bugs"); i != -1 {
		t.Errorf("Find bare id = %d", i)
	}
	for name, want := range map[string]string{
		"Board work": "board-work",
		"Mine":       "mine-2",
		"!!!":        "segment",
		" Bugs ":     "bugs-2",
	} {
		if got := p.NewSegmentID(name); got != want {
			t.Errorf("NewSegmentID(%q) = %q, want %q", name, got, want)
		}
	}
	p.Segments = append(p.Segments, Segment{ID: "mine-2"})
	if got := p.NewSegmentID("mine"); got != "mine-3" {
		t.Errorf("NewSegmentID(mine) = %q", got)
	}
}
