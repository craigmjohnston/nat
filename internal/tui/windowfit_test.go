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

func TestAppDropsKeyHintsByRank(t *testing.T) {
	// Wide enough for every hint.
	if view := stripANSI(sizedApp(80, 24).View().Content); !strings.Contains(view, "esc back") {
		t.Errorf("a wide window should draw every hint:\n%s", view)
	}

	// Narrow enough that one has to go: back is the first to be dropped, and
	// what the user most needs stays.
	view := stripANSI(sizedApp(40, 24).View().Content)
	if strings.Contains(view, "esc back") {
		t.Errorf("at 40 columns the back hint should have gone:\n%s", view)
	}
	for _, want := range []string{"q quit", "r refresh"} {
		if !strings.Contains(view, want) {
			t.Errorf("at 40 columns the view is missing %q:\n%s", want, view)
		}
	}
}

func TestHintsGoInRankOrder(t *testing.T) {
	a := NewApp(testConfig(), nil)
	// The hints as they narrow: each drop takes the whole hint, never half of
	// one, and the order is the rank order.
	dropped := []string{"esc back", "i info", "? help", "r refresh", "q quit"}
	for width := 60; width >= 0; width-- {
		line := stripANSI(a.hintLine(width))
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
	if got := lipgloss.Width(NewApp(testConfig(), nil).hintLine(1)); got > 1 {
		t.Errorf("hints are %d wide in one column", got)
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
	for _, want := range []string{p.Name, "esc back", "q quit"} {
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
