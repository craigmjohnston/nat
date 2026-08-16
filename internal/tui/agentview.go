package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/vterm"
)

// termSession is what the viewer needs of a [vterm.Session]: the screen to
// draw, the ways to type at it, and the two channels that say when to redraw
// and when it has gone. It is an interface so the flows can be driven without a
// pseudo-terminal.
type termSession interface {
	Render() string
	Cursor() (x, y int, visible bool)
	SendKey(uv.KeyPressEvent)
	SendBytes(p []byte)
	SendMouse(x, y int, button uv.MouseButton, mod uv.KeyMod, kind vterm.MouseKind)
	Paste(text string)
	Resize(cols, rows int) error
	Output() <-chan struct{}
	Done() <-chan struct{}
	Err() error
	Close()
}

// The viewer's edges, held as variables so the tests can stand in for them: the
// real ones open a pseudo-terminal and block on a channel.
var (
	startTerm  = defaultStartTerm
	awaitTerm  = defaultAwaitTerm
	awaitFrame = defaultAwaitFrame
)

// frameInterval is the floor under the time between two reads of the agent's
// screen, and so under the time between two redraws the child causes. It is the
// renderer's own frame at 60fps: a capture more often than that is a screen
// rendered to a string that nothing ever puts on the terminal.
const frameInterval = time.Second / 60

// defaultStartTerm runs cmd on a real pseudo-terminal.
func defaultStartTerm(cmd *exec.Cmd, cols, rows int) (termSession, error) {
	s, err := vterm.Start(cmd, cols, rows)
	if err != nil {
		// Returning s directly would hand back a non-nil interface holding a nil
		// pointer, which every caller would read as a session.
		return nil, err //nolint:wrapcheck // vterm names itself
	}
	return s, nil
}

// defaultAwaitTerm waits for the session to say something: that its screen has
// changed, or that it has ended. Whichever comes first is the message, and the
// handler for an output re-arms it.
func defaultAwaitTerm(s termSession) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-s.Output():
			return termOutputMsg{session: s}
		case <-s.Done():
			return termExitedMsg{session: s, err: s.Err()}
		}
	}
}

// defaultAwaitFrame is the wait armed once a frame has been drawn: the same
// listen as [defaultAwaitTerm], held off until the drawn frame's time is up.
//
// It is what keeps a chatty agent from costing a capture and a redraw per
// write. The session hands its screen over a byte at a time as the child
// produces it, and every notification that reaches the board is a render of the
// whole emulator screen followed by a render of the whole window — at the rate
// a PTY delivers a burst, most of them for a frame the terminal never shows. A
// write during the pause is not lost: the session holds one pending
// notification and coalesces the rest into it, so the wait comes straight back
// with everything written while it slept, and the screen is at most one frame
// behind the child however fast it writes.
func defaultAwaitFrame(s termSession) tea.Cmd {
	next := defaultAwaitTerm(s)
	return func() tea.Msg {
		time.Sleep(frameInterval)
		return next()
	}
}

// The messages the viewer comes back as. Each carries the session it is about,
// so a message from a viewer that has already been closed — a redraw racing the
// key that swapped the terminal for another one — is recognised and dropped.
type (
	// termStartedMsg reports the terminal the viewer runs, or the failure that
	// stopped it starting.
	termStartedMsg struct {
		session     termSession
		sliceID     string
		name        string
		tmuxSession string
		err         error
	}
	// termOutputMsg says the child has written something and the frame on
	// screen is out of date.
	termOutputMsg struct{ session termSession }
	// termExitedMsg says the child has gone, with the failure that ended it —
	// nil for the ordinary way out, which is the agent finishing.
	termExitedMsg struct {
		session termSession
		err     error
	}
)

// agentViewer is the agent's terminal as the board draws it: the session
// itself, what it is showing — a slice's page ID, or [agent.PlanSentinel] for
// the planning agent — and the last frame read off it.
//
// The frame is cached rather than rendered from the emulator on every draw: the
// screen is read under the session's own lock, and View runs far more often
// than the child writes.
type agentViewer struct {
	session     termSession
	sliceID     string
	name        string
	tmuxSession string

	// focused says the keyboard belongs to the terminal rather than the board.
	focused bool

	frame       string
	cursorX     int
	cursorY     int
	cursorShown bool
}

// capture reads the screen and the cursor off the session, which is the only
// time either is touched from the draw path.
func (v *agentViewer) capture() {
	v.frame = v.session.Render()
	v.cursorX, v.cursorY, v.cursorShown = v.session.Cursor()
}

