package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/cli"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/tui"
)

// testToken is a plausible workspace token; nothing ever sends it anywhere.
const testToken = "ntn_o_test"

// writeConfig puts a config file in a temporary XDG config home for the test.
func writeConfig(t *testing.T, contents string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "notion-agent-tracker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// configuredHome sets up a config file so the app starts on the board rather
// than in the wizard.
func configuredHome(t *testing.T) config.TokenSource {
	t.Helper()
	writeConfig(t, `{"assignee_user_name":"Craig Johnston"}`)
	// The app these tests build drives the real tmux, and on the way out it
	// asks tmux about the pane it is drawing in. Running the suite from inside
	// tmux would otherwise make that a real call about a real pane.
	t.Setenv(agent.PaneEnv, "")
	return config.StaticToken(testToken)
}

// stubReleaser stands in for the app on the way out.
type stubReleaser struct {
	err      error
	released bool
}

func (s *stubReleaser) Release() error {
	s.released = true
	return s.err
}

func TestReleaseGivesTheAgentsBack(t *testing.T) {
	var errOut bytes.Buffer
	stderr = &errOut
	t.Cleanup(func() { stderr = os.Stderr })
	r := &stubReleaser{}

	release(r)

	if !r.released {
		t.Error("the joined agent panes should be handed back")
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", errOut.String())
	}
}

// The terminal has been given up by the time this runs, so stderr is the only
// place left to say that an agent could not be freed.
func TestReleaseReportsAFailureOnStderr(t *testing.T) {
	var errOut bytes.Buffer
	stderr = &errOut
	t.Cleanup(func() { stderr = os.Stderr })

	release(&stubReleaser{err: errors.New("no server")})

	if !strings.Contains(errOut.String(), "nat: no server") {
		t.Errorf("stderr = %q, want the failure reported", errOut.String())
	}
}

// failingTokens is a TokenSource that always fails with a given error.
type failingTokens struct{ err error }

func (f failingTokens) Token() (string, error) { return "", f.err }

func TestRunQuits(t *testing.T) {
	tokens := configuredHome(t)
	var out bytes.Buffer

	if err := run(tokens, strings.NewReader("q"), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The quit is read before the first frame is flushed, so only the terminal
	// setup lands in the buffer; what the board renders is asserted in the tui
	// package. What matters here is that run drove a program and returned.
	if out.Len() == 0 {
		t.Error("nothing was written to the terminal")
	}
}

func TestRunReportsAStartupFailure(t *testing.T) {
	writeConfig(t, "{not json")

	if err := run(config.StaticToken(testToken), strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("want the parse error")
	}
}

func TestMainQuits(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader("q"), &out, &errOut)

	main()

	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", errOut.String())
	}
	if code, exited := exitCode(t); exited {
		t.Errorf("exited with %d, want a clean return", code)
	}
}

func TestMainReportsAFailure(t *testing.T) {
	writeConfig(t, "{not json")
	var out, errOut bytes.Buffer
	stubProcess(t, config.StaticToken(testToken), strings.NewReader(""), &out, &errOut)

	main()

	if !strings.Contains(errOut.String(), "nat: parse config") {
		t.Errorf("stderr = %q, want the failure reported", errOut.String())
	}
	code, exited := exitCode(t)
	if !exited || code != 1 {
		t.Errorf("exit(%d, exited=%v), want exit(1)", code, exited)
	}
}

func TestNtnCLIIsTheDefaultTokenSource(t *testing.T) {
	// Building an NtnCLI does not run the binary, so this is safe to call; it
	// just pins what main runs with by default.
	if ntnCLI() == nil {
		t.Error("want a token source")
	}
}

// lastExit records what main asked the process to exit with.
var lastExit struct {
	code   int
	exited bool
}

