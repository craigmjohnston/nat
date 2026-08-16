package agent

import (
	"errors"
	"reflect"
	"testing"
)

// busyScreen is a pane mid-turn, as Claude Code draws it: the running status
// line, counting up the turn it is in, is the whole of the signal.
const busyScreen = `● Reading internal/agent/tmux.go

✻ Cogitating… (12s · ↓ 1.2k tokens · thinking with medium effort)`

// promptScreen is a pane stopped on a permission prompt — no running line
// anywhere on it, because nothing is running to count.
const promptScreen = `Do you want to make this edit to activity.go?
❯ 1. Yes
  2. No, and tell Claude what to do differently`

// TestActivityReadsTheRunningLine pins the shape the signal is read by, off
// screens Claude Code really draws: a turn in flight counts up in brackets
// after a verb that trails off, and everything it leaves behind does not.
func TestActivityReadsTheRunningLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want Activity
	}{
		{"thinking", "✢ Channelling… (3s · thinking with medium effort)", ActivityWorking},
		{"streaming", "✻ Fluttering… (4s · ↓ 177 tokens · thinking with medium effort)", ActivityWorking},
		{"past the minute", "✽ Quantumizing… (1m 6s · ↓ 2.1k tokens · almost done thinking)", ActivityWorking},
		{"an older build's hint", "✻ Cogitating… (12s · esc to interrupt)", ActivityWorking},
		{"the turn it leaves behind", "✻ Crunched for 4s", ActivityWaiting},
		{"a turn done with a shell still up", "✻ Churned for 4s · 1 shell still running", ActivityWaiting},
		{"a line tmux truncated", "tmux focus-events off · add 'set -g focus-events on' to ~/.tmux.conf and re…", ActivityWaiting},
		{"an idle composer", "❯ ", ActivityWaiting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agentPane := pane{slice: "slice", id: "%1", session: "nat-1", window: "@1"}
			r := &fakeRunner{
				outs:     map[string]string{"list-panes": panesOutput(agentPane)},
				captures: map[string]string{"%1": tc.line},
			}
			activity, err := NewTmuxWithRunner(r).Activity()
			if err != nil {
				t.Fatalf("Activity: %v", err)
			}
			if got := activity["slice"]; got != tc.want {
				t.Errorf("activity = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestActivityClassifiesEachAgentPane(t *testing.T) {
	working := pane{slice: "slice-working", id: "%1", session: "nat-1", window: "@1"}
	waiting := pane{slice: "slice-waiting", id: "%2", session: "nat-2", window: "@2"}
	dead := pane{slice: "slice-dead", id: "%3", session: "nat-3", window: "@3", dead: true}

	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(boardPane, working, waiting, dead)},
		captures: map[string]string{
			"%1": busyScreen,
			"%2": promptScreen,
		},
	}

	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}

	want := map[string]Activity{
		"slice-working": ActivityWorking,
		"slice-waiting": ActivityWaiting,
		"slice-dead":    ActivityGone,
	}
	if !reflect.DeepEqual(activity, want) {
		t.Errorf("activity = %v, want %v", activity, want)
	}

	// The board's own pane is untagged and the dead one needs no screen read, so
	// exactly the two live agents are captured.
	if got := captured(r); !reflect.DeepEqual(got, []string{"%1", "%2"}) {
		t.Errorf("captured %v, want the two live agent panes", got)
	}
}

// The running line is read off the visible screen, joined across a wrap: -J is
// what makes a narrow pane's status line one line again.
func TestActivityCaptureArguments(t *testing.T) {
	agentPane := pane{slice: "slice", id: "%1", session: "nat-1", window: "@1"}
	r := &fakeRunner{
		outs:     map[string]string{"list-panes": panesOutput(agentPane)},
		captures: map[string]string{"%1": busyScreen},
	}

	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got := activity["slice"]; got != ActivityWorking {
		t.Errorf("activity = %v, want %v", got, ActivityWorking)
	}

	want := []string{"capture-pane", "-p", "-J", "-t", "%1"}
	found := false
	for _, c := range r.calls {
		if c.args[0] == "capture-pane" {
			found = true
			if !reflect.DeepEqual(c.args, want) {
				t.Errorf("capture args = %v, want %v", c.args, want)
			}
		}
	}
	if !found {
		t.Error("no pane was captured")
	}
}

