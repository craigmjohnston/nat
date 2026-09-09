package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

func fullConfig() config.Config {
	return config.Config{
		AgentSplitPercent: 70,
		PollSeconds:       45,
		WorkshopAgent:     config.AgentModel{Model: "sonnet", Effort: "low"},
		SliceAgent:        config.AgentModel{Model: "opus", Effort: "high"},
		Projects: map[string]config.ProjectConfig{
			"project-1": {Name: "nat", SlicesDSID: "slices-ds", WorkingDir: "/tmp/nat"},
		},
	}
}

func TestConfigShowMarkdown(t *testing.T) {
	env, out := testEnv(fullConfig(), &fakeAPI{})

	if err := Run(context.Background(), []string{"config-show"}, env); err != nil {
		t.Fatalf("config-show: %v", err)
	}
	for _, want := range []string{
		"Agent split percent: 70", "Poll seconds: 45",
		`Workshop agent: model="sonnet" effort="low"`,
		`Slice agent: model="opus" effort="high"`,
		`project-1 (nat): working_dir="/tmp/nat"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestConfigShowMarkdownWithNoProjects(t *testing.T) {
	env, out := testEnv(config.Config{}, &fakeAPI{})

	if err := Run(context.Background(), []string{"config-show"}, env); err != nil {
		t.Fatalf("config-show: %v", err)
	}
	if !strings.Contains(out.String(), "_none_") {
		t.Errorf("output = %q, want it to say there are no projects", out.String())
	}
}

func TestConfigShowJSON(t *testing.T) {
	env, out := testEnv(fullConfig(), &fakeAPI{})

	if err := Run(context.Background(), []string{"config-show", "--json"}, env); err != nil {
		t.Fatalf("config-show --json: %v", err)
	}
	var got configDoc
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := configDoc{
		AgentSplitPercent: 70,
		PollSeconds:       45,
		WorkshopAgent:     agentModelJSON{Model: "sonnet", Effort: "low"},
		SliceAgent:        agentModelJSON{Model: "opus", Effort: "high"},
		Projects: map[string]configProjectJSON{
			"project-1": {Name: "nat", WorkingDir: "/tmp/nat"},
		},
	}
	if got.AgentSplitPercent != want.AgentSplitPercent || got.PollSeconds != want.PollSeconds ||
		got.WorkshopAgent != want.WorkshopAgent || got.SliceAgent != want.SliceAgent ||
		len(got.Projects) != 1 || got.Projects["project-1"] != want.Projects["project-1"] {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

func TestConfigShowRefusesArguments(t *testing.T) {
	env, _ := testEnv(fullConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"config-show", "extra"}, env)

	if err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Errorf("err = %v, want 'takes no arguments'", err)
	}
}

func TestConfigShowRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(fullConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"config-show", "--bogus"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestConfigShowReportsNoConfiguration(t *testing.T) {
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, nil }

	err := Run(context.Background(), []string{"config-show"}, env)

	if err == nil || !strings.Contains(err.Error(), "no configuration yet") {
		t.Errorf("err = %v, want 'no configuration yet'", err)
	}
}

func TestConfigShowReportsAFailedLoad(t *testing.T) {
	want := errors.New("disk gone")
	env, _ := testEnv(config.Config{}, &fakeAPI{})
	env.Load = func() (config.Config, bool, error) { return config.Config{}, false, want }

	err := Run(context.Background(), []string{"config-show"}, env)

	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
