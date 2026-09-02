package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPathsPrintsConfigLogDirAndNudgePath(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{})

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
	env, out := testEnv(testConfig(), &fakeAPI{})

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
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"paths", "stray"}, env)

	if err == nil {
		t.Error("expected error for stray argument")
	}
	if !strings.Contains(err.Error(), "no arguments") {
		t.Errorf("error should mention arguments: %v", err)
	}
}

func TestPathsRejectsUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"paths", "--unknown"}, env)

	if err == nil {
		t.Error("expected error for unknown flag")
	}
}

// TestPathsHandlesUnresolvableHomes tests that paths reports errors when
// path resolution fails due to missing HOME.
func TestPathsHandlesUnresolvableHomes(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

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
	env, _ := testEnv(testConfig(), &fakeAPI{})

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
	env, _ := testEnv(testConfig(), &fakeAPI{})

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
