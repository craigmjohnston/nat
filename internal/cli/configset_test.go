package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

// savingEnv wires env.Save to capture what was written, so a test can assert
// on the config config-set actually saved rather than only on its own report.
func savingEnv(cfg config.Config) (Env, *config.Config) {
	env, _ := testEnv(cfg, &fakeAPI{})
	var saved config.Config
	env.Save = func(c config.Config) error {
		saved = c
		return nil
	}
	return env, &saved
}

func TestConfigSetSplitPercent(t *testing.T) {
	env, saved := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "agent_split_percent", "70"}, env)
	if err != nil {
		t.Fatalf("config-set: %v", err)
	}
	if saved.AgentSplitPercent != 70 {
		t.Errorf("saved split = %d, want 70", saved.AgentSplitPercent)
	}
}

func TestConfigSetSplitPercentEmptyUnsets(t *testing.T) {
	env, saved := savingEnv(config.Config{AgentSplitPercent: 70})

	err := Run(context.Background(), []string{"config-set", "agent_split_percent", ""}, env)
	if err != nil {
		t.Fatalf("config-set: %v", err)
	}
	if saved.AgentSplitPercent != 0 {
		t.Errorf("saved split = %d, want 0 (unset)", saved.AgentSplitPercent)
	}
}

func TestConfigSetSplitPercentRefusesOutOfBounds(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "agent_split_percent", "5"}, env)

	if err == nil || !strings.Contains(err.Error(), "between") {
		t.Errorf("err = %v, want the bounds named", err)
	}
}

func TestConfigSetSplitPercentRefusesNonNumeric(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "agent_split_percent", "lots"}, env)

	if err == nil || !strings.Contains(err.Error(), "wants a number") {
		t.Errorf("err = %v, want 'wants a number'", err)
	}
}

func TestConfigSetPollSeconds(t *testing.T) {
	env, saved := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "poll_seconds", "45"}, env)
	if err != nil {
		t.Fatalf("config-set: %v", err)
	}
	if saved.PollSeconds != 45 {
		t.Errorf("saved poll = %d, want 45", saved.PollSeconds)
	}
}

func TestConfigSetPollSecondsRefusesNonNumeric(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "poll_seconds", "lots"}, env)

	if err == nil || !strings.Contains(err.Error(), "wants a number") {
		t.Errorf("err = %v, want 'wants a number'", err)
	}
}

func TestConfigSetPollSecondsRefusesOutOfBounds(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "poll_seconds", "1"}, env)

	if err == nil || !strings.Contains(err.Error(), "between") {
		t.Errorf("err = %v, want the bounds named", err)
	}
}

func TestConfigSetModelFields(t *testing.T) {
	tests := []struct {
		key   string
		check func(config.Config) string
	}{
		{"workshop_agent.model", func(c config.Config) string { return c.WorkshopAgent.Model }},
		{"workshop_agent.effort", func(c config.Config) string { return c.WorkshopAgent.Effort }},
		{"slice_agent.model", func(c config.Config) string { return c.SliceAgent.Model }},
		{"slice_agent.effort", func(c config.Config) string { return c.SliceAgent.Effort }},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			env, saved := savingEnv(config.Config{})

			err := Run(context.Background(), []string{"config-set", tt.key, "opus"}, env)
			if err != nil {
				t.Fatalf("config-set %s: %v", tt.key, err)
			}
			if got := tt.check(*saved); got != "opus" {
				t.Errorf("%s = %q, want %q", tt.key, got, "opus")
			}
		})
	}
}

func TestConfigSetProjectWorkingDir(t *testing.T) {
	env, saved := savingEnv(testConfig())

	err := Run(context.Background(), []string{"config-set", "project.project-1.working_dir", "/new/dir"}, env)
	if err != nil {
		t.Fatalf("config-set: %v", err)
	}
	if got := saved.Projects["project-1"].WorkingDir; got != "/new/dir" {
		t.Errorf("working_dir = %q, want %q", got, "/new/dir")
	}
	// The rest of the project is left exactly as it was.
	if got := saved.Projects["project-1"].Name; got != "nat" {
		t.Errorf("name = %q, want it untouched", got)
	}
}

// TestConfigSetProjectWorkingDirMatchesANormalisedID pins the same fallback
// --project itself uses: an ID copied out of a page URL has no dashes and may
// differ in case from the key the config file stores.
func TestConfigSetProjectWorkingDirMatchesANormalisedID(t *testing.T) {
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE": {Name: "nat", WorkingDir: "/old/dir"},
	}}
	env, saved := savingEnv(cfg)

	err := Run(context.Background(), []string{
		"config-set", "project.aaaaaaaabbbbccccddddeeeeeeeeeeee.working_dir", "/new/dir",
	}, env)
	if err != nil {
		t.Fatalf("config-set: %v", err)
	}
	if got := saved.Projects["AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"].WorkingDir; got != "/new/dir" {
		t.Errorf("working_dir = %q, want %q", got, "/new/dir")
	}
}

func TestConfigSetProjectWorkingDirRefusesAnUnknownProject(t *testing.T) {
	env, _ := savingEnv(testConfig())

	err := Run(context.Background(), []string{"config-set", "project.nope.working_dir", "/new/dir"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestConfigSetRefusesAnUnknownKey(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "bogus_key", "x"}, env)

	if err == nil || !strings.Contains(err.Error(), `unknown key "bogus_key"`) {
		t.Errorf("err = %v, want the unknown key named", err)
	}
}

func TestConfigSetRefusesWrongArgumentCount(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "poll_seconds"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly a key and a value") {
		t.Errorf("err = %v, want 'want exactly a key and a value'", err)
	}
}

func TestConfigSetReportsNoConfiguration(t *testing.T) {
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"config-set", "poll_seconds", "45"}, env)

	if err == nil || !strings.Contains(err.Error(), "no configuration yet") {
		t.Errorf("err = %v, want 'no configuration yet'", err)
	}
}

func TestConfigSetReportsAFailedLoad(t *testing.T) {
	want := errors.New("disk gone")
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, want }

	err := Run(context.Background(), []string{"config-set", "poll_seconds", "45"}, env)

	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestConfigSetReportsAFailedSave(t *testing.T) {
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	want := errors.New("disk full")
	env.Save = func(config.Config) error { return want }

	err := Run(context.Background(), []string{"config-set", "poll_seconds", "45"}, env)

	if err == nil || !strings.Contains(err.Error(), "save config") {
		t.Errorf("err = %v, want the failed save named", err)
	}
}

func TestConfigSetMarkdown(t *testing.T) {
	env, _ := savingEnv(config.Config{})
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{"config-set", "poll_seconds", "45"}, env); err != nil {
		t.Fatalf("config-set: %v", err)
	}
	want := "# Config updated\n\n- poll_seconds: 45\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestConfigSetMarkdownReportsUnset(t *testing.T) {
	env, _ := savingEnv(config.Config{PollSeconds: 45})
	var out strings.Builder
	env.Out = &out

	if err := Run(context.Background(), []string{"config-set", "poll_seconds", ""}, env); err != nil {
		t.Fatalf("config-set: %v", err)
	}
	want := "# Config updated\n\n- poll_seconds: unset\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestConfigSetRefusesAnUnknownFlag(t *testing.T) {
	env, _ := savingEnv(config.Config{})

	err := Run(context.Background(), []string{"config-set", "--bogus", "poll_seconds", "45"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}
