package gh

import (
	"errors"
	"reflect"
	"testing"
)

func TestEditReviewersRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	err := NewWithRunner(runner).EditReviewers("/repos/nat", "https://github.test/o/r/pull/7",
		[]string{"octocat", " ", "org/core"}, []string{"hubot"})
	if err != nil {
		t.Fatalf("EditReviewers() = %v", err)
	}
	if runner.dir != "/repos/nat" || runner.name != Binary {
		t.Errorf("ran %q in %q", runner.name, runner.dir)
	}
	want := []string{"pr", "edit", "https://github.test/o/r/pull/7",
		"--add-reviewer", "octocat,org/core", "--remove-reviewer", "hubot"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

func TestEditReviewersOnlyRemoving(t *testing.T) {
	runner := &fakeRunner{}
	if err := NewWithRunner(runner).EditReviewers("/r", "7", nil, []string{"hubot"}); err != nil {
		t.Fatalf("EditReviewers() = %v", err)
	}
	if want := []string{"pr", "edit", "7", "--remove-reviewer", "hubot"}; !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

func TestEditReviewersRefusesBeforeGhRuns(t *testing.T) {
	runner := &fakeRunner{}
	if err := NewWithRunner(runner).EditReviewers("/r", "", []string{"a"}, nil); err == nil {
		t.Error("an empty ref was not refused")
	}
	if err := NewWithRunner(runner).EditReviewers("/r", "7", []string{" "}, nil); err == nil {
		t.Error("an edit naming nobody was not refused")
	}
	if runner.runs != 0 {
		t.Errorf("gh ran %d times, want none", runner.runs)
	}
}

func TestEditReviewersFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "could not request reviewer: octocat is the author\n"}
	err := NewWithRunner(&fakeRunner{err: refusal}).EditReviewers("/r", "7", []string{"octocat"}, nil)
	if !errors.Is(err, refusal) {
		t.Errorf("EditReviewers() = %v, want gh's refusal", err)
	}
}

func TestCollaborators(t *testing.T) {
	runner := &fakeRunner{out: "octocat\nhubot\n\n"}
	got, err := NewWithRunner(runner).Collaborators("/repos/nat")
	if err != nil {
		t.Fatalf("Collaborators() = %v", err)
	}
	if want := []string{"octocat", "hubot"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Collaborators() = %v, want %v", got, want)
	}
	want := []string{"api", "repos/{owner}/{repo}/collaborators", "--paginate", "--jq", ".[].login"}
	if !reflect.DeepEqual(runner.args, want) || runner.dir != "/repos/nat" {
		t.Errorf("ran %v in %q", runner.args, runner.dir)
	}
}

func TestCollaboratorsFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "HTTP 403\n"}
	if _, err := NewWithRunner(&fakeRunner{err: refusal}).Collaborators("/r"); !errors.Is(err, refusal) {
		t.Errorf("Collaborators() = %v, want gh's refusal", err)
	}
}
