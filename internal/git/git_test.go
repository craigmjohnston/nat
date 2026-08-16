package git

import (
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// call is one invocation the CLI made, as the fake recorded it.
type call struct {
	dir  string
	name string
	args []string
}

// fakeRunner stands in for the git binary, recording every call and answering
// each from a queue: the CLI asks for the default branch before it asks for the
// diff, so a test says what git said in that order.
type fakeRunner struct {
	outs []string
	errs []error

	calls []call
}

var _ Runner = (*fakeRunner)(nil)

func (f *fakeRunner) Run(dir, name string, args ...string) (string, error) {
	i := len(f.calls)
	f.calls = append(f.calls, call{dir: dir, name: name, args: args})
	var out string
	var err error
	if i < len(f.outs) {
		out = f.outs[i]
	}
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return out, err
}

// TestDiffRunsGit pins both invocations: the default branch read off origin's
// HEAD, and the diff taken against the merge base with it, in the slice's
// repository, with the prefixes pinned and any external diff driver refused.
func TestDiffRunsGit(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/trunk\n", "diff --git a/x b/x\n"}}
	base, diff, err := NewWithRunner(runner).Diff("/repos/nat", "slice/viewer")
	if err != nil {
		t.Fatalf("Diff() = %v, want a diff", err)
	}
	if base != "origin/trunk" {
		t.Errorf("base = %q, want the branch origin's HEAD names", base)
	}
	if diff != "diff --git a/x b/x\n" {
		t.Errorf("diff = %q, want what git wrote", diff)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("made %d calls, want the base read and the diff", len(runner.calls))
	}
	for i, c := range runner.calls {
		if c.dir != "/repos/nat" {
			t.Errorf("call %d ran in %q, want the slice's repository", i, c.dir)
		}
		if c.name != Binary {
			t.Errorf("call %d ran %q, want %q", i, c.name, Binary)
		}
	}
	wantBase := []string{"symbolic-ref", "--short", "refs/remotes/origin/HEAD"}
	if !reflect.DeepEqual(runner.calls[0].args, wantBase) {
		t.Errorf("base args = %v, want %v", runner.calls[0].args, wantBase)
	}
	wantDiff := []string{"diff", "--no-color", "--no-ext-diff", "--src-prefix=a/",
		"--dst-prefix=b/", "--merge-base", "origin/trunk", "slice/viewer"}
	if !reflect.DeepEqual(runner.calls[1].args, wantDiff) {
		t.Errorf("diff args = %v, want %v", runner.calls[1].args, wantDiff)
	}
}

// TestDiffFallsBackToMain covers a repository with no origin to read a default
// branch off: the project's own convention is what stands in for it, and the
// diff is still taken.
func TestDiffFallsBackToMain(t *testing.T) {
	runner := &fakeRunner{errs: []error{&ExitError{Code: 128, Stderr: "fatal: ref refs/remotes/origin/HEAD is not a symbolic ref\n"}}}
	base, _, err := NewWithRunner(runner).Diff("/repos/nat", "slice/viewer")
	if err != nil {
		t.Fatalf("Diff() = %v, want a diff against %s", err, DefaultBase)
	}
	if base != DefaultBase {
		t.Errorf("base = %q, want %q", base, DefaultBase)
	}
	if got := runner.calls[1].args[len(runner.calls[1].args)-2]; got != DefaultBase {
		t.Errorf("diffed against %q, want %q", got, DefaultBase)
	}
}

// TestBaseFallsBackOnAnEmptyAnswer covers a git that succeeded and said
// nothing, which names no branch to diff against either.
func TestBaseFallsBackOnAnEmptyAnswer(t *testing.T) {
	runner := &fakeRunner{outs: []string{" \n"}}
	if base := NewWithRunner(runner).Base("/repos/nat"); base != DefaultBase {
		t.Errorf("Base() = %q, want %q", base, DefaultBase)
	}
}

// TestDiffFailure passes git's own refusal straight back, with the branch it
// was asked about named: the reason is what the screen shows.
func TestDiffFailure(t *testing.T) {
	refused := &ExitError{Code: 128, Stderr: "fatal: bad revision 'slice/gone'\n"}
	runner := &fakeRunner{outs: []string{"origin/main\n"}, errs: []error{nil, refused}}
	base, diff, err := NewWithRunner(runner).Diff("/repos/nat", "slice/gone")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("Diff() = %v, want git's own error", err)
	}
	if base != "origin/main" {
		t.Errorf("base = %q, want the base to be reported even on a failure", base)
	}
	if diff != "" {
		t.Errorf("diff = %q, want nothing", diff)
	}
	if want := "fatal: bad revision 'slice/gone'"; err.Error() != want {
		t.Errorf("Diff() = %q, want %q", err, want)
	}
}

// TestExitErrorWithoutStderr covers a git that failed silently: there is
// nothing to quote, so the exit code is what there is to say.
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
// gives: os/exec's own, passed through rather than dressed up as an exit.
func TestExecRunnerReportsAMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Run(t.TempDir(), "nat-no-such-binary")
	if err == nil {
		t.Fatal("Run() = nil, want a missing binary reported")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Run() = %v, want it to say the binary is not there", err)
	}
}
