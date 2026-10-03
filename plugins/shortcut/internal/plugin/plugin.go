// Package plugin is nat-source-shortcut itself: the six protocol methods
// (describe, sidebar, container, action, event, setup) over the Shortcut API,
// and the two human subcommands (login, config) nat never calls.
//
// Run is the whole program; main only hands it the real process. Every
// method reads one JSON request from stdin and writes one JSON response to
// stdout, exit 0; a failure is exit 1 with one line on stderr, which nat
// shows the user — so that line is always worded for a person and never
// carries the token or a body Shortcut sent.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/cache"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/settings"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// TokenMissing is the line every method but describe and setup fails with
// when there is no token. nat shows it in gnat word for word — a source
// project's sidebar error, say — so it says how to fix it.
const TokenMissing = "Shortcut token missing — set it in gnat's Settings ▸ Sources or run nat-source-shortcut login"

// errTokenMissing carries TokenMissing as an error. It is capitalised
// because it is a sentence shown to a person as it stands, not wrapped.
var errTokenMissing = errors.New(TokenMissing) //nolint:staticcheck // ST1005: the line gnat shows, worded by the brief

// Budgets for one whole call. nat kills a call at 20 s (10 s for event); each
// HTTP request is separately capped at shortcut.Timeout, and these stop a
// chain of them running past nat's own limit.
var (
	callBudget  = 18 * time.Second
	eventBudget = 9 * time.Second
)

// cacheTTL is how long a sidebar or container response is served without
// asking Shortcut again.
const cacheTTL = 30 * time.Second

// Tokens is where the Shortcut token lives between runs — the Keychain, in
// production. Store asks for the token on the terminal itself (login); Save
// stores one handed in (setup), keeping it out of every argv.
type Tokens interface {
	Token() (string, error)
	Store(account string) error
	Save(account, token string) error
}

// Env is everything Run reads from the world besides stdin.
type Env struct {
	Getenv func(string) string
	Now    func() time.Time
	Tokens Tokens
	// HTTP, when set, replaces the client's own (tests shorten its timeout).
	HTTP *http.Client
}

const usage = "usage: nat-source-shortcut <describe|sidebar|container|action|event|setup> < request.json\n" +
	"       nat-source-shortcut login\n" +
	"       nat-source-shortcut config <nat project id>"

// request is every method's request in one shape: the envelope plus each
// method's own fields, the ones a method doesn't use left at zero. Unknown
// fields are ignored, as the protocol requires. It is declared here because
// nat's internal/source builds each request as an unexported struct inside
// the method that sends it; the field types are source's own.
type request struct {
	Project   source.Project `json:"project"`
	Expand    []string       `json:"expand"`    // sidebar
	ID        string         `json:"id"`        // container, setup
	Action    string         `json:"action"`    // action
	Target    source.Target  `json:"target"`    // action
	Input     string         `json:"input"`     // action, setup
	Container string         `json:"container"` // event
	Task      source.Task    `json:"task"`      // event
	Event     string         `json:"event"`     // event
}

// sidebarResponse is the sidebar method's response — in nat, likewise an
// unexported struct inside Exec.Sidebar.
type sidebarResponse struct {
	Groups []source.Group `json:"groups"`
}

// app is one method call's state.
type app struct {
	env   Env
	req   request
	sc    *shortcut.Client
	dirs  settings.Dirs
	cache cache.Cache
	now   time.Time
}

type method func(a *app, ctx context.Context) ([]byte, error)

var methods = map[string]method{
	"describe":  (*app).describe,
	"sidebar":   (*app).sidebar,
	"container": (*app).container,
	"action":    (*app).action,
	"event":     (*app).event,
	"setup":     (*app).setup,
}

// Run runs the program for args (args[0] is the binary) and returns its exit
// code: 0 success, 1 a failure (one line on stderr), 2 a usage error.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, env Env) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[1] {
	case "login":
		return login(stdout, stderr, env)
	case "config":
		if len(args) != 3 {
			_, _ = fmt.Fprintln(stderr, usage)
			return 2
		}
		return showConfig(args[2], stdout, stderr, env)
	}
	m, ok := methods[args[1]]
	if !ok || len(args) != 2 {
		_, _ = fmt.Fprintf(stderr, "shortcut: unknown method %q\n%s\n", strings.Join(args[1:], " "), usage)
		return 2
	}
	out, err := call(args[1], m, stdin, env)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, oneLine(err.Error()))
		return 1
	}
	_, _ = stdout.Write(append(out, '\n'))
	return 0
}

