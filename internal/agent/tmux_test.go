package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
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
		// Hex-filtered the sentinel would come out as "nat-a", which a short
		// or surprising slice ID could collide with.
		{"the planning agent's sentinel", PlanSentinel, PlanSession},
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
		"\t%0\tuser-shell\t@0\t0\t80\t", // a pane of the user's own, untagged
		"3b738308…8f\t%1\tnat-b4463d8f\t@1\t0\t80\tb4463d8f",
		"3b738308…09\t%2\tnat-0dfecb09\t@2\t0\t80\t0dfecb09",
		"3b738308…8f\t%3\tnat-moved\t@3\t0\t80\tb4463d8f", // a second pane claiming a slice already found
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
		"list-panes", "-a", "-F",
		"#{@nat_slice}\t#{pane_id}\t#{session_name}\t#{window_id}\t#{pane_index}\t#{pane_width}\t#{@nat_label}",
	}}
	if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], wantCall) {
		t.Errorf("calls = %+v, want exactly %+v", r.calls, wantCall)
	}
}

// A pane moved into another session is still the agent for its slice, and is
// reported under the session it has ended up in.
func TestLiveSlicesFollowsAPaneToAnotherSession(t *testing.T) {
	r := &fakeRunner{out: "3b738308…8f\t%1\tsomewhere-else\t@0\t0\t80\tb4463d8f\n"}
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
		{name: "tmux", args: append([]string{
			"new-session", "-d",
			"-s", "nat-b4463d8f",
			"-c", "/Users/craig/Projects/x",
			"-P", "-F", "#{pane_id}",
			"sh", "-c", `claude "$(cat '/tmp/prompt.md')"`,
			// Chained onto the creation, so the session never shows a status
			// bar — not even to someone attaching straight away.
			";", "set-option", "-t", "nat-b4463d8f", "status", "off",
			// The mouse is tmux's in an agent's session, so the hyperlink
			// click binding fires there.
			";", "set-option", "-t", "nat-b4463d8f", "mouse", "on",
			// Server options, chained on too: shift+enter reaches the agent,
			// and the URLs it prints stay clickable links.
			";", "set-option", "-s", "extended-keys", "on",
			";", "set-option", "-s", "-a", "terminal-features", "*:extkeys:hyperlinks",
		}, clickBindingArgs()...)},
		// The tag the agent is found by, and — chained onto it, so a pane is
		// never one without the other — the label the board's bar shows it by.
		{name: "tmux", args: []string{"set-option", "-p", "-t", "%7", "@nat_slice", id,
			";", "set-option", "-p", "-t", "%7", "@nat_label", "b4463d8f"}},
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

// clickBindingArgs is the command chain every launch ends with: the two mouse
// bindings that open the hyperlink under a click. The opener is chosen here
// independently of the code under test, so a wrong platform pick fails rather
// than agreeing with itself.
func clickBindingArgs() []string {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	open := "run-shell -b '" + opener + " #{q:mouse_hyperlink}'"
	return []string{
		";", "bind-key", "-T", "root", "MouseDown1Pane",
		"if-shell", "-F", "-t", "=", "#{!=:#{mouse_hyperlink},}",
		// On a link: open it, and still do everything a click always did.
		open + " ; select-pane -t = ; send-keys -M",
		"select-pane -t = ; send-keys -M",
		";", "bind-key", "-T", "root", "C-MouseDown1Pane",
		"if-shell", "-F", "-t", "=", "#{!=:#{mouse_hyperlink},}",
		// On a link: open it instead of tmux's stock pane swap, which is
		// never what a ctrl-click on a URL wants.
		open,
		"swap-pane -s @",
	}
}

func TestURLOpenerPerPlatform(t *testing.T) {
	if got := urlOpenerFor("darwin"); got != "open" {
		t.Errorf(`urlOpenerFor("darwin") = %q, want "open"`, got)
	}
	if got := urlOpenerFor("linux"); got != "xdg-open" {
		t.Errorf(`urlOpenerFor("linux") = %q, want "xdg-open"`, got)
	}
	if got, want := urlOpener(), urlOpenerFor(runtime.GOOS); got != want {
		t.Errorf("urlOpener() = %q, want the pick for this platform, %q", got, want)
	}
}

func TestHyperlinkClickArgs(t *testing.T) {
	got := hyperlinkClickArgs()
	if !reflect.DeepEqual(got, clickBindingArgs()) {
		t.Errorf("hyperlinkClickArgs = %q, want %q", got, clickBindingArgs())
	}
	if got[0] != ";" {
		t.Error("the bindings must be chained, not a command of their own")
	}
}

// panesOutput is what tmux prints for [Tmux.panes], one line per pane, in the
// order the fields are asked for.
func panesOutput(panes ...pane) string {
	var b strings.Builder
	for _, p := range panes {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			p.slice, p.id, p.session, p.window, p.index, p.width, p.label)
	}
	return b.String()
}

