package tui

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// windowProject is the plan the window-fitting tests render: a name and rows
// long enough that a narrow window has to cut something.
func windowProject() domain.Project {
	return domain.Project{
		ID:   testProjectID,
		Name: "notion-agent-tracker, dogfooding its own development",
		Milestones: []domain.Milestone{
			{ID: "m7", Name: "M7: Agent pane view", Order: 6, Status: domain.MilestoneActive},
		},
		Slices: []domain.Slice{
			{ID: "s1", Name: "Keep the status bar and header inside the window",
				Status: domain.SliceTodo, MilestoneID: "m7"},
		},
	}
}

// sizedApp returns an app of a given window size showing windowProject.
func sizedApp(width, height int) *App {
	a := NewApp(testConfig(), newLoadingClient())
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	p := windowProject()
	a.Update(projectLoadedMsg{project: p})
	return a
}

// windowWidths are the widths the acceptance snapshots are taken at.
var windowWidths = []int{40, 60, 80}

// checkFits asserts the view fills the window exactly and overflows no line.
func checkFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Height(view); got != height {
		t.Errorf("at %d columns the view is %d lines, want the window height of %d:\n%s",
			width, got, height, view)
	}
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("at %d columns line %d is %d wide: %q", width, i, got, stripANSI(line))
		}
	}
}

func TestAppFillsTheWindowAtEveryWidth(t *testing.T) {
	for _, width := range windowWidths {
		const height = 24
		checkFits(t, sizedApp(width, height).View().Content, width, height)
	}
}

// The same, with an agent's terminal beside the board: the split has to add up
// to the window at every width, however overflowing the frame it is drawing.
func TestAppFillsTheWindowWithATerminalOnShow(t *testing.T) {
	for _, width := range windowWidths {
		const height = 24
		a := sizedApp(width, height)
		a.launcher = &fakeLauncher{}
		term := newFakeTerm()
		term.frame = strings.Repeat(strings.Repeat("x", 200)+"\n", 40)
		a.viewer = &agentViewer{session: term, sliceID: "s1", name: "a name too long for a narrow split"}
		a.viewer.capture()
		a.resize()

		checkFits(t, a.View().Content, width, height)
	}
}

func TestAppGoldenAtEachWidth(t *testing.T) {
	for _, width := range windowWidths {
		a := sizedApp(width, 16)
		golden(t, "app-narrow-"+strconv.Itoa(width), a.View().Content)
	}
}

func TestAppHeaderBarHasADistinctAppSegment(t *testing.T) {
	a := sizedApp(80, 24)
	header := a.headerView()
	if want := a.styles.HeaderApp.Render(appName); !strings.Contains(header, want) {
		t.Errorf("header = %q, want the app's name as a segment of its own %q", header, want)
	}
	if !strings.Contains(stripANSI(header), "notion-agent-tracker") {
		t.Errorf("header = %q, want the project name beside the segment", stripANSI(header))
	}
}

func TestAppBoxesTheHeaderWithTheProgressBar(t *testing.T) {
	lines := strings.Split(stripANSI(sizedApp(80, 24).View().Content), "\n")
	// The header is a bordered section of two lines: its top border, the heading
	// — the names on the left and the plan's reading right-aligned on the same
	// line — and the bar under it, which windowProject's one Todo slice leaves
	// every cell of empty. Then the border closing the section off from the
	// board's box beneath it: there is no label line under the bar.
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Errorf("first line = %q, want the header box's top border", lines[0])
	}
	if !strings.Contains(lines[1], "nat") || !strings.Contains(lines[1], "notion-agent-tracker") {
		t.Errorf("heading = %q, want the app and project names", lines[1])
	}
	if !strings.Contains(lines[1], "M7: Agent pane view · 0/1") {
		t.Errorf("heading = %q, want the milestone and the tally on it", lines[1])
	}
	if !strings.Contains(lines[2], strings.Repeat(barCell, 80-2*framePadX)) {
		t.Errorf("second header line = %q, want the bar at the box's full width", lines[2])
	}
	if !strings.HasPrefix(lines[3], "╰") || !strings.HasSuffix(lines[3], "╯") {
		t.Errorf("line under the bar = %q, want the header box closed", lines[3])
	}
	if !strings.HasPrefix(lines[4], "╭") {
		t.Errorf("line under the header box = %q, want the board's own box", lines[4])
	}
}

