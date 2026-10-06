package gh

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// budgetAt is a budget in a temporary directory whose clock reads *now.
func budgetAt(t *testing.T, now *time.Time) *Budget {
	t.Helper()
	return NewBudget(filepath.Join(t.TempDir(), "state", BudgetFileName), func() time.Time { return *now })
}

var budgetNoon = time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)

// TestOutlookOf is the policy over a table of readings: healthy, heading
// under the reserve, under it, the reset passed, and a stop.
func TestOutlookOf(t *testing.T) {
	now := budgetNoon
	poll := 30 * time.Second
	reading := func(remaining int, resetIn, readAgo time.Duration) *BudgetReading {
		return &BudgetReading{Limit: 5000, Remaining: remaining, ResetAt: now.Add(resetIn), ReadAt: now.Add(-readAgo)}
	}
	tests := []struct {
		name      string
		st        budgetState
		projected int
		throttled bool
		paused    bool
		pollAfter time.Duration
	}{
		{name: "no reading", st: budgetState{}, pollAfter: poll},
		{name: "one reading, healthy", st: budgetState{Last: reading(4000, 30*time.Minute, 0)},
			projected: 4000, pollAfter: poll},
		{name: "spending slowly", // 10 points a minute, 30 minutes left: 4000 − 300
			st:        budgetState{Prev: reading(4010, 30*time.Minute, time.Minute), Last: reading(4000, 30*time.Minute, 0)},
			projected: 3700, pollAfter: poll},
		{name: "heading under the reserve, spare enough", // 100 a minute: 2000 − 3000 → 0; 1800s over 1000 spare is under poll
			st:        budgetState{Prev: reading(2100, 30*time.Minute, time.Minute), Last: reading(2000, 30*time.Minute, 0)},
			projected: 0, pollAfter: poll},
		{name: "heading under, stretch past the cap", // 1800s over 4 spare points
			st:        budgetState{Prev: reading(1104, 30*time.Minute, time.Minute), Last: reading(1004, 30*time.Minute, 0)},
			projected: 0, throttled: true, pollAfter: budgetCap},
		{name: "heading under, stretch within the cap", // 1800s over 10 spare: 180s
			st:        budgetState{Prev: reading(1110, 30*time.Minute, time.Minute), Last: reading(1010, 30*time.Minute, 0)},
			projected: 0, throttled: true, pollAfter: 180 * time.Second},
		{name: "nothing spent between readings", st: budgetState{Prev: reading(900, 30*time.Minute, time.Minute),
			Last: reading(900, 30*time.Minute, 0)}, projected: 900, throttled: true, pollAfter: budgetCap},
		{name: "under the reserve", st: budgetState{Last: reading(900, 30*time.Minute, 0)},
			projected: 900, throttled: true, pollAfter: budgetCap},
		{name: "reset passed", st: budgetState{Last: reading(10, -time.Minute, 0)},
			projected: 5000, pollAfter: poll},
		{name: "readings of two hours", // the hour reset between them: no rate
			st: budgetState{Prev: &BudgetReading{Limit: 5000, Remaining: 100, ResetAt: now.Add(-time.Minute), ReadAt: now.Add(-2 * time.Minute)},
				Last: reading(4990, 59*time.Minute, 0)},
			projected: 4990, pollAfter: poll},
		{name: "stopped", st: budgetState{Last: reading(0, 10*time.Minute, 0), Stop: &budgetStop{Until: now.Add(10*time.Minute + 500*time.Millisecond)}},
			projected: 0, paused: true, pollAfter: 10*time.Minute + time.Second},
		{name: "stop passed", st: budgetState{Stop: &budgetStop{Until: now.Add(-time.Second)}}, pollAfter: poll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := outlookOf(tt.st, now, poll)
			if o.Projected != tt.projected || o.Throttled != tt.throttled || o.PausedUntil.IsZero() == tt.paused ||
				o.PollAfter != tt.pollAfter {
				t.Errorf("outlookOf() = %+v, want projected %d, throttled %v, paused %v, poll after %v",
					o, tt.projected, tt.throttled, tt.paused, tt.pollAfter)
			}
			if o.Reading != tt.st.Last {
				t.Errorf("outlookOf().Reading = %+v, want the last reading", o.Reading)
			}
		})
	}
}

// TestOutlookNeverFasterThanPoll: a poll configured longer than the cap is
// never shortened by the throttle.
func TestOutlookNeverFasterThanPoll(t *testing.T) {
	st := budgetState{Last: &BudgetReading{Limit: 5000, Remaining: 10, ResetAt: budgetNoon.Add(time.Hour)}}
	if o := outlookOf(st, budgetNoon, time.Hour); o.PollAfter != time.Hour || o.Throttled {
		t.Errorf("outlookOf() = %+v, want an hour, not throttled", o)
	}
}

