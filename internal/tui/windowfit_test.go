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
			{ID: "m7", Name: "M7: Agent pane view", Order: 7, Status: domain.MilestoneActive},
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
	// The header is a bordered section of its own: its top border, the heading,
	// the bar — windowProject's one slice is Todo, so every cell is empty — and
	// the bar's label, naming the tally and the milestone the work is in, then
	// the border closing the section off from the board's box beneath it.
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Errorf("first line = %q, want the header box's top border", lines[0])
	}
	if !strings.Contains(lines[1], "nat") || !strings.Contains(lines[1], "notion-agent-tracker") {
		t.Errorf("heading = %q, want the app and project names", lines[1])
	}
	if strings.Contains(lines[1], "milestones:") {
		t.Errorf("heading = %q, want the text tally gone", lines[1])
	}
	if !strings.Contains(lines[2], strings.Repeat(barCell, 80-2*framePadX)) {
		t.Errorf("second header line = %q, want the bar at the box's full width", lines[2])
	}
	if !strings.Contains(lines[3], "0/1 · M7: Agent pane view") {
		t.Errorf("third header line = %q, want the bar's label", lines[3])
	}
	if !strings.HasPrefix(lines[4], "╰") || !strings.HasSuffix(lines[4], "╯") {
		t.Errorf("line under the bar = %q, want the header box closed", lines[4])
	}
	if !strings.HasPrefix(lines[5], "╭") {
		t.Errorf("line under the header box = %q, want the board's own box", lines[5])
	}
}

