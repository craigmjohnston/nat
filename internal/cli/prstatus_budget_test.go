package cli

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// queuedGH answers each gh call with the next of its answers, the last
// repeating, and counts the calls — whatever they are: a reading, or an
// action's comment on stdin.
type queuedGH struct {
	answers []queuedAnswer
	calls   int
}

type queuedAnswer struct {
	out string
	err error
}

func (q *queuedGH) next() (string, error) {
	a := q.answers[min(q.calls, len(q.answers)-1)]
	q.calls++
	return a.out, a.err
}

func (q *queuedGH) Run(string, string, ...string) (string, error) { return q.next() }

func (q *queuedGH) RunWithStdin(string, io.Reader, string, ...string) (string, error) { return q.next() }

// budgetPRStatus runs pr-status --json, with extra arguments, and decodes it.
func budgetPRStatus(t *testing.T, env Env, out *strings.Builder, args ...string) prStatusDoc {
	t.Helper()
	out.Reset()
	if err := Run(context.Background(), append([]string{"pr-status", "--project", "project-1", "--json"}, args...), env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	var doc prStatusDoc
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("pr-status printed %q: %v", out, err)
	}
	return doc
}

// TestBudgetStopsPollingAndNotActions is the acceptance, end to end through
// the real gh.CLI and its budget: a reading recorded, then refused on the
// limit — the stop written until the reading's reset — after which a polling
// pr-status runs no gh and reports the pause, an action still runs gh and
// fails naming the retry time, a settle read still runs, and an action that
// goes through clears the stop so polling reads again.
func TestBudgetStopsPollingAndNotActions(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	var out strings.Builder
	env.Out = &out
	now := time.Date(2026, 10, 6, 13, 0, 0, 0, time.Local)
	reset := now.Add(46 * time.Minute)
	budget := gh.NewBudget(filepath.Join(t.TempDir(), gh.BudgetFileName), func() time.Time { return now })
	healthy := `{"data":{"rateLimit":{"limit":5000,"remaining":4000,"resetAt":"` + reset.UTC().Format(time.RFC3339) + `","cost":1}}}`
	refusal := queuedAnswer{
		out: `{"errors":[{"type":"RATE_LIMITED","message":"API rate limit already exceeded for user ID 1."}]}`,
		err: &gh.ExitError{Code: 1, Stderr: "gh: API rate limit already exceeded for user ID 1.\n"},
	}
	runner := &queuedGH{answers: []queuedAnswer{{out: healthy}, refusal}}
	env.NewGH = func() GH { return gh.NewWithRunner(runner).WithBudget(budget) }

	doc := budgetPRStatus(t, env, &out)
	if rl := doc.RateLimit; rl == nil || rl.Remaining != 4000 || rl.Cost != 1 || rl.PausedUntil != nil ||
		rl.PollAfterSeconds != 30 || rl.Projected != 4000 {
		t.Fatalf("healthy rate_limit = %+v", doc.RateLimit)
	}

	doc = budgetPRStatus(t, env, &out)
	if rl := doc.RateLimit; rl == nil || rl.PausedUntil == nil || !rl.PausedUntil.Equal(reset) ||
		rl.Remaining != 4000 || rl.Cost != 0 {
		t.Fatalf("refused rate_limit = %+v, want paused until %v, the last reading's figures", doc.RateLimit, reset)
	}
	if runner.calls != 2 {
		t.Fatalf("gh ran %d times, want 2", runner.calls)
	}

	doc = budgetPRStatus(t, env, &out)
	if runner.calls != 2 {
		t.Errorf("a poll while stopped ran gh (%d calls)", runner.calls)
	}
	if rl := doc.RateLimit; rl == nil || rl.PausedUntil == nil || rl.PollAfterSeconds != int((46*time.Minute)/time.Second) {
		t.Errorf("paused rate_limit = %+v, want a 46-minute wait", doc.RateLimit)
	}

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--body", "hi", "--project", "project-1"}, env)
	want := "GitHub's API limit is spent until " + reset.Format("15:04") + "; try again then"
	if runner.calls != 3 || err == nil || err.Error() != want {
		t.Errorf("pr-comment while stopped = %v after %d calls, want gh run and %q", err, runner.calls, want)
	}

	budgetPRStatus(t, env, &out, "--settle")
	if runner.calls != 4 {
		t.Errorf("a settle read while stopped ran gh %d times in all, want 4", runner.calls)
	}

	runner.answers = append(runner.answers, queuedAnswer{out: "https://github.test/craig/nat/pull/7#issuecomment-1\n"},
		queuedAnswer{out: healthy})
	if err := Run(context.Background(), []string{"pr-comment", testSliceID, "--body", "hi", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-comment: %v", err)
	}
	doc = budgetPRStatus(t, env, &out)
	if runner.calls != 6 || doc.RateLimit == nil || doc.RateLimit.PausedUntil != nil {
		t.Errorf("after the action went through: %d calls, rate_limit %+v; want a poll run and no pause",
			runner.calls, doc.RateLimit)
	}
}

// TestPRStatusPollsUnlessSettling: pr-status reads through the polling read,
// and --settle through an action's.
func TestPRStatusPollsUnlessSettling(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	var out strings.Builder
	env.Out = &out
	reader := &fakePRReader{}
	env.NewGH = func() GH { return reader }

	budgetPRStatus(t, env, &out)
	if reader.polls != 1 || reader.calls != 1 {
		t.Errorf("pr-status: %d polls of %d reads, want its one read a poll", reader.polls, reader.calls)
	}
	budgetPRStatus(t, env, &out, "--settle")
	if reader.polls != 1 || reader.calls != 2 {
		t.Errorf("pr-status --settle: %d polls of %d reads, want the second read no poll", reader.polls, reader.calls)
	}
}

// TestPRStatusThrottled reports the budget's stretched interval, in JSON and
// in words.
func TestPRStatusThrottled(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	var out strings.Builder
	env.Out = &out
	reset := time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)
	reading := &gh.BudgetReading{Limit: 5000, Remaining: 1004, ResetAt: reset}
	reader := &fakePRReader{
		rate:    &gh.RateLimit{Limit: 5000, Remaining: 1004, ResetAt: reset},
		cost:    1,
		outlook: &gh.Outlook{Reading: reading, Projected: 300, Throttled: true, PollAfter: 120 * time.Second},
	}
	env.NewGH = func() GH { return reader }

	doc := budgetPRStatus(t, env, &out)
	want := rateLimitJSON{Limit: 5000, Remaining: 1004, ResetAt: reset, Projected: 300, Throttled: true,
		PollAfterSeconds: 120, Cost: 1}
	if doc.RateLimit == nil || *doc.RateLimit != want {
		t.Errorf("rate_limit = %+v, want %+v", doc.RateLimit, want)
	}

	out.Reset()
	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	line := "GitHub budget: 1004 of 5000 points left, resets at 2026-10-06T13:00:00Z; 300 projected at the reset; " +
		"this reading cost 1; throttled to keep the reserve; next reading in 120s\n"
	if !strings.Contains(out.String(), line) {
		t.Errorf("markdown lacks %q:\n%s", line, out.String())
	}
}

