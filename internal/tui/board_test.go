package tui

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// testProject is the plan the board tests render: a finished milestone, the
// active one holding a slice of each status, an empty queued one, and a slice
// whose milestone is not in the plan, which lands in Unassigned.
func testProject() domain.Project {
	return domain.Project{
		ID:   testProjectID,
		Name: "tracker",
		Milestones: []domain.Milestone{
			{ID: "m1", Name: "M1: Config", Order: 1, Status: domain.MilestoneDone},
			{ID: "m2", Name: "M2: Board", Order: 2, Status: domain.MilestoneActive},
			{ID: "m3", Name: "M3: Mutations", Order: 3, Status: domain.MilestoneQueued},
		},
		Slices: []domain.Slice{
			{ID: "s1", Name: "XDG config", Status: domain.SliceDone, MilestoneID: "m1"},
			{ID: "s2", Name: "Keyring", Status: domain.SliceDone, MilestoneID: "m1"},
			{ID: "s3", Name: "Domain model", Status: domain.SliceDone, MilestoneID: "m2",
				AssigneeName: "Craig Johnston", PRURL: "https://example.test/pr/1"},
			{ID: "s4", Name: "Board screen", Status: domain.SliceClaimed, MilestoneID: "m2",
				AssigneeName: "Craig Johnston"},
			{ID: "s5", Name: "Info view", Status: domain.SliceTodo, MilestoneID: "m2"},
			{ID: "s6", Name: "Stray", Status: "Unknown", MilestoneID: "gone"},
		},
	}
}

// newTestBoard returns a board showing testProject at a fixed width.
func newTestBoard() *Board {
	b := NewBoard(DefaultStyles())
	b.SetWidth(60)
	p := testProject()
	b.SetProject(&p)
	return &b
}

// rowNames is the board's rows as bare text, which is what the navigation tests
// assert against — the golden file covers how they are decorated.
func rowNames(b *Board) []string {
	names := make([]string, len(b.rows))
	for i, r := range b.rows {
		if r.kind == rowMilestone {
			names[i] = b.groups[r.group].Name()
			continue
		}
		names[i] = b.groups[r.group].Slices[r.slice].Name
	}
	return names
}

