package git

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// argsOf is every call's args, in order.
func argsOf(r *fakeRunner) [][]string {
	var all [][]string
	for _, c := range r.calls {
		all = append(all, c.args)
	}
	return all
}

// TestRebaseInProgressFindsNone asks git for both rebase directories, relative
// to the worktree, and finds neither there.
func TestRebaseInProgressFindsNone(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{outs: []string{".git/rebase-merge\n", ".git/rebase-apply\n"}}
	conflicts, underWay, err := NewWithRunner(runner).RebaseInProgress(dir)
	if err != nil || underWay || conflicts != nil {
		t.Errorf("RebaseInProgress() = %v, %v, %v — want none under way", conflicts, underWay, err)
	}
	want := [][]string{{"rev-parse", "--git-path", "rebase-merge"}, {"rev-parse", "--git-path", "rebase-apply"}}
	if got := argsOf(runner); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// A rebase directory there — named relative to the worktree, or absolute as
// a linked worktree's is — is a rebase under way, and its conflicted paths
// are read.
func TestRebaseInProgressFindsOne(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "rebase-apply"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"relative": ".git/rebase-apply\n", "absolute": filepath.Join(dir, ".git", "rebase-apply") + "\n"} {
		t.Run(name, func(t *testing.T) {
			runner := &fakeRunner{outs: []string{".git/rebase-merge\n", path, "f\nsub/g\n"}}
			conflicts, underWay, err := NewWithRunner(runner).RebaseInProgress(dir)
			if err != nil || !underWay || !reflect.DeepEqual(conflicts, []string{"f", "sub/g"}) {
				t.Errorf("RebaseInProgress() = %v, %v, %v — want under way on f and sub/g", conflicts, underWay, err)
			}
			if got := runner.calls[2].args; !reflect.DeepEqual(got, []string{"diff", "--name-only", "--diff-filter=U"}) {
				t.Errorf("conflicts read with %v", got)
			}
		})
	}
}

// A reading that cannot be made is an error, never "none under way": git
// could not name the directory, or it could not be looked at.
func TestRebaseInProgressCannotTell(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, runner := range map[string]*fakeRunner{
		"no git dir":   {errs: []error{&ExitError{Code: 128, Stderr: "fatal: not a git repository"}}},
		"not a dir":    {outs: []string{"file/rebase-merge\n"}},
		"unreadable U": {outs: []string{".", ""}, errs: []error{nil, errors.New("boom")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := NewWithRunner(runner).RebaseInProgress(dir); err == nil {
				t.Error("RebaseInProgress() = nil error, want the failed reading")
			}
		})
	}
}

// A rebase that goes through answers nothing conflicted.
func TestRebaseClean(t *testing.T) {
	runner := &fakeRunner{}
	conflicts, err := NewWithRunner(runner).Rebase("/w", "origin/main")
	if err != nil || conflicts != nil {
		t.Errorf("Rebase() = %v, %v — want clean", conflicts, err)
	}
	if got := argsOf(runner); !reflect.DeepEqual(got, [][]string{{"rebase", "origin/main"}}) {
		t.Errorf("args = %v", got)
	}
}

// A rebase stopped on a conflict answers the paths and is left stopped.
func TestRebaseStopsOnAConflict(t *testing.T) {
	runner := &fakeRunner{outs: []string{"", "f\n"}, errs: []error{&ExitError{Code: 1}}}
	conflicts, err := NewWithRunner(runner).Rebase("/w", "origin/main")
	if err != nil || !reflect.DeepEqual(conflicts, []string{"f"}) {
		t.Errorf("Rebase() = %v, %v — want stopped on f", conflicts, err)
	}
	if len(runner.calls) != 2 {
		t.Errorf("made %d calls, want no abort: %v", len(runner.calls), argsOf(runner))
	}
}

// Any other failure is aborted and returned — whether nothing is conflicted,
// the conflicts could not be read, or the abort itself fails.
func TestRebaseAbortsAnyOtherFailure(t *testing.T) {
	refused := &ExitError{Code: 1, Stderr: "error: cannot rebase: You have unstaged changes."}
	for name, runner := range map[string]*fakeRunner{
		"nothing conflicted": {errs: []error{refused}},
		"unreadable":         {errs: []error{refused, errors.New("boom")}},
		"abort fails":        {errs: []error{refused, nil, errors.New("no rebase in progress")}},
	} {
		t.Run(name, func(t *testing.T) {
			conflicts, err := NewWithRunner(runner).Rebase("/w", "origin/main")
			if !errors.Is(err, refused) || conflicts != nil {
				t.Errorf("Rebase() = %v, %v — want the refusal", conflicts, err)
			}
			want := [][]string{{"rebase", "origin/main"}, {"diff", "--name-only", "--diff-filter=U"}, {"rebase", "--abort"}}
			if got := argsOf(runner); !reflect.DeepEqual(got, want) {
				t.Errorf("args = %v, want %v", got, want)
			}
		})
	}
}
