package tui

import "charm.land/lipgloss/v2"

// Styles is every lipgloss style the interface draws with. One value is built
// at startup and shared by the screens, so colours are defined in exactly one
// place.
//
// The palette is the terminal's own 4-bit colours rather than hex values, so
// the app sits inside whatever theme the user already runs.
type Styles struct {
	// Frame is the indent every band of the layout shares, holding its content
	// away from the window's edges.
	Frame lipgloss.Style
	// Title is the app/screen heading; Subtitle the line under it.
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	// Faint is for text that should recede: hints, placeholders, counts.
	Faint lipgloss.Style
	// StatusBar is the fill of the bottom row, and StatusKey, StatusDesc,
	// StatusNote and StatusSep the text drawn on it. Each of them carries the
	// bar's own background: a segment styled with a foreground alone would reset
	// the fill and cut a hole in it.
	StatusBar  lipgloss.Style
	StatusKey  lipgloss.Style
	StatusDesc lipgloss.Style
	StatusNote lipgloss.Style
	StatusSep  lipgloss.Style
	// Error is the status bar of a failed Notion call.
	Error lipgloss.Style
	// ModeChip is the status bar's leading segment: the project's name on the
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
	// a milestone's status word the same way.
	StatusTodo      lipgloss.Style
	StatusClaimed   lipgloss.Style
	StatusDone      lipgloss.Style
	StatusUnknown   lipgloss.Style
	MilestoneQueued lipgloss.Style
	MilestoneActive lipgloss.Style
	MilestoneDone   lipgloss.Style
	// Assignee is who holds a claimed slice; PR marks one that has a pull
	// request; Live marks one with an agent session running on it.
	Assignee lipgloss.Style
	PR       lipgloss.Style
	Live     lipgloss.Style

	// BarFill and BarFillAlt colour the done part of a progress bar segment,
	// alternating between the two so adjacent milestones stay apart; BarEmpty
	// is the part still to do, and BarBoundary the rule between segments.
	BarFill     lipgloss.Style
	BarFillAlt  lipgloss.Style
	BarEmpty    lipgloss.Style
	BarBoundary lipgloss.Style
}

// DefaultStyles returns the styles the app runs with.
func DefaultStyles() Styles {
	// chip is the shape every badge shares: a space of background either side,
	// so the fill reads as a chip rather than tinted text.
	chip := lipgloss.NewStyle().Padding(0, 1)
	// bar is the status bar's fill, which every segment drawn on it inherits.
	bar := lipgloss.NewStyle().Background(lipgloss.BrightBlack)
	return Styles{
		Frame:    lipgloss.NewStyle().Padding(0, framePadX),
		Title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightCyan),
		Subtitle: lipgloss.NewStyle().Foreground(lipgloss.Cyan),
		Faint:    lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),

		StatusBar:  bar,
		StatusKey:  bar.Bold(true).Foreground(lipgloss.BrightMagenta),
		StatusDesc: bar.Foreground(lipgloss.White),
		StatusNote: bar.Foreground(lipgloss.BrightGreen),
		StatusSep:  bar.Foreground(lipgloss.White),

		Error:    chip.Bold(true).Foreground(lipgloss.BrightWhite).Background(lipgloss.Red),
		ModeChip: chip.Foreground(lipgloss.Black).Background(lipgloss.Cyan),
		Spinner:  lipgloss.NewStyle().Foreground(lipgloss.Magenta),
		HelpKey:  lipgloss.NewStyle().Foreground(lipgloss.BrightMagenta),
		HelpDesc: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),

		Cursor:      lipgloss.NewStyle().Foreground(lipgloss.BrightMagenta),
		Selected:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightWhite),
		SelectedRow: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightWhite).Background(lipgloss.Blue),
		Milestone:   lipgloss.NewStyle().Foreground(lipgloss.Cyan),

		StatusTodo:      chip.Foreground(lipgloss.BrightWhite).Background(lipgloss.BrightBlack),
		StatusClaimed:   chip.Foreground(lipgloss.Black).Background(lipgloss.Yellow),
		StatusDone:      chip.Foreground(lipgloss.Black).Background(lipgloss.Green),
		StatusUnknown:   chip.Foreground(lipgloss.BrightBlack),
		MilestoneQueued: chip.Foreground(lipgloss.BrightWhite).Background(lipgloss.BrightBlack),
		MilestoneActive: chip.Foreground(lipgloss.Black).Background(lipgloss.Cyan),
		MilestoneDone:   chip.Foreground(lipgloss.Black).Background(lipgloss.Green),

		Assignee: lipgloss.NewStyle().Foreground(lipgloss.Blue),
		PR:       lipgloss.NewStyle().Foreground(lipgloss.Magenta),
		Live:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightGreen),

		BarFill:     lipgloss.NewStyle().Foreground(lipgloss.Cyan),
		BarFillAlt:  lipgloss.NewStyle().Foreground(lipgloss.Blue),
		BarEmpty:    lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		BarBoundary: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
	}
}
