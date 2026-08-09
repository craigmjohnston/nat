package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// call is one invocation recorded by fakeRunner.
type call struct {
	name string
	args []string
}

// fakeRunner records what it was asked to run and replays a canned result.
type fakeRunner struct {
	out   string
	err   error
	calls []call
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	return f.out, f.err
}

func TestSessionName(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"uuid with dashes", "3b738308-f654-8170-8c99-eccab4463d8f", "nat-3b738308"},
		{"uuid without dashes", "3b738308f65481708c99eccab4463d8f", "nat-3b738308"},
		{"uppercase is folded", "3B738308-F654-8170-8C99-ECCAB4463D8F", "nat-3b738308"},
		{"non-hex characters are skipped", "zz3b-73g83h08f654", "nat-3b738308"},
		{"short id is used as-is", "3b73", "nat-3b73"},
		{"empty id", "", "nat-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SessionName(tt.id); got != tt.want {
				t.Errorf("SessionName(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestLiveSessions(t *testing.T) {
	r := &fakeRunner{out: "nat-3b738308\nother-session\nnat-aabbccdd\n"}
	live, err := NewTmuxWithRunner(r).LiveSessions()
	if err != nil {
		t.Fatalf("LiveSessions: %v", err)
	}

	want := map[string]bool{"nat-3b738308": true, "nat-aabbccdd": true}
	if !reflect.DeepEqual(live, want) {
		t.Errorf("live = %v, want %v", live, want)
	}

	wantCall := call{name: "tmux", args: []string{"list-sessions", "-F", "#{session_name}"}}
	if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], wantCall) {
		t.Errorf("calls = %+v, want exactly %+v", r.calls, wantCall)
	}
}

func TestLiveSessionsWithNoServerRunning(t *testing.T) {
	r := &fakeRunner{err: &ExitError{Code: 1, Stderr: "no server running on /tmp/tmux-501/default"}}
	live, err := NewTmuxWithRunner(r).LiveSessions()
	if err != nil {
		t.Fatalf("LiveSessions: %v", err)
	}
	if len(live) != 0 {
		t.Errorf("live = %v, want empty", live)
	}
}

func TestLiveSessionsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"other exit code", &ExitError{Code: 2, Stderr: "boom"}},
		{"binary missing", errors.New("exec: \"tmux\": executable file not found in $PATH")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live, err := NewTmuxWithRunner(&fakeRunner{err: tt.err}).LiveSessions()
			if err == nil {
				t.Fatal("LiveSessions: want error, got nil")
			}
			if !errors.Is(err, tt.err) {
				t.Errorf("err = %v, want it to wrap %v", err, tt.err)
			}
			if live != nil {
				t.Errorf("live = %v, want nil", live)
			}
		})
	}
}

func TestLaunch(t *testing.T) {
	r := &fakeRunner{}
	if err := NewTmuxWithRunner(r).Launch("nat-3b738308", "/Users/craig/Projects/x", "/tmp/prompt.md"); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	want := call{name: "tmux", args: []string{
		"new-session", "-d",
		"-s", "nat-3b738308",
		"-c", "/Users/craig/Projects/x",
		"sh", "-c", `claude "$(cat '/tmp/prompt.md')"`,
	}}
	if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], want) {
		t.Errorf("calls = %+v, want exactly %+v", r.calls, want)
	}
}

func TestLaunchError(t *testing.T) {
	inner := &ExitError{Code: 1, Stderr: "duplicate session: nat-3b738308"}
	err := NewTmuxWithRunner(&fakeRunner{err: inner}).Launch("nat-3b738308", "/tmp", "/tmp/prompt.md")
	if err == nil {
		t.Fatal("Launch: want error, got nil")
	}
	if !errors.Is(err, inner) {
		t.Errorf("err = %v, want it to wrap %v", err, inner)
	}
	if !strings.Contains(err.Error(), "nat-3b738308") {
		t.Errorf("err = %v, want it to name the session", err)
	}
}

