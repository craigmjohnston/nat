package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/plugins"
	"github.com/craigmjohnston/nat/internal/source"
)

// pluginEnv is an Env whose plugin installer talks to a GitHub where nat's
// own repository's latest release, 1.0.2, carries the demo plugin, and
// installs under a config dir of the test's own. The config it loads is cfg,
// and what it saves is kept in saved.
func pluginEnv(t *testing.T, cfg config.Config, found bool) (Env, *bytes.Buffer, *plugins.Manager, *config.Config) {
	t.Helper()
	t.Setenv("PATH", "")
	bin := "#!/bin/sh\n"
	sum := sha256.Sum256([]byte(bin))
	manifest := fmt.Sprintf(`{"version":"1.0.2","plugins":[{"name":"demo","title":"Demo","description":"A demo.","asset":"nat-source-demo","sha256":%q}]}`,
		hex.EncodeToString(sum[:]))
	mux := http.NewServeMux()
	serveManifest := func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(manifest)) }
	mux.HandleFunc("/craigmjohnston/nat/releases/latest/download/nat-plugins.json", serveManifest)
	mux.HandleFunc("/craigmjohnston/nat/releases/download/v1.0.2/nat-plugins.json", serveManifest)
	mux.HandleFunc("/craigmjohnston/nat/releases/download/v1.0.2/nat-source-demo", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(bin))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	m := &plugins.Manager{ConfigDir: t.TempDir(), BaseURL: srv.URL, HTTP: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }}
	state := &cfg
	var out bytes.Buffer
	return Env{
		Load:       func() (config.Config, bool, error) { return *state, found, nil },
		Save:       func(c config.Config) error { *state = c; return nil },
		NewPlugins: func() (*plugins.Manager, error) { return m, nil },
		NewSource:  func(string) (source.Client, error) { return &source.Fake{DescribeResult: demoDescribe()}, nil },
		Out:        &out,
	}, &out, m, state
}

func runPlugin(t *testing.T, env Env, out *bytes.Buffer, args ...string) string {
	t.Helper()
	out.Reset()
	if err := Run(context.Background(), args, env); err != nil {
		t.Fatalf("%s: %v", strings.Join(args, " "), err)
	}
	return out.String()
}

