package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/store"
)

// noActions stubs [JobReader] and [RunController] for the fakes of [GH] that
// never read a job or act on a run.
type noActions struct{}

func (noActions) ActionsJob(string, gh.ActionsRef) (gh.Job, error) { return gh.Job{}, nil }
func (noActions) JobLog(string, gh.ActionsRef) (string, error)     { return "", nil }
func (noActions) RunStatus(string, gh.ActionsRef) (string, error)  { return "", nil }
func (noActions) CancelRun(string, gh.ActionsRef) error            { return nil }
func (noActions) RerunRun(string, gh.ActionsRef, bool) error       { return nil }
func (noActions) RerunJob(string, gh.ActionsRef) error             { return nil }

// ReadPRs reads nothing: every pull request asked about is unread.
func (noActions) ReadPRs(gh.BatchQuery) (gh.Batch, error) {
	return gh.Batch{PRs: map[gh.PRRef]gh.PR{}, Heads: map[gh.HeadRef][]gh.HeadPR{}}, nil
}

// PollPRs reads nothing, as ReadPRs does.
func (n noActions) PollPRs(q gh.BatchQuery) (gh.Batch, error) { return n.ReadPRs(q) }

// Outlook is the configured poll alone: no budget kept.
func (noActions) Outlook(poll time.Duration) gh.Outlook { return gh.Outlook{PollAfter: poll} }

// noSleep stands in for the poll's wait, counting the waits.
func noSleep(t *testing.T) *int {
	t.Helper()
	slept := 0
	old := checksSleep
	checksSleep = func(time.Duration) { slept++ }
	t.Cleanup(func() { checksSleep = old })
	return &slept
}

// ciChecks is a pull request's checks across two runs of o/r and another
// service: run 11 still going (test running, lint queued, build passed), run
// 12 finished with one failure.
func ciChecks() []gh.Check {
	return []gh.Check{
		{Name: "test", State: "IN_PROGRESS", URL: "https://github.com/o/r/actions/runs/11/job/21"},
		{Name: "lint", State: "QUEUED", URL: "https://github.com/o/r/actions/runs/11/job/22"},
		{Name: "build", State: "SUCCESS", URL: "https://github.com/o/r/actions/runs/11/job/23"},
		{Name: "macos", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/12/job/31"},
		{Name: "docs", State: "SUCCESS", URL: "https://github.com/o/r/actions/runs/12/job/32"},
		{Name: "deploy", State: "FAILURE", URL: "https://ci.example.com/build/9"},
	}
}

func runCI(t *testing.T, fake *fakeChecksGH, args ...string) (string, int, error) {
	t.Helper()
	env, out := checksEnv(t, checksPR, fake)
	nudges := 0
	env.Nudge = func() { nudges++ }
	err := Run(context.Background(), append(args, testSliceID, "--project", "project-1"), env)
	return out.String(), nudges, err
}

// TestSliceChecksRerunAll cancels the run still going, waits for it to read
// completed, then re-runs both runs whole — one call a run — and says what it
// cancelled apart from what it re-ran, and the external check it skipped.
func TestSliceChecksRerunAll(t *testing.T) {
	slept := noSleep(t)
	fake := &fakeChecksGH{checks: ciChecks(), statuses: map[string][]string{"11": {"in_progress", "completed"}}}
	out, nudges, err := runCI(t, fake, "slice-checks-rerun", "--all", "--json")
	if err != nil {
		t.Fatalf("slice-checks-rerun --all: %v", err)
	}
	want := "cancel o/r 11,status 11,status 11,rerun 11,rerun 12"
	if got := strings.Join(fake.calls, ","); got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}
	if fake.views != 0 {
		t.Errorf("viewed the pull request %d times, want none: the batched reading names the runs", fake.views)
	}
	if *slept != 1 || nudges != 1 {
		t.Errorf("slept %d, nudged %d, want 1 and 1", *slept, nudges)
	}
	wantJSON := `{"cancelled":["test","lint"],"rerun":["test","lint","build","macos","docs"],"skipped":["deploy"]}`
	if got := compactJSON(t, out); got != wantJSON {
		t.Errorf("json = %s, want %s", got, wantJSON)
	}
}

