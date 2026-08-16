package tui

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/vterm"
)

// fakeTerm stands in for a vterm session: it records what was typed at it and
// answers with whatever frame the test set, in place of a child on a
// pseudo-terminal.
//
// The two channels are unbuffered and empty unless a test primes them, so the
// real awaitTerm blocks on one rather than spinning — which is what lets the
// one test of the real wait choose which way the select goes.
type fakeTerm struct {
	frame     string
	cursor    [2]int
	shown     bool
	resizeErr error
	err       error

	keys    []string
	events  []uv.KeyPressEvent
	raw     []string
	mice    []fakeMouse
	pastes  []string
	resizes [][2]int
	closes  int

	// cmd, cols and rows are what the session was started with.
	cmd  *exec.Cmd
	cols int
	rows int

	output chan struct{}
	done   chan struct{}
}

// fakeMouse is one mouse event as the fake was asked to send it.
type fakeMouse struct {
	x, y   int
	button uv.MouseButton
	mod    uv.KeyMod
	kind   vterm.MouseKind
}

var _ termSession = (*fakeTerm)(nil)

func newFakeTerm() *fakeTerm {
	return &fakeTerm{output: make(chan struct{}), done: make(chan struct{})}
}

func (f *fakeTerm) Render() string { return f.frame }

func (f *fakeTerm) Cursor() (int, int, bool) { return f.cursor[0], f.cursor[1], f.shown }

func (f *fakeTerm) SendKey(k uv.KeyPressEvent) {
	f.keys = append(f.keys, uv.Key(k).String())
	f.events = append(f.events, k)
}

func (f *fakeTerm) SendBytes(p []byte) { f.raw = append(f.raw, string(p)) }

func (f *fakeTerm) SendMouse(x, y int, button uv.MouseButton, mod uv.KeyMod, kind vterm.MouseKind) {
	f.mice = append(f.mice, fakeMouse{x: x, y: y, button: button, mod: mod, kind: kind})
}

func (f *fakeTerm) Paste(text string) { f.pastes = append(f.pastes, text) }

func (f *fakeTerm) Resize(cols, rows int) error {
	f.resizes = append(f.resizes, [2]int{cols, rows})
	return f.resizeErr
}

func (f *fakeTerm) Output() <-chan struct{} { return f.output }

func (f *fakeTerm) Done() <-chan struct{} { return f.done }

func (f *fakeTerm) Err() error { return f.err }

func (f *fakeTerm) Close() { f.closes++ }

// fakeTermFor stands the fake in for the real pseudo-terminal for the length of
// one test, and hands back the session every viewer opened during it will run.
func fakeTermFor(t *testing.T) *fakeTerm {
	t.Helper()
	term := newFakeTerm()
	old := startTerm
	startTerm = func(cmd *exec.Cmd, cols, rows int) (termSession, error) {
		term.cmd, term.cols, term.rows = cmd, cols, rows
		return term, nil
	}
	t.Cleanup(func() { startTerm = old })
	return term
}

// viewerApp is an app of a measured window showing an agent's terminal beside
// the board, opened the way t opens it. It returns the fake the viewer runs on.
func viewerApp(t *testing.T) (*App, *fakeLauncher, *fakeTerm) {
	t.Helper()
	app, launcher, _ := launchApp(t)
	term := fakeTermFor(t)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	id, session := sliceAt(t, app, rowTodoSlice)
	app.live = map[string]string{id: session}
	launcher.live = app.live
	feed(t, app, press(app, "t"))
	if app.viewer == nil {
		t.Fatal("t should have opened the agent's terminal")
	}
	return app, launcher, term
}

// focus hands the keyboard to the terminal, the way tab does.
func focus(t *testing.T, a *App) {
	t.Helper()
	feed(t, a, press(a, "tab"))
	if !a.viewerFocused() {
		t.Fatal("tab should have focused the terminal")
	}
}

// pressKey feeds one key press described by its keystroke to the app.
func pressKey(a *App, k tea.Key) tea.Cmd {
	_, cmd := a.Update(tea.KeyPressMsg(k))
	return cmd
}

// t opens the agent's terminal on the hidden client, and takes the board's
// columns for it.
func TestAppOpensTheAgentTerminal(t *testing.T) {
	app, launcher, term := viewerApp(t)

	if want := []string{agent.SessionName("s5")}; !reflect.DeepEqual(launcher.clients, want) {
		t.Errorf("clients = %v, want the hidden client attached to %v", launcher.clients, want)
	}
	if len(launcher.attached) != 0 {
		t.Errorf("attached = %v, want the board kept on screen", launcher.attached)
	}
	if term.cols != 50 || term.rows < minTermRows {
		t.Errorf("started at %dx%d, want the box's interior", term.cols, term.rows)
	}
	// The board has given up its share of the window to the split.
	if got, want := app.board.width, app.boardWidth(); got != want {
		t.Errorf("board width = %d, want the split's %d", got, want)
	}
	if app.busy {
		t.Error("the terminal is a widget on the board, not something it waits on")
	}
}

// t again takes it off the board, gives the pseudo-terminal back, and refetches
// the one page the agent has been working on.
func TestAppClosesTheAgentTerminal(t *testing.T) {
	client := &fakeNotion{getPage: pageFor("s5", "Info view", notion.SliceInProgress, "M2: Board")}
	app, _, term := viewerApp(t)
	app.client = client

	feed(t, app, press(app, "t"))

	if app.viewer != nil {
		t.Fatalf("viewer = %+v, want the terminal gone", app.viewer)
	}
	if term.closes != 1 {
		t.Errorf("closes = %d, want the session closed exactly once", term.closes)
	}
	if !equal(client.fetchedPages, []string{"s5"}) {
		t.Errorf("fetched %v, want the agent's slice refetched", client.fetchedPages)
	}
	if client.queriedDSIDs != nil {
		t.Errorf("queried %v, want no full reload", client.queriedDSIDs)
	}
	if got := app.boardWidth(); got != app.innerWidth() {
		t.Errorf("board width = %d, want the whole band of %d back", got, app.innerWidth())
	}
}