// The heading's reading sits at the line's right-hand end, however much room
// the names beside it leave.
func TestAppHeadingRightAlignsThePlansReading(t *testing.T) {
	line := stripANSI(strings.Split(sizedApp(80, 24).View().Content, "\n")[1])

	if got, want := strings.TrimRight(line, " │"), "M7: Agent pane view · 0/1"; !strings.HasSuffix(got, want) {
		t.Errorf("heading = %q, want it to end with %q", got, want)
	}
}

func TestAppHeaderBoxShedsTheBarBeforeTheBoardShedsItsRows(t *testing.T) {
	// The header's box is the first band to give anything up as the window
	// shortens: the bar goes, and the heading — the names and the plan's
	// reading — and a row of the plan are what a framed window keeps to the last.
	// The thresholds count the status band's own box, which takes the bottom
	// three lines before anything above it is measured.
	for _, tt := range []struct {
		height  int
		wantBar bool
	}{
		{24, true}, {11, true}, {10, false},
	} {
		view := stripANSI(sizedApp(80, tt.height).View().Content)
		if got := strings.Contains(view, strings.Repeat(barCell, 80-2*framePadX)); got != tt.wantBar {
			t.Errorf("at %d lines the bar is drawn = %v, want %v:\n%s", tt.height, got, tt.wantBar, view)
		}
		for _, want := range []string{"nat", "Agent pane view", "M7: Agent pane view · 0/1"} {
			if !strings.Contains(view, want) {
				t.Errorf("at %d lines the view is missing %q:\n%s", tt.height, want, view)
			}
		}
	}
}

// The heading gives its reading up by degrees as the window narrows: the
// milestone's name goes first and the tally last, and what is left of the line
// is always the name of where the user is.
func TestAppHeadingShedsTheMilestoneBeforeTheTally(t *testing.T) {
	for _, tt := range []struct {
		width                    int
		wantMilestone, wantTally bool
	}{
		{80, true, true}, {40, false, true}, {12, false, false},
	} {
		line := stripANSI(headingLineOf(sizedApp(tt.width, 24)))
		if got := strings.Contains(line, "M7"); got != tt.wantMilestone {
			t.Errorf("at %d columns the milestone is named = %v, want %v: %q",
				tt.width, got, tt.wantMilestone, line)
		}
		if got := strings.Contains(line, "0/1"); got != tt.wantTally {
			t.Errorf("at %d columns the tally is drawn = %v, want %v: %q",
				tt.width, got, tt.wantTally, line)
		}
		if !strings.Contains(line, "nat") {
			t.Errorf("at %d columns the heading = %q, want the app's segment kept", tt.width, line)
		}
	}
}

// headingLineOf is the heading line of a framed window: the line under the header
// box's top border.
func headingLineOf(a *App) string {
	return strings.Split(a.View().Content, "\n")[1]
}

// A plan with nothing in it has no reading to take: a tally of nothing says
// nothing, and the heading is the names alone.
func TestAppHeadingHasNoReadingWithoutAPlanToSum(t *testing.T) {
	a := NewApp(testConfig(), newLoadingClient())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.Update(projectLoadedMsg{project: domain.Project{ID: testProjectID, Name: "empty"}})

	if got := stripANSI(headingLineOf(a)); strings.Contains(got, "0/0") {
		t.Errorf("heading = %q, want no tally on a plan with no slices", got)
	}
}

func TestAppHeaderKeepsTheBarOnEveryScreen(t *testing.T) {
	// The header is a band of its own, so what it shows does not move as screens
	// are pushed over the board: only the name beside the app's segment changes.
	for _, tt := range []struct{ key, name string }{{"?", "Keys"}, {"i", "Info"}} {
		a := sizedApp(80, 24)
		press(a, tt.key)

		lines := strings.Split(stripANSI(a.View().Content), "\n")
		if !strings.Contains(lines[1], tt.name) {
			t.Errorf("heading = %q, want the screen named %q", lines[1], tt.name)
		}
		if !strings.Contains(lines[2], strings.Repeat(barCell, 80-2*framePadX)) {
			t.Errorf("on %s the header line = %q, want the bar still there", tt.name, lines[2])
		}
	}
}

func TestAppProgressBarResizesWithTheWindow(t *testing.T) {
	for _, width := range windowWidths {
		view := stripANSI(sizedApp(width, 24).View().Content)
		if !strings.Contains(view, strings.Repeat(barCell, width-2*framePadX)) {
			t.Errorf("at %d columns the bar should span the body's width:\n%s", width, view)
		}
	}
}

