package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/git"
	"github.com/craigmjohnston/nat/internal/source"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// realGitRepo is a scratch repository with one commit on main and no origin —
// the fetch fails and the base falls back to main — on the real git, which
// sp is pointed at. It answers the repository's resolved path, the one git
// names its worktrees by.
func realGitRepo(t *testing.T, sp *sourceProject) (repo string, gitRun func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitRun = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	gitRun("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun("add", "f")
	gitRun("commit", "-qm", "base")
	sp.env.NewGit = func() GitCLI { return git.New() }
	sp.env.NewWorktrees = func() actions.Worktrees { return worktree.New() }
	return dir, gitRun
}

// sliceWorktreeOf runs slice-worktree --json and decodes it.
func (sp *sourceProject) sliceWorktreeOf(t *testing.T, args ...string) actions.SliceWorktree {
	t.Helper()
	var wt actions.SliceWorktree
	out := sp.run(t, append(append([]string{"slice-worktree"}, args...), "--json", "--project", sp.id)...)
	if err := json.Unmarshal([]byte(out), &wt); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return wt
}

// A fresh cut, then the same worktree again, then a branch with no worktree
// checked out — on the real git, by nat's own naming.
func TestSliceWorktreeAgainstRealGit(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	repo, gitRun := realGitRepo(t, sp)
	task := sp.addTask(t, "Fix the login page", "c1")
	want := filepath.Join(repo+".worktrees", "slice-fix-the-login-page")

	// Fresh: cut from the base, on the derived branch; the text form is the
	// path alone.
	if out := sp.run(t, "slice-worktree", task, "--repo", repo, "--project", sp.id); out != want+"\n" {
		t.Errorf("text = %q, want the path alone", out)
	}
	if got := gitRun("-C", want, "rev-parse", "--abbrev-ref", "HEAD"); got != "slice/fix-the-login-page" {
		t.Errorf("worktree is on %q", got)
	}
	if gitRun("rev-parse", "main") != gitRun("-C", want, "rev-parse", "HEAD") {
		t.Error("the worktree was not cut from main")
	}

	// Again: the same worktree, found rather than cut.
	if got := sp.sliceWorktreeOf(t, task, "--repo", repo); got != (actions.SliceWorktree{
		Path: want, Branch: "slice/fix-the-login-page", Base: "main"}) {
		t.Errorf("reuse = %+v", got)
	}

	// A branch with work on it and no worktree is checked out, not re-cut.
	if err := os.WriteFile(filepath.Join(want, "g"), []byte("g\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun("-C", want, "add", "g")
	gitRun("-C", want, "commit", "-qm", "work")
	tip := gitRun("rev-parse", "slice/fix-the-login-page")
	gitRun("worktree", "remove", want)
	if got := sp.sliceWorktreeOf(t, task, "--repo", repo); got != (actions.SliceWorktree{
		Path: want, Branch: "slice/fix-the-login-page", Base: "main", Created: true}) {
		t.Errorf("checkout = %+v", got)
	}
	if got := gitRun("-C", want, "rev-parse", "HEAD"); got != tip {
		t.Errorf("worktree HEAD = %s, want the branch's own %s", got, tip)
	}

	// With no --repo, the slice's recorded repository is the one.
	sp.run(t, "slice-repo", task, "--repo", repo, "--project", sp.id)
	if got := sp.sliceWorktreeOf(t, task); got.Path != want || got.Created {
		t.Errorf("recorded repo = %+v", got)
	}
}

func TestSliceWorktreeRefusals(t *testing.T) {
	sp := newSourceProject(t, &source.Fake{Details: map[string]source.ContainerDetail{"c1": {ID: "c1", Title: "Card"}}})
	realGitRepo(t, sp)
	task := sp.addTask(t, "Task", "c1")
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--project", sp.id}, "want exactly one slice"},
		{[]string{"not-an-id", "--project", sp.id}, "slice-worktree"},
		{[]string{task, "--repo", "/no/such/dir", "--project", sp.id}, "/no/such/dir is not there"},
		{[]string{task}, "--project"},
		{[]string{task, "--bogus", "--project", sp.id}, "flag provided but not defined"},
		// A source task with nothing recorded has nowhere to look.
		{[]string{task, "--project", sp.id}, `"Task" has no repository yet`},
		// Outside a repository: git's own words.
		{[]string{task, "--repo", t.TempDir(), "--project", sp.id}, "not a git repository"},
	} {
		if err := sp.fail(t, append([]string{"slice-worktree"}, tt.args...)...); !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: err = %v, want %q", tt.args, err, tt.want)
		}
	}

	// A slice the plan does not hold, then a plan that will not open at all.
	if err := sp.fail(t, "slice-worktree", "0123456789abcdef0123456789abcdef", "--project", sp.id); err == nil {
		t.Error("unknown slice: want an error")
	}
	plan := sp.planPath(t)
	if err := os.Remove(plan); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := sp.fail(t, "slice-worktree", task, "--project", sp.id); err == nil {
		t.Error("broken plan: want an error")
	}
}