// The board's pane and an agent's, in windows of their own: the state before
// the agent has been shown.
var (
	boardPane  = pane{id: "%0", session: "nat-tui", window: "@0", index: "0", width: 80}
	agentApart = pane{slice: "3b738308-f654-8170-8c99-eccab4463d8f",
		id: "%1", session: "nat-b4463d8f", window: "@1", index: "0", width: 80,
		label: PaneLabel("3b738308-f654-8170-8c99-eccab4463d8f")}
)

// refreshCalls is the tmux argv sequence that redraws the bar of host's session
// for the panes of its window — the pair of calls every layout change ends
// with.
func refreshCalls(host pane, window ...pane) []call {
	return []call{
		{name: "tmux", args: []string{"list-panes", "-a", "-F", listPanesFormat()}},
		{name: "tmux", args: []string{"set-option", "-t", host.session, statusFormatOption,
			buildStatusFormat(sectionsFor(window))}},
	}
}

// breakOutCalls is the tmux argv sequence that sends one pane back to a
// session of its own, with placeholder as the pane the new session came up on.
func breakOutCalls(paneID, session, placeholder string) []call {
	return []call{
		{name: "tmux", args: []string{"new-session", "-d", "-s", session,
			"-P", "-F", "#{pane_id}", placeholderCommand,
			";", "set-option", "-t", session, "status", "off",
			";", "set-option", "-t", session, "mouse", "on"}},
		{name: "tmux", args: []string{"join-pane", "-s", paneID, "-t", session + ":"}},
		{name: "tmux", args: []string{"kill-pane", "-t", placeholder}},
	}
}

// Starting up after a run that died: the agents it left in the TUI's session,
// and any in the window this board is coming up in, are re-homed rather than
// left somewhere a window close would kill them.
func TestReclaimStraysReHomesThePanesAnEarlierRunLeft(t *testing.T) {
	stray := agentApart
	stray.session, stray.window = TUISession, "@4"
	inWindow := agentApart
	inWindow.slice, inWindow.id = "3b738308-f654-812d-ac8d-d4c80dfecb09", "%2"
	inWindow.session, inWindow.window = "someone-elses", boardPane.window
	r := &fakeRunner{
		// agentApart itself is in a session of its own already: a running
		// agent nobody has shown, which is exactly where it should be.
		outs: map[string]string{"list-panes": panesOutput(boardPane, stray, inWindow, agentApart)},
		out:  "%9\n",
	}

	moved, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id)
	if err != nil {
		t.Fatalf("ReclaimStrays: %v", err)
	}
	if moved != 2 {
		t.Errorf("moved = %d, want both strays re-homed", moved)
	}

	want := []call{{name: "tmux", args: []string{"list-panes", "-a", "-F", listPanesFormat()}}}
	want = append(want, breakOutCalls(stray.id, SessionName(stray.slice), "%9")...)
	want = append(want, breakOutCalls(inWindow.id, SessionName(inWindow.slice), "%9")...)
	want = append(want, refreshCalls(boardPane, boardPane, inWindow)...)
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %+v, want %+v", r.calls, want)
	}
}

// The board is coming up somewhere tmux does not know about, so there is no
// window of its own to sweep — but the TUI session a crash left behind is
// still there to be swept.
func TestReclaimStraysWithoutABoardPane(t *testing.T) {
	stray := agentApart
	stray.session = TUISession
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(stray, agentApart)},
		out:  "%9\n",
	}

	moved, err := NewTmuxWithRunner(r).ReclaimStrays("")
	if err != nil {
		t.Fatalf("ReclaimStrays: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved = %d, want the stray in %s re-homed", moved, TUISession)
	}
}