func TestAppBoxesTheHeaderAndTheBoard(t *testing.T) {
	for _, width := range windowWidths {
		a := sizedApp(width, 24)
		lines := strings.Split(stripANSI(a.View().Content), "\n")
		// The header takes the window's first four lines and the board's box
		// follows it, closing over the hints — as many lines as they wrapped onto
		// — and the status band's own box on the bottom three; each box runs the
		// window's full width, and the last line of the window is nat's own
		// bottom border, with no row left under it.
		hints := a.hintBandHeight()
		last := len(lines) - 1
		for _, i := range []int{0, 4, last - 2} {
			if !strings.HasPrefix(lines[i], "╭") || !strings.HasSuffix(lines[i], "╮") {
				t.Errorf("at %d columns line %d = %q, want a border's top", width, i, lines[i])
			}
		}
		for _, i := range []int{3, last - statusBoxHeight - hints, last} {
			if !strings.HasPrefix(lines[i], "╰") || !strings.HasSuffix(lines[i], "╯") {
				t.Errorf("at %d columns line %d = %q, want a border's bottom", width, i, lines[i])
			}
		}
	}
}

func TestAppTooNarrowForBordersDrawsBareBands(t *testing.T) {
	// Below the framed threshold a border would crowd out the content, so the
	// bands are drawn bare, the way a too-short window's are.
	view := sizedApp(4, 24).View().Content
	checkFits(t, view, 4, 24)
	if strings.Contains(view, "╭") {
		t.Errorf("a 4-column window should have no borders:\n%s", view)
	}
}

