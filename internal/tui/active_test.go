package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// activeProject is a plan with a slice in every state the section draws: one
// with an agent at work on it, one whose agent has stopped for input, one
// waiting on a dependency, one in progress with nothing out yet, and one handed
// back on a branch. The Todo slice the blocked one waits on is what keeps it
// blocked, and is itself no entry of the section.
func activeProject() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Config", "M2: Board"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: "s1", Name: "Working", Status: domain.SliceClaimed, MilestoneID: "M1: Config"},
			{ID: "s2", Name: "Waiting", Status: domain.SliceClaimed, MilestoneID: "M1: Config"},
			{ID: "s3", Name: "Blocked", Status: domain.SliceClaimed, MilestoneID: "M2: Board",
				DependsOn: []string{"s6"}},
			{ID: "s4", Name: "Ready to push", Status: domain.SliceClaimed, MilestoneID: "M2: Board"},
			{ID: "s5", Name: "Awaiting review", Status: domain.SliceClaimed,
				MilestoneID: "M2: Board", Branch: "slice/awaiting-review"},
			{ID: "s6", Name: "Not started", Status: domain.SliceTodo, MilestoneID: "M2: Board"},
		})
}

// activeBoard draws that plan with an agent working the first slice and one
// stopped for input on the second.
func activeBoard(width int) *Board {
	b := NewBoard(DefaultStyles())
	b.SetWidth(width)
	b.SetLive(map[string]string{"s1": agent.SessionName("s1"), "s2": agent.SessionName("s2")})
	b.SetActivity(map[string]Presence{"s1": PresenceWorking, "s2": PresenceWaiting})
	p := activeProject()
	b.SetProject(&p)
	return &b
}

// TestBoardDrawsAnActiveEntryForEveryState pins the section: every slice in
// flight is an entry of it, in the order the plan draws their milestones, and
// each one names the state it is in beside the milestone it is filed under.
func TestBoardDrawsAnActiveEntryForEveryState(t *testing.T) {
	b := activeBoard(60)

	golden(t, "board-active", b.View())

	lines := strings.Split(ansi.ReplaceAllString(b.View(), ""), "\n")
	for i, want := range []string{
		"╭─ Active ",
		"● Working", "  working · Config",
		"● Waiting", "  waiting · Config",
		"● Blocked", "  blocked · Board",
		"● Ready to push", "  ready to push · Board",
		"● Awaiting review", "  awaiting review · Board",
		"╰─",
	} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %q on it", i, lines[i], want)
		}
	}
	// The Todo slice the blocked one waits on is not in flight, so the section
	// says nothing about it.
	if strings.Contains(strings.Join(lines[:12], "\n"), "Not started") {
		t.Errorf("the section drew a slice that has not started:\n%s", b.View())
	}
}

// The dot and the state word take the state's own colour, which is what the
// section is read at a glance by.
func TestBoardColoursAnActiveEntryByItsState(t *testing.T) {
	b := activeBoard(60)
	// Off the section: a selected entry is drawn over the fill, which is a
	// colour of its own on every part of it.
	b.SelectRow(len(b.rows) - 1)
	view := b.View()

	for state, style := range map[domain.SliceState]lipgloss.Style{
		domain.SliceStateWorking:        b.styles.StateWorking,
		domain.SliceStateWaiting:        b.styles.StateWaiting,
		domain.SliceStateBlocked:        b.styles.StateBlocked,
		domain.SliceStateReadyToPush:    b.styles.StateReadyToPush,
		domain.SliceStateAwaitingReview: b.styles.StateAwaitingReview,
	} {
		if !strings.Contains(view, style.Render(state.String())) {
			t.Errorf("%v is not drawn in its own colour:\n%s", state, view)
		}
	}
}

