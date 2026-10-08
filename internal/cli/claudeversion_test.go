package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// claudeVersionFixture points every seam claude-version has at fakes: claude
// answers installed (or fails with runErr), the feed answers feedBody with
// feedStatus and counts its hits, the cache lives in a temp dir, the clock is
// pinned. It returns the hit counter, the cache path and a way to move the
// clock.
type claudeVersionFixture struct {
	hits      *int
	cachePath string
	now       *time.Time
	ran       *[][]string
}

func newClaudeVersionFixture(t *testing.T, installed string, runErr error, feedStatus int, feedBody string) claudeVersionFixture {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "" {
			t.Errorf("feed read carried an Authorization header")
		}
		w.WriteHeader(feedStatus)
		_, _ = w.Write([]byte(feedBody))
	}))
	t.Cleanup(srv.Close)

	cachePath := filepath.Join(t.TempDir(), "state", "claude-version.json")
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	var ran [][]string

	oldURL, oldNow, oldRun, oldPath, oldClaude := claudeLatestURL, claudeVersionNow, claudeRun, claudeVersionCachePath, claudePath
	t.Cleanup(func() {
		claudeLatestURL, claudeVersionNow, claudeRun, claudeVersionCachePath, claudePath = oldURL, oldNow, oldRun, oldPath, oldClaude
	})
	claudeLatestURL = srv.URL
	claudeVersionNow = func() time.Time { return now }
	claudeRun = func(_ context.Context, name string, args ...string) (string, error) {
		ran = append(ran, append([]string{name}, args...))
		return installed, runErr
	}
	// A native install unless a test says Homebrew.
	claudePath = func() (string, error) { return "/Users/x/.local/share/claude/versions/2.1.294", nil }
	claudeVersionCachePath = func() (string, error) { return cachePath, nil }
	return claudeVersionFixture{hits: &hits, cachePath: cachePath, now: &now, ran: &ran}
}

func runClaudeVersion(t *testing.T) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"claude-version", "--json"}, Env{Out: &out}); err != nil {
		t.Fatalf("claude-version: %v", err)
	}
	return out.String()
}

// Both sides read: installed is claude --version's first token, latest the
// feed's tag without its v, and the JSON is exactly that shape.
func TestClaudeVersionBothSides(t *testing.T) {
	f := newClaudeVersionFixture(t, "2.1.294 (Claude Code)\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	got := runClaudeVersion(t)
	want := "{\n  \"installed\": \"2.1.294\",\n  \"latest\": \"2.1.295\",\n  \"update_available\": true\n}\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if len(*f.ran) != 1 || strings.Join((*f.ran)[0], " ") != "claude --version" {
		t.Errorf("claude ran with %v, want --version once", *f.ran)
	}
}

