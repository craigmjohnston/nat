package tui

import (
	"image/color"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// Tokens are the semantic colours every style is built from: widgets pick a
// role, never a hex value, so the whole interface restyles by changing the
// palette in exactly one place.
//
// The values are Catppuccin: Mocha on a dark terminal, Latte on a light one.
type Tokens struct {
	// Text is the interface's ordinary foreground; Muted the text that should
	// recede: hints, placeholders, counts.
	Text  color.Color
	Muted color.Color
	// Surface is the fill of bars and quiet chips; SurfaceHi the slightly
	// raised fill of the row the cursor is on.
	Surface   color.Color
	SurfaceHi color.Color
	// Accent is the interface's own colour — headings, keys, the cursor — and
	// AccentAlt a second hue for what sits beside accented text without being
	// it: milestone names, assignees. AccentDim is the accent knocked back
	// toward the surface, for what is the interface's colour but should
	// recede: the finished stretch of the progress bar.
	Accent    color.Color
	AccentAlt color.Color
	AccentDim color.Color
	// Success, Warning and Danger colour outcomes: done/live, claimed/pending
	// confirmation, and failure.
	Success color.Color
	Warning color.Color
	Danger  color.Color
	// OnFill is the text drawn over an Accent/Warning/Danger fill, where the
	// ordinary Text would not contrast.
	OnFill color.Color
}

// NewTokens returns the palette for a dark or light terminal background:
// Catppuccin Mocha or Latte.
func NewTokens(isDark bool) Tokens {
	ld := lipgloss.LightDark(isDark)
	return Tokens{
		Text:      ld(lipgloss.Color("#4c4f69"), lipgloss.Color("#cdd6f4")),
		Muted:     ld(lipgloss.Color("#8c8fa1"), lipgloss.Color("#7f849c")), // overlay1
		Surface:   ld(lipgloss.Color("#ccd0da"), lipgloss.Color("#313244")), // surface0
		SurfaceHi: ld(lipgloss.Color("#bcc0cc"), lipgloss.Color("#45475a")), // surface1
		Accent:    ld(lipgloss.Color("#8839ef"), lipgloss.Color("#cba6f7")), // mauve
		AccentAlt: ld(lipgloss.Color("#1e66f5"), lipgloss.Color("#89b4fa")), // blue
		AccentDim: ld(lipgloss.Color("#c09ef2"), lipgloss.Color("#6c5b88")), // mauve faded toward the base
		Success:   ld(lipgloss.Color("#40a02b"), lipgloss.Color("#a6e3a1")), // green
		Warning:   ld(lipgloss.Color("#df8e1d"), lipgloss.Color("#f9e2af")), // yellow
		Danger:    ld(lipgloss.Color("#d20f39"), lipgloss.Color("#f38ba8")), // red
		OnFill:    ld(lipgloss.Color("#eff1f5"), lipgloss.Color("#11111b")), // base/crust
	}
}

// Styles is every lipgloss style the interface draws with. One value is built
// at startup and shared by the screens, then rebuilt once the terminal's
// background colour is known, so colours are defined in exactly one place.
type Styles struct {
	// Frame is the indent every band of the layout shares, holding its content
	// away from the window's edges.
	Frame lipgloss.Style
	// Title is a screen's own heading; Subtitle a section head under it.
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	// Header is the fill of the heading bar; HeaderApp the app's own segment on
	// it, distinct from HeaderTitle, the project or screen name beside it; and
	// HeaderMeta the plan's tally, right-aligned on the bar. The text styles
	// carry the bar's background for the same reason the status segments do.
	Header      lipgloss.Style
	HeaderApp   lipgloss.Style
	HeaderTitle lipgloss.Style
	HeaderMeta  lipgloss.Style
	// Box frames the body region: a real border, so the layout reads as panels
	// rather than floating text.
	Box lipgloss.Style
	// TermBox frames the embedded agent terminal beside the board, and
	// TermBoxFocused the same box while the keyboard is the terminal's. Neither
	// pads: the emulator's cells map one to one onto the box's interior, and a
	// column of padding would shift every one of them. TermEdge and
	// TermEdgeFocused draw the box's own border characters where the title line
	// builds them by hand, in the colour the matching box's border has.
	TermBox         lipgloss.Style
	TermBoxFocused  lipgloss.Style
	TermEdge        lipgloss.Style
	TermEdgeFocused lipgloss.Style
	// Modal frames a form floating over the board, and Scrim redraws the board
	// behind it knocked back to one quiet colour, so the modal is the only
	// surface at full strength while it is up.
	Modal lipgloss.Style
	Scrim lipgloss.Style
	// Faint is for text that should recede: hints, placeholders, counts.
	Faint lipgloss.Style
	// StatusKey, StatusDesc and StatusNote are the text of the status line the
	// tmux bar draws under the pane. Each of them carries the bar's own
	// background: a segment styled with a foreground alone would reset the fill
	// and cut a hole in it.
	StatusKey  lipgloss.Style
	StatusDesc lipgloss.Style
	StatusNote lipgloss.Style
	// HintKey, HintDesc and HintSep are the key hints on the window's bottom
	// row, drawn on its own background rather than on a fill of their own.
	HintKey  lipgloss.Style
	HintDesc lipgloss.Style
	HintSep  lipgloss.Style
	// Error is the status line of a failed Notion call.
	Error lipgloss.Style
	// ToastSuccess, ToastWarning and ToastError are the status-bar toasts for
	// events not scoped to a row, each the severity's colour on the bar's fill.
	ToastSuccess lipgloss.Style
	ToastWarning lipgloss.Style
	ToastError   lipgloss.Style
	// ConfirmSuccess, ConfirmWarning and ConfirmError are the inline
	// confirmations anchored to a board row: a chip filled with the severity's
	// colour. Each ConfirmFade* is the ~2-cell dithered edge drawn where the
	// chip overlaps the row's content — the severity colour over the selected
	// row's fill, so the chip reads as sliding over the row.
	ConfirmSuccess     lipgloss.Style
	ConfirmWarning     lipgloss.Style
	ConfirmError       lipgloss.Style
	ConfirmFadeSuccess lipgloss.Style
	ConfirmFadeWarning lipgloss.Style
	ConfirmFadeError   lipgloss.Style
	// PromptOption and PromptFocused are the choices of an inline prompt
	// anchored to a board row: quiet chips, with the focused one — the answer
	// enter gives — filled with the accent. PromptFade is that chip's dithered
	// edge, shaped like the confirmations'.
	PromptOption  lipgloss.Style
	PromptFocused lipgloss.Style
	PromptFade    lipgloss.Style
	// ModeChip is the status line's leading segment: the project's name on the
	// board, the screen's name everywhere else.
	ModeChip lipgloss.Style
	// Spinner styles the loading indicator.
	Spinner lipgloss.Style
	// HelpKey and HelpDesc render one key binding of the help line.
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style

	// Cursor is the marker in front of the row the user is on in the setup
	// lists, and Selected the text of that row. The board row under the cursor
	// is SelectedRow instead: a background fill run out to the board's width,
	// over a row drawn plain — its parts' own colours would break the fill.
	Cursor      lipgloss.Style
	Selected    lipgloss.Style
	SelectedRow lipgloss.Style
	// Milestone is a board group's name when it is not selected.
	Milestone lipgloss.Style
	// StatusTodo, StatusClaimed and StatusDone are the chips a slice's status
	// glyph is badged with, and StatusUnknown the chip for a status this build
	// does not know. MilestoneQueued, MilestoneActive and MilestoneDone badge
	// a milestone's status word the same way. Done is a quiet chip — green
	// text on the surface fill — because a finished row should recede, not
	// glare.
	StatusTodo      lipgloss.Style
	StatusClaimed   lipgloss.Style
	StatusDone      lipgloss.Style
	StatusUnknown   lipgloss.Style
	MilestoneQueued lipgloss.Style
	MilestoneActive lipgloss.Style
	MilestoneDone   lipgloss.Style
	// Assignee is who holds a claimed slice; PR marks one that has a pull
	// request; Live marks the agent terminal whose session is still running.
	Assignee lipgloss.Style
	PR       lipgloss.Style
	Live     lipgloss.Style
	// StarDim, StarMid and StarPeak are the star of a slice with an agent
	// working on it, brightening and settling as the pulse swells, and
	// StarWaiting the steady star of one that has stopped for input — the
	// pending yellow, because it is the board's colour for something waiting on
	// a person.
	StarDim     lipgloss.Style
	StarMid     lipgloss.Style
	StarPeak    lipgloss.Style
	StarWaiting lipgloss.Style

	// BarFill colours the done part of the milestone the work is in;
	// BarFillDone the milestones already finished, receding behind it; and
	// BarEmpty the part still to do.
	BarFill     lipgloss.Style
	BarFillDone lipgloss.Style
	BarEmpty    lipgloss.Style

	// FormTheme is the huh theme the modal forms render with, built from the
	// same tokens as everything else. huh has a light/dark probe of its own,
	// but the app has already resolved the background, so the palette is baked
	// in and the probe's answer is ignored.
	FormTheme huh.Theme
}

// NewStyles builds the styles from the palette for the given background.
func NewStyles(isDark bool) Styles {
	t := NewTokens(isDark)
	// chip is the shape every badge shares: a space of background either side,
	// so the fill reads as a chip rather than tinted text.
	chip := lipgloss.NewStyle().Padding(0, 1)
	// bar is the tmux bar's fill, which every segment drawn on it inherits.
	bar := lipgloss.NewStyle().Background(t.Surface)
	return Styles{
		Frame:    lipgloss.NewStyle().Padding(0, framePadX),
		Title:    lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		Subtitle: lipgloss.NewStyle().Foreground(t.AccentAlt),
		Faint:    lipgloss.NewStyle().Foreground(t.Muted),

		Header:      bar,
		HeaderApp:   chip.Bold(true).Foreground(t.OnFill).Background(t.Accent),
		HeaderTitle: bar.Bold(true).Foreground(t.Text),
		HeaderMeta:  bar.Foreground(t.Muted),

		Box:             lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.SurfaceHi).Padding(0, 1),
		TermBox:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.SurfaceHi),
		TermBoxFocused:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent),
		TermEdge:        lipgloss.NewStyle().Foreground(t.SurfaceHi),
		TermEdgeFocused: lipgloss.NewStyle().Foreground(t.Accent),

		Modal: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.AccentDim).Padding(0, 1),
		Scrim: lipgloss.NewStyle().Foreground(t.SurfaceHi),

		StatusKey:  bar.Bold(true).Foreground(t.Accent),
		StatusDesc: bar.Foreground(t.Text),
		StatusNote: bar.Foreground(t.Success),

		HintKey:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		HintDesc: lipgloss.NewStyle().Foreground(t.Text),
		HintSep:  lipgloss.NewStyle().Foreground(t.Muted),

		Error: chip.Bold(true).Foreground(t.OnFill).Background(t.Danger),

		ToastSuccess: bar.Foreground(t.Success),
		ToastWarning: bar.Foreground(t.Warning),
		ToastError:   bar.Foreground(t.Danger),

		ConfirmSuccess:     chip.Foreground(t.OnFill).Background(t.Success),
		ConfirmWarning:     chip.Foreground(t.OnFill).Background(t.Warning),
		ConfirmError:       chip.Foreground(t.OnFill).Background(t.Danger),
		ConfirmFadeSuccess: lipgloss.NewStyle().Foreground(t.Success).Background(t.SurfaceHi),
		ConfirmFadeWarning: lipgloss.NewStyle().Foreground(t.Warning).Background(t.SurfaceHi),
		ConfirmFadeError:   lipgloss.NewStyle().Foreground(t.Danger).Background(t.SurfaceHi),

		PromptOption:  chip.Foreground(t.Text).Background(t.Surface),
		PromptFocused: chip.Bold(true).Foreground(t.OnFill).Background(t.Accent),
		PromptFade:    lipgloss.NewStyle().Foreground(t.Accent).Background(t.SurfaceHi),

		ModeChip: chip.Foreground(t.OnFill).Background(t.Accent),
		Spinner:  lipgloss.NewStyle().Foreground(t.Accent),
		HelpKey:  lipgloss.NewStyle().Foreground(t.Accent),
		HelpDesc: lipgloss.NewStyle().Foreground(t.Muted),

		Cursor:      lipgloss.NewStyle().Foreground(t.Accent),
		Selected:    lipgloss.NewStyle().Bold(true).Foreground(t.Text),
		SelectedRow: lipgloss.NewStyle().Bold(true).Foreground(t.Text).Background(t.SurfaceHi),
		Milestone:   lipgloss.NewStyle().Foreground(t.AccentAlt),

		StatusTodo:      chip.Foreground(t.Text).Background(t.Surface),
		StatusClaimed:   chip.Foreground(t.OnFill).Background(t.Warning),
		StatusDone:      chip.Foreground(t.Success).Background(t.Surface),
		StatusUnknown:   chip.Foreground(t.Muted),
		MilestoneQueued: chip.Foreground(t.Text).Background(t.Surface),
		MilestoneActive: chip.Foreground(t.OnFill).Background(t.Accent),
		MilestoneDone:   chip.Foreground(t.Success).Background(t.Surface),

		Assignee: lipgloss.NewStyle().Foreground(t.AccentAlt),
		PR:       lipgloss.NewStyle().Foreground(t.Accent),
		Live:     lipgloss.NewStyle().Bold(true).Foreground(t.Success),

		StarDim:     lipgloss.NewStyle().Foreground(t.Muted),
		StarMid:     lipgloss.NewStyle().Foreground(t.Success),
		StarPeak:    lipgloss.NewStyle().Bold(true).Foreground(t.Success),
		StarWaiting: lipgloss.NewStyle().Bold(true).Foreground(t.Warning),

		BarFill:     lipgloss.NewStyle().Foreground(t.Accent),
		BarFillDone: lipgloss.NewStyle().Foreground(t.AccentDim),
		BarEmpty:    lipgloss.NewStyle().Foreground(t.SurfaceHi),

		FormTheme: huh.ThemeFunc(func(bool) *huh.Styles { return formStyles(t) }),
	}
}