func TestReclaimStraysReportsAFailedListing(t *testing.T) {
	boom := &ExitError{Code: 2, Stderr: "boom"}
	r := &fakeRunner{errs: map[string]error{"list-panes": boom}}

	if _, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}

// Each way the break-out itself can fail is reported naming the pane and the
// session it was headed for, so the log says which agent was left where.
func TestReclaimStraysReportsAFailedBreakOut(t *testing.T) {
	boom := &ExitError{Code: 1, Stderr: "boom"}
	stray := agentApart
	stray.session, stray.window = TUISession, "@4"

	tests := []struct {
		name string
		sub  string
		want string
	}{
		{"the session to break out into cannot be made", "new-session",
			"make session nat-b4463d8f for pane %1"},
		{"the pane cannot be moved into it", "join-pane",
			"move pane %1 into nat-b4463d8f"},
		{"the placeholder cannot be cleared", "kill-pane",
			"clear the placeholder pane"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane, stray)},
				out:  "%9\n",
				errs: map[string]error{tt.sub: boom},
			}

			_, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id)
			if err == nil {
				t.Fatal("ReclaimStrays: want error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to name %q", err, tt.want)
			}
		})
	}
}

// A half-made session named for a slice whose agent is still elsewhere would be
// a lie the next startup believes, so a failed move takes it with it.
func TestReclaimStraysClearsUpAfterAFailedBreakOut(t *testing.T) {
	stray := agentApart
	stray.session, stray.window = TUISession, "@4"
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(boardPane, stray)},
		out:  "%9\n",
		errs: map[string]error{"join-pane": &ExitError{Code: 1, Stderr: "boom"}},
	}

	if _, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id); err == nil {
		t.Fatal("ReclaimStrays: want error, got nil")
	}

	last := r.calls[len(r.calls)-1]
	want := call{name: "tmux", args: []string{"kill-session", "-t", SessionName(stray.slice)}}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("last call = %+v, want %+v", last, want)
	}
}

// One pane that will not move must not strand the ones behind it: each of them
// left joined is another agent a closing window would kill.
func TestReclaimStraysCarriesOnPastAFailure(t *testing.T) {
	first, second := agentApart, agentApart
	first.session, first.window = TUISession, "@4"
	second.slice, second.id = "3b738308-f654-812d-ac8d-d4c80dfecb09", "%2"
	second.session, second.window = TUISession, "@5"
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(boardPane, first, second)},
		errs: map[string]error{"new-session": &ExitError{Code: 1, Stderr: "duplicate session"}},
	}

	moved, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id)
	if moved != 0 {
		t.Errorf("moved = %d, want nothing moved", moved)
	}
	if err == nil {
		t.Fatal("ReclaimStrays: want the failures reported, got nil")
	}
	for _, sess := range []string{SessionName(first.slice), SessionName(second.slice)} {
		if !strings.Contains(err.Error(), sess) {
			t.Errorf("err = %v, want it to name %s", err, sess)
		}
	}
}