// TestBudgetRecordsReadings keeps the last two readings, the newest last, on
// disk where another process reads them.
func TestBudgetRecordsReadings(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	reset := now.Add(30 * time.Minute)
	b.record(&RateLimit{Limit: 5000, Remaining: 2100, ResetAt: reset})
	now = now.Add(time.Minute)
	b.record(&RateLimit{Limit: 5000, Remaining: 2000, ResetAt: reset})
	b.record(nil)
	other := NewBudget(b.path, func() time.Time { return now })
	o := other.Outlook(30 * time.Second)
	if o.Reading == nil || o.Reading.Remaining != 2000 || !o.Reading.ReadAt.Equal(now) {
		t.Fatalf("Outlook().Reading = %+v, want 2000 read now", o.Reading)
	}
	// 100 a minute for 29 minutes takes 2000 to nothing.
	if o.Projected != 0 {
		t.Errorf("Outlook().Projected = %d, want 0", o.Projected)
	}
}

// TestNilBudget tracks nothing and stretches nothing.
func TestNilBudget(t *testing.T) {
	var b *Budget
	b.record(&RateLimit{Limit: 1})
	b.clear()
	if b.pausedPoll() {
		t.Error("a nil budget paused a poll")
	}
	if o := b.Outlook(time.Minute); o.PollAfter != time.Minute || o.Reading != nil {
		t.Errorf("Outlook() = %+v, want the poll alone", o)
	}
}

// TestBudgetUnreadableFile reads a file that will not parse as no budget,
// and one that cannot be read at all likewise.
func TestBudgetUnreadableFile(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if o := b.Outlook(time.Minute); o.Reading != nil {
		t.Errorf("Outlook() over junk = %+v, want no reading", o)
	}
	dir := NewBudget(filepath.Dir(b.path), func() time.Time { return now })
	if o := dir.Outlook(time.Minute); o.Reading != nil {
		t.Errorf("Outlook() over a directory = %+v, want no reading", o)
	}
	// Saving where a directory stands fails, logged, and changes nothing.
	dir.record(&RateLimit{Limit: 1})
}

// TestRefusalLines records a stop on each of gh's refusal lines, until the
// last reading's reset, and fails with the retry time.
func TestRefusalLines(t *testing.T) {
	for _, line := range []string{
		"gh: API rate limit already exceeded for user ID 1234.",
		"GraphQL: API rate limit exceeded for user ID 1234.",
		"gh: You have exceeded a secondary rate limit. Please wait a few minutes before you try again.",
	} {
		t.Run(line, func(t *testing.T) {
			now := budgetNoon
			b := budgetAt(t, &now)
			reset := now.Add(46 * time.Minute)
			b.record(&RateLimit{Limit: 5000, Remaining: 0, ResetAt: reset})
			runner := &stderrRunner{err: &ExitError{Code: 1, Stderr: line + "\n"}}
			err := NewWithRunner(runner).WithBudget(b).MergePR("/repo", "https://github.com/craig/nat/pull/1")
			var limited *LimitError
			if !errors.As(err, &limited) || !limited.Until.Equal(reset) {
				t.Fatalf("MergePR() = %v, want a LimitError until %v", err, reset)
			}
			if want := "GitHub's API limit is spent until 12:46; try again then"; err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
			if o := b.Outlook(time.Second); !o.PausedUntil.Equal(reset) {
				t.Errorf("PausedUntil = %v, want %v", o.PausedUntil, reset)
			}
		})
	}
}

// TestRefusalWithNoReading stops for five minutes where no reading said when
// the hour resets — and likewise where the last reading's reset has passed.
func TestRefusalWithNoReading(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	if err := b.observe(errors.New("API rate limit already exceeded")); err == nil {
		t.Fatal("observe() = nil, want the refusal")
	}
	if o := b.Outlook(time.Second); !o.PausedUntil.Equal(now.Add(stopFallback)) {
		t.Errorf("PausedUntil = %v, want five minutes on", o.PausedUntil)
	}
	b.record(&RateLimit{Limit: 5000, ResetAt: now.Add(-time.Minute)})
	now = now.Add(time.Minute)
	_ = b.observe(errors.New("API rate limit already exceeded"))
	if o := b.Outlook(time.Second); !o.PausedUntil.Equal(now.Add(stopFallback)) {
		t.Errorf("PausedUntil = %v, want five minutes from the second refusal", o.PausedUntil)
	}
}

