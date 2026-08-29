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

// activeText is the section's entries as bare lines, which is what the panel's
// own box is drawn around.
func activeText(b *Board) []string {
	return strings.Split(ansi.ReplaceAllString(strings.Join(b.ActiveLines(), "\n"), ""), "\n")
}

// TestBoardDrawsAnActiveEntryForEveryState pins the section: every slice in
// flight is an entry of it, in the order the plan draws their milestones, and
// each one names the state it is in beside the milestone it is filed under.
func TestBoardDrawsAnActiveEntryForEveryState(t *testing.T) {
	b := activeBoard(60)

	golden(t, "board-active", strings.Join(b.ActiveLines(), "\n"))

	lines := activeText(b)
	for i, want := range []string{
		"● Working", "  working · Config",
		"● Waiting", "  waiting · Config",
		"● Blocked", "  blocked · Board",
		"● Ready to push", "  ready to push · Board",
		"● Awaiting review", "  awaiting review · Board",
	} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %q on it", i, lines[i], want)
		}
	}
	if got := len(lines); got != activeEntryLines*len(b.active) {
		t.Errorf("the section is %d lines, want two for each of its %d entries",
			got, len(b.active))
	}
	// The Todo slice the blocked one waits on is not in flight, so the section
	// says nothing about it.
	if strings.Contains(strings.Join(lines, "\n"), "Not started") {
		t.Errorf("the section drew a slice that has not started:\n%s", strings.Join(lines, "\n"))
	}
}

// The section is drawn in a panel of its own, so nothing of it is a row of the
// plan: the plan's own lines hold no entry, and the lines the cursor and the
// mouse are measured over are the plan's alone.
func TestBoardActiveSectionIsNoRowOfThePlan(t *testing.T) {
	b := activeBoard(60)

	if got := b.View(); strings.Contains(got, activeDot) || strings.Contains(got, "╭─ ") {
		t.Errorf("the plan drew the section inside it:\n%s", got)
	}
	if got, want := len(b.rowLines()), len(b.rows)-len(b.active); got != want {
		t.Errorf("the plan draws %d rows, want %d — the board's rows less the section's",
			got, want)
	}
	// The cursor starts on the first entry, which is in the other panel: there is
	// nothing in the plan to bring on screen for it.
	if top, height := b.CursorSpan(); top != 0 || height != 0 {
		t.Errorf("CursorSpan = (%d, %d), want nothing of the plan for a cursor in the section",
			top, height)
	}
	// The plan's first line is the plan's first row, which is the board row after
	// the last entry.
	if at, ok := b.RowAtLine(0); !ok || at != len(b.active) {
		t.Errorf("line 0 is row %d (%v), want the plan's own first row %d",
			at, ok, len(b.active))
	}
}

// The panel names the row an entry is drawn on, and its cursor span, in the
// lines of its own box — the mouse's way in, and the panel's own scrolling.
func TestBoardActivePanelLines(t *testing.T) {
	b := activeBoard(60)
	b.SelectRow(2)

	if top, height := b.ActiveCursorSpan(); top != 4 || height != activeEntryLines {
		t.Errorf("ActiveCursorSpan = (%d, %d), want the third entry's own two lines", top, height)
	}
	for line, want := range map[int]int{0: 0, 1: 0, 2: 1, 9: 4} {
		if at, ok := b.ActiveRowAtLine(line); !ok || at != want {
			t.Errorf("line %d is entry %d (%v), want %d", line, at, ok, want)
		}
	}
	for _, line := range []int{-1, activeEntryLines * len(b.active)} {
		if _, ok := b.ActiveRowAtLine(line); ok {
			t.Errorf("line %d is an entry, want none off the panel", line)
		}
	}
	// Down in the plan the cursor is in the other panel, and has nothing here.
	b.SelectRow(len(b.active))
	if _, height := b.ActiveCursorSpan(); height != 0 {
		t.Errorf("ActiveCursorSpan height = %d, want nothing for a cursor in the plan", height)
	}
}

// With the section hidden — the layout's answer for a band with no room for a
// second panel — the entries take no rows at all, so the cursor can never be
// left on one nothing draws.
func TestBoardHiddenActiveSectionTakesNoRows(t *testing.T) {
	b := activeBoard(60)
	rows := len(b.rows)

	b.SetShowActive(false)

	if b.ActiveLines() != nil {
		t.Errorf("ActiveLines = %q, want nothing drawn", b.ActiveLines())
	}
	if got, want := len(b.rows), rows-len(b.active); got != want {
		t.Errorf("rows = %d, want the plan's %d alone", got, want)
	}
	if got := b.rows[b.cursor].kind; got == rowActive {
		t.Errorf("the cursor is on a row of kind %v, want one the plan draws", got)
	}
	if _, ok := b.SelectedActive(); ok {
		t.Error("an entry is selected with the section hidden")
	}
	// The count the layout sizes the panel from is the plan's, not the rows':
	// it is what the layout asks before it has decided.
	if got := b.ActiveCount(); got != len(b.active) {
		t.Errorf("ActiveCount = %d, want the %d slices in flight", got, len(b.active))
	}

	b.SetShowActive(true)
	if got, want := len(b.rows), rows; got != want {
		t.Errorf("rows = %d, want the section back at %d", got, want)
	}
}

