package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
)

// fakeGitHub is a GitHub that serves whatever releases a test files with it,
// by path, and records every path asked for.
type fakeGitHub struct {
	srv   *httptest.Server
	mu    sync.Mutex
	files map[string][]byte
	asked []string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{files: map[string][]byte{}}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.asked = append(g.asked, r.URL.Path)
		body, ok := g.files[r.URL.Path]
		g.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

// manager is a Manager over g, installing under a temp config dir, at a fixed
// time.
func (g *fakeGitHub) manager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		ConfigDir: t.TempDir(),
		BaseURL:   g.srv.URL,
		HTTP:      g.srv.Client(),
		Now:       func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.FixedZone("BST", 3600)) },
	}
}

// release files a release of repo at version, carrying each binary named by
// plugin name, with a manifest listing them with their true digests.
func (g *fakeGitHub) release(repo, version string, latest bool, bins map[string]string) Manifest {
	man := Manifest{Version: version, Plugins: []Entry{}}
	for name, body := range bins {
		sum := sha256.Sum256([]byte(body))
		man.Plugins = append(man.Plugins, Entry{Name: name, Title: strings.ToUpper(name), Description: "the " + name + " plugin",
			Asset: "nat-source-" + name, SHA256: hex.EncodeToString(sum[:])})
		g.file("/"+repo+"/releases/download/v"+version+"/nat-source-"+name, body)
	}
	g.manifest(repo, version, latest, man)
	return man
}

// manifest files man as repo's v<version> manifest, and its latest too.
func (g *fakeGitHub) manifest(repo, version string, latest bool, man any) {
	data, _ := json.Marshal(man)
	g.file("/"+repo+"/releases/download/v"+version+"/nat-plugins.json", string(data))
	if latest {
		g.file("/"+repo+"/releases/latest/download/nat-plugins.json", string(data))
	}
}

func (g *fakeGitHub) file(path, body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.files[path] = []byte(body)
}

func (g *fakeGitHub) paths() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string{}, g.asked...)
}

func TestNew(t *testing.T) {
	m := New("/cfg")
	if m.ConfigDir != "/cfg" || m.BaseURL != "https://github.com" || m.HTTP != http.DefaultClient || m.Now == nil {
		t.Errorf("New = %+v", m)
	}
}

func TestSourcesPutNatsOwnFirstAndEachOnce(t *testing.T) {
	got := Sources(config.Config{PluginSources: []string{"a/b", DefaultSource, "a/b", "c/d"}})
	want := []string{DefaultSource, "a/b", "c/d"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Sources = %v, want %v", got, want)
	}
}

func TestValidRepo(t *testing.T) {
	for _, ok := range []string{"craigmjohnston/nat", "a/b", "Some-One/my.repo_2"} {
		if err := ValidRepo(ok); err != nil {
			t.Errorf("ValidRepo(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "nat", "a/b/c", "-a/b", "a-/b", "a/..", "a/.", "a b/c", "https://github.com/a/b", "a/"} {
		if err := ValidRepo(bad); err == nil {
			t.Errorf("ValidRepo(%q) = nil, want a refusal", bad)
		}
	}
}

func TestValidName(t *testing.T) {
	if err := ValidName("shortcut-2"); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"", "Shortcut", "../x", "a/b", "-a", "a_b"} {
		if ValidName(bad) == nil {
			t.Errorf("ValidName(%q) = nil", bad)
		}
	}
}

func TestNewerComparesDottedIntegers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.10.0", "1.9.2", true},
		{"1.9.2", "1.10.0", false},
		{"1.0.43", "1.0.42", true},
		{"1.2", "1.2.0", false},
		{"1.2.1", "1.2", true},
		{"2", "1.99", true},
		{"1.0.42", "1.0.42", false},
		{"1.0.x", "1.0.1", false},
		{"1.0.2", "junk", false},
		{"", "1", false},
		{"1.-1", "1.0", false},
		{"1.+2", "1.1", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
