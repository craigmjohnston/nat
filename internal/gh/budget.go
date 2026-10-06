package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// BudgetFileName is the file under nat's state directory — beside the last
// batched reading, github-reading.json — that keeps GitHub's budget across
// processes.
const BudgetFileName = "gh-budget.json"

const (
	// budgetReserveShare is the part of the hour's limit polling leaves alone,
	// one fifth (20 percent): what an action the user asks for spends.
	budgetReserveShare = 5
	// budgetCap is the longest a stretched interval grows to, and the interval
	// once the budget is already under the reserve.
	budgetCap = 5 * time.Minute
	// stopFallback is how long a stop holds where no reading said when the
	// hour resets.
	stopFallback = 5 * time.Minute
)

// Budget is GitHub's GraphQL budget as nat keeps it between processes: the
// last two readings a batched reading took ([BudgetReading]), and the stop a
// rate-limit refusal records. It is a file and a clock, nothing in memory, so
// the board, every pr-status gnat runs and every action an agent or the user
// runs share one budget — which is what it is on GitHub, too: the readings'
// burn rate is everyone's spend on the token, nat's or not.
//
// Every method is safe on a nil *Budget, which tracks nothing: polling is
// never paused or stretched, and a refusal is gh's own error.
type Budget struct {
	path string
	now  func() time.Time
}

// NewBudget keeps the budget at path, against now.
func NewBudget(path string, now func() time.Time) *Budget {
	return &Budget{path: path, now: now}
}

// DefaultBudget is the budget in nat's state directory, or nil where that
// directory cannot be resolved — logged, and no budget kept.
func DefaultBudget() *Budget {
	dir, err := logging.Dir()
	if err != nil {
		logging.Action("keeping no GitHub budget: the state directory is unresolved", "error", err)
		return nil
	}
	return NewBudget(filepath.Join(dir, BudgetFileName), time.Now)
}

// BudgetReading is one reading's rateLimit block, and when it was read.
type BudgetReading struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	ReadAt    time.Time `json:"read_at"`
}

// budgetStop is a refusal's stop: no polling until Until. Logged is whether a
// polling read has already said it skipped for it — once per stop, not once
// per call.
type budgetStop struct {
	Until  time.Time `json:"until"`
	Logged bool      `json:"logged,omitempty"`
}

// budgetState is the file.
type budgetState struct {
	Last *BudgetReading `json:"last,omitempty"`
	Prev *BudgetReading `json:"prev,omitempty"`
	Stop *budgetStop    `json:"stop,omitempty"`
}

// load reads the file; one missing or unreadable is no budget kept yet.
func (b *Budget) load() budgetState {
	var st budgetState
	data, err := os.ReadFile(b.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logging.Action("could not read the GitHub budget", "error", err)
		}
		return st
	}
	if err := json.Unmarshal(data, &st); err != nil {
		logging.Action("could not parse the GitHub budget", "error", err)
		return budgetState{}
	}
	return st
}

// save writes the file whole and atomically, so another nat reading it
// meanwhile reads this state or the last. A failure is logged: the budget
// throttles less well, nothing more.
func (b *Budget) save(st budgetState) {
	data, err := json.Marshal(st)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(b.path), 0o755)
	}
	if err == nil {
		tmp := b.path + ".tmp"
		if err = os.WriteFile(tmp, data, 0o644); err == nil {
			if err = os.Rename(tmp, b.path); err != nil {
				_ = os.Remove(tmp)
			}
		}
	}
	if err != nil {
		logging.Action("could not keep the GitHub budget", "error", err)
	}
}

// record keeps a reading's rate limit as the last, the one before as the
// previous: the two the burn rate is read off.
func (b *Budget) record(rl *RateLimit) {
	if b == nil || rl == nil {
		return
	}
	st := b.load()
	st.Prev = st.Last
	st.Last = &BudgetReading{Limit: rl.Limit, Remaining: rl.Remaining, ResetAt: rl.ResetAt, ReadAt: b.now()}
	b.save(st)
}

// refuse records the stop a refusal earns — until the last reading's reset,
// else [stopFallback] from now — and returns the error an action fails with.
// A refusal for the stop already recorded keeps its Logged.
func (b *Budget) refuse() *LimitError {
	now := b.now()
	st := b.load()
	until := now.Add(stopFallback)
	if st.Last != nil && st.Last.ResetAt.After(now) {
		until = st.Last.ResetAt
	}
	if st.Stop == nil || !st.Stop.Until.Equal(until) {
		st.Stop = &budgetStop{Until: until}
	}
	b.save(st)
	logging.Action("GitHub refused on its API limit: polling stopped", "until", until.Format(time.RFC3339))
	return &LimitError{Until: until, now: now}
}

// clear drops a stop: a call that went through is GitHub answering again.
func (b *Budget) clear() {
	if b == nil {
		return
	}
	st := b.load()
	if st.Stop == nil {
		return
	}
	st.Stop = nil
	b.save(st)
	logging.Action("GitHub answered again: polling stop cleared")
}

// pausedPoll reports whether a polling read is to be skipped — a stop whose
// retry time is still ahead — saying so in the log the first time for each
// stop.
func (b *Budget) pausedPoll() bool {
	if b == nil {
		return false
	}
	now := b.now()
	st := b.load()
	if st.Stop == nil || !now.Before(st.Stop.Until) {
		return false
	}
	if !st.Stop.Logged {
		logging.Action("left the pull requests unread: GitHub's API limit is spent",
			"until", st.Stop.Until.Format(time.RFC3339))
		st.Stop.Logged = true
		b.save(st)
	}
	return true
}

