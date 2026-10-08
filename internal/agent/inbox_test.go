package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
)

// quickInbox shortens a send's wait on the mod for one test.
func quickInbox(t *testing.T, wait time.Duration) {
	t.Helper()
	oldWait, oldPoll := inboxWait, inboxPoll
	inboxWait, inboxPoll = wait, time.Millisecond
	t.Cleanup(func() { inboxWait, inboxPoll = oldWait, oldPoll })
}

// inboxRunner is a tmux whose session was launched with dir as its inbox.
func inboxRunner(dir string) *fakeRunner {
	return &fakeRunner{outs: map[string]string{"show-environment": inboxEnv + "=" + dir + "\n"}}
}

// takeOne stands for the mod: it waits for one prompt to land in dir, reads
// it and removes it, answering what it read and the file's and directory's
// modes on the channel.
type taken struct {
	name, text      string
	fileMode, dMode os.FileMode
}

func takeOne(t *testing.T, dir string) <-chan taken {
	t.Helper()
	got := make(chan taken, 1)
	go func() {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				path := filepath.Join(dir, e.Name())
				text, _ := os.ReadFile(path)
				info, _ := os.Stat(path)
				dinfo, _ := os.Stat(dir)
				_ = os.Remove(path)
				got <- taken{e.Name(), string(text), info.Mode().Perm(), dinfo.Mode().Perm()}
				return
			}
		}
		close(got)
	}()
	return got
}

func sent(r *fakeRunner, sub string) bool {
	return slices.ContainsFunc(r.calls, func(c call) bool { return len(c.args) > 1 && c.args[1] == sub })
}

// A session's inbox sits under nat's state directory, beside agent-status/.
func TestInboxDir(t *testing.T) {
	state, err := logging.Dir()
	if err != nil {
		t.Fatalf("logging.Dir: %v", err)
	}
	got, err := InboxDir("nat-b4463d8f")
	if err != nil || got != filepath.Join(state, "agent-inbox", "nat-b4463d8f") {
		t.Errorf("InboxDir = %q, %v; want agent-inbox/nat-b4463d8f under %s", got, err, state)
	}
}

// An inbox directory that cannot be resolved launches with none: the session
// is sent its prompts by paste.
func TestInboxEnvArgsWithNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	if got := inboxEnvArgs("nat-1"); got != nil {
		t.Errorf("inboxEnvArgs = %v, want none with no state directory", got)
	}
}

