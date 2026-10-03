package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/fakeshortcut"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
)

// fakeTokens stands in for the Keychain; nothing in these tests touches the
// real one.
type fakeTokens struct {
	token    string
	err      error
	storeErr error
	stored   []string
	onStore  string // the token Store "saves"
	saved    [][2]string
	saveErr  error
	lost     bool // Save "succeeds" but the token never lands
	asked    []string
}

func (f *fakeTokens) Token() (string, error) { return f.token, f.err }

// Has is a stored token — each ask recorded, so a test can see describe
// asked rather than read.
func (f *fakeTokens) Has() bool {
	f.asked = append(f.asked, "has")
	return f.token != "" && f.err == nil
}

func (f *fakeTokens) Save(account, token string) error {
	f.saved = append(f.saved, [2]string{account, token})
	if f.saveErr != nil {
		return f.saveErr
	}
	if !f.lost {
		f.token, f.err = token, nil
	}
	return nil
}

func (f *fakeTokens) Store(account string) error {
	f.stored = append(f.stored, account)
	if f.storeErr != nil {
		return f.storeErr
	}
	f.token = f.onStore
	return nil
}

// harness is the plugin pointed at a seeded fake Shortcut, with its config
// and cache in a temp dir.
type harness struct {
	t      *testing.T
	fake   *fakeshortcut.Server
	vars   map[string]string
	tokens *fakeTokens
	now    time.Time
	dir    string
}

const project = `"project":{"id":"p1","name":"Work","working_dir":"/tmp"}`

func newHarness(t *testing.T) *harness {
	t.Helper()
	fake := fakeshortcut.Seed(secret)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	return &harness{
		t:    t,
		fake: fake,
		vars: map[string]string{
			"HOME":             filepath.Join(dir, "home"),
			"XDG_CONFIG_HOME":  filepath.Join(dir, "config"),
			"XDG_CACHE_HOME":   filepath.Join(dir, "cache"),
			"SHORTCUT_API_URL": srv.URL + fakeshortcut.Prefix,
			"USER":             "craig",
		},
		tokens: &fakeTokens{token: secret},
		now:    fakeshortcut.SeedNow,
		dir:    dir,
	}
}

func (h *harness) env() Env {
	return Env{
		Getenv: func(k string) string { return h.vars[k] },
		Now:    func() time.Time { return h.now },
		Tokens: h.tokens,
	}
}

// run runs the binary with args and stdin, returning exit code, stdout and
// stderr.
func (h *harness) run(stdin string, args ...string) (int, string, string) {
	var out, errb strings.Builder
	code := Run(append([]string{"nat-source-shortcut"}, args...), strings.NewReader(stdin), &out, &errb, h.env())
	return code, out.String(), errb.String()
}

// call runs a method with the project envelope plus extra (a JSON fragment,
// without braces), failing the test on a non-zero exit.
func (h *harness) call(method, extra string) string {
	h.t.Helper()
	code, out, errs := h.run(req(extra), method)
	if code != 0 {
		h.t.Fatalf("%s %s: exit %d, stderr %q", method, extra, code, errs)
	}
	return out
}

// fail runs a method expecting exit 1 and returns its stderr.
func (h *harness) fail(method, extra string) string {
	h.t.Helper()
	code, out, errs := h.run(req(extra), method)
	if code != 1 || out != "" {
		h.t.Fatalf("%s %s: exit %d, stdout %q; want a failure", method, extra, code, out)
	}
	return strings.TrimSuffix(errs, "\n")
}

func req(extra string) string {
	if extra == "" {
		return "{" + project + "}"
	}
	return "{" + project + "," + extra + "}"
}

// writes is the fake's non-GET requests since the last reset, one per line.
func (h *harness) writes() string {
	var lines []string
	for _, r := range h.fake.Writes() {
		lines = append(lines, r.String())
	}
	return strings.Join(lines, "\n")
}

