package agent

import (
	"errors"
	"reflect"
	"testing"
)

// A live agent pane is working unless its agent has said it is waiting; a dead
// one is gone whatever it said. The board's own pane is untagged and left out.
// Nothing is read off any screen: the scan is the one tmux call.
func TestActivityClassifiesEachAgentPane(t *testing.T) {
	working := pane{slice: "slice-working", id: "%1", session: "nat-1", window: "@1"}
	waiting := pane{slice: "slice-waiting", id: "%2", session: "nat-2", window: "@2", waiting: true}
	dead := pane{slice: "slice-dead", id: "%3", session: "nat-3", window: "@3", dead: true, waiting: true}

	r := &fakeRunner{outs: map[string]string{"list-panes": panesOutput(boardPane, working, waiting, dead)}}

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
	if len(r.calls) != 1 || r.calls[0].args[1] != "list-panes" {
		t.Errorf("calls = %v, want the one pane scan and no capture-pane", r.calls)
	}
}

// A slice with two panes tagged for it is answered for once, by the same pane
// LiveSlices names — the first one found.
func TestActivityAnswersOncePerSlice(t *testing.T) {
	first := pane{slice: "slice", id: "%1", session: "nat-1", window: "@1"}
	second := pane{slice: "slice", id: "%2", session: "nat-2", window: "@2", waiting: true}
	r := &fakeRunner{outs: map[string]string{"list-panes": panesOutput(first, second)}}

	activity, err := NewTmuxWithRunner(r).Activity()
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got := activity["slice"]; got != ActivityWorking {
		t.Errorf("activity = %v, want the first pane's %v", got, ActivityWorking)
	}
}

func TestSetWaiting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		waiting bool
		want    []string
	}{
		{"sets the flag", true, []string{"-u", "set-option", "-p", "-t", "%4", WaitingPaneOption, "1"}},
		{"clears it", false, []string{"-u", "set-option", "-p", "-u", "-t", "%4", WaitingPaneOption}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{outs: map[string]string{"display-message": "%4\tplan:project\n"}}
			if err := NewTmuxWithRunner(r).SetWaiting("%4", tc.waiting); err != nil {
				t.Fatalf("SetWaiting: %v", err)
			}
			want := []call{
				{name: TmuxBinary, args: []string{"-u", "display-message", "-p", "-t", "%4", "#{pane_id}\t#{@nat_slice}"}},
				{name: TmuxBinary, args: tc.want},
			}
			if !reflect.DeepEqual(r.calls, want) {
				t.Errorf("calls = %v, want %v", r.calls, want)
			}
		})
	}
}

// A pane with no agent tag, or one tmux cannot find (it answers that with an
// empty line), is refused with nothing written.
func TestSetWaitingRefusesAPaneNatDidNotLaunch(t *testing.T) {
	for _, display := range []string{"%4\t\n", "\n", "%9\tslice\n"} {
		r := &fakeRunner{outs: map[string]string{"display-message": display}}
		if err := NewTmuxWithRunner(r).SetWaiting("%4", true); !errors.Is(err, ErrNotAgentPane) {
			t.Errorf("display %q: err = %v, want %v", display, err, ErrNotAgentPane)
		}
		if len(r.calls) != 1 {
			t.Errorf("display %q: calls = %v, want the read alone", display, r.calls)
		}
	}
}

func TestSetWaitingTmuxFails(t *testing.T) {
	boom := errors.New("boom")
	for _, r := range []*fakeRunner{
		{errs: map[string]error{"display-message": boom}},
		{outs: map[string]string{"display-message": "%4\tslice\n"}, errs: map[string]error{"set-option": boom}},
	} {
		if err := NewTmuxWithRunner(r).SetWaiting("%4", true); !errors.Is(err, boom) {
			t.Errorf("err = %v, want it to wrap %v", err, boom)
		}
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
