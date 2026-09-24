package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
)

// A bare pre-upgrade planning session belongs to no project, so it is the one
// any project would attach — and the one that refuses every project.
func TestWorkshopLaunchRefusesAlreadyLive(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	runner := &agentTestRunner{liveSessions: map[string]string{agent.PlanSentinel: agent.PlanSession}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "already live") {
		t.Errorf("err = %v, want 'already live'", err)
	}
}

// A planning agent on another project is no reason to refuse this one: they
// are scoped per project now, so two can be workshopped at once.
func TestWorkshopLaunchIgnoresAnotherProjectsPlanningAgent(t *testing.T) {
	env, out := testEnv(testConfig(t), &fakeAPI{})
	runner := &agentTestRunner{liveSessions: map[string]string{
		agent.PlanTag("project-2"): agent.PlanSessionName("project-2"),
	}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	if err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env); err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	if !strings.Contains(out.String(), agent.PlanSessionName("project-1")) {
		t.Errorf("output = %q, want this project's own session", out.String())
	}
}

// This project's own planning agent is what refuses.
func TestWorkshopLaunchRefusesThisProjectsPlanningAgent(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	runner := &agentTestRunner{liveSessions: map[string]string{
		agent.PlanTag("project-1"): agent.PlanSessionName("project-1"),
	}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), agent.PlanSessionName("project-1")) {
		t.Errorf("err = %v, want the project's own session named", err)
	}
}

func TestWorkshopLaunchesAPlainSession(t *testing.T) {
	api := &fakeAPI{}
	cfg := testConfig(t)
	cfg.WorkshopAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	env, out := testEnv(cfg, api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	want := "# Planning agent launched\n\n- Session: " + agent.PlanSessionName("project-1") + "\n- Working directory: /tmp/nat\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'sonnet'") || !strings.Contains(argv, "--effort 'low'") {
		t.Errorf("launch argv = %q, want the config's workshop_agent", argv)
	}
	if want := agent.PlanTag("project-1"); len(runner.tagged) != 1 || runner.tagged[0] != want {
		t.Errorf("tagged panes = %v, want %q", runner.tagged, want)
	}
}

// With no request the launch is a plain planning session, and no longer reads
// the project page to decide that: a page that cannot be read (the plan's
// inlined conventions are the one read left, and it fails soft) does not fail
// the launch, and the output carries only the session and its directory.
func TestWorkshopLaunchWithNoRequestIsAPlainSessionWhateverThePageSays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	api := &fakeAPI{blocksErr: errors.New("notion is down")}
	env, out := testEnv(testConfig(t), api)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output %q is not JSON: %v", out.String(), err)
	}
	if len(got) != 2 || got["session"] == nil || got["workdir"] == nil {
		t.Errorf("output = %v, want only session and workdir", got)
	}
	if prompt := launchedPlanPrompt(t, dir); !strings.Contains(prompt, "/queue-work") || strings.Contains(prompt, "## The request") {
		t.Errorf("prompt = %q, want a plain planning prompt with no request", prompt)
	}
}

func TestWorkshopLaunchMarkdownNamesOnlyTheSessionAndDirectory(t *testing.T) {
	got := workshopLaunchMarkdown("plan-x", "/w")
	if want := "# Planning agent launched\n\n- Session: plan-x\n- Working directory: /w\n"; got != want {
		t.Errorf("markdown = %q, want %q", got, want)
	}
}

