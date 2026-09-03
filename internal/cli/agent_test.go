package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/notion"
)

// agentTestRunner is a fake tmux runner, standing in for the tmux binary
// across every command that reads or drives a live session: which slices are
// running (list-panes), a prompt sent to one (set-buffer, paste-buffer), an
// interrupt (send-keys Escape) and a launch (new-session, set-option).
type agentTestRunner struct {
	liveSessions map[string]string
	liveErr      string // returned as an ExitError from list-panes when set
	// liveFatalErr is a list-panes failure tmux's own "no server" exit code
	// (1) does not swallow — everything list-panes can genuinely fail with,
	// short of there being no server at all.
	liveFatalErr string
	sends        []struct {
		session string
		prompt  string
	}
	sendErr      string // returned as an ExitError from paste-buffer when set
	interrupts   []string
	interruptErr string // returned as an ExitError from send-keys Escape when set
	// stagingBuffer holds the most recent prompt text staged for sending.
	stagingBuffer string

	// launchPane is the pane ID new-session answers with; "%1" when unset.
	launchPane string
	launchErr  string // returned as an ExitError from new-session when set
	// launchArgs is the full new-session argv, for a test that wants to read
	// back the model and effort a launch was given.
	launchArgs []string
	// tagged records the slice ID every launched pane was tagged with.
	tagged []string
}

func (r *agentTestRunner) Run(name string, args ...string) (string, error) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list-panes":
		if r.liveFatalErr != "" {
			return "", fmt.Errorf("%s", r.liveFatalErr)
		}
		if r.liveErr != "" {
			return "", &agent.ExitError{Code: 1, Stderr: r.liveErr}
		}
		return r.formatPanes(), nil
	case "set-buffer":
		// set-buffer -b <buffer> -- <prompt>: the prompt is everything after --.
		for i := 0; i < len(args); i++ {
			if args[i] == "--" && i+1 < len(args) {
				r.stagingBuffer = args[i+1]
				break
			}
		}
		return "", nil
	case "paste-buffer":
		if r.sendErr != "" {
			return "", &agent.ExitError{Code: 1, Stderr: r.sendErr}
		}
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-t" {
				session := args[i+1]
				r.sends = append(r.sends, struct {
					session string
					prompt  string
				}{session, r.stagingBuffer})
				r.stagingBuffer = ""
				return "", nil
			}
		}
		return "", nil
	case "send-keys":
		if r.interruptErr != "" {
			return "", &agent.ExitError{Code: 1, Stderr: r.interruptErr}
		}
		// send-keys is used for both the interrupt and the enter after a
		// pasted prompt; only the former names Escape.
		if len(args) > 2 && args[1] == "-t" {
			session := args[2]
			if len(args) > 3 && args[3] == "Escape" {
				r.interrupts = append(r.interrupts, session)
			}
		}
		return "", nil
	case "new-session":
		r.launchArgs = args
		if r.launchErr != "" {
			return "", &agent.ExitError{Code: 1, Stderr: r.launchErr}
		}
		pane := r.launchPane
		if pane == "" {
			pane = "%1"
		}
		return pane + "\n", nil
	case "set-option":
		if len(args) > 0 {
			r.tagged = append(r.tagged, args[len(args)-1])
		}
		return "", nil
	}
	return "", fmt.Errorf("unexpected command: %s %v", name, args)
}

func (r *agentTestRunner) formatPanes() string {
	var result string
	// Format: #{@nat_slice}\t#{pane_id}\t#{session_name}\t#{window_id}\t#{pane_dead}
	for sliceID, session := range r.liveSessions {
		// sliceID\tpane_id\tsession_name\twindow_id\tpane_dead
		result += fmt.Sprintf("%s\t%s\t%s\t@0\t0\n", sliceID, "pane_id", session)
	}
	return result
}

func TestAgentSendRefusesEmptyPrompt(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	env.In = strings.NewReader("")
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", sliceID, "--text", "", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-send: expected error for empty prompt")
	}
	if !strings.Contains(err.Error(), "no prompt") {
		t.Errorf("agent-send error: %v, want 'no prompt'", err)
	}
}

func TestAgentSendRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-send: expected error for missing slice")
	}
	if !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("agent-send error: %v, want 'want exactly one'", err)
	}
}

func TestAgentSendWithLiveSession(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{testSliceID: "nat-abcd1234"},
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", testSliceID, "--text", "Fix the button", "--project", "project-1",
	}, env)
	if err != nil {
		t.Errorf("agent-send: unexpected error: %v", err)
	}
	if len(runner.sends) != 1 {
		t.Fatalf("sends: got %d, want 1", len(runner.sends))
	}
	if runner.sends[0].session != "nat-abcd1234" {
		t.Errorf("session: got %q, want %q", runner.sends[0].session, "nat-abcd1234")
	}
	if runner.sends[0].prompt != "Fix the button" {
		t.Errorf("prompt: got %q, want 'Fix the button'", runner.sends[0].prompt)
	}
}