// stubProcess points main at test doubles for the process's edges, restoring
// the real ones afterwards.
//
// It also pretends the test binary is already inside tmux, so that main's
// hosting step is a no-op: without it, a run on a machine with tmux installed
// would exec tmux over the test binary. The arguments are emptied for the same
// reason — the test binary's own flags would otherwise read as a subcommand.
func stubProcess(t *testing.T, tokens config.TokenSource, in io.Reader, out, errOut io.Writer) {
	t.Helper()
	t.Setenv(tmuxEnv, "/private/tmp/tmux-501/default,1,0")
	oldTokens, oldIn, oldOut, oldErr, oldExit, oldArgs := newTokens, stdin, stdout, stderr, exit, args
	t.Cleanup(func() {
		newTokens, stdin, stdout, stderr, exit, args = oldTokens, oldIn, oldOut, oldErr, oldExit, oldArgs
	})
	lastExit.code, lastExit.exited = 0, false
	args = func() []string { return nil }
	newTokens = func() config.TokenSource { return tokens }
	stdin, stdout, stderr = in, out, errOut
	exit = func(code int) { lastExit.code, lastExit.exited = code, true }
}

// exitCode returns the code main exited with, and whether it exited at all.
func exitCode(t *testing.T) (int, bool) {
	t.Helper()
	return lastExit.code, lastExit.exited
}

// execCall records the arguments host handed to exec.
type execCall struct {
	argv0 string
	argv  []string
	env   []string
}

// stubHost points host at test doubles for the process's edges: the PATH
// lookup, this binary's path, and the exec that would otherwise replace the
// test binary with tmux. It returns the exec calls made, which is empty when
// host decided to run in place.
func stubHost(t *testing.T, tmuxPath string, lookErr, selfErr, execErr error) *[]execCall {
	t.Helper()
	oldLook, oldSelf, oldExec := lookPath, executable, execProcess
	t.Cleanup(func() { lookPath, executable, execProcess = oldLook, oldSelf, oldExec })

	var calls []execCall
	lookPath = func(string) (string, error) { return tmuxPath, lookErr }
	executable = func() (string, error) { return "/usr/local/bin/nat", selfErr }
	execProcess = func(argv0 string, argv, env []string) error {
		calls = append(calls, execCall{argv0: argv0, argv: argv, env: env})
		return execErr
	}
	return &calls
}

// outsideTmux clears both the marker tmux sets in its panes and the opt-out, so
// host takes the re-exec path.
func outsideTmux(t *testing.T) {
	t.Helper()
	t.Setenv(tmuxEnv, "")
	t.Setenv(noTmuxEnv, "")
}

func TestHostRunsTheTUIInsideTmux(t *testing.T) {
	outsideTmux(t)
	calls := stubHost(t, "/opt/homebrew/bin/tmux", nil, nil, nil)

	if err := host(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(*calls))
	}
	got := (*calls)[0]
	if got.argv0 != "/opt/homebrew/bin/tmux" {
		t.Errorf("argv0 = %q, want the tmux found on PATH", got.argv0)
	}
	want := []string{"tmux", "new-session", "-A", "-s", "nat-tui", "/usr/local/bin/nat",
		";", "set-option", "-t", "nat-tui", "status", "off"}
	if !reflect.DeepEqual(got.argv, want) {
		t.Errorf("argv = %q, want %q", got.argv, want)
	}
	if len(got.env) == 0 {
		t.Error("want the environment passed through to tmux")
	}
}

// Inside a pane there is already a window to join agents into, and nesting a
// session in one is not what hosting means.
func TestHostRunsInPlaceInsideTmux(t *testing.T) {
	t.Setenv(tmuxEnv, "/private/tmp/tmux-501/default,1,0")
	calls := stubHost(t, "/opt/homebrew/bin/tmux", nil, nil, nil)

	if err := host(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("exec calls = %v, want none", *calls)
	}
}

func TestHostRunsInPlaceWhenOptedOut(t *testing.T) {
	t.Setenv(tmuxEnv, "")
	t.Setenv(noTmuxEnv, "1")
	calls := stubHost(t, "/opt/homebrew/bin/tmux", nil, nil, nil)

	if err := host(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("exec calls = %v, want none", *calls)
	}
}

