package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestTokensSwitchWithTheBackground(t *testing.T) {
	dark, light := NewTokens(true), NewTokens(false)
	if dark.Accent == light.Accent {
		t.Error("dark and light accents are the same colour")
	}
	if dark.Text == light.Text {
		t.Error("dark and light text are the same colour")
	}
	if dark.Surface == light.Surface {
		t.Error("dark and light surfaces are the same colour")
	}
}

func TestStylesDrawFromTheTokens(t *testing.T) {
	tok := NewTokens(true)
	s := NewStyles(true)
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Title foreground", s.Title.GetForeground(), tok.Accent},
		{"Faint foreground", s.Faint.GetForeground(), tok.Muted},
		{"StatusBar background", s.StatusBar.GetBackground(), tok.Surface},
		{"StatusKey foreground", s.StatusKey.GetForeground(), tok.Accent},
		{"StatusNote foreground", s.StatusNote.GetForeground(), tok.Success},
		{"Error background", s.Error.GetBackground(), tok.Danger},
		{"ModeChip background", s.ModeChip.GetBackground(), tok.Accent},
		{"SelectedRow background", s.SelectedRow.GetBackground(), tok.SurfaceHi},
		{"StatusClaimed background", s.StatusClaimed.GetBackground(), tok.Warning},
		{"MilestoneActive background", s.MilestoneActive.GetBackground(), tok.Accent},
		{"Live foreground", s.Live.GetForeground(), tok.Success},
		{"BarFill foreground", s.BarFill.GetForeground(), tok.Accent},
		{"BarFillDone foreground", s.BarFillDone.GetForeground(), tok.AccentDim},
		{"BarEmpty foreground", s.BarEmpty.GetForeground(), tok.SurfaceHi},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// The Done chips must recede: coloured text on the quiet surface fill, never a
// solid green block.
func TestDoneChipsRecede(t *testing.T) {
	tok := NewTokens(true)
	s := NewStyles(true)
	for name, chip := range map[string]lipgloss.Style{
		"StatusDone": s.StatusDone, "MilestoneDone": s.MilestoneDone,
	} {
		if got := chip.GetBackground(); got != tok.Surface {
			t.Errorf("%s background = %v, want the surface fill %v", name, got, tok.Surface)
		}
		if got := chip.GetForeground(); got != tok.Success {
			t.Errorf("%s foreground = %v, want the success colour %v", name, got, tok.Success)
		}
	}
}

func TestDefaultStylesAreTheDarkPalette(t *testing.T) {
	if got, want := DefaultStyles().Title.GetForeground(), NewTokens(true).Accent; got != want {
		t.Errorf("default Title foreground = %v, want the dark accent %v", got, want)
	}
}

func TestFormThemeDrawsFromTheTokens(t *testing.T) {
	for _, isDark := range []bool{true, false} {
		tok := NewTokens(isDark)
		// The probe argument is ignored: the palette was resolved when the
		// styles were built, so both answers come back the same.
		for _, probe := range []bool{true, false} {
			f := NewStyles(isDark).FormTheme.Theme(probe)
			if got := f.Focused.Title.GetForeground(); got != tok.Accent {
				t.Errorf("isDark=%v probe=%v: form title foreground = %v, want %v", isDark, probe, got, tok.Accent)
			}
			if got := f.Focused.TextInput.Prompt.GetForeground(); got != tok.Accent {
				t.Errorf("isDark=%v probe=%v: prompt foreground = %v, want %v", isDark, probe, got, tok.Accent)
			}
			if got := f.Focused.FocusedButton.GetBackground(); got != tok.Accent {
				t.Errorf("isDark=%v probe=%v: focused button background = %v, want %v", isDark, probe, got, tok.Accent)
			}
			if got := f.Focused.ErrorMessage.GetForeground(); got != tok.Danger {
				t.Errorf("isDark=%v probe=%v: error foreground = %v, want %v", isDark, probe, got, tok.Danger)
			}
			if got := f.Group.Description.GetForeground(); got != tok.Muted {
				t.Errorf("isDark=%v probe=%v: description foreground = %v, want %v", isDark, probe, got, tok.Muted)
			}
		}
	}
}

func TestAppRestylesOnTheBackgroundAnswer(t *testing.T) {
	a := NewApp(testConfig(), nil)
	a.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})

	want := NewTokens(false).Accent
	if got := a.styles.Title.GetForeground(); got != want {
		t.Errorf("app Title foreground = %v, want the light accent %v", got, want)
	}
	if got := a.board.styles.Title.GetForeground(); got != want {
		t.Errorf("board Title foreground = %v, want the light accent %v", got, want)
	}
	if got := a.info.styles.Title.GetForeground(); got != want {
		t.Errorf("info Title foreground = %v, want the light accent %v", got, want)
	}
	if got := a.spinner.Style.GetForeground(); got != want {
		t.Errorf("spinner foreground = %v, want the light accent %v", got, want)
	}
}

func TestAppRestylesTheWizardToo(t *testing.T) {
	o := NewOnboarding(testConfig(), nil, nil)
	o.search = newSearchPicker(o.styles, 60, 20)
	a := NewAppWithOnboarding(testConfig(), nil, o)
	a.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})

	want := NewTokens(false).Accent
	if got := o.styles.Title.GetForeground(); got != want {
		t.Errorf("wizard Title foreground = %v, want the light accent %v", got, want)
	}
	if got := o.tree.styles.Title.GetForeground(); got != want {
		t.Errorf("tree Title foreground = %v, want the light accent %v", got, want)
	}
	if got := o.search.styles.Title.GetForeground(); got != want {
		t.Errorf("search Title foreground = %v, want the light accent %v", got, want)
	}
}