// launchedPlanPrompt reads back the prompt file the launch wrote — the test
// sets TMPDIR to dir, so the file is findable without threading the path out.
func launchedPlanPrompt(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "nat-prompt-*", agent.PlanSessionName("project-1")+".md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("prompt files = %v (err %v), want exactly one", matches, err)
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWorkshopLaunchFoldsTheRequestIntoThePrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{
		"workshop-launch", "--request", "Add dark mode to the board.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	prompt := launchedPlanPrompt(t, dir)
	if !strings.Contains(prompt, "## The request") || !strings.Contains(prompt, "Add dark mode to the board.") {
		t.Errorf("prompt = %q, want the request folded in", prompt)
	}
}

// A hand-run workshop-launch with no --frontend claims nothing about where
// the user is; --frontend gnat carries that claim into the prompt, the same
// one gnat's own NatClient passes on every launch.
func TestWorkshopLaunchWritesTheFrontendIntoThePrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{
		"workshop-launch", "--project", "project-1", "--frontend", "gnat",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	prompt := launchedPlanPrompt(t, dir)
	if !strings.Contains(prompt, "The user is driving this from gnat, the macOS app.") {
		t.Errorf("prompt does not name gnat as the frontend:\n%s", prompt)
	}
}

func TestWorkshopLaunchWritesNoFrontendNoteWhenUnset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	if prompt := launchedPlanPrompt(t, dir); strings.Contains(prompt, "driving this from") {
		t.Errorf("prompt names a frontend for an unflagged launch:\n%s", prompt)
	}
}

// An invalid --frontend value is refused before a session is launched.
func TestWorkshopLaunchRefusesAnInvalidFrontend(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{
		"workshop-launch", "--project", "project-1", "--frontend", "web",
	}, env)
	if err == nil {
		t.Fatal("workshop-launch: expected error for an invalid --frontend value")
	}
	if !strings.Contains(err.Error(), "--frontend") {
		t.Errorf("workshop-launch error: %v, want it to name --frontend", err)
	}
}

func TestWorkshopLaunchReadsTheRequestFromStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	env.In = strings.NewReader("  A request too long for an argument.  \n")
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--request", "-", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	if !strings.Contains(launchedPlanPrompt(t, dir), "A request too long for an argument.") {
		t.Errorf("prompt should carry the stdin request, trimmed")
	}
}

func TestWorkshopLaunchRefusesStdinRequestWithNothingToRead(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "--request", "-", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "--request - was given but there is nothing to read") {
		t.Errorf("err = %v, want the empty stdin named", err)
	}
}

func TestWorkshopLaunchModelFlagsOverrideConfig(t *testing.T) {
	cfg := testConfig(t)
	cfg.WorkshopAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	env, _ := testEnv(cfg, &fakeAPI{})
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{
		"workshop-launch", "--model", "opus", "--effort", "high", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'opus'") || !strings.Contains(argv, "--effort 'high'") {
		t.Errorf("launch argv = %q, want the flags rather than the config", argv)
	}
}

// A headless launch pins Claude Code's theme to "auto", same as every other
// launch: there is no palette to guess ahead of time any more, only whether
// to ask Claude Code to find out for itself once something attaches.
func TestWorkshopLaunchCarriesTheAutoTheme(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}

	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, `--settings '{"theme":"auto","statusLine":{"type":"command"`) {
		t.Errorf("launch argv = %q, want the theme pinned to auto", argv)
	}
}

func TestWorkshopLaunchJSON(t *testing.T) {
	env, out := testEnv(testConfig(t), &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch --json: %v", err)
	}
	var got workshopLaunchJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := workshopLaunchJSON{Session: agent.PlanSessionName("project-1"), Workdir: "/tmp/nat"}
	if got != want {
		t.Errorf("json = %+v, want %+v", got, want)
	}
}

func TestWorkshopLaunchRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "extra", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Errorf("err = %v, want 'takes no arguments'", err)
	}
}

func TestWorkshopLaunchRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestWorkshopLaunchRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestWorkshopLaunchReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	// A fake tmux with nothing live, so the liveness check answers for this test
	// rather than for whatever the machine running it happens to have launched.
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "launch planning agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file named", err)
	}
}

func TestWorkshopLaunchReportsATmuxFailure(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	runner := &agentTestRunner{launchErr: "duplicate session"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "duplicate session") {
		t.Errorf("err = %v, want tmux's own failure", err)
	}
}