func TestClipLines(t *testing.T) {
	for _, tt := range []struct {
		s    string
		n    int
		want string
	}{
		{"a\nb\nc", 2, "a\nb"},
		{"a\nb", 3, "a\nb"},
		{"a\nb", 0, ""},
		{"a\nb", -1, ""},
	} {
		if got := clipLines(tt.s, tt.n); got != tt.want {
			t.Errorf("clipLines(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestAppWrapsKeyHintsThenDropsThemByRank(t *testing.T) {
	// Wide enough for every hint on one line: the cursor starts on the
	// milestone, and the row draws its whole set, help included.
	if view := stripANSI(sizedApp(80, 24).View().Content); !strings.Contains(view, "? help") {
		t.Errorf("a wide window should draw every hint:\n%s", view)
	}

	// Narrow enough that they no longer fit on one line: they wrap onto the
	// next rather than going, so nothing is lost.
	view := stripANSI(sizedApp(40, 24).View().Content)
	for _, want := range []string{"a add slice", "enter expand/collapse", "z show done", "? help"} {
		if !strings.Contains(view, want) {
			t.Errorf("at 40 columns the view is missing %q:\n%s", want, view)
		}
	}

	// Narrow and short together: with the body down to its last rows the hints
	// have only the one line to wrap onto, and the ranks decide what goes.
	view = stripANSI(sizedApp(40, 10).View().Content)
	if strings.Contains(view, "? help") {
		t.Errorf("in a 40x10 window help should have gone:\n%s", view)
	}
	if !strings.Contains(view, "a add slice") {
		t.Errorf("in a 40x10 window the view is missing the write keys:\n%s", view)
	}
}

// The hints sit on the window's bottom lines, and what they say follows the
// cursor: the milestone's actions on a milestone, the slice's on a slice, and
// the global set where nothing is selected.
func TestAppHintsRowIsContextual(t *testing.T) {
	a := sizedApp(80, 24)

	// hintRows is the hints band as one string, however many lines it wrapped
	// onto, taken from above the status band's box at the bottom of the window.
	hintRows := func() string {
		lines := strings.Split(stripANSI(a.View().Content), "\n")
		end := len(lines) - statusBoxHeight
		return strings.Join(lines[end-a.hintBandHeight():end], "\n")
	}

	rows := hintRows()
	for _, want := range []string{"a add slice", "enter expand/collapse", "z show done", "? help"} {
		if !strings.Contains(rows, want) {
			t.Errorf("hints on a milestone = %q, want %q", rows, want)
		}
	}

	press(a, "j")
	rows = hintRows()
	for _, want := range []string{"e edit", "m move", "d delete", "l launch agent", "t show/hide agent", "? help"} {
		if !strings.Contains(rows, want) {
			t.Errorf("hints on a slice = %q, want %q", rows, want)
		}
	}
	if strings.Contains(rows, "add slice") {
		t.Errorf("hints on a slice = %q, want the milestone's hints gone", rows)
	}

	press(a, "?")
	rows = hintRows()
	for _, want := range []string{"r refresh", "esc back", "q quit"} {
		if !strings.Contains(rows, want) {
			t.Errorf("hints on the help screen = %q, want the global set with %q", rows, want)
		}
	}
}

// The board-wide hide-done toggle is named on both the milestone and the slice
// hints, and names what the key would do next rather than the state the board
// is already in.
func TestAppHintsNameTheHideDoneToggle(t *testing.T) {
	a := sizedApp(80, 24)

	for _, hints := range [][]hint{a.board.milestoneHints(), a.board.sliceHints()} {
		// The board starts with them hidden, so the key shows them.
		if line := stripANSI(strings.Join(a.wrapHints(hints, 100, 1), "\n")); !strings.Contains(line, "z show done") {
			t.Errorf("hints = %q, want the toggle offering to show", line)
		}
	}

	press(a, "z")
	for _, hints := range [][]hint{a.board.milestoneHints(), a.board.sliceHints()} {
		if line := stripANSI(strings.Join(a.wrapHints(hints, 100, 1), "\n")); !strings.Contains(line, "z hide done") {
			t.Errorf("hints = %q, want the toggle offering to hide once they are shown", line)
		}
	}
}

// The toggle is the first hint to go when even wrapping cannot fit them all —
// it acts on the whole board rather than on the row the rest are about, so it
// goes ahead even of the way to the help screen.
func TestAppHintsDropTheHideDoneToggleFirst(t *testing.T) {
	a := sizedApp(80, 24)

	hints := append(a.board.milestoneHints(), hint{a.keys.Help, 2})
	// One line only, and not wide enough for the set: the ranks decide what goes.
	line := stripANSI(strings.Join(a.wrapHints(hints, 45, 1), "\n"))
	if strings.Contains(line, "done") {
		t.Errorf("hints = %q, want the toggle dropped first", line)
	}
	if !strings.Contains(line, "? help") {
		t.Errorf("hints = %q, want help to outlive the toggle", line)
	}
}

// Hints wrap before they drop: a window too narrow for them all on one line
// stacks them onto the lines it has, and no hint is ever broken across two.
func TestHintsWrapRatherThanDrop(t *testing.T) {
	a := NewApp(testConfig(), nil)
	hints := a.keys.statusHints()
	whole := []string{"esc back", "i info", "? help", "r refresh", "q quit"}

	// Down to 20 columns the whole set still stacks onto the lines it is allowed;
	// below that even a stack cannot hold them, and they drop by rank instead —
	// which is what TestHintsGoInRankOrder pins.
	for width := 60; width >= 20; width-- {
		lines := a.wrapHints(hints, width, hintsMaxHeight)
		if len(lines) > hintsMaxHeight {
			t.Fatalf("at %d columns the hints take %d lines", width, len(lines))
		}
		for _, line := range lines {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("at %d columns a wrapped line is %d wide: %q", width, got, stripANSI(line))
			}
		}
		// Down to the width of the widest hint the stack still holds them all,
		// each whole rather than split over the line break.
		block := stripANSI(strings.Join(lines, "\n"))
		for _, hint := range whole {
			if !strings.Contains(block, hint) {
				t.Fatalf("at %d columns %q was dropped or split:\n%s", width, hint, block)
			}
		}
	}
}

// The hints band grows with the wrapping and the body gives up the lines,
// rather than the hints being cut off at one line.
func TestAppHintsBandGrowsAsTheyWrap(t *testing.T) {
	wide, narrow := sizedApp(120, 24), sizedApp(44, 24)

	if got := wide.hintBandHeight(); got != 1 {
		t.Errorf("hint band = %d lines at 120 columns, want them all on one", got)
	}
	if got := narrow.hintBandHeight(); got < 2 {
		t.Errorf("hint band = %d lines at 44 columns, want them wrapped onto more", got)
	}
	// The body pays for the extra lines, and the window still adds up exactly.
	checkFits(t, narrow.View().Content, 44, 24)
	if got := narrow.bodyBoxHeight(); got < bodyBoxMin {
		t.Errorf("body box = %d lines, want the hints to stop at %d", got, bodyBoxMin)
	}
}

// A window with no lines to spare keeps the hints to one and falls back to
// dropping them by rank, so what is left is still whole.
func TestHintsGoInRankOrder(t *testing.T) {
	a := NewApp(testConfig(), nil)
	// The global hints as they narrow onto a single line: each drop takes the
	// whole hint, never half of one, and the order is the rank order.
	dropped := []string{"esc back", "i info", "? help", "r refresh", "q quit"}
	for width := 60; width >= 0; width-- {
		line := stripANSI(strings.Join(a.wrapHints(a.keys.statusHints(), width, 1), "\n"))
		if width > 0 && lipgloss.Width(line) > width {
			t.Fatalf("at %d columns the hints are %d wide: %q", width, lipgloss.Width(line), line)
		}
		// Whatever is still drawn is drawn whole, and only from the tail of the
		// drop order — no hint outlives one ranked above it.
		var gone int
		for _, hint := range dropped {
			if strings.Contains(line, hint) {
				break
			}
			gone++
		}
		for _, hint := range dropped[gone:] {
			if !strings.Contains(line, hint) {
				t.Fatalf("at %d columns %q went before %q: %q", width, hint, dropped[gone], line)
			}
		}
	}
}

func TestHintsAreTruncatedOnceThereIsNothingLeftToDrop(t *testing.T) {
	// One column holds no whole hint, so what is left is cut to fit rather than
	// overflowing — on however many lines it is allowed.
	a := NewApp(testConfig(), nil)
	for _, lines := range []int{1, hintsMaxHeight} {
		for _, line := range a.wrapHints(a.keys.statusHints(), 1, lines) {
			if got := lipgloss.Width(line); got > 1 {
				t.Errorf("hints are %d wide in one column over %d lines", got, lines)
			}
		}
	}
}

// A band with no room for the hints at all draws none, rather than one line it
// has not got.
func TestHintsWithNoRoomDrawNothing(t *testing.T) {
	a := NewApp(testConfig(), nil)
	if lines := a.wrapHints(a.keys.statusHints(), 40, 0); lines != nil {
		t.Errorf("hints = %q, want none", lines)
	}
}

func TestAppStatusLineStaysWithinTheWindowAsItNarrows(t *testing.T) {
	// The line also goes out as the terminal title: one line, never wider than
	// the window it sits in. Narrow enough and it is the chip alone, cut to fit;
	// the loop takes that branch too.
	for width := 1; width <= 80; width++ {
		line := sizedApp(width, 24).windowTitle()
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("at %d columns the status line is %d wide, want it cut to the window",
				width, got)
		}
		if strings.Contains(line, "\n") {
			t.Fatalf("at %d columns the status line took more than one line", width)
		}
	}
}

// The board carries no chip: the heading names the app already. A screen over
// it leads with its own name instead.
func TestAppStatusLineChipsOnlyTheScreensOverTheBoard(t *testing.T) {
	a := sizedApp(80, 24)
	if line := a.windowTitle(); strings.Contains(line, "nat") {
		t.Errorf("status line = %q, want no chip naming the app on the board", line)
	}
	if line := stripANSI(a.statusBandLine()); strings.HasPrefix(line, " ") {
		t.Errorf("status band = %q, want the line to start where the header's does", line)
	}

	press(a, "?")
	if line := a.windowTitle(); !strings.HasPrefix(line, "help") {
		t.Errorf("status line = %q, want the help screen's chip leading it", line)
	}
	if line := stripANSI(a.statusBandLine()); !strings.Contains(line, "help") {
		t.Errorf("status band = %q, want the chip drawn on it", line)
	}
}

// The title the terminal window takes is the band's own line as plain text: a
// title is text, and a terminal shows escape codes rather than obeying them.
func TestAppWindowTitleIsThePlainStatusLine(t *testing.T) {
	a := sizedApp(80, 24)
	a.note = "Saved."

	line := a.windowTitle()
	if !strings.Contains(line, "Saved.") {
		t.Errorf("window title = %q, want what the band says", line)
	}
	if strings.Contains(line, "notion-agent-tracker") {
		t.Errorf("window title = %q, want the project name left to the heading", line)
	}
	if line != stripANSI(line) {
		t.Errorf("window title = %q, want it plain text", line)
	}
}

// statusBandLine is the one line inside the status band's box.
func (a *App) statusBandLine() string {
	region := a.statusRegion()
	return region[1]
}

func TestAppStatusLineCarriesNoKeyHints(t *testing.T) {
	// The hints have a row of their own in the window, so the line is the chip
	// and the message alone.
	a := sizedApp(80, 24)
	a.note = "Saved."

	line := a.windowTitle()
	if strings.Contains(line, "q quit") || strings.Contains(line, "? help") {
		t.Errorf("status line = %q, want the hints left to their own row", line)
	}
	if !strings.Contains(line, "Saved.") {
		t.Errorf("status line = %q, want the note beside the chip", line)
	}
}

func TestAppStatusLineKeepsALongNoteToOneLine(t *testing.T) {
	// A long note is truncated to the window rather than wrapped or overflowed.
	a := sizedApp(80, 24)
	a.note = strings.Repeat("very long ", 20)

	line := a.windowTitle()
	if got := lipgloss.Width(line); got > 80 {
		t.Errorf("status line is %d wide, want it cut to the window", got)
	}
	if !strings.Contains(line, "very long") {
		t.Errorf("status line = %q, want the note's leading text kept", line)
	}
}

// The window ends at nat's own bottom border, with the status band boxed above
// it and no blank row under it.
func TestAppDrawsItsOwnBoxedStatusBand(t *testing.T) {
	a := sizedApp(80, 24)
	a.note = "Saved."

	lines := strings.Split(stripANSI(a.View().Content), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Errorf("last line = %q, want nat's own bottom border", last)
	}
	band := lines[len(lines)-2]
	if !strings.Contains(band, "Saved.") {
		t.Errorf("status band = %q, want the note drawn on it", band)
	}
	if !strings.HasPrefix(lines[len(lines)-3], "╭") {
		t.Errorf("line above the band = %q, want the box opened", lines[len(lines)-3])
	}
}

func TestAppHintsRowIsEmptyForAnOpenForm(t *testing.T) {
	// A form owns the keys the hints name, so only its own way out is offered,
	// on the status line itself.
	a := sizedFormApp(t, 80, 24)
	if row := stripANSI(a.hintsView()); row != "" {
		t.Errorf("hints row = %q, want it empty while a form is open", row)
	}
	line := stripANSI(a.statusBandLine())
	if !strings.Contains(line, "esc cancel") || strings.Contains(line, "refresh") {
		t.Errorf("status line = %q, want only the form's own prompt", line)
	}
}

func TestAppKeepsALongErrorOnOneLine(t *testing.T) {
	const width, height = 40, 24
	a := sizedApp(width, height)
	a.Update(notionErrMsg{err: errors.New("load milestones: " + strings.Repeat("boom ", 40))})

	checkFits(t, a.View().Content, width, height)
	// The head of the error names what failed, so it is the part that survives
	// the cut to the one line the bar has for it.
	line := a.windowTitle()
	if !strings.Contains(line, "load milestones") {
		t.Errorf("status line = %q, want the error's leading text", line)
	}
	if lipgloss.Width(line) > width || strings.Contains(line, "\n") {
		t.Errorf("status line = %q, want one line cut to the window", line)
	}
}

func TestAppKeepsALongNoteOnOneLine(t *testing.T) {
	const width, height = 40, 24
	a := sizedApp(width, height)
	a.note = "Saved " + strings.Repeat("very ", 30) + "long."

	checkFits(t, a.View().Content, width, height)
}

func TestAppKeepsAMultiLineMessageOnOneLine(t *testing.T) {
	// Notion errors can carry a body with newlines in it; the status line is one
	// line whatever it is given, and a title with a break in it would be two.
	const width, height = 60, 24
	a := sizedApp(width, height)
	a.Update(notionErrMsg{err: errors.New("save slice:\nbody\nlines")})

	checkFits(t, a.View().Content, width, height)
	line := a.windowTitle()
	if !strings.Contains(line, "save slice:") {
		t.Errorf("status line = %q, want the error's first line", line)
	}
	if strings.Contains(line, "body") || strings.Contains(line, "\n") {
		t.Errorf("status line = %q, want the error's later lines gone", line)
	}
}

func TestAppKeepsALongFormHeadingInTheWindow(t *testing.T) {
	const width, height = 40, 24
	a := sizedFormApp(t, width, height)

	checkFits(t, a.View().Content, width, height)
}

func TestAppKeepsALongProjectNameInTheWindow(t *testing.T) {
	const width, height = 40, 24
	a := sizedApp(width, height)

	view := stripANSI(a.View().Content)
	checkFits(t, a.View().Content, width, height)
	if !strings.Contains(view, "notion-agent-tracker") {
		t.Errorf("the project name should keep its leading text:\n%s", view)
	}
}

func TestAppKeepsTheOtherBoardStatesInTheWindow(t *testing.T) {
	const width, height = 40, 24
	tests := map[string]func(*App){
		"loading":    func(a *App) { a.loading, a.project = true, nil },
		"no project": func(a *App) { a.project, a.cfg.ActiveProjectID = nil, strings.Repeat("id-", 30) },
	}
	for name, set := range tests {
		t.Run(name, func(t *testing.T) {
			a := sizedApp(width, height)
			set(a)
			checkFits(t, a.View().Content, width, height)
		})
	}
}

func TestAppWithoutAWindowSizeDrawsEverything(t *testing.T) {
	// Before the first resize there is nothing to fit to, so nothing is cut.
	a := NewApp(testConfig(), newLoadingClient())
	p := windowProject()
	a.Update(projectLoadedMsg{project: p})

	if got := a.innerWidth(); got != 0 {
		t.Errorf("inner width = %d, want an unmeasured window", got)
	}
	view := stripANSI(a.View().Content)
	for _, want := range []string{p.Name, "a add slice", "? help"} {
		if !strings.Contains(view, want) {
			t.Errorf("an unmeasured window is missing %q:\n%s", want, view)
		}
	}
}

func TestAppInnerWidthNeverGoesNegative(t *testing.T) {
	// A window narrower than the frame's own padding leaves nothing inside it.
	a := NewApp(testConfig(), nil)
	a.Update(tea.WindowSizeMsg{Width: 2, Height: 10})

	if got := a.innerWidth(); got != 0 {
		t.Errorf("inner width = %d, want 0", got)
	}
}

func TestAppInfoScreenReasonFitsTheWindow(t *testing.T) {
	const width, height = 40, 24
	a := NewApp(config.Config{ActiveProjectID: strings.Repeat("id-", 30)}, nil)
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	press(a, "i")

	checkFits(t, a.View().Content, width, height)
}

// tallProject is a plan with far more slices than a short window has lines, so
// the board has to be scrolled to see all of it.
func tallProject() domain.Project {
	p := domain.Project{
		ID:         testProjectID,
		Name:       "tracker",
		Milestones: []domain.Milestone{{ID: "M1: Long", Name: "M1: Long", Order: 1, Status: domain.MilestoneActive}},
	}
	for i := range 40 {
		p.Slices = append(p.Slices, domain.Slice{
			ID:     "s" + strconv.Itoa(i),
			Name:   "Slice number " + strconv.Itoa(i),
			Status: domain.SliceTodo, MilestoneID: "M1: Long",
		})
	}
	return p
}

// tallApp returns an app of a given window size showing tallProject.
func tallApp(width, height int) *App {
	a := NewApp(testConfig(), newLoadingClient())
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	p := tallProject()
	a.Update(projectLoadedMsg{project: p})
	return a
}

func TestAppEveryScreenFitsASmallWindow(t *testing.T) {
	// 80×20 is the small window the layout has to hold every screen inside.
	const width, height = 80, 20
	tests := map[string]func(*App){
		"board": func(*App) {},
		"help":  func(a *App) { press(a, "?") },
		"info":  func(a *App) { press(a, "i") },
		"form":  func(a *App) { a.board.cursor = 0; press(a, "a") },
	}
	for name, open := range tests {
		t.Run(name, func(t *testing.T) {
			a := tallApp(width, height)
			open(a)
			checkFits(t, a.View().Content, width, height)
		})
	}
}

func TestAppClipsAPlanTallerThanTheWindow(t *testing.T) {
	const width, height = 80, 20
	a := tallApp(width, height)

	view := stripANSI(a.View().Content)
	checkFits(t, view, width, height)
	if !strings.Contains(view, "Slice number 0") {
		t.Errorf("the plan should be shown from the top:\n%s", view)
	}
	if strings.Contains(view, "Slice number 39") {
		t.Errorf("a row past the window should have been clipped:\n%s", view)
	}
}

func TestAppScrollsTheBoardToKeepTheCursorVisible(t *testing.T) {
	const width, height = 80, 20
	a := tallApp(width, height)

	// Down to the last row: the board scrolls only as far as it must, so the
	// cursor lands on the bottom line of the band rather than the top.
	for range len(a.board.rows) - 1 {
		press(a, "j")
	}
	// The bar lives in the header's own box now, so the viewport is the whole
	// of the body band — measured here, since the hints row a slice draws is
	// taller than a milestone's and the band pays for it.
	body := a.boardVP.Height()
	if got, want := a.boardVP.YOffset(), len(a.board.rows)-body; got != want {
		t.Errorf("offset = %d, want %d — the least scroll that shows the cursor", got, want)
	}
	view := stripANSI(a.View().Content)
	if !strings.Contains(view, "Slice number 39") {
		t.Errorf("the cursor's row should be on screen:\n%s", view)
	}

	// And back to the top the same way.
	for range len(a.board.rows) - 1 {
		press(a, "k")
	}
	if got := a.boardVP.YOffset(); got != 0 {
		t.Errorf("offset = %d, want the board back at the top", got)
	}
	if view := stripANSI(a.View().Content); !strings.Contains(view, "Slice number 0") {
		t.Errorf("the cursor's row should be on screen:\n%s", view)
	}
}

// wrappedApp returns an app of a given window size whose slice rows are too
// long for it, so every one of them wraps onto continuation lines.
func wrappedApp(width, height int) *App {
	a := NewApp(testConfig(), newLoadingClient())
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	p := tallProject()
	for i := range p.Slices {
		p.Slices[i].Name += ", named at a length that no narrow board can hold on one line"
	}
	a.Update(projectLoadedMsg{project: p})
	return a
}

// TestAppScrollsAWrappedRowOnScreenWhole pins the scrolling against rows that
// are more than one line: moving onto one brings all of it into the band, not
// just the line the cursor marker is on.
func TestAppScrollsAWrappedRowOnScreenWhole(t *testing.T) {
	a := wrappedApp(40, 20)

	for range len(a.board.rows) - 1 {
		press(a, "j")
	}

	top, rows := a.board.CursorSpan()
	if rows < 2 {
		t.Fatalf("the cursor's row is %d lines, want it wrapped", rows)
	}
	off, band := a.boardVP.YOffset(), a.boardVP.Height()
	if top < off || top+rows > off+band {
		t.Errorf("the row spans lines %d..%d, outside the band %d..%d", top, top+rows, off, off+band)
	}
	if view := stripANSI(a.View().Content); !strings.Contains(view, "Slice number 39") {
		t.Errorf("the cursor's row should be on screen:\n%s", view)
	}
}

// TestAppScrollsToTheTopOfARowTallerThanTheBand pins the one row that cannot
// come on screen whole: its first line wins, since that is the one carrying the
// cursor marker.
func TestAppScrollsToTheTopOfARowTallerThanTheBand(t *testing.T) {
	a := wrappedApp(24, 9)
	band := a.boardVP.Height()

	for range len(a.board.rows) - 1 {
		press(a, "j")
	}

	top, rows := a.board.CursorSpan()
	if rows <= band {
		t.Fatalf("the cursor's row is %d lines in a band of %d, want it taller", rows, band)
	}
	if got := a.boardVP.YOffset(); got != top {
		t.Errorf("offset = %d, want %d — the top of the row the cursor is on", got, top)
	}
}

func TestAppScrollsTheHelpScreen(t *testing.T) {
	a := tallApp(80, 20)
	press(a, "?")

	press(a, "j")

	if got := a.helpVP.YOffset(); got != 1 {
		t.Errorf("offset = %d, want the key routed to the help screen", got)
	}
	// The key list is longer than the window, and paging down reaches its end.
	for !a.helpVP.AtBottom() {
		press(a, "f")
	}
	view := stripANSI(a.View().Content)
	checkFits(t, view, 80, 20)
	if !strings.Contains(view, "page down") {
		t.Errorf("the keys past the window's bottom should be reachable:\n%s", view)
	}
}

func TestAppSharesAShortWindowOutFromTheBottom(t *testing.T) {
	// The status band takes the bottom rows first, then the header its own, then
	// the hints row, and the body has what is left: too short a window loses the
	// body, then its borders, then the hints, and the heading is the one line it
	// keeps to the last — under it, at a single line, only the bare status line
	// is left. From 10 lines the layout is framed — the boxed header, the body's
	// own border and the band's box — and below that the bands are drawn bare.
	// The header box gives up the progress bar before the body gives up its last
	// row; the heading under it carries the plan's tally either way.
	//
	// At 80 columns the hints fit on one line, so the body keeps the row they
	// would otherwise wrap onto.
	for _, tt := range []struct{ height, header, body int }{
		{20, 4, 10}, {12, 4, 2}, {11, 4, 1}, {10, 3, 1}, {9, 1, 6}, {6, 1, 3}, {2, 1, 0}, {1, 0, 0},
	} {
		a := tallApp(80, tt.height)
		if got := a.headerBandHeight(); got != tt.header {
			t.Errorf("at %d lines the header is %d, want %d", tt.height, got, tt.header)
		}
		if got := a.bodyHeight(); got != tt.body {
			t.Errorf("at %d lines the body is %d, want %d", tt.height, got, tt.body)
		}
		checkFits(t, a.View().Content, 80, tt.height)
	}
}
