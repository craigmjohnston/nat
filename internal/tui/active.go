package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// The Active section is the board's first band: the slices in flight, drawn as
// a boxed vertical list above the plan rather than as rows of it. What is being
// worked on is what the board is read for, and in the plan it is scattered
// through the milestones it happens to be filed under — here it is one short
// list, in one place, whatever the plan's shape.
//
// activeTitle is the heading let into the box's top border, and activeDot the
// mark each entry leads with: one cell, coloured by the state the entry is in,
// which is what a glance at the section reads.
const (
	activeTitle = "Active"
	activeDot   = "●"
)

// activeSlices is the plan's slices in flight, in the order the board draws
// their milestones. In flight is the classifier's own answer — a slice
// [domain.StateOf] says nothing about is one there is nothing to say about —
// which is every slice in progress and nothing else: a Todo slice has not
// started, a Done one has finished, and a status this build does not know is
// neither.
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

// SetPRState records how ready each read pull request is, keyed by slice ID.
// It is the board's second background reading — see [App.refreshPRStates] —
// and, like the activity watcher's, a slice it says nothing about simply keeps
// the state it would have had before there was any reading at all.
func (b *Board) SetPRState(state map[string]domain.PRReadiness) { b.prState = state }

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

// renderActive draws one entry of the section, with the box's own edges around
// it: a state dot and the slice's name, then a muted line naming the state and
// the milestone the slice is filed under. Two lines rather than one because the
// name is what the entry is read by and the state is what it is scanned for,
// and neither should give way to the other on a narrow board.
//
// The box's top border belongs to the first entry and its bottom border to the
// last, along with the blank line that sets the plan apart from it — a line of
// the board that is no row's is a line the cursor and the mouse cannot account
// for, which is the rule the Done section's own blank line follows.
func (b Board) renderActive(i int, r row) []string {
	s := b.active[r.slice]
	selected := i == b.cursor
	var fill color.Color
	if selected {
		fill = b.styles.ActiveFill
	}
	width := b.activeWidth()
	state := b.state(s)
	st := b.stateStyle(state)

	head := wash(st, fill).Render(activeDot) + wash(b.styles.ActiveName, fill).Render(" "+s.Name)
	// The state line is indented under the name, past the dot's own cell, and
	// that indent carries the fill like everything else on the line: a space
	// drawn plain would cut a hole in a selected entry's highlight.
	foot := wash(lipgloss.NewStyle(), fill).Render("  ") +
		wash(st, fill).Render(state.String()) +
		wash(b.styles.Faint, fill).Render(" · "+b.groupTitleOf(s))
	lines := []string{
		b.activeRow(width, fill, head),
		b.activeRow(width, fill, foot),
	}
	if r.slice == 0 {
		lines = append([]string{b.activeTop(width)}, lines...)
	}
	if r.slice == len(b.active)-1 {
		lines = append(lines, b.activeBottom(width), "")
	}
	return lines
}

// activeRow is one line of an entry inside the box: the body between the box's
// edges, held off them by a space either side and run out to the interior with
// the entry's own fill, so a selected entry's highlight is the width of the
// section rather than of its text. A body too wide for the interior is cut
// there — the section is a list of what is in flight, and a name that wraps
// would cost the entry below it the line it is read on. A box too narrow for
// any body at all is its own two edges, and one narrower still is cut to the
// board's width like every other line of it.
func (b Board) activeRow(width int, fill color.Color, body string) string {
	pad := wash(lipgloss.NewStyle(), fill)
	interior := max(width-2, 0)
	line := ""
	if room := interior - 2; room > 0 {
		line = pad.Render(" ") + fit(body, room)
	}
	if n := interior - lipgloss.Width(line); n > 0 {
		line += pad.Render(strings.Repeat(" ", n))
	}
	border := lipgloss.RoundedBorder()
	edge := b.styles.ActiveEdge
	return fit(edge.Render(border.Left)+line+edge.Render(border.Right), width)
}

// activeTop is the box's top border with the section's heading let into it,
// shaped like the agent terminal's own title line — lipgloss has no
// border-title API — and activeBottom the plain border that closes it.
func (b Board) activeTop(width int) string {
	border := lipgloss.RoundedBorder()
	edge := b.styles.ActiveEdge
	head := edge.Render(border.TopLeft+border.Top+" ") + b.styles.ActiveTitle.Render(activeTitle) + " "
	fill := max(width-lipgloss.Width(head)-lipgloss.Width(border.TopRight), 0)
	return fit(head+edge.Render(strings.Repeat(border.Top, fill)+border.TopRight), width)
}

func (b Board) activeBottom(width int) string {
	border := lipgloss.RoundedBorder()
	fill := max(width-lipgloss.Width(border.BottomLeft)-lipgloss.Width(border.BottomRight), 0)
	return fit(b.styles.ActiveEdge.Render(
		border.BottomLeft+strings.Repeat(border.Bottom, fill)+border.BottomRight), width)
}

// activeWidth is how wide the section is drawn: the board's own width, so it
// squares off with the plan under it. An unmeasured board has none to take, and
// the box is sized to the widest thing in it instead — the heading, or an
// entry's own lines — rather than drawn ragged.
func (b Board) activeWidth() int {
	if b.width > 0 {
		return b.width
	}
	// Two border cells, and a space either side of the body.
	const frame = 4
	width := lipgloss.Width(activeTitle) + frame
	for _, s := range b.active {
		width = max(width, lipgloss.Width(activeDot+" "+s.Name)+frame)
		width = max(width, lipgloss.Width("  "+b.state(s).String()+" · "+b.groupTitleOf(s))+frame)
	}
	return width
}