func TestLaunchArgsQuotesThePromptPath(t *testing.T) {
	args := LaunchArgs("nat-1", "/tmp", "/tmp/craig's prompt.md")
	got := args[len(args)-1]
	want := `claude "$(cat '/tmp/craig'\''s prompt.md')"`
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

func TestAttachCmd(t *testing.T) {
	cmd := AttachCmd("nat-3b738308")
	want := []string{"tmux", "attach-session", "-t", "nat-3b738308"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
}

func TestTmuxAttachCmd(t *testing.T) {
	cmd := NewTmux().AttachCmd("nat-3b738308")
	want := []string{"tmux", "attach-session", "-t", "nat-3b738308"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
}

func TestWritePromptFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	path, err := WritePromptFile("nat-3b738308", "do the work")
	if err != nil {
		t.Fatalf("WritePromptFile: %v", err)
	}
	if got := filepath.Base(path); got != "nat-3b738308.md" {
		t.Errorf("file = %q, want it named after the session", got)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(body) != "do the work" {
		t.Errorf("contents = %q, want the prompt", body)
	}

	// The prompt is nobody else's business, and neither is the directory it
	// lands in: the agent obeys whatever it reads there.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %v, want 0600", got)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dir.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %v, want 0700", got)
	}
}

func TestWritePromptFileGivesEachLaunchItsOwnDirectory(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	first, err := WritePromptFile("nat-3b738308", "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := WritePromptFile("nat-3b738308", "two")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("both launches wrote %q, want a fresh directory each time", first)
	}
}

func TestWritePromptFileError(t *testing.T) {
	t.Run("no temp dir to write in", func(t *testing.T) {
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-there"))

		if _, err := WritePromptFile("nat-1", "prompt"); err == nil {
			t.Fatal("WritePromptFile: want error, got nil")
		} else if !strings.Contains(err.Error(), "create prompt dir") {
			t.Errorf("err = %v, want it to name the failed step", err)
		}
	})

	t.Run("directory cannot be written to", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}

		if _, err := writePromptInto(dir, "nat-1", "prompt"); err == nil {
			t.Fatal("writePromptInto: want error, got nil")
		} else if !strings.Contains(err.Error(), "write prompt file") {
			t.Errorf("err = %v, want it to name the failed step", err)
		}
	})
}

func TestExitErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *ExitError
		want string
	}{
		{"with stderr", &ExitError{Code: 1, Stderr: "  no server running\n"}, "exit status 1: no server running"},
		{"without stderr", &ExitError{Code: 2, Stderr: "  \n"}, "exit status 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecRunner(t *testing.T) {
	out, err := ExecRunner{}.Run("sh", "-c", "printf hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hello" {
		t.Errorf("out = %q, want %q", out, "hello")
	}
}

func TestExecRunnerNonZeroExit(t *testing.T) {
	out, err := ExecRunner{}.Run("sh", "-c", "printf partial; printf boom >&2; exit 3")

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exitErr.Code != 3 {
		t.Errorf("code = %d, want 3", exitErr.Code)
	}
	if exitErr.Stderr != "boom" {
		t.Errorf("stderr = %q, want %q", exitErr.Stderr, "boom")
	}
	if out != "partial" {
		t.Errorf("out = %q, want %q", out, "partial")
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Run("notion-agent-tracker-no-such-binary")

	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want a non-exit error", err)
	}
	var notFound *exec.Error
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want *exec.Error", err)
	}
}

func TestNewTmuxUsesExecRunner(t *testing.T) {
	if _, ok := NewTmux().runner.(ExecRunner); !ok {
		t.Errorf("runner = %T, want ExecRunner", NewTmux().runner)
	}
}

// The prefix is what tells our sessions apart from the user's own, so the two
// places that depend on it have to agree.
func TestSessionNameCarriesThePrefix(t *testing.T) {
	name := SessionName("3b738308f65481708c99eccab4463d8f")
	if !strings.HasPrefix(name, SessionPrefix) {
		t.Fatalf("%q does not start with %q", name, SessionPrefix)
	}
	r := &fakeRunner{out: fmt.Sprintf("%s\n", name)}
	live, err := NewTmuxWithRunner(r).LiveSessions()
	if err != nil {
		t.Fatalf("LiveSessions: %v", err)
	}
	if !live[name] {
		t.Errorf("live = %v, want it to contain %q", live, name)
	}
}