// TestRateLimitOf: no block where nothing was asked, or nothing was ever read
// and nothing stops polling; a stop with no reading is the pause alone.
func TestRateLimitOf(t *testing.T) {
	if rl := rateLimitOf(gh.Batch{}, nil); rl != nil {
		t.Errorf("nothing asked: %+v, want none", rl)
	}
	if rl := rateLimitOf(gh.Batch{}, &budgetReport{outlook: gh.Outlook{PollAfter: time.Minute}}); rl != nil {
		t.Errorf("nothing ever read: %+v, want none", rl)
	}
	until := time.Date(2026, 10, 6, 13, 46, 0, 0, time.UTC)
	rl := rateLimitOf(gh.Batch{}, &budgetReport{outlook: gh.Outlook{PausedUntil: until, PollAfter: time.Minute}})
	if rl == nil || rl.PausedUntil == nil || !rl.PausedUntil.Equal(until) || rl.Limit != 0 {
		t.Fatalf("paused with no reading: %+v", rl)
	}
	line := budgetLine(rl)
	if !strings.Contains(line, "GitHub refused on its limit: polling paused until 2026-10-06T13:46:00Z; next reading in 60s") {
		t.Errorf("budgetLine() = %q", line)
	}
	if budgetLine(nil) != "" {
		t.Error("budgetLine(nil) said something")
	}
}
