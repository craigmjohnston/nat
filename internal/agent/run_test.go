package agent

import (
	"slices"
	"strings"
	"testing"
)

func TestRunSessionName(t *testing.T) {
	for _, tt := range []struct{ id, label, want string }{
		{"3ef38308-f654-8197-b70f-df65ce31f137", "Run", "nat-run-ce31f137-run"},
		{"3b738308-f654-811c-948d-e1fb36f71df3", "Dev server: 2", "nat-run-36f71df3-dev-server-2"},
		{"abc", "▶", "nat-run-abc-run"},
	} {
		if got := RunSessionName(tt.id, tt.label); got != tt.want {
			t.Errorf("RunSessionName(%q, %q) = %q, want %q", tt.id, tt.label, got, tt.want)
		}
	}
}

// subcommands is each call's tmux subcommand, the -u flag stepped over.
func subcommands(r *fakeRunner) []string {
	var out []string
	for _, c := range r.calls {
		out = append(out, c.args[1])
	}
	return out
}

// callOf is the first call of a subcommand.
func callOf(r *fakeRunner, sub string) []string {
	for _, c := range r.calls {
		if c.args[1] == sub {
			return c.args
		}
	}
	return nil
}

// A run with nothing of its name live starts the command by sh -c in the
// directory and tags the pane as a run — never as a slice.
func TestLaunchRunStartsAndTagsARun(t *testing.T) {
	r := &fakeRunner{
		// display-message -p falls back to the current client for a target
		// that is not there, answering rather than failing: has-session is
		// what says there is nothing to end.
		outs: map[string]string{"new-session": "%9\n", "display-message": "\n"},
		errs: map[string]error{"has-session": &ExitError{Code: 1, Stderr: "can't find session"}},
	}
	if err := NewTmuxWithRunner(r).LaunchRun("nat-run-x-run", "/w/run-main", "make run", "p:Run"); err != nil {
		t.Fatalf("LaunchRun: %v", err)
	}
	if slices.Contains(subcommands(r), "kill-session") {
		t.Errorf("calls = %v, want nothing killed", subcommands(r))
	}
	ns := strings.Join(callOf(r, "new-session"), " ")
	for _, want := range []string{"-s nat-run-x-run", "-c /w/run-main", "sh -c make run", "status off", "mouse on"} {
		if !strings.Contains(ns, want) {
			t.Errorf("new-session = %q, want %q", ns, want)
		}
	}
	tag := callOf(r, "set-option")
	if !slices.Equal(tag[2:], []string{"-p", "-t", "%9", RunPaneOption, "p:Run"}) {
		t.Errorf("tag = %v", tag)
	}
	for _, c := range r.calls {
		if slices.Contains(c.args, SlicePaneOption) {
			t.Errorf("a run's pane was given a slice tag: %v", c.args)
		}
	}
}

// The same run still live is killed by its exact name before it starts again.
func TestLaunchRunReplacesALiveRun(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"display-message": "p:Run\n", "new-session": "%9\n"}}
	if err := NewTmuxWithRunner(r).LaunchRun("nat-run-x-run", "/w", "make run", "p:Run"); err != nil {
		t.Fatalf("LaunchRun: %v", err)
	}
	subs := subcommands(r)
	k, n := slices.Index(subs, "kill-session"), slices.Index(subs, "new-session")
	if k < 0 || k > n {
		t.Fatalf("calls = %v, want the earlier run killed before the new one starts", subs)
	}
	if got := callOf(r, "kill-session"); !slices.Equal(got[2:], []string{"-t", "=nat-run-x-run"}) {
		t.Errorf("kill = %v, want the exact name", got)
	}
	if got := callOf(r, "has-session"); got[3] != "=nat-run-x-run" {
		t.Errorf("has-session = %v, want the exact name", got)
	}
	if got := callOf(r, "display-message"); got[4] != "=nat-run-x-run:" {
		t.Errorf("display-message = %v, want the exact name", got)
	}
}

// A session of the run's name with no run tag is not nat's: it is left alone
// and the run refused.
func TestLaunchRunLeavesAForeignSessionAlone(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"display-message": "\n"}}
	err := NewTmuxWithRunner(r).LaunchRun("nat-run-x-run", "/w", "make run", "p:Run")
	if err == nil || !strings.Contains(err.Error(), "not a run of nat's") {
		t.Fatalf("err = %v", err)
	}
	if slices.Contains(subcommands(r), "kill-session") || slices.Contains(subcommands(r), "new-session") {
		t.Errorf("calls = %v, want nothing killed or started", subcommands(r))
	}
}

func TestLaunchRunFailures(t *testing.T) {
	gone := &ExitError{Code: 1, Stderr: "can't find session"}
	for name, r := range map[string]*fakeRunner{
		"read tag": {errs: map[string]error{"display-message": &ExitError{Code: 1}}},
		"kill":     {outs: map[string]string{"display-message": "p:Run"}, errs: map[string]error{"kill-session": &ExitError{Code: 1}}},
		"start":    {errs: map[string]error{"has-session": gone, "new-session": &ExitError{Code: 1, Stderr: "bad dir"}}},
		"tag":      {outs: map[string]string{"new-session": "%9"}, errs: map[string]error{"has-session": gone, "set-option": &ExitError{Code: 1}}},
	} {
		if err := NewTmuxWithRunner(r).LaunchRun("nat-run-x-run", "/w", "make run", "p:Run"); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
