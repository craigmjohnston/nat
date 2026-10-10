package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Activity is how the agent in a pane is getting on: working away, stopped and
// waiting to be told something, or gone. It is a reading of one moment, taken
// by polling — there is nothing to subscribe to — so a caller re-reads it
// rather than being told when it changes.
//
// The zero value is the honest answer for a slice with no reading: nothing
// says it is gone, nor that it has stopped.
type Activity int

const (
	// ActivityUnknown is a slice with no reading of its pane.
	ActivityUnknown Activity = iota
	// ActivityWorking is an agent getting on with the slice.
	ActivityWorking
	// ActivityWaiting is one marked as stopped and needing the user (`nat
	// agent-waiting`, run by the embedded mod), and not yet marked back at work.
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

// Activity reports how every agent on the server is getting on, keyed by the
// page ID of the slice its pane is tagged with — the same keys [Tmux.LiveSlices]
// answers in, so a caller can lay one over the other.
//
// It is a poll: a call is one scan of the panes, a local socket call, and the
// caller decides how often to take one.
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
		activity[p.slice] = classify(p)
	}
	return activity, nil
}

// classify reads one agent pane's state off the scan alone: a dead pane is
// gone, a pane whose agent has said it needs the user ([WaitingPaneOption]) is
// waiting, and every other live pane is working. Nothing is read off the
// screen — what an agent is doing is the agent's to say, not nat's to infer.
func classify(p pane) Activity {
	switch {
	case p.dead:
		return ActivityGone
	case p.waiting:
		return ActivityWaiting
	}
	return ActivityWorking
}

// ErrNotAgentPane is [Tmux.SetWaiting] refusing a pane that carries no
// [SlicePaneOption] tag — one nat did not launch an agent in, or one that is
// not there at all.
var ErrNotAgentPane = errors.New("this pane is not one nat launched an agent in")

// SetWaiting sets the waiting flag ([WaitingPaneOption]) on the agent pane
// paneID, or clears it. Setting a flag already set and clearing one already
// clear both succeed. A pane with no agent tag is refused with
// [ErrNotAgentPane] before anything is written.
//
// The pane is read back by its own ID as well as its tag: tmux answers a
// display-message aimed at a pane it cannot find with an empty line rather
// than an error, so a pane that is not there reads as an untagged one.
func (t *Tmux) SetWaiting(paneID string, waiting bool) error {
	out, err := t.run("display-message", "-p", "-t", paneID, "#{pane_id}\t#{"+SlicePaneOption+"}")
	if err != nil {
		return fmt.Errorf("read tmux pane %s: %w", paneID, err)
	}
	id, tag, _ := strings.Cut(strings.TrimRight(out, "\r\n"), "\t")
	if id != paneID || tag == "" {
		return ErrNotAgentPane
	}
	args := []string{"set-option", "-p", "-t", paneID, WaitingPaneOption, "1"}
	if !waiting {
		args = []string{"set-option", "-p", "-u", "-t", paneID, WaitingPaneOption}
	}
	if _, err := t.run(args...); err != nil {
		return fmt.Errorf("mark tmux pane %s: %w", paneID, err)
	}
	logging.Action("agent marked its pane", "pane", paneID, "tag", tag, "waiting", waiting)
	return nil
}
