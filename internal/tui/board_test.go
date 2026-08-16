package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// testProject is the plan the board tests render: a finished milestone, the one
// in flight holding a slice of each status, an empty one after it, and a slice
// whose milestone is not in the plan, which lands in Unassigned.
//
// The milestones are the options of the slices' Milestone column, which is
// where every project keeps its plan: each one's ID is its name, and its status
// is read back off the slices under it.
func testProject() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Config", "M2: Board", "M3: Mutations"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: "s1", Name: "XDG config", Status: domain.SliceDone, MilestoneID: "M1: Config"},
			{ID: "s2", Name: "Keyring", Status: domain.SliceDone, MilestoneID: "M1: Config"},
			{ID: "s3", Name: "Domain model", Status: domain.SliceDone, MilestoneID: "M2: Board",
				AssigneeName: "Craig Johnston", PRURL: "https://example.test/pr/1"},
			{ID: "s4", Name: "Board screen", Status: domain.SliceClaimed, StatusName: "In progress",
				MilestoneID: "M2: Board", AssigneeName: "Craig Johnston"},
			{ID: "s5", Name: "Info view", Status: domain.SliceTodo, MilestoneID: "M2: Board"},
			{ID: "s6", Name: "Stray", Status: "Unknown", MilestoneID: "gone"},
		})
}

// newTestBoard returns a board showing testProject at a fixed width, with the
// hide-done toggle off. Most board tests are about fold state, navigation and
// layout and want every slice on show; the ones about the toggle turn it back
// on, and TestBoardHidesDoneSlicesByDefault covers the state a real board
// starts in.
func newTestBoard() *Board {
	b := NewBoard(DefaultStyles())
	b.hideDone = false
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
		switch r.kind {
		case rowSection:
			names[i] = "Done section"
		case rowMilestone:
			names[i] = b.groups[r.group].Name()
		default:
			names[i] = b.groups[r.group].Slices[r.slice].Name
		}
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
		"Done section",
		"M2: Board", "Domain model", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}
	if got := rowNames(b); !equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestBoardFoldsDoneMilestonesIntoOneSection(t *testing.T) {
	b := newTestBoard()
	// A second Done milestone, later in the plan than the active one.
	p := testProject()
	p.Milestones = append(p.Milestones,
		domain.Milestone{ID: "m0", Name: "M0: Setup", Order: 4, Status: domain.MilestoneDone})
	p.Slices = append(p.Slices,
		domain.Slice{ID: "s7", Name: "Scaffold", Status: domain.SliceDone, MilestoneID: "m0"})
	b.SetProject(&p)

	want := []string{
		"Done section",
		"M2: Board", "Domain model", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}
	if got := rowNames(b); !equal(got, want) {
		t.Errorf("rows = %q, want both Done milestones behind one section: %q", got, want)
	}
	if view := ansi.ReplaceAllString(b.View(), ""); !strings.Contains(view, "2 milestones · 3/3") {
		t.Errorf("view is missing the plural aggregate:\n%s", view)
	}

	// Expanding the section gathers the Done milestones under it, wherever they
	// sat in the plan, each collapsed to its own row.
	b.cursor = 0
	b.Update(keyPress("enter"))
	want = []string{
		"Done section", "M1: Config", "M0: Setup",
		"M2: Board", "Domain model", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}
	if got := rowNames(b); !equal(got, want) {
		t.Errorf("rows = %q, want the section expanded: %q", got, want)
	}
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want it left on the section row", b.cursor)
	}

	// A revealed Done milestone still expands to its slices, as it always did.
	b.cursor = 1
	b.Update(keyPress("enter"))
	if got := rowNames(b); !equal(got[:4], []string{"Done section", "M1: Config", "XDG config", "Keyring"}) {
		t.Errorf("rows = %q, want M1's slices revealed", got)
	}

	// Collapsing the section from its own row hides it all again.
	b.cursor = 0
	b.Update(keyPress("enter"))
	if got := rowNames(b); !equal(got, []string{
		"Done section",
		"M2: Board", "Domain model", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}) {
		t.Errorf("rows = %q, want the section collapsed again", got)
	}
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want it left on the section row", b.cursor)
	}
}