// The dot and the state word take the state's own colour, which is what the
// section is read at a glance by.
func TestBoardColoursAnActiveEntryByItsState(t *testing.T) {
	b := activeBoard(60)
	// Off the section: a selected entry is drawn over the fill, which is a
	// colour of its own on every part of it.
	b.SelectRow(len(b.rows) - 1)
	view := strings.Join(b.ActiveLines(), "\n")

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
// panel, not a row — so a board with no work under way reads exactly as it did
// before there was one.
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
	if b.ActiveLines() != nil || b.ActiveHeight() != 0 {
		t.Errorf("ActiveLines = %q (%d lines), want no section at all",
			b.ActiveLines(), b.ActiveHeight())
	}
	if got := b.View(); strings.Contains(got, activeDot) {
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

	lines := b.ActiveLines()[:activeEntryLines]
	fill := backgroundOf(lipgloss.NewStyle().Background(b.styles.ActiveFill))
	for _, line := range lines {
		if !strings.Contains(line, fill) {
			t.Errorf("a selected entry's line is unfilled:\n%q", line)
		}
		if got := lipgloss.Width(line); got != 60 {
			t.Errorf("line %q is %d wide, want the section run out to 60", line, got)
		}
	}
	if !strings.Contains(lines[0], wash(b.styles.StateWorking, b.styles.ActiveFill).Render(activeDot)) {
		t.Errorf("the dot lost its state colour to the fill:\n%q", lines[0])
	}

	// An entry the cursor is not on carries no fill at all.
	next := b.ActiveLines()[activeEntryLines : 2*activeEntryLines]
	if strings.Contains(strings.Join(next, "\n"), fill) {
		t.Errorf("an unselected entry is filled:\n%q", next)
	}
}

// An unmeasured board has no width to run its entries out to, so each line is
// what it says and nothing of it is lost.
func TestBoardActiveSectionWithoutAWidth(t *testing.T) {
	b := activeBoard(0)

	lines := activeText(b)
	if got := lines[len(lines)-2]; got != "● Awaiting review" {
		t.Errorf("line = %q, want the longest entry drawn whole and no wider", got)
	}
}

// A section narrower than anything it holds is cut to the board's width like
// every other line of it.
func TestBoardActiveSectionNarrows(t *testing.T) {
	for width := 1; width <= 40; width++ {
		b := activeBoard(width)
		for _, line := range b.ActiveLines() {
			if got := lipgloss.Width(line); got != width {
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

// TestBoardDrawsAConfirmationOnTheSelectedActiveEntry pins the entry's half of
// the anchoring: a confirmation set while the cursor is in the section is drawn
// on the entry's foot line, exactly where a plan row's last line carries it.
// Without it the keys that open one would swallow input with nothing on screen
// saying why.
func TestBoardDrawsAConfirmationOnTheSelectedActiveEntry(t *testing.T) {
	b := activeBoard(60)
	b.SetConfirm("Approve \"Working\"?", sevSuccess)

	golden(t, "board-active-confirm", strings.Join(b.ActiveLines(), "\n"))

	lines := activeText(b)
	if strings.Contains(lines[0], "Approve") {
		t.Errorf("the confirmation is on the entry's name line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Approve \"Working\"?") {
		t.Errorf("foot line = %q, want the confirmation anchored to it", lines[1])
	}
	// It belongs to the entry the cursor is on and to no other.
	for i, line := range lines[activeEntryLines:] {
		if strings.Contains(line, "Approve") {
			t.Errorf("entry line %d = %q, want the confirmation on the selected entry alone",
				i+activeEntryLines, line)
		}
	}
	if got := lipgloss.Width(b.ActiveLines()[1]); got != 60 {
		t.Errorf("the foot line is %d wide, want the section's own 60", got)
	}
}

// TestBoardDrawsThePromptOnTheSelectedActiveEntry is the other thing anchored
// to a row: the question waiting to be answered, with its choices side by side,
// drawn on the entry the cursor is on.
func TestBoardDrawsThePromptOnTheSelectedActiveEntry(t *testing.T) {
	b := activeBoard(60)
	b.SetPrompt([]string{"release", "cancel"})

	golden(t, "board-active-prompt", strings.Join(b.ActiveLines(), "\n"))

	lines := activeText(b)
	if !strings.Contains(lines[1], "release") || !strings.Contains(lines[1], "cancel") {
		t.Errorf("foot line = %q, want the prompt's choices on it", lines[1])
	}
	// A prompt takes the place of a confirmation rather than sitting beside it,
	// the way it does on a plan row.
	b.SetConfirm("Approve?", sevSuccess)
	b.SetPrompt([]string{"release", "cancel"})
	if strings.Contains(activeText(b)[1], "Approve") {
		t.Errorf("the confirmation outlived the prompt: %q", activeText(b)[1])
	}
}

// An unmeasured section has no right edge to anchor a chip to, so it simply
// follows the entry's foot line — the same answer [Board.overlayChip] gives an
// unmeasured plan.
func TestBoardActiveEntryChipWithoutAWidth(t *testing.T) {
	b := activeBoard(0)
	b.SetConfirm("Approve?", sevSuccess)

	if got := activeText(b)[1]; !strings.Contains(got, "· Config  Approve?") {
		t.Errorf("foot line = %q, want the confirmation following it", got)
	}
}

// With the cursor down in the plan the section carries nothing at all: the
// confirmation is anchored to the row it was opened on, which is no entry of
// this panel.
func TestBoardActiveSectionCarriesNoChipForAPlanRow(t *testing.T) {
	b := activeBoard(60)
	b.cursor = b.activeRowCount()
	b.SetConfirm("Approve?", sevSuccess)

	if got := strings.Join(activeText(b), "\n"); strings.Contains(got, "Approve?") {
		t.Errorf("the section drew a chip for a row of the plan:\n%s", got)
	}
}

// cursorOnActive puts the cursor on the Active section's entry for the named
// slice, which is the row the section's own keys act on.
func cursorOnActive(t *testing.T, a *App, id string) {
	t.Helper()
	for i, r := range a.board.rows {
		if r.kind == rowActive && a.board.active[r.slice].ID == id {
			a.board.cursor = i
			return
		}
	}
	t.Fatalf("no active entry for slice %q", id)
}

// TestApproveFromAnActiveEntry covers an entry taken all the way: v on it reads
// the same slice a plan row would, and approving what that shows opens the pull
// request. An entry is the slice drawn a second time, so every key that acts on
// one has to find it there.
func TestApproveFromAnActiveEntry(t *testing.T) {
	app, prs, _, workdir := approveApp(t)
	app.board.SetWidth(60)
	cursorOnActive(t, app, handedBack)

	approve(t, app)

	want := prCall{workdir, "slice/approve", "", ""}
	if len(prs.made) != 1 || prs.made[0] != want {
		t.Fatalf("gh was asked for %v, want %v", prs.made, want)
	}
	// The slice is Done with nothing yet read of its pull request, so it has
	// left the section along with the entry the review was opened from — the
	// confirmation is the board's, anchored to wherever the cursor now is.
	if !strings.Contains(app.board.confirmText, "Approve action") {
		t.Errorf("confirmation = %q, want it to name the slice", app.board.confirmText)
	}
}

// R on an entry is the release prompt, anchored to the entry the way it is to
// the row further down the plan.
func TestReleaseFromAnActiveEntry(t *testing.T) {
	app, _, _, _ := approveApp(t)
	app.board.SetWidth(60)
	cursorOnActive(t, app, handedBack)

	feed(t, app, press(app, "R"))

	if !app.board.Prompting() {
		t.Fatalf("R on an entry opened no prompt: %q", app.board.confirmText)
	}
	if got := activeText(&app.board)[1]; !strings.Contains(got, "release") {
		t.Errorf("entry foot line = %q, want the release prompt drawn on it", got)
	}
}

// l on an entry is the launch prompt: a slice in flight with no session on it
// is one an agent can be put back on, and the entry draws the prompt for the
// same reason it draws the release one — a key that says nothing is a key that
// looks broken.
func TestLaunchFromAnActiveEntryPromptsOnTheEntry(t *testing.T) {
	app, _, _, _ := approveApp(t)
	app.board.SetWidth(60)
	cursorOnActive(t, app, handedBack)

	feed(t, app, press(app, "l"))

	if !app.board.Prompting() {
		t.Fatalf("l on an entry opened no prompt: %q", app.board.confirmText)
	}
	if got := activeText(&app.board)[1]; !strings.Contains(got, "launch") {
		t.Errorf("entry foot line = %q, want the launch prompt drawn on it", got)
	}
}