// The viewer's measurements: the least columns a box of the split may be, and
// the floor the terminal itself is sized to — a child told it has two lines
// draws something nobody can read, and tmux refuses sizes below its own
// minimum. The pre-measure size is what a viewer opened before the first resize
// starts at; the resize that follows corrects it.
const (
	minBoxWidth  = 4
	minTermCols  = 20
	minTermRows  = 5
	initTermCols = 80
	initTermRows = 24
)

// The bytes the CSI-u encodings of the two enters the board forwards by hand.
// The emulator would send an ordinary carriage return for both — the modifier
// is the whole point of them in claude, where shift+enter breaks the line and
// ctrl+enter accepts — so they are written raw. The client advertises extkeys,
// so the tmux on the far end passes them through.
const (
	shiftEnterBytes = "\x1b[13;2u"
	ctrlEnterBytes  = "\x1b[13;5u"
)

// openAgentViewer swaps the terminal beside the board for one attached to the
// named session. Whatever was there is closed first — one agent is on show at a
// time, and the board is the thing the split is beside.
func (a *App) openAgentViewer(sliceID, name, session string) tea.Cmd {
	return tea.Sequence(a.closeViewer(), a.startViewer(sliceID, name, session))
}

// startViewer is the command that opens the pseudo-terminal: a hidden tmux
// client attached to the agent's session, so nat owns the whole rectangle and
// the agent goes on living in the session it was launched into.
func (a *App) startViewer(sliceID, name, session string) tea.Cmd {
	cols, rows := a.termSize()
	l := a.launcher
	return func() tea.Msg {
		s, err := startTerm(l.AttachClientCmd(session), cols, rows)
		if err != nil {
			return termStartedMsg{err: fmt.Errorf("show the agent for %q: %w", name, err)}
		}
		return termStartedMsg{session: s, sliceID: sliceID, name: name, tmuxSession: session}
	}
}

// termStarted puts the terminal that came back beside the board.
//
// It opens unfocused, which is what the join-pane split did with tmux's -d: the
// board keeps the keyboard, and tab is how the agent is typed at.
func (a *App) termStarted(msg termStartedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	v := &agentViewer{session: msg.session, sliceID: msg.sliceID,
		name: msg.name, tmuxSession: msg.tmuxSession}
	v.capture()
	a.viewer = v
	// The board has just lost most of its columns to the split.
	a.resize()
	return a, awaitTerm(msg.session)
}

// termOutput redraws the frame and waits for the next change. A message about a
// session that is no longer on screen is one the close raced, and says nothing
// about the terminal that is.
//
// The wait it re-arms is the throttled one: a frame has just been drawn, and
// the next is not worth reading off the emulator until this one has been on
// screen — see [defaultAwaitFrame].
func (a *App) termOutput(msg termOutputMsg) (tea.Model, tea.Cmd) {
	if !a.viewing(msg.session) {
		return a, nil
	}
	a.viewer.capture()
	return a, awaitFrame(msg.session)
}

// termExited takes the terminal off the board: the client has reported EOF,
// which is the pseudo-terminal's way of saying the session it was attached to
// has gone.
//
// The news is the end of the PTY rather than the status the client exited with
// — `tmux attach-session` exits zero whether its session ended under it or the
// user detached, so a code would say nothing anyway, and [vterm.Session.Err]
// reports only a PTY that failed, never a child's exit. A message about a
// session no longer on screen is one the close raced.
func (a *App) termExited(msg termExitedMsg) (tea.Model, tea.Cmd) {
	if !a.viewing(msg.session) {
		return a, nil
	}
	return a, tea.Batch(a.viewerExited(msg.err), a.refreshLive())
}

// viewing reports whether s is the session the board is showing.
func (a *App) viewing(s termSession) bool {
	return a.viewer != nil && a.viewer.session == s
}

// viewerExited is what a viewer whose agent has gone does, whichever way the
// news arrives first: the client reporting EOF, or the live poll finding the
// session no longer running. Whichever it is, the terminal goes — a box holding
// a frame nothing will ever write to again is the board's width being held by a
// dead agent — and the one that arrives second finds nothing on show and says
// nothing.
//
// That the terminal closes either way is also what keeps the refetch to one:
// dropping the viewer is what the second piece of news is recognised by.
func (a *App) viewerExited(err error) tea.Cmd {
	v := a.dropViewer()
	if v == nil {
		return nil
	}
	if err != nil {
		// The client died with its session still running: the session is named,
		// because reattaching to it is what the user will want next.
		return a.showToast(fmt.Sprintf("Lost the terminal for %q (%s): %v",
			v.name, v.tmuxSession, err), sevError)
	}
	return a.afterViewing(v)
}

