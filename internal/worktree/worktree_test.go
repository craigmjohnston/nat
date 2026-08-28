package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// call is one invocation the fake runner was asked for.
type call struct {
	dir  string
	name string
	args []string
}

// reply is what the fake runner answers with, in the order the calls come.
type reply struct {
	out string
	err error
}

// fakeRunner stands in for the git binary, recording the calls the CLI makes
// and answering each with whatever the test wants git to have said. A call past
// the end of the replies answers with nothing at all, which is what a git that
// succeeded silently looks like.
type fakeRunner struct {
	replies []reply
	calls   []call
}

var _ Runner = (*fakeRunner)(nil)

func (f *fakeRunner) Run(dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{dir: dir, name: name, args: args})
	if len(f.calls) <= len(f.replies) {
		r := f.replies[len(f.calls)-1]
		return r.out, r.err
	}
	return "", nil
}

// The three answers a create asks for, in order: where the repository is,
// whether the branch is already there, and the add itself.
const (
	commonDir = "/repos/nat/.git\n"
	wantPath  = "/repos/nat.worktrees/slice-worktrees"
)

// noBranch is `git rev-parse --verify --quiet` on a ref that is not there: it
// exits 1 and says nothing at all, which is what --quiet is for.
var noBranch = &ExitError{Code: 1}

// listPorcelain is `git worktree list --porcelain` as git writes it: one record
// per worktree, opened by its path, and a detached one naming no branch.
const listPorcelain = `worktree /repos/nat
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /repos/nat.worktrees/slice-worktrees
HEAD 2222222222222222222222222222222222222222
branch refs/heads/slice/worktrees

worktree /repos/nat.worktrees/spike
HEAD 3333333333333333333333333333333333333333
detached

`

// TestCreateCutsAWorktree pins the invocations: where the repository is, whether
// the branch is already there, and then the worktree cut at nat's own path with
// the branch made from the base it was given.
func TestCreateCutsAWorktree(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: commonDir}, {err: noBranch}, {}}}
	path, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if err != nil {
		t.Fatalf("Create() = %v, want a worktree", err)
	}
	if path != wantPath {
		t.Errorf("Create() = %q, want %q", path, wantPath)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("made %d calls, want the root, the branch and the add", len(runner.calls))
	}
	for i, c := range runner.calls {
		if c.dir != "/repos/nat" {
			t.Errorf("call %d ran in %q, want the slice's repository", i, c.dir)
		}
		if c.name != Binary {
			t.Errorf("call %d ran %q, want %q", i, c.name, Binary)
		}
	}
	want := []string{"rev-parse", "--path-format=absolute", "--git-common-dir"}
	if !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[0].args, want)
	}
	want = []string{"rev-parse", "--verify", "--quiet", "refs/heads/slice/worktrees"}
	if !reflect.DeepEqual(runner.calls[1].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[1].args, want)
	}
	want = []string{"worktree", "add", wantPath, "-b", "slice/worktrees", "origin/main"}
	if !reflect.DeepEqual(runner.calls[2].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[2].args, want)
	}
}

// TestCreateWithoutABase covers the caller that resolved no ref: git is told
// nothing about a start point and cuts the branch where it would have anyway.
func TestCreateWithoutABase(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: commonDir}, {err: noBranch}, {}}}
	if _, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", ""); err != nil {
		t.Fatalf("Create() = %v, want a worktree", err)
	}
	want := []string{"worktree", "add", wantPath, "-b", "slice/worktrees"}
	if !reflect.DeepEqual(runner.calls[2].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[2].args, want)
	}
}

// TestCreateReusesAnExistingBranch covers the branch a finished slice left
// behind: it is checked out as it stands, and the base is not consulted, since
// its commits are the work a relaunch wants.
func TestCreateReusesAnExistingBranch(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: commonDir}, {out: "abc123\n"}, {}}}
	path, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if err != nil {
		t.Fatalf("Create() = %v, want a worktree", err)
	}
	if path != wantPath {
		t.Errorf("Create() = %q, want %q", path, wantPath)
	}
	want := []string{"worktree", "add", wantPath, "slice/worktrees"}
	if !reflect.DeepEqual(runner.calls[2].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[2].args, want)
	}
}

// TestCreateWithoutARepository covers a directory git will not answer for:
// there is nowhere to put a worktree, so nothing is attempted.
func TestCreateWithoutARepository(t *testing.T) {
	refused := &ExitError{Code: 128, Stderr: "fatal: not a git repository\n"}
	runner := &fakeRunner{replies: []reply{{err: refused}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Create() = %v, want git's own error", err)
	}
	if len(runner.calls) != 1 {
		t.Errorf("made %d calls, want the root alone", len(runner.calls))
	}
}

// TestCreateWhenTheRootIsEmpty covers a git that answered and said nothing: an
// empty path would put the worktree beside the filesystem root.
func TestCreateWhenTheRootIsEmpty(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: "  \n"}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if err == nil || !strings.Contains(err.Error(), "names no repository") {
		t.Errorf("Create() = %v, want it to report the repository it could not find", err)
	}
}

