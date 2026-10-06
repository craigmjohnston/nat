package gh

import (
	"errors"
	"strings"
	"testing"
)

// TestEditPRBodyRunsGh pins the invocation: gh, in the slice's repository,
// told which pull request to edit and to read the new body off its own stdin.
func TestEditPRBodyRunsGh(t *testing.T) {
	runner := &fakeRunner{out: "https://github.test/craig/nat/pull/7\n"}
	if err := NewWithRunner(runner).EditPRBody("/repos/nat", "7", "A new description."); err != nil {
		t.Fatalf("EditPRBody() = %v, want the description written", err)
	}
	if runner.dir != "/repos/nat" || runner.name != Binary {
		t.Errorf("ran %q in %q, want %q in the slice's repository", runner.name, runner.dir, Binary)
	}
	want := []string{"pr", "edit", "7", "--body-file", "-"}
	if strings.Join(runner.args, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
	if runner.stdin != "A new description." {
		t.Errorf("stdin = %q, want the description", runner.stdin)
	}
}

// TestEditPRBodyRefusesBeforeGh refuses an unnamed pull request and an empty
// body without running gh at all.
func TestEditPRBodyRefusesBeforeGh(t *testing.T) {
	for _, tc := range []struct{ ref, body, want string }{
		{"", "a description", "needs a pull request"},
		{"7", "  \n", "needs a description"},
	} {
		runner := &fakeRunner{}
		err := NewWithRunner(runner).EditPRBody("/repos/nat", tc.ref, tc.body)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("EditPRBody(%q, %q) = %v, want %q", tc.ref, tc.body, err, tc.want)
		}
		if runner.runs != 0 {
			t.Errorf("ran gh %d times, want it not run at all", runner.runs)
		}
	}
}

// TestEditPRBodyNeedsAStdinRunner refuses a runner that cannot carry the body.
func TestEditPRBodyNeedsAStdinRunner(t *testing.T) {
	err := NewWithRunner(noStdinRunner{}).EditPRBody("/repos/nat", "7", "a description")
	if err == nil || !strings.Contains(err.Error(), "cannot carry a description") {
		t.Errorf("EditPRBody() = %v, want it to refuse a runner with no stdin", err)
	}
}

// TestEditPRBodyFailure hands gh's own words back.
func TestEditPRBodyFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "GraphQL: Could not resolve to a PullRequest\n"}
	err := NewWithRunner(&fakeRunner{err: refusal}).EditPRBody("/repos/nat", "7", "a description")
	if !errors.Is(err, error(refusal)) {
		t.Errorf("EditPRBody() = %v, want gh's own refusal", err)
	}
}