// closeViewer takes the terminal off the board at the user's asking, and brings
// the board up to date with whatever the agent did while it was on show.
func (a *App) closeViewer() tea.Cmd {
	v := a.dropViewer()
	if v == nil {
		return nil
	}
	return a.afterViewing(v)
}

// dropViewer takes the terminal off the board and hands its pseudo-terminal
// back, returning the viewer that was there — or nil where there was none, which
// is how a second report of the same exit is told from the first.
func (a *App) dropViewer() *agentViewer {
	v := a.viewer
	if v == nil {
		return nil
	}
	a.viewer = nil
	v.session.Close()
	// The board has the window's width back.
	a.resize()
	return v
}

// afterViewing brings the board up to date with what the agent did while it was
// on show. A slice's agent has been working one page, so that page is refetched;
// the planning agent works the whole plan, so the plan is reloaded.
func (a *App) afterViewing(v *agentViewer) tea.Cmd {
	if v.sliceID == agent.PlanSentinel {
		return a.startLoad()
	}
	if a.project == nil || a.client == nil {
		return nil
	}
	return a.refreshSlice(v.sliceID)
}

// viewerVisible reports whether the terminal is actually drawn: only on the
// board, and only in a window big enough for the framed layout. Help, info, a
// form and the unframed fallback each take the whole band — the viewer stays
// alive behind them, undrawn, and comes back with the board.
func (a *App) viewerVisible() bool {
	return a.viewer != nil && a.screen == screenBoard && a.framed()
}

// viewerFocused reports whether the keyboard belongs to the terminal.
func (a *App) viewerFocused() bool {
	return a.viewer != nil && a.viewer.focused
}

// viewerKey is every key press while the terminal has the keyboard. Only the
// way out is the board's; the rest go to the agent, ctrl+c included — a session
// being typed at is one where ctrl+c means "stop what you are doing", not "quit
// nat", and ctrl+\ is the way back either way.
//
// The outer tmux's prefix is swallowed rather than forwarded: the hidden client
// is a client of the same server, and a prefix reaching it would work the
// board's own window from inside the agent's.
//
// Anything that stands for characters is written as those characters and only
// the rest is handed to the emulator's key encoder — see [printableText].
func (a *App) viewerKey(msg tea.KeyPressMsg) tea.Cmd {
	v := a.viewer
	switch {
	case key.Matches(msg, a.keys.Unfocus):
		v.focused = false
	case key.Matches(msg, a.keys.TmuxPrefix):
	case key.Matches(msg, a.keys.ShiftEnter):
		v.session.SendBytes([]byte(shiftEnterBytes))
	case key.Matches(msg, a.keys.CtrlEnter):
		v.session.SendBytes([]byte(ctrlEnterBytes))
	case printableText(msg):
		// The characters the key stands for, sent as themselves. The emulator's
		// own encoder writes a printable key only when it carries no modifier
		// at all, so a capital letter — shift plus the unshifted code — and
		// every piece of shifted punctuation reached the agent as nothing.
		v.session.SendBytes([]byte(msg.Text))
	default:
		v.session.SendKey(uv.KeyPressEvent(msg))
	}
	return nil
}

// printableText reports whether the key is text to be typed rather than a
// keystroke to be encoded. A key that stands for characters carries them in
// Text; the arrows, the function keys and the rest carry none, and are the
// emulator's to encode. Ctrl is excluded because a ctrl combination is a
// control byte however it was decoded, never the letter it was struck with.
func printableText(msg tea.KeyPressMsg) bool {
	return msg.Text != "" && msg.Mod&tea.ModCtrl == 0
}

