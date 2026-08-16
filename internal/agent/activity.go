package agent

import (
	"errors"
	"strings"

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

// workingMarker is what Claude Code prints on its status line for as long as it
// is busy and only while it is busy — the hint that the key which interrupts it
// is escape. It is matched case-folded and against the visible screen alone, so
// the same words further up the scrollback (an agent that has printed this very
// sentence) say nothing about now.
//
// It is the marker rather than the composer box or a prompt's wording because
// it is the one thing every busy screen has and no idle one does: Claude Code
// stops for input in several shapes — a permission prompt, a question, the end
// of a turn — and enumerating them would be a list that goes stale, where
// "anything that is not busy is waiting" cannot.
const workingMarker = "esc to interrupt"

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
	if strings.Contains(strings.ToLower(out), workingMarker) {
		return ActivityWorking
	}
	return ActivityWaiting
}