// golden compares got against testdata/<name>.golden, rewriting it under
// -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v — rerun with -update to create it", err)
	}
	if got != string(want) {
		t.Errorf("render does not match %s:\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}

func TestBoardExpandsOnlyTheActiveMilestoneByDefault(t *testing.T) {
	b := newTestBoard()

	want := []string{
		"M1: Config",
		"M2: Board", "Domain model", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}
	if got := rowNames(b); !equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestBoardRendersExpandedAndCollapsedGroups(t *testing.T) {
	golden(t, "board-default", newTestBoard().View())
}

func TestBoardRendersEveryGroupCollapsed(t *testing.T) {
	b := newTestBoard()
	for k := range b.expanded {
		b.expanded[k] = false
	}
	b.rebuild()

	golden(t, "board-collapsed", b.View())
}

func TestBoardRendersEveryGroupExpanded(t *testing.T) {
	b := newTestBoard()
	for k := range b.expanded {
		b.expanded[k] = true
	}
	b.rebuild()
	b.cursor = 2 // a slice row, so the cursor is shown on one

	golden(t, "board-expanded", b.View())
}

// ansi strips the colour codes, leaving the characters the alignment tests
// measure columns over.
var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestBoardRendersAsATable pins the tabular layout: the milestone's plan
// number is its own column rather than an inline "M2:" prefix, and the counts
// and status pills right-align into straight vertical columns.
func TestBoardRendersAsATable(t *testing.T) {
	b := newTestBoard()
	lines := strings.Split(ansi.ReplaceAllString(b.View(), ""), "\n")

	for _, line := range lines {
		if strings.Contains(line, "M1") || strings.Contains(line, "M2") || strings.Contains(line, "M3") {
			t.Errorf("line %q still carries an inline milestone number", line)
		}
	}
	for i, want := range map[int]string{0: "1 ▸ Config", 1: "2 ▾ Board", 5: "3 ▸ Mutations", 6: "▾ Unassigned"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want it to contain %q", i, lines[i], want)
		}
	}

	// endColumn is the display column a substring ends on, which is what has
	// to match for the cells to read as one column — byte offsets would lie on
	// the lines that carry multi-byte runes like the cursor and fold glyphs.
	endColumn := func(line, sub string) int {
		at := strings.Index(line, sub)
		if at < 0 {
			return -1
		}
		return lipgloss.Width(line[:at+len(sub)])
	}
	countEnd, pillEnd := -1, -1
	for i, count := range map[int]string{0: "2/2", 1: "1/3", 5: "0/0", 6: "0/1"} {
		got := endColumn(lines[i], count)
		if got < 0 {
			t.Fatalf("line %d = %q, want the count %q on it", i, lines[i], count)
		}
		if countEnd == -1 {
			countEnd = got
		}
		if got != countEnd {
			t.Errorf("line %d ends its count at column %d, want %d:\n%q", i, got, countEnd, lines[i])
		}
	}
	for i, pill := range map[int]string{0: "Done", 1: "Active", 5: "Queued"} {
		got := endColumn(lines[i], pill)
		if got < 0 {
			t.Fatalf("line %d = %q, want the pill %q on it", i, lines[i], pill)
		}
		if pillEnd == -1 {
			pillEnd = got
		}
		if got != pillEnd {
			t.Errorf("line %d ends its pill at column %d, want %d:\n%q", i, got, pillEnd, lines[i])
		}
	}
}

func TestBoardMarksSlicesWithALiveSession(t *testing.T) {
	b := newTestBoard()
	// The claimed slice's agent, and one working a slice of another project.
	b.SetLive(map[string]string{
		"s4":            agent.SessionName("s4"),
		"another-slice": "nat-deadbeef",
	})

	golden(t, "board-live", b.View())
}

func TestBoardFillsTheSelectedRowToItsWidth(t *testing.T) {
	b := newTestBoard()
	b.cursor = 2 // a slice row

	lines := strings.Split(b.View(), "\n")
	if got := lipgloss.Width(lines[2]); got != 60 {
		t.Errorf("the selected row is %d wide, want the fill run out to 60", got)
	}
	if got := lipgloss.Width(lines[3]); got >= 60 {
		t.Errorf("an unselected row is %d wide, want it left unfilled", got)
	}
}

func TestBoardTruncatesRowsToItsWidth(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(14)

	for _, line := range strings.Split(b.View(), "\n") {
		if got := lipgloss.Width(line); got > 14 {
			t.Errorf("line %q is %d wide, want at most 14", line, got)
		}
	}
}

// longRowProject is the plan the narrowing tests render: names long enough that
// the chips have to go, on a slice carrying every one of them.
func longRowProject() domain.Project {
	return domain.Project{
		ID:   testProjectID,
		Name: "tracker",
		Milestones: []domain.Milestone{
			{ID: "m7", Name: "M7: Agent pane view, joined beside the board pane",
				Order: 7, Status: domain.MilestoneActive},
		},
		Slices: []domain.Slice{
			{ID: "s1", Name: "Degrade slice rows gracefully as the board narrows",
				Status: domain.SliceClaimed, MilestoneID: "m7",
				AssigneeName: "Craig Johnston", PRURL: "https://example.test/pr/9"},
			{ID: "s2", Name: "Keep the status bar and header inside the window",
				Status: domain.SliceTodo, MilestoneID: "m7"},
		},
	}
}

// newLongRowBoard returns a board showing longRowProject at width, with an agent
// live on its claimed slice.
func newLongRowBoard(width int) *Board {
	b := NewBoard(DefaultStyles())
	b.SetWidth(width)
	b.SetLive(map[string]string{"s1": agent.SessionName("s1")})
	p := longRowProject()
	b.SetProject(&p)
	return &b
}

func TestBoardDropsChipsInOrderAsItNarrows(t *testing.T) {
	// At 80 every part of the row fits.
	view := newLongRowBoard(80).View()
	golden(t, "board-narrow-80", view)
	for _, want := range []string{"Degrade slice rows gracefully as the board narrows",
		"●", "@Craig Johnston", "PR", "0/2", "Active"} {
		if !strings.Contains(view, want) {
			t.Errorf("at 80 the view is missing %q:\n%s", want, view)
		}
	}

	// At 60 the PR marker and the assignee have gone, but the live marker, the
	// names and the milestone's count are all still there.
	view = newLongRowBoard(60).View()
	golden(t, "board-narrow-60", view)
	for _, want := range []string{"Degrade slice rows gracefully as the board narrows",
		"Keep the status bar and header inside the window", "●", "0/2"} {
		if !strings.Contains(view, want) {
			t.Errorf("at 60 the view is missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"PR", "@Craig Johnston", "Active"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("at 60 the view should have dropped %q:\n%s", unwanted, view)
		}
	}

	// At 40 nothing is left to drop, so the names are truncated instead — but
	// each row still leads with its status glyph and the start of its name.
	view = newLongRowBoard(40).View()
	golden(t, "board-narrow-40", view)
	for _, want := range []string{"Degrade slice rows", "Keep the status bar", "Agent pane"} {
		if !strings.Contains(view, want) {
			t.Errorf("at 40 the view is missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"PR", "@Craig Johnston", "Active", "0/2", "●"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("at 40 the view should have dropped %q:\n%s", unwanted, view)
		}
	}
}

func TestBoardNeverExceedsItsWidthAsItNarrows(t *testing.T) {
	for width := 1; width <= 80; width++ {
		for _, line := range strings.Split(newLongRowBoard(width).View(), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("at width %d the line %q is %d wide", width, line, got)
			}
		}
	}
}

func TestBoardWithoutAWidthDropsNothing(t *testing.T) {
	b := newLongRowBoard(0)

	view := b.View()
	for _, want := range []string{"●", "@Craig Johnston", "PR", "Active"} {
		if !strings.Contains(view, want) {
			t.Errorf("an unmeasured board should draw everything, missing %q:\n%s", want, view)
		}
	}
}

func TestBoardWithoutAProjectSaysSo(t *testing.T) {
	b := NewBoard(DefaultStyles())

	if got := b.View(); !strings.Contains(got, "No milestones yet") {
		t.Errorf("view = %q, want the empty state", got)
	}
	if _, ok := b.SelectedSlice(); ok {
		t.Error("there is nothing to select")
	}
}

func TestBoardNavigatesWithoutWrapping(t *testing.T) {
	b := newTestBoard()

	// Up at the top stays put.
	b.Update(keyPress("up"))
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want it held at the top", b.cursor)
	}

	for _, k := range []string{"j", "down", "j"} {
		b.Update(keyPress(k))
	}
	if b.cursor != 3 {
		t.Fatalf("cursor = %d, want 3", b.cursor)
	}
	b.Update(keyPress("k"))
	if b.cursor != 2 {
		t.Errorf("cursor = %d, want 2 after k", b.cursor)
	}

	// Down at the bottom stays put.
	for range len(b.rows) + 2 {
		b.Update(keyPress("j"))
	}
	if b.cursor != len(b.rows)-1 {
		t.Errorf("cursor = %d, want it held at the bottom row %d", b.cursor, len(b.rows)-1)
	}
}

func TestBoardEnterCollapsesAndExpands(t *testing.T) {
	b := newTestBoard()
	b.cursor = 1 // the active milestone's own row

	b.Update(keyPress("enter"))
	if got := rowNames(b); !equal(got, []string{
		"M1: Config", "M2: Board", "M3: Mutations", domain.UnassignedName, "Stray",
	}) {
		t.Errorf("rows = %q, want the active milestone collapsed", got)
	}
	if b.cursor != 1 {
		t.Errorf("cursor = %d, want it left on the collapsed milestone", b.cursor)
	}

	b.Update(keyPress("enter"))
	if got := len(b.rows); got != 8 {
		t.Errorf("rows = %d, want the slices back", got)
	}
}

func TestBoardEnterOnASliceCollapsesItsMilestone(t *testing.T) {
	b := newTestBoard()
	b.cursor = 3 // the second slice of the active milestone

	b.Update(keyPress("enter"))

	if b.cursor != 1 {
		t.Errorf("cursor = %d, want it moved to the milestone row it collapsed", b.cursor)
	}
	if _, ok := b.SelectedSlice(); ok {
		t.Error("the cursor should no longer be on a slice")
	}
}

func TestBoardEnterOnAnEmptyBoardDoesNothing(t *testing.T) {
	b := NewBoard(DefaultStyles())

	b.Update(keyPress("enter"))

	if b.cursor != 0 || len(b.rows) != 0 {
		t.Errorf("cursor = %d, rows = %d, want an untouched empty board", b.cursor, len(b.rows))
	}
}

func TestBoardSelectedSlice(t *testing.T) {
	b := newTestBoard()

	if _, ok := b.SelectedSlice(); ok {
		t.Error("the cursor starts on a milestone, not a slice")
	}
	b.cursor = 2
	got, ok := b.SelectedSlice()
	if !ok || got.ID != "s3" {
		t.Errorf("selected = %+v (ok=%v), want s3", got, ok)
	}
}

func TestBoardSelectedMilestone(t *testing.T) {
	if _, ok := NewBoard(DefaultStyles()).SelectedMilestone(); ok {
		t.Error("an empty board has no milestone under the cursor")
	}

	b := newTestBoard()
	got, ok := b.SelectedMilestone()
	if !ok || got.ID != "m1" {
		t.Errorf("selected = %+v (ok=%v), want m1", got, ok)
	}
	b.cursor = 2
	if _, ok := b.SelectedMilestone(); ok {
		t.Error("the cursor is on a slice, not a milestone")
	}
	b.cursor = len(b.rows) - 2 // the Unassigned group's own row
	if _, ok := b.SelectedMilestone(); ok {
		t.Error("the Unassigned group is not a milestone")
	}
}

func TestBoardKeepsFoldStateAndClampsTheCursorAcrossAReload(t *testing.T) {
	b := newTestBoard()
	b.cursor = 1
	b.Update(keyPress("enter")) // collapse the active milestone
	b.cursor = len(b.rows) - 1

	// A reload that has lost everything but the first milestone.
	p := domain.Project{Milestones: []domain.Milestone{
		{ID: "m2", Name: "M2: Board", Order: 2, Status: domain.MilestoneActive},
	}}
	b.SetProject(&p)

	if b.expanded["m2"] {
		t.Error("the user's collapse should survive a reload")
	}
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want it clamped to the only row left", b.cursor)
	}
}

func TestBoardHandlesAProjectWithNothingInIt(t *testing.T) {
	b := newTestBoard()
	b.cursor = 4

	b.SetProject(&domain.Project{})

	if b.cursor != 0 || len(b.rows) != 0 {
		t.Errorf("cursor = %d, rows = %d, want an empty board with a valid cursor", b.cursor, len(b.rows))
	}
	if got := b.View(); !strings.Contains(got, "No milestones yet") {
		t.Errorf("view = %q, want the empty state", got)
	}
}

func TestBoardIgnoresNonKeyMessages(t *testing.T) {
	b := newTestBoard()

	if cmd := b.Update(tea.WindowSizeMsg{Width: 10, Height: 10}); cmd != nil {
		t.Error("the board has nothing to do with a resize")
	}
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want it untouched", b.cursor)
	}
}

