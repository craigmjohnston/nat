package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/store"
)

// sourceConfig is testConfig with its one project made a source project of
// the demo plugin.
func sourceConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.Projects[testProjectID] = config.ProjectConfig{Name: "tracker", WorkingDir: "/work",
		Backend: config.BackendSource, Source: "demo"}
	return cfg
}

// sourceApp is an app on sourceConfig whose plugin is fake, with no Notion
// client at all: a source project never needs one.
func sourceApp(t *testing.T, fake *source.Fake) *App {
	t.Helper()
	a := NewApp(sourceConfig(t), nil)
	a.newSource = func(name string) (source.Client, error) {
		if name != "demo" {
			t.Errorf("newSource(%q), want the project's own plugin", name)
		}
		return fake, nil
	}
	return a
}

// A source project's store is its plan file wrapped so the plugin hears of
// every task — built without a Notion client — and the plugin is told the
// project as config names it.
func TestAppStoreForBuildsASourcedStore(t *testing.T) {
	fake := &source.Fake{}
	a := sourceApp(t, fake)
	cfg := a.cfg.Projects[testProjectID]
	st, err := a.storeFor(testProjectID, cfg)
	if err != nil {
		t.Fatalf("storeFor = %v, want a store", err)
	}
	if _, ok := st.(*store.Sourced); !ok {
		t.Fatalf("store = %T, want *store.Sourced", st)
	}
	proj := store.ProjectOf(testProjectID, cfg)
	if _, err := st.AddSlice(context.Background(), proj, store.NewSlice{Title: "Fix the spinner",
		Milestone: domain.Milestone{ID: "c1", Name: "Checkout times out"}}); err != nil {
		t.Fatalf("AddSlice = %v", err)
	}
	want := source.Project{ID: testProjectID, Name: "tracker", WorkingDir: "/work"}
	if len(fake.Events) != 1 || fake.Events[0].Project != want || fake.Events[0].Event != source.EventCreated {
		t.Errorf("events = %+v, want one created event for %+v", fake.Events, want)
	}
	if again, _ := a.storeFor(testProjectID, cfg); again != st {
		t.Error("storeFor reopened a store it already held")
	}
}

// A plugin that cannot be found does not stop the plan opening — it is nat's
// own file — but every plugin call then fails with the lookup's own error.
func TestAppStoreForOpensAProjectWhosePluginIsMissing(t *testing.T) {
	a := sourceApp(t, nil)
	boom := errors.New("not installed")
	a.newSource = func(string) (source.Client, error) { return nil, boom }
	st, err := a.storeFor(testProjectID, a.cfg.Projects[testProjectID])
	if err != nil {
		t.Fatalf("storeFor = %v, want the plan opened regardless", err)
	}
	cr, ok := st.(store.ContainerReader)
	if !ok {
		t.Fatalf("store = %T, want a source project's store", st)
	}
	if _, err := cr.Container(context.Background(), "c1"); !errors.Is(err, boom) {
		t.Errorf("Container = %v, want the plugin lookup's own failure", err)
	}
}

// A source project's containers draw as milestones, nothing else changed.
func TestBoardDrawsASourceProjectsContainersAsMilestones(t *testing.T) {
	a := sourceApp(t, &source.Fake{})
	cfg := a.cfg.Projects[testProjectID]
	st, err := a.storeFor(testProjectID, cfg)
	if err != nil {
		t.Fatalf("storeFor = %v", err)
	}
	ctx, proj := context.Background(), store.ProjectOf(testProjectID, cfg)
	for _, n := range []store.NewSlice{
		{Title: "Fix the spinner", Milestone: domain.Milestone{ID: "sc-4821", Name: "Checkout times out"}},
		{Title: "Retry the payment call", Milestone: domain.Milestone{ID: "sc-4821", Name: "Checkout times out"}},
		{Title: "Add the export button", Milestone: domain.Milestone{ID: "sc-4830", Name: "CSV export"}},
	} {
		if _, err := st.AddSlice(ctx, proj, n); err != nil {
			t.Fatalf("AddSlice(%q) = %v", n.Title, err)
		}
	}
	plan, err := st.Plan(ctx, proj)
	if err != nil {
		t.Fatalf("Plan = %v", err)
	}
	b := NewBoard(DefaultStyles())
	b.SetWidth(60)
	b.SetProject(&plan.Project)
	golden(t, "board-source", b.View())
}

// `n` on a source project is refused with a toast naming where tasks are added
// from, and no form opens.
func TestAddSliceRefusedOnASourceProject(t *testing.T) {
	a := newWriteApp(t, &fakeNotion{})
	cfg := a.cfg.Projects[testProjectID]
	cfg.Backend, cfg.Source = config.BackendSource, "shortcut"
	a.cfg.Projects[testProjectID] = cfg
	a.board.cursor = rowActiveMilestone

	feed(t, a, a.addSlice())
	if a.form != nil {
		t.Errorf("form = %T, want none opened", a.form)
	}
	want := "Add tasks to a shortcut project from gnat or `nat slice-add --container`."
	if a.toast != want || a.toastSev != sevWarning {
		t.Errorf("toast = %q (%v), want %q as a warning", a.toast, a.toastSev, want)
	}
}

// The real plugin lookup: nat's own plugins directory, an error for a name
// with no plugin, and an error where there is no config directory at all.
func TestDefaultNewSource(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "notion-agent-tracker", "plugins", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nat-source-demo"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if c, err := defaultNewSource("demo"); err != nil || c == nil {
		t.Errorf("defaultNewSource(demo) = %v, %v, want the installed plugin", c, err)
	}
	if _, err := defaultNewSource("nat-test-absent"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("defaultNewSource(absent) = %v, want it reported not installed", err)
	}
}

func TestDefaultNewSourceReportsAnUnreadablePluginsDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "notion-agent-tracker"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file where the plugins directory should be cannot be listed.
	if err := os.WriteFile(filepath.Join(xdg, "notion-agent-tracker", "plugins"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := defaultNewSource("demo"); err == nil || !strings.Contains(err.Error(), "look for the demo task source") {
		t.Errorf("defaultNewSource = %v, want the lookup's failure", err)
	}
}

func TestDefaultNewSourceReportsNoConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if _, err := defaultNewSource("demo"); err == nil {
		t.Error("want an error with no config directory to look in")
	}
}