// call decodes the request, finds the token and runs m within its budget.
func call(name string, m method, stdin io.Reader, env Env) ([]byte, error) {
	var req request
	b, err := io.ReadAll(stdin)
	if err != nil || json.Unmarshal(b, &req) != nil {
		return nil, errors.New("shortcut: request is not one JSON object")
	}
	// describe and setup are sent an empty project — no project is in
	// question — and need no token: describe is how nat learns a token is
	// wanted at all, and setup is how one arrives. Every other method is about
	// a project, and needs one.
	tok := ""
	if name != "describe" && name != "setup" {
		if req.Project.ID == "" {
			return nil, errors.New("shortcut: request has no project")
		}
		if tok = env.Getenv("SHORTCUT_API_TOKEN"); tok == "" {
			if tok, err = env.Tokens.Token(); err != nil || tok == "" {
				return nil, errTokenMissing
			}
		}
	}
	budget := callBudget
	if name == "event" {
		budget = eventBudget
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return m(newApp(env, req, tok), ctx)
}

func newApp(env Env, req request, tok string) *app {
	dirs := settings.Dirs{Getenv: env.Getenv}
	return &app{
		env:   env,
		req:   req,
		sc:    newClient(env, tok),
		dirs:  dirs,
		cache: cache.Cache{Dir: dirs.CacheDir(), TTL: cacheTTL, Now: env.Now},
		now:   env.Now(),
	}
}

// newClient is a Shortcut client for tok on the configured API, over the
// HTTP override when there is one.
func newClient(env Env, tok string) *shortcut.Client {
	sc := shortcut.New(env.Getenv("SHORTCUT_API_URL"), tok)
	if env.HTTP != nil {
		sc.HTTP = env.HTTP
	}
	return sc
}

// account is the Keychain account the token is stored under: the user, else
// "nat".
func account(env Env) string {
	if a := env.Getenv("USER"); a != "" {
		return a
	}
	return "nat"
}

// oneLine is s cut at its first newline: nat shows only the first stderr
// line, so nothing worth reading may come after one.
func oneLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// settings loads the config file and returns it with this project's entry.
func (a *app) settings() (*settings.File, *settings.Project, error) {
	f, err := settings.Load(a.dirs.ConfigFile())
	if err != nil {
		return nil, nil, err
	}
	return f, f.Project(a.req.Project.ID), nil
}

// cached serves key from the cache while fresh; otherwise builds it, caching
// the result, and on a failed build serves the stale entry if there is one.
// Only with nothing cached at all does the failure reach nat.
func (a *app) cached(key string, build func() (any, error)) ([]byte, error) {
	pid := a.req.Project.ID
	stale, fresh, ok := a.cache.Get(pid, key)
	if fresh {
		return stale, nil
	}
	v, err := build()
	if err != nil {
		if ok {
			return stale, nil
		}
		return nil, err
	}
	b := marshal(v)
	a.cache.Put(pid, key, b)
	return b, nil
}

// login stores a token in the Keychain (security prompts for it itself) and
// checks it against Shortcut.
func login(stdout, stderr io.Writer, env Env) int {
	_, _ = fmt.Fprintln(stdout, "Paste a Shortcut API token (Shortcut ▸ Settings ▸ API Tokens) when asked; it goes straight into the Keychain.")
	if err := env.Tokens.Store(account(env)); err != nil {
		_, _ = fmt.Fprintf(stderr, "shortcut: couldn't store the token in the Keychain: %v\n", err)
		return 1
	}
	tok, err := env.Tokens.Token()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "shortcut: the token didn't reach the Keychain")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), callBudget)
	defer cancel()
	me, err := newClient(env, tok).Me(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "shortcut: token stored, but Shortcut refused it: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Logged in to Shortcut workspace %q as %s (@%s).\n", me.Workspace2.URLSlug, me.Name, me.MentionName)
	return 0
}

// tokenField is the one setup field: the API token, set from gnat's Settings
// ▸ Sources.
const tokenField = "token"

// setup stores the token gnat's Settings handed in — in the Keychain, through
// Tokens.Save, so it is in no argv — then checks it against Shortcut exactly
// as login does: read it back, ask who it belongs to. The token is never in
// the reply or an error line.
func (a *app) setup(ctx context.Context) ([]byte, error) {
	if a.req.ID != tokenField {
		return nil, fmt.Errorf("shortcut: no setup field %q — the only one is %s", a.req.ID, tokenField)
	}
	tok := strings.TrimSpace(a.req.Input)
	if tok == "" {
		return nil, errors.New("shortcut: no token given")
	}
	if err := a.env.Tokens.Save(account(a.env), tok); err != nil {
		return nil, fmt.Errorf("shortcut: couldn't store the token in the Keychain: %v", err)
	}
	stored, err := a.env.Tokens.Token()
	if err != nil || stored == "" {
		return nil, errors.New("shortcut: the token didn't reach the Keychain")
	}
	me, err := newClient(a.env, stored).Me(ctx)
	if err != nil {
		return nil, fmt.Errorf("shortcut: token stored, but Shortcut refused it: %v", err)
	}
	return marshal(source.ActionResult{Message: fmt.Sprintf("Logged in to %s as %s", me.Workspace2.URLSlug, me.Name)}), nil
}

// showConfig prints one project's settings as JSON, defaults filled in.
func showConfig(project string, stdout, stderr io.Writer, env Env) int {
	path := settings.Dirs{Getenv: env.Getenv}.ConfigFile()
	f, err := settings.Load(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	b, _ := json.MarshalIndent(f.Project(project), "", "  ")
	_, _ = fmt.Fprintf(stdout, "%s\n", b)
	return 0
}

// marshal is v as JSON. Every v is one of the protocol package's structs,
// which Marshal cannot fail on.
func marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
