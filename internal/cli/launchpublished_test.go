package cli

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeReviewGH answers a launch's review snapshot with fixed text, stubbing
// the rest of [GH] through fakePRBase.
type fakeReviewGH struct{ fakePRBase }

func (f *fakeReviewGH) ReviewComments(dir, ref string) (string, error) {
	return "reviewer: rename the helper", nil
}

const publishedPR = "https://github.test/craig/nat/pull/7"

// publishedLaunchEnv is a project whose one slice is approved and resumed —
// its pull request recorded, its branch cleared — at status, with gh
// answering for that pull request.
func publishedLaunchEnv(t *testing.T, status string) (Env, *fakeAPI, *agentTestRunner, *strings.Builder) {
	t.Helper()
	dir := t.TempDir()
	page := slicePageWithAllFields(testSliceID, "Write the UI", status, "m1", "", "", publishedPR, dir)
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {page}}}
	env, _ := testEnv(testClaimConfig(t), api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGH = func() GH { return &fakeReviewGH{} }
	var out strings.Builder
	env.Out = &out
	return env, api, runner, &out
}

// slice-launch on a slice in progress with its pull request recorded is an
// ordinary relaunch: the slice is claimed like any other, and the prompt tells
// the agent its pull request is open, with the review gathered into it.
func TestSliceLaunchRelaunchesAPublishedSlice(t *testing.T) {
	env, api, runner, out := publishedLaunchEnv(t, notion.SliceInProgress)

	if err := Run(context.Background(), []string{"slice-launch", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-launch: %v", err)
	}
	if len(api.updates) != 1 {
		t.Errorf("updates = %+v, want the claim", api.updates)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("json = %s: %v", out.String(), err)
	}
	if _, ok := got["fix"]; ok {
		t.Errorf("json = %s, want no fix key", out.String())
	}
	m := regexp.MustCompile(`\$\(cat '([^']+)'\)`).FindStringSubmatch(strings.Join(runner.launchArgs, " "))
	if m == nil {
		t.Fatalf("launch argv = %v, want the prompt file", runner.launchArgs)
	}
	prompt, err := os.ReadFile(m[1])
	if err != nil {
		t.Fatalf("read the prompt: %v", err)
	}
	for _, want := range []string{"## The pull request", publishedPR, "reviewer: rename the helper", "nat complete-slice " + testSliceID} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("prompt does not say %q:\n%s", want, prompt)
		}
	}
}

// A Done slice is not launched, pull request or not: its work is merged, and
// nothing is written or started.
func TestSliceLaunchRefusesADoneSlice(t *testing.T) {
	env, api, runner, _ := publishedLaunchEnv(t, notion.SliceDone)
	err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), `"Write the UI" is Done`) {
		t.Errorf("err = %v, want a refusal by name", err)
	}
	if len(api.updates) != 0 || len(api.appends) != 0 || len(runner.launchArgs) != 0 {
		t.Errorf("updates %+v, appends %+v, launch %v — want nothing written or launched", api.updates, api.appends, runner.launchArgs)
	}
}
