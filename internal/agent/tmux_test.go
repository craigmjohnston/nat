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

// fakeRunner records what it was asked to run and replays a canned result. A
// launch is two tmux calls, so out and err can be given per tmux subcommand;
// anything not named there gets the bare out and err.
type fakeRunner struct {
	out   string
	err   error
	outs  map[string]string
	errs  map[string]error
	calls []call
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	out, err := f.out, f.err
	if o, ok := f.outs[sub]; ok {
		out = o
	}
	if e, ok := f.errs[sub]; ok {
		err = e
	}
	return out, err
}

func TestSessionName(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"uuid with dashes", "3b738308-f654-8170-8c99-eccab4463d8f", "nat-b4463d8f"},
		{"uuid without dashes", "3b738308f65481708c99eccab4463d8f", "nat-b4463d8f"},
		{"uppercase is folded", "3B738308-F654-8170-8C99-ECCAB4463D8F", "nat-b4463d8f"},
		{"non-hex characters are skipped", "zz3b-73g83h08f654", "nat-8308f654"},
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

// The bug this replaced: page IDs from one Notion workspace share a long
// leading prefix, so a name taken off the front is the same name for every
// slice of a project — and the second launch is refused as a duplicate.
func TestSessionNameDistinguishesIDsSharingAPrefix(t *testing.T) {
	first := SessionName("3b738308-f654-8170-8c99-eccab4463d8f")
	second := SessionName("3b738308-f654-812d-ac8d-d4c80dfecb09")
	if first == second {
		t.Errorf("both slices name session %q, want a name each", first)
	}
}

func TestLiveSlices(t *testing.T) {
	r := &fakeRunner{out: strings.Join([]string{
		"\t%0\tuser-shell\t@0", // a pane of the user's own, untagged
		"3b738308…8f\t%1\tnat-b4463d8f\t@1",
		"3b738308…09\t%2\tnat-0dfecb09\t@2",
		"3b738308…8f\t%3\tnat-moved\t@3", // a second pane claiming a slice already found
		"a line tmux did not write",
		"",
	}, "\n")}

	live, err := NewTmuxWithRunner(r).LiveSlices()
	if err != nil {
		t.Fatalf("LiveSlices: %v", err)
	}

	want := map[string]string{"3b738308…8f": "nat-b4463d8f", "3b738308…09": "nat-0dfecb09"}
	if !reflect.DeepEqual(live, want) {
		t.Errorf("live = %v, want %v", live, want)
	}

	wantCall := call{name: "tmux", args: []string{
		"list-panes", "-a", "-F", "#{@nat_slice}\t#{pane_id}\t#{session_name}\t#{window_id}",
	}}
	if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], wantCall) {
		t.Errorf("calls = %+v, want exactly %+v", r.calls, wantCall)
	}
}

// A pane moved into another session is still the agent for its slice, and is
// reported under the session it has ended up in.
func TestLiveSlicesFollowsAPaneToAnotherSession(t *testing.T) {
	r := &fakeRunner{out: "3b738308…8f\t%1\tsomewhere-else\t@0\n"}
	live, err := NewTmuxWithRunner(r).LiveSlices()
	if err != nil {
		t.Fatalf("LiveSlices: %v", err)
	}
	if got := live["3b738308…8f"]; got != "somewhere-else" {
		t.Errorf("session = %q, want %q", got, "somewhere-else")
	}
}

func TestLiveSlicesWithNoServerRunning(t *testing.T) {
	r := &fakeRunner{err: &ExitError{Code: 1, Stderr: "no server running on /tmp/tmux-501/default"}}
	live, err := NewTmuxWithRunner(r).LiveSlices()
	if err != nil {
		t.Fatalf("LiveSlices: %v", err)
	}
	if len(live) != 0 {
		t.Errorf("live = %v, want empty", live)
	}
}

