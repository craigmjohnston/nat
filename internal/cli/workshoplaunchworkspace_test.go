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

const testWorkspace = "9f8e7d6c-5b4a-4c3d-8e2f-1a0b9c8d7e6f"

// workspaceLaunchEnv is testEnv with the state directory and the prompt
// files' temp directory both pointed at directories of the test's own.
func workspaceLaunchEnv(t *testing.T, cfg config.Config, runner *agentTestRunner) (Env, *strings.Builder, string, string) {
	t.Helper()
	state, tmp := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", tmp)
	prev := stateDir
	stateDir = func() (string, error) { return state, nil }
	t.Cleanup(func() { stateDir = prev })

	env, _ := testEnv(cfg, &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var sb strings.Builder
	env.Out = &sb
	return env, &sb, state, tmp
}

func TestWorkshopLaunchWorkspaceLaunchesTheNewProjectPromptOnAScratchDir(t *testing.T) {
	cfg := testConfig(t)
	cfg.WorkshopAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	runner := &agentTestRunner{}
	env, out, state, tmp := workspaceLaunchEnv(t, cfg, runner)

	err := Run(context.Background(), []string{
		"workshop-launch", "--workspace", testWorkspace, "--request", "A habit tracker.",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch --workspace: %v", err)
	}

	workdir := filepath.Join(state, "workspaces", testWorkspace)
	if info, err := os.Stat(workdir); err != nil || !info.IsDir() {
		t.Errorf("scratch dir %s: %v, want it created", workdir, err)
	}
	want := "# Planning agent launched\n\n- Session: " + agent.PlanSessionName(testWorkspace) + "\n- Working directory: " + workdir + "\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	if want := agent.PlanTag(testWorkspace); len(runner.tagged) != 1 || runner.tagged[0] != want {
		t.Errorf("tagged panes = %v, want %q", runner.tagged, want)
	}
	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'sonnet'") || !strings.Contains(argv, "--effort 'low'") {
		t.Errorf("launch argv = %q, want the config's workshop_agent", argv)
	}

	matches, _ := filepath.Glob(filepath.Join(tmp, "nat-prompt-*", agent.PlanSessionName(testWorkspace)+".md"))
	if len(matches) != 1 {
		t.Fatalf("prompt files = %v, want exactly one", matches)
	}
	b, _ := os.ReadFile(matches[0])
	if want := agent.NewProjectPrompt(testWorkspace, "A habit tracker."); string(b) != want {
		t.Errorf("prompt = %q, want the new-project prompt seeded with the request", b)
	}
}

func TestWorkshopLaunchWorkspaceFlagsOverrideTheConfigsModel(t *testing.T) {
	cfg := testConfig(t)
	cfg.WorkshopAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	runner := &agentTestRunner{}
	env, _, _, _ := workspaceLaunchEnv(t, cfg, runner)

	err := Run(context.Background(), []string{
		"workshop-launch", "--workspace", testWorkspace, "--request", "x", "--model", "opus", "--effort", "high",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch --workspace: %v", err)
	}
	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'opus'") || !strings.Contains(argv, "--effort 'high'") {
		t.Errorf("launch argv = %q, want the flags to win", argv)
	}
}

func TestWorkshopLaunchWorkspaceJSON(t *testing.T) {
	env, out, state, _ := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{})

	err := Run(context.Background(), []string{
		"workshop-launch", "--json", "--workspace", testWorkspace, "--request", "x",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch --workspace: %v", err)
	}
	var got workshopLaunchJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output %q is not JSON: %v", out.String(), err)
	}
	if got.Session != agent.PlanSessionName(testWorkspace) || got.Workdir != filepath.Join(state, "workspaces", testWorkspace) {
		t.Errorf("output = %+v, want the workspace's own session and scratch dir", got)
	}
}

// A second press on the same tab must attach, not duplicate: the launch says a
// session is live and the app's poll finds it — the same refusal a project's
// planning agent gives.
func TestWorkshopLaunchWorkspaceRefusesItsOwnLiveSession(t *testing.T) {
	runner := &agentTestRunner{liveSessions: map[string]string{
		agent.PlanTag(testWorkspace): agent.PlanSessionName(testWorkspace),
	}}
	env, _, _, _ := workspaceLaunchEnv(t, testConfig(t), runner)

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "x"}, env)

	if err == nil || !strings.Contains(err.Error(), "already live: "+agent.PlanSessionName(testWorkspace)) {
		t.Errorf("err = %v, want the live session named", err)
	}
	if len(runner.tagged) != 0 {
		t.Errorf("tagged = %v, want no second launch", runner.tagged)
	}
}

