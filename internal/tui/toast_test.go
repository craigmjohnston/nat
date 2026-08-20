package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/config"
)

// realDismissAfter is the production dismissal timer, put aside by TestMain
// before the suite swaps in the inert one.
var realDismissAfter func(tea.Msg) tea.Cmd

// The real timer delivers its message once the wait is up — run against a
// wait short enough to sit out.
func TestDismissAfterDeliversItsMessage(t *testing.T) {
	restore := dismissDuration
	dismissDuration = time.Millisecond
	t.Cleanup(func() { dismissDuration = restore })

	msg := runMsg(t, realDismissAfter(toastGoneMsg{id: 7}))
	if got, ok := msg.(toastGoneMsg); !ok || got.id != 7 {
		t.Errorf("msg = %#v, want the toastGoneMsg it was given", msg)
	}
}

// firingDismiss makes the dismissal timers fire as soon as their command is
// run, in place of the four-second tick, restoring the suite's inert version
// after.
func firingDismiss(t *testing.T) {
	t.Helper()
	restore := dismissAfter
	dismissAfter = func(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
	t.Cleanup(func() { dismissAfter = restore })
}

func TestToastAutoDismisses(t *testing.T) {
	firingDismiss(t)
	app := NewApp(config.Config{}, nil)

	cmd := app.showToast("Switched.", sevSuccess)
	if app.toast != "Switched." || app.toastSev != sevSuccess {
		t.Fatalf("toast = %q/%v, want it up before the timer", app.toast, app.toastSev)
	}
	app.Update(runMsg(t, cmd))
	if app.toast != "" {
		t.Errorf("toast = %q, want it dismissed by its timer", app.toast)
	}
}

func TestAStaleTimerLeavesANewerToastAlone(t *testing.T) {
	firingDismiss(t)
	app := NewApp(config.Config{}, nil)

	first := app.showToast("First.", sevSuccess)
	app.showToast("Second.", sevWarning)
	app.Update(runMsg(t, first))
	if app.toast != "Second." {
		t.Errorf("toast = %q, want the newer toast to outlive the older timer", app.toast)
	}
}

func TestConfirmAutoDismisses(t *testing.T) {
	firingDismiss(t)
	app := NewApp(config.Config{}, nil)

	cmd := app.showConfirm("Saved.", sevSuccess)
	if app.board.confirmText != "Saved." {
		t.Fatalf("confirm = %q, want it up before the timer", app.board.confirmText)
	}
	app.Update(runMsg(t, cmd))
	if app.board.confirmText != "" {
		t.Errorf("confirm = %q, want it dismissed by its timer", app.board.confirmText)
	}
}

func TestAStaleTimerLeavesANewerConfirmAlone(t *testing.T) {
	firingDismiss(t)
	app := NewApp(config.Config{}, nil)

	first := app.showConfirm("First.", sevSuccess)
	app.showConfirm("Second.", sevWarning)
	app.Update(runMsg(t, first))
	if app.board.confirmText != "Second." {
		t.Errorf("confirm = %q, want the newer confirmation to outlive the older timer", app.board.confirmText)
	}
}

func TestBoardNavigationDismissesTheConfirm(t *testing.T) {
	b := newTestBoard()
	b.SetConfirm("Saved.", sevSuccess)
	b.move(1)
	if b.confirmText != "" {
		t.Errorf("confirm = %q, want it dismissed when the cursor leaves the row", b.confirmText)
	}

	b.SetConfirm("Saved.", sevSuccess)
	b.toggle()
	if b.confirmText != "" {
		t.Errorf("confirm = %q, want it dismissed when a fold moves the cursor", b.confirmText)
	}

	// A move that goes nowhere leaves the cursor — and the confirmation — alone.
	b.cursor = 0
	b.SetConfirm("Saved.", sevSuccess)
	b.move(-1)
	if b.confirmText != "Saved." {
		t.Errorf("confirm = %q, want a blocked move to keep it", b.confirmText)
	}
}

// The confirmation overlaps the selected row's content at a narrow width, with
// the dithered fade on its left edge; at a comfortable width it sits on the
// empty fill with no fade at all.
func TestBoardConfirmOverlapGolden(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(40)
	b.cursor = 4 // Info view
	b.SetConfirm(`Launched nat-5 for "Info view".`, sevSuccess)
	golden(t, "board-confirm-overlap", b.View())
}

func TestBoardConfirmFitsGolden(t *testing.T) {
	b := newTestBoard()
	b.cursor = 4
	b.SetConfirm("Saved.", sevSuccess)
	golden(t, "board-confirm-fits", b.View())
}

// The prompt is drawn in the confirmation's shape and place: over the selected
// row from its right edge, with the dithered fade where it overlaps the row's
// content and the focused choice filled with the accent.
func TestBoardPromptOverlapGolden(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(40)
	b.cursor = 4 // Info view
	b.SetPrompt(launchChoices)
	golden(t, "board-prompt-overlap", b.View())
}

func TestBoardPromptFocusGolden(t *testing.T) {
	b := newTestBoard()
	b.cursor = 4
	b.SetPrompt(launchChoices)
	b.MovePrompt(1)
	golden(t, "board-prompt-focused", b.View())
}

// One question at a time on a row: opening a prompt takes down whatever the
// last action left there to read.
func TestBoardPromptReplacesTheConfirm(t *testing.T) {
	b := newTestBoard()
	b.cursor = 4
	b.SetConfirm("Saved.", sevSuccess)
	b.SetPrompt(launchChoices)

	if b.confirmText != "" {
		t.Errorf("confirm = %q, want it taken down by the prompt", b.confirmText)
	}
	if line := stripANSI(selectedLine(b)); strings.Contains(line, "Saved.") {
		t.Errorf("row = %q, want the prompt drawn in the confirmation's place", line)
	}
}

// With no prompt up there is nothing to focus and nothing to step.
func TestBoardPromptWithNothingToAnswer(t *testing.T) {
	b := newTestBoard()
	b.MovePrompt(1)
	if b.Prompting() {
		t.Error("stepping a prompt that is not there should not open one")
	}
	if got := b.PromptChoice(); got != 0 {
		t.Errorf("choice = %d, want the first", got)
	}
}

// selectedLine is the last line of the board's render that the cursor's row is
// drawn on, which is the one a confirmation is anchored to. It is read off the
// cursor's span rather than off the cursor itself: a row is not a line — the
// Active section takes several above the plan, and a wrapped row takes more
// than one of its own.
func selectedLine(b *Board) string {
	top, height := b.CursorSpan()
	return strings.Split(b.View(), "\n")[top+height-1]
}

func TestBoardConfirmOnAnUnmeasuredBoardFollowsTheRow(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(0)
	b.cursor = 4
	b.SetConfirm("Saved.", sevSuccess)
	if line := stripANSI(selectedLine(b)); !strings.HasSuffix(line, " Saved. ") {
		t.Errorf("line = %q, want the chip simply appended", line)
	}
}

func TestBoardConfirmWiderThanTheBoardIsCut(t *testing.T) {
	b := newTestBoard()
	b.SetWidth(6)
	b.cursor = 4
	b.SetConfirm("A confirmation much wider than the board.", sevError)
	line := selectedLine(b)
	if got := lipgloss.Width(line); got != 6 {
		t.Errorf("width = %d, want the chip cut to the board", got)
	}
	if text := stripANSI(line); !strings.HasPrefix(text, " A con") {
		t.Errorf("line = %q, want the chip's leading text", text)
	}
}

func TestBoardConfirmFadeShrinksWhenTheRoomForItDoes(t *testing.T) {
	b := newTestBoard()
	// " Hi " is 4 cells, so at width 5 the chip leaves one column: half the
	// fade, not none of it and not a line pushed over width.
	b.SetWidth(5)
	b.cursor = 4
	b.SetConfirm("Hi", sevWarning)
	line := selectedLine(b)
	if got := lipgloss.Width(line); got != 5 {
		t.Errorf("width = %d, want the row held to the board", got)
	}
	if text := stripANSI(line); text != "▒ Hi " {
		t.Errorf("line = %q, want the fade cut to the one spare column", text)
	}
}

func TestSeverityStyleMapping(t *testing.T) {
	s := DefaultStyles()
	toasts := map[severity]lipgloss.Style{
		sevSuccess: s.ToastSuccess, sevWarning: s.ToastWarning, sevError: s.ToastError,
	}
	chips := map[severity]lipgloss.Style{
		sevSuccess: s.ConfirmSuccess, sevWarning: s.ConfirmWarning, sevError: s.ConfirmError,
	}
	fades := map[severity]lipgloss.Style{
		sevSuccess: s.ConfirmFadeSuccess, sevWarning: s.ConfirmFadeWarning, sevError: s.ConfirmFadeError,
	}
	for sev, want := range toasts {
		if got := s.toastStyle(sev); got.Render("x") != want.Render("x") {
			t.Errorf("toastStyle(%v) renders %q, want %q", sev, got.Render("x"), want.Render("x"))
		}
	}
	for sev := range chips {
		chip, fade := s.confirmStyles(sev)
		if chip.Render("x") != chips[sev].Render("x") {
			t.Errorf("confirmStyles(%v) chip renders %q, want %q", sev, chip.Render("x"), chips[sev].Render("x"))
		}
		if fade.Render("x") != fades[sev].Render("x") {
			t.Errorf("confirmStyles(%v) fade renders %q, want %q", sev, fade.Render("x"), fades[sev].Render("x"))
		}
	}
}

func TestAppPutsAToastOnTheStatusLine(t *testing.T) {
	app := NewApp(config.Config{}, nil)
	app.toast, app.toastSev = "Switched to \"other\".", sevSuccess

	if got := app.View().WindowTitle; !strings.Contains(got, "Switched to \"other\".") {
		t.Errorf("window title = %q, want the toast", got)
	}
}

// The status line speaks in one voice at a time: an error beats the toast, and
// so does progress in flight.
func TestStatusLinePrefersTheErrorAndTheNoteToTheToast(t *testing.T) {
	app := sizedApp(80, 24)
	app.toast, app.toastSev = "Switched.", sevSuccess

	app.note = "Saving…"
	if bar := app.windowTitle(); strings.Contains(bar, "Switched.") || !strings.Contains(bar, "Saving…") {
		t.Errorf("bar = %q, want the note spoken over the toast", bar)
	}

	app.err = errors.New("load milestones: boom")
	if bar := app.windowTitle(); strings.Contains(bar, "Switched.") || !strings.Contains(bar, "boom") {
		t.Errorf("bar = %q, want the error spoken over everything", bar)
	}
}

// An open form owns the status line's message slot: its esc hint matters more
// than a toast left over from before it opened.
func TestStatusLinePrefersTheFormHintToTheToast(t *testing.T) {
	app := sizedApp(80, 24)
	app.toast, app.toastSev = "Switched.", sevSuccess
	app.openForm(newNewProjectForm(app.styles.FormTheme))

	if bar := app.windowTitle(); strings.Contains(bar, "Switched.") || !strings.Contains(bar, "esc cancel") {
		t.Errorf("bar = %q, want the form's esc hint over the toast", bar)
	}
}