func TestLiveSlicesError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"other exit code", &ExitError{Code: 2, Stderr: "boom"}},
		{"binary missing", errors.New("exec: \"tmux\": executable file not found in $PATH")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live, err := NewTmuxWithRunner(&fakeRunner{err: tt.err}).LiveSlices()
			if err == nil {
				t.Fatal("LiveSlices: want error, got nil")
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
	r := &fakeRunner{outs: map[string]string{"new-session": "%7\n"}}
	id := "3b738308-f654-8170-8c99-eccab4463d8f"
	if err := NewTmuxWithRunner(r).Launch("nat-b4463d8f", "/Users/craig/Projects/x", "/tmp/prompt.md", id); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	want := []call{
		{name: "tmux", args: []string{
			"new-session", "-d",
			"-s", "nat-b4463d8f",
			"-c", "/Users/craig/Projects/x",
			"-P", "-F", "#{pane_id}",
			"sh", "-c", `claude "$(cat '/tmp/prompt.md')"`,
		}},
		{name: "tmux", args: []string{"set-option", "-p", "-t", "%7", "@nat_slice", id}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %+v, want %+v", r.calls, want)
	}
}

func TestLaunchError(t *testing.T) {
	inner := &ExitError{Code: 1, Stderr: "duplicate session: nat-b4463d8f"}
	err := NewTmuxWithRunner(&fakeRunner{err: inner}).Launch("nat-b4463d8f", "/tmp", "/tmp/prompt.md", "3b73")
	if err == nil {
		t.Fatal("Launch: want error, got nil")
	}
	if !errors.Is(err, inner) {
		t.Errorf("err = %v, want it to wrap %v", err, inner)
	}
	if !strings.Contains(err.Error(), "nat-b4463d8f") {
		t.Errorf("err = %v, want it to name the session", err)
	}
}

// An untagged pane is an agent nothing can find again, so the failure is
// reported even though the session itself came up.
func TestLaunchTagError(t *testing.T) {
	inner := &ExitError{Code: 1, Stderr: "can't find pane: %7"}
	r := &fakeRunner{
		outs: map[string]string{"new-session": "%7\n"},
		errs: map[string]error{"set-option": inner},
	}

	err := NewTmuxWithRunner(r).Launch("nat-b4463d8f", "/tmp", "/tmp/prompt.md", "3b73")
	if err == nil {
		t.Fatal("Launch: want error, got nil")
	}
	if !errors.Is(err, inner) {
		t.Errorf("err = %v, want it to wrap %v", err, inner)
	}
	if !strings.Contains(err.Error(), "3b73") {
		t.Errorf("err = %v, want it to name the slice", err)
	}
}

// panesOutput is what tmux prints for [Tmux.panes], one line per pane, in the
// order the fields are asked for.
func panesOutput(panes ...pane) string {
	var b strings.Builder
	for _, p := range panes {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", p.slice, p.id, p.session, p.window)
	}
	return b.String()
}

// The board's pane and an agent's, in windows of their own: the state before
// the agent has been shown.
var (
	boardPane  = pane{id: "%0", session: "nat-tui", window: "@0"}
	agentApart = pane{slice: "3b738308-f654-8170-8c99-eccab4463d8f",
		id: "%1", session: "nat-b4463d8f", window: "@1"}
)

func TestShowPaneJoinsTheAgentBesideTheBoard(t *testing.T) {
	r := &fakeRunner{out: panesOutput(boardPane, agentApart)}

	joined, err := NewTmuxWithRunner(r).ShowPane(agentApart.slice, boardPane.id, 65)
	if err != nil {
		t.Fatalf("ShowPane: %v", err)
	}
	if !joined {
		t.Error("joined = false, want the agent shown beside the board")
	}

	want := []call{
		{name: "tmux", args: []string{"list-panes", "-a", "-F", listPanesFormat()}},
		// The board's own session, not -g: the user's own sessions are theirs.
		{name: "tmux", args: []string{"set-option", "-t", "nat-tui", "mouse", "on"}},
		// -d leaves the keyboard on the board, so the plan stays usable.
		{name: "tmux", args: []string{"join-pane", "-h", "-d", "-l", "65%", "-s", "%1", "-t", "%0"}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %+v, want %+v", r.calls, want)
	}
}

// The configured width reaches tmux as the size of the joined pane — the
// agent's share of the window, not the board's.
func TestShowPaneHonoursTheSplitWidth(t *testing.T) {
	r := &fakeRunner{out: panesOutput(boardPane, agentApart)}

	if _, err := NewTmuxWithRunner(r).ShowPane(agentApart.slice, boardPane.id, 80); err != nil {
		t.Fatalf("ShowPane: %v", err)
	}

	join := r.calls[len(r.calls)-1].args
	if got := join[4]; got != "80%" {
		t.Errorf("size = %q, want the agent 80%% of the window", got)
	}
}

// Pressing t again: the pane is already in the board's window, so it goes back
// to a session of its own — named after its slice, as it was when it launched.
func TestShowPaneBreaksAJoinedAgentBackOut(t *testing.T) {
	joinedPane := agentApart
	joinedPane.session, joinedPane.window = boardPane.session, boardPane.window
	r := &fakeRunner{
		outs:  map[string]string{"list-panes": panesOutput(boardPane, joinedPane)},
		out:   "%9\n",
		calls: nil,
	}

	joined, err := NewTmuxWithRunner(r).ShowPane(joinedPane.slice, boardPane.id, 65)
	if err != nil {
		t.Fatalf("ShowPane: %v", err)
	}
	if joined {
		t.Error("joined = true, want the agent sent back to its own session")
	}

	session := SessionName(joinedPane.slice)
	want := []call{
		{name: "tmux", args: []string{"list-panes", "-a", "-F", listPanesFormat()}},
		{name: "tmux", args: []string{"new-session", "-d", "-s", session,
			"-P", "-F", "#{pane_id}", placeholderCommand}},
		{name: "tmux", args: []string{"join-pane", "-s", "%1", "-t", session + ":"}},
		{name: "tmux", args: []string{"kill-pane", "-t", "%9"}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %+v, want %+v", r.calls, want)
	}
}

// The agent is found by its tag rather than its session, which is what makes
// the second press work at all: a joined pane's launch session is long gone.
func TestShowPaneFindsAnAgentWhoseSessionIsGone(t *testing.T) {
	joinedPane := agentApart
	joinedPane.session, joinedPane.window = "somewhere-else", "@7"
	r := &fakeRunner{outs: map[string]string{"list-panes": panesOutput(boardPane, joinedPane)}}

	joined, err := NewTmuxWithRunner(r).ShowPane(joinedPane.slice, boardPane.id, 65)
	if err != nil {
		t.Fatalf("ShowPane: %v", err)
	}
	if !joined {
		t.Error("joined = false, want a pane in another window joined in")
	}
}

func TestShowPaneErrors(t *testing.T) {
	boom := &ExitError{Code: 1, Stderr: "boom"}
	joinedPane := agentApart
	joinedPane.session, joinedPane.window = boardPane.session, boardPane.window

	tests := []struct {
		name   string
		runner *fakeRunner
		want   string
	}{
		{
			name:   "the panes cannot be listed",
			runner: &fakeRunner{errs: map[string]error{"list-panes": &ExitError{Code: 2, Stderr: "boom"}}},
			want:   "list tmux panes",
		},
		{
			name:   "no pane is tagged for the slice",
			runner: &fakeRunner{outs: map[string]string{"list-panes": panesOutput(boardPane)}},
			want:   "no agent pane is tagged for slice",
		},
		{
			name:   "the board's own pane is not there",
			runner: &fakeRunner{outs: map[string]string{"list-panes": panesOutput(agentApart)}},
			want:   "the board's own pane %0 is not in tmux",
		},
		{
			name: "the mouse cannot be enabled",
			runner: &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane, agentApart)},
				errs: map[string]error{"set-option": boom},
			},
			want: "enable the mouse in nat-tui",
		},
		{
			name: "the pane cannot be joined",
			runner: &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane, agentApart)},
				errs: map[string]error{"join-pane": boom},
			},
			want: "join pane %1 beside the board",
		},
		{
			name: "the session to break out into cannot be made",
			runner: &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane, joinedPane)},
				errs: map[string]error{"new-session": boom},
			},
			want: "make session nat-b4463d8f for pane %1",
		},
		{
			name: "the pane cannot be moved into it",
			runner: &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane, joinedPane)},
				errs: map[string]error{"join-pane": boom},
			},
			want: "move pane %1 into nat-b4463d8f",
		},
		{
			name: "the placeholder cannot be cleared",
			runner: &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane, joinedPane)},
				errs: map[string]error{"kill-pane": boom},
			},
			want: "clear the placeholder pane",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTmuxWithRunner(tt.runner).ShowPane(agentApart.slice, boardPane.id, 65)
			if err == nil {
				t.Fatal("ShowPane: want error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to name %q", err, tt.want)
			}
		})
	}
}