// TestSliceChecksRerunFailed re-runs only the failed jobs of a run that is
// not going, skips the failed external check, and refuses where nothing has
// failed.
func TestSliceChecksRerunFailed(t *testing.T) {
	noSleep(t)
	fake := &fakeChecksGH{checks: ciChecks()}
	out, _, err := runCI(t, fake, "slice-checks-rerun", "--failed")
	if err != nil {
		t.Fatalf("slice-checks-rerun --failed: %v", err)
	}
	if got := strings.Join(fake.calls, ","); got != "rerun 12 --failed" {
		t.Errorf("calls = %s, want only run 12's failed jobs", got)
	}
	if want := "Re-ran: macos\nSkipped, no GitHub Actions run behind them: deploy\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}

	// A failure in a run still going: cancel, then the run whole, siblings
	// and all — never left cancelled.
	checks := ciChecks()
	checks[2].State = "FAILURE"
	fake = &fakeChecksGH{checks: checks}
	out, _, err = runCI(t, fake, "slice-checks-rerun", "--failed")
	if err != nil {
		t.Fatalf("slice-checks-rerun --failed over a run going: %v", err)
	}
	if got := strings.Join(fake.calls, ","); got != "cancel o/r 11,status 11,rerun 11,rerun 12 --failed" {
		t.Errorf("calls = %s", got)
	}
	if want := "Cancelled first, since a run still going cannot be re-run: test, lint\nRe-ran: test, lint, build, macos\nSkipped, no GitHub Actions run behind them: deploy\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}

	passing := []gh.Check{{Name: "test", State: "SUCCESS", URL: "https://github.com/o/r/actions/runs/11/job/21"}}
	fake = &fakeChecksGH{checks: passing}
	if _, _, err := runCI(t, fake, "slice-checks-rerun", "--failed"); err == nil || !strings.Contains(err.Error(), "no GitHub Actions check has failed") || len(fake.calls) != 0 {
		t.Errorf("--failed with nothing failed = %v after %v, want a refusal before gh", err, fake.calls)
	}
}

