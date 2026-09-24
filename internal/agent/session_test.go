package agent

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
)

func TestSessionTag(t *testing.T) {
	tag := SessionTag("proj-1", "sess-1")
	if tag != "session:proj-1:sess-1" {
		t.Errorf("SessionTag = %q, want %q", tag, "session:proj-1:sess-1")
	}
	if !IsSessionTag(tag) {
		t.Errorf("IsSessionTag(%q) = false, want true", tag)
	}
	if IsSessionTag("3b738308f6548180989bd53fb10249ee") {
		t.Error("IsSessionTag on a slice page ID = true, want false")
	}
	if IsSessionTag(PlanTag("proj-1")) {
		t.Error("IsSessionTag on a plan tag = true, want false")
	}
}

func TestAdHocSessionName(t *testing.T) {
	got := AdHocSessionName("8f654180-9b8d-53fb-1024-9ee08f654180")
	if !strings.HasPrefix(got, SessionPrefix+"session-") {
		t.Errorf("AdHocSessionName = %q, want prefix %q", got, SessionPrefix+"session-")
	}
	if got != SessionPrefix+"session-"+SessionIDPrefix("8f654180-9b8d-53fb-1024-9ee08f654180") {
		t.Errorf("AdHocSessionName = %q, want it built from SessionIDPrefix", got)
	}
}

func TestLaunchBareTagsThePaneAndRunsNoPrompt(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"new-session": "%7\n"}}
	tmux := NewTmuxWithRunner(r)
	if err := tmux.LaunchBare("nat-session-abcd1234", "/tmp/work", "session:proj:sess", config.AgentModel{Model: "opus"}); err != nil {
		t.Fatalf("LaunchBare: %v", err)
	}

	var sawClaude, sawTag bool
	for _, c := range r.calls {
		for i, a := range c.args {
			if strings.HasPrefix(a, `claude --model 'opus' --settings '{"theme":"auto","statusLine":`) {
				sawClaude = true
			}
			if a == "@nat_slice" && i+1 < len(c.args) && c.args[i+1] == "session:proj:sess" {
				sawTag = true
			}
		}
	}
	if !sawClaude {
		t.Errorf("calls = %+v, want a bare claude command with no prompt file", r.calls)
	}
	if !sawTag {
		t.Errorf("calls = %+v, want the pane tagged with the session's own tag", r.calls)
	}
}

func TestLaunchBareError(t *testing.T) {
	inner := &ExitError{Code: 1, Stderr: "duplicate session"}
	if err := NewTmuxWithRunner(&fakeRunner{err: inner}).LaunchBare("nat-session-x", "/tmp", "session:p:s", config.AgentModel{}); err == nil {
		t.Fatal("LaunchBare: want error, got nil")
	}
}

func TestLaunchBareTagFailureIsReported(t *testing.T) {
	r := &fakeRunner{
		outs: map[string]string{"new-session": "%7\n"},
		errs: map[string]error{"set-option": &ExitError{Code: 1, Stderr: "no such pane"}},
	}
	if err := NewTmuxWithRunner(r).LaunchBare("nat-session-x", "/tmp", "session:p:s", config.AgentModel{}); err == nil {
		t.Fatal("LaunchBare: want the tag failure reported")
	}
}