// A half-made session named for a slice whose agent is still elsewhere would be
// a lie the next press believes, so a failed move takes it with it.
func TestShowPaneClearsUpAfterAFailedBreakOut(t *testing.T) {
	joinedPane := agentApart
	joinedPane.session, joinedPane.window = boardPane.session, boardPane.window
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(boardPane, joinedPane)},
		errs: map[string]error{"join-pane": &ExitError{Code: 1, Stderr: "boom"}},
	}

	if _, err := NewTmuxWithRunner(r).ShowPane(joinedPane.slice, boardPane.id, 65); err == nil {
		t.Fatal("ShowPane: want error, got nil")
	}

	last := r.calls[len(r.calls)-1]
	want := call{name: "tmux", args: []string{"kill-session", "-t", SessionName(joinedPane.slice)}}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("last call = %+v, want %+v", last, want)
	}
}

func TestHostPane(t *testing.T) {
	t.Setenv(PaneEnv, "%3")
	if got := HostPane(); got != "%3" {
		t.Errorf("HostPane() = %q, want %q", got, "%3")
	}
	// Outside tmux there is no window to show an agent in, which the caller
	// reads off the empty answer.
	t.Setenv(PaneEnv, "")
	if got := HostPane(); got != "" {
		t.Errorf("HostPane() = %q, want it empty outside tmux", got)
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

// The TUI's own session shares the prefix agent sessions use, so a session list
// still reads as one family, and `-A` is what lets a second launch attach.
func TestHostArgs(t *testing.T) {
	got := HostArgs("/usr/local/bin/nat")
	want := []string{"new-session", "-A", "-s", "nat-tui", "/usr/local/bin/nat"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
	if !strings.HasPrefix(TUISession, SessionPrefix) {
		t.Errorf("TUISession = %q, want the %q prefix", TUISession, SessionPrefix)
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

// The prefix is what tells our sessions apart from the user's own in a session
// list, so the name still has to carry it.
func TestSessionNameCarriesThePrefix(t *testing.T) {
	name := SessionName("3b738308f65481708c99eccab4463d8f")
	if !strings.HasPrefix(name, SessionPrefix) {
		t.Errorf("%q does not start with %q", name, SessionPrefix)
	}
}

// The pane tag is the identity of a running agent, so the option Launch sets
// and the one LiveSlices reads have to be the same one.
func TestLaunchTagsWhatLiveSlicesReads(t *testing.T) {
	id := "3b738308-f654-8170-8c99-eccab4463d8f"
	session := SessionName(id)

	launch := &fakeRunner{outs: map[string]string{"new-session": "%7"}}
	if err := NewTmuxWithRunner(launch).Launch(session, "/tmp", "/tmp/prompt.md", id); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	tag := launch.calls[1].args
	option, value := tag[len(tag)-2], tag[len(tag)-1]

	// tmux reports the option back where the format asked for it, which is the
	// first field of the line.
	format := listPanesFormat()
	if !strings.HasPrefix(format, "#{"+option+"}\t") {
		t.Fatalf("format %q does not read back %q", format, option)
	}

	read := &fakeRunner{out: fmt.Sprintf("%s\t%%1\t%s\t@0\n", value, session)}
	live, err := NewTmuxWithRunner(read).LiveSlices()
	if err != nil {
		t.Fatalf("LiveSlices: %v", err)
	}
	if got := live[id]; got != session {
		t.Errorf("live[%q] = %q, want %q", id, got, session)
	}
}
