package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// callTimeout bounds one call to a plugin. A plugin is usually a round trip to
// someone else's API, so it gets a while — but a bounded while, since the
// board or the app is waiting on the answer. A var, not a const, so the tests
// can shorten it.
var callTimeout = 20 * time.Second

// eventTimeout bounds an event, shorter than any other call: an event is
// fire-and-forget, sent after nat's own write has already landed, and nothing
// nat does next depends on how the plugin took it.
var eventTimeout = 10 * time.Second

// Runner runs a command in a working directory and returns its standard
// output. It is the seam the tests replace: the real one starts a subprocess.
// It is internal/gh's seam re-declared, not shared — each wrapped binary keeps
// its own, so neither package reaches into the other.
type Runner interface {
	Run(dir, name string, args ...string) (string, error)
}

// StdinRunner is a Runner that can also carry input on the command's standard
// input, which is where every request to a plugin goes.
type StdinRunner interface {
	Runner
	RunWithStdin(dir string, stdin io.Reader, name string, args ...string) (string, error)
}

// ExecRunner is a StdinRunner backed by real subprocesses, each bounded by
// Timeout — callTimeout when it is left zero.
type ExecRunner struct {
	Timeout time.Duration
}

var (
	_ Runner      = ExecRunner{}
	_ StdinRunner = ExecRunner{}
)

// Run executes name with args in dir, returning its standard output. A
// non-zero exit becomes an [ExitError] carrying what the command wrote to
// stderr; anything else — a plugin that is not there, say — is returned as
// os/exec reported it.
func (r ExecRunner) Run(dir, name string, args ...string) (string, error) {
	return r.run(dir, nil, name, args...)
}

// RunWithStdin is [ExecRunner.Run] with the command's standard input wired to
// stdin.
func (r ExecRunner) RunWithStdin(dir string, stdin io.Reader, name string, args ...string) (string, error) {
	return r.run(dir, stdin, name, args...)
}

// run is Run and RunWithStdin's shared implementation, so the timeout, the
// working directory and the exit handling are written once.
func (r ExecRunner) run(dir string, stdin io.Reader, name string, args ...string) (string, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = callTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	// A plugin that is a script may leave a child holding its pipes after it
	// is killed; without a delay the wait would last as long as the child.
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	stdout := &capWriter{max: maxStdout}
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if stdout.over {
		// Checked first: a plugin cut off mid-write dies of the closed pipe, and
		// its exit says nothing about why.
		return "", fmt.Errorf("%s wrote more than %d bytes to stdout", name, maxStdout)
	}
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s timed out after %s", name, timeout)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), &ExitError{Code: exitErr.ExitCode(), Stderr: stderr.String()}
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// maxStdout is the most a plugin may write to stdout in one call: the
// protocol's 4 MiB. A var, not a const, so the tests can shrink it.
var maxStdout = 4 << 20

// errStdoutFull is what a [capWriter] answers once the cap is passed, which
// stops os/exec copying and closes the plugin's pipe.
var errStdoutFull = errors.New("stdout cap reached")

// capWriter buffers a plugin's stdout up to max bytes and refuses the write
// that would pass it, remembering that it did.
type capWriter struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		w.over = true
		return 0, errStdoutFull
	}
	return w.buf.Write(p)
}

func (w *capWriter) String() string { return w.buf.String() }

// ExitError is a plugin that ran and refused: its exit code, and whatever it
// wrote to stderr on the way out.
type ExitError struct {
	Code   int
	Stderr string
}

// Error is the plugin's own message when it wrote one — the first non-empty
// line of its stderr, the one worth a toast — and the exit code when it didn't.
func (e *ExitError) Error() string {
	if s := firstLine(e.Stderr); s != "" {
		return s
	}
	return fmt.Sprintf("plugin exited %d", e.Code)
}

// firstLine is the first non-empty line of stderr.
func firstLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return ""
}

// Exec is a task source reached through its binary: Name is the source's name
// (the `<name>` of `nat-source-<name>`), Path the binary itself.
type Exec struct {
	Name string
	Path string

	runner      StdinRunner
	eventRunner StdinRunner
}

var _ Client = (*Exec)(nil)

// New returns an Exec running the plugin at path for real, its events on the
// shorter event timeout.
func New(name, path string) *Exec {
	return &Exec{
		Name:        name,
		Path:        path,
		runner:      ExecRunner{Timeout: callTimeout},
		eventRunner: ExecRunner{Timeout: eventTimeout},
	}
}

// NewWithRunner returns an Exec that runs every call, events included,
// through r.
func NewWithRunner(name, path string, r StdinRunner) *Exec {
	return &Exec{Name: name, Path: path, runner: r, eventRunner: r}
}

// Describe asks the plugin what it is, and refuses one that speaks any
// protocol but this build's.
func (e *Exec) Describe(ctx context.Context, p Project) (Describe, error) {
	var d Describe
	req := struct {
		Project Project `json:"project"`
	}{p}
	if err := e.call(ctx, e.runner, "describe", p, req, &d); err != nil {
		return Describe{}, err
	}
	if d.Protocol != ProtocolVersion {
		return Describe{}, fmt.Errorf("source plugin %s speaks protocol %d; this nat speaks protocol %d", e.Name, d.Protocol, ProtocolVersion)
	}
	if err := ValidateDescribe(d); err != nil {
		return Describe{}, e.invalid("describe", p, err)
	}
	return d, nil
}

