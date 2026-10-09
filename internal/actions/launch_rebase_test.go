package actions

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/notion"
)

// rebaseLaunch relaunches slice s5, handed back on slice/info-view, against a
// repository that tests it conflicting and answers the rebase as r is set up
// to, and reports the context the prompt was written with.
func rebaseLaunch(t *testing.T, r *fakeRepo) agent.PromptContext {
	t.Helper()
	r.base, r.merge = "origin/main", git.MergeConflicted
	s := domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed}
	dir := repoDir(t)
	w := &fakeWorktrees{existing: map[string]string{"slice/info-view": dir + "-worktrees/slice/info-view"}}
	client := &fakeClient{blocks: func(id string) ([]notion.Block, error) {
		if id == "s5" {
			return handedBackBody(t), nil
		}
		return nil, nil
	}}
	res, err := Launch(context.Background(), &fakeLauncher{}, w, r, client.store(), nil, "u1",
		agent.PromptContext{Slice: s, WorkingDir: dir}, config.AgentModel{})
	if err != nil {
		t.Fatalf("Launch() = %v, want it to go through", err)
	}
	return res.Context
}

// The launch rebases a conflicted hand-back in its worktree before the agent
// starts, and the prompt is told how far it got; a rebase it cannot tell is
// not under way, or one that failed, is left to the agent.
func TestLaunchRebasesAConflictedHandBack(t *testing.T) {
	cases := []struct {
		name          string
		repo          fakeRepo
		want          agent.ConflictRebase
		wantPaths     []string
		wantRebaseRun bool
	}{
		{"clean", fakeRepo{}, agent.RebasedAtLaunch, nil, true},
		{"stopped", fakeRepo{rebaseConflicts: []string{"f"}}, agent.RebaseStoppedAtLaunch, []string{"f"}, true},
		{"failed", fakeRepo{rebaseErr: errors.New("hook failed")}, agent.RebaseLeftToAgent, nil, true},
		{"under way", fakeRepo{underWay: true, underWayConflicts: []string{"g"}}, agent.RebaseUnderWay, []string{"g"}, false},
		{"unreadable", fakeRepo{underWayErr: errors.New("no git dir")}, agent.RebaseLeftToAgent, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.repo
			c := rebaseLaunch(t, &r)
			if c.ConflictBase != "origin/main" || c.ConflictRebase != tc.want || !reflect.DeepEqual(c.ConflictPaths, tc.wantPaths) {
				t.Errorf("context base %q, rebase %v, paths %v — want origin/main, %v, %v",
					c.ConflictBase, c.ConflictRebase, c.ConflictPaths, tc.want, tc.wantPaths)
			}
			if ran := len(r.rebased) == 1 && r.rebased[0] == c.WorkingDir+"|origin/main"; ran != tc.wantRebaseRun {
				t.Errorf("rebased %v, want a rebase onto the base in the worktree: %v", r.rebased, tc.wantRebaseRun)
			}
		})
	}
}