// A slice with two panes tagged for it is answered for once, by the same pane
// LiveSlices names — the first one found.
func TestActivityAnswersOncePerSlice(t *testing.T) {
	first := pane{slice: "slice", id: "%1", session: "nat-1", window: "@1"}
	second := pane{slice: "slice", id: "%2", session: "nat-2", window: "@2"}
	r := &fakeRunner{
		outs:     map[string]string{"list-panes": panesOutput(first, second)},
		captures: map[string]string{"%1": busyScreen, "%2": promptScreen},
	}

	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got := activity["slice"]; got != ActivityWorking {
		t.Errorf("activity = %v, want the first pane's %v", got, ActivityWorking)
	}
	if got := captured(r); !reflect.DeepEqual(got, []string{"%1"}) {
		t.Errorf("captured %v, want only the pane LiveSlices would name", got)
	}
}

// A pane that has gone between the scan and the capture reads as gone, not as
// an agent sitting there with nothing to say.
func TestActivityPaneVanishesMidRead(t *testing.T) {
	agentPane := pane{slice: "slice", id: "%1", session: "nat-1", window: "@1"}
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(agentPane)},
		errs: map[string]error{"capture-pane": &ExitError{Code: 1, Stderr: "can't find pane: %1"}},
	}

	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got := activity["slice"]; got != ActivityGone {
		t.Errorf("activity = %v, want %v", got, ActivityGone)
	}
}

// A capture that failed for any other reason — no tmux binary at all — leaves
// the state unread rather than declaring a running agent gone.
func TestActivityCaptureFailsOutright(t *testing.T) {
	agentPane := pane{slice: "slice", id: "%1", session: "nat-1", window: "@1"}
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(agentPane)},
		errs: map[string]error{"capture-pane": errors.New("exec: \"tmux\": executable file not found in $PATH")},
	}

	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got := activity["slice"]; got != ActivityUnknown {
		t.Errorf("activity = %v, want %v", got, ActivityUnknown)
	}
}

// No server at all is no agents, which is the ordinary state before the first
// launch rather than a failure.
func TestActivityWithNoServerRunning(t *testing.T) {
	r := &fakeRunner{err: &ExitError{Code: 1, Stderr: "no server running on /tmp/tmux-501/default"}}
	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if len(activity) != 0 {
		t.Errorf("activity = %v, want none", activity)
	}
}

// A pane scan that fails outright is reported, not answered with an empty
// reading that would read as every agent having stopped.
func TestActivityScanFails(t *testing.T) {
	boom := errors.New("boom")
	r := &fakeRunner{errs: map[string]error{"list-panes": boom}}
	if _, err := NewTmuxWithRunner(r).Activity(); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}

func TestActivityString(t *testing.T) {
	for _, tt := range []struct {
		activity Activity
		want     string
	}{
		{ActivityWorking, "working"},
		{ActivityWaiting, "waiting"},
		{ActivityGone, "gone"},
		{ActivityUnknown, "unknown"},
		{Activity(99), "unknown"},
	} {
		if got := tt.activity.String(); got != tt.want {
			t.Errorf("Activity(%d).String() = %q, want %q", tt.activity, got, tt.want)
		}
	}
}

// captured is the panes the runner was asked to capture, in order.
func captured(r *fakeRunner) []string {
	var panes []string
	for _, c := range r.calls {
		if c.args[0] == "capture-pane" {
			panes = append(panes, c.args[len(c.args)-1])
		}
	}
	return panes
}
