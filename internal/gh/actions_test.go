package gh

import (
	"errors"
	"slices"
	"testing"
	"time"
)

var jobRef = ActionsRef{Owner: "o", Repo: "r", Run: "11", Job: "22"}

// TestActionsJob reads a running job's status, times, runner and steps off
// the REST API, a null time as zero.
func TestActionsJob(t *testing.T) {
	runner := &fakeRunner{out: `{"status":"in_progress","conclusion":null,
		"created_at":"2026-10-05T11:04:00Z","started_at":"2026-10-05T11:04:09Z","runner_name":"GitHub Actions 7",
		"steps":[
			{"name":"Set up job","status":"completed","conclusion":"success","started_at":"2026-10-05T11:04:09Z","completed_at":"2026-10-05T11:04:10Z"},
			{"name":"Test","status":"in_progress","conclusion":null,"started_at":"2026-10-05T11:04:10Z","completed_at":null},
			{"name":"Post","status":"pending","conclusion":null,"started_at":null,"completed_at":null}]}`}
	job, err := NewWithRunner(runner).ActionsJob("/repo", jobRef)
	if err != nil {
		t.Fatalf("ActionsJob: %v", err)
	}
	if want := []string{"api", "repos/o/r/actions/jobs/22"}; !slices.Equal(runner.args, want) || runner.dir != "/repo" {
		t.Errorf("ran %v in %q, want %v", runner.args, runner.dir, want)
	}
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	if job.Status != JobInProgress || job.Conclusion != "" || job.Runner != "GitHub Actions 7" ||
		!job.CreatedAt.Equal(at("2026-10-05T11:04:00Z")) || !job.StartedAt.Equal(at("2026-10-05T11:04:09Z")) {
		t.Errorf("job = %+v", job)
	}
	want := []JobStep{
		{Name: "Set up job", Status: "completed", Conclusion: "success", StartedAt: at("2026-10-05T11:04:09Z"), CompletedAt: at("2026-10-05T11:04:10Z")},
		{Name: "Test", Status: "in_progress", StartedAt: at("2026-10-05T11:04:10Z")},
		{Name: "Post", Status: "pending"},
	}
	if !slices.EqualFunc(job.Steps, want, func(a, b JobStep) bool {
		return a.Name == b.Name && a.Status == b.Status && a.Conclusion == b.Conclusion &&
			a.StartedAt.Equal(b.StartedAt) && a.CompletedAt.Equal(b.CompletedAt)
	}) {
		t.Errorf("steps = %+v, want %+v", job.Steps, want)
	}
}

// TestActionsJobRefusals: a ref short of its repository or job is refused
// before gh runs; a refusal and unreadable JSON are errors.
func TestActionsJobRefusals(t *testing.T) {
	for _, ref := range []ActionsRef{{Repo: "r", Job: "1"}, {Owner: "o", Job: "1"}, {Owner: "o", Repo: "r", Run: "1"}} {
		runner := &fakeRunner{}
		if _, err := NewWithRunner(runner).ActionsJob("/repo", ref); err == nil || runner.runs != 0 {
			t.Errorf("ActionsJob(%+v) = %v after %d runs, want a refusal before gh", ref, err, runner.runs)
		}
		if _, err := NewWithRunner(runner).JobLog("/repo", ref); err == nil || runner.runs != 0 {
			t.Errorf("JobLog(%+v) = %v after %d runs, want a refusal before gh", ref, err, runner.runs)
		}
	}
	if _, err := NewWithRunner(&fakeRunner{err: &ExitError{Code: 1, Stderr: "gh: Not Found (HTTP 404)"}}).ActionsJob("/repo", jobRef); err == nil {
		t.Error("ActionsJob over a refusal: want an error")
	}
	if _, err := NewWithRunner(&fakeRunner{out: "nope"}).ActionsJob("/repo", jobRef); err == nil {
		t.Error("ActionsJob over unreadable JSON: want an error")
	}
}

// TestJobLog reads a job's log trimmed, tells GitHub's not-yet refusal apart
// as ErrLogNotReady, and returns any other failure as it came.
func TestJobLog(t *testing.T) {
	runner := &fakeRunner{out: "line 1\nline 2\n"}
	out, err := NewWithRunner(runner).JobLog("/repo", jobRef)
	if err != nil || out != "line 1\nline 2" {
		t.Fatalf("JobLog = %q, %v", out, err)
	}
	if want := []string{"api", "repos/o/r/actions/jobs/22/logs"}; !slices.Equal(runner.args, want) {
		t.Errorf("ran %v, want %v", runner.args, want)
	}
	notYet := &fakeRunner{
		out: `<?xml version="1.0" encoding="utf-8"?><Error><Code>BlobNotFound</Code></Error>`,
		err: &ExitError{Code: 1, Stderr: "gh: HTTP 404\n"},
	}
	if _, err := NewWithRunner(notYet).JobLog("/repo", jobRef); !errors.Is(err, ErrLogNotReady) {
		t.Errorf("JobLog of a running job = %v, want ErrLogNotReady", err)
	}
	other := &ExitError{Code: 1, Stderr: "gh: Not Found (HTTP 404)"}
	if _, err := NewWithRunner(&fakeRunner{err: other}).JobLog("/repo", jobRef); err != other {
		t.Errorf("JobLog over another refusal = %v, want it as it came", err)
	}
}

