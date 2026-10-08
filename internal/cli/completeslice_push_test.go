package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

// pushEnv is a completable slice with a worktree, whose git answers with
// runner: the worktree is on slice/render-the-board unless the test says
// otherwise.
func pushEnv(t *testing.T, runner *fakeGitRunner) (*fakeAPI, Env, *strings.Builder) {
	t.Helper()
	api := completableAPI()
	env, out := completeEnv(t, api)
	withWorktree(&env)
	if runner.base == "" {
		runner.base = "slice/render-the-board"
	}
	env.NewGit = func() GitCLI { return git.NewWithRunner(runner) }
	return api, env, out
}

// The hand-back an agent ends on now: no --branch, a clean worktree. The
// branch is read off the worktree, pushed once with the lease, and recorded.
func TestCompleteSlicePushesTheWorktreesBranch(t *testing.T) {
	runner := &fakeGitRunner{}
	api, env, out := pushEnv(t, runner)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Wrote the renderer.",
		"--pr-description", "Render the board", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}
	want := []pushCall{{dir: "/tmp/nat.worktrees/x",
		args: []string{"push", "--force-with-lease", "-u", "origin", "slice/render-the-board"}}}
	if !reflect.DeepEqual(runner.pushes, want) {
		t.Errorf("pushes = %+v, want exactly %+v", runner.pushes, want)
	}
	if len(api.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one", api.updates)
	}
	if spans := api.updates[0].props[notion.PropBranch].RichText; len(spans) != 1 ||
		spans[0].Text == nil || spans[0].Text.Content != "slice/render-the-board" {
		t.Errorf("branch = %+v, want the worktree's", spans)
	}
	if !strings.Contains(out.String(), "- Branch: slice/render-the-board") {
		t.Errorf("output = %q, want the branch named", out.String())
	}
}

// A dirty worktree is not a hand-back: refused naming every path, with
// nothing pushed and nothing written.
func TestCompleteSliceRefusesADirtyWorktree(t *testing.T) {
	runner := &fakeGitRunner{statusOut: " M internal/a.go\n?? notes.txt\n"}
	api, env, out := pushEnv(t, runner)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Wrote the renderer.", "--project", "project-1",
	}, env)
	if err == nil {
		t.Fatal("complete-slice: want a refusal")
	}
	for _, path := range []string{"internal/a.go", "notes.txt"} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("err = %q, want it naming %s", err, path)
		}
	}
	assertNothingHandedBack(t, api, runner, out)
}

// A push git refuses is the command's error, all of git's output in it, and
// the page is left as it was.
func TestCompleteSliceReportsARefusedPush(t *testing.T) {
	stderr := "To github.com:x/y.git\n ! [rejected]        slice/render-the-board (stale info)\nerror: failed to push some refs\n"
	runner := &fakeGitRunner{pushErr: &git.ExitError{Code: 1, Stderr: stderr}}
	api, env, out := pushEnv(t, runner)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Wrote the renderer.", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "(stale info)") || !strings.Contains(err.Error(), "failed to push some refs") {
		t.Fatalf("err = %v, want git's whole output", err)
	}
	if len(runner.pushes) != 1 {
		t.Errorf("pushes = %+v, want the one attempt", runner.pushes)
	}
	if len(api.appends) != 0 || len(api.updates) != 0 || out.Len() != 0 {
		t.Errorf("writes %+v %+v, output %q; want none", api.appends, api.updates, out.String())
	}
}

// A slice with no worktree and no --branch names nothing to hand back: it is
// refused, pointing at both flags, rather than closed Done.
func TestCompleteSliceWithNoWorktreeNeedsABranchOrNoBranch(t *testing.T) {
	runner := &fakeGitRunner{}
	api := completableAPI()
	env, out := completeEnv(t, api)
	env.NewGit = func() GitCLI { return git.NewWithRunner(runner) }

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Wrote the docs.", "--project", "project-1",
	}, env)
	var usage *UsageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "--branch") || !strings.Contains(err.Error(), "--no-branch") {
		t.Fatalf("err = %v, want a usage error naming --branch and --no-branch", err)
	}
	assertNothingHandedBack(t, api, runner, out)
}

// Blocked work reports a stop, not a result: no dirty check, no push.
func TestCompleteSliceBlockedNeitherChecksNorPushes(t *testing.T) {
	runner := &fakeGitRunner{statusOut: " M internal/a.go\n"}
	api, env, _ := pushEnv(t, runner)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--blocked", "--summary", "Stuck.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}
	if len(runner.pushes) != 0 {
		t.Errorf("pushes = %+v, want none", runner.pushes)
	}
	if len(api.appends) != 1 {
		t.Errorf("appends = %+v, want the blocked note", api.appends)
	}
}

// A project with no Branch column cannot hold the hand-back an omitted
// --branch means, so it is refused before anything is pushed.
func TestCompleteSliceRefusesAHandBackWithNoColumnBeforePushing(t *testing.T) {
	runner := &fakeGitRunner{}
	api, env, out := pushEnv(t, runner)
	ds := assigneeSlicesDS()
	ds.Properties[notion.PropBranch] = notion.PropertySchema{Type: "url"}
	api.dataSources = map[string]notion.DataSource{"slices-ds": ds}

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Done.", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "--no-branch") {
		t.Fatalf("err = %v, want --no-branch offered", err)
	}
	assertNothingHandedBack(t, api, runner, out)
}

func assertNothingHandedBack(t *testing.T, api *fakeAPI, runner *fakeGitRunner, out *strings.Builder) {
	t.Helper()
	if len(runner.pushes) != 0 {
		t.Errorf("pushes = %+v, want none", runner.pushes)
	}
	if len(api.appends) != 0 || len(api.updates) != 0 {
		t.Errorf("writes = %+v %+v, want none", api.appends, api.updates)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want nothing", out.String())
	}
}
