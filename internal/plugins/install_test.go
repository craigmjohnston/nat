package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

// readDisk reads back what an install wrote: the binary and the record.
func readDisk(t *testing.T, m *Manager, name string) (string, fs.FileMode, diskRecord) {
	t.Helper()
	dir := m.pluginDir(name)
	bin := filepath.Join(dir, "nat-source-"+name)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(bin)
	raw, err := os.ReadFile(filepath.Join(dir, "installed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec diskRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	return string(data), info.Mode().Perm(), rec
}

// leftovers is every file in a plugin's directory but the binary and the
// record — a download that was not cleaned up.
func leftovers(t *testing.T, m *Manager, name string) []string {
	t.Helper()
	entries, _ := os.ReadDir(m.pluginDir(name))
	var out []string
	for _, e := range entries {
		if e.Name() != "nat-source-"+name && e.Name() != "installed.json" {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestInstallFromTheFirstSourceOfferingIt(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0.42", true, map[string]string{"other": "x"})
	g.release("someone/plugins", "2.1", true, map[string]string{"demo": "#!/bin/sh\necho demo\n"})
	m := g.manager(t)

	rec, err := m.Install(context.Background(), []string{DefaultSource, "someone/plugins"}, "demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(m.ConfigDir, "plugins", "demo", "nat-source-demo")
	want := Record{Name: "demo", Path: bin, Source: "someone/plugins", Version: "2.1",
		SHA256: rec.SHA256, InstalledAt: "2026-10-03T11:00:00Z"}
	if rec != want || len(rec.SHA256) != 64 {
		t.Errorf("Install = %+v, want %+v", rec, want)
	}
	body, mode, disk := readDisk(t, m, "demo")
	if body != "#!/bin/sh\necho demo\n" || mode != 0o755 {
		t.Errorf("binary = %q %v", body, mode)
	}
	if disk != (diskRecord{Source: "someone/plugins", Version: "2.1", SHA256: rec.SHA256, InstalledAt: rec.InstalledAt}) {
		t.Errorf("record = %+v", disk)
	}
	if l := leftovers(t, m, "demo"); len(l) != 0 {
		t.Errorf("left behind %v", l)
	}
	wantPaths := []string{
		"/craigmjohnston/nat/releases/latest/download/nat-plugins.json",
		"/someone/plugins/releases/latest/download/nat-plugins.json",
		"/someone/plugins/releases/download/v2.1/nat-source-demo",
	}
	if got := g.paths(); strings.Join(got, " ") != strings.Join(wantPaths, " ") {
		t.Errorf("asked for %v, want %v", got, wantPaths)
	}
}

func TestInstallOverAManagedInstallIsTheUpdate(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0.1", false, map[string]string{"demo": "old"})
	g.release(DefaultSource, "1.0.2", true, map[string]string{"demo": "new"})
	m := g.manager(t)
	ctx := context.Background()

	if _, err := m.Install(ctx, []string{DefaultSource}, "demo", DefaultSource, "1.0.1"); err != nil {
		t.Fatal(err)
	}
	if body, _, rec := readDisk(t, m, "demo"); body != "old" || rec.Version != "1.0.1" {
		t.Fatalf("pinned install = %q %+v", body, rec)
	}
	if _, err := m.Install(ctx, []string{DefaultSource}, "demo", "", ""); err != nil {
		t.Fatal(err)
	}
	if body, _, rec := readDisk(t, m, "demo"); body != "new" || rec.Version != "1.0.2" {
		t.Errorf("update = %q %+v", body, rec)
	}

	// A record that will not parse is still one nat wrote.
	if err := os.WriteFile(filepath.Join(m.pluginDir("demo"), "installed.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, []string{DefaultSource}, "demo", "", ""); err != nil {
		t.Errorf("over a broken record = %v", err)
	}
}

func TestInstallNeverClobbersAManualInstall(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "new"})
	m := g.manager(t)
	dir := m.pluginDir("demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nat-source-demo"), []byte("mine"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := m.Install(context.Background(), []string{DefaultSource}, "demo", "", "")
	if err == nil || err.Error() != "plugin demo is installed by hand at "+dir+": nat will not overwrite it — remove it first" {
		t.Errorf("Install over manual = %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "nat-source-demo")); string(data) != "mine" {
		t.Errorf("manual binary now %q", data)
	}
	if len(g.paths()) != 0 {
		t.Error("a refused install fetched anything")
	}
}

func TestInstallRefusesADigestMismatchAndCleansUp(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "real"})
	g.file("/craigmjohnston/nat/releases/download/v1.0/nat-source-demo", "tampered")
	m := g.manager(t)

	_, err := m.Install(context.Background(), []string{DefaultSource}, "demo", "", "")
	if err == nil || !strings.Contains(err.Error(), "download plugin demo: its SHA-256 is ") || !strings.HasSuffix(err.Error(), "— not installed") {
		t.Errorf("mismatch = %v", err)
	}
	if _, err := os.Stat(m.pluginDir("demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a fresh install's directory survived its failure: %v", err)
	}
}

func TestInstallRefusals(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	m := g.manager(t)
	ctx := context.Background()
	src := []string{DefaultSource, "dead/source"}

	if _, err := m.Install(ctx, src, "../x", "", ""); err == nil || !strings.Contains(err.Error(), "is not a plugin name") {
		t.Errorf("bad name = %v", err)
	}
	if _, err := m.Install(ctx, src, "demo", "not a repo", ""); err == nil || !strings.Contains(err.Error(), "is not a GitHub repository") {
		t.Errorf("bad --source = %v", err)
	}
	_, err := m.Install(ctx, src, "nope", "", "")
	if err == nil || !strings.HasPrefix(err.Error(), "no source offers plugin nope: craigmjohnston/nat v1.0 offers no plugin nope; read plugin source dead/source: GET ") {
		t.Errorf("offered nowhere = %v", err)
	}
	if _, err := m.Install(ctx, src, "demo", "", "9.9"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("no such version = %v", err)
	}

	// The asset the manifest names is not there.
	g.manifest("gap/py", "1.0", true, Manifest{Version: "1.0", Plugins: []Entry{{Name: "demo", Asset: "nat-source-demo", SHA256: strings.Repeat("0", 64)}}})
	if _, err := m.Install(ctx, src, "demo", "gap/py", ""); err == nil || !strings.HasPrefix(err.Error(), "download plugin demo: GET ") {
		t.Errorf("missing asset = %v", err)
	}
}

func TestInstallFilesystemFailures(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	ctx := context.Background()
	src := []string{DefaultSource}

	// plugins is a file: the plugin's directory cannot even be looked at.
	m := g.manager(t)
	if err := os.WriteFile(filepath.Join(m.ConfigDir, "plugins"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, src, "demo", "", ""); err == nil {
		t.Error("Install under a file = nil")
	}

	// A record that cannot be read at all.
	m = g.manager(t)
	if err := os.MkdirAll(filepath.Join(m.pluginDir("demo"), "installed.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, src, "demo", "", ""); err == nil || strings.Contains(err.Error(), "by hand") {
		t.Errorf("unreadable record = %v", err)
	}

	// The config dir cannot be written to.
	m = g.manager(t)
	readOnly := m.ConfigDir
	if err := os.Chmod(readOnly, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })
	if _, err := m.Install(ctx, src, "demo", "", ""); err == nil {
		t.Error("Install into a read-only config dir = nil")
	}

	// A managed install's directory that cannot be written to.
	m = managedInstall(t, g)
	locked := m.pluginDir("demo")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := m.Install(ctx, src, "demo", "", ""); err == nil {
		t.Error("Install into a read-only plugin dir = nil")
	}

	// The binary's place is taken by a directory that will not be replaced.
	m = managedInstall(t, g)
	bin := filepath.Join(m.pluginDir("demo"), "nat-source-demo")
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bin, "full"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, src, "demo", "", ""); err == nil {
		t.Error("rename over a full directory = nil")
	}
	if l := leftovers(t, m, "demo"); len(l) != 0 {
		t.Errorf("left behind %v", l)
	}

	boom := errors.New("boom")
	m = managedInstall(t, g)
	chmod = func(string, os.FileMode) error { return boom }
	if _, err := m.Install(ctx, src, "demo", "", ""); !errors.Is(err, boom) {
		t.Errorf("chmod failure = %v", err)
	}
	chmod = os.Chmod

	m = g.manager(t)
	writeRecord = func(string, []byte, os.FileMode) error { return boom }
	if _, err := m.Install(ctx, src, "demo", "", ""); !errors.Is(err, boom) {
		t.Errorf("record failure = %v", err)
	}
	writeRecord = os.WriteFile
	if _, err := os.Stat(m.pluginDir("demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a fresh install with no record left its binary behind")
	}
}

// managedInstall is a fresh Manager with demo already installed from g.
func managedInstall(t *testing.T, g *fakeGitHub) *Manager {
	t.Helper()
	m := g.manager(t)
	if _, err := m.Install(context.Background(), []string{DefaultSource}, "demo", "", ""); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDownloadFailures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/short", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("only some"))
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0123456789"))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	m := &Manager{BaseURL: srv.URL, HTTP: srv.Client()}

	var sink strings.Builder
	if _, err := m.download(context.Background(), srv.URL+"/short", &sink); err == nil {
		t.Error("a cut-off body downloaded")
	}
	maxBinary = 5
	defer func() { maxBinary = 256 << 20 }()
	if _, err := m.download(context.Background(), srv.URL+"/big", &sink); err == nil || !strings.Contains(err.Error(), "not a plugin") {
		t.Errorf("an oversized body = %v", err)
	}
}

func TestUninstall(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	t.Setenv("PATH", "")

	m := managedInstall(t, g)
	gone, err := m.Uninstall("demo", "Demo", config.Config{}, false, noSave(t))
	dir := gone.Path
	if err != nil || dir != m.pluginDir("demo") || gone.ProjectsDeleted == nil || len(gone.ProjectsDeleted) != 0 {
		t.Fatalf("Uninstall = %+v, %v", gone, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the plugin's directory survived")
	}

	// A hand-placed one goes too: the user asked.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Uninstall("demo", "Demo", config.Config{}, false, noSave(t)); err != nil {
		t.Errorf("manual uninstall = %v", err)
	}

	if _, err := m.Uninstall("demo", "Demo", config.Config{}, false, noSave(t)); err == nil || err.Error() != "no plugin demo is installed" {
		t.Errorf("nothing installed = %v", err)
	}
	if _, err := m.Uninstall("Bad", "Bad", config.Config{}, false, noSave(t)); err == nil {
		t.Error("a bad name uninstalled")
	}
}

func TestUninstallRefusesWhileAProjectUsesIt(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	m := managedInstall(t, g)
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"p2": {Name: "Work", Backend: config.BackendSource, Source: "demo"},
		"p1": {Name: "Home", Backend: config.BackendSource, Source: "demo"},
		"p3": {Name: "Other", Backend: config.BackendSource, Source: "else"},
		"p4": {Name: "Local", Backend: config.BackendLocal},
	}}
	_, err := m.Uninstall("demo", "Demo", cfg, false, noSave(t))
	if err == nil || err.Error() != "plugin demo is the source of Demo (p1), Demo (p2): delete or move those projects first, or pass --delete-projects" {
		t.Errorf("in use = %v", err)
	}
	if _, err := os.Stat(m.pluginDir("demo")); err != nil {
		t.Error("a refused uninstall removed the plugin")
	}
}

func TestUninstallRefusesAPluginOnPath(t *testing.T) {
	m := &Manager{ConfigDir: t.TempDir()}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "nat-source-demo"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	_, err := m.Uninstall("demo", "Demo", config.Config{}, false, noSave(t))
	if err == nil || err.Error() != "plugin demo is on PATH at "+filepath.Join(bin, "nat-source-demo")+", not installed by nat: remove it there" {
		t.Errorf("on PATH = %v", err)
	}

	// A plugins dir that cannot be read.
	if err := os.WriteFile(filepath.Join(m.ConfigDir, "plugins"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Uninstall("demo", "Demo", config.Config{}, false, noSave(t)); err == nil {
		t.Error("an unreadable plugins dir = nil")
	}
}

func TestUninstallReportsARemovalFailure(t *testing.T) {
	m := &Manager{ConfigDir: t.TempDir()}
	dir := filepath.Join(m.pluginDir("demo"), "inner")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := m.Uninstall("demo", "Demo", config.Config{}, false, noSave(t)); err == nil {
		t.Error("an unremovable directory uninstalled")
	}
}

// noSave is a save an uninstall that deletes no project never calls.
func noSave(t *testing.T) func(config.Config) error {
	return func(config.Config) error {
		t.Helper()
		t.Error("saved the config with no project to delete")
		return nil
	}
}

// sourceProjects is a config with two source projects of demo (plans in
// plans), one of another plugin and a local one, demo's Work active.
func sourceProjects(plans string) config.Config {
	return config.Config{ActiveProjectID: "p2", Projects: map[string]config.ProjectConfig{
		"p2": {Name: "Work", Backend: config.BackendSource, Source: "demo", PlanDir: plans},
		"p1": {Name: "Home", Backend: config.BackendSource, Source: "demo", PlanDir: plans},
		"p3": {Name: "Other", Backend: config.BackendSource, Source: "else", PlanDir: plans},
		"p4": {Name: "Local", Backend: config.BackendLocal, PlanDir: plans},
	}}
}

func TestUninstallDeletesItsProjectsWhenAsked(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	m := managedInstall(t, g)
	plans := t.TempDir()
	// p1's plan has SQLite's journal files beside it; p2's plan is already gone.
	for _, f := range []string{"p1.db", "p1.db-wal", "p1.db-shm", "p3.db", "p4.db"} {
		if err := os.WriteFile(filepath.Join(plans, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var saves []config.Config
	save := func(c config.Config) error {
		// Each save comes after that project's plan is gone.
		for id := range sourceProjects(plans).Projects {
			if _, kept := c.Projects[id]; !kept {
				if _, err := os.Stat(filepath.Join(plans, id+".db")); err == nil {
					t.Errorf("%s's entry went before its plan", id)
				}
			}
		}
		saves = append(saves, c)
		return nil
	}
	gone, err := m.Uninstall("demo", "Demo", sourceProjects(plans), true, save)
	if err != nil {
		t.Fatal(err)
	}
	want := []DeletedProject{{ID: "p1", Name: "Demo"}, {ID: "p2", Name: "Demo"}}
	if !reflect.DeepEqual(gone.ProjectsDeleted, want) || gone.Path != m.pluginDir("demo") {
		t.Errorf("Uninstall = %+v", gone)
	}
	left, _ := os.ReadDir(plans)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if strings.Join(names, " ") != "p3.db p4.db" {
		t.Errorf("plans left = %v", names)
	}
	if len(saves) != 2 {
		t.Fatalf("saves = %d, want one per project", len(saves))
	}
	last := saves[1]
	if len(last.Projects) != 2 || last.ActiveProjectID != "" {
		t.Errorf("last save = %+v, want p3 and p4 and no active project", last)
	}
	if _, err := os.Stat(m.pluginDir("demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the plugin's directory survived")
	}
}

func TestUninstallDeletingProjectsStopsOnAFailure(t *testing.T) {
	g := newFakeGitHub(t)
	g.release(DefaultSource, "1.0", true, map[string]string{"demo": "x"})
	m := managedInstall(t, g)

	// A plan the OS will not remove: nothing of that project goes, nor the plugin.
	plans := t.TempDir()
	if err := os.WriteFile(filepath.Join(plans, "p1.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(plans, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(plans, 0o755) })
	_, err := m.Uninstall("demo", "Demo", sourceProjects(plans), true, noSave(t))
	if err == nil || !strings.HasPrefix(err.Error(), "delete project Demo (p1): ") {
		t.Errorf("unremovable plan = %v", err)
	}
	if _, err := os.Stat(filepath.Join(plans, "p1.db")); err != nil {
		t.Error("the refused plan went")
	}
	if _, err := os.Stat(m.pluginDir("demo")); err != nil {
		t.Error("a failed delete removed the plugin")
	}

	// A config that will not save.
	boom := errors.New("boom")
	_, err = m.Uninstall("demo", "Demo", sourceProjects(t.TempDir()), true, func(config.Config) error { return boom })
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "save config") {
		t.Errorf("failed save = %v", err)
	}

	// A plan with nowhere to be: no home to put nat's data directory under.
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	_, err = m.Uninstall("demo", "Demo", sourceProjects(""), true, noSave(t))
	if err == nil || !strings.Contains(err.Error(), "home") {
		t.Errorf("no plan path = %v", err)
	}
}

// TestUninstallDeletesNoProjectOfAPluginItCannotRemove: a plugin on PATH is
// refused before any of its projects is touched.
func TestUninstallDeletesNoProjectOfAPluginItCannotRemove(t *testing.T) {
	m := &Manager{ConfigDir: t.TempDir()}
	t.Setenv("PATH", "")
	plans := t.TempDir()
	if err := os.WriteFile(filepath.Join(plans, "p1.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Uninstall("demo", "Demo", sourceProjects(plans), true, noSave(t)); err == nil || err.Error() != "no plugin demo is installed" {
		t.Errorf("not installed = %v", err)
	}
	if _, err := os.Stat(filepath.Join(plans, "p1.db")); err != nil {
		t.Error("a refused uninstall deleted a project's plan")
	}
}