func TestAppHeaderBoxShedsTheBarBeforeTheBoardShedsItsRows(t *testing.T) {
	// The header's box is the first band to give anything up as the window
	// shortens: the bar's label goes, then the bar, and the heading and a row of
	// the plan are what a framed window keeps to the last.
	for _, tt := range []struct {
		height             int
		wantBar, wantLabel bool
	}{
		{24, true, true}, {11, true, false}, {10, false, false},
	} {
		view := stripANSI(sizedApp(80, tt.height).View().Content)
		if got := strings.Contains(view, strings.Repeat(barCell, 80-2*framePadX)); got != tt.wantBar {
			t.Errorf("at %d lines the bar is drawn = %v, want %v:\n%s", tt.height, got, tt.wantBar, view)
		}
		if got := strings.Contains(view, "0/1 · M7"); got != tt.wantLabel {
			t.Errorf("at %d lines the label is drawn = %v, want %v:\n%s", tt.height, got, tt.wantLabel, view)
		}
		for _, want := range []string{"nat", "Agent pane view"} {
			if !strings.Contains(view, want) {
				t.Errorf("at %d lines the view is missing %q:\n%s", tt.height, want, view)
			}
		}
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

func TestAppBoxesTheHeaderTheBoardAndTheStatusBar(t *testing.T) {
	for _, width := range windowWidths {
		lines := strings.Split(stripANSI(sizedApp(width, 24).View().Content), "\n")
		// The header takes the window's first five lines, the board's box follows
		// it, and the status box takes the last three, with the hints row on its
		// own line between the two; each box runs the window's full width.
		for _, i := range []int{0, 5, len(lines) - 3} {
			if !strings.HasPrefix(lines[i], "╭") || !strings.HasSuffix(lines[i], "╮") {
				t.Errorf("at %d columns line %d = %q, want a border's top", width, i, lines[i])
			}
		}
		for _, i := range []int{4, len(lines) - 5, len(lines) - 1} {
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

func TestAppDropsKeyHintsByRank(t *testing.T) {
	// Wide enough for every hint: the cursor starts on the milestone, and the
	// row draws its whole set, help included.
	if view := stripANSI(sizedApp(80, 24).View().Content); !strings.Contains(view, "? help") {
		t.Errorf("a wide window should draw every hint:\n%s", view)
	}

	// Narrow enough that some have to go: help and the toggle drop first, and
	// the actions that write survive.
	view := stripANSI(sizedApp(40, 24).View().Content)
	for _, gone := range []string{"? help", "enter expand/collapse"} {
		if strings.Contains(view, gone) {
			t.Errorf("at 40 columns %q should have gone:\n%s", gone, view)
		}
	}
	for _, want := range []string{"a add slice", "Q advance milestone"} {
		if !strings.Contains(view, want) {
			t.Errorf("at 40 columns the view is missing %q:\n%s", want, view)
		}
	}
}

// The hints row sits on its own line directly above the status box, and what
// it says follows the cursor: the milestone's actions on a milestone, the
// slice's on a slice, and the global set where nothing is selected.
func TestAppHintsRowIsContextual(t *testing.T) {
	a := sizedApp(80, 24)

	view := stripANSI(a.View().Content)
	lines := strings.Split(view, "\n")
	row := lines[len(lines)-4]
	for _, want := range []string{"a add slice", "Q advance milestone", "enter expand/collapse", "? help"} {
		if !strings.Contains(row, want) {
			t.Errorf("hints row on a milestone = %q, want %q", row, want)
		}
	}

	press(a, "j")
	lines = strings.Split(stripANSI(a.View().Content), "\n")
	row = lines[len(lines)-4]
	for _, want := range []string{"e edit", "m move", "d delete", "l launch agent", "t show/hide agent", "? help"} {
		if !strings.Contains(row, want) {
			t.Errorf("hints row on a slice = %q, want %q", row, want)
		}
	}
	if strings.Contains(row, "add slice") {
		t.Errorf("hints row on a slice = %q, want the milestone's hints gone", row)
	}

	press(a, "?")
	lines = strings.Split(stripANSI(a.View().Content), "\n")
	row = lines[len(lines)-4]
	for _, want := range []string{"r refresh", "esc back", "q quit"} {
		if !strings.Contains(row, want) {
			t.Errorf("hints row on the help screen = %q, want the global set with %q", row, want)
		}
	}
}

func TestHintsGoInRankOrder(t *testing.T) {
	a := NewApp(testConfig(), nil)
	// The global hints as they narrow: each drop takes the whole hint, never
	// half of one, and the order is the rank order.
	dropped := []string{"esc back", "i info", "? help", "r refresh", "q quit"}
	for width := 60; width >= 0; width-- {
		line := stripANSI(a.fitHints(a.keys.statusHints(), width))
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
	// overflowing.
	a := NewApp(testConfig(), nil)
	if got := lipgloss.Width(a.fitHints(a.keys.statusHints(), 1)); got > 1 {
		t.Errorf("hints are %d wide in one column", got)
	}
}

func TestAppStatusBarIsExactlyTheWindowWideAsItNarrows(t *testing.T) {
	// The bar fills the window's bottom row whatever it holds. Narrow enough and
	// it is the chip alone, cut to fit; the loop takes that branch too.
	for width := 1; width <= 80; width++ {
		bar := sizedApp(width, 24).statusBar()
		if got := lipgloss.Width(bar); got != width {
			t.Fatalf("at %d columns the status bar is %d wide, want the window filled",
				width, got)
		}
		if strings.Contains(bar, "\n") {
			t.Fatalf("at %d columns the status bar took more than one line", width)
		}
	}
}

func TestAppStatusBarLeadsWithTheModeChip(t *testing.T) {
	bar := stripANSI(sizedApp(80, 24).statusBar())
	// The bar is indented like every other band, and the chip names the app
	// rather than the project — the heading already does that.
	if want := "   nat "; !strings.HasPrefix(bar, want) {
		t.Errorf("status bar = %q, want it led by the chip %q", bar, want)
	}
	if strings.Contains(bar, "notion-agent-tracker") {
		t.Errorf("status bar = %q, want the project name left to the heading", bar)
	}
}

func TestAppStatusBarCarriesNoKeyHints(t *testing.T) {
	// The hints have a row of their own above the bar, so the bar is the chip
	// and the message alone.
	a := sizedApp(80, 24)
	a.note = "Saved."

	bar := stripANSI(a.statusBar())
	if strings.Contains(bar, "q quit") || strings.Contains(bar, "? help") {
		t.Errorf("status bar = %q, want the hints on their own row", bar)
	}
	if !strings.Contains(bar, "Saved.") {
		t.Errorf("status bar = %q, want the note beside the chip", bar)
	}
}

func TestAppStatusBarKeepsALongNoteInTheBar(t *testing.T) {
	// A long note is truncated to the bar rather than wrapped or overflowed.
	a := sizedApp(80, 24)
	a.note = strings.Repeat("very long ", 20)

	bar := a.statusBar()
	if got := lipgloss.Width(bar); got != 80 {
		t.Errorf("status bar is %d wide, want the window filled", got)
	}
	if !strings.Contains(stripANSI(bar), "very long") {
		t.Errorf("status bar = %q, want the note's leading text kept", stripANSI(bar))
	}
}

func TestAppHintsRowIsEmptyForAnOpenForm(t *testing.T) {
	// A form owns the keys the hints name, so only its own way out is offered,
	// on the bar itself.
	a := sizedFormApp(t, 80, 24)
	if row := stripANSI(a.hintsView()); row != "" {
		t.Errorf("hints row = %q, want it empty while a form is open", row)
	}
	bar := stripANSI(a.statusBar())
	if !strings.Contains(bar, "esc cancel") || strings.Contains(bar, "refresh") {
		t.Errorf("status bar = %q, want only the form's own prompt", bar)
	}
}

func TestAppKeepsALongErrorOnOneLine(t *testing.T) {
	const width, height = 40, 24
	a := sizedApp(width, height)
	a.Update(notionErrMsg{err: errors.New("load milestones: " + strings.Repeat("boom ", 40))})

	view := a.View().Content
	checkFits(t, view, width, height)
	// The head of the error names what failed, so it is the part that survives.
	if !strings.Contains(stripANSI(view), "load milestones") {
		t.Errorf("the error should keep its leading text:\n%s", view)
	}
}

func TestAppKeepsALongNoteOnOneLine(t *testing.T) {
	const width, height = 40, 24
	a := sizedApp(width, height)
	a.note = "Saved " + strings.Repeat("very ", 30) + "long."

	checkFits(t, a.View().Content, width, height)
}

func TestAppKeepsAMultiLineMessageOnOneLine(t *testing.T) {
	// Notion errors can carry a body with newlines in it; the status bar is one
	// line whatever it is given.
	const width, height = 60, 24
	a := sizedApp(width, height)
	a.Update(notionErrMsg{err: errors.New("save slice:\nbody\nlines")})

	view := a.View().Content
	checkFits(t, view, width, height)
	if strings.Contains(stripANSI(view), "body") {
		t.Errorf("the error's later lines should have gone:\n%s", view)
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
		Milestones: []domain.Milestone{{ID: "m1", Name: "M1: Long", Order: 1, Status: domain.MilestoneActive}},
	}
	for i := range 40 {
		p.Slices = append(p.Slices, domain.Slice{
			ID:     "s" + strconv.Itoa(i),
			Name:   "Slice number " + strconv.Itoa(i),
			Status: domain.SliceTodo, MilestoneID: "m1",
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
	// The bar lives in the header's own box now, so the viewport is the whole
	// of the body band.
	body := a.boardVP.Height()

	// Down to the last row: the board scrolls only as far as it must, so the
	// cursor lands on the bottom line of the band rather than the top.
	for range len(a.board.rows) - 1 {
		press(a, "j")
	}
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
	// The status bar takes the bottom rows first, then the header and the hints
	// row what is left: too short a window loses the body, then its borders,
	// then the hints, then the header, never the bar. From 10 lines the layout
	// is framed — the boxed header, the boxed status bar and the body's own
	// border — and below that the bands are drawn bare. The header box gives up
	// the progress bar's label, then the bar itself, before the body gives up
	// its last row.
	for _, tt := range []struct{ height, header, body int }{
		{20, 5, 9}, {12, 5, 1}, {11, 4, 1}, {10, 3, 1}, {9, 1, 6}, {5, 1, 2}, {2, 1, 0}, {1, 0, 0},
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
