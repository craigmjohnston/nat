package gh

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeRunner stands in for the gh binary, recording the one call the CLI makes
// and answering with whatever the test wants gh to have said. It answers
// [StdinRunner] as well as [Runner], recording whatever was on stdin, so it
// serves every command in the package including [CLI.CommentPR].
type fakeRunner struct {
	out string
	err error

	dir   string
	name  string
	args  []string
	runs  int
	stdin string
}

var (
	_ Runner      = (*fakeRunner)(nil)
	_ StdinRunner = (*fakeRunner)(nil)
)

func (f *fakeRunner) Run(dir, name string, args ...string) (string, error) {
	f.runs++
	f.dir, f.name, f.args = dir, name, args
	return f.out, f.err
}

func (f *fakeRunner) RunWithStdin(dir string, stdin io.Reader, name string, args ...string) (string, error) {
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		f.stdin = string(b)
	}
	return f.Run(dir, name, args...)
}

// TestCreatePRRunsGh pins the invocation for a hand-back that left no
// description: gh, in the slice's repository, told which branch to open the
// pull request from and to fill the title and body in from its commits rather
// than asking.
func TestCreatePRRunsGh(t *testing.T) {
	runner := &fakeRunner{out: "https://github.test/craig/nat/pull/7\n"}
	url, err := NewWithRunner(runner).CreatePR("/repos/nat", "slice/approve", "", "")
	if err != nil {
		t.Fatalf("CreatePR() = %v, want a pull request", err)
	}
	if want := "https://github.test/craig/nat/pull/7"; url != want {
		t.Errorf("CreatePR() = %q, want %q", url, want)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "create", "--head", "slice/approve", "--fill"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// TestCreatePRWithADescription pins the other invocation: a hand-back that
// recorded a description opens the pull request with it, title and body, and
// --fill goes — there is nothing left for gh to fill in.
func TestCreatePRWithADescription(t *testing.T) {
	runner := &fakeRunner{out: "https://github.test/craig/nat/pull/7\n"}
	_, err := NewWithRunner(runner).CreatePR("/repos/nat", "slice/approve",
		"Open the PR with the recorded description", "What it does, and why.")
	if err != nil {
		t.Fatalf("CreatePR() = %v, want a pull request", err)
	}
	want := []string{"pr", "create", "--head", "slice/approve",
		"--title", "Open the PR with the recorded description", "--body", "What it does, and why."}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// TestCreatePRWithATitleAlone covers a one-line description: gh is still told
// the title, and the body it is given is the empty one there is.
func TestCreatePRWithATitleAlone(t *testing.T) {
	runner := &fakeRunner{out: "https://github.test/craig/nat/pull/7\n"}
	if _, err := NewWithRunner(runner).CreatePR("/repos/nat", "slice/approve", "One line", ""); err != nil {
		t.Fatalf("CreatePR() = %v, want a pull request", err)
	}
	want := []string{"pr", "create", "--head", "slice/approve", "--title", "One line", "--body", ""}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// TestCreatePRTakesTheLastURL covers gh printing something before the URL: the
// pull request is the last URL on stdout, not the first line of it.
func TestCreatePRTakesTheLastURL(t *testing.T) {
	runner := &fakeRunner{out: "https://github.test/craig/nat/tree/slice/approve\n" +
		"https://github.test/craig/nat/pull/7\n\n"}
	url, err := NewWithRunner(runner).CreatePR("/repos/nat", "slice/approve", "", "")
	if err != nil {
		t.Fatalf("CreatePR() = %v, want a pull request", err)
	}
	if want := "https://github.test/craig/nat/pull/7"; url != want {
		t.Errorf("CreatePR() = %q, want %q", url, want)
	}
}

// TestCreatePRWithoutAURL covers a gh that succeeded but said nothing we can
// record: there is no pull request to write down, so it is a failure here.
func TestCreatePRWithoutAURL(t *testing.T) {
	runner := &fakeRunner{out: "Creating pull request…\n"}
	_, err := NewWithRunner(runner).CreatePR("/repos/nat", "slice/approve", "", "")
	if err == nil || !strings.Contains(err.Error(), "no pull request URL") {
		t.Errorf("CreatePR() = %v, want it to report the missing URL", err)
	}
}

// TestCreatePRFailure passes gh's own refusal straight back: the reason is what
// the board shows.
func TestCreatePRFailure(t *testing.T) {
	refused := &ExitError{Code: 1, Stderr: "\na pull request for branch \"slice/approve\" already exists\nUsage: gh pr create\n"}
	runner := &fakeRunner{err: refused}
	_, err := NewWithRunner(runner).CreatePR("/repos/nat", "slice/approve", "", "")
	if !errors.Is(err, error(refused)) {
		t.Fatalf("CreatePR() = %v, want gh's own error", err)
	}
	if want := `a pull request for branch "slice/approve" already exists`; err.Error() != want {
		t.Errorf("CreatePR() = %q, want %q", err, want)
	}
}

// TestExitErrorWithoutStderr covers a gh that failed silently: there is nothing
// to quote, so the exit code is what there is to say.
func TestExitErrorWithoutStderr(t *testing.T) {
	err := &ExitError{Code: 4, Stderr: " \n\n"}
	if want := "gh exited 4"; err.Error() != want {
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

// TestExecRunnerReportsAMissingBinary covers the failure a machine without gh
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

// TestExecRunnerRunWithStdinCarriesInput covers the seam [CLI.CommentPR]
// needs: what is handed as stdin is what the command reads off it, in the
// directory it was told to run in.
func TestExecRunnerRunWithStdinCarriesInput(t *testing.T) {
	dir := t.TempDir()
	out, err := ExecRunner{}.RunWithStdin(dir, strings.NewReader("said on stdin"), "cat")
	if err != nil {
		t.Fatalf("RunWithStdin() = %v, want it to run", err)
	}
	if out != "said on stdin" {
		t.Errorf("RunWithStdin() = %q, want the input echoed back", out)
	}
}
