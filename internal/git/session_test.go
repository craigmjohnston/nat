package git

import (
	"errors"
	"testing"
)

func TestCurrentBranch(t *testing.T) {
	runner := &fakeRunner{outs: []string{"session/abcd1234\n"}}
	got, err := NewWithRunner(runner).CurrentBranch("/repo")
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if got != "session/abcd1234" {
		t.Errorf("CurrentBranch = %q, want %q", got, "session/abcd1234")
	}
}

// A detached HEAD is what symbolic-ref refuses over, and is not an error
// worth reporting — it is an ordinary state to be in.
func TestCurrentBranchDetachedHEAD(t *testing.T) {
	runner := &fakeRunner{errs: []error{&ExitError{Code: 1}}}
	got, err := NewWithRunner(runner).CurrentBranch("/repo")
	if err != nil {
		t.Fatalf("CurrentBranch on a detached HEAD: %v, want no error", err)
	}
	if got != "" {
		t.Errorf("CurrentBranch on a detached HEAD = %q, want empty", got)
	}
}

func TestCurrentBranchOtherFailure(t *testing.T) {
	boom := errors.New("boom")
	runner := &fakeRunner{errs: []error{boom}}
	if _, err := NewWithRunner(runner).CurrentBranch("/repo"); err == nil {
		t.Fatal("CurrentBranch: want an error surfaced for a non-git failure")
	}
}

func TestReflogBranches(t *testing.T) {
	log := "checkout: moving from main to session/abcd1234\n" +
		"some other reflog entry\n" +
		"checkout: moving from session/abcd1234 to feature/two\n" +
		"checkout: moving from feature/two to session/abcd1234\n"
	runner := &fakeRunner{outs: []string{log}}
	got, err := NewWithRunner(runner).ReflogBranches("/repo")
	if err != nil {
		t.Fatalf("ReflogBranches: %v", err)
	}
	want := []string{"main", "session/abcd1234", "feature/two"}
	if len(got) != len(want) {
		t.Fatalf("ReflogBranches = %v, want %v", got, want)
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("ReflogBranches[%d] = %q, want %q", i, got[i], b)
		}
	}
}

func TestReflogBranchesFailure(t *testing.T) {
	runner := &fakeRunner{errs: []error{errors.New("boom")}}
	if _, err := NewWithRunner(runner).ReflogBranches("/repo"); err == nil {
		t.Fatal("ReflogBranches: want the failure surfaced")
	}
}

func TestDiffWorkingTreeFrom(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/main\n", "diff --git a/x b/x\n"}}
	base, diff, err := NewWithRunner(runner).DiffWorkingTreeFrom("/repo", "")
	if err != nil {
		t.Fatalf("DiffWorkingTreeFrom: %v", err)
	}
	if base != "origin/main" {
		t.Errorf("base = %q, want origin/main", base)
	}
	if diff != "diff --git a/x b/x\n" {
		t.Errorf("diff = %q, want git's own diff", diff)
	}
	last := runner.calls[len(runner.calls)-1]
	for _, a := range last.args {
		if a == "" {
			t.Errorf("args = %v, want no empty branch argument", last.args)
		}
	}
}

func TestDiffWorkingTreeFromFailure(t *testing.T) {
	runner := &fakeRunner{outs: []string{"origin/main\n"}, errs: []error{nil, errors.New("boom")}}
	if _, _, err := NewWithRunner(runner).DiffWorkingTreeFrom("/repo", ""); err == nil {
		t.Fatal("DiffWorkingTreeFrom: want the failure surfaced")
	}
}