// A plan with nothing in flight draws no section at all — not a heading, not a
// box, not a blank line — so a board with no work under way reads exactly as it
// did before there was one.
func TestBoardWithNothingInFlightDrawsNoActiveSection(t *testing.T) {
	b := NewBoard(DefaultStyles())
	b.SetWidth(60)
	p := domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Config"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: "s1", Name: "Not started", Status: domain.SliceTodo, MilestoneID: "M1: Config"},
			{ID: "s2", Name: "Finished", Status: domain.SliceDone, MilestoneID: "M1: Config"},
		})
	b.SetProject(&p)

	golden(t, "board-active-empty", b.View())
	if len(b.active) != 0 {
		t.Errorf("active = %+v, want nothing in flight", b.active)
	}
	// The heading is looked for in the border it is let into: "Active" on its
	// own is also what a milestone in flight is badged with.
	if got := b.View(); strings.Contains(got, "╭─ ") || strings.Contains(got, activeDot) {
		t.Errorf("a plan with nothing in flight drew a section:\n%s", got)
	}
	if got := b.rows[0].kind; got != rowMilestone {
		t.Errorf("first row is kind %v, want the plan's own first row", got)
	}
}

// A status this build does not know is in no state the section can name, so it
// is left to the plan below rather than drawn as work in flight.
func TestBoardLeavesAnUnknownStatusOutOfTheActiveSection(t *testing.T) {
	b := newTestBoard()

	for _, s := range b.active {
		if s.Name == "Stray" {
			t.Errorf("active = %+v, want the unknown status left out", b.active)
		}
	}
}

// The cursor runs from the section straight on into the plan: the entries are
// rows of the same board, so j walks out of the box and k walks back into it.
func TestBoardCursorMovesThroughTheActiveSectionAndIntoThePlan(t *testing.T) {
	b := activeBoard(60)

	var names []string
	for range len(b.active) {
		s, ok := b.SelectedActive()
		if !ok {
			t.Fatalf("row %d is not an entry of the section", b.cursor)
		}
		names = append(names, s.Name)
		b.Update(keyPress("j"))
	}
	want := []string{"Working", "Waiting", "Blocked", "Ready to push", "Awaiting review"}
	if !equal(names, want) {
		t.Errorf("the cursor walked %q, want %q", names, want)
	}
	// One more step off the last entry is into the plan itself.
	if r := b.rows[b.cursor]; r.kind != rowMilestone {
		t.Errorf("the row under the cursor is kind %v, want the plan's first milestone", r.kind)
	}

	b.Update(keyPress("k"))
	if s, ok := b.SelectedActive(); !ok || s.Name != "Awaiting review" {
		t.Errorf("selected = %+v (ok=%v), want back into the section", s, ok)
	}
}

// An entry is the slice it names, so every key that acts on a slice acts on the
// one under the cursor here — and no key folds it, since there is nothing in an
// entry to fold.
func TestBoardActiveEntryIsASlice(t *testing.T) {
	b := activeBoard(60)

	got, ok := b.SelectedSlice()
	if !ok || got.ID != "s1" {
		t.Errorf("selected = %+v (ok=%v), want the entry's own slice", got, ok)
	}
	if _, ok := b.SelectedMilestone(); ok {
		t.Error("an entry of the section is a slice, not a milestone")
	}

	rows := len(b.rows)
	b.Update(keyPress("enter"))
	if b.cursor != 0 || len(b.rows) != rows {
		t.Errorf("enter changed the board: cursor = %d, rows = %d", b.cursor, len(b.rows))
	}
}

// The selected entry is drawn on a fill run out to the section's own width, and
// every part of it keeps its colour over that fill rather than being flattened
// by it: the dot is what the entry is read by.
func TestBoardFillsTheSelectedActiveEntry(t *testing.T) {
	b := activeBoard(60)

	lines := b.rowLines()[0]
	// The box's top border, the entry's two lines.
	if len(lines) != 3 {
		t.Fatalf("the first entry is %d lines, want the top border and its own two", len(lines))
	}
	fill := backgroundOf(lipgloss.NewStyle().Background(b.styles.ActiveFill))
	for _, line := range lines[1:] {
		if !strings.Contains(line, fill) {
			t.Errorf("a selected entry's line is unfilled:\n%q", line)
		}
		if got := lipgloss.Width(line); got != 60 {
			t.Errorf("line %q is %d wide, want the section run out to 60", line, got)
		}
	}
	if !strings.Contains(lines[1], wash(b.styles.StateWorking, b.styles.ActiveFill).Render(activeDot)) {
		t.Errorf("the dot lost its state colour to the fill:\n%q", lines[1])
	}

	// An entry the cursor is not on carries no fill at all.
	if strings.Contains(strings.Join(b.rowLines()[1], "\n"), fill) {
		t.Errorf("an unselected entry is filled:\n%q", b.rowLines()[1])
	}
}