// Another workspace's, a project's, and the legacy bare session are none of
// this tab's: it launches alongside all of them.
func TestWorkshopLaunchWorkspaceIgnoresEveryOtherPlanningAgent(t *testing.T) {
	runner := &agentTestRunner{liveSessions: map[string]string{
		agent.PlanTag("another-workspace"): "nat-plan-1",
		agent.PlanTag("project-1"):         "nat-plan-2",
		agent.PlanSentinel:                 "nat-plan",
	}}
	env, _, _, _ := workspaceLaunchEnv(t, testConfig(t), runner)

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "x"}, env)

	if err != nil {
		t.Fatalf("workshop-launch --workspace: %v", err)
	}
	if len(runner.tagged) != 1 {
		t.Errorf("tagged = %v, want one launch", runner.tagged)
	}
}

func TestWorkshopLaunchWorkspaceNeedsARequest(t *testing.T) {
	env, _, _, _ := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{})

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "  "}, env)

	if err == nil || !strings.Contains(err.Error(), "needs a --request") {
		t.Errorf("err = %v, want the missing request named", err)
	}
}

func TestWorkshopLaunchRefusesWorkspaceWithProject(t *testing.T) {
	env, _, _, _ := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{})

	err := Run(context.Background(), []string{
		"workshop-launch", "--workspace", testWorkspace, "--project", "project-1", "--request", "x",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("err = %v, want both flags refused", err)
	}
}

func TestWorkshopLaunchWorkspaceReportsAMissingStateDir(t *testing.T) {
	env, _, _, _ := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{})
	stateDir = func() (string, error) { return "", errors.New("no home directory") }

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "x"}, env)

	if err == nil || !strings.Contains(err.Error(), "no home directory") {
		t.Errorf("err = %v, want the state dir failure", err)
	}
}

func TestWorkshopLaunchWorkspaceReportsAScratchDirItCannotMake(t *testing.T) {
	env, _, state, _ := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{})
	// A file where the workspaces directory belongs.
	if err := os.WriteFile(filepath.Join(state, "workspaces"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "x"}, env)

	if err == nil || !strings.Contains(err.Error(), "create the scratch directory") {
		t.Errorf("err = %v, want the scratch dir failure", err)
	}
}

func TestWorkshopLaunchWorkspaceReportsAPromptItCannotWrite(t *testing.T) {
	env, _, _, tmp := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{})
	// TMPDIR is a file, so the prompt's directory cannot be made under it.
	file := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", file)

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "x"}, env)

	if err == nil || !strings.Contains(err.Error(), "launch planning agent") {
		t.Errorf("err = %v, want the prompt write failure", err)
	}
}

func TestWorkshopLaunchWorkspaceReportsAFailedLaunch(t *testing.T) {
	env, _, _, _ := workspaceLaunchEnv(t, testConfig(t), &agentTestRunner{launchErr: "duplicate session"})

	err := Run(context.Background(), []string{"workshop-launch", "--workspace", testWorkspace, "--request", "x"}, env)

	if err == nil || !strings.Contains(err.Error(), "duplicate session") {
		t.Errorf("err = %v, want the launch failure", err)
	}
}

func TestAgentKillWorkshopWorkspaceKillsOnlyItsOwnSession(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})
	runner := &agentTestRunner{liveSessions: map[string]string{
		agent.PlanTag(testWorkspace): "nat-plan-ws",
		agent.PlanTag("project-1"):   "nat-plan-p1",
		agent.PlanSentinel:           "nat-plan",
	}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"agent-kill", "--workshop", "--workspace", testWorkspace}, env)

	if err != nil {
		t.Fatalf("agent-kill --workshop --workspace: %v", err)
	}
	if len(runner.kills) != 1 || runner.kills[0] != "nat-plan-ws" {
		t.Errorf("kills = %v, want only the workspace's session", runner.kills)
	}
}

// The legacy bare session is a project's to attach, never a tab's to kill.
func TestAgentKillWorkshopWorkspaceNeverFallsBackToTheLegacySentinel(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})
	runner := &agentTestRunner{liveSessions: map[string]string{agent.PlanSentinel: "nat-plan"}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"agent-kill", "--workshop", "--workspace", testWorkspace}, env)

	if err == nil || !strings.Contains(err.Error(), "no live planning session") {
		t.Errorf("err = %v, want no live planning session", err)
	}
	if len(runner.kills) != 0 {
		t.Errorf("kills = %v, want none", runner.kills)
	}
}

func TestAgentKillRefusesWorkspaceWithoutWorkshop(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"agent-kill", testSliceID, "--workspace", testWorkspace}, env)

	if err == nil || !strings.Contains(err.Error(), "only names a planning agent") {
		t.Errorf("err = %v, want --workspace refused without --workshop", err)
	}
}

func TestAgentKillRefusesWorkspaceWithProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{
		"agent-kill", "--workshop", "--workspace", testWorkspace, "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("err = %v, want both flags refused", err)
	}
}