// Nothing to reclaim is the ordinary startup, and it costs one tmux call.
func TestReclaimStraysWithNothingToDo(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"list-panes": panesOutput(boardPane)}}

	moved, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id)
	if err != nil || moved != 0 {
		t.Fatalf("ReclaimStrays = %d, %v, want 0, nil", moved, err)
	}
	if len(r.calls) != 1 {
		t.Errorf("calls = %+v, want only the pane list", r.calls)
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
	// The command is the argument after "sh -c", wherever the argv puts it.
	sh := slices.Index(args, "sh")
	if sh < 0 || sh+2 >= len(args) {
		t.Fatalf("args = %v, want an sh -c command in there", args)
	}
	got := args[sh+2]
	want := `claude "$(cat '/tmp/craig'\''s prompt.md')"`
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

// The TUI's own session shares the prefix agent sessions use, so a session list
// still reads as one family, and `-A` is what lets a second launch attach.
func TestHostArgs(t *testing.T) {
	got := HostArgs("/usr/local/bin/nat")
	// No mouse-on for the board's session: it gets its mouse at join time, the
	// way it always has.
	want := append([]string{"new-session", "-A", "-s", "nat-tui", "/usr/local/bin/nat"},
		statusBarArgs("nat-tui")...)
	want = append(want, []string{
		";", "set-option", "-s", "extended-keys", "on",
		";", "set-option", "-s", "-a", "terminal-features", "*:extkeys:hyperlinks"}...)
	want = append(want, clickBindingArgs()...)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
	if !strings.HasPrefix(TUISession, SessionPrefix) {
		t.Errorf("TUISession = %q, want the %q prefix", TUISession, SessionPrefix)
	}
}

// An agent's own session hides the tmux status bar as part of the one command
// that makes it, so it is never up even for a moment. The inherited case needs
// no test of its own: running inside the user's tmux makes no session at all,
// and the argv above are the only places one is made. The board's session is
// the exception, and gets a bar of nat's own — see the status bar tests below.
func TestAgentSessionsChainStatusOff(t *testing.T) {
	launch := LaunchArgs("nat-1", "/tmp", "/tmp/prompt.md")
	chained := append(statusOffArgs("nat-1"), mouseOnArgs("nat-1")...)
	chained = append(chained, inputFeatureArgs()...)
	chained = append(chained, hyperlinkClickArgs()...)
	if !reflect.DeepEqual(launch[len(launch)-len(chained):], chained) {
		t.Errorf("LaunchArgs = %v, want it to end with %v", launch, chained)
	}
	for _, args := range [][]string{statusOffArgs("nat-1"), mouseOnArgs("nat-1")} {
		if args[0] != ";" {
			t.Errorf("%v must be chained, not a command of its own", args)
		}
	}
}

// Shift+enter and clickable links ride on tmux server options: extended-keys
// forwards the modified key to an agent that asks for it, and the
// terminal-features entry both asks the outer terminal for extended keys
// (extkeys) and lets OSC 8 hyperlinks pass through (hyperlinks). They are
// chained onto every session nat itself creates; inside the user's own tmux
// the first agent launch is what sets them.
func TestSessionsNatCreatesEnableExtendedKeysAndHyperlinks(t *testing.T) {
	suffix := inputFeatureArgs()
	if suffix[0] != ";" {
		t.Error("the feature options must be chained, not a command of their own")
	}
	joined := strings.Join(suffix, " ")
	for _, want := range []string{
		"; set-option -s extended-keys on",
		"; set-option -s -a terminal-features *:extkeys:hyperlinks",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("inputFeatureArgs = %q, want it to contain %q", joined, want)
		}
	}
	suffix = append(suffix, hyperlinkClickArgs()...)
	for _, args := range [][]string{
		LaunchArgs("nat-1", "/tmp", "/tmp/prompt.md"),
		HostArgs("/usr/local/bin/nat"),
	} {
		if !reflect.DeepEqual(args[len(args)-len(suffix):], suffix) {
			t.Errorf("args = %v, want them to end with %v", args, suffix)
		}
	}
}

// The board's session gets the bar back, themed as nat's own, and every one of
// the options is chained onto the new-session rather than run on its own.
func TestStatusBarArgsSetTheBarOnTheBoardsSession(t *testing.T) {
	args := statusBarArgs("nat-tui")
	for i := 0; i < len(args); i += 6 {
		if args[i] != ";" {
			t.Fatalf("args = %v, want every option chained with %q", args, ";")
		}
		if got := args[i+1 : i+4]; !reflect.DeepEqual(got, []string{"set-option", "-t", "nat-tui"}) {
			t.Errorf("args = %v, want each option set on the named session", got)
		}
	}

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"; set-option -t nat-tui status on",
		// Under the frames whose width its sections are, so it reads as their
		// footer rather than a row of its own.
		"; set-option -t nat-tui status-position bottom",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("statusBarArgs = %q, want it to contain %q", joined, want)
		}
	}
}