// TestCreateFailure passes git's own refusal straight back.
func TestCreateFailure(t *testing.T) {
	refused := &ExitError{Code: 128, Stderr: "\nfatal: 'x' is already used by worktree\nhint: use --force\n"}
	runner := &fakeRunner{replies: []reply{{out: commonDir}, {err: noBranch}, {err: refused}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Create() = %v, want git's own error", err)
	}
	if want := "fatal: 'x' is already used by worktree"; err.Error() != want {
		t.Errorf("Create() = %q, want %q", err, want)
	}
}

// TestCreateWithoutGit covers the machine the launch path falls back to the
// shared checkout on: the missing binary is its own error, and os/exec's report
// is still inside it.
func TestCreateWithoutGit(t *testing.T) {
	missing := &exec.Error{Name: Binary, Err: exec.ErrNotFound}
	runner := &fakeRunner{replies: []reply{{err: missing}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Create() = %v, want it to say git is not installed", err)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Create() = %v, want os/exec's own report kept", err)
	}
}

// TestCreateWithoutGitOnTheAdd covers the same failure on the call that does the
// work, since the root is read with a different command.
func TestCreateWithoutGitOnTheAdd(t *testing.T) {
	missing := &exec.Error{Name: Binary, Err: exec.ErrNotFound}
	runner := &fakeRunner{replies: []reply{{out: commonDir}, {err: noBranch}, {err: missing}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees", "origin/main")
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Create() = %v, want it to say git is not installed", err)
	}
}

// TestRemoveTakesTheWorktreeAndTheBranch pins the invocations: the worktree
// found by branch, removed by path, and then the branch deleted the safe way.
func TestRemoveTakesTheWorktreeAndTheBranch(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}, {}, {}}}
	if err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees"); err != nil {
		t.Fatalf("Remove() = %v, want it removed", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("made %d calls, want the list, the removal and the branch", len(runner.calls))
	}
	for i, c := range runner.calls {
		if c.dir != "/repos/nat" || c.name != Binary {
			t.Errorf("call %d ran %q in %q, want %q in the slice's repository",
				i, c.name, c.dir, Binary)
		}
	}
	want := []string{"worktree", "remove", wantPath}
	if !reflect.DeepEqual(runner.calls[1].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[1].args, want)
	}
	want = []string{"branch", "-d", "slice/worktrees"}
	if !reflect.DeepEqual(runner.calls[2].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[2].args, want)
	}
}

// TestRemoveKeepsAnUndeletableBranch covers the branch a squash merge leaves:
// git will not delete it by the ancestry test, and a leftover branch is
// harmless, so the removal still succeeded.
func TestRemoveKeepsAnUndeletableBranch(t *testing.T) {
	refused := &ExitError{Code: 1, Stderr: "error: the branch 'slice/worktrees' is not fully merged\n"}
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}, {}, {err: refused}}}
	if err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees"); err != nil {
		t.Errorf("Remove() = %v, want the worktree gone whatever became of the branch", err)
	}
}

// TestRemoveWithoutAWorktree covers a branch git has no worktree for: there is
// nothing to remove, and the reason comes back rather than a silent success.
func TestRemoveWithoutAWorktree(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}}}
	err := NewWithRunner(runner).Remove("/repos/nat", "slice/elsewhere")
	if err == nil || !strings.Contains(err.Error(), "slice/elsewhere") {
		t.Fatalf("Remove() = %v, want it to name the branch it could not find", err)
	}
	if len(runner.calls) != 1 {
		t.Errorf("made %d calls, want the list alone", len(runner.calls))
	}
}

// TestRemoveFailure covers the refusal that matters: a worktree with work in it
// is kept, and git's own reason is what comes back.
func TestRemoveFailure(t *testing.T) {
	refused := &ExitError{Code: 1, Stderr: "fatal: contains modified or untracked files\n"}
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}, {err: refused}}}
	err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Remove() = %v, want git's own error", err)
	}
	if want := "fatal: contains modified or untracked files"; err.Error() != want {
		t.Errorf("Remove() = %q, want %q", err, want)
	}
}

// TestRemoveWithoutGit covers the missing binary on the removal itself.
func TestRemoveWithoutGit(t *testing.T) {
	missing := &exec.Error{Name: Binary, Err: exec.ErrNotFound}
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}, {err: missing}}}
	err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees")
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Remove() = %v, want it to say git is not installed", err)
	}
}

