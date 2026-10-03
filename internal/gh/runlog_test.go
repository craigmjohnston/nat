package gh

import (
	"errors"
	"slices"
	"testing"
)

// TestActionsRun reads a run and a job off an Actions check's URL, a run alone
// off a run's, and nothing off a status some other service reported.
func TestActionsRun(t *testing.T) {
	tests := []struct {
		url, run, job string
		ok            bool
	}{
		{url: "https://github.com/o/r/actions/runs/123/job/456", run: "123", job: "456", ok: true},
		{url: "https://github.com/o/r/actions/runs/123", run: "123", ok: true},
		{url: "https://ci.example.com/build/9"},
		{url: ""},
	}
	for _, tt := range tests {
		run, job, ok := ActionsRun(tt.url)
		if run != tt.run || job != tt.job || ok != tt.ok {
			t.Errorf("ActionsRun(%q) = %q, %q, %v, want %q, %q, %v", tt.url, run, job, ok, tt.run, tt.job, tt.ok)
		}
	}
}

// TestFailedLog asks gh for one job's failed steps where a job is named, and
// for the run's where only the run is, trailing newlines trimmed.
func TestFailedLog(t *testing.T) {
	runner := &fakeRunner{out: "step failed\n\n"}
	out, err := NewWithRunner(runner).FailedLog("/repo", "123", "456")
	if err != nil || out != "step failed" {
		t.Fatalf("FailedLog = %q, %v, want the log", out, err)
	}
	if want := []string{"run", "view", "--job", "456", "--log-failed"}; !slices.Equal(runner.args, want) || runner.dir != "/repo" {
		t.Errorf("ran %v in %q, want %v in /repo", runner.args, runner.dir, want)
	}
	if _, err := NewWithRunner(runner).FailedLog("/repo", "123", ""); err != nil {
		t.Fatalf("FailedLog(run) = %v", err)
	}
	if want := []string{"run", "view", "123", "--log-failed"}; !slices.Equal(runner.args, want) {
		t.Errorf("ran %v, want %v", runner.args, want)
	}
}

// TestFailedLogRefusals refuses a read naming nothing before gh runs, and
// passes a gh that failed back as itself.
func TestFailedLogRefusals(t *testing.T) {
	runner := &fakeRunner{}
	if _, err := NewWithRunner(runner).FailedLog("/repo", "", ""); err == nil || runner.runs != 0 {
		t.Errorf("FailedLog naming nothing = %v after %d runs, want a refusal before gh", err, runner.runs)
	}
	runner.err = errors.New("boom")
	if _, err := NewWithRunner(runner).FailedLog("/repo", "1", ""); err == nil {
		t.Error("FailedLog: want the failure surfaced")
	}
}