// The planning agent works the whole plan rather than a page of it, so putting
// its terminal away reloads the lot.
func TestAppClosingThePlanningTerminalReloadsThePlan(t *testing.T) {
	client := newLoadingClient()
	app, _, _ := launchApp(t)
	app.client = client
	fakeTermFor(t)
	app.live = map[string]string{agent.PlanSentinel: agent.PlanSession}

	feed(t, app, press(app, "w"))
	feed(t, app, press(app, "w"))

	if client.fetchedPages != nil {
		t.Errorf("fetched %v, want no single-page refetch of the sentinel", client.fetchedPages)
	}
	if len(client.queriedDSIDs) != 1 {
		t.Errorf("queried %v, want the full load", client.queriedDSIDs)
	}
}

// One agent is on show at a time: opening a second closes the first, and the
// slice the first was about is refetched on the way past.
func TestAppSwapsOneAgentTerminalForAnother(t *testing.T) {
	app, launcher, first := viewerApp(t)
	second := fakeTermFor(t)
	other, _ := sliceAt(t, app, rowClaimedSlice)
	app.live[other] = agent.SessionName(other)

	feed(t, app, press(app, "t"))

	if first.closes != 1 {
		t.Errorf("closes = %d, want the terminal that was on show closed", first.closes)
	}
	if app.viewer == nil || app.viewer.session != second {
		t.Fatalf("viewer = %+v, want the second agent on show", app.viewer)
	}
	if app.viewer.sliceID != other {
		t.Errorf("viewer slice = %q, want %q", app.viewer.sliceID, other)
	}
	if want := []string{agent.SessionName("s5"), agent.SessionName(other)}; !reflect.DeepEqual(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
}

func TestAppReportsATerminalThatWillNotStart(t *testing.T) {
	app, _, _ := launchApp(t)
	startTerm = func(*exec.Cmd, int, int) (termSession, error) { return nil, errors.New("no pty") }
	t.Cleanup(func() {
		startTerm = func(*exec.Cmd, int, int) (termSession, error) { return newFakeTerm(), nil }
	})
	id, session := sliceAt(t, app, rowTodoSlice)
	app.live = map[string]string{id: session}

	feed(t, app, press(app, "t"))

	if app.viewer != nil {
		t.Errorf("viewer = %+v, want nothing on show", app.viewer)
	}
	if app.err == nil || !strings.Contains(app.err.Error(), `show the agent for "Info view"`) {
		t.Errorf("err = %v, want the failed start", app.err)
	}
}

// The three ways a viewer opens: t on a slice, w on the plan, and the attach
// that follows a launch — which now works with no tmux window to join into.
func TestAppLaunchOpensTheAgentTerminal(t *testing.T) {
	app, launcher, _ := launchApp(t)
	term := fakeTermFor(t)
	app.board.cursor = rowTodoSlice

	launch(t, app)

	if want := []string{agent.SessionName("s5")}; !reflect.DeepEqual(launcher.clients, want) {
		t.Errorf("clients = %v, want %v", launcher.clients, want)
	}
	if len(launcher.attached) != 0 {
		t.Errorf("attached = %v, want the board kept on screen", launcher.attached)
	}
	if app.viewer == nil || app.viewer.session != term {
		t.Fatalf("viewer = %+v, want the launched agent on show", app.viewer)
	}
	if app.busy {
		t.Error("the launch is over; nothing is in flight")
	}
}

// The terminal is sized from the configured split, which is what the join-pane
// view used too.
func TestAppTerminalTakesTheConfiguredShare(t *testing.T) {
	app, _, _ := launchApp(t)
	term := fakeTermFor(t)
	app.cfg.AgentSplitPercent = 80
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	id, session := sliceAt(t, app, rowTodoSlice)
	app.live = map[string]string{id: session}

	feed(t, app, press(app, "t"))

	if _, want := app.splitWidths(); want != 80 {
		t.Errorf("terminal box = %d columns, want the configured 80%%", want)
	}
	if term.cols != 78 {
		t.Errorf("started at %d columns, want the box's interior", term.cols)
	}
}

// Neither box is squeezed out of existence, however narrow the window.
func TestSplitWidthsKeepBothBoxes(t *testing.T) {
	app, _, _ := launchApp(t)
	for _, width := range []int{0, 6, 10, 40, 80} {
		app.width = width
		board, term := app.splitWidths()
		if board < minBoxWidth || term < minBoxWidth {
			t.Errorf("at %d columns the split is %d/%d, want both at least %d", width, board, term, minBoxWidth)
		}
	}
}

// A window resize is passed to the child, so the winch reaches the hidden
// client and tmux resizes the session inside it.
func TestAppResizesTheTerminalWithTheWindow(t *testing.T) {
	app, _, term := viewerApp(t)
	before := len(term.resizes)

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if len(term.resizes) <= before {
		t.Fatalf("resizes = %v, want the new size passed on", term.resizes)
	}
	cols, rows := app.termSize()
	if got := term.resizes[len(term.resizes)-1]; got != [2]int{cols, rows} {
		t.Errorf("resized to %v, want %dx%d", got, cols, rows)
	}
}

// A child that will not resize is not a reason to stop drawing it.
func TestAppSurvivesATerminalThatWillNotResize(t *testing.T) {
	app, _, term := viewerApp(t)
	term.resizeErr = errors.New("no pty")

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if app.err != nil {
		t.Errorf("err = %v, want the failure kept off the board", app.err)
	}
}

// Before the window has been measured there is no box to size the child to, so
// it starts at what a terminal is assumed to be.
func TestTermSizeBeforeTheFirstResize(t *testing.T) {
	app, _, _ := launchApp(t)
	app.width, app.height = 0, 0
	if cols, rows := app.termSize(); cols != initTermCols || rows != initTermRows {
		t.Errorf("termSize = %dx%d, want %dx%d", cols, rows, initTermCols, initTermRows)
	}
}

func TestTermSizeHasAFloor(t *testing.T) {
	app, _, _ := launchApp(t)
	app.width, app.height = 10, 8
	if cols, rows := app.termSize(); cols != minTermCols || rows != minTermRows {
		t.Errorf("termSize = %dx%d, want the floor of %dx%d", cols, rows, minTermCols, minTermRows)
	}
}

// While the terminal has the keyboard everything goes to the agent — the
// board's own movement keys, the quit key, esc, and ctrl+c included.
func TestAppFocusedTerminalTakesEveryKey(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)
	cursor := app.board.cursor

	for _, k := range []string{"j", "q", "l", "esc", "ctrl+c"} {
		if cmd := press(app, k); isQuitCmd(cmd) {
			t.Fatalf("%q quit the app instead of reaching the agent", k)
		}
	}

	// The letters are text and go as their own bytes; the two that stand for no
	// character are the emulator's to encode.
	if want := []string{"j", "q", "l"}; !reflect.DeepEqual(term.raw, want) {
		t.Errorf("raw = %q, want %q", term.raw, want)
	}
	if want := []string{"esc", "ctrl+c"}; !reflect.DeepEqual(term.keys, want) {
		t.Errorf("keys = %v, want %v", term.keys, want)
	}
	if app.board.cursor != cursor {
		t.Errorf("cursor = %d, want the board left where it was", app.board.cursor)
	}
}

