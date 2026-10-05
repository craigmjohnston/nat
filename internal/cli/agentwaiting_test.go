package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
)

// paneRunner answers display-message with one pane's ID and tag and records
// every tmux call, so a test can see that a refusal wrote nothing.
type paneRunner struct {
	display    string
	displayErr error
	setErr     error
	calls      [][]string
}

func (r *paneRunner) Run(_ string, args ...string) (string, error) {
	r.calls = append(r.calls, args)
	switch args[1] {
	case "display-message":
		return r.display, r.displayErr
	case "set-option":
		return "", r.setErr
	}
	return "", nil
}

// sets is every set-option call the runner was asked to make.
func (r *paneRunner) sets() [][]string {
	var sets [][]string
	for _, c := range r.calls {
		if c[1] == "set-option" {
			sets = append(sets, c)
		}
	}
	return sets
}

func markEnv(r *paneRunner, out *strings.Builder, nudges *int) Env {
	return Env{
		NewTmux: func() *agent.Tmux { return agent.NewTmuxWithRunner(r) },
		Out:     out,
		Nudge:   func() { *nudges++ },
	}
}

func TestAgentWaitingAndWorkingMarkTheCallersOwnPane(t *testing.T) {
	for _, tc := range []struct {
		command string
		wantSet []string
		wantOut string
	}{
		{"agent-waiting", []string{"-u", "set-option", "-p", "-t", "%7", agent.WaitingPaneOption, "1"},
			"Marked as waiting on the user.\n"},
		{"agent-working", []string{"-u", "set-option", "-p", "-u", "-t", "%7", agent.WaitingPaneOption},
			"Marked as working.\n"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			t.Setenv(agent.PaneEnv, "%7")
			r := &paneRunner{display: "%7\tslice-1\n"}
			var out strings.Builder
			nudges := 0

			if err := Run(context.Background(), []string{tc.command}, markEnv(r, &out, &nudges)); err != nil {
				t.Fatalf("%s: %v", tc.command, err)
			}
			if got := r.sets(); !reflect.DeepEqual(got, [][]string{tc.wantSet}) {
				t.Errorf("sets = %v, want %v", got, tc.wantSet)
			}
			if out.String() != tc.wantOut {
				t.Errorf("out = %q, want %q", out.String(), tc.wantOut)
			}
			if nudges != 1 {
				t.Errorf("nudges = %d, want 1", nudges)
			}
		})
	}
}

// Every refusal lands before any tmux write and nudges nothing.
func TestAgentWaitingRefusals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pane    string
		args    []string
		display string
		wantErr string
	}{
		{"outside tmux", "", nil, "", "$TMUX_PANE is unset"},
		{"an untagged pane", "%7", nil, "%7\t\n", "not one nat launched an agent in"},
		{"a pane tmux cannot find", "%7", nil, "\n", "not one nat launched an agent in"},
		{"a project pinned", "%7", []string{"--project", "x"}, "%7\tslice-1\n", "flag provided but not defined"},
		{"an argument", "%7", []string{"slice-1"}, "%7\tslice-1\n", "unexpected argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agent.PaneEnv, tc.pane)
			r := &paneRunner{display: tc.display}
			var out strings.Builder
			nudges := 0

			err := Run(context.Background(), append([]string{"agent-waiting"}, tc.args...), markEnv(r, &out, &nudges))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to say %q", err, tc.wantErr)
			}
			if got := r.sets(); len(got) != 0 {
				t.Errorf("sets = %v, want none", got)
			}
			if nudges != 0 {
				t.Errorf("nudges = %d, want none", nudges)
			}
		})
	}
}

// A tmux that fails outright is the command's error, with no nudge.
func TestAgentWorkingTmuxFails(t *testing.T) {
	boom := errors.New("boom")
	for _, r := range []*paneRunner{
		{displayErr: boom},
		{display: "%7\tslice-1\n", setErr: boom},
	} {
		t.Setenv(agent.PaneEnv, "%7")
		var out strings.Builder
		nudges := 0
		err := Run(context.Background(), []string{"agent-working"}, markEnv(r, &out, &nudges))
		if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "agent-working: ") {
			t.Errorf("err = %v, want agent-working's wrap of %v", err, boom)
		}
		if nudges != 0 {
			t.Errorf("nudges = %d, want none", nudges)
		}
	}
}