// Every agent and ad hoc session names its inbox where -e is taken, and on a
// tmux too old for it names none.
func TestLaunchesCarryTheInbox(t *testing.T) {
	dir, err := InboxDir("nat-1")
	if err != nil {
		t.Fatalf("InboxDir: %v", err)
	}
	want := inboxEnv + "=" + dir
	for _, tt := range []struct {
		version string
		want    bool
	}{{"tmux 3.5a\n", true}, {"tmux 3.0a\n", false}} {
		for name, launch := range map[string]func(*Tmux) error{
			"Launch":     func(tm *Tmux) error { return tm.Launch("nat-1", "/tmp", "/tmp/p.md", "3b73", config.AgentModel{}) },
			"LaunchBare": func(tm *Tmux) error { return tm.LaunchBare("nat-1", "/tmp", "session:p:s", config.AgentModel{}) },
		} {
			r := &fakeRunner{outs: map[string]string{"-V": tt.version, "new-session": "%7\n"}}
			if err := launch(NewTmuxWithRunner(r)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got := slices.Contains(r.calls[1].args, want); got != tt.want {
				t.Errorf("%s on %s: inbox carried = %v, want %v (args %v)", name, tt.version, got, tt.want, r.calls[1].args)
			}
		}
	}
	if args := LaunchArgs("nat-1", "/tmp", "/tmp/p.md", config.AgentModel{}, true); !slices.Contains(args, want) {
		t.Errorf("LaunchArgs = %v, want the inbox carried", args)
	}
}

// What tmux answers for a session's inbox: only an absolute directory set on
// the session is one.
func TestSessionInbox(t *testing.T) {
	for _, tt := range []struct {
		name string
		r    *fakeRunner
		want string
	}{
		{"set", inboxRunner("/state/agent-inbox/nat-1"), "/state/agent-inbox/nat-1"},
		{"never set", &fakeRunner{errs: map[string]error{"show-environment": &ExitError{Stderr: "unknown variable: NAT_INBOX"}}}, ""},
		{"removed", &fakeRunner{outs: map[string]string{"show-environment": "-" + inboxEnv + "\n"}}, ""},
		{"relative", inboxRunner("agent-inbox/nat-1"), ""},
	} {
		if got := NewTmuxWithRunner(tt.r).sessionInbox("nat-1"); got != tt.want {
			t.Errorf("%s: sessionInbox = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// A send the mod takes is written whole under a send-ordered name, private to
// the user, and nothing is pasted — the waiting flag still comes off.
func TestSendPromptThroughTheInbox(t *testing.T) {
	quickInbox(t, 5*time.Second)
	dir := filepath.Join(t.TempDir(), "agent-inbox", "nat-b4463d8f")
	waiting := agentApart
	waiting.waiting = true
	r := inboxRunner(dir)
	r.outs["list-panes"] = panesOutput(waiting)
	r.outs["display-message"] = waiting.id + "\t" + waiting.slice + "\n"
	got := takeOne(t, dir)
	if err := NewTmuxWithRunner(r).SendPrompt("nat-b4463d8f", "line one\nline two"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	took, ok := <-got
	if !ok {
		t.Fatal("no prompt landed in the inbox")
	}
	if took.text != "line one\nline two" || took.fileMode != 0o600 || took.dMode != 0o700 {
		t.Errorf("took %+v, want the whole text in a 0600 file in a 0700 directory", took)
	}
	if digits := strings.TrimSuffix(took.name, ".md"); digits == took.name || strings.Trim(digits, "0123456789") != "" {
		t.Errorf("file name = %q, want <unix nanoseconds>.md", took.name)
	}
	if sent(r, "set-buffer") || sent(r, "paste-buffer") || sent(r, "send-keys") {
		t.Errorf("calls = %v, want nothing pasted", r.calls)
	}
	if !sent(r, "set-option") {
		t.Errorf("calls = %v, want the waiting flag cleared", r.calls)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("inbox holds %v after the send, want nothing", entries)
	}
}

// A send no mod takes — an older Claude Code, a session from before this nat —
// is taken back out of the inbox and pasted, so it arrives once.
func TestSendPromptPastesWhatTheInboxDoesNotTake(t *testing.T) {
	quickInbox(t, 10*time.Millisecond)
	dir := filepath.Join(t.TempDir(), "nat-1")
	r := inboxRunner(dir)
	if err := NewTmuxWithRunner(r).SendPrompt("nat-1", "hello"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	if !sent(r, "set-buffer") || !sent(r, "paste-buffer") || !sent(r, "send-keys") {
		t.Errorf("calls = %v, want the prompt pasted", r.calls)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("inbox holds %v after the paste, want the file taken back", entries)
	}
}

// An inbox that cannot be written to is no reason not to send: it pastes.
func TestSendPromptPastesWhenTheInboxCannotBeWritten(t *testing.T) {
	quickInbox(t, time.Second)
	root := t.TempDir()
	notADir := filepath.Join(root, "file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	readOnly := filepath.Join(root, "ro")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
	// A directory standing where the file is renamed to.
	clash := filepath.Join(root, "clash")
	when := time.Unix(0, 1700000000000000001)
	if err := os.MkdirAll(filepath.Join(clash, "1700000000000000001.md", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldNow := inboxNow
	inboxNow = func() time.Time { return when }
	t.Cleanup(func() { inboxNow = oldNow })

	for name, dir := range map[string]string{
		"not a directory": filepath.Join(notADir, "nat-1"),
		"read-only":       readOnly,
		"rename refused":  clash,
	} {
		r := inboxRunner(dir)
		if err := NewTmuxWithRunner(r).SendPrompt("nat-1", "hello"); err != nil {
			t.Fatalf("%s: SendPrompt: %v", name, err)
		}
		if !sent(r, "paste-buffer") {
			t.Errorf("%s: calls = %v, want the prompt pasted", name, r.calls)
		}
	}
	if _, err := os.Stat(filepath.Join(clash, ".1700000000000000001.tmp")); !os.IsNotExist(err) {
		t.Errorf("temp file left behind after a refused rename: %v", err)
	}
}

// A paste that fails after the inbox gave up is still the send's error.
func TestSendPromptPasteFailureAfterTheInbox(t *testing.T) {
	quickInbox(t, time.Millisecond)
	r := inboxRunner(filepath.Join(t.TempDir(), "nat-1"))
	r.errs = map[string]error{"set-buffer": os.ErrClosed}
	if err := NewTmuxWithRunner(r).SendPrompt("nat-1", "hello"); err == nil || !strings.Contains(err.Error(), "stage the prompt") {
		t.Errorf("SendPrompt error = %v, want the paste's", err)
	}
}