// The outer tmux's prefix is swallowed: the hidden client is a client of the
// same server, and a prefix reaching it would drive the board's own window.
func TestAppFocusedTerminalSwallowsTheTmuxPrefix(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)

	pressKey(app, tea.Key{Code: 'b', Mod: tea.ModCtrl})

	if len(term.keys) != 0 || len(term.raw) != 0 {
		t.Errorf("keys = %v, raw = %q, want the prefix swallowed", term.keys, term.raw)
	}
	if !app.viewerFocused() {
		t.Error("the prefix should not have taken the keyboard back")
	}
}

// The two enters claude tells apart go as CSI-u, which the emulator's own
// encoding would flatten to a carriage return.
func TestAppFocusedTerminalSendsTheModifiedEnters(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)

	pressKey(app, tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift})
	pressKey(app, tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl})

	if want := []string{shiftEnterBytes, ctrlEnterBytes}; !reflect.DeepEqual(term.raw, want) {
		t.Errorf("raw = %q, want %q", term.raw, want)
	}
	if len(term.keys) != 0 {
		t.Errorf("keys = %v, want the enters sent raw", term.keys)
	}
}

// Everything that stands for characters is typed at the agent as those
// characters: a capital letter and shifted punctuation both arrive as a
// modified key the emulator's encoder would write nothing for, and an accented
// character is a rune the keyboard never had a code for on its own.
func TestAppFocusedTerminalTypesTheTextOfAKey(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)

	for _, k := range []tea.Key{
		{Code: 'a', Mod: tea.ModShift, ShiftedCode: 'A', Text: "A"},
		{Code: '1', Mod: tea.ModShift, ShiftedCode: '!', Text: "!"},
		{Code: ';', Mod: tea.ModShift, ShiftedCode: ':', Text: ":"},
		{Code: '-', Mod: tea.ModShift, ShiftedCode: '_', Text: "_"},
		{Code: 'é', Text: "é"},
	} {
		pressKey(app, k)
	}

	if want := []string{"A", "!", ":", "_", "é"}; !reflect.DeepEqual(term.raw, want) {
		t.Errorf("raw = %q, want %q", term.raw, want)
	}
	if len(term.keys) != 0 {
		t.Errorf("keys = %v, want the text sent as itself", term.keys)
	}
}

// An alt-modified key carries no text of its own — the decoder drops it for
// anything beyond shift — so it stays the emulator's to encode, which is where
// the escape prefix comes from.
func TestAppFocusedTerminalLeavesAltToTheEncoder(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)

	pressKey(app, tea.Key{Code: 'a', Mod: tea.ModAlt})
	pressKey(app, tea.Key{Code: tea.KeyUp})

	if want := []string{"alt+a", "up"}; !reflect.DeepEqual(term.keys, want) {
		t.Errorf("keys = %v, want %v", term.keys, want)
	}
	if len(term.raw) != 0 {
		t.Errorf("raw = %q, want the keys left to the encoder", term.raw)
	}
}

// A ctrl combination is a control byte however it was decoded, so a terminal
// that reported text alongside it does not get the letter typed at it.
func TestAppFocusedTerminalPrefersCtrlOverText(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)

	pressKey(app, tea.Key{Code: 'd', Mod: tea.ModCtrl, Text: "d"})

	// The key's own String is its text, so the modifier is read off the event
	// the encoder was handed.
	want := []uv.KeyPressEvent{{Code: 'd', Mod: uv.ModCtrl, Text: "d"}}
	if !reflect.DeepEqual(term.events, want) {
		t.Errorf("events = %+v, want %+v", term.events, want)
	}
	if len(term.raw) != 0 {
		t.Errorf("raw = %q, want the combination encoded", term.raw)
	}
}

