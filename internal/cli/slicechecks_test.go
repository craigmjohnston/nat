package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakeChecksGH answers ViewPR with a fixed set of checks and FailedLog from a
// table keyed by job (or run, where no job is named), stubbing the rest of
// [GH] through fakePRBase.
type fakeChecksGH struct {
	fakePRBase
	checks  []gh.Check
	viewErr error
	logs    map[string]string
	logErr  error
	logRuns []string
	// prState is the pull request's state, OPEN where left empty.
	prState string

	// jobs answers ActionsJob by job id, jobErr fails it; jobLogs answers
	// JobLog by job id, jobLogErr fails it.
	jobs      map[string]gh.Job
	jobErr    error
	jobLogs   map[string]string
	jobLogErr error

	// statuses answers RunStatus by run, one word per poll (the last
	// repeating), statusErr failing the first poll; failOn fails the run
	// call whose record starts with it; calls records every run call.
	statuses  map[string][]string
	statusErr error
	failOn    string
	calls     []string
	// views counts ViewPR, which only slice-checks itself may make.
	views int
}

func (f *fakeChecksGH) ViewPR(dir, ref string) (gh.PR, error) {
	f.views++
	return f.pr(ref)
}

// ReadPRs answers the batched reading with the same pull request ViewPR
// would — what slice-checks-rerun and -cancel read it by.
func (f *fakeChecksGH) ReadPRs(q gh.BatchQuery) (gh.Batch, error) {
	batch := gh.Batch{PRs: map[gh.PRRef]gh.PR{}}
	for _, ref := range q.PRs {
		pr, err := f.pr(checksPR)
		if err != nil {
			return gh.Batch{}, err
		}
		batch.PRs[ref] = pr
	}
	return batch, nil
}

func (f *fakeChecksGH) pr(ref string) (gh.PR, error) {
	if f.viewErr != nil {
		return gh.PR{}, f.viewErr
	}
	state := f.prState
	if state == "" {
		state = "OPEN"
	}
	return gh.PR{URL: ref, State: state, Checks: f.checks}, nil
}

func (f *fakeChecksGH) ActionsJob(dir string, ref gh.ActionsRef) (gh.Job, error) {
	return f.jobs[ref.Job], f.jobErr
}

func (f *fakeChecksGH) JobLog(dir string, ref gh.ActionsRef) (string, error) {
	return f.jobLogs[ref.Job], f.jobLogErr
}

func (f *fakeChecksGH) record(call string) error {
	f.calls = append(f.calls, call)
	if f.failOn != "" && strings.HasPrefix(call, f.failOn) {
		return errors.New("gh refused " + call)
	}
	return nil
}

func (f *fakeChecksGH) RunStatus(dir string, ref gh.ActionsRef) (string, error) {
	f.calls = append(f.calls, "status "+ref.Run)
	if f.statusErr != nil {
		err := f.statusErr
		f.statusErr = nil
		return "", err
	}
	words := f.statuses[ref.Run]
	if len(words) == 0 {
		return gh.RunCompleted, nil
	}
	word := words[0]
	if len(words) > 1 {
		f.statuses[ref.Run] = words[1:]
	}
	return word, nil
}

func (f *fakeChecksGH) CancelRun(dir string, ref gh.ActionsRef) error {
	return f.record("cancel " + ref.Owner + "/" + ref.Repo + " " + ref.Run)
}

func (f *fakeChecksGH) RerunRun(dir string, ref gh.ActionsRef, failedOnly bool) error {
	call := "rerun " + ref.Run
	if failedOnly {
		call += " --failed"
	}
	return f.record(call)
}

func (f *fakeChecksGH) RerunJob(dir string, ref gh.ActionsRef) error {
	return f.record("rerun --job " + ref.Job)
}

func (f *fakeChecksGH) FailedLog(dir string, ref gh.ActionsRef) (string, error) {
	f.logRuns = append(f.logRuns, ref.Run+"/"+ref.Job)
	if f.logErr != nil {
		return "", f.logErr
	}
	return f.logs[ref.Job], nil
}

const checksPR = "https://github.test/craig/nat/pull/7"

func checksEnv(t *testing.T, pr string, fake *fakeChecksGH) (Env, *strings.Builder) {
	t.Helper()
	api := &fakeAPI{pages: map[string][]notion.Page{
		"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, pr)},
	}}
	env, _ := testEnv(testConfig(t), api)
	var out strings.Builder
	env.Out = &out
	env.NewGH = func() GH { return fake }
	return env, &out
}

