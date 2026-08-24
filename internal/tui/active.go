package tui

import (
	"image/color"
	"maps"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// The Active section is a panel of the body band in its own right: the slices
// in flight, drawn as a boxed vertical list above the plan's own box rather
// than as rows inside it. What is being worked on is what the board is read
// for, and in the plan it is scattered through the milestones it happens to be
// filed under — here it is one short list, in one place, whatever the plan's
// shape.
//
// The two panels are siblings, framed the way the header and the body already
// are, so no border sits inside another. They are still one board underneath:
// the entries are rows of it like any other, first of them all, so the cursor
// runs from the section straight on into the plan and every key that acts on a
// slice acts on the entry under it.
//
// activeTitle is the heading let into the panel's top border, and activeDot the
// mark each entry leads with: one cell, coloured by the state the entry is in,
// which is what a glance at the section reads.
const (
	activeTitle = "Active"
	activeDot   = "●"
)

// activeEntryLines is how many lines one entry takes: the name, and the state
// under it. Every entry is that tall — a body too wide for the panel is cut
// rather than wrapped — which is what lets a line of the panel be turned back
// into the entry drawn on it by dividing.
const activeEntryLines = 2

// activeSlices is the plan's slices in flight, in the order the board draws
// their milestones. In flight is the classifier's own answer — a slice
// [domain.StateOf] says nothing about is one there is nothing to say about —
// which is every slice in progress, and a Done slice for exactly as long as gh
// says its pull request is still open. Nothing else: a Todo slice has not
// started, a status this build does not know is neither, and a Done slice
// whose pull request has landed, or which nothing has been read of at all, is
// finished as far as the board is concerned.
func (b Board) activeSlices() []domain.Slice {
	var active []domain.Slice
	for _, g := range b.groups {
		for _, s := range g.Slices {
			if b.state(s) != domain.SliceStateNone {
				active = append(active, s)
			}
		}
	}
	return active
}

// state is where a slice has got to, as the section names it: the domain's own
// classification, given what the board's two agent readings say about the
// slice, what gh last said about its pull request, and the plan its
// dependencies are read from.
func (b Board) state(s domain.Slice) domain.SliceState {
	return domain.StateOf(s, b.agentPresence(s.ID), b.prState[s.ID], b.byID)
}

// SetPRState records how ready each pull request read as still open is, keyed
// by slice ID. It is the board's second background reading — see
// [App.refreshPRStates] — and, like the activity watcher's, a slice it says
// nothing about simply keeps the state it would have had before there was any
// reading at all.
//
// Unlike the other readings it can change which slices the section holds at
// all, since a Done slice is in it for exactly as long as its pull request is
// open. So the rows are rebuilt — but only by a reading that says something
// new, because most of them say what the last one did and a rebuild the user
// cannot see is one the cursor pays for.
func (b *Board) SetPRState(state map[string]domain.PRReadiness) {
	if maps.Equal(b.prState, state) {
		return
	}
	was, wasSlice := b.cursorRow()
	b.prState = state
	b.rebuild()
	b.restoreCursor(was, wasSlice)
}

// cursorRow is the row the cursor is on and, for an entry of the Active
// section, the ID of the slice it draws. The row addresses the section by
// position, and a position is exactly what a rebuild moves — the entries above
// an entry are the ones that come and go.
func (b Board) cursorRow() (row, string) {
	if b.cursor >= len(b.rows) {
		return row{}, ""
	}
	r := b.rows[b.cursor]
	if r.kind == rowActive {
		return r, b.active[r.slice].ID
	}
	return r, ""
}

// restoreCursor puts the cursor back on the row [Board.cursorRow] found, after
// a rebuild that may have moved it: the slice it was on for an entry of the
// section, and the row itself for everything else. A slice that has left the
// section — its pull request has landed — leaves the cursor where the entry was
// instead, which is the same place the plain rebuild's clamp would have left it.
func (b *Board) restoreCursor(was row, sliceID string) {
	if sliceID != "" && b.cursorTo(func(r row) bool {
		return r.kind == rowActive && b.active[r.slice].ID == sliceID
	}) {
		return
	}
	b.cursorTo(func(r row) bool { return r == was })
}

// agentPresence is the board's two readings of a slice's agent — the live map
// of running sessions and the activity watcher's classification of them — as
// the one value the domain rule takes. Liveness comes first for the reason
// [Board.presence] puts it first: an agent that has gone has no state left,
// whatever was last read of it.
func (b Board) agentPresence(sliceID string) domain.AgentPresence {
	p, live := b.presence(sliceID)
	switch {
	case !live:
		return domain.AgentNone
	case p == PresenceWorking:
		return domain.AgentWorking
	case p == PresenceWaiting:
		return domain.AgentWaiting
	}
	return domain.AgentUnknown
}

// stateStyle is the colour a state is drawn in: the dot, and the state word on
// the entry's second line. The roles are the ones the rest of the board already
// reads these states in — the Working orange of a star at work, the pending
// yellow of one stopped for input, the muted text of a blocked row, and the
// Success green of work waiting to be reviewed. A slice in progress with
// nothing out yet takes AccentAlt: it is neither finished nor gone wrong —
// often it is simply an agent running outside nat's own sessions — so it reads
// as ordinary work rather than as something to put right. A pull request the
// review is over on takes that same Success green in bold: it is the end of the
// state the entry was already in, said louder rather than said in a new colour.
func (b Board) stateStyle(s domain.SliceState) lipgloss.Style {
	switch s {
	case domain.SliceStateWorking:
		return b.styles.StateWorking
	case domain.SliceStateWaiting:
		return b.styles.StateWaiting
	case domain.SliceStateBlocked:
		return b.styles.StateBlocked
	case domain.SliceStateAwaitingReview:
		return b.styles.StateAwaitingReview
	case domain.SliceStateReadyToMerge:
		return b.styles.StateReadyToMerge
	}
	return b.styles.StateReadyToPush
}

// SelectedActive is the slice under the cursor when the cursor is in the Active
// section. It is a slice of the plan like any other — the section is a second
// place the same page is drawn — so the keys that act on a slice act on this
// one; see [Board.SelectedSlice].
func (b Board) SelectedActive() (domain.Slice, bool) {
	if b.cursor >= len(b.rows) {
		return domain.Slice{}, false
	}
	r := b.rows[b.cursor]
	if r.kind != rowActive {
		return domain.Slice{}, false
	}
	return b.active[r.slice], true
}

// ActiveCount is how many slices are in flight, which is what the layout sizes
// the panel from, and ActiveHeight the lines their entries take. Both answer
// off the plan alone rather than off the rows: whether the panel is drawn is
// what the layout is deciding when it asks.
func (b Board) ActiveCount() int  { return len(b.active) }
func (b Board) ActiveHeight() int { return activeEntryLines * len(b.active) }

// SetShowActive says whether the section is drawn at all, which is the layout's
// answer and not the board's: a body band with no room for a second panel draws
// none. The section's rows come off the board with it, so the cursor can never
// be left on an entry nothing draws — a plan with the section hidden behaves
// exactly as one with nothing in flight.
func (b *Board) SetShowActive(show bool) {
	if b.showActive == show {
		return
	}
	b.showActive = show
	b.rebuild()
}

// activeRowCount is how many of the board's rows are the section's. They are
// always the first of them, which is what lets the plan's rows be addressed by
// the same index either way: see [Board.rowLines].
func (b Board) activeRowCount() int {
	if !b.showActive {
		return 0
	}
	return len(b.active)
}

// ActiveLines is the panel's interior: every entry as the two lines it is drawn
// on, run out to the board's width. The panel's own frame is the layout's —
// see [App.activeRegion] — so there are no edges here, only what goes between
// them.
func (b Board) ActiveLines() []string {
	var lines []string
	for i := range b.activeRowCount() {
		lines = append(lines, b.renderActive(i)...)
	}
	return lines
}

// ActiveCursorSpan is where the entry under the cursor sits in those lines: the
// line it starts on, and how many it takes. A cursor down in the plan is in the
// other panel entirely, and has nothing here to bring on screen — which is what
// a height of zero says.
func (b Board) ActiveCursorSpan() (top, height int) {
	if b.cursor >= b.activeRowCount() {
		return 0, 0
	}
	return b.cursor * activeEntryLines, activeEntryLines
}

// ActiveRowAtLine is the board row drawn on a line of the panel, counted from
// its first line, and whether that line is an entry's at all: the mouse's way
// back from a line of the panel to the row it points at.
func (b Board) ActiveRowAtLine(line int) (int, bool) {
	if line < 0 || line >= activeEntryLines*b.activeRowCount() {
		return 0, false
	}
	return line / activeEntryLines, true
}

// renderActive draws one entry of the section: a state dot and the slice's
// name, then a muted line naming the state and the milestone the slice is filed
// under. Two lines rather than one because the name is what the entry is read
// by and the state is what it is scanned for, and neither should give way to
// the other on a narrow board.
//
// The entry is addressed by its place in the section's own list, which is also
// its row on the board — the entries are the first rows there are — so the
// cursor picks the entry out by the same index.
func (b Board) renderActive(i int) []string {
	s := b.active[i]
	var fill color.Color
	if i == b.cursor {
		fill = b.styles.ActiveFill
	}
	state := b.state(s)
	st := b.stateStyle(state)

	head := wash(st, fill).Render(activeDot) + wash(b.styles.ActiveName, fill).Render(" "+s.Name)
	// The state line is indented under the name, past the dot's own cell, and
	// that indent carries the fill like everything else on the line: a space
	// drawn plain would cut a hole in a selected entry's highlight.
	foot := wash(lipgloss.NewStyle(), fill).Render("  ") +
		wash(st, fill).Render(state.String()) +
		wash(b.styles.Faint, fill).Render(" · "+b.groupTitleOf(s))
	return []string{b.activeRow(fill, head), b.activeRow(fill, foot)}
}

// activeRow is one line of an entry: the body run out to the board's width with
// the entry's own fill, so a selected entry's highlight is the width of the
// panel rather than of its text. A body too wide is cut there — the section is
// a list of what is in flight, and a name that wraps would cost the entry below
// it the line it is read on. An unmeasured board has no width to run out to, so
// the line is what it says and nothing more.
func (b Board) activeRow(fill color.Color, body string) string {
	if b.width <= 0 {
		return body
	}
	line := fit(body, b.width)
	if n := b.width - lipgloss.Width(line); n > 0 {
		line += wash(lipgloss.NewStyle(), fill).Render(strings.Repeat(" ", n))
	}
	return line
}

// The panel's frame, in lines: the title line let into its top border and the
// bottom border under it when the window is framed, and the bare heading alone
// when it is not — below the framed threshold the section follows every other
// band and draws bare.
const (
	activeBoxFrame  = 2
	activeBareFrame = 1
)

// activeFrame is what the section costs beyond its entries, given how the
// window is drawn.
func (a *App) activeFrame() int {
	if a.framed() {
		return activeBoxFrame
	}
	return activeBareFrame
}

// activeFits reports whether the body band has room for a panel of its own:
// enough lines for the section's frame and an entry line, with the plan left
// the least a band of it is worth drawing in. It is what [Board.SetShowActive]
// is told, so a band too short simply has no section — and the board's rows
// say so too, rather than leaving the cursor on an entry nothing draws.
//
// An unmeasured window has no lines to share out and draws its bands one after
// another at whatever size they come out, so the section is drawn there like
// everything else.
func (a *App) activeFits() bool {
	if a.board.ActiveCount() == 0 {
		return false
	}
	if a.width <= 0 || a.height <= 0 {
		return true
	}
	keep := bodyBoxMin
	if !a.framed() {
		keep = 1
	}
	return a.bodyBoxHeight()-keep >= a.activeFrame()+1
}

// activeVisible reports whether the section is drawn: there is work in flight,
// a plan to draw it from, room in the band for a panel, and the board is the
// screen on show. Help, info, the diff and a form each take the whole band —
// they are what the user is looking at, and the plan behind them is not.
func (a *App) activeVisible() bool {
	return a.project != nil && a.screen == screenBoard && a.activeFits()
}

// activeBandHeight is the lines the section takes of the body band, frame
// included: as many as its entries need, and never more than the band can give
// while the plan keeps its own. Nothing at all is a band with no section in it,
// which is the body band exactly as it was before there was one.
func (a *App) activeBandHeight() int {
	if !a.activeVisible() || a.width <= 0 || a.height <= 0 {
		return 0
	}
	keep := bodyBoxMin
	if !a.framed() {
		keep = 1
	}
	return min(a.board.ActiveHeight()+a.activeFrame(), a.bodyBoxHeight()-keep)
}

// activeHeight is the lines inside that band the entries themselves are drawn
// on. An unmeasured window has no band to measure, and every entry is drawn.
func (a *App) activeHeight() int {
	if h := a.activeBandHeight(); h > 0 {
		return max(h-a.activeFrame(), 0)
	}
	return 0
}

// activeRegion is the panel: a hand-built title line — lipgloss has no
// border-title API — over the layout's own box drawn without its top border, so
// the two read as one panel and the plan's box below it as its sibling. It is
// shaped like the agent terminal's own region, and for the same reason.
func (a *App) activeRegion(width, height int) []string {
	lines := []string{a.activeTitleLine(width)}
	if height > 1 {
		interior := max(width-a.styles.Box.GetHorizontalFrameSize(), 0)
		box := a.styles.Box.BorderTop(false).Width(width).Height(height - 1).
			Render(clipLines(fit(a.activeView(), interior), max(height-2, 0)))
		lines = append(lines, strings.Split(box, "\n")...)
	}
	// Padded out and cut back, so the region is exactly the lines it was given
	// however the box came out.
	lines = append(lines, make([]string, max(height-len(lines), 0))...)
	return fitLines(lines[:max(height, 0)], width)
}

// activeTitleLine is the panel's top border with the section's heading let into
// it, drawn in the frame's own colour so it closes the box the layout draws the
// rest of.
func (a *App) activeTitleLine(width int) string {
	border := lipgloss.RoundedBorder()
	edge := a.styles.ActiveEdge
	head := edge.Render(border.TopLeft+border.Top+" ") + a.styles.ActiveTitle.Render(activeTitle) + " "
	fill := max(width-lipgloss.Width(head)-lipgloss.Width(border.TopRight), 0)
	return fit(head+edge.Render(strings.Repeat(border.Top, fill)+border.TopRight), width)
}

// activeBandView is the section as a bare band: its heading on a line of its
// own, where the panel has one let into its border, and the entries under it.
// It is what a window below the framed threshold draws, and what an unmeasured
// one does.
func (a *App) activeBandView() string {
	return a.styles.ActiveTitle.Render(activeTitle) + "\n" + a.activeView()
}

// activeView is the entries as the panel shows them: the window of lines the
// band has room for, scrolled to keep the entry under the cursor in it. A band
// with no measured height shows every entry, since there is nothing to scroll
// in.
func (a *App) activeView() string {
	lines := a.board.ActiveLines()
	if h := a.activeHeight(); h > 0 && h < len(lines) {
		top := min(a.activeOffset, len(lines)-h)
		lines = lines[top : top+h]
	}
	return strings.Join(lines, "\n")
}

// syncActive scrolls the panel the least it can to bring the entry under the
// cursor into it, the way [App.syncBoard] scrolls the plan. The two panels
// scroll independently: a cursor in one has nothing to say about where the
// other is.
func (a *App) syncActive() {
	h := a.activeHeight()
	if h <= 0 {
		a.activeOffset = 0
		return
	}
	if top, rows := a.board.ActiveCursorSpan(); rows > 0 {
		switch {
		case top < a.activeOffset:
			a.activeOffset = top
		case top+rows > a.activeOffset+h:
			a.activeOffset = top + rows - h
		}
	}
	a.activeOffset = max(0, min(a.activeOffset, a.board.ActiveHeight()-h))
}
