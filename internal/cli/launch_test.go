package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

func TestSliceLaunchRefusesDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceDone, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-launch: expected error for Done slice")
	}
	if !strings.Contains(err.Error(), "Done") {
		t.Errorf("slice-launch error: %v, want 'Done'", err)
	}
}

func TestSliceLaunchRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-launch: expected error for missing slice")
	}
	if !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("slice-launch error: %v, want 'want exactly one'", err)
	}
}

func TestSliceLaunchRefusesBlocked(t *testing.T) {
	api := &fakeAPI{
		dataSources: map[string]notion.DataSource{
			"slices-ds": assigneeSlicesDS("m1"),
		},
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageWithBranch(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "other-slice"),
				slicePageWithBranch("other-slice", "Set up", notion.SliceTodo, "m1", ""),
			},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-launch: expected error for blocked slice")
	}
	if !strings.Contains(err.Error(), "waits on") {
		t.Errorf("slice-launch error: %v, want 'waits on'", err)
	}
}

func TestSliceLaunchRefusesAlreadyRunning(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{testSliceID: "nat-abcd1234"},
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-launch: expected error for already running slice")
	}
	if !strings.Contains(err.Error(), "already has a live session") {
		t.Errorf("slice-launch error: %v, want 'already has a live session'", err)
	}
}

func TestSliceLaunchRefusesNoAssignee(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
		},
	}
	cfg := testClaimConfig()
	cfg.AssigneeUserID = ""
	env, _ := testEnv(cfg, api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("slice-launch: expected error for missing assignee")
	}
	if !strings.Contains(err.Error(), "no assignee") {
		t.Errorf("slice-launch error: %v, want 'no assignee'", err)
	}
}

func TestSliceLaunchRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceLaunchRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-launch", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceLaunchRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceLaunchReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testClaimConfig(), api)

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

// slicePageForLaunch is a Todo slice with no dependencies, filed under a
// repo the test names directly — an empty directory rather than a git
// repository, so the launch falls back to the shared checkout and neither a
// Worktrees nor a Repo fake has to drive an actual worktree cut.
func slicePageForLaunch(dir string) notion.Page {
	return slicePageWithAllFields(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "", "", dir)
}

func TestSliceLaunchClaimsAndLaunches(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	cfg := testClaimConfig()
	cfg.SliceAgent = config.AgentModel{Model: "sonnet", Effort: "high"}
	env, _ := testEnv(cfg, api)
	runner := &agentTestRunner{launchPane: "%9"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	nudged := false
	env.Nudge = func() { nudged = true }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("slice-launch: %v", err)
	}

	wantSession := agent.SessionName(testSliceID)
	wantToast := dir + " is not a git repository — the agent runs in the shared checkout."
	want := fmt.Sprintf("# Launched\n\n- Session: %s\n- Working directory: %s\n- Branch: \n\nWarning: %s\n",
		wantSession, dir, wantToast)
	if out.String() != want {
		t.Errorf("output =\n%q\nwant\n%q", out.String(), want)
	}
	if !nudged {
		t.Error("want the board nudged once the claim landed")
	}
	if len(api.updates) != 1 || api.updates[0].id != testSliceID {
		t.Fatalf("updates = %+v, want the slice claimed", api.updates)
	}
	if status := api.updates[0].props[notion.PropStatus]; status.Select == nil || status.Select.Name != notion.SliceInProgress {
		t.Errorf("status = %+v, want In progress", status)
	}
	if len(runner.tagged) != 1 || runner.tagged[0] != testSliceID {
		t.Errorf("tagged panes = %v, want the slice tagged on its own", runner.tagged)
	}
	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'sonnet'") || !strings.Contains(argv, "--effort 'high'") {
		t.Errorf("launch argv = %q, want the config's slice_agent", argv)
	}
}

func TestSliceLaunchModelFlagsOverrideConfig(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	cfg := testClaimConfig()
	cfg.SliceAgent = config.AgentModel{Model: "sonnet", Effort: "low"}
	env, _ := testEnv(cfg, api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--model", "opus", "--effort", "xhigh", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("slice-launch: %v", err)
	}

	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, "--model 'opus'") || !strings.Contains(argv, "--effort 'xhigh'") {
		t.Errorf("launch argv = %q, want the flags rather than the config", argv)
	}
	if strings.Contains(argv, "sonnet") || strings.Contains(argv, "low") {
		t.Errorf("launch argv = %q, want none of the config's pair", argv)
	}
}

func TestSliceLaunchJSON(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("slice-launch --json: %v", err)
	}

	var got launchJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	want := launchJSON{
		Session: agent.SessionName(testSliceID),
		Workdir: dir,
		Warning: dir + " is not a git repository — the agent runs in the shared checkout.",
	}
	if got != want {
		t.Errorf("json = %+v\nwant %+v", got, want)
	}
}

func TestSliceLaunchReportsAClaimFailure(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{
		pages:     map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}},
		updateErr: errors.New("notion is down"),
	}
	env, _ := testEnv(testClaimConfig(), api)
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(&agentTestRunner{}) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)

	want := `Could not claim "Write the UI": notion is down — no agent was launched.`
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestSliceLaunchReportsATmuxFailure(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{launchErr: "duplicate session"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "duplicate session") {
		t.Errorf("err = %v, want tmux's own failure", err)
	}
	if len(api.updates) != 1 {
		t.Errorf("updates = %+v, want the claim to have landed before tmux refused", api.updates)
	}
}
