package worktree

import (
	"errors"
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

// fakeRunner stands in for the worktrunk binary, recording the calls the CLI
// makes and answering each with whatever the test wants worktrunk to have said.
// A call past the end of the replies answers with nothing at all, which is what
// a worktrunk that succeeded silently looks like.
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

// listJSON is `wt list --format json` as worktrunk writes it, cut down to the
// two fields this package reads.
const listJSON = `[
  {"branch": "main", "path": "/repos/nat", "kind": "worktree"},
  {"branch": "slice/worktrees", "path": "/repos/nat-worktrees", "kind": "worktree"}
]`

// TestCreateRunsWorktrunk pins the invocation: worktrunk, in the slice's
// repository, told to cut the branch, not to change a directory there is no
// shell for and not to ask anyone anything — then asked where it put it.
func TestCreateRunsWorktrunk(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{}, {out: listJSON}}}
	path, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees")
	if err != nil {
		t.Fatalf("Create() = %v, want a worktree", err)
	}
	if want := "/repos/nat-worktrees"; path != want {
		t.Errorf("Create() = %q, want %q", path, want)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("made %d calls, want the switch and the list", len(runner.calls))
	}
	for i, c := range runner.calls {
		if c.dir != "/repos/nat" {
			t.Errorf("call %d ran in %q, want the slice's repository", i, c.dir)
		}
		if c.name != Binary {
			t.Errorf("call %d ran %q, want %q", i, c.name, Binary)
		}
	}
	want := []string{"switch", "--create", "slice/worktrees", "--no-cd", "-y"}
	if !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[0].args, want)
	}
	want = []string{"list", "--format", "json"}
	if !reflect.DeepEqual(runner.calls[1].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[1].args, want)
	}
}

// TestCreateFailure passes worktrunk's own refusal straight back, and asks for
// no path: there is no worktree to name.
func TestCreateFailure(t *testing.T) {
	refused := &ExitError{Code: 1, Stderr: "\nbranch 'slice/worktrees' already exists\nhint: switch to it instead\n"}
	runner := &fakeRunner{replies: []reply{{err: refused}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Create() = %v, want worktrunk's own error", err)
	}
	if want := "branch 'slice/worktrees' already exists"; err.Error() != want {
		t.Errorf("Create() = %q, want %q", err, want)
	}
	if len(runner.calls) != 1 {
		t.Errorf("made %d calls, want the switch alone", len(runner.calls))
	}
}

// TestCreateWithoutWorktrunk covers the machine the launch path falls back to
// the shared checkout on: the missing binary is its own error, and os/exec's
// report is still inside it.
func TestCreateWithoutWorktrunk(t *testing.T) {
	missing := &exec.Error{Name: Binary, Err: exec.ErrNotFound}
	runner := &fakeRunner{replies: []reply{{err: missing}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Create() = %v, want it to say worktrunk is not installed", err)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Create() = %v, want os/exec's own report kept", err)
	}
}

// TestCreateWhenTheListFails covers a switch that worked and a list that did
// not: there is a worktree, but nothing here can say where, so it is a failure.
func TestCreateWhenTheListFails(t *testing.T) {
	refused := &ExitError{Code: 2, Stderr: "not a git repository\n"}
	runner := &fakeRunner{replies: []reply{{}, {err: refused}}}
	_, err := NewWithRunner(runner).Create("/repos/nat", "slice/worktrees")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Create() = %v, want the list's own error", err)
	}
}

// TestRemoveRunsWorktrunk pins the invocation, and leaves what removal means —
// the branch deleted only if merged, a dirty worktree refused — to worktrunk.
func TestRemoveRunsWorktrunk(t *testing.T) {
	runner := &fakeRunner{}
	if err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees"); err != nil {
		t.Fatalf("Remove() = %v, want it removed", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("made %d calls, want one", len(runner.calls))
	}
	if runner.calls[0].dir != "/repos/nat" || runner.calls[0].name != Binary {
		t.Errorf("ran %q in %q, want %q in the slice's repository",
			runner.calls[0].name, runner.calls[0].dir, Binary)
	}
	want := []string{"remove", "slice/worktrees", "-y"}
	if !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Errorf("args = %v, want %v", runner.calls[0].args, want)
	}
}

// TestRemoveFailure covers the refusal that matters: a worktree with work in it
// is kept, and worktrunk's own reason is what comes back.
func TestRemoveFailure(t *testing.T) {
	refused := &ExitError{Code: 1, Stderr: "worktree has uncommitted changes\n"}
	runner := &fakeRunner{replies: []reply{{err: refused}}}
	err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Remove() = %v, want worktrunk's own error", err)
	}
	if want := "worktree has uncommitted changes"; err.Error() != want {
		t.Errorf("Remove() = %q, want %q", err, want)
	}
}

// TestRemoveWithoutWorktrunk covers the missing binary on the other operation.
func TestRemoveWithoutWorktrunk(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{err: &exec.Error{Name: Binary, Err: exec.ErrNotFound}}}}
	err := NewWithRunner(runner).Remove("/repos/nat", "slice/worktrees")
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Remove() = %v, want it to say worktrunk is not installed", err)
	}
}

// TestPathUnknownBranch covers a branch the list does not name: there is no
// path, and an empty string would read as one.
func TestPathUnknownBranch(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: listJSON}}}
	_, err := NewWithRunner(runner).Path("/repos/nat", "slice/elsewhere")
	if err == nil || !strings.Contains(err.Error(), "slice/elsewhere") {
		t.Errorf("Path() = %v, want it to name the branch it could not find", err)
	}
}

// TestPathBranchWithoutAWorktree covers what --branches-style entries look
// like: a branch worktrunk knows about that is checked out nowhere.
func TestPathBranchWithoutAWorktree(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: `[{"branch": "slice/worktrees", "path": "", "kind": "branch"}]`}}}
	_, err := NewWithRunner(runner).Path("/repos/nat", "slice/worktrees")
	if err == nil || !strings.Contains(err.Error(), "no worktree") {
		t.Errorf("Path() = %v, want it to report the branch has no worktree", err)
	}
}

// TestPathUnreadableJSON covers a worktrunk that printed something else — an
// older schema, a banner on stdout — rather than the list this reads.
func TestPathUnreadableJSON(t *testing.T) {
	runner := &fakeRunner{replies: []reply{{out: "not json at all\n"}}}
	_, err := NewWithRunner(runner).Path("/repos/nat", "slice/worktrees")
	if err == nil || !strings.Contains(err.Error(), "no readable worktrees") {
		t.Errorf("Path() = %v, want it to report the unreadable list", err)
	}
}

// TestExitErrorWithoutStderr covers a worktrunk that failed silently: there is
// nothing to quote, so the exit code is what there is to say.
func TestExitErrorWithoutStderr(t *testing.T) {
	err := &ExitError{Code: 4, Stderr: " \n\n"}
	if want := "wt exited 4"; err.Error() != want {
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

// TestExecRunnerReportsAMissingBinary covers the failure a machine without
// worktrunk gives: os/exec's own, passed through rather than dressed up as an
// exit, which is what [classify] recognises it by.
func TestExecRunnerReportsAMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Run(t.TempDir(), "nat-no-such-binary")
	if err == nil {
		t.Fatal("Run() = nil, want a missing binary reported")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Run() = %v, want it to say the binary is not there", err)
	}
}
