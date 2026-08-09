package tui

import "charm.land/lipgloss/v2"

// Styles is every lipgloss style the interface draws with. One value is built
// at startup and shared by the screens, so colours are defined in exactly one
// place.
//
// The palette is the terminal's own 4-bit colours rather than hex values, so
// the app sits inside whatever theme the user already runs.
type Styles struct {
	// App is the outer frame every full-window screen is rendered into.
	App lipgloss.Style
	// Title is the app/screen heading; Subtitle the line under it.
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	// Faint is for text that should recede: hints, placeholders, counts.
	Faint lipgloss.Style
	// Note is a transient, benign message in the status bar.
	Note lipgloss.Style
	// Error is the status bar of a failed Notion call.
	Error lipgloss.Style
	// Spinner styles the loading indicator.
	Spinner lipgloss.Style
	// HelpKey and HelpDesc render one key binding of the help line.
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style
}

// DefaultStyles returns the styles the app runs with.
func DefaultStyles() Styles {
	return Styles{
		App:      lipgloss.NewStyle().Padding(1, 2),
		Title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightCyan),
		Subtitle: lipgloss.NewStyle().Foreground(lipgloss.Cyan),
		Faint:    lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		Note:     lipgloss.NewStyle().Foreground(lipgloss.Green),
		Error:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightWhite).Background(lipgloss.Red).Padding(0, 1),
		Spinner:  lipgloss.NewStyle().Foreground(lipgloss.Magenta),
		HelpKey:  lipgloss.NewStyle().Foreground(lipgloss.BrightMagenta),
		HelpDesc: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
	}
}