// ctrl+\ is the one key the terminal never gets, and the way back to the board
// from anything the agent is doing.
func TestAppUnfocusAlwaysWorks(t *testing.T) {
	app, _, term := viewerApp(t)
	focus(t, app)

	pressKey(app, tea.Key{Code: '\\', Mod: tea.ModCtrl})

	if app.viewerFocused() {
		t.Error("ctrl+\\ should have taken the keyboard back")
	}
	if len(term.keys) != 0 || len(term.raw) != 0 {
		t.Errorf("keys = %v, raw = %q, want the key kept from the agent", term.keys, term.raw)
	}
	// And the board answers its own keys again.
	feed(t, app, press(app, "j"))
	if len(term.keys) != 0 {
		t.Errorf("keys = %v, want the board's keys back", term.keys)
	}
}

// There is nothing to type at without a terminal on show, and nothing to type
// at one whose agent has gone.
func TestAppFocusNeedsATerminalToType(t *testing.T) {
	app, _, _ := launchApp(t)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if cmd := press(app, "tab"); cmd != nil || app.viewerFocused() {
		t.Error("there is no terminal to focus")
	}

	app, _, _ = viewerApp(t)
	app.viewer.exited = true
	press(app, "tab")
	if app.viewerFocused() {
		t.Error("an agent that has gone cannot be typed at")
	}
}

// A paste goes to the agent while it has the keyboard, and wherever it went
// before otherwise.
func TestAppPasteReachesTheFocusedTerminal(t *testing.T) {
	app, _, term := viewerApp(t)

	app.Update(tea.PasteMsg{Content: "before"})
	if len(term.pastes) != 0 {
		t.Errorf("pastes = %q, want the board's paste left alone", term.pastes)
	}

	focus(t, app)
	app.Update(tea.PasteMsg{Content: "after"})

	if want := []string{"after"}; !reflect.DeepEqual(term.pastes, want) {
		t.Errorf("pastes = %q, want %q", term.pastes, want)
	}
}

// The screen is redrawn as the child writes, and the waiting is re-armed each
// time so the next write is noticed too. The re-arm is the throttled one: the
// frame just drawn is the floor under the next read.
func TestAppRedrawsTheTerminalAsItWrites(t *testing.T) {
	app, _, term := viewerApp(t)
	var waits int
	awaitFrame = func(termSession) tea.Cmd {
		waits++
		return nil
	}
	t.Cleanup(func() { awaitFrame = func(termSession) tea.Cmd { return nil } })
	term.frame = "a fresh frame"

	_, cmd := app.Update(termOutputMsg{session: term})

	if app.viewer.frame != "a fresh frame" {
		t.Errorf("frame = %q, want the new one", app.viewer.frame)
	}
	if waits != 1 || cmd != nil {
		t.Errorf("waits = %d, want the wait re-armed once", waits)
	}
}

// A message from a session that is no longer on show is one the close raced,
// and says nothing about the terminal that is.
func TestAppIgnoresMessagesFromAClosedTerminal(t *testing.T) {
	app, _, term := viewerApp(t)
	stale := newFakeTerm()
	stale.frame = "the terminal that went"
	term.frame = "the terminal on show"
	app.viewer.capture()

	app.Update(termOutputMsg{session: stale})
	app.Update(termExitedMsg{session: stale})

	if app.viewer.frame != "the terminal on show" {
		t.Errorf("frame = %q, want the stale frame ignored", app.viewer.frame)
	}
	if app.viewer.exited {
		t.Error("a stale exit should not retire the terminal on show")
	}

	app.viewer = nil
	app.Update(termOutputMsg{session: stale})
	app.Update(termExitedMsg{session: stale})
}

// The two orders the news of an agent's exit can arrive in — the client
// reporting EOF first, and the live poll noticing first — settle the same way:
// the frame stays on screen, marked exited, and the slice is refetched once.
func TestAppTerminalExitConverges(t *testing.T) {
	tests := []struct {
		name  string
		order []func(term *fakeTerm) tea.Msg
	}{
		{"the client goes first", []func(*fakeTerm) tea.Msg{
			func(term *fakeTerm) tea.Msg { return termExitedMsg{session: term} },
			func(*fakeTerm) tea.Msg { return liveSessionsMsg{live: map[string]string{}} },
		}},
		{"the poll goes first", []func(*fakeTerm) tea.Msg{
			func(*fakeTerm) tea.Msg { return liveSessionsMsg{live: map[string]string{}} },
			func(term *fakeTerm) tea.Msg { return termExitedMsg{session: term} },
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeNotion{getPage: pageFor("s5", "Info view", notion.SliceDone, "M2: Board")}
			app, _, term := viewerApp(t)
			app.client = client
			term.frame = "the agent's last words"

			for _, msg := range tt.order {
				feed(t, app, mustCmd(app.Update(msg(term))))
			}

			if app.viewer == nil {
				t.Fatal("the frame should stay on screen after the agent has gone")
			}
			if !app.viewer.exited {
				t.Error("the viewer should be marked exited")
			}
			if app.viewerFocused() {
				t.Error("there is nothing left to type at")
			}
			if !equal(client.fetchedPages, []string{"s5"}) {
				t.Errorf("fetched %v, want the slice refetched exactly once", client.fetchedPages)
			}
		})
	}
}

// A failed read of the child is reported rather than passed off as the agent
// finishing.
func TestAppReportsATerminalThatFailed(t *testing.T) {
	app, _, term := viewerApp(t)

	feed(t, app, mustCmd(app.Update(termExitedMsg{session: term, err: errors.New("read pty")})))

	if !strings.Contains(app.toast, "read pty") {
		t.Errorf("toast = %q, want the failure reported", app.toast)
	}
	if !app.viewer.exited {
		t.Error("the viewer should still be marked exited")
	}
}

// mustCmd is the command half of an Update, for feeding onward.
func mustCmd(_ tea.Model, cmd tea.Cmd) tea.Cmd { return cmd }

