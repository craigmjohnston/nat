package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// sizedFormApp returns an app of a given window size with the add-slice form
// open over the board, started the way the runtime starts one.
func sizedFormApp(t *testing.T, width, height int) *App {
	t.Helper()
	a := newWriteApp(&fakeNotion{})
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	feed(t, a, a.addSlice())
	if a.form == nil {
		t.Fatal("no form is open")
	}
	return a
}

// everyModal is one of each form the app opens, so the sizing tests cover the
// modal interface rather than one implementation of it.
func everyModal(t *testing.T) map[string]modal {
	t.Helper()
	m := domain.Milestone{ID: "m2", Name: "M2: Board", Status: domain.MilestoneQueued}
	s := domain.Slice{ID: "s5", Name: "Info view", Status: domain.SliceTodo, MilestoneID: "m2"}
	cfg := config.Config{ActiveProjectID: "p1", Projects: map[string]config.ProjectConfig{
		"p1": {Name: "tracker"},
		"p2": {Name: "other"},
	}}
	return map[string]modal{
		"add slice":      newAddSliceForm(DefaultStyles().FormTheme, m),
		"edit slice":     newEditSliceForm(DefaultStyles().FormTheme, s, "The brief."),
		"move slice":     newMoveSliceForm(DefaultStyles().FormTheme, s, []domain.Milestone{m}),
		"launch":         newLaunchForm(DefaultStyles().FormTheme, s, t.TempDir()),
		"attach":         newAttachForm(DefaultStyles().FormTheme, s, "nat-12345678"),
		"new project":    newNewProjectForm(DefaultStyles().FormTheme),
		"switch project": newSwitchProjectForm(DefaultStyles().FormTheme, cfg),
	}
}

func TestEveryModalTakesTheSizeItIsGiven(t *testing.T) {
	for name, f := range everyModal(t) {
		t.Run(name, func(t *testing.T) {
			const width, height = 30, 8
			f.SetSize(width, height)

			view := f.View()
			if got := lipgloss.Width(view); got > width {
				t.Errorf("form is %d columns wide, want no more than %d:\n%s", got, width, view)
			}
			// huh draws a blank line above its key hints on top of the height it
			// was given, which is what formChromeHeight leaves room for.
			if got := lipgloss.Height(view); got > height+1 {
				t.Errorf("form is %d lines tall, want no more than %d:\n%s", got, height+1, view)
			}
		})
	}
}

func TestModalsIgnoreAnUnknownSize(t *testing.T) {
	// Before the first resize there is no window to measure, so the form is left
	// to size itself rather than told it has nothing.
	for name, f := range everyModal(t) {
		t.Run(name, func(t *testing.T) {
			before := f.View()
			f.SetSize(0, 0)
			if got := f.View(); got != before {
				t.Errorf("view changed on an unknown size:\n%s", got)
			}
		})
	}
}

func TestAppSizesAnOpenFormToTheWindow(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 20}, {120, 40}} {
		a := sizedFormApp(t, size.width, size.height)

		view := a.View().Content
		if got := lipgloss.Width(view); got > size.width {
			t.Errorf("%dx%d: view is %d columns wide, want it inside the window:\n%s",
				size.width, size.height, got, view)
		}
		if got := lipgloss.Height(view); got != size.height {
			t.Errorf("%dx%d: view is %d lines tall, want the window filled exactly:\n%s",
				size.width, size.height, got, view)
		}
		// Nothing is wrapped onto a line of its own outside the frame: every line
		// starts with the frame's own padding, or is blank.
		for i, line := range strings.Split(stripANSI(view), "\n") {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "  ") {
				t.Errorf("%dx%d: line %d escapes the frame: %q", size.width, size.height, i, line)
			}
		}
	}
}

func TestAppResizesTheFormItIsShowing(t *testing.T) {
	a := sizedFormApp(t, 120, 40)
	wide := lipgloss.Width(a.form.View())

	a.Update(tea.WindowSizeMsg{Width: 40, Height: 20})

	narrow := lipgloss.Width(a.form.View())
	if narrow >= wide {
		t.Errorf("form is %d columns after narrowing from %d, want it to follow", narrow, wide)
	}
	if got := lipgloss.Height(a.View().Content); got != 20 {
		t.Errorf("view is %d lines after the resize, want the new window height of 20", got)
	}
}

func TestAppLeavesTheFormAloneWithoutAWindowSize(t *testing.T) {
	// An app that has not been sized yet still opens forms — the resize that
	// follows is what sizes them.
	a := newWriteApp(&fakeNotion{})
	width, height := a.formSize()
	if width > 0 || height > 0 {
		t.Errorf("form size = %dx%d, want an unknown size before the first resize", width, height)
	}
	feed(t, a, a.addSlice())
	if a.form == nil {
		t.Fatal("no form is open")
	}
}

func TestFormGoldenAtEachWidth(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
	}{
		{"form-narrow", 40, 20},
		{"form-wide", 100, 24},
	} {
		t.Run(tt.name, func(t *testing.T) {
			golden(t, tt.name, sizedFormApp(t, tt.width, tt.height).View().Content)
		})
	}
}