// viewerMouse is every mouse event while a terminal is on the board. Until the
// join-pane split was retired the outer tmux delivered clicks straight to the
// agent's own pane; nat draws the rectangle itself now, so it carries the mouse
// there too.
//
// An event inside the box goes to the agent at the cell it landed on, with
// whatever modifiers were held as it happened — the tmux on the far end binds
// ctrl+click as well as click, and a click stripped of its ctrl is a different
// gesture. A press there also hands the keyboard over, so tab is not the only
// way to type at an agent. An event anywhere else is not the terminal's, and it
// says so — a press there hands the keyboard back on its way past, and the
// board takes the event from [App.mouseEvent].
func (a *App) viewerMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !a.viewerVisible() {
		return nil, false
	}
	kind, ok := mouseKind(msg)
	if !ok {
		return nil, false
	}
	m := msg.Mouse()
	x, y, inside := a.termCell(m.X, m.Y)
	if !inside {
		if kind == vterm.MousePress {
			a.viewer.focused = false
		}
		return nil, false
	}
	if kind == vterm.MousePress {
		a.viewer.focused = true
	}
	a.viewer.session.SendMouse(x, y, m.Button, m.Mod, kind)
	return nil, true
}

// mouseKind is the session's name for the kind of event a message is, and
// whether it is one the terminal takes at all.
func mouseKind(msg tea.MouseMsg) (vterm.MouseKind, bool) {
	switch msg.(type) {
	case tea.MouseClickMsg:
		return vterm.MousePress, true
	case tea.MouseReleaseMsg:
		return vterm.MouseRelease, true
	case tea.MouseMotionMsg:
		return vterm.MouseMotion, true
	case tea.MouseWheelMsg:
		return vterm.MouseWheel, true
	default:
		return 0, false
	}
}

// termCell turns a cell of the window into a cell of the terminal's screen, and
// reports whether the event landed on that screen at all. It is the inverse of
// what [App.viewerCursor] does with the cursor: the frame starts a column in
// from the box's left edge, which the split puts at the board's width, and a
// line down from the title, in a body band that starts under the header.
func (a *App) termCell(mx, my int) (x, y int, ok bool) {
	board, term := a.splitWidths()
	x, y = mx-board-1, my-a.headerBandHeight()-1
	// The interior of the box as it is drawn, which the emulator is never
	// smaller than: a window too small for [minTermCols] rows and columns
	// leaves the child a screen wider than the box showing it.
	cols, rows := term-2, a.bodyBoxHeight()-2
	if x < 0 || y < 0 || x >= cols || y >= rows {
		return 0, 0, false
	}
	return x, y, true
}

// focusViewer hands the keyboard to the terminal. There is nothing to type
// into when the terminal is not on screen.
func (a *App) focusViewer() tea.Cmd {
	if !a.viewerVisible() {
		return nil
	}
	a.viewer.focused = true
	return nil
}

// fullAttachFlow hands the whole terminal to the agent's session, for the times
// the split is not enough room. It is the old way of viewing an agent, kept as
// a hatch: unlike the viewer it takes the board off screen until the user
// detaches.
func (a *App) fullAttachFlow() tea.Cmd {
	if a.launcher == nil || a.busy {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to attach to its agent.", sevWarning)
	}
	session := a.live[s.ID]
	if session == "" {
		return a.showConfirm(fmt.Sprintf("No agent session is running for %q.", s.Name), sevWarning)
	}
	a.busy, a.note = true, ""
	return attach(a.launcher, s.ID, session)
}

// splitWidths shares the window's columns between the board's box and the
// terminal's, at the same percentage the join-pane split used. Neither box goes
// below a border and a column of content: a split with nothing in one side is
// worse than a cramped one.
func (a *App) splitWidths() (board, term int) {
	term = max(a.width*a.cfg.SplitPercent()/100, minBoxWidth)
	board = max(a.width-term, minBoxWidth)
	return board, term
}

// boardWidth is the columns the board itself draws in: the body band, less
// whatever the terminal beside it has taken.
func (a *App) boardWidth() int {
	if !a.viewerVisible() {
		return a.innerWidth()
	}
	board, _ := a.splitWidths()
	return max(board-a.styles.Box.GetHorizontalFrameSize(), 0)
}

// termSize is the terminal's own size in cells: the interior of its box, which
// is what the child is told and what its output is interpreted at. Before the
// first resize there is no box to measure, so it starts at the size a terminal
// is assumed to be and is corrected by the resize that follows.
func (a *App) termSize() (cols, rows int) {
	if a.width <= 0 || a.height <= 0 {
		return initTermCols, initTermRows
	}
	_, term := a.splitWidths()
	// The box's border costs a column each side; the title line and the bottom
	// border a line each.
	return max(term-2, minTermCols), max(a.bodyBoxHeight()-2, minTermRows)
}