// A failed live read proves nothing about the agent, so it leaves the terminal
// as it is.
func TestAppKeepsTheTerminalThroughAFailedPoll(t *testing.T) {
	app, _, _ := viewerApp(t)

	app.Update(liveSessionsMsg{err: errors.New("no server")})

	if app.viewer == nil || app.viewer.exited {
		t.Errorf("viewer = %+v, want it left alone", app.viewer)
	}
}

// Closing an exited terminal is only the frame being dismissed: its refetch
// happened when the agent went.
func TestAppClosingAnExitedTerminalRefetchesNothing(t *testing.T) {
	client := &fakeNotion{getPage: pageFor("s5", "Info view", notion.SliceDone, "M2: Board")}
	app, _, term := viewerApp(t)
	app.client = client
	feed(t, app, mustCmd(app.Update(termExitedMsg{session: term})))
	fetched := len(client.fetchedPages)

	feed(t, app, press(app, "t"))

	if app.viewer != nil {
		t.Errorf("viewer = %+v, want the frame dismissed", app.viewer)
	}
	if len(client.fetchedPages) != fetched {
		t.Errorf("fetched %v, want no second refetch", client.fetchedPages)
	}
}

// The real wait: whichever of the two channels speaks first is the message.
func TestAwaitTermReportsWhicheverComesFirst(t *testing.T) {
	term := newFakeTerm()
	close(term.output)
	if msg, ok := defaultAwaitTerm(term)().(termOutputMsg); !ok || msg.session != term {
		t.Errorf("msg = %#v, want the output of %p", msg, term)
	}

	gone := newFakeTerm()
	gone.err = errors.New("read pty")
	close(gone.done)
	msg, ok := defaultAwaitTerm(gone)().(termExitedMsg)
	if !ok || msg.session != gone {
		t.Fatalf("msg = %#v, want the exit of %p", msg, gone)
	}
	if !errors.Is(msg.err, gone.err) {
		t.Errorf("err = %v, want %v", msg.err, gone.err)
	}
}

// The throttled wait reports the same thing the bare one does, a frame later:
// a child that has already written is not read again until the frame on screen
// has had its time.
func TestAwaitFrameHoldsOffAFrame(t *testing.T) {
	term := newFakeTerm()
	close(term.output)

	start := time.Now()
	msg, ok := defaultAwaitFrame(term)().(termOutputMsg)
	elapsed := time.Since(start)

	if !ok || msg.session != term {
		t.Fatalf("msg = %#v, want the output of %p", msg, term)
	}
	if elapsed < frameInterval {
		t.Errorf("waited %v, want at least a frame (%v)", elapsed, frameInterval)
	}
}

// A burst of output costs one capture per frame rather than one per write. The
// child here is always ready with something — its output channel never empties
// — which is what a chatty agent looks like to the board, and the rate the
// screen is read at is still the frame rate.
func TestBurstsOfOutputAreCapturedAtTheFrameRate(t *testing.T) {
	app, _, term := viewerApp(t)
	awaitFrame = defaultAwaitFrame
	t.Cleanup(func() { awaitFrame = func(termSession) tea.Cmd { return nil } })
	// Buffered and topped up after every capture, so the wait always has a
	// notification to come back with however fast it goes round: the previous
	// wait drained it, and the child has written again by the time the next one
	// listens.
	term.output = make(chan struct{}, 1)

	const frames = 5
	start := time.Now()
	captures := 0
	msg := tea.Msg(termOutputMsg{session: term})
	for captures < frames {
		_, cmd := app.Update(msg)
		captures++
		if cmd == nil {
			t.Fatalf("capture %d re-armed nothing", captures)
		}
		term.output <- struct{}{}
		msg = cmd()
	}
	elapsed := time.Since(start)

	if captures != frames {
		t.Fatalf("captured %d times, want %d", captures, frames)
	}
	if want := (frames - 1) * frameInterval; elapsed < want {
		t.Errorf("%d captures took %v, want at least %v — the burst is not throttled",
			frames, elapsed, want)
	}
}

// The plan beside a terminal is drawn once and handed back: a change confined
// to the terminal box costs nothing on the board's side of the split, however
// fast the agent writes. Only something that says the board itself has changed
// draws its rows again, and all of those go through syncBoard.
func TestTheBoardIsNotRedrawnForTheTerminal(t *testing.T) {
	app, _, term := viewerApp(t)
	// Short enough that the plan overflows its band, so there is something a
	// scroll would uncover.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	width, _ := app.splitWidths()
	height := app.bodyBoxHeight()
	region := func() string { return strings.Join(app.boardRegion(width, height), "\n") }
	before := region()

	term.frame = "the child has written"
	app.Update(termOutputMsg{session: term})
	// The scroll window moves under the board with nothing to say it has: only
	// a redraw of the boxed rows could put the lines it uncovered on screen.
	app.boardVP.SetYOffset(app.boardVP.YOffset() + 1)

	if got := region(); got != before {
		t.Error("the board was redrawn for a change confined to the terminal")
	}

	// And the scroll that does say so — the wheel over the plan — draws them.
	app.boardVP.SetYOffset(0)
	app.scrollBoard(wheelLines)
	if app.boardVP.YOffset() == 0 {
		t.Fatal("the fixture plan does not scroll, so there is nothing to redraw")
	}
	if got := region(); got == before {
		t.Error("a scrolled board should have been drawn again")
	}
}

// A plan that has not landed is the one thing not kept: what stands in for it
// is a spinner, and a spinner held from one frame to the next is a stopped one.
func TestTheSpinningBoardIsNotKept(t *testing.T) {
	app, _, _ := viewerApp(t)
	width, _ := app.splitWidths()
	height := app.bodyBoxHeight()
	app.project, app.loading = nil, true

	first := strings.Join(app.boardRegion(width, height), "\n")
	app.spinner, _ = app.spinner.Update(app.spinner.Tick())
	if got := strings.Join(app.boardRegion(width, height), "\n"); got == first {
		t.Error("the spinner should turn while the plan is still loading")
	}
}