// TestPathReadsTheListing covers the ordinary read: the branch's own record,
// past the main worktree and whatever else the repository has out.
func TestPathReadsTheListing(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}}}
	path, err := NewWithRunner(runner).Path("/repos/nat", "slice/worktrees")
	if err != nil {
		t.Fatalf("Path() = %v, want the worktree", err)
	}
	if path != wantPath {
		t.Errorf("Path() = %q, want %q", path, wantPath)
	}
	want := []string{"worktree", "list", "--porcelain"}
	if !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[0].args, want)
	}
}

// TestPathByFullRef covers a caller naming the branch as git does: the ref is
// taken as it stands rather than having refs/heads/ put in front of it twice.
func TestPathByFullRef(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}}}
	path, err := NewWithRunner(runner).Path("/repos/nat", "refs/heads/main")
	if err != nil {
		t.Fatalf("Path() = %v, want the main worktree", err)
	}
	if want := "/repos/nat"; path != want {
		t.Errorf("Path() = %q, want %q", path, want)
	}
}

// TestPathUnknownBranch covers a branch the listing does not name — the
// ordinary answer for a slice nobody has worked yet — and the detached worktree
// beside it, which names no branch to match.
func TestPathUnknownBranch(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: listPorcelain}}}
	_, err := NewWithRunner(runner).Path("/repos/nat", "slice/elsewhere")
	if err == nil || !strings.Contains(err.Error(), "slice/elsewhere") {
		t.Errorf("Path() = %v, want it to name the branch it could not find", err)
	}
}

// TestPathFailure covers a git that refused to list at all.
func TestPathFailure(t *testing.T) {
	refused := &ExitError{Code: 128, Stderr: "fatal: not a git repository\n"}
	runner := &fakeRunner{replies: []reply{{err: refused}}}
	_, err := NewWithRunner(runner).Path("/repos/nat", "slice/worktrees")
	if !errors.Is(err, error(refused)) {
		t.Errorf("Path() = %v, want git's own error", err)
	}
}

// TestPathWithoutGit covers the missing binary on the read the launch path takes
// first, which is where a machine without git falls back to the shared checkout.
func TestPathWithoutGit(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{err: &exec.Error{Name: Binary, Err: exec.ErrNotFound}}}}
	_, err := NewWithRunner(runner).Path("/repos/nat", "slice/worktrees")
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Path() = %v, want it to say git is not installed", err)
	}
}

// TestPathSlug covers the branch names that are not directory names: the
// separators collapse, the ends come off, and a branch with nothing usable in it
// still names something.
func TestPathSlug(t *testing.T) {
	for branch, want := range map[string]string{
		"slice/worktrees":       "slice-worktrees",
		"slice/a b/c":           "slice-a-b-c",
		"-slice/worktrees.":     "slice-worktrees",
		"feat/v1.2_x":           "feat-v1.2_x",
		"//":                    "worktree",
		"слайс":                 "worktree",
		"slice/reimplement-git": "slice-reimplement-git",
	} {
		if got := pathSlug(branch); got != want {
			t.Errorf("pathSlug(%q) = %q, want %q", branch, got, want)
		}
	}
}

// TestExitErrorWithoutStderr covers a git that failed silently: there is nothing
// to quote, so the exit code is what there is to say.
func TestExitErrorWithoutStderr(t *testing.T) {
	err := &ExitError{Code: 4, Stderr: " \n\n"}
	if want := "git exited 4"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err, want)
	}
}

// TestNewDrivesTheRealBinary pins what the constructor with no seam in it is
// wired to.
func TestNewDrivesTheRealBinary(t *testing.T) {
	if _, ok := New().runner.(ExecRunner); !ok {
		t.Errorf("New() runs through %T, want the real subprocesses", New().runner)
	}
}

// TestExecRunnerRunsInTheDirectory covers the real runner: a command really
// starts, really runs where it was told to, and its standard output comes back.
func TestExecRunnerRunsInTheDirectory(t *testing.T) {
	dir := t.TempDir()
	// -P, because the shell inherits PWD from this process and would otherwise
	// print that rather than where it was actually started.
	out, err := ExecRunner{}.Run(dir, "sh", "-c", "pwd -P")
	if err != nil {
		t.Fatalf("Run() = %v, want it to run", err)
	}
	// macOS puts the temp directory under a symlink, so the shell's answer is
	// compared by its resolved path rather than by the string handed in.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("Run() ran in %q, want %q", got, want)
	}
}