// TestOtherFailuresRecordNoStop: a gh failing for any other reason is its
// own error, and stops nothing.
func TestOtherFailuresRecordNoStop(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	want := &ExitError{Code: 1, Stderr: "no pull requests found"}
	if err := b.observe(want); err != want {
		t.Errorf("observe() = %v, want gh's own error", err)
	}
	if b.pausedPoll() {
		t.Error("an ordinary failure paused polling")
	}
}

// TestPollSkippedWhileStopped: a polling read before the retry time runs no
// gh and reads nothing, logged once per stop; an action's read still runs,
// and its success clears the stop, so the next poll runs.
func TestPollSkippedWhileStopped(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	runner := &scriptRunner{
		outs: []string{"", fixture(t, "graphql-open.json")},
		errs: []error{&ExitError{Code: 1, Stderr: "gh: API rate limit already exceeded for user ID 1"}},
	}
	c := NewWithRunner(runner).WithBudget(b)
	if _, err := c.PollPRs(BatchQuery{PRs: []PRRef{natPR}}); err == nil {
		t.Fatal("the refused poll read no refusal")
	}
	for range 3 {
		batch, err := c.PollPRs(BatchQuery{PRs: []PRRef{natPR}})
		if err != nil || len(batch.PRs) != 0 {
			t.Fatalf("PollPRs() while stopped = %+v, %v; want nothing read and no error", batch, err)
		}
	}
	if len(runner.docs) != 1 {
		t.Fatalf("gh ran %d times, want once: no poll while stopped", len(runner.docs))
	}
	if st := b.load(); st.Stop == nil || !st.Stop.Logged {
		t.Errorf("stop = %+v, want logged once", st.Stop)
	}
	batch, err := c.ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
	if err != nil || len(batch.PRs) != 1 {
		t.Fatalf("an action's ReadPRs() = %+v, %v; want it run and read", batch, err)
	}
	if st := b.load(); st.Stop != nil {
		t.Errorf("stop = %+v after a success, want it cleared", st.Stop)
	}
	if _, err := c.PollPRs(BatchQuery{PRs: []PRRef{natPR}}); err != nil || len(runner.docs) != 3 {
		t.Errorf("PollPRs() after the clear ran gh %d times (%v), want a third run", len(runner.docs), err)
	}
	now = now.Add(time.Hour)
	if b.pausedPoll() {
		t.Error("a poll past the retry time was paused")
	}
}

// TestRefusalRefreshKeepsLogged: a refusal for the stop already recorded is
// not a new stop to log again; one with a new retry time is.
func TestRefusalRefreshKeepsLogged(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	b.record(&RateLimit{Limit: 5000, ResetAt: now.Add(time.Hour)})
	_ = b.refuse()
	b.pausedPoll()
	_ = b.refuse()
	if st := b.load(); !st.Stop.Logged {
		t.Errorf("stop = %+v, want the refresh to keep logged", st.Stop)
	}
	b.record(&RateLimit{Limit: 5000, ResetAt: now.Add(2 * time.Hour)})
	_ = b.refuse()
	if st := b.load(); st.Stop.Logged {
		t.Errorf("stop = %+v, want a new stop unlogged", st.Stop)
	}
}

// TestDocumentRefusedOnTheLimit: GraphQL's own refusal in the answer — its
// RATE_LIMITED type, or its words — records the stop where gh's stderr did
// not say so; without a budget it is GitHub's message.
func TestDocumentRefusedOnTheLimit(t *testing.T) {
	answer := `{"errors":[{"type":"RATE_LIMITED","message":"API rate limit already exceeded for user ID 1."}]}`
	now := budgetNoon
	b := budgetAt(t, &now)
	runner := &scriptRunner{outs: []string{answer}, errs: []error{errors.New("exit status 1")}}
	_, err := NewWithRunner(runner).WithBudget(b).ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
	var limited *LimitError
	if !errors.As(err, &limited) {
		t.Fatalf("ReadPRs() = %v, want a LimitError", err)
	}
	// gh's stderr said so as well: the runner's refusal is the one returned.
	said := &scriptRunner{outs: []string{answer}, errs: []error{&ExitError{Code: 1, Stderr: "gh: API rate limit already exceeded"}}}
	_, err = NewWithRunner(said).WithBudget(budgetAt(t, &now)).ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
	if !errors.As(err, &limited) {
		t.Errorf("ReadPRs() refused on stderr and in the answer = %v, want a LimitError", err)
	}
	other := `{"errors":[{"type":"RATE_LIMITED","message":"slow down"}]}`
	_, err = NewWithRunner(&scriptRunner{outs: []string{other}}).WithBudget(budgetAt(t, &now)).ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
	if !errors.As(err, &limited) {
		t.Errorf("ReadPRs() on RATE_LIMITED = %v, want a LimitError", err)
	}
	_, err = NewWithRunner(&scriptRunner{outs: []string{answer}}).ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
	if err == nil || !strings.HasPrefix(err.Error(), "GraphQL: API rate limit") {
		t.Errorf("ReadPRs() with no budget = %v, want GitHub's message", err)
	}
}

