package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// askedApp returns an app with a question open over the board, and reports
// whether the work behind it was dispatched.
func askedApp(t *testing.T, p prompt) (*App, *bool) {
	t.Helper()
	done := false
	p.confirm = func(*App) tea.Cmd {
		done = true
		return func() tea.Msg { return sliceSavedMsg{note: "Done."} }
	}
	a := newSizedWriteApp(&fakeNotion{})
	a.ask(p)
	return a, &done
}

func TestPromptAnswersAreYesAndNo(t *testing.T) {
	tests := []struct {
		key      string
		wantDone bool
		wantNote string
	}{
		{"y", true, "Working…"},
		{"Y", true, "Working…"},
		{"n", false, "Left alone."},
		{"N", false, "Left alone."},
		{"esc", false, "Left alone."},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			app, done := askedApp(t, prompt{question: "Delete it?", busy: "Working…", deny: "Left alone."})

			press(app, tt.key)

			if app.prompt != nil {
				t.Error("the question should be gone once it is answered")
			}
			if *done != tt.wantDone {
				t.Errorf("dispatched = %v, want %v", *done, tt.wantDone)
			}
			if app.busy != tt.wantDone {
				t.Errorf("busy = %v, want %v", app.busy, tt.wantDone)
			}
			if app.note != tt.wantNote {
				t.Errorf("note = %q, want %q", app.note, tt.wantNote)
			}
		})
	}
}

func TestPromptWithoutADenyNoteSaysItWasCancelled(t *testing.T) {
	app, _ := askedApp(t, prompt{question: "Delete it?"})

	press(app, "n")

	if app.note != "Cancelled." {
		t.Errorf("note = %q, want the cancelled note", app.note)
	}
}

func TestPromptSwallowsEveryOtherKey(t *testing.T) {
	// Each of these would act on the board the question is about: reloading the
	// plan out from under it, leaving with it still up, or opening a second one.
	for _, k := range []string{"r", "q", "?", "i", "j", "a", "d", "Q", "enter", "x"} {
		t.Run(k, func(t *testing.T) {
			app, done := askedApp(t, prompt{question: "Delete it?"})
			before := app.board.cursor

			if cmd := press(app, k); cmd != nil {
				t.Errorf("%q did something behind the question", k)
			}

			if app.prompt == nil {
				t.Errorf("%q dropped the question", k)
			}
			if *done {
				t.Errorf("%q dispatched the work", k)
			}
			if app.screen != screenBoard || app.form != nil {
				t.Errorf("%q left the board: screen = %v, form = %v", k, app.screen, app.form)
			}
			if app.board.cursor != before {
				t.Errorf("%q moved the cursor to %d", k, app.board.cursor)
			}
		})
	}
}

// ctrl+c is the one key nothing owns: a question is no reason to stay in a
// program the user is leaving.
func TestPromptDoesNotBlockForceQuit(t *testing.T) {
	app, _ := askedApp(t, prompt{question: "Delete it?"})

	if cmd := press(app, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c should still quit with a question open")
	}
}

func TestPromptTakesTheBarFromTheHintsAndTheNote(t *testing.T) {
	app, _ := askedApp(t, prompt{question: "Delete it?"})
	app.note = "Saved."

	bar := stripANSI(app.statusBar())
	if !strings.Contains(bar, "Delete it? (y/n)") {
		t.Errorf("status bar = %q, want the question on it", bar)
	}
	for _, gone := range []string{"q quit", "r refresh", "Saved."} {
		if strings.Contains(bar, gone) {
			t.Errorf("status bar = %q, want %q gone while the question is up", bar, gone)
		}
	}
}

// An error already on the bar is not lost — it is still there to read, and to
// dismiss, once the question has been answered.
func TestPromptComesBeforeAnError(t *testing.T) {
	app, _ := askedApp(t, prompt{question: "Delete it?"})
	app.err = errors.New("load milestones: boom")

	if bar := stripANSI(app.statusBar()); !strings.Contains(bar, "Delete it?") ||
		strings.Contains(bar, "boom") {
		t.Errorf("status bar = %q, want the question in front of the error", bar)
	}

	press(app, "n")

	if bar := stripANSI(app.statusBar()); !strings.Contains(bar, "boom") {
		t.Errorf("status bar = %q, want the error back once the question is answered", bar)
	}
}

func TestPromptKeepsItsKeysInANarrowWindow(t *testing.T) {
	// The question is what gets cut; the keys are what the bar cannot afford to
	// lose, so they are still there at every width that holds anything at all.
	for width := 30; width <= 80; width++ {
		a := sizedApp(width, 24)
		a.ask(prompt{question: strings.Repeat("Delete this very long thing? ", 5)})

		bar := a.statusBar()
		if got := lipgloss.Width(bar); got != width {
			t.Fatalf("at %d columns the bar is %d wide", width, got)
		}
		if !strings.Contains(stripANSI(bar), "(y/n)") {
			t.Fatalf("at %d columns the keys have gone: %q", width, stripANSI(bar))
		}
	}
}

func TestPromptOnAnUnmeasuredWindowIsDrawnWhole(t *testing.T) {
	// Before the first resize there is nothing to fit to, so nothing is cut.
	a := newWriteApp(&fakeNotion{})
	a.ask(prompt{question: "Delete it?"})

	if bar := stripANSI(a.statusBar()); !strings.Contains(bar, "Delete it? (y/n)") {
		t.Errorf("status bar = %q, want the whole question", bar)
	}
}

func TestPromptKeepsAMultiLineQuestionOnOneLine(t *testing.T) {
	const width, height = 60, 24
	a := sizedApp(width, height)
	a.ask(prompt{question: "Delete it?\nreally?"})

	view := a.View().Content
	checkFits(t, view, width, height)
	if strings.Contains(stripANSI(view), "really") {
		t.Errorf("the question's later lines should have gone:\n%s", view)
	}
}

func TestPromptGolden(t *testing.T) {
	a := sizedApp(80, 16)
	a.ask(prompt{question: `Delete "Keep the status bar and header inside the window"?`})
	golden(t, "app-prompt", a.View().Content)
}