// TestSliceChecksVerdicts reads each of the three verdicts off the pull
// request's own checks, and lists every check by name, state and run URL.
func TestSliceChecksVerdicts(t *testing.T) {
	tests := []struct {
		name   string
		checks []gh.Check
		want   string
	}{
		{"passing", []gh.Check{{Name: "test", State: "SUCCESS", URL: "https://ci.test/1"}}, "passing"},
		{"pending", []gh.Check{{Name: "test", State: "IN_PROGRESS", URL: "https://ci.test/1"}}, "pending"},
		{"failing", []gh.Check{
			{Name: "lint", State: "SUCCESS", URL: "https://ci.test/2"},
			{Name: "test", State: "FAILURE", URL: "https://ci.test/1"},
		}, "failing"},
	}
	for _, tt := range tests {
		env, out := checksEnv(t, checksPR, &fakeChecksGH{checks: tt.checks})
		if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err != nil {
			t.Fatalf("%s: slice-checks: %v", tt.name, err)
		}
		if !strings.HasPrefix(out.String(), "Checks: "+tt.want+" — "+checksPR+"\n") {
			t.Errorf("%s: output = %q, want the %s verdict first", tt.name, out.String(), tt.want)
		}
		for _, c := range tt.checks {
			if line := fmt.Sprintf("- %s — %s — %s\n", c.Name, c.State, c.URL); !strings.Contains(out.String(), line) {
				t.Errorf("%s: output missing %q:\n%s", tt.name, line, out.String())
			}
		}
	}
}

// TestSliceChecksJSON is the structured form: the PR, the verdict, every
// check — and no log key where --log was not asked for.
func TestSliceChecksJSON(t *testing.T) {
	env, out := checksEnv(t, checksPR, &fakeChecksGH{checks: []gh.Check{
		{Name: "test", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/1/job/2"},
	}})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --json: %v", err)
	}
	want := `{"checks":[{"name":"test","state":"FAILURE","url":"https://github.com/o/r/actions/runs/1/job/2"}],"pr":"` + checksPR + `","verdict":"failing"}`
	if got := compactJSON(t, out.String()); got != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}

// TestSliceChecksLog appends the failed log of an Actions check — its job's,
// cut to the last 200 lines with a line saying how many went — and nothing but
// the URL for a status another service reported, or for a check that passed.
func TestSliceChecksLog(t *testing.T) {
	var long []string
	for i := 1; i <= 250; i++ {
		long = append(long, fmt.Sprintf("line %d", i))
	}
	fake := &fakeChecksGH{
		checks: []gh.Check{
			{Name: "test", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/11/job/22"},
			{Name: "deploy", State: "ERROR", URL: "https://ci.example.com/build/9"},
			{Name: "lint", State: "SUCCESS", URL: "https://github.com/o/r/actions/runs/11/job/33"},
		},
		logs: map[string]string{"22": strings.Join(long, "\n")},
	}
	env, out := checksEnv(t, checksPR, fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log: %v", err)
	}
	if want := []string{"11/22"}; strings.Join(fake.logRuns, ",") != strings.Join(want, ",") {
		t.Errorf("read logs %v, want only the failed Actions job's %v", fake.logRuns, want)
	}
	got := out.String()
	for _, want := range []string{
		"## test failed log", "… 50 earlier lines cut\nline 51\n", "line 250\n```",
		"- deploy — ERROR — https://ci.example.com/build/9",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"line 50\n", "## deploy", "## lint"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output carries %q:\n%s", unwanted, got)
		}
	}
}

// TestSliceChecksLogShortAndUnreadable prints a log under the limit whole,
// and says under its check, in both forms, why a log could not be read — the
// verdict still answers.
func TestSliceChecksLogShortAndUnreadable(t *testing.T) {
	failed := []gh.Check{{Name: "test", State: "FAILURE", URL: "https://github.com/o/r/actions/runs/11"}}
	env, out := checksEnv(t, checksPR, &fakeChecksGH{checks: failed, logs: map[string]string{"": "boom\nat step 3"}})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log: %v", err)
	}
	if !strings.Contains(out.String(), "```\nboom\nat step 3\n```") || strings.Contains(out.String(), "cut") {
		t.Errorf("output = %q, want the short log whole", out.String())
	}

	env, out = checksEnv(t, checksPR, &fakeChecksGH{checks: failed, logErr: errors.New("gone\nand more")})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log over an unreadable log: %v", err)
	}
	want := "- test — FAILURE — https://github.com/o/r/actions/runs/11\n  log not available: gone\n"
	if !strings.HasPrefix(out.String(), "Checks: failing") || !strings.Contains(out.String(), want) ||
		strings.Contains(out.String(), "failed log") || strings.Contains(out.String(), "and more") {
		t.Errorf("output = %q, want the verdict and %q, and no log", out.String(), want)
	}

	env, out = checksEnv(t, checksPR, &fakeChecksGH{checks: failed, logErr: errors.New("gone\nand more")})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log --json over an unreadable log: %v", err)
	}
	wantJSON := `{"checks":[{"log_error":"gone","name":"test","state":"FAILURE","url":"https://github.com/o/r/actions/runs/11"}],"pr":"` + checksPR + `","verdict":"failing"}`
	if got := compactJSON(t, out.String()); got != wantJSON {
		t.Errorf("json = %s, want %s", got, wantJSON)
	}
}

