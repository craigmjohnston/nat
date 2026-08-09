package main

import (
	"errors"
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