func TestBoardKeepsTheDoneSectionStateAcrossAReload(t *testing.T) {
	b := newTestBoard()
	b.cursor = 0
	b.Update(keyPress("enter")) // expand the Done section

	p := testProject()
	b.SetProject(&p)

	if got := rowNames(b); !equal(got[:2], []string{"Done section", "M1: Config"}) {
		t.Errorf("rows = %q, want the section still expanded after a reload", got)
	}
}

func TestBoardWithoutDoneMilestonesHasNoDoneSection(t *testing.T) {
	b := newTestBoard()
	p := testProject()
	p.Milestones = p.Milestones[1:] // drop the Done milestone
	p.Slices = p.Slices[2:]
	b.SetProject(&p)

	for _, name := range rowNames(b) {
		if name == "Done section" {
			t.Fatalf("rows = %q, want no Done section", rowNames(b))
		}
	}
}

// A board starts with the Done slices of unfinished milestones already hidden:
// what is left to do is what the board is read for.
func TestBoardHidesDoneSlicesByDefault(t *testing.T) {
	b := NewBoard(DefaultStyles())
	b.SetWidth(60)
	p := testProject()
	b.SetProject(&p)

	for _, name := range rowNames(&b) {
		if name == "Domain model" {
			t.Fatalf("rows = %q, want the Done slice hidden from the start", rowNames(&b))
		}
	}

	// And the key is what turns it off again.
	b.Update(keyPress("z"))
	if got := rowNames(&b); !slices.Contains(got, "Domain model") {
		t.Errorf("rows = %q, want the Done slice shown after the toggle", got)
	}
}

func TestBoardHidesDoneSlicesOfUnfinishedMilestones(t *testing.T) {
	b := newTestBoard()

	b.Update(keyPress("z"))

	// M2 is still in flight, so its Done slice goes; the Claimed and Todo ones
	// stay, and so does the Unassigned group's slice, which is not Done.
	want := []string{
		"Done section",
		"M2: Board", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}
	if got := rowNames(b); !equal(got, want) {
		t.Errorf("rows = %q, want the Done slice hidden: %q", got, want)
	}

	// The toggle is a toggle: pressing it again puts them back.
	b.Update(keyPress("z"))
	if got := rowNames(b); !equal(got, []string{
		"Done section",
		"M2: Board", "Domain model", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}) {
		t.Errorf("rows = %q, want the Done slice back", got)
	}
}

func TestBoardHideDoneLeavesTheDoneSectionAlone(t *testing.T) {
	b := newTestBoard()
	b.cursor = 0
	b.Update(keyPress("enter")) // expand the Done section
	b.cursor = 1
	b.Update(keyPress("enter")) // expand M1 inside it

	b.Update(keyPress("z"))

	want := []string{
		"Done section", "M1: Config", "XDG config", "Keyring",
		"M2: Board", "Board screen", "Info view",
		"M3: Mutations",
		domain.UnassignedName, "Stray",
	}
	if got := rowNames(b); !equal(got, want) {
		t.Errorf("rows = %q, want a Done milestone still listing all its slices: %q", got, want)
	}
}

func TestBoardHideDoneSurvivesAReload(t *testing.T) {
	b := newTestBoard()
	b.Update(keyPress("z"))

	p := testProject()
	b.SetProject(&p)

	for _, name := range rowNames(b) {
		if name == "Domain model" {
			t.Fatalf("rows = %q, want the Done slice still hidden after a reload", rowNames(b))
		}
	}
}

// Hiding a slice is a rendering decision only: the milestone's own count and
// the progress math behind it still weigh every slice it has.
func TestBoardHideDoneStillCountsTheHiddenSlices(t *testing.T) {
	b := newTestBoard()
	b.Update(keyPress("z"))

	view := ansi.ReplaceAllString(b.View(), "")
	if !strings.Contains(view, "1/3") {
		t.Errorf("view is missing M2's full 1/3 count:\n%s", view)
	}
	if !strings.Contains(view, "· 1 done hidden") {
		t.Errorf("view is missing the hidden-slice cue:\n%s", view)
	}
}