// The section closes with its bottom border and a blank line, and both belong
// to the last entry's own row — a line of the board that is no row's is one the
// cursor and the mouse cannot account for.
func TestBoardActiveSectionClosesWithTheLastEntry(t *testing.T) {
	b := activeBoard(60)
	last := len(b.active) - 1

	lines := b.rowLines()[last]
	if len(lines) != 4 {
		t.Fatalf("the last entry is %d lines, want its own two, the border and the gap", len(lines))
	}
	if got := ansi.ReplaceAllString(lines[2], ""); !strings.HasPrefix(got, "╰") {
		t.Errorf("line = %q, want the box closed under the last entry", got)
	}
	if lines[3] != "" {
		t.Errorf("line = %q, want the blank line that sets the plan apart", lines[3])
	}

	b.SelectRow(last)
	top, height := b.CursorSpan()
	if height != 4 {
		t.Errorf("span height = %d, want the border and the gap counted with the row", height)
	}
	if at, ok := b.RowAtLine(top + 3); !ok || at != last {
		t.Errorf("line %d is row %d (%v), want the entry's own", top+3, at, ok)
	}
}

// An unmeasured board has no width to take, so the section is sized to the
// widest thing in it and nothing of an entry is lost.
func TestBoardActiveSectionWithoutAWidth(t *testing.T) {
	b := activeBoard(0)

	lines := strings.Split(ansi.ReplaceAllString(b.View(), ""), "\n")
	widest := 0
	for _, line := range lines[:12] {
		widest = max(widest, lipgloss.Width(line))
	}
	for i, line := range lines[:12] {
		if got := lipgloss.Width(line); got != widest {
			t.Errorf("line %d = %q is %d wide, want the box squared off at %d", i, line, got, widest)
		}
	}
	if !strings.Contains(lines[9], "● Awaiting review") {
		t.Errorf("line = %q, want the longest entry drawn whole", lines[9])
	}
}

// A section narrower than anything it holds is cut to the board's width like
// every other row, down to a box that is nothing but its own edges.
func TestBoardActiveSectionNarrows(t *testing.T) {
	for width := 1; width <= 40; width++ {
		b := activeBoard(width)
		for _, line := range strings.Split(b.View(), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("at width %d the line %q is %d wide", width, line, got)
			}
		}
	}
}

// TestBoardActivePresenceReadings pins what the section makes of the board's
// two agent readings: a live agent nobody has classified is working, and one
// that has gone leaves the slice to the rest of the rule.
func TestBoardActivePresenceReadings(t *testing.T) {
	b := activeBoard(60)

	for _, tc := range []struct {
		name     string
		live     map[string]string
		activity map[string]Presence
		want     domain.AgentPresence
	}{
		{"no session", nil, nil, domain.AgentNone},
		{"unclassified", map[string]string{"s1": "nat-1"}, nil, domain.AgentUnknown},
		{"working", map[string]string{"s1": "nat-1"},
			map[string]Presence{"s1": PresenceWorking}, domain.AgentWorking},
		{"waiting", map[string]string{"s1": "nat-1"},
			map[string]Presence{"s1": PresenceWaiting}, domain.AgentWaiting},
		{"gone", nil, map[string]Presence{"s1": PresenceWorking}, domain.AgentNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b.SetLive(tc.live)
			b.SetActivity(tc.activity)
			if got := b.agentPresence("s1"); got != tc.want {
				t.Errorf("agentPresence = %v, want %v", got, tc.want)
			}
		})
	}
}
