package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// The stdout and stderr `git merge-tree --write-tree --no-messages --name-only`
// actually wrote (git 2.45) for each outcome, read off a scratch repository.
const (
	// cleanMerge is a merge with no conflict: the tree it wrote, alone.
	cleanMerge = "07477309334432af81ec29536da80a4051d03a6c\n"
	// conflictedMerge is the tree, then one conflicted path per line.
	conflictedMerge = "7ad6f124c521168e40e0020ed2d8fd2c5eefcbc2\nf\n"
	// unresolvedStderr is what git says, exiting 1 with nothing on stdout,
	// for a revision it cannot resolve — the exit code a conflict has too.
	unresolvedStderr = "merge-tree: slice/gone - not something we can merge\n"
)

// TestConflictsWithBaseRunsGit pins every invocation: the fetch, the default
// branch off origin's HEAD, and the merge tested against it in the object
// store, in the slice's repository, with the output pinned.
func TestConflictsWithBaseRunsGit(t *testing.T) {
	runner := &fakeRunner{outs: []string{"", "origin/trunk\n", cleanMerge}}
	if got := NewWithRunner(runner).ConflictsWithBase("/repos/nat", "slice/viewer"); got != MergeClean {
		t.Errorf("ConflictsWithBase() = %v, want MergeClean", got)
	}
	want := [][]string{
		{"fetch", "origin"},
		{"symbolic-ref", "--short", "refs/remotes/origin/HEAD"},
		{"merge-tree", "--write-tree", "--no-messages", "--name-only", "origin/trunk", "slice/viewer"},
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("made %d calls, want %d", len(runner.calls), len(want))
	}
	for i, c := range runner.calls {
		if c.dir != "/repos/nat" || c.name != Binary {
			t.Errorf("call %d ran %q in %q, want %q in the slice's repository", i, c.name, c.dir, Binary)
		}
		if !reflect.DeepEqual(c.args, want[i]) {
			t.Errorf("call %d args = %v, want %v", i, c.args, want[i])
		}
	}
}

// TestConflictsWithBaseReadsGitsAnswer derives each state from what git
// actually writes and exits with.
func TestConflictsWithBaseReadsGitsAnswer(t *testing.T) {
	cases := []struct {
		name string
		out  string
		err  error
		want MergeState
	}{
		{"clean", cleanMerge, nil, MergeClean},
		{"conflicted", conflictedMerge, &ExitError{Code: 1}, MergeConflicted},
		{"a SHA-256 repository", "6c2f0e29a4d8f3b5c7e1a9d0b2c4e6f8a1b3c5d7e9f0a2b4c6d8e0f1a3b5c7d9\nf\n",
			&ExitError{Code: 1}, MergeConflicted},
		{"an unresolvable branch", "", &ExitError{Code: 1, Stderr: unresolvedStderr}, MergeUnknown},
		{"a git with no --write-tree", "", &ExitError{Code: 129, Stderr: "usage: git merge-tree"}, MergeUnknown},
		{"a tree but a fatal exit", cleanMerge, &ExitError{Code: 128}, MergeUnknown},
		{"no git at all", "", exec.ErrNotFound, MergeUnknown},
		{"a clean exit with no tree", "", nil, MergeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{
				outs: []string{"", "origin/main\n", tc.out},
				errs: []error{nil, nil, tc.err},
			}
			if got := NewWithRunner(runner).ConflictsWithBase("/repos/nat", "slice/x"); got != tc.want {
				t.Errorf("ConflictsWithBase() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestConflictsWithBaseOfAMissingRepository is unknown, never an error and
// never conflicted, when every call fails the way a directory that is not
// there makes git fail.
func TestConflictsWithBaseOfAMissingRepository(t *testing.T) {
	gone := &ExitError{Code: 128, Stderr: "fatal: cannot change to '/repos/gone': No such file or directory\n"}
	runner := &fakeRunner{errs: []error{gone, gone, gone, gone}}
	if got := NewWithRunner(runner).ConflictsWithBase("/repos/gone", "slice/x"); got != MergeUnknown {
		t.Errorf("ConflictsWithBase() = %v, want MergeUnknown", got)
	}
}

// TestConflictsWithBaseAgainstRealGit runs the real binary over a scratch
// repository with no origin — the fetch fails and the base falls back to main,
// both logged — so the output shapes above are git's own, not a guess.
func TestConflictsWithBaseAgainstRealGit(t *testing.T) {
	if _, err := exec.LookPath(Binary); err != nil {
		t.Skip("no git on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(Binary, append([]string{"-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("f", "a\n")
	run("add", "f")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "slice/clean")
	write("g", "g\n")
	run("add", "g")
	run("commit", "-qm", "g")
	run("checkout", "-q", "main")
	run("checkout", "-qb", "slice/conflicted")
	write("f", "c\n")
	run("commit", "-qam", "c")
	run("checkout", "-q", "main")
	write("f", "d\n")
	run("commit", "-qam", "d")

	cli := New()
	for branch, want := range map[string]MergeState{
		"slice/clean":      MergeClean,
		"slice/conflicted": MergeConflicted,
		"slice/gone":       MergeUnknown,
	} {
		if got := cli.ConflictsWithBase(dir, branch); got != want {
			t.Errorf("ConflictsWithBase(%q) = %v, want %v", branch, got, want)
		}
	}
	if got := cli.ConflictsWithBase(filepath.Join(dir, "missing"), "slice/clean"); got != MergeUnknown {
		t.Errorf("ConflictsWithBase(missing repository) = %v, want MergeUnknown", got)
	}
}