// TestReadingCostAndRecord: a reading's cost is the sum over its documents,
// and its rate limit is recorded.
func TestReadingCostAndRecord(t *testing.T) {
	answer := `{"data":{"rateLimit":{"limit":5000,"remaining":4200,"resetAt":"2026-10-06T13:00:00Z","cost":1}}}`
	now := budgetNoon
	b := budgetAt(t, &now)
	var prs []PRRef
	for n := 1; n <= batchChunk+1; n++ {
		prs = append(prs, PRRef{Owner: "craig", Repo: "nat", Number: n})
	}
	c := NewWithRunner(&scriptRunner{outs: []string{answer}}).WithBudget(b)
	batch, err := c.ReadPRs(BatchQuery{PRs: prs})
	if err != nil || batch.Cost != 2 {
		t.Fatalf("ReadPRs() cost %d (%v), want 2 over two documents", batch.Cost, err)
	}
	if o := c.Outlook(time.Second); o.Reading == nil || o.Reading.Remaining != 4200 {
		t.Errorf("Outlook().Reading = %+v, want 4200 recorded", o.Reading)
	}
}

// TestLimitErrorAnotherDay names the date where the retry is not today.
func TestLimitErrorAnotherDay(t *testing.T) {
	err := &LimitError{Until: budgetNoon.Add(24 * time.Hour), now: budgetNoon}
	if want := "GitHub's API limit is spent until 7 Oct 12:00; try again then"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err, want)
	}
}

// TestWithNilBudget keeps the runner as it was.
func TestWithNilBudget(t *testing.T) {
	r := &scriptRunner{}
	if c := NewWithRunner(r).WithBudget(nil); c.runner != Runner(r) || c.budget != nil {
		t.Errorf("WithBudget(nil) = %+v, want c unchanged", c)
	}
}

// TestDefaultBudget is kept in the state directory, and none where it cannot
// be resolved.
func TestDefaultBudget(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if b := DefaultBudget(); b != nil {
		t.Errorf("DefaultBudget() with no home = %+v, want none", b)
	}
}

// TestCeilSecond rounds up, and leaves whole seconds be.
func TestCeilSecond(t *testing.T) {
	if got := ceilSecond(1500 * time.Millisecond); got != 2*time.Second {
		t.Errorf("ceilSecond(1.5s) = %v", got)
	}
	if got := ceilSecond(3 * time.Second); got != 3*time.Second {
		t.Errorf("ceilSecond(3s) = %v", got)
	}
}

// stderrRunner fails every run with err, and carries stdin for a comment.
type stderrRunner struct {
	err   error
	stdin bool
}

func (r *stderrRunner) Run(string, string, ...string) (string, error) { return "", r.err }

func (r *stderrRunner) RunWithStdin(string, io.Reader, string, ...string) (string, error) {
	r.stdin = true
	return "", r.err
}

// TestBudgetRunnerStdin carries a comment through to a runner that takes
// stdin, keeping the budget, and refuses one that cannot.
func TestBudgetRunnerStdin(t *testing.T) {
	now := budgetNoon
	b := budgetAt(t, &now)
	inner := &stderrRunner{err: &ExitError{Code: 1, Stderr: "exceeded a secondary rate limit"}}
	_, err := NewWithRunner(inner).WithBudget(b).CommentPR("/repo", "https://github.com/craig/nat/pull/1", "hi")
	var limited *LimitError
	if !inner.stdin || !errors.As(err, &limited) {
		t.Errorf("CommentPR() = %v (stdin %v), want the refusal through stdin", err, inner.stdin)
	}
	_, err = NewWithRunner(&scriptRunner{}).WithBudget(b).CommentPR("/repo", "https://github.com/craig/nat/pull/1", "hi")
	if err == nil || !strings.Contains(err.Error(), "standard input") {
		t.Errorf("CommentPR() through a runner with no stdin = %v", err)
	}
}