// Both borders in the one neutral colour, so the split reads as a seam and the
// stock green highlight on the focused edge is gone.
func TestStatusBarArgsNeutraliseThePaneBorders(t *testing.T) {
	args := statusBarArgs("nat-tui")
	styles := map[string]string{}
	for i, a := range args {
		if a == "pane-border-style" || a == "pane-active-border-style" {
			styles[a] = args[i+1]
		}
	}
	if len(styles) != 2 {
		t.Fatalf("statusBarArgs = %v, want both border styles set", args)
	}
	if styles["pane-border-style"] != styles["pane-active-border-style"] {
		t.Errorf("border styles = %v, want the active one to match the inactive one", styles)
	}
	if !strings.Contains(styles["pane-border-style"], paneBorderFG) {
		t.Errorf("border style = %q, want nat's neutral %q", styles["pane-border-style"], paneBorderFG)
	}
}

// One section per pane, each exactly as wide as its pane, with the one column
// of pane border between them: the sections total the window width, so the bar
// reads as a footer to the frames.
func TestBuildStatusFormatSizesEachSectionToItsPane(t *testing.T) {
	format := buildStatusFormat([]statusSection{
		{index: "0", content: paneTitleContent, width: 40},
		{index: "1", content: literalContent("b4463d8f"), width: 39},
	})
	for _, want := range []string{"#{=40;p40:", "#{=39;p39:", paneTitleContent, "b4463d8f", statusSeparator} {
		if !strings.Contains(format, want) {
			t.Errorf("format = %q, want it to contain %q", format, want)
		}
	}
	// tmux resolves the focus itself at redraw: each section asks whether the
	// window's focused pane is the one it was built for, so moving the focus
	// needs no command sent.
	for _, want := range []string{
		"#{?#{==:#{pane_index},0}," + activeSectionStyle + "," + inactiveSectionStyle + "}",
		"#{?#{==:#{pane_index},1}," + activeSectionStyle + "," + inactiveSectionStyle + "}",
	} {
		if !strings.Contains(format, want) {
			t.Errorf("format = %q, want it to contain %q", format, want)
		}
	}
	// A comma inside the branches of #{?...} is where tmux splits them, so the
	// styles have to be spelled with spaces.
	for _, style := range []string{statusBarFG, statusBarBG, statusBarAccent, statusBarOnFill} {
		if strings.Contains(format, style+",") {
			t.Errorf("format = %q, want no comma after the style %q", format, style)
		}
	}
	// A pane ID would not survive: tmux passes the format through strftime,
	// which eats the % an ID begins with.
	if strings.Contains(format, "pane_id") {
		t.Errorf("format = %q, want the focus decided on the pane index", format)
	}
}

// The board alone: one section, the whole window wide, and in the accent
// without asking — the sole pane of a window is the focused one.
func TestBuildStatusFormatDrawsTheBoardAloneAsOneSection(t *testing.T) {
	format := buildStatusFormat([]statusSection{{index: "0", content: paneTitleContent, width: 80}})
	if want := activeSectionStyle + "#{=80;p80:#{l: }#{pane_title}}#[default]"; format != want {
		t.Errorf("format = %q, want %q", format, want)
	}
}

// The bar a board session comes up with, before there is a window to measure:
// one section, wider than any terminal, which tmux clips at the window's edge.
func TestInitialStatusFormatFillsTheRow(t *testing.T) {
	format := initialStatusFormat()
	if want := fmt.Sprintf("#{=%d;p%d:", initialStatusWidth, initialStatusWidth); !strings.Contains(format, want) {
		t.Errorf("format = %q, want it to contain %q", format, want)
	}
	if !strings.Contains(format, paneTitleContent) {
		t.Errorf("format = %q, want the board's section drawn from its pane title", format)
	}
}

