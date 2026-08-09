package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
)

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

// configuredHome sets up a config file and a stored API key, so the app starts
// on the board rather than in the wizard.
func configuredHome(t *testing.T) *config.MemorySecrets {
	t.Helper()
	writeConfig(t, `{"assignee_user_name":"Craig Johnston"}`)
	secrets := &config.MemorySecrets{}
	if err := secrets.SetAPIKey("ntn_secret"); err != nil {
		t.Fatal(err)
	}
	return secrets
}

func TestRunQuits(t *testing.T) {
	secrets := configuredHome(t)
	var out bytes.Buffer

	if err := run(secrets, strings.NewReader("q"), &out); err != nil {
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

	if err := run(&config.MemorySecrets{}, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("want the parse error")
	}
}

func TestMainQuits(t *testing.T) {
	secrets := configuredHome(t)
	var out, errOut bytes.Buffer
	stubProcess(t, secrets, strings.NewReader("q"), &out, &errOut)

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
	stubProcess(t, &config.MemorySecrets{}, strings.NewReader(""), &out, &errOut)

	main()

	if !strings.Contains(errOut.String(), "notion-agent-tracker: parse config") {
		t.Errorf("stderr = %q, want the failure reported", errOut.String())
	}
	code, exited := exitCode(t)
	if !exited || code != 1 {
		t.Errorf("exit(%d, exited=%v), want exit(1)", code, exited)
	}
}

func TestKeychainIsTheDefaultSecretsStore(t *testing.T) {
	// Building a Keyring does not touch the OS keychain, so this is safe to
	// call; it just pins what main runs with by default.
	if keychain() == nil {
		t.Error("want a secrets store")
	}
}

// lastExit records what main asked the process to exit with.
var lastExit struct {
	code   int
	exited bool
}

// stubProcess points main at test doubles for the process's edges, restoring
// the real ones afterwards.
func stubProcess(t *testing.T, secrets config.Secrets, in io.Reader, out, errOut io.Writer) {
	t.Helper()
	oldSecrets, oldIn, oldOut, oldErr, oldExit := newSecrets, stdin, stdout, stderr, exit
	t.Cleanup(func() {
		newSecrets, stdin, stdout, stderr, exit = oldSecrets, oldIn, oldOut, oldErr, oldExit
	})
	lastExit.code, lastExit.exited = 0, false
	newSecrets = func() config.Secrets { return secrets }
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

	app, err := buildApp(&config.MemorySecrets{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(app.View().Content, "enter") {
		t.Errorf("view = %q, want the wizard's form", app.View().Content)
	}
}

func TestBuildAppStartsOnboardingWithoutAStoredKey(t *testing.T) {
	writeConfig(t, `{"assignee_user_name":"Craig Johnston"}`)

	app, err := buildApp(&config.MemorySecrets{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(app.View().Content, "The board is not built yet") {
		t.Error("want the wizard, not the board, when the API key is missing")
	}
}

func TestBuildAppStartsOnTheBoard(t *testing.T) {
	writeConfig(t, `{"assignee_user_name":"Craig Johnston"}`)
	secrets := &config.MemorySecrets{}
	if err := secrets.SetAPIKey("ntn_secret"); err != nil {
		t.Fatal(err)
	}

	app, err := buildApp(secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := app.View().Content; !strings.Contains(got, "Craig Johnston") {
		t.Errorf("view = %q, want the board for the loaded config", got)
	}
}

func TestBuildAppReportsAnUnreadableConfig(t *testing.T) {
	writeConfig(t, "{not json")

	if _, err := buildApp(&config.MemorySecrets{}); err == nil {
		t.Fatal("want the parse error")
	}
}

func TestBuildAppReportsAKeyringFailure(t *testing.T) {
	writeConfig(t, `{}`)

	_, err := buildApp(brokenSecrets{})
	if err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Fatalf("err = %v, want the keyring failure", err)
	}
}

// brokenSecrets is a config.Secrets whose reads fail for a reason other than
// the key being absent.
type brokenSecrets struct{}

func (brokenSecrets) GetAPIKey() (string, error) { return "", errors.New("keychain locked") }
func (brokenSecrets) SetAPIKey(string) error     { return nil }
func (brokenSecrets) DeleteAPIKey() error        { return nil }
