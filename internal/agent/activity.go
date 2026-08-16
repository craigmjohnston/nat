package agent

import (
	"errors"
	"regexp"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Activity is how the agent in a pane is getting on: working away, stopped and
// waiting to be told something, or gone. It is a reading of one moment, taken
// by polling — there is nothing to subscribe to — so a caller re-reads it
// rather than being told when it changes.
//
// The zero value is the honest answer for a pane nothing could be read of: it
// is there, so it is neither gone nor known to have stopped.
type Activity int

const (
	// ActivityUnknown is a pane whose state could not be read.
	ActivityUnknown Activity = iota
	// ActivityWorking is an agent getting on with the slice.
	ActivityWorking
	// ActivityWaiting is one that has stopped and wants input — a permission
	// prompt, a question, or simply the end of its turn.
	ActivityWaiting
	// ActivityGone is a pane whose command has exited. It is a state a pane can
	// only be listed in where tmux's remain-on-exit is on; an agent whose pane
	// has been reaped is not in the reading at all.
	ActivityGone
)

// String names the state for logs and test failures.
func (a Activity) String() string {
	switch a {
	case ActivityWorking:
		return "working"
	case ActivityWaiting:
		return "waiting"
	case ActivityGone:
		return "gone"
	default:
		return "unknown"
	}
}

// workingLine is Claude Code's running status line, matched by its shape rather
// than by any of its wording: a verb that trails off, then the elapsed time of
// the turn in brackets — "✻ Quantumizing… (1m 6s · ↓ 2.1k tokens · thinking
// with medium effort)". Every busy screen has one and no idle one does, which
// is why it is the signal: Claude Code stops for input in several shapes — a
// permission prompt, a question, the end of a turn — and enumerating those
// would be a list that goes stale, where "anything that is not busy is waiting"
// cannot.
//
// The shape is what survives the wording changing under us, which is exactly
// what went wrong before: the marker was the hint that escape interrupts, and
// current Claude Code prints no such hint at any pane width, so every working
// agent read as waiting and its star stopped moving. The verb is a different
// word every turn and the bracket holds whatever the turn has to say for
// itself, but a turn in flight always counts up in there — where the line the
// same turn leaves behind when it is done has no ellipsis and no bracket at all
// ("✻ Crunched for 4s"), and so cannot be mistaken for it.
//
// It is matched against the visible screen alone, so the same shape further up
// the scrollback says nothing about now, and against the line as tmux joins it
// (-J), so a narrow pane's wrapped status line is one line again.
var workingLine = regexp.MustCompile(`…\s*\(\d+[smh]`)

// Activity reports how every agent on the server is getting on, keyed by the
// page ID of the slice its pane is tagged with — the same keys [Tmux.LiveSlices]
// answers in, so a caller can lay one over the other.
//
// It is a poll: a call is one scan of the panes plus one screen read per agent,
// all of them local socket calls, and the caller decides how often to take one.
// Panes that are not ours carry no slice tag and are left out.
func (t *Tmux) Activity() (map[string]Activity, error) {
	panes, err := t.panes()
	if err != nil {
		return nil, err
	}

	activity := map[string]Activity{}
	for _, p := range panes {
		if p.slice == "" {
			continue
		}
		// Two panes tagged for one slice should not happen; as in LiveSlices,
		// the first found is the answer, so both agree on which pane they mean.
		if _, seen := activity[p.slice]; seen {
			continue
		}
		activity[p.slice] = t.classify(p)
	}
	return activity, nil
}

// classify reads one agent pane's state. A dead pane is settled without a
// capture — there is nothing on its screen worth classifying.
func (t *Tmux) classify(p pane) Activity {
	if p.dead {
		return ActivityGone
	}
	out, err := t.runner.Run(TmuxBinary, "capture-pane", "-p", "-J", "-t", p.id)
	if err != nil {
		// tmux exits non-zero for a pane it cannot find, which is the pane
		// having gone between the scan and the capture — a race we lose often
		// enough, since an agent finishing is exactly when both happen. Anything
		// else (no tmux binary at all) leaves the state unread rather than
		// declaring an agent that is still there gone.
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return ActivityGone
		}
		logging.Error("could not read an agent pane's screen", "pane", p.id, "error", err)
		return ActivityUnknown
	}
	if workingLine.MatchString(out) {
		return ActivityWorking
	}
	return ActivityWaiting
}