// What each pane's section draws: its own label, the bare word for a pane an
// earlier run tagged before there were labels, and its own title — where nat
// writes its messages — for the untagged pane nat itself is drawing in.
func TestPaneStatusContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		pane pane
		want string
	}{
		{"a labelled agent", pane{slice: "3b73", label: "b4463d8f"}, literalContent("b4463d8f")},
		{"an agent from before the labels", pane{slice: "3b73"}, literalContent(agentLabel)},
		{"the board's own pane", pane{}, paneTitleContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.pane.statusContent(); got != tc.want {
				t.Errorf("statusContent() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The bar is pushed onto the board's own session, built from the panes of the
// window its pane is in — the panes of other windows are another bar's.
func TestRefreshStatusBarBuildsTheBarFromTheWindowsPanes(t *testing.T) {
	joined := agentApart
	joined.session, joined.window, joined.id = boardPane.session, boardPane.window, "%1"
	joined.index, joined.width = "1", 39
	board := boardPane
	board.width = 40
	elsewhere := agentApart
	elsewhere.id, elsewhere.window, elsewhere.label = "%3", "@9", "eeeeeeee"
	r := &fakeRunner{outs: map[string]string{"list-panes": panesOutput(board, joined, elsewhere)}}

	if err := NewTmuxWithRunner(r).RefreshStatusBar(board.id); err != nil {
		t.Fatalf("RefreshStatusBar: %v", err)
	}

	want := refreshCalls(board, board, joined)
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %+v, want %+v", r.calls, want)
	}
	format := r.calls[1].args[4]
	if !strings.Contains(format, "#{=40;p40:") || !strings.Contains(format, "#{=39;p39:") {
		t.Errorf("format = %q, want a section the width of each pane", format)
	}
	if strings.Contains(format, elsewhere.label) {
		t.Errorf("format = %q, want nothing from another window in it", format)
	}
}

// The board is not a pane on this server, so there is no bar of ours under it
// to redraw.
func TestRefreshStatusBarWithoutABoardPane(t *testing.T) {
	r := &fakeRunner{outs: map[string]string{"list-panes": panesOutput(agentApart)}}

	if err := NewTmuxWithRunner(r).RefreshStatusBar("%404"); err != nil {
		t.Fatalf("RefreshStatusBar: %v", err)
	}
	if len(r.calls) != 1 {
		t.Errorf("calls = %+v, want only the pane list", r.calls)
	}
}

func TestRefreshStatusBarErrors(t *testing.T) {
	boom := &ExitError{Code: 2, Stderr: "boom"}
	for _, tc := range []struct {
		name   string
		runner *fakeRunner
		want   string
	}{
		{
			name:   "the panes cannot be listed",
			runner: &fakeRunner{errs: map[string]error{"list-panes": boom}},
			want:   "list tmux panes",
		},
		{
			name: "the option cannot be set",
			runner: &fakeRunner{
				outs: map[string]string{"list-panes": panesOutput(boardPane)},
				errs: map[string]error{"set-option": boom},
			},
			want: "redraw the status bar of nat-tui",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := NewTmuxWithRunner(tc.runner).RefreshStatusBar(boardPane.id)
			if err == nil {
				t.Fatal("RefreshStatusBar: want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to name %q", err, tc.want)
			}
		})
	}
}

// A bar that will not redraw is chrome the re-homing can do without: the panes
// have already gone where they were asked to go, and saying how many is what
// the caller is waiting on.
func TestReclaimStraysSurvivesABarThatWillNotRedraw(t *testing.T) {
	stray := agentApart
	stray.session, stray.window = TUISession, "@4"
	r := &fakeRunner{
		outs: map[string]string{"list-panes": panesOutput(boardPane, stray)},
		out:  "%9\n",
		// The break-out's own set-options are chained into the new-session
		// argv, so the only bare one in this path is the redraw's.
		errs: map[string]error{"set-option": &ExitError{Code: 2, Stderr: "boom"}},
	}

	moved, err := NewTmuxWithRunner(r).ReclaimStrays(boardPane.id)
	if err != nil {
		t.Fatalf("ReclaimStrays: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved = %d, want the stray reported despite the bar", moved)
	}
}

// The label is the session name a user attaches the agent by, without the
// prefix every pane on nat's own bar would carry.
func TestPaneLabel(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"3b738308-f654-8170-8c99-eccab4463d8f", "b4463d8f"},
		{PlanSentinel, PlanSentinel},
	} {
		if got := PaneLabel(tc.id); got != tc.want {
			t.Errorf("PaneLabel(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// wantAttachArgs is the argv both attaches build: the client features are a
// top-level flag, so they precede the command.
var wantAttachArgs = []string{
	"tmux", "-T", "256,RGB,extkeys,focus", "attach-session", "-t", "nat-3b738308",
}

// envValues is every value the environment of cmd gives name, so a test can
// assert both that a variable is gone and that another appears exactly once.
func envValues(cmd *exec.Cmd, name string) []string {
	var out []string
	for _, entry := range cmd.Env {
		if got, value, found := strings.Cut(entry, "="); found && got == name {
			out = append(out, value)
		}
	}
	return out
}

func TestAttachCmd(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	t.Setenv("TMUX_PANE", "%7")
	t.Setenv("TERM", "xterm-ghostty")

	cmd := AttachCmd("nat-3b738308")

	if !reflect.DeepEqual(cmd.Args, wantAttachArgs) {
		t.Errorf("args = %v, want %v", cmd.Args, wantAttachArgs)
	}
	for _, name := range []string{"TMUX", "TMUX_PANE"} {
		if got := envValues(cmd, name); got != nil {
			t.Errorf("%s = %v, want it scrubbed", name, got)
		}
	}
	if got := envValues(cmd, "TERM"); !reflect.DeepEqual(got, []string{"xterm-ghostty"}) {
		t.Errorf("TERM = %v, want the caller's kept", got)
	}
}

func TestTmuxAttachCmd(t *testing.T) {
	cmd := NewTmux().AttachCmd("nat-3b738308")
	if !reflect.DeepEqual(cmd.Args, wantAttachArgs) {
		t.Errorf("args = %v, want %v", cmd.Args, wantAttachArgs)
	}
}

func TestAttachClientCmd(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	t.Setenv("TMUX_PANE", "%7")
	t.Setenv("TERM", "xterm-ghostty")

	cmd := AttachClientCmd("nat-3b738308")

	if !reflect.DeepEqual(cmd.Args, wantAttachArgs) {
		t.Errorf("args = %v, want %v", cmd.Args, wantAttachArgs)
	}
	for _, name := range []string{"TMUX", "TMUX_PANE"} {
		if got := envValues(cmd, name); got != nil {
			t.Errorf("%s = %v, want it scrubbed", name, got)
		}
	}
	if got := envValues(cmd, "TERM"); !reflect.DeepEqual(got, []string{"xterm-256color"}) {
		t.Errorf("TERM = %v, want exactly one xterm-256color", got)
	}
}

func TestTmuxAttachClientCmd(t *testing.T) {
	cmd := NewTmux().AttachClientCmd("nat-3b738308")
	if !reflect.DeepEqual(cmd.Args, wantAttachArgs) {
		t.Errorf("args = %v, want %v", cmd.Args, wantAttachArgs)
	}
	if got := envValues(cmd, "TERM"); !reflect.DeepEqual(got, []string{"xterm-256color"}) {
		t.Errorf("TERM = %v, want exactly one xterm-256color", got)
	}
}

// An environment entry with no "=" in it is not a variable and belongs to
// neither name, so the scrub has to leave it alone rather than mistake it for
// one of the names it is dropping.
func TestScrubEnvKeepsNamelessEntries(t *testing.T) {
	got := scrubEnv([]string{"TMUX", "TMUX=abc", "PATH=/bin"}, "TMUX")
	want := []string{"TMUX", "PATH=/bin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scrubEnv = %v, want %v", got, want)
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
	// The tagging call sets the slice tag first and the status bar's label
	// after it, so the tag is the option-and-value the call opens with.
	tag := launch.calls[1].args
	option, value := tag[4], tag[5]

	// tmux reports the option back where the format asked for it, which is the
	// first field of the line.
	format := listPanesFormat()
	if !strings.HasPrefix(format, "#{"+option+"}\t") {
		t.Fatalf("format %q does not read back %q", format, option)
	}

	read := &fakeRunner{out: fmt.Sprintf("%s\t%%1\t%s\t@0\t0\t80\t\n", value, session)}
	live, err := NewTmuxWithRunner(read).LiveSlices()
	if err != nil {
		t.Fatalf("LiveSlices: %v", err)
	}
	if got := live[id]; got != session {
		t.Errorf("live[%q] = %q, want %q", id, got, session)
	}
}