func TestHostExplainsHowToInstallTmux(t *testing.T) {
	outsideTmux(t)
	stubHost(t, "", exec.ErrNotFound, nil, nil)

	err := host()
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("err = %v, want it to wrap ErrNotFound", err)
	}
	for _, want := range []string{"brew install tmux", noTmuxEnv + "=1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q", err, want)
		}
	}
}

func TestHostReportsAnUnfindableBinary(t *testing.T) {
	outsideTmux(t)
	sentinel := errors.New("no such file")
	calls := stubHost(t, "/opt/homebrew/bin/tmux", nil, sentinel, nil)

	err := host()
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to wrap %v", err, sentinel)
	}
	if len(*calls) != 0 {
		t.Errorf("exec calls = %v, want none", *calls)
	}
}

func TestHostReportsAFailedExec(t *testing.T) {
	outsideTmux(t)
	sentinel := errors.New("permission denied")
	stubHost(t, "/opt/homebrew/bin/tmux", nil, nil, sentinel)

	err := host()
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to wrap %v", err, sentinel)
	}
}

// A missing tmux has to be reported plainly on stderr, before Bubble Tea has
// taken the terminal over — and the app must not go on to start without it.
func TestMainReportsAMissingTmux(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader("q"), &out, &errOut)
	outsideTmux(t)
	stubHost(t, "", exec.ErrNotFound, nil, nil)

	main()

	if !strings.Contains(errOut.String(), "nat: tmux not found on PATH") {
		t.Errorf("stderr = %q, want the missing tmux reported", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("terminal output = %q, want the TUI never to have started", out.String())
	}
	code, exited := exitCode(t)
	if !exited || code != 1 {
		t.Errorf("exit(%d, exited=%v), want exit(1)", code, exited)
	}
}

func TestBuildAppStartsOnboardingWithoutAConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	app, err := buildApp(config.StaticToken(testToken))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(app.View().Content, "Which database holds your projects?") {
		t.Errorf("view = %q, want the wizard's first step", app.View().Content)
	}
}

func TestBuildAppStartsOnTheBoard(t *testing.T) {
	writeConfig(t, `{"assignee_user_name":"Craig Johnston"}`)

	app, err := buildApp(config.StaticToken(testToken))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The config names no active project, so the board draws its empty state —
	// which is still the board, not onboarding.
	if got := app.View().Content; !strings.Contains(got, "No project selected") {
		t.Errorf("view = %q, want the board's empty state", got)
	}
}

// The client must be built from the token *source*, not a token read once at
// startup, or a credential rotated mid-session would never be picked up.
func TestBuildAppPassesTheTokenSourceToTheClient(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := newClient
	t.Cleanup(func() { newClient = old })

	var captured notion.TokenFunc
	newClient = func(token notion.TokenFunc) tui.NotionAPI {
		captured = token
		return old(token)
	}

	if _, err := buildApp(config.StaticToken(testToken)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("no token source reached the client")
	}
	got, err := captured()
	if err != nil || got != testToken {
		t.Errorf("token source yielded %q, %v, want %q", got, err, testToken)
	}
}

func TestBuildAppReportsAnUnreadableConfig(t *testing.T) {
	writeConfig(t, "{not json")

	if _, err := buildApp(config.StaticToken(testToken)); err == nil {
		t.Fatal("want the parse error")
	}
}

func TestBuildAppExplainsHowToInstallTheCLI(t *testing.T) {
	writeConfig(t, `{}`)

	_, err := buildApp(failingTokens{err: config.ErrNtnNotInstalled})
	if !errors.Is(err, config.ErrNtnNotInstalled) {
		t.Fatalf("err = %v, want ErrNtnNotInstalled", err)
	}
	if !strings.Contains(err.Error(), "ntn.dev") {
		t.Errorf("err = %q, want it to say how to install ntn", err)
	}
}

func TestBuildAppExplainsHowToLogIn(t *testing.T) {
	writeConfig(t, `{}`)

	_, err := buildApp(failingTokens{err: config.ErrNtnNotLoggedIn})
	if !errors.Is(err, config.ErrNtnNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNtnNotLoggedIn", err)
	}
	if !strings.Contains(err.Error(), "ntn login") {
		t.Errorf("err = %q, want it to say to run `ntn login`", err)
	}
}

