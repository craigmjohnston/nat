package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/store"
)

func TestPathsPrintsConfigLogDirAndNudgePath(t *testing.T) {
	env, out := testEnv(testConfig(t), &fakeAPI{})

	if err := Run(context.Background(), []string{"paths"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Config:") {
		t.Errorf("output missing 'Config:': %q", output)
	}
	if !strings.Contains(output, "Log dir:") {
		t.Errorf("output missing 'Log dir:': %q", output)
	}
	if !strings.Contains(output, "Nudge:") {
		t.Errorf("output missing 'Nudge:': %q", output)
	}
}

func TestPathsPrintsJSON(t *testing.T) {
	env, out := testEnv(testConfig(t), &fakeAPI{})

	if err := Run(context.Background(), []string{"paths", "--json"}, env); err != nil {
		t.Fatalf("paths --json: %v", err)
	}

	var got pathsJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}

	if got.Config == "" {
		t.Error("config path is empty")
	}
	if got.LogDir == "" {
		t.Error("log dir is empty")
	}
	if got.Nudge == "" {
		t.Error("nudge path is empty")
	}
}

func TestPathsTakesNoArguments(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"paths", "stray"}, env)

	if err == nil {
		t.Error("expected error for stray argument")
	}
	if !strings.Contains(err.Error(), "no arguments") {
		t.Errorf("error should mention arguments: %v", err)
	}
}

func TestPathsRejectsUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"paths", "--unknown"}, env)

	if err == nil {
		t.Error("expected error for unknown flag")
	}
}

// TestPathsHandlesUnresolvableHomes tests that paths reports errors when
// path resolution fails due to missing HOME.
func TestPathsHandlesUnresolvableHomes(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	err := Run(context.Background(), []string{"paths"}, env)

	if err == nil {
		t.Error("expected error when path resolution fails")
	}
}

// TestPathsHandlesUnresolvableLogDir tests that paths reports errors when
// log dir resolution fails. To test this independently, we need to set
// CONFIG_HOME so config.Path() succeeds, but unset HOME and XDG_STATE_HOME
// so logging.Dir() fails.
func TestPathsHandlesUnresolvableLogDir(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	// Set a valid XDG_CONFIG_HOME so config.Path() succeeds
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg_config")
	// But make logging.Dir() fail
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	err := Run(context.Background(), []string{"paths"}, env)

	if err == nil {
		t.Error("expected error when log dir resolution fails")
	}
	if !strings.Contains(err.Error(), "log dir") {
		t.Errorf("error should mention log dir: %v", err)
	}
}

// TestPathsHandlesUnresolvableNudgePath tests that paths reports errors when
// nudge path resolution fails. We use the nudgePathFunc hook to stub out
// nudge.Path() to return an error.
func TestPathsHandlesUnresolvableNudgePath(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	// Save the original nudgePathFunc and restore it at the end
	origNudgePathFunc := nudgePathFunc
	defer func() { nudgePathFunc = origNudgePathFunc }()

	// Stub nudge.Path() to return an error
	nudgePathFunc = func() (string, error) {
		return "", fmt.Errorf("nudge path resolution failed")
	}

	err := Run(context.Background(), []string{"paths"}, env)

	if err == nil {
		t.Error("expected error when nudge path resolution fails")
	}
	if !strings.Contains(err.Error(), "nudge path") {
		t.Errorf("error should mention nudge path: %v", err)
	}
}

// With --project, a local project's plan file is in nat's data directory,
// or the one its entry chose; a source project's likewise.
func TestPathsPrintsAProjectsPlanFile(t *testing.T) {
	cfg := testConfig(t)
	cfg.Projects["Local-1"] = config.ProjectConfig{Name: "Here", Backend: config.BackendLocal}
	cfg.Projects["chosen"] = config.ProjectConfig{Name: "There", Backend: config.BackendLocal, PlanDir: "/plans"}
	cfg.Projects["work"] = config.ProjectConfig{Backend: config.BackendSource, Source: "demo", PlanDir: "/src"}
	dataDir, err := store.LocalDir()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"local1": filepath.Join(dataDir, "local-1.db"),
		"chosen": "/plans/chosen.db",
		"work":   "/src/work.db",
	} {
		env, out := testEnv(cfg, &fakeAPI{})
		if err := Run(context.Background(), []string{"paths", "--json", "--project", id}, env); err != nil {
			t.Fatalf("%s: paths: %v", id, err)
		}
		var got pathsJSON
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("%s: not JSON: %v", id, err)
		}
		if got.Plan != want {
			t.Errorf("%s: plan = %q, want %q", id, got.Plan, want)
		}
	}

	env, out := testEnv(cfg, &fakeAPI{})
	if err := Run(context.Background(), []string{"paths", "--project", "chosen"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}
	if !strings.Contains(out.String(), "Plan:    /plans/chosen.db\n") {
		t.Errorf("output = %q, want a Plan line", out.String())
	}
}

// A project in Notion has no plan file, so nothing is said of one.
func TestPathsPrintsNoPlanFileForANotionProject(t *testing.T) {
	env, out := testEnv(testConfig(t), &fakeAPI{})
	if err := Run(context.Background(), []string{"paths", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}
	if strings.Contains(out.String(), `"plan"`) {
		t.Errorf("output = %s, want no plan", out.String())
	}
	env, out = testEnv(testConfig(t), &fakeAPI{})
	if err := Run(context.Background(), []string{"paths", "--project", "project-1"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}
	if strings.Contains(out.String(), "Plan:") {
		t.Errorf("output = %q, want no Plan line", out.String())
	}
}

func TestPathsRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	err := Run(context.Background(), []string{"paths", "--project", "nope"}, env)
	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

// With --project, paths names the repository's own default branch by name —
// the settings sheet's base placeholder — and nothing for a project with no
// working directory.
func TestPathsPrintsTheRepositorysDefaultBase(t *testing.T) {
	cfg := testConfig(t)
	cfg.Projects["work"] = config.ProjectConfig{Backend: config.BackendSource, Source: "demo"}
	env, out := testEnv(cfg, &fakeAPI{})
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{base: "origin/trunk"}) }
	if err := Run(context.Background(), []string{"paths", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}
	var got pathsJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DefaultBase != "trunk" {
		t.Errorf("default_base = %q, want trunk", got.DefaultBase)
	}

	env, out = testEnv(cfg, &fakeAPI{})
	env.NewGit = func() GitCLI { return git.NewWithRunner(&fakeGitRunner{base: "origin/trunk"}) }
	if err := Run(context.Background(), []string{"paths", "--project", "project-1"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}
	if !strings.Contains(out.String(), "Base:    trunk\n") {
		t.Errorf("output = %q, want a Base line", out.String())
	}

	env, out = testEnv(cfg, &fakeAPI{})
	if err := Run(context.Background(), []string{"paths", "--json", "--project", "work"}, env); err != nil {
		t.Fatalf("paths: %v", err)
	}
	if strings.Contains(out.String(), "default_base") {
		t.Errorf("output = %s, want no default base with no working directory", out.String())
	}
}