// The real start is what the app runs with when nothing is standing in for it.
// It is not run here: it would open a pseudo-terminal and a tmux client on it.
func TestTheRealTerminalEdgesAreThere(t *testing.T) {
	if _, err := defaultStartTerm(exec.Command("does-not-exist-nat"), 80, 24); err == nil {
		t.Error("starting a command that is not there should fail")
	}
}

// The background poll goes on running with a terminal on show, focused or not:
// a plan landing behind the agent changes nothing the user is in the middle of.
func TestPollRunsWithATerminalOnShow(t *testing.T) {
	app, _, _ := viewerApp(t)
	focus(t, app)

	if app.pollSuspended() {
		t.Error("an open terminal should not suspend the poll")
	}
	if cmd := app.polled(); cmd == nil {
		t.Error("the poll should still refetch the plan")
	}
}

// termOrigin is the window cell the terminal's own (0, 0) is drawn at: a column
// in from the box the split puts beside the board, and a line under its title.
func termOrigin(a *App) (x, y int) {
	board, _ := a.splitWidths()
	return board + 1, a.headerBandHeight() + 1
}

// mouseAt sends one mouse event of the given kind at a window cell.
func mouseAt(a *App, kind vterm.MouseKind, x, y int) {
	mouseAtWithMod(a, kind, x, y, 0)
}

// mouseAtWithMod sends one mouse event at a window cell with modifiers held.
func mouseAtWithMod(a *App, kind vterm.MouseKind, x, y int, mod tea.KeyMod) {
	m := tea.Mouse{X: x, Y: y, Button: tea.MouseLeft, Mod: mod}
	var msg tea.Msg
	switch kind {
	case vterm.MousePress:
		msg = tea.MouseClickMsg(m)
	case vterm.MouseRelease:
		msg = tea.MouseReleaseMsg(m)
	case vterm.MouseMotion:
		msg = tea.MouseMotionMsg(m)
	case vterm.MouseWheel:
		msg = tea.MouseWheelMsg(m)
	}
	a.Update(msg)
}

// A mouse event inside the terminal's box reaches the agent at the cell it
// landed on, whichever kind it is.
func TestViewerMouseInsideTheBoxReachesTheAgent(t *testing.T) {
	app, _, term := viewerApp(t)
	originX, originY := termOrigin(app)
	_, termWidth := app.splitWidths()

	for _, kind := range []vterm.MouseKind{
		vterm.MousePress, vterm.MouseRelease, vterm.MouseMotion, vterm.MouseWheel,
	} {
		mouseAt(app, kind, originX, originY)
	}
	// The far corner of the box's interior, which is the last cell the child has.
	lastX, lastY := termWidth-3, app.bodyBoxHeight()-3
	mouseAt(app, vterm.MouseWheel, originX+lastX, originY+lastY)

	want := []fakeMouse{
		{x: 0, y: 0, button: tea.MouseLeft, kind: vterm.MousePress},
		{x: 0, y: 0, button: tea.MouseLeft, kind: vterm.MouseRelease},
		{x: 0, y: 0, button: tea.MouseLeft, kind: vterm.MouseMotion},
		{x: 0, y: 0, button: tea.MouseLeft, kind: vterm.MouseWheel},
		{x: lastX, y: lastY, button: tea.MouseLeft, kind: vterm.MouseWheel},
	}
	if !reflect.DeepEqual(term.mice, want) {
		t.Errorf("mice = %+v, want %+v", term.mice, want)
	}
}

// The modifiers held during an event go to the agent with it: the tmux on the
// far end of the terminal binds C-MouseDown1Pane as well as MouseDown1Pane, and
// a ctrl+click that arrives stripped of its ctrl fires the wrong binding.
func TestViewerMouseCarriesTheModifiers(t *testing.T) {
	app, _, term := viewerApp(t)
	originX, originY := termOrigin(app)

	mouseAtWithMod(app, vterm.MousePress, originX+1, originY+1, tea.ModCtrl)
	mouseAtWithMod(app, vterm.MouseWheel, originX+1, originY+1, tea.ModShift|tea.ModAlt)

	want := []fakeMouse{
		{x: 1, y: 1, button: tea.MouseLeft, mod: tea.ModCtrl, kind: vterm.MousePress},
		{x: 1, y: 1, button: tea.MouseLeft, mod: tea.ModShift | tea.ModAlt, kind: vterm.MouseWheel},
	}
	if !reflect.DeepEqual(term.mice, want) {
		t.Errorf("mice = %+v, want %+v", term.mice, want)
	}
}

// A click inside the box takes the keyboard, so tab is not the only way to type
// at an agent, and a click on the board hands it straight back.
func TestViewerClickMovesTheFocus(t *testing.T) {
	app, _, term := viewerApp(t)
	originX, originY := termOrigin(app)

	mouseAt(app, vterm.MousePress, originX+3, originY+2)
	if !app.viewerFocused() {
		t.Fatal("a click in the terminal should have focused it")
	}

	mouseAt(app, vterm.MousePress, 0, originY)
	if app.viewerFocused() {
		t.Error("a click on the board should have taken the keyboard back")
	}
	if want := []fakeMouse{{x: 3, y: 2, button: tea.MouseLeft, kind: vterm.MousePress}}; !reflect.DeepEqual(term.mice, want) {
		t.Errorf("mice = %+v, want only the click inside the box %+v", term.mice, want)
	}
}