// TestSliceChecksNoPullRequest says when checks run and exits 0, in either
// form, without asking gh anything.
func TestSliceChecksNoPullRequest(t *testing.T) {
	fake := &fakeChecksGH{viewErr: errors.New("gh must not be asked")}
	env, out := checksEnv(t, "", fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks: %v", err)
	}
	if !strings.Contains(out.String(), "checks run once the slice is approved") {
		t.Errorf("output = %q, want it to say when checks run", out.String())
	}
	env, out = checksEnv(t, "", fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --json: %v", err)
	}
	if want := `{"checks":[],"pr":"","verdict":"none"}`; compactJSON(t, out.String()) != want {
		t.Errorf("json = %s, want %s", out.String(), want)
	}
}

// TestSliceChecksRefusals: a gh read that fails is the command's error, as
// are a bad invocation, an unknown project and a slice that cannot be read.
func TestSliceChecksRefusals(t *testing.T) {
	env, _ := checksEnv(t, checksPR, &fakeChecksGH{viewErr: errors.New("rate limited")})
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err == nil ||
		!strings.Contains(err.Error(), "rate limited") {
		t.Errorf("err = %v, want the gh failure", err)
	}
	for _, args := range [][]string{
		{"slice-checks", "--project", "project-1"},
		{"slice-checks", testSliceID, "--bogus", "--project", "project-1"},
		{"slice-checks", "not a page", "--project", "project-1"},
		{"slice-checks", testSliceID},
		{"slice-checks", "00000000-0000-0000-0000-000000000099", "--project", "project-1"},
	} {
		env, _ := checksEnv(t, checksPR, &fakeChecksGH{})
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%v: want a refusal", args)
		}
	}
}

// TestSliceChecksRefusesAnUnopenablePlan reports a plan that cannot be opened
// before gh is asked anything.
func TestSliceChecksRefusesAnUnopenablePlan(t *testing.T) {
	env, _ := checksEnv(t, checksPR, &fakeChecksGH{})
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
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--project", "project-1"}, env); err == nil {
		t.Error("slice-checks over an unopenable plan: want an error")
	}
}

func compactJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, s)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// fixedNow pins checksNow for a test, so a running job's durations are exact.
func fixedNow(t *testing.T, at time.Time) {
	t.Helper()
	old := checksNow
	checksNow = func() time.Time { return at }
	t.Cleanup(func() { checksNow = old })
}