func TestBuildAppPassesThroughAnUnrecognisedTokenFailure(t *testing.T) {
	writeConfig(t, `{}`)
	sentinel := errors.New("keychain locked")

	_, err := buildApp(failingTokens{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to wrap %v", err, sentinel)
	}
	if strings.Contains(err.Error(), "ntn login") {
		t.Errorf("err = %q, want no misleading login hint", err)
	}
}

// Guard against the hint text drifting away from the constant main builds it
// from, which is the only thing tying the message to the real binary name.
func TestAuthHintNamesTheBinary(t *testing.T) {
	err := authHint(config.ErrNtnNotLoggedIn)
	if want := fmt.Sprintf("%s login", config.NtnBinary); !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to contain %q", err, want)
	}
}

// stubCommand points main's headless path at a fake Notion client and the
// given arguments, on top of the process doubles stubProcess installs.
func stubCommand(t *testing.T, commandArgs []string, api cli.API) {
	t.Helper()
	oldClient := newCLIClient
	t.Cleanup(func() { newCLIClient = oldClient })
	newCLIClient = func(notion.TokenFunc) cli.API { return api }
	args = func() []string { return commandArgs }
}

// stubAPI is a Notion client that answers every call with nothing, which is
// enough for a command to run end to end.
type stubAPI struct{ err error }

func (s stubAPI) QueryDataSource(context.Context, string, map[string]any, []notion.Sort) ([]notion.Page, error) {
	return nil, s.err
}

func (s stubAPI) GetBlockChildren(context.Context, string) ([]notion.Block, error) {
	return nil, s.err
}

// A subcommand runs headless and exits: no board, and no tmux, which would send
// the output somewhere nobody is looking.
func TestMainRunsACommandInsteadOfTheBoard(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader(""), &out, &errOut)
	outsideTmux(t)
	calls := stubHost(t, "/opt/homebrew/bin/tmux", nil, nil, nil)
	stubCommand(t, []string{"help"}, stubAPI{})

	main()

	if out.String() != cli.Usage {
		t.Errorf("stdout = %q, want the usage text", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", errOut.String())
	}
	if code, exited := exitCode(t); exited {
		t.Errorf("exited with %d, want a clean return", code)
	}
	if len(*calls) != 0 {
		t.Errorf("exec calls = %+v, want none: a command does not go through tmux", *calls)
	}
}

func TestMainReportsAFailedCommand(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader(""), &out, &errOut)
	stubCommand(t, []string{"bogus"}, stubAPI{})

	main()

	if !strings.Contains(errOut.String(), `nat: unknown command "bogus"`) {
		t.Errorf("stderr = %q, want the misuse reported", errOut.String())
	}
	code, exited := exitCode(t)
	if !exited || code != 1 {
		t.Errorf("exit(%d, exited=%v), want exit(1)", code, exited)
	}
}

// A command hits the same unusable credential the board does, and deserves the
// same way out of it.
func TestMainExplainsHowToLogInFromACommand(t *testing.T) {
	writeConfig(t, `{"active_project_id":"p1","projects":{"p1":{"name":"nat"}}}`)
	var out, errOut bytes.Buffer
	stubProcess(t, config.StaticToken(""), strings.NewReader(""), &out, &errOut)
	stubCommand(t, []string{"info"}, stubAPI{err: config.ErrNtnNotLoggedIn})

	main()

	if !strings.Contains(errOut.String(), fmt.Sprintf("%s login", config.NtnBinary)) {
		t.Errorf("stderr = %q, want the login hint", errOut.String())
	}
	code, exited := exitCode(t)
	if !exited || code != 1 {
		t.Errorf("exit(%d, exited=%v), want exit(1)", code, exited)
	}
}

// DefaultNewClient is what the real binary builds its headless client with.
func TestCommandsUseTheRealNotionClient(t *testing.T) {
	client := newCLIClient(func() (string, error) { return testToken, nil })
	if _, ok := client.(*notion.Client); !ok {
		t.Errorf("client is %T, want *notion.Client", client)
	}
}