// Everything outside the box belongs to whatever else the window is drawing.
func TestViewerDropsMouseOutsideTheBox(t *testing.T) {
	app, _, term := viewerApp(t)
	originX, originY := termOrigin(app)
	_, termWidth := app.splitWidths()

	for _, at := range [][2]int{
		{originX - 1, originY},                       // the box's left border
		{originX, originY - 1},                       // its title line
		{originX + termWidth - 2, originY},           // its right border
		{originX, originY + app.bodyBoxHeight() - 2}, // its bottom border
		{0, 0}, // the header, above the band entirely
	} {
		mouseAt(app, vterm.MouseMotion, at[0], at[1])
	}

	if term.mice != nil {
		t.Errorf("mice = %+v, want nothing sent from outside the box", term.mice)
	}
	if app.viewerFocused() {
		t.Error("motion outside the box should not have moved the keyboard")
	}
}

// oddMouseMsg is a mouse message of no kind the terminal knows, which the
// library could grow one of.
type oddMouseMsg struct{ at tea.Mouse }

func (m oddMouseMsg) Mouse() tea.Mouse { return m.at }

func (m oddMouseMsg) String() string { return m.at.String() }

// A kind of mouse event the session has no name for goes nowhere, rather than
// reaching the agent as some other kind.
func TestViewerDropsAnUnknownMouseKind(t *testing.T) {
	app, _, term := viewerApp(t)
	originX, originY := termOrigin(app)

	app.Update(oddMouseMsg{at: tea.Mouse{X: originX, Y: originY, Button: tea.MouseLeft}})

	if term.mice != nil {
		t.Errorf("mice = %+v, want nothing sent", term.mice)
	}
	if app.viewerFocused() {
		t.Error("an event of no known kind should not have moved the keyboard")
	}
}

// A terminal nobody can see, and one whose agent has gone, take no mouse: the
// first is behind something else, the second a frame being read.
func TestViewerTakesNoMouseWhenItIsNotLive(t *testing.T) {
	app, _, term := viewerApp(t)
	originX, originY := termOrigin(app)

	app.viewer.exited = true
	mouseAt(app, vterm.MousePress, originX, originY)
	app.viewer.exited = false

	app.screen = screenHelp
	mouseAt(app, vterm.MousePress, originX, originY)
	app.screen = screenBoard

	app.viewer = nil
	mouseAt(app, vterm.MousePress, originX, originY)

	if term.mice != nil {
		t.Errorf("mice = %+v, want nothing sent", term.mice)
	}
}

// The mouse is reported only while there is a terminal to route it to, so the
// user's own selection and scrollback are left alone the rest of the time.
func TestAppAsksForTheMouseOnlyWithATerminalUp(t *testing.T) {
	app, _, _ := viewerApp(t)

	if got := app.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Errorf("mouse mode = %v, want all motion beside a terminal", got)
	}

	feed(t, app, press(app, "t"))
	if got := app.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("mouse mode = %v, want the mouse handed back with the terminal gone", got)
	}
}

// The hints say what the keys do while a terminal is up, and each state says
// only what applies to it.
func TestAppViewerHints(t *testing.T) {
	app, _, _ := viewerApp(t)

	shown := hintText(app)
	for _, want := range []string{"tab/click type at the agent", "t hide the agent", "T agent full-screen"} {
		if !strings.Contains(shown, want) {
			t.Errorf("hints = %q, want %q", shown, want)
		}
	}
	if strings.Contains(shown, "? help") {
		t.Errorf("hints = %q, want the row's own keys made way", shown)
	}

	focus(t, app)
	if got := hintText(app); !strings.Contains(got, `ctrl+\/click back to the board`) || strings.Contains(got, "tab") {
		t.Errorf("hints = %q, want only the way back", got)
	}

	app.viewer.focused, app.viewer.exited = false, true
	if got := hintText(app); !strings.Contains(got, "t close the agent") {
		t.Errorf("hints = %q, want the way to dismiss the frame", got)
	}

	feed(t, app, press(app, "t"))
	if got := hintText(app); !strings.Contains(got, "? help") {
		t.Errorf("hints = %q, want the ordinary hints back", got)
	}
}

// hintText is the hints row as the user reads it.
func hintText(a *App) string {
	return stripANSI(strings.Join(a.hintLines(), "\n"))
}

// The terminal is drawn only on the board, and only in a window big enough for
// the framed layout. It stays alive behind anything pushed over it.
func TestViewerIsDrawnOnlyOnTheBoard(t *testing.T) {
	app, _, term := viewerApp(t)
	term.frame = "the agent is working"
	app.viewer.capture()

	if !strings.Contains(stripANSI(app.View().Content), "the agent is working") {
		t.Error("the terminal should be drawn beside the board")
	}

	feed(t, app, press(app, "?"))
	if strings.Contains(stripANSI(app.View().Content), "the agent is working") {
		t.Error("the help screen takes the whole band")
	}
	if app.viewer == nil {
		t.Fatal("the terminal should still be alive behind it")
	}

	feed(t, app, press(app, "?"))
	if !strings.Contains(stripANSI(app.View().Content), "the agent is working") {
		t.Error("the terminal should come back with the board")
	}

	app.Update(tea.WindowSizeMsg{Width: 20, Height: 4})
	if app.viewerVisible() {
		t.Error("a window too small for the frame draws the board alone")
	}
}

// The cursor is the app's only while the agent has the keyboard and the child
// has it shown, and it lands where the frame draws it.
func TestAppShowsTheTerminalCursorWhenFocused(t *testing.T) {
	app, _, term := viewerApp(t)
	term.cursor, term.shown = [2]int{3, 2}, true

	if app.View().Cursor != nil {
		t.Error("an unfocused terminal does not own the cursor")
	}

	focus(t, app)
	app.viewer.capture()
	cursor := app.View().Cursor
	if cursor == nil {
		t.Fatal("a focused terminal should own the cursor")
	}
	board, _ := app.splitWidths()
	if cursor.X != board+1+3 || cursor.Y != app.headerBandHeight()+1+2 {
		t.Errorf("cursor = %+v, want it inside the terminal's box", cursor.Position)
	}

	term.shown = false
	app.viewer.capture()
	if app.View().Cursor != nil {
		t.Error("a hidden cursor is not drawn")
	}
}