func TestAgentSendRefusesNoLiveSession(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{},
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", testSliceID, "--text", "Fix the button", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-send: expected error for no live session")
	}
	if !strings.Contains(err.Error(), "no live session") {
		t.Errorf("agent-send error: %v, want 'no live session'", err)
	}
}

func TestAgentSendFromStdin(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{testSliceID: "nat-abcd1234"},
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	env.In = strings.NewReader("Read the docs")
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Errorf("agent-send: unexpected error: %v", err)
	}
	if len(runner.sends) != 1 {
		t.Fatalf("sends: got %d, want 1", len(runner.sends))
	}
	if runner.sends[0].prompt != "Read the docs" {
		t.Errorf("prompt: got %q, want 'Read the docs'", runner.sends[0].prompt)
	}
}

func TestAgentSendFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{testSliceID: "nat-abcd1234"},
		sendErr:      "tmux failed",
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", testSliceID, "--text", "Fix the button", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-send: expected error from tmux")
	}
}

func TestAgentInterruptRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-interrupt", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-interrupt: expected error for missing slice")
	}
	if !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("agent-interrupt error: %v, want 'want exactly one'", err)
	}
}

func TestAgentInterruptWithLiveSession(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{testSliceID: "nat-abcd1234"},
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-interrupt", testSliceID, "--project", "project-1",
	}, env)
	if err != nil {
		t.Errorf("agent-interrupt: unexpected error: %v", err)
	}
	if len(runner.interrupts) != 1 {
		t.Fatalf("interrupts: got %d, want 1", len(runner.interrupts))
	}
	if runner.interrupts[0] != "nat-abcd1234" {
		t.Errorf("session: got %q, want %q", runner.interrupts[0], "nat-abcd1234")
	}
}

func TestAgentInterruptRefusesNoLiveSession(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{},
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-interrupt", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-interrupt: expected error for no live session")
	}
	if !strings.Contains(err.Error(), "no live session") {
		t.Errorf("agent-interrupt error: %v, want 'no live session'", err)
	}
}

func TestAgentInterruptFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{
		liveSessions: map[string]string{testSliceID: "nat-abcd1234"},
		interruptErr: "tmux failed",
	}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-interrupt", testSliceID, "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-interrupt: expected error from tmux")
	}
}

func TestAgentSendInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-send", "not-a-uuid", "--text", "Fix it", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-send: expected error for invalid slice ID")
	}
	if !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("agent-send error: %v, want 'not a slice'", err)
	}
}

func TestAgentInterruptInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	var out strings.Builder
	env.Out = &out

	err := Run(context.Background(), []string{
		"agent-interrupt", "not-a-uuid", "--project", "project-1",
	}, env)
	if err == nil {
		t.Errorf("agent-interrupt: expected error for invalid slice ID")
	}
	if !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("agent-interrupt error: %v, want 'not a slice'", err)
	}
}

// errReader is a stdin that fails, for the read the prompt commands make when
// no --text is given.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestAgentSendRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"agent-send", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestAgentSendRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"agent-send", testSliceID, "--text", "x", "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestAgentSendRefusesNoStdin(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	env.In = nil

	err := Run(context.Background(), []string{"agent-send", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no stdin") {
		t.Errorf("err = %v, want 'no stdin'", err)
	}
}

func TestAgentSendReportsAFailedStdinRead(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})
	env.In = errReader{err: errors.New("pipe closed")}

	err := Run(context.Background(), []string{"agent-send", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "read the prompt: pipe closed") {
		t.Errorf("err = %v, want the read's failure named", err)
	}
}

func TestAgentSendReportsAFailedLiveRead(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{liveFatalErr: "no server running"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{
		"agent-send", testSliceID, "--text", "Fix it", "--project", "project-1",
	}, env)

	if err == nil || !strings.Contains(err.Error(), "could not read live sessions") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

func TestAgentInterruptRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"agent-interrupt", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestAgentInterruptRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testClaimConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"agent-interrupt", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestAgentInterruptReportsAFailedLiveRead(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceInProgress, "m1", "Craig Johnston", "")},
		},
	}
	env, _ := testEnv(testClaimConfig(), api)
	runner := &agentTestRunner{liveFatalErr: "no server running"}
	env.NewTmux = func() *agent.Tmux { return agent.NewTmuxWithRunner(runner) }

	err := Run(context.Background(), []string{"agent-interrupt", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "could not read live sessions") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}