// Outlook is the budget's policy for the next polling read: the last
// reading, what it projects will be left at the reset, whether polling is
// stretched (Throttled) or stopped (PausedUntil, zero unless it is), and the
// interval nat wants before the next reading.
type Outlook struct {
	Reading     *BudgetReading
	Projected   int
	Throttled   bool
	PausedUntil time.Time
	PollAfter   time.Duration
}

// Outlook is the policy as the budget stands, for a poll configured at poll.
func (b *Budget) Outlook(poll time.Duration) Outlook {
	if b == nil {
		return Outlook{PollAfter: poll}
	}
	return outlookOf(b.load(), b.now(), poll)
}

// outlookOf is the policy, in one place.
//
// The projection is remaining − rate × (reset − now), the rate read off the
// last two readings of the same hour — everyone's spend on the token, not
// only nat's. The reserve is a fifth of the limit. While the projection stays
// at or above it, the interval is poll. Once it would dip below, polling
// spends only what is spare: the time to the reset over the points above the
// reserve, a point a reading, floored at poll and capped at [budgetCap] —
// already at or under the reserve, the cap. A reset already passed is a full
// hour again. A stop whose retry time is ahead is a pause until then, never
// shorter than poll. No path asks for a reading GitHub would refuse.
func outlookOf(st budgetState, now time.Time, poll time.Duration) Outlook {
	o := Outlook{Reading: st.Last, PollAfter: poll}
	if last := st.Last; last != nil {
		left := last.ResetAt.Sub(now)
		if left <= 0 {
			o.Projected = last.Limit
		} else {
			rate := burnRate(st.Prev, last)
			o.Projected = max(0, last.Remaining-int(math.Ceil(rate*left.Seconds())))
			reserve := last.Limit / budgetReserveShare
			if o.Projected < reserve {
				stretch := budgetCap
				if spare := last.Remaining - reserve; spare > 0 {
					stretch = min(budgetCap, left/time.Duration(spare))
				}
				o.PollAfter = max(poll, ceilSecond(stretch))
				o.Throttled = o.PollAfter > poll
			}
		}
	}
	if st.Stop != nil && now.Before(st.Stop.Until) {
		o.PausedUntil = st.Stop.Until
		o.PollAfter = max(poll, ceilSecond(st.Stop.Until.Sub(now)))
		o.Throttled = false
	}
	return o
}

// burnRate is points a second between two readings of one hour; zero where
// there is no earlier reading, the hour reset between them, or nothing was
// spent.
func burnRate(prev, last *BudgetReading) float64 {
	if prev == nil || !prev.ResetAt.Equal(last.ResetAt) {
		return 0
	}
	elapsed := last.ReadAt.Sub(prev.ReadAt).Seconds()
	used := prev.Remaining - last.Remaining
	if elapsed <= 0 || used <= 0 {
		return 0
	}
	return float64(used) / elapsed
}

// ceilSecond rounds d up to a whole second.
func ceilSecond(d time.Duration) time.Duration {
	if r := d % time.Second; r != 0 {
		return d - r + time.Second
	}
	return d
}

// LimitError is a gh call GitHub refused on its API limit, and when to try
// again.
type LimitError struct {
	Until time.Time
	now   time.Time
}

// Error names the retry time in local time — its date too, where that is not
// today.
func (e *LimitError) Error() string {
	at := e.Until.Local()
	when := at.Format("15:04")
	if y, m, d := at.Date(); e.now.IsZero() || !sameDay(e.now.Local(), y, m, d) {
		when = at.Format("2 Jan 15:04")
	}
	return fmt.Sprintf("GitHub's API limit is spent until %s; try again then", when)
}

func sameDay(t time.Time, y int, m time.Month, d int) bool {
	ty, tm, td := t.Date()
	return ty == y && tm == m && td == d
}

// limitRefusals are gh's words for a rate-limit refusal, lower-cased: the
// primary limit (as REST and GraphQL each put it) and the secondary one.
var limitRefusals = []string{
	"api rate limit already exceeded",
	"api rate limit exceeded",
	"exceeded a secondary rate limit",
}

// isLimitRefusal reports whether text is gh refusing on GitHub's API limit.
func isLimitRefusal(text string) bool {
	text = strings.ToLower(text)
	for _, words := range limitRefusals {
		if strings.Contains(text, words) {
			return true
		}
	}
	return false
}

// budgetRunner is a Runner that keeps the budget: a call refused on the limit
// records a stop and fails as a [LimitError]; one that went through clears
// any stop.
type budgetRunner struct {
	inner  Runner
	budget *Budget
}

var _ StdinRunner = budgetRunner{}

func (r budgetRunner) Run(dir, name string, args ...string) (string, error) {
	out, err := r.inner.Run(dir, name, args...)
	return out, r.budget.observe(err)
}

func (r budgetRunner) RunWithStdin(dir string, stdin io.Reader, name string, args ...string) (string, error) {
	inner, ok := r.inner.(StdinRunner)
	if !ok {
		return "", fmt.Errorf("%s runner cannot carry a comment on its standard input", Binary)
	}
	out, err := inner.RunWithStdin(dir, stdin, name, args...)
	return out, r.budget.observe(err)
}

// observe is what a finished call tells the budget, and the error the caller
// sees.
func (b *Budget) observe(err error) error {
	if err == nil {
		b.clear()
		return nil
	}
	text := err.Error()
	var exit *ExitError
	if errors.As(err, &exit) {
		text = exit.Stderr
	}
	if !isLimitRefusal(text) {
		return err
	}
	return b.refuse()
}