// T hands the whole window to the agent's session, the way the board did
// before there was a terminal to draw it in.
func TestAppFullAttachHatch(t *testing.T) {
	app, launcher, _ := launchApp(t)
	id, session := sliceAt(t, app, rowTodoSlice)
	app.live = map[string]string{id: session}

	if cmd := press(app, "T"); cmd == nil {
		t.Fatal("T should attach to the live session")
	}
	if want := []string{session}; !equal(launcher.attached, want) {
		t.Errorf("attached = %v, want %v", launcher.attached, want)
	}
	if len(launcher.clients) != 0 {
		t.Errorf("clients = %v, want no embedded terminal", launcher.clients)
	}
	if !app.busy {
		t.Error("the terminal is the session's until it is given back")
	}
}

func TestAppFullAttachIsRefused(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(*App)
		want string
	}{
		{"no launcher", func(a *App) { a.launcher = nil }, ""},
		{"already in flight", func(a *App) { a.busy = true }, ""},
		{"a milestone under the cursor", func(a *App) { a.board.cursor = rowActiveMilestone }, "Move to a slice"},
		{"nothing running", func(a *App) { a.live = nil }, "No agent session is running"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, launcher, _ := launchApp(t)
			id, session := sliceAt(t, app, rowTodoSlice)
			app.live = map[string]string{id: session}
			tt.set(app)

			feed(t, app, press(app, "T"))

			if len(launcher.attached) != 0 {
				t.Errorf("attached = %v, want nothing attached to", launcher.attached)
			}
			if tt.want != "" && !strings.Contains(app.board.confirmText, tt.want) {
				t.Errorf("confirm = %q, want %q", app.board.confirmText, tt.want)
			}
		})
	}
}

// How the whole window reads with an agent's terminal beside the board, and
// how it changes when the keyboard is the agent's.
func TestAppAgentViewerGolden(t *testing.T) {
	a := sizedApp(80, 16)
	a.launcher = &fakeLauncher{}
	term := newFakeTerm()
	term.frame = "❯ claude\n  Working on the slice…\n  · thinking"
	a.viewer = &agentViewer{session: term, sliceID: "s1", name: "Keep the status bar in", tmuxSession: "nat-s1"}
	a.viewer.capture()
	a.resize()

	golden(t, "app-agent-viewer-80", a.View().Content)

	a.viewer.focused = true
	golden(t, "app-agent-viewer-focused-80", a.View().Content)
}

// An exited agent says so in its title, in place of the live mark.
func TestViewerTitleMarksAnExitedAgent(t *testing.T) {
	app, _, term := viewerApp(t)
	term.frame = "done"

	feed(t, app, mustCmd(app.Update(termExitedMsg{session: term})))

	view := stripANSI(app.View().Content)
	if !strings.Contains(view, "exited") {
		t.Errorf("view is missing the exited mark:\n%s", view)
	}
}

// A frame wider or taller than the box it is drawn in is cut rather than
// allowed to push the border out.
func TestViewerContentIsClipped(t *testing.T) {
	app, _, term := viewerApp(t)
	term.frame = strings.Repeat("x", 200) + "\n" + strings.Repeat("y\n", 60)
	app.viewer.capture()

	checkFits(t, app.View().Content, 80, 24)
}

// The default split is the one the config names.
func TestViewerUsesTheConfiguredSplitDefault(t *testing.T) {
	app, _, _ := launchApp(t)
	app.width = 100
	if _, term := app.splitWidths(); term != config.DefaultSplitPercent {
		t.Errorf("terminal box = %d, want the default %d%% of 100", term, config.DefaultSplitPercent)
	}
}

// The real start does open a pseudo-terminal, which is worth running once: it
// is what every viewer runs on, and the fake proves nothing about it.
func TestDefaultStartTermRunsAChild(t *testing.T) {
	s, err := defaultStartTerm(exec.Command("true"), 20, 5)
	if err != nil {
		t.Fatalf("defaultStartTerm: %v", err)
	}
	defer s.Close()
	<-s.Done()
	if s.Err() != nil {
		t.Errorf("Err = %v, want the ordinary way out", s.Err())
	}
}

// With no plan on the board — a viewer left open across a project switch — a
// slice's page has nowhere to be patched into, so nothing is refetched.
func TestAfterViewingNeedsAPlanToPatch(t *testing.T) {
	app, _, _ := viewerApp(t)
	app.project = nil

	if cmd := app.afterViewing(app.viewer); cmd != nil {
		t.Error("there is no plan to patch the slice into")
	}
}

// A band with only its title line in it draws that and no box: there is no
// room left for one, and half a border reads as a glitch.
func TestViewerRegionInATitlesWorthOfLines(t *testing.T) {
	app, _, _ := viewerApp(t)

	lines := app.viewerRegion(20, 1)

	if len(lines) != 1 {
		t.Fatalf("lines = %d, want the one line it was given", len(lines))
	}
	if got := stripANSI(lines[0]); !strings.Contains(got, "Info view") {
		t.Errorf("line = %q, want the title", got)
	}
}

// A region shorter than the band it is laid into pads out rather than running
// off the end of its own lines.
func TestLineAtPastTheEnd(t *testing.T) {
	if got := lineAt([]string{"one"}, 3); got != "" {
		t.Errorf("lineAt = %q, want nothing past the end", got)
	}
}
