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
	if err := NewWithRunner(runner).MergePR("/repos/nat", "https://github.test/craig/nat/pull/7", MergeOptions{}); err != nil {
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

// The ref goes through as it stands, not just the pull request number gh
// prefers.
func TestMergePRTakesTheRefAsItStands(t *testing.T) {
	ref := "slice/merge-the-pr-from-the-viewer"
	runner := &fakeRunner{}
	if err := NewWithRunner(runner).MergePR("/repos/nat", ref, MergeOptions{}); err != nil {
		t.Fatalf("MergePR(%q) = %v, want the merge to have happened", ref, err)
	}
	if runner.args[2] != ref {
		t.Errorf("merged %q, want %q", runner.args[2], ref)
	}
}

// A merge with nothing named would merge whatever branch the directory happens
// to be on, so it is refused before gh is run at all.
func TestMergePRNeedsARef(t *testing.T) {
	runner := &fakeRunner{}
	err := NewWithRunner(runner).MergePR("/repos/nat", "", MergeOptions{})
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
	err := NewWithRunner(&fakeRunner{err: refusal}).MergePR("/repos/nat", "7", MergeOptions{})
	if !errors.Is(err, error(refusal)) {
		t.Fatalf("MergePR() = %v, want gh's own refusal", err)
	}
	if !strings.Contains(err.Error(), "base branch policy") {
		t.Errorf("error = %q, want gh's first stderr line", err)
	}
}

// TestMergePRAsConfigured: the project's strategy is the flag, and
// --delete-branch rides only where it is asked for.
func TestMergePRAsConfigured(t *testing.T) {
	for _, tt := range []struct {
		opts MergeOptions
		want []string
	}{
		{MergeOptions{Method: "squash"}, []string{"pr", "merge", "7", "--squash"}},
		{MergeOptions{Method: "rebase", DeleteBranch: true}, []string{"pr", "merge", "7", "--rebase", "--delete-branch"}},
		{MergeOptions{DeleteBranch: true}, []string{"pr", "merge", "7", "--merge", "--delete-branch"}},
	} {
		runner := &fakeRunner{}
		if err := NewWithRunner(runner).MergePR("/repos/nat", "7", tt.opts); err != nil {
			t.Fatalf("MergePR(%+v) = %v", tt.opts, err)
		}
		if !reflect.DeepEqual(runner.args, tt.want) {
			t.Errorf("MergePR(%+v) args = %v, want %v", tt.opts, runner.args, tt.want)
		}
	}
}

// TestMergePRRefusesAnUnknownMethod: a word gh has no flag for is refused
// before gh runs.
func TestMergePRRefusesAnUnknownMethod(t *testing.T) {
	runner := &fakeRunner{}
	err := NewWithRunner(runner).MergePR("/repos/nat", "7", MergeOptions{Method: "octopus"})
	if err == nil || !strings.Contains(err.Error(), `no merge method "octopus"`) {
		t.Errorf("MergePR() = %v, want the method refused", err)
	}
	if runner.name != "" {
		t.Errorf("gh ran (%v), want nothing run", runner.args)
	}
}
