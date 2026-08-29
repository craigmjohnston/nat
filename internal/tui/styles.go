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
	// Working is the colour of an agent getting on with a slice: the orange
	// Claude Code itself thinks in, taken from the palette rather than from
	// Claude's own brand hex, so it restyles with every other token. It is its
	// own role because none of the outcome colours mean this — an agent at work
	// is neither a success nor a warning — and because it has to read apart
	// from the Warning an agent stopped for input is marked with.
	Working color.Color
	// OnFill is the text drawn over an Accent/Warning/Danger fill, where the
	// ordinary Text would not contrast.
	OnFill color.Color
	// SuccessWash and DangerWash are the outcome colours knocked almost all the
	// way back to the terminal's own background, for a whole row tinted rather
	// than text coloured: the added and removed lines of a syntax-highlighted
	// diff, whose foreground belongs to the code and so cannot be the green and
	// red as well. They are washes rather than fills — text of any colour has to
	// stay readable on them, which is what rules out Success and Danger
	// themselves.
	SuccessWash color.Color
	DangerWash  color.Color
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
		Working:   ld(lipgloss.Color("#fe640b"), lipgloss.Color("#fab387")), // peach
		OnFill:    ld(lipgloss.Color("#eff1f5"), lipgloss.Color("#11111b")), // base/crust
		// The green and the red mixed a fifth of the way into the base each
		// palette is drawn on, which is as far as a wash can go before the code
		// on top of it stops reading.
		SuccessWash: ld(lipgloss.Color("#d0e2d1"), lipgloss.Color("#24352d")),
		DangerWash:  ld(lipgloss.Color("#eac8d3"), lipgloss.Color("#3e1b30")),
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
	// HeaderMeta the plan's tally, right-aligned on the bar, with
	// HeaderMilestone naming the milestone the work is in beside it. The text
	// styles carry the bar's own background: a segment styled with a foreground
	// alone would reset the fill and cut a hole in it.
	Header          lipgloss.Style
	HeaderApp       lipgloss.Style
	HeaderTitle     lipgloss.Style
	HeaderMeta      lipgloss.Style
	HeaderMilestone lipgloss.Style
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
	// app draws in its bottom band. None of them fills a background: the band is
	// a bordered section like the header and the body, and a fill under the line
	// would read as a bar sitting inside the frame rather than as part of it.
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
	// ToastSuccess, ToastWarning and ToastError are the status band's toasts for
	// events not scoped to a row, each the severity's colour on the band's own
	// background.
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
	// ModeChip is the status line's leading segment: the name of the screen over
	// the board. The board itself draws no chip — its heading names the app.
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
	// Blocked marks a slice waiting on slices that are not Done. It is the
	// muted text the counts and hints take, because a blocked row is one there
	// is nothing to do about yet: it should read as receding rather than as
	// something gone wrong.
	Blocked lipgloss.Style
	// Review marks a slice handed back on a branch and waiting to be reviewed.
	// It takes the Success green the finished things take: the work itself is
	// done, and what is pending is a person opening the pull request from it.
	Review lipgloss.Style
	// PROpen, PRDraft, PRMerged and PRClosed are the chip the pull request
	// screen names a pull request's state with: GitHub's own four words, in the
	// roles the rest of the interface reads them in — the Success green of work
	// that stands, the quiet surface chip of one not offered for merging yet,
	// the Accent of the one ending that is the work landing, and the Danger red
	// of the ending that is not.
	PROpen   lipgloss.Style
	PRDraft  lipgloss.Style
	PRMerged lipgloss.Style
	PRClosed lipgloss.Style
	// CheckPass, CheckFail, CheckPending and CheckSkip colour a status check on
	// the pull request screen — its mark, its state word, and the rollup line
	// summarising the lot — in the same four roles the rest of the interface
	// reads outcomes in: the Success green of something that stands, the Danger
	// red of something that did not, the Warning yellow of something still
	// waiting on a machine, and the muted text of a check that never ran, which
	// is nothing gone wrong and nothing to wait for either.
	CheckPass    lipgloss.Style
	CheckFail    lipgloss.Style
	CheckPending lipgloss.Style
	CheckSkip    lipgloss.Style
	// DiffFile, DiffMeta, DiffHunk, DiffAdd and DiffDel colour the unified diff
	// of a slice's branch by the shape of the line: the "diff --git" line that
	// opens a file's section, the header lines under it, a hunk's @@ line, and
	// the added and removed lines themselves. Added and removed take the Success
	// and Danger colours the rest of the interface reads outcomes in — which is
	// what a diff's green and red are anyway. In a file the viewer found a
	// language for they are the +/- alone, the rest of the line being the
	// Syntax styles below; in one it did not they are the whole line, as they
	// always were.
	DiffFile lipgloss.Style
	DiffMeta lipgloss.Style
	DiffHunk lipgloss.Style
	DiffAdd  lipgloss.Style
	DiffDel  lipgloss.Style
	// DiffCount is the ± tally beside a path in the diff's file list, and
	// DiffRule the rule between the list and the diff beside it.
	DiffCount lipgloss.Style
	DiffRule  lipgloss.Style
	// DiffComment marks a line of the diff carrying a review comment waiting to
	// be sent. It takes the pending yellow rather than either of the diff's own
	// colours: the mark sits beside added and removed lines alike, and a green
	// or red one would read as part of the change rather than as something
	// waiting on a person. DiffCommentText is what the comment itself is drawn
	// in under the lines it was left on: the same yellow without the weight,
	// since the mark is a mark and the text is several rows of prose.
	DiffComment     lipgloss.Style
	DiffCommentText lipgloss.Style
	// The Syntax styles colour a diff line by what the file's own language makes
	// of it, where the viewer could find one: ordinary text, a comment, a
	// keyword, a string, a number, and the names a file declares — a function, a
	// type, a package. They are few on purpose. A screen for reading a change
	// wants those told apart and no more; a colour per token type would be a
	// diff that is harder to read rather than easier.
	SyntaxText    lipgloss.Style
	SyntaxComment lipgloss.Style
	SyntaxKeyword lipgloss.Style
	SyntaxString  lipgloss.Style
	SyntaxNumber  lipgloss.Style
	SyntaxName    lipgloss.Style
	// DiffAddFill and DiffDelFill are the wash a highlighted file's added and
	// removed lines are drawn on. They are what keeps such a line reading as
	// added or removed once the syntax has taken the foreground the green and
	// the red used to have — and they are colours rather than styles because a
	// row is several separately rendered runs, and a background applied to the
	// finished string would not survive the reset each of them ends with: it is
	// merged into every run's own style instead ([wash]).
	//
	// A file with no language keeps the foreground colouring and takes no wash,
	// so what falls back falls all the way back.
	DiffAddFill color.Color
	DiffDelFill color.Color

	// ActiveEdge is the border of the Active section at the top of the board,
	// ActiveTitle the heading let into that border, and ActiveName a slice's own
	// name on the first line of its entry. ActiveFill is the subtle fill the
	// selected entry is drawn on — a colour rather than a style, since an entry
	// is several separately rendered pieces and each has to keep its own state
	// colour over the fill, which is what [wash] merges it into.
	ActiveEdge  lipgloss.Style
	ActiveTitle lipgloss.Style
	ActiveName  lipgloss.Style
	ActiveFill  color.Color
	// The State styles colour an Active entry by where the slice has got to: its
	// dot, and the state word on the line under it. They are the roles the rest
	// of the board already reads these states in — the Working orange of a star
	// at work, the pending yellow of one stopped for input, the muted text of a
	// blocked row, the Success green of work waiting to be reviewed — bar the
	// slice in progress with nothing out yet, which takes AccentAlt because it
	// is ordinary work rather than anything to put right. A review that is over
	// takes that same green in bold: it is the end of the state beside it rather
	// than a state of another kind.
	StateWorking        lipgloss.Style
	StateWaiting        lipgloss.Style
	StateBlocked        lipgloss.Style
	StateReadyToPush    lipgloss.Style
	StateAwaitingReview lipgloss.Style
	StateReadyToMerge   lipgloss.Style

	// StarDim, StarMid and StarPeak are the star of a slice with an agent
	// working on it, brightening and settling as the pulse swells: all three
	// the Working orange, separated by weight the way Claude Code's own
	// spinner separates them. StarWaiting is the steady star of one that has
	// stopped for input — the pending yellow, because it is the board's colour
	// for something waiting on a person, and because it has to read apart from
	// the orange at a glance.
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
	// bar is the heading bar's fill, which every segment drawn on it inherits.
	bar := lipgloss.NewStyle().Background(t.Surface)
	return Styles{
		Frame:    lipgloss.NewStyle().Padding(0, framePadX),
		Title:    lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		Subtitle: lipgloss.NewStyle().Foreground(t.AccentAlt),
		Faint:    lipgloss.NewStyle().Foreground(t.Muted),

		Header:          bar,
		HeaderApp:       chip.Bold(true).Foreground(t.OnFill).Background(t.Accent),
		HeaderTitle:     bar.Bold(true).Foreground(t.Text),
		HeaderMeta:      bar.Foreground(t.Muted),
		HeaderMilestone: bar.Foreground(t.AccentAlt),

		Box:             lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.SurfaceHi).Padding(0, 1),
		TermBox:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.SurfaceHi),
		TermBoxFocused:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent),
		TermEdge:        lipgloss.NewStyle().Foreground(t.SurfaceHi),
		TermEdgeFocused: lipgloss.NewStyle().Foreground(t.Accent),

		Modal: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.AccentDim).Padding(0, 1),
		Scrim: lipgloss.NewStyle().Foreground(t.SurfaceHi),

		StatusKey:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		StatusDesc: lipgloss.NewStyle().Foreground(t.Text),
		StatusNote: lipgloss.NewStyle().Foreground(t.Success),

		HintKey:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		HintDesc: lipgloss.NewStyle().Foreground(t.Text),
		HintSep:  lipgloss.NewStyle().Foreground(t.Muted),

		Error: chip.Bold(true).Foreground(t.OnFill).Background(t.Danger),

		ToastSuccess: lipgloss.NewStyle().Foreground(t.Success),
		ToastWarning: lipgloss.NewStyle().Foreground(t.Warning),
		ToastError:   lipgloss.NewStyle().Foreground(t.Danger),

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
		Blocked:  lipgloss.NewStyle().Foreground(t.Muted),
		Review:   lipgloss.NewStyle().Bold(true).Foreground(t.Success),

		PROpen:   chip.Foreground(t.OnFill).Background(t.Success),
		PRDraft:  chip.Foreground(t.Text).Background(t.Surface),
		PRMerged: chip.Foreground(t.OnFill).Background(t.Accent),
		PRClosed: chip.Foreground(t.OnFill).Background(t.Danger),

		CheckPass:    lipgloss.NewStyle().Foreground(t.Success),
		CheckFail:    lipgloss.NewStyle().Bold(true).Foreground(t.Danger),
		CheckPending: lipgloss.NewStyle().Foreground(t.Warning),
		CheckSkip:    lipgloss.NewStyle().Foreground(t.Muted),

		DiffFile:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		DiffMeta:  lipgloss.NewStyle().Foreground(t.Muted),
		DiffHunk:  lipgloss.NewStyle().Foreground(t.AccentAlt),
		DiffAdd:   lipgloss.NewStyle().Foreground(t.Success),
		DiffDel:   lipgloss.NewStyle().Foreground(t.Danger),
		DiffCount: lipgloss.NewStyle().Foreground(t.Muted),
		DiffRule:  lipgloss.NewStyle().Foreground(t.SurfaceHi),

		DiffComment:     lipgloss.NewStyle().Bold(true).Foreground(t.Warning),
		DiffCommentText: lipgloss.NewStyle().Foreground(t.Warning),

		SyntaxText:    lipgloss.NewStyle().Foreground(t.Text),
		SyntaxComment: lipgloss.NewStyle().Foreground(t.Muted),
		SyntaxKeyword: lipgloss.NewStyle().Foreground(t.Accent),
		// The strings take the Warning yellow rather than the green they take in
		// most themes: the green is what an added line is, and a string drawn in
		// it inside a diff would be saying the wrong thing twice.
		SyntaxString: lipgloss.NewStyle().Foreground(t.Warning),
		SyntaxNumber: lipgloss.NewStyle().Foreground(t.Working),
		SyntaxName:   lipgloss.NewStyle().Foreground(t.AccentAlt),

		DiffAddFill: t.SuccessWash,
		DiffDelFill: t.DangerWash,

		ActiveEdge:  lipgloss.NewStyle().Foreground(t.SurfaceHi),
		ActiveTitle: lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		ActiveName:  lipgloss.NewStyle().Bold(true).Foreground(t.Text),
		ActiveFill:  t.SurfaceHi,

		StateWorking:        lipgloss.NewStyle().Foreground(t.Working),
		StateWaiting:        lipgloss.NewStyle().Foreground(t.Warning),
		StateBlocked:        lipgloss.NewStyle().Foreground(t.Muted),
		StateReadyToPush:    lipgloss.NewStyle().Foreground(t.AccentAlt),
		StateAwaitingReview: lipgloss.NewStyle().Foreground(t.Success),
		StateReadyToMerge:   lipgloss.NewStyle().Foreground(t.Success).Bold(true),

		StarDim:     lipgloss.NewStyle().Faint(true).Foreground(t.Working),
		StarMid:     lipgloss.NewStyle().Foreground(t.Working),
		StarPeak:    lipgloss.NewStyle().Bold(true).Foreground(t.Working),
		StarWaiting: lipgloss.NewStyle().Bold(true).Foreground(t.Warning),

		BarFill:     lipgloss.NewStyle().Foreground(t.Accent),
		BarFillDone: lipgloss.NewStyle().Foreground(t.AccentDim),
		BarEmpty:    lipgloss.NewStyle().Foreground(t.SurfaceHi),

		FormTheme: huh.ThemeFunc(func(bool) *huh.Styles { return formStyles(t) }),
	}
}

// toastStyle is the status band's style for a toast of the given severity.
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