// A collapsed milestone shows no slices at all, so the toggle hides nothing of
// it and it says nothing about hidden slices.
func TestBoardHideDoneCuesOnlyExpandedMilestones(t *testing.T) {
	b := newTestBoard()
	b.hideDone = true
	for k := range b.expanded {
		b.expanded[k] = false
	}
	b.rebuild()

	if view := ansi.ReplaceAllString(b.View(), ""); strings.Contains(view, "done hidden") {
		t.Errorf("a collapsed board should cue nothing:\n%s", view)
	}
}

func TestBoardHideDoneKeepsTheCursorOnAVisibleRow(t *testing.T) {
	b := newTestBoard()

	// On the Done slice itself, which is about to go: the cursor falls back to
	// its milestone's own row.
	b.cursor = 2
	b.Update(keyPress("z"))
	if got := rowNames(b)[b.cursor]; got != "M2: Board" {
		t.Errorf("cursor is on %q, want it fallen back to the milestone row", got)
	}

	// On a row that survives: the cursor follows it to its new index rather than
	// staying on a number.
	b.Update(keyPress("z"))
	b.cursor = 4 // Info view
	b.Update(keyPress("z"))
	if got := rowNames(b)[b.cursor]; got != "Info view" {
		t.Errorf("cursor is on %q, want it kept on Info view", got)
	}
}

func TestBoardHideDoneOnAnEmptyBoardDoesNothing(t *testing.T) {
	b := NewBoard(DefaultStyles())

	b.Update(keyPress("z"))

	if b.hideDone {
		t.Error("the toggle should still have flipped")
	}
	if b.cursor != 0 || len(b.rows) != 0 {
		t.Errorf("cursor = %d, rows = %d, want an empty board untouched", b.cursor, len(b.rows))
	}
}

