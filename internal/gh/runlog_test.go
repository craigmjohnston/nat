package gh

import (
	"errors"
	"slices"
	"testing"
)

// TestActionsRun reads the repository, run and job off an Actions check's
// URL, the repository and run alone off a run's, and nothing off a status some
// other service reported.
func TestActionsRun(t *testing.T) {
	tests := []struct {
		url  string
		want ActionsRef
		ok   bool
	}{
		{url: "https://github.com/o/r/actions/runs/123/job/456", want: ActionsRef{Owner: "o", Repo: "r", Run: "123", Job: "456"}, ok: true},
		{url: "https://github.com/o/r/actions/runs/123", want: ActionsRef{Owner: "o", Repo: "r", Run: "123"}, ok: true},
		{url: "/actions/runs/123/job/456", want: ActionsRef{Run: "123", Job: "456"}, ok: true},
		{url: "https://ci.example.com/build/9"},
		{url: ""},
	}
	for _, tt := range tests {
		got, ok := ActionsRun(tt.url)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ActionsRun(%q) = %+v, %v, want %+v, %v", tt.url, got, ok, tt.want, tt.ok)
		}
	}
}

// scriptedRunner answers each gh call in turn from outs/errs, recording every
// call's args — FailedLog's fallback is a second call after a first refused.
type scriptedRunner struct {
	outs  []string
	errs  []error
	calls [][]string
}

func (s *scriptedRunner) Run(dir, name string, args ...string) (string, error) {
	i := len(s.calls)
	s.calls = append(s.calls, args)
	return s.outs[i], s.errs[i]
}

var inProgress = &ExitError{Code: 1, Stderr: "run 123 is still in progress; logs will be available when it is complete\n"}

// TestFailedLog asks gh for one job's failed steps where a job is named, and
// for the run's where only the run is, trailing newlines trimmed.
func TestFailedLog(t *testing.T) {
	runner := &fakeRunner{out: "step failed\n\n"}
	out, err := NewWithRunner(runner).FailedLog("/repo", ActionsRef{Owner: "o", Repo: "r", Run: "123", Job: "456"})
	if err != nil || out != "step failed" {
		t.Fatalf("FailedLog = %q, %v, want the log", out, err)
	}
	if want := []string{"run", "view", "--job", "456", "--log-failed"}; !slices.Equal(runner.args, want) || runner.dir != "/repo" || runner.runs != 1 {
		t.Errorf("ran %v in %q (%d runs), want %v in /repo once", runner.args, runner.dir, runner.runs, want)
	}
	if _, err := NewWithRunner(runner).FailedLog("/repo", ActionsRef{Run: "123"}); err != nil {
		t.Fatalf("FailedLog(run) = %v", err)
	}
	if want := []string{"run", "view", "123", "--log-failed"}; !slices.Equal(runner.args, want) {
		t.Errorf("ran %v, want %v", runner.args, want)
	}
}

// TestFailedLogWhileTheRunGoesOn reads a failed job's own log through the
// REST API, in the repository its URL names, where gh refuses the failed
// steps' read because a sibling job is still running.
func TestFailedLogWhileTheRunGoesOn(t *testing.T) {
	runner := &scriptedRunner{outs: []string{"", "whole job log\n"}, errs: []error{inProgress, nil}}
	out, err := NewWithRunner(runner).FailedLog("/repo", ActionsRef{Owner: "o", Repo: "r", Run: "123", Job: "456"})
	if err != nil || out != "whole job log" {
		t.Fatalf("FailedLog = %q, %v, want the job's log", out, err)
	}
	want := [][]string{
		{"run", "view", "--job", "456", "--log-failed"},
		{"api", "repos/o/r/actions/jobs/456/logs"},
	}
	if !slices.EqualFunc(runner.calls, want, slices.Equal) {
		t.Errorf("ran %v, want %v", runner.calls, want)
	}

	runner = &scriptedRunner{outs: []string{"", ""}, errs: []error{inProgress, errors.New("not found")}}
	if _, err := NewWithRunner(runner).FailedLog("/repo", ActionsRef{Owner: "o", Repo: "r", Run: "123", Job: "456"}); err == nil || err.Error() != "not found" {
		t.Errorf("FailedLog over a failed fallback = %v, want the fallback's error", err)
	}
}

// TestFailedLogNoFallback returns gh's refusal as the error, with no second
// call, where it refused for another reason, or where the URL named no job
// (a run's log is only readable once it completes) or no repository.
func TestFailedLogNoFallback(t *testing.T) {
	tests := []struct {
		name string
		ref  ActionsRef
		err  error
	}{
		{"another refusal", ActionsRef{Owner: "o", Repo: "r", Run: "1", Job: "2"}, &ExitError{Code: 1, Stderr: "HTTP 404: Not Found"}},
		{"not an exit", ActionsRef{Owner: "o", Repo: "r", Run: "1", Job: "2"}, errors.New("still in progress, but not from gh")},
		{"no job", ActionsRef{Owner: "o", Repo: "r", Run: "1"}, inProgress},
		{"no repository", ActionsRef{Run: "1", Job: "2"}, inProgress},
	}
	for _, tt := range tests {
		runner := &scriptedRunner{outs: []string{""}, errs: []error{tt.err}}
		if _, err := NewWithRunner(runner).FailedLog("/repo", tt.ref); !errors.Is(err, tt.err) || len(runner.calls) != 1 {
			t.Errorf("%s: FailedLog = %v after %d calls, want %v after one", tt.name, err, len(runner.calls), tt.err)
		}
	}
}

// TestFailedLogRefusals refuses a read naming nothing before gh runs.
func TestFailedLogRefusals(t *testing.T) {
	runner := &fakeRunner{}
	if _, err := NewWithRunner(runner).FailedLog("/repo", ActionsRef{}); err == nil || runner.runs != 0 {
		t.Errorf("FailedLog naming nothing = %v after %d runs, want a refusal before gh", err, runner.runs)
	}
}
