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
	"github.com/craigmjohnston/nat/internal/notion"
)

func TestWorkshopLaunchRefusesAlreadyLive(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	runner := &agentTestRunner{liveSessions: map[string]string{agent.PlanSentinel: agent.PlanSession}}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "already live") {
		t.Errorf("err = %v, want 'already live'", err)
	}
}

func TestWorkshopLaunchesAPlainSession(t *testing.T) {
	api := &fakeAPI{}
	cfg := testConfig()
	cfg.WorkshopAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	env, out := testEnv(cfg, api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	want := "# Planning agent launched\n\n- Session: " + agent.PlanSession + "\n- Working directory: /tmp/nat\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'sonnet'") || !strings.Contains(argv, "--effort 'low'") {
		t.Errorf("launch argv = %q, want the config's workshop_agent", argv)
	}
	if len(runner.tagged) != 1 || runner.tagged[0] != agent.PlanSentinel {
		t.Errorf("tagged panes = %v, want the plan sentinel", runner.tagged)
	}
}

func TestWorkshopLaunchesOnTheWishlist(t *testing.T) {
	api := &fakeAPI{blocks: wishlistBlocks(t)}
	env, out := testEnv(testConfig(), api)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	if !strings.Contains(out.String(), "pending wishlist") {
		t.Errorf("output = %q, want it to say it launched on the wishlist", out.String())
	}
}

// launchedPlanPrompt reads back the prompt file the launch wrote — the test
// sets TMPDIR to dir, so the file is findable without threading the path out.
func launchedPlanPrompt(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "nat-prompt-*", agent.PlanSession+".md"))
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
	env, _ := testEnv(testConfig(), &fakeAPI{})
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

func TestWorkshopLaunchRequestOutranksThePendingWishlist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	api := &fakeAPI{blocks: wishlistBlocks(t)}
	env, out := testEnv(testConfig(), api)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{
		"workshop-launch", "--request", "Something else entirely.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("workshop-launch: %v", err)
	}
	if strings.Contains(out.String(), "pending wishlist") {
		t.Errorf("output = %q, a request should not launch on the wishlist", out.String())
	}
	prompt := launchedPlanPrompt(t, dir)
	if !strings.Contains(prompt, "Something else entirely.") || strings.Contains(prompt, "Add dark mode.") {
		t.Errorf("prompt = %q, want the request and not the wishlist", prompt)
	}
}

func TestWorkshopLaunchReadsTheRequestFromStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	env, _ := testEnv(testConfig(), &fakeAPI{})
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
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "--request", "-", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "--request - was given but there is nothing to read") {
		t.Errorf("err = %v, want the empty stdin named", err)
	}
}

// wishlistBlocks is a project page body carrying one pending wishlist item.
func wishlistBlocks(t *testing.T) []notion.Block {
	t.Helper()
	const raw = `[
		{"id":"h1","type":"heading_2","heading_2":{"rich_text":[{"plain_text":"Wishlist"}]}},
		{"id":"b1","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"plain_text":"Add dark mode."}]}}
	]`
	var blocks []notion.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks
}

func TestWorkshopLaunchModelFlagsOverrideConfig(t *testing.T) {
	cfg := testConfig()
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

func TestWorkshopLaunchJSON(t *testing.T) {
	env, out := testEnv(testConfig(), &fakeAPI{})
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("workshop-launch --json: %v", err)
	}
	var got workshopLaunchJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := workshopLaunchJSON{Session: agent.PlanSession, Workdir: "/tmp/nat"}
	if got != want {
		t.Errorf("json = %+v, want %+v", got, want)
	}
}

func TestWorkshopLaunchRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "extra", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Errorf("err = %v, want 'takes no arguments'", err)
	}
}

func TestWorkshopLaunchRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestWorkshopLaunchRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"workshop-launch", "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestWorkshopLaunchReportsAFailedPageRead(t *testing.T) {
	api := &fakeAPI{blocksErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(), api)
	// A fake tmux with nothing live, so the liveness check ahead of the read
	// answers for this test rather than for whatever the machine running it
	// happens to have launched.
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load project page") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestWorkshopLaunchReportsAFailedPromptFile(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))
	env, _ := testEnv(testConfig(), &fakeAPI{})
	// The fake tmux for the reason TestWorkshopLaunchReportsAFailedPageRead
	// carries one.
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "launch planning agent: create prompt dir") {
		t.Errorf("err = %v, want the failed prompt file named", err)
	}
}

func TestWorkshopLaunchReportsATmuxFailure(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	runner := &agentTestRunner{launchErr: "duplicate session"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"workshop-launch", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "duplicate session") {
		t.Errorf("err = %v, want tmux's own failure", err)
	}
}