// TestRunStatus reads a run's status word, pinned to its repository.
func TestRunStatus(t *testing.T) {
	runner := &fakeRunner{out: `{"status":"completed"}`}
	status, err := NewWithRunner(runner).RunStatus("/repo", jobRef)
	if err != nil || status != RunCompleted {
		t.Fatalf("RunStatus = %q, %v", status, err)
	}
	if want := []string{"run", "view", "11", "--json", "status", "--repo", "o/r"}; !slices.Equal(runner.args, want) {
		t.Errorf("ran %v, want %v", runner.args, want)
	}
	if _, err := NewWithRunner(&fakeRunner{}).RunStatus("/repo", ActionsRef{}); err == nil {
		t.Error("RunStatus with no run: want a refusal")
	}
	if _, err := NewWithRunner(&fakeRunner{err: errors.New("gh missing")}).RunStatus("/repo", jobRef); err == nil {
		t.Error("RunStatus over a failure: want an error")
	}
	if _, err := NewWithRunner(&fakeRunner{out: "nope"}).RunStatus("/repo", jobRef); err == nil {
		t.Error("RunStatus over unreadable JSON: want an error")
	}
}

// TestRunActions sends each of cancel, re-run, re-run failed and re-run job
// with exactly gh's arguments — pinned to the run's repository where the ref
// names it, not where it does not — and refuses one missing its id.
func TestRunActions(t *testing.T) {
	runOnly := ActionsRef{Run: "11", Job: "22"}
	tests := []struct {
		name string
		call func(CLI) error
		want []string
	}{
		{"cancel", func(c CLI) error { return c.CancelRun("/repo", jobRef) }, []string{"run", "cancel", "11", "--repo", "o/r"}},
		{"rerun", func(c CLI) error { return c.RerunRun("/repo", jobRef, false) }, []string{"run", "rerun", "11", "--repo", "o/r"}},
		{"rerun failed", func(c CLI) error { return c.RerunRun("/repo", jobRef, true) }, []string{"run", "rerun", "11", "--failed", "--repo", "o/r"}},
		{"rerun job", func(c CLI) error { return c.RerunJob("/repo", jobRef) }, []string{"run", "rerun", "--job", "22", "--repo", "o/r"}},
		{"unpinned", func(c CLI) error { return c.RerunJob("/repo", runOnly) }, []string{"run", "rerun", "--job", "22"}},
	}
	for _, tt := range tests {
		runner := &fakeRunner{}
		if err := tt.call(NewWithRunner(runner)); err != nil {
			t.Errorf("%s: %v", tt.name, err)
		}
		if !slices.Equal(runner.args, tt.want) || runner.dir != "/repo" {
			t.Errorf("%s ran %v in %q, want %v", tt.name, runner.args, runner.dir, tt.want)
		}
		refused := &fakeRunner{err: &ExitError{Code: 1}}
		if err := tt.call(NewWithRunner(refused)); err == nil {
			t.Errorf("%s over a refusal: want an error", tt.name)
		}
	}
	for name, call := range map[string]func(CLI) error{
		"cancel":    func(c CLI) error { return c.CancelRun("/repo", ActionsRef{}) },
		"rerun":     func(c CLI) error { return c.RerunRun("/repo", ActionsRef{}, false) },
		"rerun job": func(c CLI) error { return c.RerunJob("/repo", ActionsRef{Run: "1"}) },
	} {
		runner := &fakeRunner{}
		if err := call(NewWithRunner(runner)); err == nil || runner.runs != 0 {
			t.Errorf("%s with no id = %v after %d runs, want a refusal before gh", name, err, runner.runs)
		}
	}
}

// TestExitCode is gh's code where it refused and -1 where it never ran.
func TestExitCode(t *testing.T) {
	if got := exitCode(&ExitError{Code: 4}); got != 4 {
		t.Errorf("exitCode = %d, want 4", got)
	}
	if got := exitCode(errors.New("not found")); got != -1 {
		t.Errorf("exitCode = %d, want -1", got)
	}
}
