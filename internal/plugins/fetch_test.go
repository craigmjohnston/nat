package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadSourceReadsTheLatestManifest(t *testing.T) {
	g := newFakeGitHub(t)
	want := g.release("someone/plugins", "1.0.4", true, map[string]string{"demo": "bin"})
	m := g.manager(t)

	got, err := m.ReadSource(context.Background(), "someone/plugins", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.0.4" || len(got.Plugins) != 1 || got.Plugins[0] != want.Plugins[0] {
		t.Errorf("ReadSource = %+v, want %+v", got, want)
	}
	if p := g.paths(); len(p) != 1 || p[0] != "/someone/plugins/releases/latest/download/nat-plugins.json" {
		t.Errorf("asked for %v", p)
	}
}

func TestReadSourceReadsOneVersionsManifest(t *testing.T) {
	g := newFakeGitHub(t)
	g.release("someone/plugins", "1.0.3", false, map[string]string{"demo": "old"})
	m := g.manager(t)

	got, err := m.ReadSource(context.Background(), "someone/plugins", "1.0.3")
	if err != nil || got.Version != "1.0.3" {
		t.Fatalf("ReadSource = %+v, %v", got, err)
	}
	if p := g.paths(); p[0] != "/someone/plugins/releases/download/v1.0.3/nat-plugins.json" {
		t.Errorf("asked for %v", p)
	}

	// A release whose manifest names another version is not that version.
	g.manifest("someone/plugins", "1.0.2", false, Manifest{Version: "1.0.9"})
	if _, err := m.ReadSource(context.Background(), "someone/plugins", "1.0.2"); err == nil ||
		!strings.Contains(err.Error(), "the v1.0.2 release's manifest says it is 1.0.9") {
		t.Errorf("mismatched version = %v", err)
	}
}

func TestReadSourceFailuresConcludeNothing(t *testing.T) {
	g := newFakeGitHub(t)
	m := g.manager(t)
	ctx := context.Background()

	if _, err := m.ReadSource(ctx, "no/release", ""); err == nil || !strings.Contains(err.Error(), "404 Not Found") ||
		!strings.HasPrefix(err.Error(), "read plugin source no/release: GET ") {
		t.Errorf("no release = %v", err)
	}

	g.file("/bad/json/releases/latest/download/nat-plugins.json", "<html>secret page</html>")
	if _, err := m.ReadSource(ctx, "bad/json", ""); err == nil || err.Error() != "read plugin source bad/json: malformed manifest" {
		t.Errorf("malformed = %v", err)
	}

	good := Entry{Name: "demo", Asset: "nat-source-demo", SHA256: strings.Repeat("ab", 32)}
	for _, c := range []struct {
		man  Manifest
		want string
	}{
		{Manifest{Version: "v1.0"}, `version "v1.0" is not a dotted version`},
		{Manifest{Version: "1", Plugins: []Entry{{Name: "Bad"}}}, `"Bad" is not a plugin name`},
		{Manifest{Version: "1", Plugins: []Entry{good, good}}, "plugin demo is listed twice"},
		{Manifest{Version: "1", Plugins: []Entry{{Name: "demo", Asset: "../x", SHA256: good.SHA256}}}, `asset "../x" is not a file name`},
		{Manifest{Version: "1", Plugins: []Entry{{Name: "demo", Asset: "x", SHA256: "abc"}}}, `sha256 "abc" is not a SHA-256 digest`},
		{Manifest{Version: "1", Plugins: []Entry{{Name: "demo", Asset: "x", SHA256: "zz"}}}, `sha256 "zz" is not a SHA-256 digest`},
	} {
		g.manifest("in/valid", "1", true, c.man)
		_, err := m.ReadSource(ctx, "in/valid", "")
		if err == nil || !strings.Contains(err.Error(), "invalid manifest: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v = %v, want %q", c.man, err, c.want)
		}
	}

	bad := *m
	bad.BaseURL = "https://bad\x7f"
	if _, err := bad.ReadSource(ctx, "a/b", ""); err == nil {
		t.Error("an unbuildable request read")
	}

	gone := httptest.NewTLSServer(http.NotFoundHandler())
	gone.Close()
	bad.BaseURL = gone.URL
	if _, err := bad.ReadSource(ctx, "a/b", ""); err == nil {
		t.Error("a server that is not there read")
	}
}

func TestRedirectsAreFollowedOverHTTPSOnly(t *testing.T) {
	g := newFakeGitHub(t)
	g.release("a/b", "1.0", false, nil)
	m := g.manager(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/a/b/releases/latest/download/nat-plugins.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, g.srv.URL+"/a/b/releases/download/v1.0/nat-plugins.json", http.StatusFound)
	})
	mux.HandleFunc("/plain/b/releases/latest/download/nat-plugins.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/nat-plugins.json", http.StatusFound)
	})
	mux.HandleFunc("/loop/b/releases/latest/download/nat-plugins.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	})
	front := httptest.NewUnstartedServer(mux)
	front.TLS = g.srv.TLS
	front.StartTLS()
	defer front.Close()
	m.BaseURL = front.URL

	if man, err := m.ReadSource(context.Background(), "a/b", ""); err != nil || man.Version != "1.0" {
		t.Errorf("https redirect = %+v, %v", man, err)
	}
	if _, err := m.ReadSource(context.Background(), "plain/b", ""); err == nil ||
		!strings.Contains(err.Error(), "refused a redirect to http://example.invalid: plugins are fetched over https only") {
		t.Errorf("http redirect = %v", err)
	}
	if _, err := m.ReadSource(context.Background(), "loop/b", ""); err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Errorf("redirect loop = %v", err)
	}
}