// TestSliceChecksRerunCheck re-runs a finished check's own job, two named in
// one finished run job by job, and one named in a run still going by
// cancelling the run and re-running it whole, its stopped sibling named as
// cancelled.
func TestSliceChecksRerunCheck(t *testing.T) {
	noSleep(t)
	fake := &fakeChecksGH{checks: ciChecks()}
	out, _, err := runCI(t, fake, "slice-checks-rerun", "--check", "macos", "--check", "docs", "--check", "macos")
	if err != nil {
		t.Fatalf("slice-checks-rerun --check: %v", err)
	}
	if got := strings.Join(fake.calls, ","); got != "rerun --job 31,rerun --job 32" {
		t.Errorf("calls = %s", got)
	}
	if out != "Re-ran: macos, docs\n" {
		t.Errorf("output = %q", out)
	}

	fake = &fakeChecksGH{checks: ciChecks()}
	out, _, err = runCI(t, fake, "slice-checks-rerun", "--check", "test", "--json")
	if err != nil {
		t.Fatalf("slice-checks-rerun --check over a run going: %v", err)
	}
	if got := strings.Join(fake.calls, ","); got != "cancel o/r 11,status 11,rerun 11" {
		t.Errorf("calls = %s", got)
	}
	if want := `{"cancelled":["test","lint"],"rerun":["test","lint","build"],"skipped":[]}`; compactJSON(t, out) != want {
		t.Errorf("json = %s, want %s", compactJSON(t, out), want)
	}

	// A check whose URL names its run alone is re-run with the run whole.
	runOnly := []gh.Check{{Name: "ci", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/40"}}
	fake = &fakeChecksGH{checks: runOnly}
	if _, _, err := runCI(t, fake, "slice-checks-rerun", "--check", "ci"); err != nil || strings.Join(fake.calls, ",") != "rerun 40" {
		t.Errorf("--check of a run-only URL = %v, calls %v, want the run whole", err, fake.calls)
	}
	fake = &fakeChecksGH{checks: runOnly, failOn: "rerun"}
	if _, _, err := runCI(t, fake, "slice-checks-rerun", "--check", "ci"); err == nil {
		t.Error("--check of a run-only URL gh refuses: want an error")
	}
}

// TestSliceChecksRerunRefusals: a usage error, a name that is no check
// (listing the checks), an external check, no pull request, a pull request
// not open, one gh cannot read, and a plan where no Actions run is behind
// any check — each before any run is acted on.
func TestSliceChecksRerunRefusals(t *testing.T) {
	noSleep(t)
	tests := []struct {
		name string
		fake *fakeChecksGH
		args []string
		want string
	}{
		{"no mode", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-rerun"}, "exactly one of"},
		{"two modes", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-rerun", "--all", "--failed"}, "exactly one of"},
		{"bad flag", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-rerun", "--bogus"}, ""},
		{"unknown", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-rerun", "--check", "nope"}, "the checks are: test, lint, build, macos, docs, deploy"},
		{"external", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-rerun", "--check", "deploy"}, "other than GitHub Actions"},
		{"merged", &fakeChecksGH{checks: ciChecks(), prState: "MERGED"}, []string{"slice-checks-rerun", "--all"}, "is merged, not open"},
		{"unread", &fakeChecksGH{viewErr: errors.New("rate limited")}, []string{"slice-checks-rerun", "--all"}, "rate limited"},
		{"none", &fakeChecksGH{checks: []gh.Check{{Name: "deploy", State: "FAILURE", URL: "https://ci.example.com/9"}}}, []string{"slice-checks-rerun", "--all"}, "no check has a GitHub Actions run"},
		{"cancel unknown", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-cancel", "--check", "nope"}, "no check is named"},
		{"cancel bad flag", &fakeChecksGH{checks: ciChecks()}, []string{"slice-checks-cancel", "--bogus"}, ""},
	}
	for _, tt := range tests {
		_, nudges, err := runCI(t, tt.fake, tt.args...)
		if err == nil || !strings.Contains(err.Error(), tt.want) || len(tt.fake.calls) != 0 || nudges != 0 {
			t.Errorf("%s: err = %v after %v (nudged %d), want %q before any run call", tt.name, err, tt.fake.calls, nudges, tt.want)
		}
	}
	for cmd, mode := range map[string]string{"slice-checks-rerun": "--all", "slice-checks-cancel": "--json"} {
		env, _ := checksEnv(t, "", &fakeChecksGH{})
		if err := Run(context.Background(), []string{cmd, testSliceID}, env); err == nil {
			t.Errorf("%s with no project: want a refusal", cmd)
		}
		if err := Run(context.Background(), []string{cmd, "--project", "project-1"}, env); err == nil {
			t.Errorf("%s with no slice: want a refusal", cmd)
		}
		if err := Run(context.Background(), []string{cmd, "not a page", mode, "--project", "project-1"}, env); err == nil {
			t.Errorf("%s of no page: want a refusal", cmd)
		}
		if err := Run(context.Background(), []string{cmd, "00000000-0000-0000-0000-000000000099", mode, "--project", "project-1"}, env); err == nil {
			t.Errorf("%s of an unknown slice: want a refusal", cmd)
		}
		if err := Run(context.Background(), []string{cmd, testSliceID, mode, "--project", "project-1"}, env); err == nil ||
			!strings.Contains(err.Error(), "no pull request recorded") {
			t.Errorf("%s with no pull request = %v, want a refusal", cmd, err)
		}
	}
}

// TestSliceChecksRerunFailures: a cancel gh refuses, a run that never reads
// completed (polled the whole bound, nothing re-run, the cancel said to have
// been sent — a status read that fails polled again) and a re-run gh refuses
// each fail the command, naming what was already sent.
func TestSliceChecksRerunFailures(t *testing.T) {
	slept := noSleep(t)
	fake := &fakeChecksGH{checks: ciChecks(), failOn: "cancel"}
	if _, nudges, err := runCI(t, fake, "slice-checks-rerun", "--all"); err == nil || !strings.Contains(err.Error(), "cancel the run of test, lint first") || nudges != 0 {
		t.Errorf("refused cancel = %v (nudged %d)", err, nudges)
	}

	fake = &fakeChecksGH{checks: ciChecks(), statuses: map[string][]string{"11": {"in_progress"}}, statusErr: errors.New("blip")}
	_, _, err := runCI(t, fake, "slice-checks-rerun", "--all")
	if err == nil || !strings.Contains(err.Error(), "cancelled test, lint, but run 11 had not finished cancelling after 2m0s — nothing was re-run") {
		t.Errorf("timeout = %v", err)
	}
	if *slept != rerunPolls || strings.Contains(strings.Join(fake.calls, ","), "rerun") {
		t.Errorf("slept %d, calls %v, want %d polls and no re-run", *slept, fake.calls, rerunPolls)
	}

	fake = &fakeChecksGH{checks: ciChecks(), failOn: "rerun 12"}
	if _, _, err := runCI(t, fake, "slice-checks-rerun", "--all"); err == nil ||
		!strings.Contains(err.Error(), "(already cancelled test, lint; re-ran test, lint, build)") {
		t.Errorf("refused re-run = %v", err)
	}
	fake = &fakeChecksGH{checks: ciChecks(), failOn: "rerun 12"}
	if _, _, err := runCI(t, fake, "slice-checks-rerun", "--failed"); err == nil {
		t.Error("refused re-run of failed jobs: want an error")
	}
	fake = &fakeChecksGH{checks: ciChecks(), failOn: "rerun --job"}
	if _, _, err := runCI(t, fake, "slice-checks-rerun", "--check", "docs"); err == nil {
		t.Error("refused re-run of a job: want an error")
	}
}

// TestSliceChecksCancel cancels every run still going, naming each check it
// stopped, and skips an external check still pending; --check cancels only
// the named check's run, siblings named; nothing going is refused; a refused
// cancel fails naming what was already sent.
func TestSliceChecksCancel(t *testing.T) {
	checks := append(ciChecks(),
		gh.Check{Name: "e2e", State: "IN_PROGRESS", URL: "https://github.com/x/y/actions/runs/50/job/51"},
		gh.Check{Name: "vercel", State: "PENDING", URL: "https://vercel.example.com/1"})
	fake := &fakeChecksGH{checks: checks}
	out, nudges, err := runCI(t, fake, "slice-checks-cancel")
	if err != nil {
		t.Fatalf("slice-checks-cancel: %v", err)
	}
	if got := strings.Join(fake.calls, ","); got != "cancel o/r 11,cancel x/y 50" || nudges != 1 || fake.views != 0 {
		t.Errorf("calls = %s, nudged %d, viewed %d, want no view", got, nudges, fake.views)
	}
	if want := "Cancelled: test, lint, e2e\nSkipped, no GitHub Actions run behind them: vercel\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}

	fake = &fakeChecksGH{checks: checks}
	out, _, err = runCI(t, fake, "slice-checks-cancel", "--check", "lint", "--json")
	if err != nil {
		t.Fatalf("slice-checks-cancel --check: %v", err)
	}
	if want := `{"cancelled":["test","lint"],"skipped":[]}`; compactJSON(t, out) != want || strings.Join(fake.calls, ",") != "cancel o/r 11" {
		t.Errorf("json = %s, calls %v", compactJSON(t, out), fake.calls)
	}

	fake = &fakeChecksGH{checks: ciChecks()}
	if _, _, err := runCI(t, fake, "slice-checks-cancel", "--check", "macos"); err == nil || !strings.Contains(err.Error(), "nothing to cancel") || len(fake.calls) != 0 {
		t.Errorf("cancel of a finished run = %v, calls %v", err, fake.calls)
	}

	fake = &fakeChecksGH{checks: checks, failOn: "cancel x/y"}
	if _, _, err := runCI(t, fake, "slice-checks-cancel"); err == nil || !strings.Contains(err.Error(), "(already cancelled test, lint)") {
		t.Errorf("refused cancel = %v", err)
	}
}

// TestSentSoFar is empty where nothing went out.
func TestSentSoFar(t *testing.T) {
	if got := sentSoFar(checksActionDoc{}); got != "" {
		t.Errorf("sentSoFar = %q", got)
	}
}

// TestSliceChecksRerunRefusesAnUnopenablePlan reports a plan that cannot be
// opened before gh is asked anything.
func TestSliceChecksRerunRefusesAnUnopenablePlan(t *testing.T) {
	fake := &fakeChecksGH{checks: ciChecks()}
	env, _ := checksEnv(t, checksPR, fake)
	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"slice-checks-cancel", testSliceID, "--project", "project-1"}, env); err == nil || len(fake.calls) != 0 {
		t.Errorf("slice-checks-cancel over an unopenable plan = %v after %v, want an error before gh", err, fake.calls)
	}
}