func TestPluginInstallListUninstall(t *testing.T) {
	env, out, m, _ := pluginEnv(t, config.Config{PluginSources: []string{"dead/source"}}, true)

	var listed plugins.Listing
	if err := json.Unmarshal([]byte(runPlugin(t, env, out, "plugin-list", "--json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sources) != 2 || !listed.Sources[0].Default || listed.Sources[0].Version != "1.0.2" ||
		listed.Sources[1].Error == "" || len(listed.Installed) != 0 ||
		len(listed.Available) != 1 || listed.Available[0].Installed || listed.Available[0].Title != "Demo" {
		t.Errorf("plugin-list = %+v", listed)
	}

	var rec plugins.Record
	if err := json.Unmarshal([]byte(runPlugin(t, env, out, "plugin-install", "demo", "--version", "v1.0.2", "--json")), &rec); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(m.ConfigDir, "plugins", "demo", "nat-source-demo")
	if rec.Name != "demo" || rec.Path != bin || rec.Version != "1.0.2" || rec.Source != plugins.DefaultSource || rec.InstalledAt != "2026-10-03T12:00:00Z" {
		t.Errorf("plugin-install = %+v", rec)
	}
	if got := runPlugin(t, env, out, "plugin-install", "demo", "--source", plugins.DefaultSource); got != "Installed demo 1.0.2 from craigmjohnston/nat at "+bin+".\n" {
		t.Errorf("plugin-install text = %q", got)
	}

	text := runPlugin(t, env, out, "plugin-list")
	for _, want := range []string{"sources:\n  craigmjohnston/nat\t1.0.2\n  dead/source\terror: ",
		"installed:\n  demo\tmanaged\t" + bin + "\t1.0.2 from craigmjohnston/nat\n",
		"available:\n  demo\tcraigmjohnston/nat 1.0.2\tDemo\t(installed)\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("plugin-list text missing %q:\n%s", want, text)
		}
	}

	var gone pluginUninstalledJSON
	if err := json.Unmarshal([]byte(runPlugin(t, env, out, "plugin-uninstall", "demo", "--json")), &gone); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gone, pluginUninstalledJSON{Name: "demo", Path: filepath.Dir(bin), ProjectsDeleted: []plugins.DeletedProject{}}) {
		t.Errorf("plugin-uninstall = %+v", gone)
	}
	if !strings.Contains(out.String(), `"projects_deleted": []`) {
		t.Errorf("plugin-uninstall JSON leaves out an empty projects_deleted: %s", out.String())
	}
	runPlugin(t, env, out, "plugin-install", "demo")
	if got := runPlugin(t, env, out, "plugin-uninstall", "demo"); got != "Uninstalled demo from "+filepath.Dir(bin)+".\n" {
		t.Errorf("plugin-uninstall text = %q", got)
	}
}

// TestPluginUninstallDeletesProjects: --delete-projects deletes every source
// project of the plugin — plan file and config entry — says so, and nudges.
func TestPluginUninstallDeletesProjects(t *testing.T) {
	plans := t.TempDir()
	cfg := config.Config{ActiveProjectID: "p1", Projects: map[string]config.ProjectConfig{
		"p1": {Name: "Work", Backend: config.BackendSource, Source: "demo", PlanDir: plans},
		"p2": {Name: "Home", Backend: config.BackendSource, Source: "demo", PlanDir: plans},
		"p3": {Name: "Mine", Backend: config.BackendLocal, PlanDir: plans},
	}}
	env, out, m, saved := pluginEnv(t, cfg, true)
	nudges := 0
	env.Nudge = func() { nudges++ }
	ctx := context.Background()
	for _, id := range []string{"p1", "p2", "p3"} {
		if err := os.WriteFile(filepath.Join(plans, id+".db"), []byte("plan"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Install(ctx, []string{plugins.DefaultSource}, "demo", "", ""); err != nil {
		t.Fatal(err)
	}

	var gone pluginUninstalledJSON
	if err := json.Unmarshal([]byte(runPlugin(t, env, out, "plugin-uninstall", "demo", "--delete-projects", "--json")), &gone); err != nil {
		t.Fatal(err)
	}
	// Each named by its plugin's title, whatever its entry says.
	want := []plugins.DeletedProject{{ID: "p1", Name: "Demo source"}, {ID: "p2", Name: "Demo source"}}
	if !reflect.DeepEqual(gone.ProjectsDeleted, want) || gone.Path != filepath.Join(m.ConfigDir, "plugins", "demo") {
		t.Errorf("plugin-uninstall --delete-projects = %+v", gone)
	}
	if _, ok := saved.Projects["p3"]; !ok || len(saved.Projects) != 1 || saved.ActiveProjectID != "" {
		t.Errorf("saved = %+v, want only p3 and no active project", saved)
	}
	if _, err := os.Stat(filepath.Join(plans, "p3.db")); err != nil {
		t.Error("another project's plan was deleted")
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want 1", nudges)
	}

	// The plain form says what went.
	saved.Projects["p4"] = config.ProjectConfig{Name: "Again", Backend: config.BackendSource, Source: "demo", PlanDir: plans}
	if _, err := m.Install(ctx, []string{plugins.DefaultSource}, "demo", "", ""); err != nil {
		t.Fatal(err)
	}
	text := runPlugin(t, env, out, "plugin-uninstall", "demo", "--delete-projects")
	if text != "Deleted project Demo source (p4).\nUninstalled demo from "+gone.Path+".\n" {
		t.Errorf("plugin-uninstall --delete-projects text = %q", text)
	}
}

func TestPluginListTextShowsAnUpdate(t *testing.T) {
	got := pluginListText(pluginListingJSON{Installed: []installedPluginJSON{
		{Installed: plugins.Installed{Name: "demo", Kind: plugins.KindManaged, Path: "/p", Source: "a/b", Version: "1", Update: "2"}},
		{Installed: plugins.Installed{Name: "hand", Kind: plugins.KindManual, Path: "/h"}},
	}})
	if !strings.Contains(got, "  demo\tmanaged\t/p\t1 from a/b\t(update: 2)\n  hand\tmanual\t/h\n") {
		t.Errorf("text = %q", got)
	}
}

// TestPluginListDescribesEachInstalled: every installed plugin carries its
// describe's setup fields, or — where describe failed — none and the line it
// failed with: the plugin's own stderr line, else nat's words.
func TestPluginListDescribesEachInstalled(t *testing.T) {
	env, out, m, _ := pluginEnv(t, config.Config{}, true)
	for _, name := range []string{"broken", "missing", "shortcut"} {
		dir := filepath.Join(m.ConfigDir, "plugins", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "nat-source-"+name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	withSetup := demoDescribe()
	set := true
	withSetup.Setup = []source.SetupField{{ID: "token", Label: "API token", Input: source.InputSecret, Hint: "Settings", Set: &set}}
	env.NewSource = func(name string) (source.Client, error) {
		switch name {
		case "broken":
			return &source.Fake{DescribeErr: fmt.Errorf("nat-source-broken describe: %w",
				&source.ExitError{Code: 1, Stderr: "Token missing — set it in Settings\nmore\n"})}, nil
		case "missing":
			return nil, errors.New("no task source plugin named \"missing\"")
		}
		return &source.Fake{DescribeResult: withSetup}, nil
	}

	var listed pluginListingJSON
	if err := json.Unmarshal([]byte(runPlugin(t, env, out, "plugin-list", "--json")), &listed); err != nil {
		t.Fatal(err)
	}
	got := map[string]installedPluginJSON{}
	for _, p := range listed.Installed {
		got[p.Name] = p
	}
	if p := got["broken"]; p.DescribeError != "Token missing — set it in Settings" || p.Setup == nil || len(p.Setup) != 0 {
		t.Errorf("broken = %+v, want its stderr line and an empty setup list", p)
	}
	if p := got["missing"]; p.DescribeError != `no task source plugin named "missing"` {
		t.Errorf("missing = %+v, want nat's own words", p)
	}
	if p := got["shortcut"]; p.DescribeError != "" || !reflect.DeepEqual(p.Setup, withSetup.Setup) {
		t.Errorf("shortcut = %+v, want its setup fields", p)
	}
	if !strings.Contains(out.String(), `"setup": []`) || !strings.Contains(out.String(), `"describe_error": ""`) ||
		!strings.Contains(out.String(), `"set": true`) {
		t.Errorf("JSON leaves out an empty field: %s", out.String())
	}

	text := runPlugin(t, env, out, "plugin-list")
	for _, want := range []string{"\terror: Token missing — set it in Settings\n", "\tsetup: token\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("plugin-list text missing %q:\n%s", want, text)
		}
	}
}

func TestPluginCommandRefusals(t *testing.T) {
	env, _, m, _ := pluginEnv(t, config.Config{Projects: map[string]config.ProjectConfig{
		"p1": {Name: "Work", Backend: config.BackendSource, Source: "demo"},
	}}, true)
	ctx := context.Background()
	if _, err := m.Install(ctx, []string{plugins.DefaultSource}, "demo", "", ""); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"plugin-list", "x"}, "plugin-list: takes no arguments"},
		{[]string{"plugin-list", "--nope"}, "plugin-list: flag provided but not defined"},
		{[]string{"plugin-install"}, "plugin-install: want exactly one plugin"},
		{[]string{"plugin-install", "--nope"}, "plugin-install: flag provided"},
		{[]string{"plugin-install", "nope"}, "no source offers plugin nope"},
		{[]string{"plugin-uninstall"}, "plugin-uninstall: want exactly one plugin"},
		{[]string{"plugin-uninstall", "--nope"}, "plugin-uninstall: flag provided"},
		{[]string{"plugin-uninstall", "demo"}, "plugin demo is the source of Demo source (p1): delete or move those projects first, or pass --delete-projects"},
	} {
		err := Run(ctx, c.args, env)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v = %v, want %q", c.args, err, c.want)
		}
	}

	// A plugins dir that cannot be read fails the listing.
	broken := *m
	broken.ConfigDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(broken.ConfigDir, "plugins"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env.NewPlugins = func() (*plugins.Manager, error) { return &broken, nil }
	if err := Run(ctx, []string{"plugin-list"}, env); err == nil || !strings.Contains(err.Error(), "look for task source plugins") {
		t.Errorf("unreadable plugins dir = %v", err)
	}

	// Neither a failed config nor a failed installer gets as far as a fetch.
	boom := errors.New("boom")
	env.NewPlugins = func() (*plugins.Manager, error) { return nil, boom }
	for _, args := range [][]string{{"plugin-list"}, {"plugin-install", "demo"}, {"plugin-uninstall", "demo"}} {
		if err := Run(ctx, args, env); !errors.Is(err, boom) {
			t.Errorf("%v with no installer = %v", args, err)
		}
	}
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, boom }
	for _, args := range [][]string{{"plugin-list"}, {"plugin-install", "demo"}, {"plugin-uninstall", "demo"}} {
		if err := Run(ctx, args, env); !errors.Is(err, boom) {
			t.Errorf("%v with no config = %v", args, err)
		}
	}
}

func TestPluginSourceAddAndRemove(t *testing.T) {
	env, out, _, saved := pluginEnv(t, config.Config{}, true)

	var got pluginSourcesJSON
	if err := json.Unmarshal([]byte(runPlugin(t, env, out, "plugin-source-add", "someone/plugins", "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Sources, " ") != "craigmjohnston/nat someone/plugins" || len(saved.PluginSources) != 1 {
		t.Errorf("plugin-source-add = %+v, saved %v", got, saved.PluginSources)
	}
	if text := runPlugin(t, env, out, "plugin-source-add", "other/one"); text != "craigmjohnston/nat\nsomeone/plugins\nother/one\n" {
		t.Errorf("plugin-source-add text = %q", text)
	}
	if text := runPlugin(t, env, out, "plugin-source-remove", "someone/plugins"); text != "craigmjohnston/nat\nother/one\n" {
		t.Errorf("plugin-source-remove text = %q", text)
	}
	if strings.Join(saved.PluginSources, " ") != "other/one" {
		t.Errorf("saved = %v", saved.PluginSources)
	}

	ctx := context.Background()
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"plugin-source-add"}, "plugin-source-add: want exactly one plugin source, as owner/repo"},
		{[]string{"plugin-source-add", "--nope"}, "plugin-source-add: flag provided"},
		{[]string{"plugin-source-add", "nope"}, `plugin-source-add: "nope" is not a GitHub repository`},
		{[]string{"plugin-source-add", "other/one"}, "plugin-source-add: other/one is already a plugin source"},
		{[]string{"plugin-source-add", plugins.DefaultSource}, "plugin-source-add: craigmjohnston/nat is already a plugin source"},
		{[]string{"plugin-source-remove", plugins.DefaultSource}, "plugin-source-remove: craigmjohnston/nat is nat's own plugin source and is always read"},
		{[]string{"plugin-source-remove", "never/added"}, "plugin-source-remove: never/added is not a plugin source"},
	} {
		err := Run(ctx, c.args, env)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v = %v, want %q", c.args, err, c.want)
		}
	}

	boom := errors.New("boom")
	env.Save = func(config.Config) error { return boom }
	if err := Run(ctx, []string{"plugin-source-add", "a/b"}, env); !errors.Is(err, boom) {
		t.Errorf("failed save = %v", err)
	}
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }
	if err := Run(ctx, []string{"plugin-source-add", "a/b"}, env); err == nil || !strings.Contains(err.Error(), "no configuration yet") {
		t.Errorf("no config = %v", err)
	}
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, boom }
	if err := Run(ctx, []string{"plugin-source-add", "a/b"}, env); !errors.Is(err, boom) {
		t.Errorf("failed load = %v", err)
	}
}

func TestDefaultNewPlugins(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	m, err := DefaultNewPlugins()
	if err != nil || m.ConfigDir != filepath.Join(cfgDir, "notion-agent-tracker") || m.BaseURL != "https://github.com" {
		t.Errorf("DefaultNewPlugins = %+v, %v", m, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, err := DefaultNewPlugins(); err == nil {
		t.Error("DefaultNewPlugins with no home = nil")
	}
}