// invalid is the error for a response that decoded but breaks one of the
// protocol's rules: the rule is named, the response is not, and the log line
// carries only who answered what.
func (e *Exec) invalid(method string, p Project, err error) error {
	logging.Error("task source answered an invalid response", "plugin", e.Name, "method", method, "project", p.ID, "err", err)
	return fmt.Errorf("%s %s: invalid response: %w", e.binary(), method, err)
}

// Sidebar asks for the plugin's sidebar tree, with the lazy groups named in
// expand filled in.
func (e *Exec) Sidebar(ctx context.Context, p Project, expand []string) ([]Group, error) {
	if expand == nil {
		expand = []string{}
	}
	req := struct {
		Project Project  `json:"project"`
		Expand  []string `json:"expand"`
	}{p, expand}
	var resp struct {
		Groups []Group `json:"groups"`
	}
	if err := e.call(ctx, e.runner, "sidebar", p, req, &resp, "expand", expand); err != nil {
		return nil, err
	}
	if err := ValidateGroups(resp.Groups); err != nil {
		return nil, e.invalid("sidebar", p, err)
	}
	return resp.Groups, nil
}

// Container asks for one container's detail.
func (e *Exec) Container(ctx context.Context, p Project, id string) (ContainerDetail, error) {
	req := struct {
		Project Project `json:"project"`
		ID      string  `json:"id"`
	}{p, id}
	var d ContainerDetail
	if err := e.call(ctx, e.runner, "container", p, req, &d, "container", id); err != nil {
		return ContainerDetail{}, err
	}
	if err := ValidateContainer(d); err != nil {
		return ContainerDetail{}, e.invalid("container", p, err)
	}
	return d, nil
}

// Action runs one of the plugin's named actions against target, with the
// input it asked for.
func (e *Exec) Action(ctx context.Context, p Project, action string, target Target, input string) (ActionResult, error) {
	req := struct {
		Project Project `json:"project"`
		Action  string  `json:"action"`
		Target  Target  `json:"target"`
		Input   string  `json:"input,omitempty"`
	}{p, action, target, input}
	var r ActionResult
	if err := e.call(ctx, e.runner, "action", p, req, &r,
		"action", action, "group", target.Group, "container", target.Container); err != nil {
		return ActionResult{}, err
	}
	return r, nil
}

// Event tells the plugin what happened to a task. It is fire-and-forget: the
// plugin's stdout is not read, so one that prints nothing at all is as good as
// one that prints `{}`.
func (e *Exec) Event(ctx context.Context, p Project, container string, task Task, event string) error {
	req := struct {
		Project   Project `json:"project"`
		Container string  `json:"container"`
		Task      Task    `json:"task"`
		Event     string  `json:"event"`
	}{p, container, task, event}
	return e.call(ctx, e.eventRunner, "event", p, req, nil,
		"event", event, "container", container, "task", task.ID)
}

// Setup hands the plugin the value of one of its describe's setup fields — a
// token, most often — and returns what it says about it. Like describe it is
// about no project, so the envelope's are all "". The input travels on stdin
// alone: it is never in argv, never logged and never in an error, which
// carries only the plugin's own stderr line.
func (e *Exec) Setup(ctx context.Context, id, input string) (string, error) {
	req := struct {
		Project Project `json:"project"`
		ID      string  `json:"id"`
		Input   string  `json:"input"`
	}{Project{}, id, input}
	var r ActionResult
	if err := e.call(ctx, e.runner, "setup", Project{}, req, &r, "id", id); err != nil {
		return "", err
	}
	return r.Message, nil
}

// binary is the plugin as it is named in an error.
func (e *Exec) binary() string { return "nat-source-" + e.Name }

// call sends one request and decodes its answer into resp (nil: not read).
// ids are slog key/value pairs naming what the call was about, for the log
// line a failure leaves — and they are all a log line ever carries: the
// request and the response are never logged, nor put in an error.
func (e *Exec) call(ctx context.Context, r StdinRunner, method string, p Project, req, resp any, ids ...any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Cannot fail: every request is strings, slices and structs of them.
	body, _ := json.Marshal(req)
	out, err := r.RunWithStdin(p.WorkingDir, bytes.NewReader(body), e.Path, method)
	if err != nil {
		attrs := append([]any{"plugin", e.Name, "method", method, "project", p.ID}, ids...)
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			attrs = append(attrs, "code", exitErr.Code)
		}
		logging.Error("task source call failed", append(attrs, "err", err)...)
		return fmt.Errorf("%s %s: %w", e.binary(), method, err)
	}
	if resp == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(out), resp); err != nil {
		// The decoder's own message can quote the response, so it is dropped.
		logging.Error("task source answered malformed JSON", "plugin", e.Name, "method", method, "project", p.ID)
		return fmt.Errorf("%s %s: malformed response", e.binary(), method)
	}
	return nil
}
