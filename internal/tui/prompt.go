package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// prompt is a yes/no question asked in the status bar rather than on a screen
// of its own. A single question is not worth the board leaving the screen: the
// plan stays visible behind the answer, which is what makes the question
// answerable in the first place — what a milestone or a slice is now is the
// context for moving it on.
//
// While one is open it owns the keyboard, so none of the board's keys can fire
// on the way past an unanswered question.
type prompt struct {
	// question is what the bar asks; the keys that answer it are drawn after it.
	question string
	// confirm is the work y dispatches, and busy what the bar says while that
	// work is in flight.
	confirm func(a *App) tea.Cmd
	busy    string
	// deny is what the bar says when the answer is no.
	deny string
}

// deniedNote is what the bar says for a question answered no that has nothing
// of its own to say.
const deniedNote = "Cancelled."

// ask opens an inline prompt over the board. It replaces whatever the bar was
// saying: the question is now the whole of what the app is waiting on.
func (a *App) ask(p prompt) {
	if p.deny == "" {
		p.deny = deniedNote
	}
	a.prompt, a.note = &p, ""
}

// answerPrompt handles a key press while a prompt is open: y dispatches the
// work, n and esc drop it, and every other key does nothing at all. Keys are
// swallowed rather than passed on because every one of them would act on the
// board the question is about — r would reload the plan out from under it, and
// q would leave with it still on the bar.
func (a *App) answerPrompt(msg tea.KeyPressMsg) tea.Cmd {
	p := a.prompt
	switch {
	case key.Matches(msg, a.keys.Yes):
		a.prompt = nil
		a.busy, a.note = true, p.busy
		return p.confirm(a)
	case key.Matches(msg, a.keys.No):
		// Nothing was done and nothing needs reloading: the board behind the
		// question never went anywhere.
		a.prompt = nil
		a.note = p.deny
	}
	return nil
}

// promptKeys is what the bar draws after the question. They are rendered apart
// from it so that a question too long for the bar is cut and they are not: a
// question with no keys under it is a bar that has stopped saying what to do.
func (a *App) promptKeys() string {
	return a.styles.StatusDesc.Render("(") +
		a.styles.StatusKey.Render("y/n") +
		a.styles.StatusDesc.Render(")")
}

// promptLine is the open question as the bar asks it, cut to the room it has.
func (a *App) promptLine(width int) string {
	keys := a.promptKeys()
	room := width
	if width > 0 {
		room = width - lipgloss.Width(keys) - 1
	}
	return a.styles.StatusPrompt.Render(fit(oneLine(a.prompt.question), room)) + " " + keys
}
