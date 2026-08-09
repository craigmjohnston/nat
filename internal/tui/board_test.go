package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
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

func TestBoardTruncatesRowsToItsWidth(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(14)

	for _, line := range strings.Split(b.View(), "\n") {
		if got := lipgloss.Width(line); got > 14 {
			t.Errorf("line %q is %d wide, want at most 14", line, got)
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
	if got := b.renderSlice(b.groups[3].Slices[0]); !strings.Contains(got, "Stray") {
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
	for _, want := range []string{"M1: Config", "M2: Board", "Board screen"} {
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