// gets is the GET paths since the last reset, sorted (they run in parallel).
func (h *harness) gets() []string {
	var out []string
	for _, r := range h.fake.Requests() {
		if r.Method == "GET" {
			p := r.Path
			if q := r.Query.Get("query"); q != "" {
				p += " " + q
			}
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

func (h *harness) writeConfig(p settings.Project) {
	h.t.Helper()
	f := &settings.File{Projects: map[string]*settings.Project{"p1": &p}}
	if err := f.Save(settings.Dirs{Getenv: h.env().Getenv}.ConfigFile()); err != nil {
		h.t.Fatal(err)
	}
}

func TestUsage(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{nil, {"bogus"}, {"sidebar", "extra"}, {"config"}, {"config", "a", "b"}} {
		code, out, errs := h.run("{}", args...)
		if code != 2 || out != "" || !strings.Contains(errs, "usage: nat-source-shortcut") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out, errs)
		}
	}
}

func TestBadRequests(t *testing.T) {
	h := newHarness(t)
	for _, stdin := range []string{"", "not json", "[1]", `{"project":"x"}`} {
		code, _, errs := h.run(stdin, "sidebar")
		if code != 1 || errs != "shortcut: request is not one JSON object\n" {
			t.Errorf("%q: exit %d, stderr %q", stdin, code, errs)
		}
	}
	var out, errb strings.Builder
	code := Run([]string{"x", "sidebar"}, errReader{}, &out, &errb, h.env())
	if code != 1 || errb.String() != "shortcut: request is not one JSON object\n" {
		t.Errorf("unreadable stdin: exit %d, stderr %q", code, errb.String())
	}
	for _, m := range []string{"sidebar", "container", "action", "event"} {
		code, _, errs := h.run(`{"project":{"id":""}}`, m)
		if code != 1 || errs != "shortcut: request has no project\n" {
			t.Errorf("%s with no project: exit %d, stderr %q", m, code, errs)
		}
	}
	// Unknown request fields are ignored.
	if code, _, errs := h.run(`{"project":{"id":"p1"},"future":{"x":1}}`, "describe"); code != 0 {
		t.Errorf("unknown field refused: %q", errs)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestTokenMissing(t *testing.T) {
	h := newHarness(t)
	h.tokens.err = errors.New("not found")
	for _, m := range []string{"sidebar", "container", "action", "event"} {
		code, out, errs := h.run(req(`"id":"4821"`), m)
		if code != 1 || out != "" || errs != TokenMissing+"\n" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", m, code, out, errs)
		}
	}
	h.tokens.err = nil
	h.tokens.token = ""
	if errs := h.fail("sidebar", `"expand":[]`); errs != TokenMissing {
		t.Errorf("empty Keychain token: %q", errs)
	}
	if len(h.fake.Requests()) != 0 {
		t.Errorf("Shortcut was called with no token: %v", h.fake.Requests())
	}

	// SHORTCUT_API_TOKEN wins over the Keychain, even one that fails.
	h.tokens.err = errors.New("not found")
	h.vars["SHORTCUT_API_TOKEN"] = secret
	h.call("sidebar", `"expand":[]`)
}

func TestLogin(t *testing.T) {
	h := newHarness(t)
	h.tokens = &fakeTokens{err: nil, onStore: secret}
	code, out, errs := h.run("", "login")
	if code != 0 || errs != "" || !strings.Contains(out, `Logged in to Shortcut workspace "scratch" as Craig Scratch (@craig).`) {
		t.Errorf("login: exit %d, stdout %q, stderr %q", code, out, errs)
	}
	if !slices.Equal(h.tokens.stored, []string{"craig"}) {
		t.Errorf("stored under %v", h.tokens.stored)
	}
	if strings.Contains(out, secret) {
		t.Errorf("login printed the token: %q", out)
	}

	h.vars["USER"] = ""
	h.tokens = &fakeTokens{onStore: "wrong"}
	code, _, errs = h.run("", "login")
	if code != 1 || errs != "shortcut: token stored, but Shortcut refused it: shortcut: GET /member: 401 Unauthorized\n" {
		t.Errorf("refused token: exit %d, stderr %q", code, errs)
	}
	if !slices.Equal(h.tokens.stored, []string{"nat"}) {
		t.Errorf("no USER: stored under %v", h.tokens.stored)
	}

	h.tokens = &fakeTokens{storeErr: errors.New("exit status 1")}
	if code, _, errs := h.run("", "login"); code != 1 || !strings.Contains(errs, "couldn't store the token") {
		t.Errorf("store failed: exit %d, stderr %q", code, errs)
	}
	h.tokens = &fakeTokens{err: errors.New("gone")}
	if code, _, errs := h.run("", "login"); code != 1 || !strings.Contains(errs, "didn't reach the Keychain") {
		t.Errorf("read-back failed: exit %d, stderr %q", code, errs)
	}

	// The HTTP override reaches login's client too.
	h.tokens = &fakeTokens{onStore: secret}
	env := h.env()
	env.HTTP = &httpClientRefusing
	var out2, errb strings.Builder
	if code := Run([]string{"x", "login"}, strings.NewReader(""), &out2, &errb, env); code != 1 {
		t.Errorf("login with a refusing client: exit %d", code)
	}
}

// setupReq is a setup request as nat sends one: an empty project, the field
// and the value.
func setupReq(id, input string) string {
	return fmt.Sprintf(`{"project":{"id":"","name":"","working_dir":""},"id":%q,"input":%q}`, id, input)
}

func TestSetup(t *testing.T) {
	h := newHarness(t)
	h.tokens = &fakeTokens{err: errors.New("not found")}
	code, out, errs := h.run(setupReq("token", " "+secret+"\n"), "setup")
	if code != 0 || errs != "" || out != `{"message":"Logged in to scratch as Craig Scratch"}`+"\n" {
		t.Errorf("setup: exit %d, stdout %q, stderr %q", code, out, errs)
	}
	if want := [][2]string{{"craig", secret}}; !slices.Equal(h.tokens.saved, want) {
		t.Errorf("saved %v, want %v (trimmed, under USER)", h.tokens.saved, want)
	}
	if got := h.gets(); !slices.Equal(got, []string{"/member"}) {
		t.Errorf("setup read %v, want /member alone", got)
	}
	// Once set, the other methods find it.
	h.call("sidebar", `"expand":[]`)

	h.vars["USER"] = ""
	for _, c := range []struct {
		name   string
		tokens *fakeTokens
		req    string
		want   string
		saved  [][2]string
	}{
		{"Shortcut refuses the token", &fakeTokens{}, setupReq("token", "wrong"),
			"shortcut: token stored, but Shortcut refused it: shortcut: GET /member: 401 Unauthorized", [][2]string{{"nat", "wrong"}}},
		{"the Keychain write fails", &fakeTokens{saveErr: errors.New("exit status 50")}, setupReq("token", secret),
			"shortcut: couldn't store the token in the Keychain: exit status 50", [][2]string{{"nat", secret}}},
		{"the token never lands", &fakeTokens{lost: true, err: errors.New("gone")}, setupReq("token", secret),
			"shortcut: the token didn't reach the Keychain", [][2]string{{"nat", secret}}},
		{"an unknown id", &fakeTokens{}, setupReq("workspace", secret),
			`shortcut: no setup field "workspace" — the only one is token`, nil},
		{"an empty input", &fakeTokens{}, setupReq("token", " \n"), "shortcut: no token given", nil},
	} {
		h.tokens = c.tokens
		code, out, errs := h.run(c.req, "setup")
		if code != 1 || out != "" || errs != c.want+"\n" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", c.name, code, out, errs)
		}
		if strings.Contains(errs, secret) {
			t.Errorf("%s: the token is on stderr: %q", c.name, errs)
		}
		if !slices.Equal(h.tokens.saved, c.saved) {
			t.Errorf("%s: saved %v, want %v", c.name, h.tokens.saved, c.saved)
		}
	}
}

func TestConfigCommand(t *testing.T) {
	h := newHarness(t)
	code, out, _ := h.run("", "config", "p1")
	var p settings.Project
	if code != 0 || json.Unmarshal([]byte(out), &p) != nil || len(p.Segments) != 1 || p.Segments[0].Query != "owner:me" {
		t.Errorf("config default: exit %d, %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "config", "nat-source-shortcut", "config.json")); err == nil {
		t.Error("config wrote a file just by printing")
	}
	h.writeConfig(settings.Project{Team: "board", Segments: []settings.Segment{{ID: "b", Name: "Bugs", Query: "type:bug"}}})
	_, out, _ = h.run("", "config", "p1")
	if !strings.Contains(out, `"team": "board"`) || !strings.Contains(out, `"query": "type:bug"`) {
		t.Errorf("config = %s", out)
	}
	h.corruptConfig()
	if code, _, errs := h.run("", "config", "p1"); code != 1 || !strings.Contains(errs, "not valid JSON") {
		t.Errorf("corrupt config: exit %d, %q", code, errs)
	}
}

func (h *harness) corruptConfig() {
	path := settings.Dirs{Getenv: h.env().Getenv}.ConfigFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("a\nb"); got != "a" {
		t.Errorf("oneLine = %q", got)
	}
}

// secret is the fake's token: distinctive, so a test can tell it never
// reached stdout or stderr.
const secret = "t0k3n-s3cr3t"