// The same version on both sides is no update.
func TestClaudeVersionUpToDate(t *testing.T) {
	newClaudeVersionFixture(t, "2.1.295 (Claude Code)\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	got := runClaudeVersion(t)
	if !strings.Contains(got, `"update_available": false`) {
		t.Errorf("output = %s, want no update available", got)
	}
}

// A claude that cannot be run leaves installed out, and with it any update.
func TestClaudeVersionInstalledUnreadable(t *testing.T) {
	newClaudeVersionFixture(t, "", errors.New("not found"), http.StatusOK, `{"tag_name":"v2.1.295"}`)
	got := runClaudeVersion(t)
	want := "{\n  \"latest\": \"2.1.295\",\n  \"update_available\": false\n}\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// A claude that answers nothing at all is unreadable too.
func TestClaudeVersionInstalledEmpty(t *testing.T) {
	newClaudeVersionFixture(t, "  \n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	if got := runClaudeVersion(t); strings.Contains(got, "installed") {
		t.Errorf("output = %s, want installed left out", got)
	}
}

// A feed that fails — an error status, a body that will not parse, one with
// no tag, or no server at all — leaves latest out, never fails the command,
// and caches nothing.
func TestClaudeVersionLatestUnreadable(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limited", http.StatusForbidden, `{"message":"API rate limit exceeded"}`},
		{"not json", http.StatusOK, `<html>`},
		{"no tag", http.StatusOK, `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newClaudeVersionFixture(t, "2.1.294\n", nil, tt.status, tt.body)
			got := runClaudeVersion(t)
			want := "{\n  \"installed\": \"2.1.294\",\n  \"update_available\": false\n}\n"
			if got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
			if _, err := os.Stat(f.cachePath); err == nil {
				t.Error("a failed read was cached")
			}
		})
	}
	t.Run("unreachable", func(t *testing.T) {
		newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{}`)
		claudeLatestURL = "http://127.0.0.1:0/"
		if got := runClaudeVersion(t); strings.Contains(got, "latest") {
			t.Errorf("output = %s, want latest left out", got)
		}
	})
	t.Run("bad url", func(t *testing.T) {
		newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{}`)
		claudeLatestURL = "://"
		if got := runClaudeVersion(t); strings.Contains(got, "latest") {
			t.Errorf("output = %s, want latest left out", got)
		}
	})
}

// The feed is read once an hour: a second read within it is the cache's, one
// after it reads the feed again and writes the cache back.
func TestClaudeVersionCache(t *testing.T) {
	f := newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	runClaudeVersion(t)
	if *f.hits != 1 {
		t.Fatalf("feed hits = %d, want 1", *f.hits)
	}
	*f.now = f.now.Add(59 * time.Minute)
	if got := runClaudeVersion(t); !strings.Contains(got, `"latest": "2.1.295"`) {
		t.Errorf("cached output = %s", got)
	}
	if *f.hits != 1 {
		t.Errorf("feed hits within the hour = %d, want 1", *f.hits)
	}
	*f.now = f.now.Add(2 * time.Minute)
	runClaudeVersion(t)
	if *f.hits != 2 {
		t.Errorf("feed hits after the hour = %d, want 2", *f.hits)
	}
	data, err := os.ReadFile(f.cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var cached claudeVersionCache
	if err := json.Unmarshal(data, &cached); err != nil || cached.Latest != "2.1.295" || !cached.ReadAt.Equal(*f.now) {
		t.Errorf("cache = %s (%v), want 2.1.295 read at %s", data, err, *f.now)
	}
}

// A cache that will not parse is read past to the feed.
func TestClaudeVersionCorruptCache(t *testing.T) {
	f := newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	if err := os.MkdirAll(filepath.Dir(f.cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.cachePath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	runClaudeVersion(t)
	if *f.hits != 1 {
		t.Errorf("feed hits = %d, want 1", *f.hits)
	}
}

// With no state dir to keep a cache in, the feed is read every time and the
// answer still given.
func TestClaudeVersionNoCacheDir(t *testing.T) {
	f := newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	claudeVersionCachePath = func() (string, error) { return "", errors.New("no home") }
	runClaudeVersion(t)
	runClaudeVersion(t)
	if *f.hits != 2 {
		t.Errorf("feed hits = %d, want 2", *f.hits)
	}
}

// A cache that cannot be written is logged; the answer stands.
func TestClaudeVersionCacheUnwritable(t *testing.T) {
	f := newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f.cachePath = filepath.Join(blocker, "claude-version.json")
	claudeVersionCachePath = func() (string, error) { return f.cachePath, nil }
	if got := runClaudeVersion(t); !strings.Contains(got, `"latest": "2.1.295"`) {
		t.Errorf("output = %s", got)
	}
}

// The plain-text answer says each side, unknown where unread.
func TestClaudeVersionText(t *testing.T) {
	newClaudeVersionFixture(t, "", errors.New("not found"), http.StatusOK, `{"tag_name":"v2.1.295"}`)
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"claude-version"}, Env{Out: &out}); err != nil {
		t.Fatalf("claude-version: %v", err)
	}
	want := "installed: unknown\nlatest: 2.1.295\nupdate available: false\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestClaudeVersionRefusals(t *testing.T) {
	newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{}`)
	for _, args := range [][]string{
		{"claude-version", "extra"},
		{"claude-version", "--project", "x"},
		{"claude-update", "extra"},
		{"claude-update", "--bogus"},
	} {
		if err := Run(context.Background(), args, Env{Out: &bytes.Buffer{}}); err == nil {
			t.Errorf("%v: want a refusal", args)
		}
	}
}

// The default seams: the cache in nat's state directory, and claude run off
// PATH with its combined output.
func TestClaudeVersionDefaults(t *testing.T) {
	prev := stateDir
	t.Cleanup(func() { stateDir = prev })
	stateDir = func() (string, error) { return "/state", nil }
	if got, err := claudeVersionCachePath(); err != nil || got != "/state/claude-version.json" {
		t.Errorf("claudeVersionCachePath() = %q, %v", got, err)
	}
	stateDir = func() (string, error) { return "", errors.New("no home") }
	if _, err := claudeVersionCachePath(); err == nil {
		t.Error("an unresolvable state directory resolved")
	}

	bin := t.TempDir()
	script := "#!/bin/sh\necho \"2.1.294 (Claude Code) $1\"\necho oops >&2\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	out, err := claudeRun(context.Background(), "claude", "--version")
	if err != nil || out != "2.1.294 (Claude Code) --version\noops\n" {
		t.Errorf("claudeRun = %q, %v", out, err)
	}

	// claude's real path, through a symlink — and none with no claude.
	link := filepath.Join(t.TempDir(), "claude")
	if err := os.Symlink(filepath.Join(bin, "claude"), link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(link))
	want, _ := filepath.EvalSymlinks(filepath.Join(bin, "claude"))
	if got, err := claudePath(); err != nil || got != want {
		t.Errorf("claudePath() = %q, %v, want %q", got, err, want)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := claudePath(); err == nil {
		t.Error("claudePath() found a claude on an empty PATH")
	}
}

func TestHomebrewCask(t *testing.T) {
	for path, want := range map[string]string{
		"/opt/homebrew/Caskroom/claude-code@latest/2.1.294/claude": "claude-code@latest",
		"/opt/homebrew/Caskroom/claude-code/2.1.290/claude":        "claude-code",
		"/Users/x/.local/share/claude/versions/2.1.294":            "",
	} {
		if got := homebrewCask(path); got != want {
			t.Errorf("homebrewCask(%q) = %q, want %q", path, got, want)
		}
	}
}

// A Homebrew install is upgraded through brew, its cask named — `claude
// update` there installs nothing.
func TestClaudeUpdateHomebrew(t *testing.T) {
	f := newClaudeVersionFixture(t, "==> Upgrading claude-code@latest\n", nil, http.StatusOK, `{}`)
	claudePath = func() (string, error) { return "/opt/homebrew/Caskroom/claude-code@latest/2.1.294/claude", nil }
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"claude-update"}, Env{Out: &out}); err != nil {
		t.Fatalf("claude-update: %v", err)
	}
	if len(*f.ran) != 1 || strings.Join((*f.ran)[0], " ") != "brew upgrade claude-code@latest" {
		t.Errorf("ran %v, want brew upgrade claude-code@latest", *f.ran)
	}
	if out.String() != "==> Upgrading claude-code@latest\n" {
		t.Errorf("output = %q", out.String())
	}
}