// launchRepo is a real repository whose main and slice/info-view (checked out
// in a linked worktree, as a launch places an agent) have each changed f since
// they parted. The branch's commits are its args, each f's whole content; main
// then commits mainF. It answers the repository, the worktree and a git runner.
func launchRepo(t *testing.T, branchF []string, mainF string) (repo, wt string, run func(dir string, args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, wt = filepath.Join(tmp, "nat"), filepath.Join(tmp, "nat-worktrees", "info-view")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	run = func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commitF := func(dir, content, msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		run(dir, "commit", "-qam", msg)
	}
	run(repo, "init", "-q", "-b", "main")
	run(repo, "config", "user.name", "nat")
	run(repo, "config", "user.email", "nat@example.test")
	run(repo, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(repo, "add", "f")
	run(repo, "commit", "-qm", "base")
	run(repo, "worktree", "add", "-q", "-b", "slice/info-view", wt)
	for _, content := range branchF {
		commitF(wt, content, "branch "+strings.TrimSpace(content))
	}
	commitF(repo, mainF, "main "+strings.TrimSpace(mainF))
	return repo, wt, run
}

// realRebaseLaunch relaunches the handed-back slice/info-view on the real git,
// and answers the prompt it was launched with.
func realRebaseLaunch(t *testing.T, repo, wt string) (agent.PromptContext, string) {
	t.Helper()
	s := domain.Slice{ID: "s5", Name: "Info view", Branch: "slice/info-view", Status: domain.SliceClaimed}
	client := &fakeClient{blocks: func(id string) ([]notion.Block, error) {
		if id == "s5" {
			return handedBackBody(t), nil
		}
		return nil, nil
	}}
	l := &fakeLauncher{}
	res, err := Launch(context.Background(), l, &fakeWorktrees{existing: map[string]string{"slice/info-view": wt}},
		git.New(), client.store(), nil, "u1", agent.PromptContext{Slice: s, WorkingDir: repo}, config.AgentModel{})
	if err != nil || len(l.launches) != 1 {
		t.Fatalf("Launch() = %v with %d launches, want one", err, len(l.launches))
	}
	prompt, err := os.ReadFile(l.launches[0].promptFile)
	if err != nil {
		t.Fatal(err)
	}
	return res.Context, string(prompt)
}

// rebaseUnderWay says whether the worktree is stopped part way through a
// rebase, by asking git itself.
func rebaseUnderWay(t *testing.T, wt string, run func(string, ...string) string) bool {
	t.Helper()
	_, err := os.Stat(run(wt, "rev-parse", "--path-format=absolute", "--git-path", "rebase-merge"))
	return err == nil
}

// On the real git, a branch that conflicts with main launches with its rebase
// stopped on the conflict and the conflicted file in the prompt.
func TestLaunchStopsARealRebaseOnItsConflict(t *testing.T) {
	repo, wt, run := launchRepo(t, []string{"b\n"}, "c\n")
	c, prompt := realRebaseLaunch(t, repo, wt)
	if c.ConflictBase != "main" || c.ConflictRebase != agent.RebaseStoppedAtLaunch {
		t.Fatalf("context base %q, rebase %v — want main, stopped", c.ConflictBase, c.ConflictRebase)
	}
	if !rebaseUnderWay(t, wt, run) {
		t.Error("the worktree has no rebase stopped in it")
	}
	if !strings.Contains(prompt, "is stopped on the first commit that conflicts, with these files conflicted:\n\n- `f`\n") {
		t.Errorf("prompt does not list the conflicted file:\n%s", prompt)
	}
}

// On the real git, a branch whose merge into main conflicts but whose rebase
// does not — its first commit already on main as a squash, its second building
// on it — launches rebased, with only the gate and the hand-back asked for.
func TestLaunchRebasesARealBranchCleanly(t *testing.T) {
	repo, wt, run := launchRepo(t, []string{"b\n", "c\n"}, "b\n")
	c, prompt := realRebaseLaunch(t, repo, wt)
	if c.ConflictBase != "main" || c.ConflictRebase != agent.RebasedAtLaunch {
		t.Fatalf("context base %q, rebase %v — want main, rebased", c.ConflictBase, c.ConflictRebase)
	}
	if rebaseUnderWay(t, wt, run) {
		t.Error("the worktree is still part way through a rebase")
	}
	if run(wt, "merge-base", "main", "HEAD") != run(repo, "rev-parse", "main") {
		t.Error("the branch is not on main's tip")
	}
	if !strings.Contains(prompt, "the\nrebase went through with no conflict.") {
		t.Errorf("prompt does not say the branch was rebased:\n%s", prompt)
	}
}

// On the real git, a rebase already stopped in the worktree is left as it
// is: the launch starts none, and the prompt says one is under way.
func TestLaunchLeavesARealRebaseUnderWay(t *testing.T) {
	repo, wt, run := launchRepo(t, []string{"b\n"}, "c\n")
	if out, err := exec.Command("git", "-C", wt, "rebase", "main").CombinedOutput(); err == nil {
		t.Fatalf("the rebase went through, want it stopped:\n%s", out)
	}
	head := run(wt, "rev-parse", "HEAD")
	c, prompt := realRebaseLaunch(t, repo, wt)
	if c.ConflictRebase != agent.RebaseUnderWay || !reflect.DeepEqual(c.ConflictPaths, []string{"f"}) {
		t.Fatalf("context rebase %v, paths %v — want under way, [f]", c.ConflictRebase, c.ConflictPaths)
	}
	if !rebaseUnderWay(t, wt, run) || run(wt, "rev-parse", "HEAD") != head {
		t.Error("the rebase under way was not left as it was")
	}
	if !strings.Contains(prompt, "A rebase is already under way in the worktree") {
		t.Errorf("prompt does not say a rebase is under way:\n%s", prompt)
	}
}