// TestExecRunnerReportsAnExit covers a command that ran and refused: the exit
// code and what it wrote to stderr both come back.
func TestExecRunnerReportsAnExit(t *testing.T) {
	out, err := ExecRunner{}.Run(t.TempDir(), "sh", "-c", "echo said; echo boom >&2; exit 3")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() = %v, want an *ExitError", err)
	}
	if exitErr.Code != 3 || strings.TrimSpace(exitErr.Stderr) != "boom" {
		t.Errorf("Run() = %+v, want exit 3 with boom on stderr", exitErr)
	}
	if strings.TrimSpace(out) != "said" {
		t.Errorf("Run() stdout = %q, want what the command printed before it failed", out)
	}
}

// TestExecRunnerReportsAMissingBinary covers the failure a machine without git
// gives: os/exec's own, passed through rather than dressed up as an exit, which
// is what [classify] recognises it by.
func TestExecRunnerReportsAMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Run(t.TempDir(), "nat-no-such-binary")
	if err == nil {
		t.Fatal("Run() = nil, want a missing binary reported")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Run() = %v, want it to say the binary is not there", err)
	}
}

// runGit is the real runner as a function, so a test may call it inside an if
// statement, where a composite literal would need parentheses of its own.
var runGit = ExecRunner{}.Run

// realRepo is a git repository with one commit on main, made where nat would
// find one: a directory of its own, so the worktrees cut beside it land in the
// test's own temporary directory and go with it.
func realRepo(t *testing.T) string {
	t.Helper()
	// macOS puts the temp directory under a symlink and git answers with the
	// resolved path, so the repository is named by that from the start.
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "nat")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "nat@example.test"},
		{"config", "user.name", "nat"},
		{"commit", "--allow-empty", "-m", "first"},
	} {
		if out, err := runGit(dir, Binary, args...); err != nil {
			t.Fatalf("git %v = %v (%s)", args, err, out)
		}
	}
	return dir
}

// TestAgainstARealRepository is the three operations against git itself: a
// branch with no worktree has no path, a create cuts one at nat's own path with
// the branch based on what it was given, the path is then found by branch, and
// a remove takes both the worktree and the branch away again.
func TestAgainstARealRepository(t *testing.T) {
	dir := realRepo(t)
	c := New()

	if _, err := c.Path(dir, "slice/worktrees"); err == nil {
		t.Fatal("Path() found a worktree for a branch nobody has worked")
	}

	path, err := c.Create(dir, "slice/worktrees", "main")
	if err != nil {
		t.Fatalf("Create() = %v, want a worktree", err)
	}
	if want := filepath.Join(dir+dirSuffix, "slice-worktrees"); path != want {
		t.Fatalf("Create() = %q, want %q", path, want)
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		t.Fatalf("the worktree is not there: %v", err)
	}
	head, err := runGit(path, Binary, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(head); got != "slice/worktrees" {
		t.Errorf("the worktree is on %q, want the slice's branch", got)
	}

	found, err := c.Path(dir, "slice/worktrees")
	if err != nil {
		t.Fatalf("Path() = %v, want the worktree just cut", err)
	}
	if found != path {
		t.Errorf("Path() = %q, want %q", found, path)
	}

	if err := c.Remove(dir, "slice/worktrees"); err != nil {
		t.Fatalf("Remove() = %v, want it removed", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the worktree is still at %q", path)
	}
	// The branch had nothing on it that main did not, so git deleted it too.
	if _, err := runGit(dir, Binary, "rev-parse", "--verify", "--quiet",
		"refs/heads/slice/worktrees"); err == nil {
		t.Error("the branch is still there, want it deleted with its worktree")
	}
}

// TestARealRepositoryKeepsWork covers the refusal that matters against git
// itself: a worktree with uncommitted work in it is not removed, and the branch
// a relaunch would want survives with it.
func TestARealRepositoryKeepsWork(t *testing.T) {
	dir := realRepo(t)
	c := New()

	path, err := c.Create(dir, "slice/worktrees", "main")
	if err != nil {
		t.Fatalf("Create() = %v, want a worktree", err)
	}
	if err := os.WriteFile(filepath.Join(path, "work.txt"), []byte("half done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(dir, "slice/worktrees"); err == nil {
		t.Fatal("Remove() = nil, want git's refusal to throw work away")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the worktree went anyway: %v", err)
	}
}

// TestARealRepositoryReusesABranch covers the relaunch of a slice whose branch
// outlived its worktree: the existing branch is checked out with its commits on
// it rather than cut a second time from the base.
func TestARealRepositoryReusesABranch(t *testing.T) {
	dir := realRepo(t)
	c := New()

	if _, err := runGit(dir, Binary, "branch", "slice/worktrees"); err != nil {
		t.Fatal(err)
	}
	path, err := c.Create(dir, "slice/worktrees", "main")
	if err != nil {
		t.Fatalf("Create() = %v, want the existing branch checked out", err)
	}
	head, err := runGit(path, Binary, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(head); got != "slice/worktrees" {
		t.Errorf("the worktree is on %q, want the branch it already had", got)
	}
}
