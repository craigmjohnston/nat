package tui

import (
	"strings"
	"testing"
)

func TestModalFloatsOverTheDimmedBoard(t *testing.T) {
	a := sizedFormApp(t, 100, 24)

	view := stripANSI(a.modalView())
	// The board is still on show behind the form's box.
	if !strings.Contains(view, "▾ Board") {
		t.Errorf("the board should show behind the modal:\n%s", view)
	}
	if !strings.Contains(view, "Title") {
		t.Errorf("the form should be on the modal:\n%s", view)
	}
	// The box floats: its top border starts past the margin rather than at the
	// body's left edge. It is the rightmost box on the backdrop — the Active
	// section behind it sits at the board's own left edge — so the corner
	// furthest in is the modal's.
	top := -1
	for line := range strings.SplitSeq(view, "\n") {
		if i := strings.IndexRune(line, '╭'); i > top {
			top = i
		}
	}
	if top < modalMarginX {
		t.Errorf("modal border starts at column %d, want it floated past the margin of %d:\n%s",
			top, modalMarginX, view)
	}
}

func TestScrimStripsTheBoardsOwnColours(t *testing.T) {
	a := sizedFormApp(t, 100, 24)

	scrim := a.scrimView()
	// The board draws background fills — the selected row, the progress bar —
	// none of which belong on the scrim: behind a modal everything recedes to
	// the one quiet colour.
	if strings.Contains(scrim, "[48;") {
		t.Errorf("the scrim carries a background fill from the board:\n%q", scrim)
	}
	// Rendering the scrim pads its lines out to the widest, so the texts are
	// compared with their trailing fill trimmed.
	if got, want := trimTrailing(stripANSI(scrim)), trimTrailing(stripANSI(a.boardView())); got != want {
		t.Errorf("the scrim should redraw the board's own text:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// trimTrailing trims the trailing spaces from every line of s.
func trimTrailing(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

func TestModalKeepsKeysFromTheBoard(t *testing.T) {
	a := sizedFormApp(t, 100, 24)
	before := a.board.Cursor()

	press(a, "j")

	if got := a.board.Cursor(); got != before {
		t.Errorf("board cursor moved from %d to %d while a modal was open", before, got)
	}
	if a.form == nil {
		t.Error("the form was dismissed by a key meant for it")
	}
}

func TestModalDrawsAloneBeforeTheFirstResize(t *testing.T) {
	// An unsized window has no band to centre on or board to fade, so the box
	// is simply drawn.
	a := newWriteApp(&fakeNotion{})
	a.board.cursor = rowActiveMilestone
	feed(t, a, a.addSlice())

	view := stripANSI(a.modalView())
	if !strings.Contains(view, "Title") || !strings.Contains(view, "╭") {
		t.Errorf("the box should draw unsized:\n%s", view)
	}
	if strings.Contains(view, "▾ Board") {
		t.Errorf("no board should be behind an unsized modal:\n%s", view)
	}
}