// A claude that cannot be found is updated as `claude update`, whose own
// failure then says so.
func TestClaudeUpdateNoClaudePath(t *testing.T) {
	f := newClaudeVersionFixture(t, "", errors.New("not found"), http.StatusOK, `{}`)
	claudePath = func() (string, error) { return "", errors.New("not found") }
	err := Run(context.Background(), []string{"claude-update"}, Env{Out: &bytes.Buffer{}})
	if err == nil || !strings.HasPrefix(err.Error(), "claude update: ") {
		t.Errorf("err = %v, want claude update's failure", err)
	}
	if strings.Join((*f.ran)[0], " ") != "claude update" {
		t.Errorf("ran %v", *f.ran)
	}
}

// A cache path that is a directory is read past and cannot be written: the
// answer stands either way.
func TestClaudeVersionCacheIsADirectory(t *testing.T) {
	f := newClaudeVersionFixture(t, "2.1.294\n", nil, http.StatusOK, `{"tag_name":"v2.1.295"}`)
	if err := os.MkdirAll(f.cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := runClaudeVersion(t); !strings.Contains(got, `"latest": "2.1.295"`) {
		t.Errorf("output = %s", got)
	}
}

func TestNewerVersion(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"2.1.295", "2.1.294", true},
		{"2.1.294", "2.1.295", false},
		{"2.1.294", "2.1.294", false},
		{"2.10.0", "2.9.9", true},
		{"2.1.1", "2.1", true},
		{"2.1", "2.1.1", false},
		{"2.1.0-beta", "2.1.0", false},
	} {
		if got := newerVersion(tt.a, tt.b); got != tt.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// claude-update runs claude update and relays what it said.
func TestClaudeUpdate(t *testing.T) {
	f := newClaudeVersionFixture(t, "Successfully updated from 2.1.294 to version 2.1.295\n", nil, http.StatusOK, `{}`)
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"claude-update", "--json"}, Env{Out: &out}); err != nil {
		t.Fatalf("claude-update: %v", err)
	}
	want := "{\n  \"output\": \"Successfully updated from 2.1.294 to version 2.1.295\\n\"\n}\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	if len(*f.ran) != 1 || strings.Join((*f.ran)[0], " ") != "claude update" {
		t.Errorf("claude ran with %v, want update once", *f.ran)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"claude-update"}, Env{Out: &out}); err != nil {
		t.Fatalf("claude-update: %v", err)
	}
	if out.String() != "Successfully updated from 2.1.294 to version 2.1.295\n" {
		t.Errorf("text output = %q", out.String())
	}
}

// A failed update is the command's error, carrying what claude said.
func TestClaudeUpdateFailure(t *testing.T) {
	newClaudeVersionFixture(t, "Error: permission denied\n", errors.New("exit status 1"), http.StatusOK, `{}`)
	err := Run(context.Background(), []string{"claude-update", "--json"}, Env{Out: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want claude's own words", err)
	}
}
