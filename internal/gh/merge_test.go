package gh

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestMergePRRunsGh pins the invocation: gh, in the slice's repository, told
// which pull request to merge and which strategy to merge it with — the flag
// being what keeps gh from asking at a prompt nothing here could answer.
func TestMergePRRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	if err := NewWithRunner(runner).MergePR("/repos/nat", "https://github.test/craig/nat/pull/7"); err != nil {
		t.Fatalf("MergePR() = %v, want the merge to have happened", err)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "merge", "https://github.test/craig/nat/pull/7", "--merge"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// The ref goes through as it stands, whichever of the three things gh merges a
// pull request by it is.
func TestMergePRTakesTheRefAsItStands(t *testing.T) {
	for _, ref := range []string{"7", "slice/merge-the-pr-from-the-viewer",
		"https://github.com/craigmjohnston/nat/pull/7"} {
		runner := &fakeRunner{}
		if err := NewWithRunner(runner).MergePR("/repos/nat", ref); err != nil {
			t.Fatalf("MergePR(%q) = %v, want the merge to have happened", ref, err)
		}
		if runner.args[2] != ref {
			t.Errorf("merged %q, want %q", runner.args[2], ref)
		}
	}
}

// A merge with nothing named would merge whatever branch the directory happens
// to be on, so it is refused before gh is run at all.
func TestMergePRNeedsARef(t *testing.T) {
	runner := &fakeRunner{}
	err := NewWithRunner(runner).MergePR("/repos/nat", "")
	if err == nil {
		t.Fatal("MergePR() = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "pr merge") {
		t.Errorf("error = %q, want it to name the command", err)
	}
	if runner.runs != 0 {
		t.Errorf("ran gh %d times, want none", runner.runs)
	}
}

// A gh that ran and refused comes back as it wrote it: branch protection, a
// review dismissed by a push, a check that went red since the reading.
func TestMergePRReportsWhatGhSaid(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "Pull request is not mergeable: the base branch policy prohibits the merge.\n"}
	err := NewWithRunner(&fakeRunner{err: refusal}).MergePR("/repos/nat", "7")
	if !errors.Is(err, error(refusal)) {
		t.Fatalf("MergePR() = %v, want gh's own refusal", err)
	}
	if !strings.Contains(err.Error(), "base branch policy") {
		t.Errorf("error = %q, want gh's first stderr line", err)
	}
}
