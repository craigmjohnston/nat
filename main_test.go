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
	"github.com/craigmjohnston/nat/internal/logging"
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
	// The app these tests build drives the real tmux, and reads the pane it is
	// drawing in from the environment. Running the suite from inside tmux would
	// otherwise make that a real call about a real pane.
	t.Setenv(agent.PaneEnv, "")
	return config.StaticToken(testToken)
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

// The command line the binary really routes on, pinned so the stubbing the
// rest of these tests do cannot hide it.
func TestProcessArgsDropsTheBinaryName(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"nat", "info", "--json"}

	if got := processArgs(); !reflect.DeepEqual(got, []string{"info", "--json"}) {
		t.Errorf("processArgs() = %q, want the arguments after the binary name", got)
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
// the real ones afterwards. The arguments are emptied because the test binary's
// own flags would otherwise read as a subcommand, and the tmux check is stubbed
// into finding one, so the suite passes on a machine without it.
//
// The home directory is a temporary one because main opens a log file under it,
// and a test suite has no business writing to the log the real binary keeps.
func stubProcess(t *testing.T, tokens config.TokenSource, in io.Reader, out, errOut io.Writer) {
	t.Helper()
	stubLookPath(t, "/opt/homebrew/bin/tmux", nil)
	tempLogHome(t)
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

// tempLogHome points the log directory at somewhere of the test's own,
// whichever platform's convention resolves it, and returns the home it used.
func tempLogHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	return home
}

// logContents returns everything main's run wrote to the log file.
func logContents(t *testing.T) string {
	t.Helper()
	path, err := logging.Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A startup failure has to survive the terminal it was reported on: a nat
// started in something that closes with the process takes the message with it.
func TestMainLogsAStartupFailureAndSaysWhereTheLogIs(t *testing.T) {
	writeConfig(t, "{not json")
	var out, errOut bytes.Buffer
	stubProcess(t, config.StaticToken(testToken), strings.NewReader(""), &out, &errOut)

	main()

	path, err := logging.Path()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "log: "+path) {
		t.Errorf("stderr = %q, want it to say where the log is", errOut.String())
	}
	if got := logContents(t); !strings.Contains(got, "parse config") {
		t.Errorf("log = %q, want the failure in it", got)
	}
}

// The ordinary run leaves a trace too, so a log opened after a crash starts
// with the run that crashed rather than with nothing at all.
func TestMainLogsThatItStarted(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader("q"), &out, &errOut)

	main()

	if got := logContents(t); !strings.Contains(got, "nat starting") {
		t.Errorf("log = %q, want the start of the run in it", got)
	}
}

// A log that cannot be opened is said once and then left alone: it is not a
// reason to refuse to run.
func TestMainRunsWithoutALogItCannotOpen(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader("q"), &out, &errOut)
	blockLogDir(t)

	main()

	if !strings.Contains(errOut.String(), "nat: could not open the log file:") {
		t.Errorf("stderr = %q, want the unopenable log reported", errOut.String())
	}
	if code, exited := exitCode(t); exited {
		t.Errorf("exited with %d, want the app to have run anyway", code)
	}
}

// blockLogDir puts a plain file where the log directory's parent has to be, so
// the log cannot be created under it.
func blockLogDir(t *testing.T) {
	t.Helper()
	dir, err := logging.Dir()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(filepath.Dir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Without a log there is no location worth printing, and the failure is still
// reported the ordinary way.
func TestFailSaysNothingAboutALogThereIsNot(t *testing.T) {
	var errOut bytes.Buffer
	stderr = &errOut
	oldExit := exit
	t.Cleanup(func() { stderr, exit = os.Stderr, oldExit })
	exit = func(int) {}

	fail("", errors.New("no config"))

	if got := errOut.String(); got != "nat: no config\n" {
		t.Errorf("stderr = %q, want just the failure", got)
	}
}

// exitCode returns the code main exited with, and whether it exited at all.
func exitCode(t *testing.T) (int, bool) {
	t.Helper()
	return lastExit.code, lastExit.exited
}

// stubLookPath points the tmux check at a test double for the PATH lookup, so
// the suite neither depends on a tmux being installed nor on one not being.
func stubLookPath(t *testing.T, tmuxPath string, lookErr error) {
	t.Helper()
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(string) (string, error) { return tmuxPath, lookErr }
}

// nat runs in the terminal it was started in: the check is that the binary the
// agents need is there, and nothing is launched to host the board.
func TestRequireTmuxAcceptsATmuxOnPath(t *testing.T) {
	stubLookPath(t, "/opt/homebrew/bin/tmux", nil)

	if err := requireTmux(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireTmuxExplainsHowToInstallTmux(t *testing.T) {
	stubLookPath(t, "", exec.ErrNotFound)

	err := requireTmux()
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("err = %v, want it to wrap ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "brew install tmux") {
		t.Errorf("err = %q, want it to say how to install tmux", err)
	}
}

// A missing tmux has to be reported plainly on stderr, before Bubble Tea has
// taken the terminal over — and the app must not go on to start without it: an
// agent it could not launch is the whole point of the board.
func TestMainReportsAMissingTmux(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader("q"), &out, &errOut)
	stubLookPath(t, "", exec.ErrNotFound)

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

func (s stubAPI) DataSourceOrder(context.Context, string) ([]string, error) {
	return nil, s.err
}

func (s stubAPI) GetDataSource(context.Context, string) (*notion.DataSource, error) {
	return &notion.DataSource{}, s.err
}

func (s stubAPI) UpdateDataSourceProperties(context.Context, string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
	return &notion.DataSource{}, s.err
}

func (s stubAPI) CreatePage(context.Context, notion.Parent, map[string]notion.PropertyValue, []map[string]any) (*notion.Page, error) {
	return &notion.Page{}, s.err
}

func (s stubAPI) GetPage(context.Context, string) (*notion.Page, error) {
	return &notion.Page{}, s.err
}

func (s stubAPI) GetBlockChildren(context.Context, string) ([]notion.Block, error) {
	return nil, s.err
}

func (s stubAPI) AppendBlockChildren(context.Context, string, []map[string]any) ([]notion.Block, error) {
	return nil, s.err
}

func (s stubAPI) AppendBlockChildrenAfter(context.Context, string, string, []map[string]any) ([]notion.Block, error) {
	return nil, s.err
}

func (s stubAPI) DeleteBlock(context.Context, string) error { return s.err }

func (s stubAPI) UpdatePageProperties(context.Context, string, map[string]notion.PropertyValue) (*notion.Page, error) {
	return &notion.Page{}, s.err
}

func (s stubAPI) CreateProject(context.Context, string, string, bool) (*notion.ProjectStructure, error) {
	return &notion.ProjectStructure{}, s.err
}

// A subcommand runs headless and exits: no board, and no tmux check either —
// none of them launches an agent, so a machine without tmux runs them all.
func TestMainRunsACommandInsteadOfTheBoard(t *testing.T) {
	tokens := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, tokens, strings.NewReader(""), &out, &errOut)
	stubLookPath(t, "", exec.ErrNotFound)
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
	stubCommand(t, []string{"info", "--project", "p1"}, stubAPI{err: config.ErrNtnNotLoggedIn})

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
