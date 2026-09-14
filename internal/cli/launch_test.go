package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

func TestSliceLaunchRefusesDone(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceDone, "m1", "", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(t), api)
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
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})
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
	env, _ := testEnv(testClaimConfig(t), api)
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
	env, _ := testEnv(testClaimConfig(t), api)
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
	cfg := testClaimConfig(t)
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
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestSliceLaunchRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-launch", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestSliceLaunchRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestSliceLaunchReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testClaimConfig(t), api)

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

// A plan that will not read costs the launch its milestone digest rather
// than the launch itself: the agent still starts, with no digest section
// filled in.
// The plan read this used to survive — falling back to no milestone digest —
// is now the same read [store.ForProject] makes to hydrate the local plan in
// the first place, before slice-launch reads the slice at all: a workspace
// that will not answer it fails the whole command, not just the digest.
func TestSliceLaunchFailsWhenThePlanCannotBeHydrated(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{
		pages:    map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}},
		queryErr: map[string]error{"slices-ds": errors.New("notion: 500")},
	}
	env, _ := testEnv(testClaimConfig(t), api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "notion: 500") {
		t.Fatalf("err = %v, want the failed hydrate reported", err)
	}
	if len(runner.launchArgs) != 0 {
		t.Error("want no agent launched: the plan could not even be read")
	}
}

// The plan is read a second time, after the slice itself, purely for the
// milestone digest — a local read with nothing to do with the hydrate above.
// A plan that cannot answer that second read costs the launch only its
// digest, not the launch itself: the agent still starts, with no digest
// section filled in.
func TestSliceLaunchGoesAheadWithoutADigestWhenThatSecondPlanReadFails(t *testing.T) {
	dir := t.TempDir()
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Write the UI", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET repo = ? WHERE id = ?`, dir, testSliceID); err != nil {
			t.Fatalf("seed the repo: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE milestones`); err != nil {
			t.Fatalf("break the plan's milestones table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	runner := &agentTestRunner{launchPane: "%9"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("slice-launch: %v, want it to launch anyway with no digest", err)
	}
	if len(runner.launchArgs) == 0 {
		t.Error("want the agent launched despite the failed digest read")
	}
}

// A launch that claims nothing and starts nothing — here, a claim whose
// local write itself fails — reports itself as the toast [actions.Launch]
// already built, the same way the board would show it, rather than a bare
// "no error" success.
func TestSliceLaunchReportsALaunchThatClaimedNothing(t *testing.T) {
	dir := t.TempDir()
	cfg := testClaimConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Write the UI", "Todo", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET repo = ? WHERE id = ?`, dir, testSliceID); err != nil {
			t.Fatalf("seed the repo: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	runner := &agentTestRunner{launchPane: "%9"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)

	if err == nil {
		t.Fatal("slice-launch over a plan that cannot claim the slice: want an error")
	}
	if len(runner.launchArgs) != 0 {
		t.Error("want no agent launched: the claim never landed")
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
	cfg := testClaimConfig(t)
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

// The bug this pins down: slice-launch built its PromptContext from the
// project's ID, the slice and the working directory alone, so the prompt told
// the agent it was working a slice of the "" project and that start-slice
// "claims the slice for  and prints your brief". The board's own launch fills
// in the project and the assignee, and the headless launch must hand
// [agent.Prompt] — whose output the golden files in internal/agent/testdata
// pin — the same context.
func TestSliceLaunchWritesTheFullPromptContext(t *testing.T) {
	dir := t.TempDir()
	page := slicePageForLaunch(dir)
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {page}}}
	cfg := testClaimConfig(t)
	env, _ := testEnv(cfg, api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("slice-launch: %v", err)
	}

	argv := strings.Join(runner.launchArgs, " ")
	m := regexp.MustCompile(`\$\(cat '([^']+)'\)`).FindStringSubmatch(argv)
	if m == nil {
		t.Fatalf("launch argv = %q, want the prompt file read back with $(cat ...)", argv)
	}
	prompt, readErr := os.ReadFile(m[1])
	if readErr != nil {
		t.Fatalf("read the prompt file: %v", readErr)
	}

	want := agent.Prompt(agent.PromptContext{
		Slice:        domain.SliceFromPage(page),
		Project:      cfg.Projects["project-1"],
		ProjectID:    "project-1",
		WorkingDir:   dir,
		AssigneeName: cfg.AssigneeUserName,
	})
	if string(prompt) != want {
		t.Errorf("prompt =\n%s\nwant\n%s", prompt, want)
	}
}

// A hand-run slice-launch with no --frontend claims nothing about where the
// user is; --frontend gnat carries that claim straight into the prompt, the
// same one gnat's own NatClient passes on every launch.
func TestSliceLaunchWritesTheFrontendIntoThePrompt(t *testing.T) {
	dir := t.TempDir()
	page := slicePageForLaunch(dir)
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {page}}}
	cfg := testClaimConfig(t)
	env, _ := testEnv(cfg, api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--project", "project-1", "--frontend", "gnat",
	}, env)
	if err != nil {
		t.Fatalf("slice-launch: %v", err)
	}

	argv := strings.Join(runner.launchArgs, " ")
	m := regexp.MustCompile(`\$\(cat '([^']+)'\)`).FindStringSubmatch(argv)
	if m == nil {
		t.Fatalf("launch argv = %q, want the prompt file read back with $(cat ...)", argv)
	}
	prompt, readErr := os.ReadFile(m[1])
	if readErr != nil {
		t.Fatalf("read the prompt file: %v", readErr)
	}
	if !strings.Contains(string(prompt), "The user is driving this from gnat, the macOS app.") {
		t.Errorf("prompt does not name gnat as the frontend:\n%s", prompt)
	}
}

