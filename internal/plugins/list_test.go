package plugins

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListSaysEverything(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0.1", false, map[string]string{"demo": "old", "pinned": "p"})
	g.release(DefaultSource, "1.0.2", true, map[string]string{"demo": "new", "pinned": "p2"})
	g.release("someone/plugins", "3.0", true, map[string]string{"extra": "e"})
	m := g.manager(t)
	ctx := context.Background()

	// demo at an older release, so an update shows; pinned at a version that
	// will not parse, so none does; a hand-placed one; one on PATH; and one
	// managed install from a source no longer read.
	if _, err := m.Install(ctx, []string{DefaultSource}, "demo", "", "1.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, []string{DefaultSource}, "pinned", "", ""); err != nil {
		t.Fatal(err)
	}
	pinned := filepath.Join(m.pluginDir("pinned"), "installed.json")
	if err := os.WriteFile(pinned, []byte(`{"source":"craigmjohnston/nat","version":"dev"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, []string{"someone/plugins"}, "extra", "", ""); err != nil {
		t.Fatal(err)
	}
	writeExec(t, filepath.Join(m.pluginDir("hand"), "nat-source-hand"))
	bin := t.TempDir()
	writeExec(t, filepath.Join(bin, "nat-source-onpath"))
	t.Setenv("PATH", bin)

	got, err := m.List(ctx, []string{DefaultSource, "dead/source"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || got.Sources[0] != (SourceStatus{Repo: DefaultSource, Version: "1.0.2", Default: true}) ||
		got.Sources[1].Repo != "dead/source" || got.Sources[1].Error == "" || got.Sources[1].Version != "" {
		t.Errorf("sources = %+v", got.Sources)
	}
	wantInstalled := []Installed{
		{Name: "demo", Path: filepath.Join(m.pluginDir("demo"), "nat-source-demo"), Kind: KindManaged, Source: DefaultSource, Version: "1.0.1", Update: "1.0.2"},
		{Name: "extra", Path: filepath.Join(m.pluginDir("extra"), "nat-source-extra"), Kind: KindManaged, Source: "someone/plugins", Version: "3.0"},
		{Name: "hand", Path: filepath.Join(m.pluginDir("hand"), "nat-source-hand"), Kind: KindManual},
		{Name: "onpath", Path: filepath.Join(bin, "nat-source-onpath"), Kind: KindPath},
		{Name: "pinned", Path: filepath.Join(m.pluginDir("pinned"), "nat-source-pinned"), Kind: KindManaged, Source: DefaultSource, Version: "dev"},
	}
	if !reflect.DeepEqual(got.Installed, wantInstalled) {
		t.Errorf("installed =\n%+v\nwant\n%+v", got.Installed, wantInstalled)
	}
	if len(got.Available) != 2 {
		t.Fatalf("available = %+v", got.Available)
	}
	for _, a := range got.Available {
		if a.Source != DefaultSource || a.Version != "1.0.2" || !a.Installed || a.Title == "" || a.Description == "" {
			t.Errorf("available = %+v", a)
		}
	}
}

func TestListOffersAnUpdateOnlyForAPluginTheSourceStillCarries(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	m := g.manager(t)
	t.Setenv("PATH", "")
	if _, err := m.Install(context.Background(), []string{DefaultSource}, "demo", "", ""); err != nil {
		t.Fatal(err)
	}
	g.release(DefaultSource, "2.0", true, map[string]string{"other": "y"})

	got, err := m.List(context.Background(), []string{DefaultSource})
	if err != nil {
		t.Fatal(err)
	}
	if got.Installed[0].Update != "" || got.Available[0].Installed {
		t.Errorf("listing = %+v", got)
	}
}

func TestListEmptyIsListsNotNulls(t *testing.T) {
	m := &Manager{ConfigDir: t.TempDir()}
	t.Setenv("PATH", "")
	got, err := m.List(context.Background(), nil)
	if err != nil || got.Sources == nil || got.Installed == nil || got.Available == nil {
		t.Errorf("List = %+v, %v", got, err)
	}

	if err := os.WriteFile(filepath.Join(m.ConfigDir, "plugins"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.List(context.Background(), nil); err == nil {
		t.Error("an unreadable plugins dir listed")
	}
}

func writeExec(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