// resizeTerm tells the session the size its box now is, so the winch reaches
// the hidden client and tmux resizes the session inside it. A failure is logged
// rather than raised: the frame is still drawn, at the size it was.
func (a *App) resizeTerm() {
	if a.viewer == nil {
		return
	}
	cols, rows := a.termSize()
	if err := a.viewer.session.Resize(cols, rows); err != nil {
		logging.Error("could not resize the agent terminal", "error", err)
	}
}

// viewerRegion is the terminal's box: a hand-built title line — lipgloss has no
// border-title API — over the frame in a box drawn without its top border, so
// the two read as one panel.
func (a *App) viewerRegion(width, height int) []string {
	style, edge := a.styles.TermBox, a.styles.TermEdge
	if a.viewer.focused {
		style, edge = a.styles.TermBoxFocused, a.styles.TermEdgeFocused
	}
	lines := []string{a.viewerTitle(edge, width)}
	if height > 1 {
		box := style.BorderTop(false).Width(width).Height(height - 1).
			Render(a.viewerContent(width-2, height-2))
		lines = append(lines, strings.Split(box, "\n")...)
	}
	// Padded out and cut back, so the region is exactly the lines it was given
	// however the box came out.
	lines = append(lines, make([]string, max(height-len(lines), 0))...)
	return fitLines(lines[:max(height, 0)], width)
}

// viewerTitle is the box's top border with the agent's name let into it, and
// beside it the mark that says an agent is running. There is no other state to
// draw: a
// terminal on the board is one with an agent behind it, since the box goes as
// soon as the agent does.
func (a *App) viewerTitle(edge lipgloss.Style, width int) string {
	border := lipgloss.RoundedBorder()
	mark := a.styles.Live.Render("●")
	head := edge.Render(border.TopLeft+border.Top+" ") +
		a.styles.Selected.Render(a.viewer.name) + " " + mark + " "
	fill := max(width-lipgloss.Width(head)-lipgloss.Width(border.TopRight), 0)
	return head + edge.Render(strings.Repeat(border.Top, fill)+border.TopRight)
}

// viewerContent is the cached frame cut to the box's interior. The emulator is
// resized with the box, so this ordinarily changes nothing — it is what keeps a
// frame rendered at the old size from pushing the border out while the resize is
// still on its way to the child.
func (a *App) viewerContent(width, height int) string {
	lines := strings.Split(a.viewer.frame, "\n")
	if len(lines) > height {
		lines = lines[:max(height, 0)]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(fitLines(lines, width), "\n")
}

// fitLines cuts every line to width and pads it back out to exactly that, so a
// region can be laid beside another line for line.
func fitLines(lines []string, width int) []string {
	// Cut before padding out: a Width alone wraps what overruns onto a line the
	// region has no room for, which is how a long agent name would push the
	// bands below it off the window.
	fill := lipgloss.NewStyle().Width(max(width, 0))
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = fill.Render(fit(line, width))
	}
	return out
}

// viewerCursor is where the terminal's cursor is on the window, and whether it
// should be shown at all: only while the terminal has the keyboard, and only
// while the child has it shown.
func (a *App) viewerCursor() (x, y int, ok bool) {
	if !a.viewerVisible() || !a.viewer.focused || !a.viewer.cursorShown {
		return 0, 0, false
	}
	board, _ := a.splitWidths()
	// The frame starts a column in from the box's left edge, and a line down
	// from the title, inside a body band that starts under the header.
	return board + 1 + a.viewer.cursorX, a.headerBandHeight() + 1 + a.viewer.cursorY, true
}

// viewerHints are the hints row while a terminal is beside the board: how the
// keyboard is handed over and back, how the terminal is dismissed, and the
// hatch to the full-screen attach. A focused terminal has every key but one, so
// that one is all there is to name.
//
// Both ways over the line name the mouse beside the key, since a click inside
// the terminal takes the keyboard and a click on the board hands it back.
func (a *App) viewerHints() []hint {
	closeKey := a.board.keys.Attach
	if a.viewer.sliceID == agent.PlanSentinel {
		closeKey = a.board.keys.Plan
	}
	if a.viewer.focused {
		return []hint{{orClick(a.keys.Unfocus), 1}}
	}
	return []hint{
		{orClick(a.board.keys.Focus), 3},
		{shortHint(closeKey, "hide the agent"), 2},
		{a.board.keys.FullAttach, 1},
	}
}

// orClick is a hint for something a click does as well as a key: the binding's
// own words, over a key the click is named in.
func orClick(b key.Binding) key.Binding {
	return key.NewBinding(key.WithHelp(b.Help().Key+"/click", b.Help().Desc))
}