func TestBoardSwallowsTheReservedKeys(t *testing.T) {
	for _, k := range []string{"Q", "a", "e", "m", "d", "l", "t"} {
		b := newTestBoard()

		if cmd := b.Update(keyPress(k)); cmd != nil {
			t.Errorf("%q should do nothing yet", k)
		}
		if b.cursor != 0 || len(b.rows) != 8 {
			t.Errorf("%q changed the board: cursor = %d, rows = %d", k, b.cursor, len(b.rows))
		}
	}
}

func TestBoardUnknownSliceStatusStillDraws(t *testing.T) {
	b := newTestBoard()

	// The Unassigned group's slice has a status this build does not know.
	if got := b.renderSlice("  ", b.groups[3].Slices[0], false); !strings.Contains(got, "Stray") {
		t.Errorf("render = %q, want the slice name", got)
	}
}

func TestAppHelpListsTheBoardKeys(t *testing.T) {
	app := NewApp(testConfig(), nil)
	press(app, "?")

	view := app.View().Content
	for _, want := range []string{"quit", "Board", "expand/collapse", "launch agent"} {
		if !strings.Contains(view, want) {
			t.Errorf("help is missing %q:\n%s", want, view)
		}
	}
}

func TestAppShowsTheBoardAndRoutesKeysToIt(t *testing.T) {
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	app.Update(projectLoadedMsg{project: testProject()})

	view := app.View().Content
	for _, want := range []string{"Config", "Board", "Board screen"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
	if got := app.board.width; got != 56 {
		t.Errorf("board width = %d, want the window less the frame", got)
	}

	press(app, "j")
	if app.board.cursor != 1 {
		t.Errorf("cursor = %d, want the key routed to the board", app.board.cursor)
	}
}

func TestAppDoesNotRouteKeysToTheBoardFromAnotherScreen(t *testing.T) {
	app := NewApp(testConfig(), newLoadingClient())
	app.Update(projectLoadedMsg{project: testProject()})
	press(app, "?")

	press(app, "j")

	if app.board.cursor != 0 {
		t.Errorf("cursor = %d, want the help screen to keep the key", app.board.cursor)
	}
}

// keyPress builds the key message for a binding's key name.
func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
	default:
		return tea.KeyPressMsg(tea.Key{Code: rune(s[0]), Text: s})
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