// toastStyle is the status-bar style a toast of the given severity renders in.
func (s Styles) toastStyle(sev severity) lipgloss.Style {
	switch sev {
	case sevWarning:
		return s.ToastWarning
	case sevError:
		return s.ToastError
	}
	return s.ToastSuccess
}

// confirmStyles are the chip and fade styles an inline confirmation of the
// given severity renders in.
func (s Styles) confirmStyles(sev severity) (chip, fade lipgloss.Style) {
	switch sev {
	case sevWarning:
		return s.ConfirmWarning, s.ConfirmFadeWarning
	case sevError:
		return s.ConfirmError, s.ConfirmFadeError
	}
	return s.ConfirmSuccess, s.ConfirmFadeSuccess
}

// formStyles adapts the tokens for huh's widgets, giving each part the same
// role its counterpart has in the rest of the interface.
func formStyles(t Tokens) *huh.Styles {
	s := huh.ThemeBase(true)

	s.Focused.Base = s.Focused.Base.BorderForeground(t.SurfaceHi)
	s.Focused.Card = s.Focused.Base
	s.Focused.Title = s.Focused.Title.Bold(true).Foreground(t.Accent)
	s.Focused.NoteTitle = s.Focused.NoteTitle.Bold(true).Foreground(t.Accent)
	s.Focused.Directory = s.Focused.Directory.Foreground(t.AccentAlt)
	s.Focused.File = s.Focused.File.Foreground(t.Text)
	s.Focused.Description = s.Focused.Description.Foreground(t.Muted)
	s.Focused.ErrorIndicator = s.Focused.ErrorIndicator.Foreground(t.Danger)
	s.Focused.ErrorMessage = s.Focused.ErrorMessage.Foreground(t.Danger)
	s.Focused.SelectSelector = s.Focused.SelectSelector.Foreground(t.Accent)
	s.Focused.NextIndicator = s.Focused.NextIndicator.Foreground(t.Accent)
	s.Focused.PrevIndicator = s.Focused.PrevIndicator.Foreground(t.Accent)
	s.Focused.Option = s.Focused.Option.Foreground(t.Text)
	s.Focused.MultiSelectSelector = s.Focused.MultiSelectSelector.Foreground(t.Accent)
	s.Focused.SelectedOption = s.Focused.SelectedOption.Foreground(t.Success)
	s.Focused.SelectedPrefix = s.Focused.SelectedPrefix.Foreground(t.Success)
	s.Focused.UnselectedOption = s.Focused.UnselectedOption.Foreground(t.Text)
	s.Focused.UnselectedPrefix = s.Focused.UnselectedPrefix.Foreground(t.Muted)
	s.Focused.FocusedButton = s.Focused.FocusedButton.Foreground(t.OnFill).Background(t.Accent)
	s.Focused.BlurredButton = s.Focused.BlurredButton.Foreground(t.Text).Background(t.Surface)

	s.Focused.TextInput.Cursor = s.Focused.TextInput.Cursor.Foreground(t.Accent)
	s.Focused.TextInput.Placeholder = s.Focused.TextInput.Placeholder.Foreground(t.Muted)
	s.Focused.TextInput.Prompt = s.Focused.TextInput.Prompt.Foreground(t.Accent)
	s.Focused.TextInput.Text = s.Focused.TextInput.Text.Foreground(t.Text)

	// Re-derived from the recoloured Focused, keeping the base theme's blurred
	// shape: a hidden border and no selection indicators.
	s.Blurred = s.Focused
	s.Blurred.Base = s.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	s.Blurred.Card = s.Blurred.Base
	s.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	s.Blurred.NextIndicator = lipgloss.NewStyle()
	s.Blurred.PrevIndicator = lipgloss.NewStyle()

	s.Help.ShortKey = s.Help.ShortKey.Foreground(t.Accent)
	s.Help.ShortDesc = s.Help.ShortDesc.Foreground(t.Muted)
	s.Help.ShortSeparator = s.Help.ShortSeparator.Foreground(t.Muted)
	s.Help.FullKey = s.Help.FullKey.Foreground(t.Accent)
	s.Help.FullDesc = s.Help.FullDesc.Foreground(t.Muted)
	s.Help.FullSeparator = s.Help.FullSeparator.Foreground(t.Muted)
	s.Help.Ellipsis = s.Help.Ellipsis.Foreground(t.Muted)

	s.Group.Title = s.Focused.Title
	s.Group.Description = s.Focused.Description
	return s
}

// DefaultStyles returns the styles the app starts with: the dark palette,
// which stands until the terminal answers the background-colour query.
func DefaultStyles() Styles {
	return NewStyles(true)
}
