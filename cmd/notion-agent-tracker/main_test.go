package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/tui"
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

	if !strings.Contains(errOut.String(), "notion-agent-tracker: parse config") {
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
func stubProcess(t *testing.T, tokens config.TokenSource, in io.Reader, out, errOut io.Writer) {
	t.Helper()
	oldTokens, oldIn, oldOut, oldErr, oldExit := newTokens, stdin, stdout, stderr, exit
	t.Cleanup(func() {
		newTokens, stdin, stdout, stderr, exit = oldTokens, oldIn, oldOut, oldErr, oldExit
	})
	lastExit.code, lastExit.exited = 0, false
	newTokens = func() config.TokenSource { return tokens }
	stdin, stdout, stderr = in, out, errOut
	exit = func(code int) { lastExit.code, lastExit.exited = code, true }
}

// exitCode returns the code main exited with, and whether it exited at all.
func exitCode(t *testing.T) (int, bool) {
	t.Helper()
	return lastExit.code, lastExit.exited
}

func TestBuildAppStartsOnboardingWithoutAConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	app, err := buildApp(config.StaticToken(testToken))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(app.View().Content, "Looking for databases") {
		t.Errorf("view = %q, want the wizard's first step", app.View().Content)
	}
}

func TestBuildAppStartsOnTheBoard(t *testing.T) {
	writeConfig(t, `{"assignee_user_name":"Craig Johnston"}`)

	app, err := buildApp(config.StaticToken(testToken))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := app.View().Content; !strings.Contains(got, "Craig Johnston") {
		t.Errorf("view = %q, want the board for the loaded config", got)
	}
}

func TestBuildAppPassesTheTokenToTheClient(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := newClient
	t.Cleanup(func() { newClient = old })

	var got string
	newClient = func(token string) tui.NotionAPI {
		got = token
		return old(token)
	}

	if _, err := buildApp(config.StaticToken(testToken)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != testToken {
		t.Errorf("client built with %q, want %q", got, testToken)
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
