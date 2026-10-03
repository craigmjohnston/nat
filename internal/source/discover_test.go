package source

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// install writes a file at path with mode, making its directory.
func install(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

// withPath points Discover's PATH at dirs for one test.
func withPath(t *testing.T, path string) {
	t.Helper()
	old := pathEnv
	pathEnv = func() string { return path }
	t.Cleanup(func() { pathEnv = old })
}

func TestDiscoverSearchesThePluginsDirThenPath(t *testing.T) {
	cfg := t.TempDir()
	plugins := filepath.Join(cfg, "plugins")
	install(t, filepath.Join(plugins, "sc", "nat-source-sc"), 0o755)
	install(t, filepath.Join(plugins, "off", "nat-source-off"), 0o644)     // not executable
	install(t, filepath.Join(plugins, "wrong", "nat-source-other"), 0o755) // not its dir's name
	install(t, filepath.Join(plugins, "loose"), 0o755)                     // a file, not a dir

	bin := t.TempDir()
	install(t, filepath.Join(bin, "nat-source-x"), 0o755)
	install(t, filepath.Join(bin, "nat-source-sc"), 0o755)  // shadowed
	install(t, filepath.Join(bin, "nat-source-no"), 0o600)  // not executable
	install(t, filepath.Join(bin, "nat-source-"), 0o755)    // no name
	install(t, filepath.Join(bin, "something-else"), 0o755) // not a plugin
	if err := os.Mkdir(filepath.Join(bin, "nat-source-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(bin, "nat-source-x"), filepath.Join(bin, "nat-source-link")); err != nil {
		t.Fatal(err)
	}
	withPath(t, filepath.Join(cfg, "absent")+string(os.PathListSeparator)+bin)

	got, err := Discover(cfg)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	want := []Plugin{
		{Name: "link", Path: filepath.Join(bin, "nat-source-link")},
		{Name: "sc", Path: filepath.Join(plugins, "sc", "nat-source-sc")},
		{Name: "x", Path: filepath.Join(bin, "nat-source-x")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverWithNoPluginsDirIsNoPlugins(t *testing.T) {
	withPath(t, "")
	got, err := Discover(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Errorf("Discover() = %+v, %v, want nothing and no error", got, err)
	}
}

func TestDiscoverReportsAnUnreadablePluginsDir(t *testing.T) {
	cfg := t.TempDir()
	install(t, filepath.Join(cfg, "plugins"), 0o644) // a file where the dir goes
	withPath(t, "")
	if _, err := Discover(cfg); err == nil {
		t.Error("Discover() = nil, want the read error")
	}
	if _, _, err := Find(cfg, "sc"); err == nil {
		t.Error("Find() = nil, want the read error")
	}
}

func TestPathEnvIsTheProcessPath(t *testing.T) {
	t.Setenv("PATH", "/a:/b")
	if got := pathEnv(); got != "/a:/b" {
		t.Errorf("pathEnv() = %q, want $PATH", got)
	}
}

func TestFind(t *testing.T) {
	cfg := t.TempDir()
	install(t, filepath.Join(cfg, "plugins", "sc", "nat-source-sc"), 0o755)
	withPath(t, "")
	p, ok, err := Find(cfg, "sc")
	if err != nil || !ok || p != (Plugin{Name: "sc", Path: filepath.Join(cfg, "plugins", "sc", "nat-source-sc")}) {
		t.Errorf("Find(sc) = %+v, %v, %v, want the plugin", p, ok, err)
	}
	if p, ok, err := Find(cfg, "jira"); err != nil || ok || p != (Plugin{}) {
		t.Errorf("Find(jira) = %+v, %v, %v, want a miss", p, ok, err)
	}
}