// TestSliceChecksLogRunningJob says under a running Actions check where its
// job stands — running, the step in progress and for how long, the job's
// runner and how long it has run — then each step with its status and time,
// and that GitHub gives no log until the job ends; under a queued one, how
// long it has waited for a runner; and nothing more under a pending check
// whose URL names no job, or that another service reported.
func TestSliceChecksLogRunningJob(t *testing.T) {
	base := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return base.Add(d) }
	fixedNow(t, at(10*time.Minute))
	fake := &fakeChecksGH{
		checks: []gh.Check{
			{Name: "test", State: "IN_PROGRESS", URL: "https://github.com/o/r/actions/runs/11/job/21"},
			{Name: "lint", State: "QUEUED", URL: "https://github.com/o/r/actions/runs/11/job/22"},
			{Name: "macos", State: "IN_PROGRESS", URL: "https://github.com/o/r/actions/runs/12"},
			{Name: "vercel", State: "PENDING", URL: "https://vercel.example.com/1"},
		},
		jobs: map[string]gh.Job{
			"21": {Status: gh.JobInProgress, CreatedAt: at(0), StartedAt: at(time.Minute), Runner: "GitHub Actions 7", Steps: []gh.JobStep{
				{Name: "Set up job", Status: "completed", Conclusion: "success", StartedAt: at(time.Minute), CompletedAt: at(time.Minute + 2*time.Second)},
				{Name: "Test", Status: "in_progress", StartedAt: at(2 * time.Minute)},
				{Name: "Post", Status: "pending"},
			}},
			"22": {Status: gh.JobQueued, CreatedAt: at(7 * time.Minute)},
		},
		jobLogErr: gh.ErrLogNotReady,
	}
	env, out := checksEnv(t, checksPR, fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log: %v", err)
	}
	want := `Checks: pending — ` + checksPR + `

- test — IN_PROGRESS — https://github.com/o/r/actions/runs/11/job/21
  in progress for 9m0s on GitHub Actions 7, at step "Test" for 8m0s
  steps:
  - Set up job — success — 2s
  - Test — in progress — 8m0s so far
  - Post — pending
  log: GitHub gives no log until the job ends
- lint — QUEUED — https://github.com/o/r/actions/runs/11/job/22
  queued, no runner yet — waiting 3m0s
  log: GitHub gives no log until the job ends
- macos — IN_PROGRESS — https://github.com/o/r/actions/runs/12
- vercel — PENDING — https://vercel.example.com/1
`
	if out.String() != want {
		t.Errorf("output =\n%s\nwant\n%s", out.String(), want)
	}

	env, out = checksEnv(t, checksPR, fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log --json: %v", err)
	}
	wantJSON := `{"checks":[` +
		`{"job":{"created_at":"2026-10-05T11:00:00Z","runner":"GitHub Actions 7","started_at":"2026-10-05T11:01:00Z","status":"in_progress","steps":[` +
		`{"completed_at":"2026-10-05T11:01:02Z","conclusion":"success","name":"Set up job","started_at":"2026-10-05T11:01:00Z","status":"completed"},` +
		`{"name":"Test","started_at":"2026-10-05T11:02:00Z","status":"in_progress"},` +
		`{"name":"Post","status":"pending"}]},"log_pending":true,"name":"test","state":"IN_PROGRESS","url":"https://github.com/o/r/actions/runs/11/job/21"},` +
		`{"job":{"created_at":"2026-10-05T11:07:00Z","status":"queued","steps":[]},"log_pending":true,"name":"lint","state":"QUEUED","url":"https://github.com/o/r/actions/runs/11/job/22"},` +
		`{"name":"macos","state":"IN_PROGRESS","url":"https://github.com/o/r/actions/runs/12"},` +
		`{"name":"vercel","state":"PENDING","url":"https://vercel.example.com/1"}],` +
		`"pr":"` + checksPR + `","verdict":"pending"}`
	if got := compactJSON(t, out.String()); got != wantJSON {
		t.Errorf("json =\n%s\nwant\n%s", got, wantJSON)
	}
}

// TestSliceChecksLogRunningJobReads prints a log GitHub gave, cut as a failed
// one is; says under the check why a job or log could not be read, the
// verdict and every other check still printed; and reads a job with no start
// time as waiting with no time to give.
func TestSliceChecksLogRunningJobReads(t *testing.T) {
	fixedNow(t, time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC))
	running := []gh.Check{
		{Name: "test", State: "IN_PROGRESS", URL: "https://github.com/o/r/actions/runs/11/job/21"},
		{Name: "lint", State: "SUCCESS", URL: "https://github.com/o/r/actions/runs/11/job/22"},
	}
	fake := &fakeChecksGH{checks: running, jobs: map[string]gh.Job{"21": {Status: gh.JobInProgress, StartedAt: time.Date(2026, 10, 5, 11, 0, 5, 0, time.UTC)}},
		jobLogs: map[string]string{"21": "step one\nstep two"}}
	env, out := checksEnv(t, checksPR, fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log: %v", err)
	}
	for _, want := range []string{"  in progress for 0s\n- lint", "## test log\n\n```\nstep one\nstep two\n```"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}

	fake = &fakeChecksGH{checks: running, jobErr: errors.New("HTTP 502\nmore"), jobLogErr: errors.New("HTTP 500")}
	env, out = checksEnv(t, checksPR, fake)
	if err := Run(context.Background(), []string{"slice-checks", testSliceID, "--log", "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-checks --log over failed reads: %v", err)
	}
	want := "Checks: pending — " + checksPR + "\n\n- test — IN_PROGRESS — https://github.com/o/r/actions/runs/11/job/21\n" +
		"  job not available: HTTP 502\n  log not available: HTTP 500\n- lint — SUCCESS — https://github.com/o/r/actions/runs/11/job/22\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}

	if got := jobMarkdown(jobJSON{Status: gh.JobQueued}, time.Now()); got != "  queued, no runner yet\n" {
		t.Errorf("a queued job with no times = %q", got)
	}
	if got := elapsed(time.Now(), time.Now().Add(-time.Minute)); got != 0 {
		t.Errorf("elapsed backwards = %v, want 0", got)
	}
}