// An invalid --frontend value is refused before anything is claimed or
// launched, rather than silently ignored or read as unspecified.
func TestSliceLaunchRefusesAnInvalidFrontend(t *testing.T) {
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
	}}
	env, _ := testEnv(testClaimConfig(t), api)
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"slice-launch", testSliceID, "--project", "project-1", "--frontend", "web",
	}, env)
	if err == nil {
		t.Fatal("slice-launch: expected error for an invalid --frontend value")
	}
	if !strings.Contains(err.Error(), "--frontend") {
		t.Errorf("slice-launch error: %v, want it to name --frontend", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing claimed for a refused launch", api.updates)
	}
}

func TestSliceLaunchModelFlagsOverrideConfig(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	cfg := testClaimConfig(t)
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

// A headless launch pins Claude Code's theme to "auto", same as every other
// launch: there is no palette to guess ahead of time any more, only whether
// to ask Claude Code to find out for itself once something attaches.
func TestSliceLaunchCarriesTheAutoTheme(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	env, _ := testEnv(testClaimConfig(t), api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("slice-launch: %v", err)
	}

	argv := strings.Join(runner.launchArgs, " ")
	if !strings.Contains(argv, `--settings '{"theme":"auto"}'`) {
		t.Errorf("launch argv = %q, want the theme pinned to auto", argv)
	}
}

func TestSliceLaunchJSON(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	env, _ := testEnv(testClaimConfig(t), api)
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

// A claim's push to the workspace failing no longer stops a launch: the claim
// lands in the local plan first, and that write is what claiming answers for
// now. The slice is left dirty for a later sync to send what the push could
// not.
func TestSliceLaunchClaimsLocallyAndLaunchesEvenWhenThePushFails(t *testing.T) {
	dir := t.TempDir()
	cfg := testClaimConfig(t)
	api := &fakeAPI{
		pages:     map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}},
		updateErr: errors.New("notion is down"),
	}
	env, _ := testEnv(cfg, api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }

	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)

	if err != nil {
		t.Fatalf("slice-launch: %v, want it to succeed: the claim landed locally even though "+
			"the push to the workspace failed", err)
	}
	if len(runner.launchArgs) == 0 {
		t.Error("want the agent launched: the claim landed locally")
	}

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	local, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() {
		if err := local.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	}()
	dirty, err := local.Dirty(context.Background(), testSliceID)
	if err != nil {
		t.Fatalf("read whether the slice is dirty: %v", err)
	}
	if !dirty {
		t.Error("slice not marked dirty, want the failed push left for a later sync to send")
	}
}

func TestSliceLaunchReportsATmuxFailure(t *testing.T) {
	dir := t.TempDir()
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {slicePageForLaunch(dir)}}}
	env, _ := testEnv(testClaimConfig(t), api)
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