func TestBoardRendersWithDoneSlicesHidden(t *testing.T) {
	b := newTestBoard()
	b.Update(keyPress("z"))

	golden(t, "board-hide-done", b.View())
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
	// Twice: the first pass expands the Done section, whose rebuild is what
	// seeds the fold state of the milestones inside it.
	for range 2 {
		for k := range b.expanded {
			b.expanded[k] = true
		}
		b.rebuild()
	}
	b.cursor = 3 // a slice row, so the cursor is shown on one

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
	for i, want := range map[int]string{0: "▸ Done", 1: "2 ▾ Board", 5: "3 ▸ Mutations", 6: "▾ Unassigned"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want it to contain %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[0], "1 milestone · 2/2") {
		t.Errorf("line 0 = %q, want the Done section's aggregate on it", lines[0])
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
	for i, count := range map[int]string{1: "1/3", 5: "0/0", 6: "0/1"} {
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
	for i, pill := range map[int]string{1: "Active", 5: "Queued"} {
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

// blockedBoard is testProject with the Todo slice waiting on the one in
// progress: "Info view" draws the blocked chip and every other row — "Domain
// model" among them, which waits on a slice already Done — draws none.
func blockedBoard() *Board {
	b := NewBoard(DefaultStyles())
	b.hideDone = false
	b.SetWidth(60)
	p := testProject()
	p.Slices[4].DependsOn = []string{"s4"} // Info view waits on Board screen
	p.Slices[2].DependsOn = []string{"s1"} // Domain model waited on one now Done
	b.SetProject(&p)
	return &b
}

func TestBoardMarksABlockedSlice(t *testing.T) {
	b := blockedBoard()

	golden(t, "board-blocked", b.View())

	// The chip is on the blocked row and on no other: a slice whose
	// dependencies are all Done is not waiting on anything.
	for _, line := range strings.Split(ansi.ReplaceAllString(b.View(), ""), "\n") {
		if want := strings.Contains(line, "Info view"); strings.Contains(line, "⊘ blocked") != want {
			t.Errorf("the blocked chip is on the wrong row:\n%s", line)
		}
	}
}

// A blocked row is a row like any other as the board narrows: the chip gives
// way with the rest of it rather than pushing anything off the board.
func TestBoardWrapsABlockedRow(t *testing.T) {
	b := blockedBoard()
	b.SetWidth(30)

	view := b.View()
	golden(t, "board-blocked-narrow", view)
	flat := strings.Join(strings.Fields(ansi.ReplaceAllString(view, "")), " ")
	for _, want := range []string{"Info view", "⊘ blocked"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the narrow board is missing %q:\n%s", want, view)
		}
	}
}

// A dependency the plan does not hold — a trashed slice, or one filed outside
// the project — blocks nothing: a slice waiting forever on a page nobody can
// see would never be launchable again.
func TestBoardPassesOverAnUnreadableDependency(t *testing.T) {
	b := NewBoard(DefaultStyles())
	b.SetWidth(60)
	p := testProject()
	p.Slices[4].DependsOn = []string{"gone"}
	b.SetProject(&p)

	if got := b.Blockers(p.Slices[4]); len(got) != 0 {
		t.Errorf("blockers = %+v, want none", got)
	}
	if view := ansi.ReplaceAllString(b.View(), ""); strings.Contains(view, "blocked") {
		t.Errorf("a slice waiting on a page the plan cannot see is drawn blocked:\n%s", view)
	}
}

// The index is rebuilt with the plan, so a board that had one and lost it —
// the state it starts in — answers for a slice from anywhere.
func TestBoardWithNoPlanBlocksNothing(t *testing.T) {
	b := NewBoard(DefaultStyles())

	if got := b.Blockers(domain.Slice{ID: "s5", DependsOn: []string{"s4"}}); len(got) != 0 {
		t.Errorf("blockers = %+v, want none", got)
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

func TestBoardFillsEveryLineOfASelectedWrappedRow(t *testing.T) {
	b := newLongRowBoard(40)
	b.cursor = 1 // the long slice row, which wraps at this width

	lines := b.rowLines()[1]
	if len(lines) < 2 {
		t.Fatalf("the row is %d lines, want it wrapped", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 40 {
			t.Errorf("line %d of the selected row is %d wide, want the fill run out to 40", i, got)
		}
	}
}

// TestBoardContinuesTheStatusStripDownAWrappedRow pins the strip: every line of
// a wrapped slice row carries the status cell's background, so the status reads
// as a band beside the whole row rather than a mark on its first line.
func TestBoardContinuesTheStatusStripDownAWrappedRow(t *testing.T) {
	b := newLongRowBoard(40)

	for _, r := range []struct {
		row  int
		want string
	}{{1, claimedBG}, {2, todoBG}} {
		lines := b.rowLines()[r.row]
		if len(lines) < 2 {
			t.Fatalf("row %d is %d lines, want it wrapped", r.row, len(lines))
		}
		for i, line := range lines {
			if !strings.Contains(line, r.want) {
				t.Errorf("line %d of row %d is missing the status strip %q:\n%q",
					i, r.row, r.want, line)
			}
			plain := ansi.ReplaceAllString(line, "")
			// The strip takes the same cells on every line, so the name and its
			// continuations all start at the same column.
			if i > 0 && strings.TrimLeft(plain[:8], " ") != "" {
				t.Errorf("line %d of row %d does not indent past the strip:\n%q",
					i, r.row, plain)
			}
		}
	}
}

// claimedBG and todoBG are the background sequences of the two status chips the
// long-row board draws, which is what the strip is looked for by.
var (
	claimedBG = backgroundOf(DefaultStyles().StatusClaimed)
	todoBG    = backgroundOf(DefaultStyles().StatusTodo)
)

// backgroundOf is the escape sequence a chip style paints its background with.
func backgroundOf(st lipgloss.Style) string {
	rendered := st.Render(" ")
	at := strings.Index(rendered, "48;2;")
	if at < 0 {
		return ""
	}
	return rendered[at : at+strings.Index(rendered[at:], "m")]
}

// TestBoardCursorSpanCountsAWrappedRowsLines pins what the layout scrolls by:
// the line the selected row starts on, and how many lines it takes.
func TestBoardCursorSpanCountsAWrappedRowsLines(t *testing.T) {
	b := newLongRowBoard(40)
	b.cursor = 2 // the second slice, under a milestone row that wraps too

	top, height := b.CursorSpan()
	lines := b.rowLines()
	want := len(lines[0]) + len(lines[1])
	if top != want || height != len(lines[2]) {
		t.Errorf("CursorSpan() = (%d, %d), want (%d, %d)", top, height, want, len(lines[2]))
	}
	if height < 2 {
		t.Errorf("the row is %d lines, want the span to cover a wrapped row", height)
	}
}

func TestBoardCursorSpanWithoutRows(t *testing.T) {
	b := NewBoard(DefaultStyles())

	if top, height := b.CursorSpan(); top != 0 || height != 1 {
		t.Errorf("CursorSpan() = (%d, %d), want (0, 1) on an empty board", top, height)
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
				Order: 6, Status: domain.MilestoneActive},
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
// live on its claimed slice, pulsed to [testPulseFrame] so the star it draws
// can be looked for by its glyph alone.
func newLongRowBoard(width int) *Board {
	b := NewBoard(DefaultStyles())
	b.SetWidth(width)
	b.SetLive(map[string]string{"s1": agent.SessionName("s1")})
	b.Pulse(testPulseFrame)
	p := longRowProject()
	b.SetProject(&p)
	return &b
}

// TestBoardWrapsRowsAsItNarrows pins the narrowing: a row too wide for the
// board takes another line rather than losing anything off its tail, so every
// part of it — the names, the chips, the milestone's count and pill — is still
// on the board however narrow it gets.
func TestBoardWrapsRowsAsItNarrows(t *testing.T) {
	parts := []string{"Degrade slice rows gracefully as the board narrows",
		"Keep the status bar and header inside the window",
		pulseFrames[testPulseFrame], "@Craig Johnston", "#9", "0/2", "Active"}

	// At 80 every part of the row fits on the one line.
	view := newLongRowBoard(80).View()
	golden(t, "board-narrow-80", view)
	for _, want := range parts {
		if !strings.Contains(view, want) {
			t.Errorf("at 80 the view is missing %q:\n%s", want, view)
		}
	}
	if got := len(strings.Split(view, "\n")); got != 3 {
		t.Errorf("at 80 the board is %d lines, want one per row", got)
	}

	// At 60 and at 40 the rows wrap, and nothing has gone: only the names, now
	// broken across lines, read differently.
	for _, width := range []int{60, 40} {
		b := newLongRowBoard(width)
		view := b.View()
		golden(t, fmt.Sprintf("board-narrow-%d", width), view)
		flat := strings.Join(strings.Fields(ansi.ReplaceAllString(view, "")), " ")
		for _, want := range parts {
			if !strings.Contains(flat, want) {
				t.Errorf("at %d the view is missing %q:\n%s", width, want, view)
			}
		}
		if got := len(strings.Split(view, "\n")); got <= len(b.rows) {
			t.Errorf("at %d the board is %d lines over %d rows, want them wrapped",
				width, got, len(b.rows))
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
	for _, want := range []string{pulseFrames[testPulseFrame], "@Craig Johnston", "#9", "Active"} {
		if !strings.Contains(view, want) {
			t.Errorf("an unmeasured board should draw everything, missing %q:\n%s", want, view)
		}
	}
}

// TestPRLabelNamesTheNumber pins what the chip reads: the number off the tail
// of a forge's PR URL, and the bare word when there is none to read.
func TestPRLabelNamesTheNumber(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://github.com/craigmjohnston/nat/pull/71", "#71"},
		{"https://github.com/craigmjohnston/nat/pull/71/", "#71"},
		{"https://github.com/craigmjohnston/nat/pull/71/files?w=1", "PR"},
		{"https://github.com/craigmjohnston/nat/pull/71?w=1", "#71"},
		{"https://github.com/craigmjohnston/nat/pull/71#discussion", "#71"},
		{"https://example.test/some/review/page", "PR"},
		{"", "PR"},
	} {
		if got := prLabel(tc.url); got != tc.want {
			t.Errorf("prLabel(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// prChipURL is the target of the first OSC 8 hyperlink in s, and "" when there
// is none: what a terminal would open on a click.
func prChipURL(s string) string {
	const open = "\x1b]8;;"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	end := strings.IndexByte(rest, '\a')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// TestBoardLinksThePRChip pins the chip as a hyperlink: the number is drawn,
// wrapped in an OSC 8 pointing at the PR and closed again, and the escape costs
// the row no cells — so a terminal that ignores it sees only "#9".
func TestBoardLinksThePRChip(t *testing.T) {
	const url = "https://example.test/pr/9"
	view := newLongRowBoard(0).View()

	if got := prChipURL(view); got != url {
		t.Errorf("the chip links to %q, want %q", got, url)
	}
	if !strings.Contains(view, "#9\x1b[m\x1b]8;;\a") {
		t.Errorf("the chip should close its hyperlink after the number:\n%q", view)
	}
	linked := strings.Split(view, "\n")[1]
	if got, want := lipgloss.Width(linked), lipgloss.Width(stripANSI(linked)); got != want {
		t.Errorf("the linked row measures %d cells, want %d — the escape draws nothing", got, want)
	}
}

// TestBoardLinksThePRChipOnTheSelectedRow pins the link surviving the selected
// row's fill, which draws the row's own parts plain: the colour goes, the
// hyperlink stays, because it is what the chip is for.
func TestBoardLinksThePRChipOnTheSelectedRow(t *testing.T) {
	b := newLongRowBoard(0)
	b.Update(tea.KeyPressMsg{Code: 'j'}) // onto the slice with the PR

	view := b.View()
	if got, want := prChipURL(view), "https://example.test/pr/9"; got != want {
		t.Errorf("the selected row's chip links to %q, want %q", got, want)
	}
	if !strings.Contains(stripANSI(view), "#9") {
		t.Errorf("the selected row should still name the PR:\n%s", view)
	}
}

// TestBoardDropsThePRChipLast pins the chip's rank as the board narrows: it is
// the last chip of the row, so it is the first pushed off the row's own line —
// the slice's state is worth more of a cramped row than a link out of the app.
func TestBoardDropsThePRChipLast(t *testing.T) {
	b := newLongRowBoard(43)

	// Row 1 is the slice carrying both an assignee and a PR.
	lines := b.rowLines()[1]
	if len(lines) < 2 {
		t.Fatalf("at 43 the row should have wrapped, got %q", lines)
	}
	last := stripANSI(lines[len(lines)-1])
	if !strings.Contains(last, "#9") {
		t.Errorf("the PR chip should be the part pushed onto the last line:\n%q", lines)
	}
	if strings.Contains(last, "@Craig Johnston") {
		t.Errorf("the assignee should outlast the PR chip on the lines above:\n%q", lines)
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
		"Done section", "M2: Board", "M3: Mutations", domain.UnassignedName, "Stray",
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
	if _, ok := b.SelectedMilestone(); ok {
		t.Error("the Done section is not a milestone")
	}
	b.cursor = 1
	got, ok := b.SelectedMilestone()
	if !ok || got.ID != "M2: Board" {
		t.Errorf("selected = %+v (ok=%v), want M2: Board", got, ok)
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
		{ID: "M2: Board", Name: "M2: Board", Order: 1, Status: domain.MilestoneActive},
	}}
	b.SetProject(&p)

	if b.expanded["M2: Board"] {
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
	got := strings.Join(b.renderSlice("  ", b.groups[3].Slices[0], false), "\n")
	if !strings.Contains(got, "Stray") {
		t.Errorf("render = %q, want the slice name", got)
	}
}

func TestAppHelpListsTheBoardKeys(t *testing.T) {
	app := NewApp(testConfig(), nil)
	press(app, "?")

	view := app.View().Content
	for _, want := range []string{"quit", "Board", "expand/collapse", "launch agent", "hide/show done slices"} {
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
	for _, want := range []string{"Done", "Board", "Board screen"} {
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
	case "shift+enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModShift})
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
