package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeFixGH answers a fix launch's gate with a pull request in one state (or
// a refusal) and its review snapshot with fixed text, stubbing the rest of
// [GH] through fakePRBase.
type fakeFixGH struct {
	fakePRBase
	state  string
	err    error
	viewed []string
}

func (f *fakeFixGH) ViewPR(dir, ref string) (gh.PR, error) {
	f.viewed = append(f.viewed, ref)
	return gh.PR{State: f.state}, f.err
}

func (f *fakeFixGH) ReviewComments(dir, ref string) (string, error) {
	return "reviewer: rename the helper", nil
}

const fixPR = "https://github.test/craig/nat/pull/7"

// fixLaunchEnv is a project whose one slice is approved — in progress, its
// pull request recorded — with gh answering for that pull request.
func fixLaunchEnv(t *testing.T, status string, fake *fakeFixGH) (Env, *fakeAPI, *agentTestRunner, *strings.Builder) {
	t.Helper()
	dir := t.TempDir()
	page := slicePageWithAllFields(testSliceID, "Write the UI", status, "m1", "slice/write-the-ui", "", fixPR, dir)
	api := &fakeAPI{pages: map[string][]notion.Page{"slices-ds": {page}}}
	env, _ := testEnv(testClaimConfig(t), api)
	runner := &agentTestRunner{}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.NewGit = func() GitCLI { return nil }
	env.NewWorktrees = func() actions.Worktrees { return nil }
	env.NewGH = func() GH { return fake }
	var out strings.Builder
	env.Out = &out
	return env, api, runner, &out
}

// slice-launch on an approved slice with its pull request open starts the fix
// prompt — the review gathered into it — claims nothing, files the Relaunched
// that says a fix is under way, and says "fix" in its JSON.
func TestSliceLaunchStartsAFixSessionOnAnApprovedSlice(t *testing.T) {
	fake := &fakeFixGH{state: "OPEN"}
	env, api, runner, out := fixLaunchEnv(t, notion.SliceInProgress, fake)

	if err := Run(context.Background(), []string{"slice-launch", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-launch: %v", err)
	}
	if len(fake.viewed) != 1 || fake.viewed[0] != fixPR {
		t.Errorf("gh read %v, want the recorded pull request asked about", fake.viewed)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing claimed", api.updates)
	}
	if len(api.appends) != 1 || !strings.Contains(blocksJSON(t, api.appends[0].children), notion.RelaunchedHeading) {
		t.Errorf("appends = %+v, want the one Relaunched", api.appends)
	}
	var got launchJSON
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil || !got.Fix {
		t.Errorf("json = %s (%v), want fix true", out.String(), err)
	}
	m := regexp.MustCompile(`\$\(cat '([^']+)'\)`).FindStringSubmatch(strings.Join(runner.launchArgs, " "))
	if m == nil {
		t.Fatalf("launch argv = %v, want the prompt file", runner.launchArgs)
	}
	prompt, err := os.ReadFile(m[1])
	if err != nil {
		t.Fatalf("read the prompt: %v", err)
	}
	for _, want := range []string{"working the review of one already-published", "reviewer: rename the helper", "nat complete-slice " + testSliceID} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("prompt does not say %q:\n%s", want, prompt)
		}
	}
}

// The markdown form says it was a fix session; a Done slice with its pull
// request still open (Done under the old rule) is one too, and its
// dependencies are not asked about.
func TestSliceLaunchFixMarkdown(t *testing.T) {
	env, _, _, out := fixLaunchEnv(t, notion.SliceDone, &fakeFixGH{state: "OPEN"})
	if err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-launch: %v", err)
	}
	if !strings.HasPrefix(out.String(), "# Launched a fix session\n") {
		t.Errorf("output = %q, want it to say a fix session", out.String())
	}
}

// A pull request merged, closed or unreadable refuses the fix launch with the
// board's own words, before any worktree is cut or anything is written.
func TestSliceLaunchRefusesAFixOnAPullRequestNotOpen(t *testing.T) {
	tests := []struct {
		name  string
		state string
		err   error
		want  string
	}{
		{"merged", gh.PRStateMerged, nil, "has already merged"},
		{"closed", gh.PRStateClosed, nil, "is closed"},
		{"unreadable", "", errors.New("rate limited"), "Could not read the pull request"},
	}
	for _, tt := range tests {
		env, api, runner, _ := fixLaunchEnv(t, notion.SliceInProgress, &fakeFixGH{state: tt.state, err: tt.err})
		err := Run(context.Background(), []string{"slice-launch", testSliceID, "--project", "project-1"}, env)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
		if len(api.updates) != 0 || len(api.appends) != 0 || len(runner.launchArgs) != 0 {
			t.Errorf("%s: updates %+v, appends %+v, launch %v — want nothing written or launched", tt.name, api.updates, api.appends, runner.launchArgs)
		}
	}
}

func blocksJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
