package tui

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/git"
)

// headerRows is the header row of every box in the body, in the order they are
// drawn: the rows a file is folded away by, and all a folded one leaves.
func headerRows(d *Diff) []string {
	var heads []string
	for _, row := range bodyRows(d) {
		if strings.HasPrefix(row, boxTopLeft) {
			heads = append(heads, row)
		}
	}
	return heads
}

// TestDiffViewedCollapsesTheBox covers the fold itself: the file the cursor is
// in goes to its header row alone — no diff lines and no footer row — with a
// tick where the rule would be, and the key again brings it all back.
func TestDiffViewedCollapsesTheBox(t *testing.T) {
	d := newTestDiff()
	whole := bodyRows(d)
	// The rows the first file's lines take at this width, and its footer row.
	folded := d.tops[1] - d.offsets[0]

	d.Update(keyPress("enter"))

	rows := bodyRows(d)
	if want := len(whole) - folded; len(rows) != want {
		t.Errorf("body is %d rows, want %d — the first file's lines and its footer gone",
			len(rows), want)
	}
	for _, row := range rows {
		if strings.Contains(row, "lines := b.rows") {
			t.Errorf("row %q is still drawn, want the collapsed file's lines folded away", row)
		}
	}
	head := headerRows(d)[0]
	if !strings.HasPrefix(head, boxTopLeft+viewedMark) || !strings.HasSuffix(head, boxTopRight) {
		t.Errorf("header row = %q, want it marked viewed and closed", head)
	}
	if !strings.Contains(head, "internal/tui/board.go") || !strings.Contains(head, "+3 -3") {
		t.Errorf("header row = %q, want the path and the tally still on it", head)
	}
	if width := len([]rune(head)); width != d.diffWidth() {
		t.Errorf("header row is %d columns wide, want the diff's own %d", width, d.diffWidth())
	}

	d.Update(keyPress("enter"))
	if got := bodyRows(d); len(got) != len(whole) {
		t.Errorf("body is %d rows after unfolding, want the %d it started with", len(got), len(whole))
	}
}

// TestDiffViewedFoldsTheFileTheCursorIsIn covers which file the key acts on:
// the one the cursor is in rather than the first, and the cursor lands on the
// header row it leaves — the only row that file still has.
func TestDiffViewedFoldsTheFileTheCursorIsIn(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("n"))

	d.Update(keyPress("enter"))

	if d.viewedFile(0) || !d.viewedFile(1) {
		t.Errorf("viewed = %v, want the second file alone folded", d.viewed)
	}
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: boxHeaderRow}) {
		t.Errorf("cursor at %+v, want the header row of the file it folded", got)
	}
	if d.cursor != 1 {
		t.Errorf("the list marks file %d, want the one that was folded", d.cursor)
	}

	// Unfolding puts it back on the first line of the file's diff, which is
	// where the box is read from.
	d.Update(keyPress("enter"))
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: firstShown(d.files[1])}) {
		t.Errorf("cursor at %+v, want the first line of the file it unfolded", got)
	}
}

// TestDiffCursorMovesOverACollapsedFile covers j and k against a fold: the
// header row is the one place in a collapsed file the cursor stops, and the
// steps either side of it reach the files around it.
func TestDiffCursorMovesOverACollapsedFile(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("n"))
	d.Update(keyPress("enter"))
	d.Update(keyPress("p"))

	// Down through the rest of the first file, then onto the fold and past it.
	for range len(shownLines(d.files[0])) - 1 {
		d.Update(keyPress("j"))
	}
	d.Update(keyPress("j"))
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: boxHeaderRow}) {
		t.Fatalf("cursor at %+v, want the collapsed file's header row", got)
	}
	d.Update(keyPress("j"))
	if got := d.lines[d.line]; got != (bodyLine{file: 2, line: firstShown(d.files[2])}) {
		t.Errorf("cursor at %+v, want the first line of the file after the fold", got)
	}
	d.Update(keyPress("k"))
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: boxHeaderRow}) {
		t.Errorf("cursor at %+v, want the fold on the way back up", got)
	}
}

// TestDiffJumpsLandOnACollapsedHeader covers n and p over a fold: the jumps
// move through a collapsed file like any other and land on its header, which is
// all there is of it.
func TestDiffJumpsLandOnACollapsedHeader(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("n"))
	d.Update(keyPress("enter"))
	d.Update(keyPress("p"))

	d.Update(keyPress("n"))
	if d.cursor != 1 {
		t.Fatalf("cursor = %d after n, want the collapsed file", d.cursor)
	}
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: boxHeaderRow}) {
		t.Errorf("cursor at %+v, want the collapsed file's header row", got)
	}
	// The fold leaves the body barely longer than the band, so the viewport
	// holds at its own bottom rather than scrolling the header row to the top.
	bottom := strings.Count(d.vp.GetContent(), "\n") + 1 - diffTestHeight
	if got, want := d.vp.YOffset(), min(d.tops[1], bottom); got != want {
		t.Errorf("scrolled to line %d, want the box's own row, %d", got, want)
	}
	d.Update(keyPress("n"))
	if d.cursor != 2 {
		t.Errorf("cursor = %d, want the jump to carry on past the fold", d.cursor)
	}
}

// TestDiffCommentRefusedOnACollapsedFile covers the review keys over a fold:
// there is nothing on show to say anything about, and no end of a range to mark
// either.
func TestDiffCommentRefusedOnACollapsedFile(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("enter"))
	if _, _, _, _, ok := d.Selection(); ok {
		t.Error("Selection() should find nothing on a collapsed file's header row")
	}
	if d.ToggleSelect() || d.Selecting() {
		t.Error("ToggleSelect() should mark nothing on a collapsed file's header row")
	}
}

// TestDiffViewedKeepsThePendingComments covers a comment left on a file that is
// then folded away: it is still pending, and still goes to the agent — folding
// is about what is on screen and nothing else.
func TestDiffViewedKeepsThePendingComments(t *testing.T) {
	d := newTestDiff()
	d.SetComment(firstFile, 5, 1, "this line")

	d.Update(keyPress("enter"))

	if got := d.Pending(); got != 1 {
		t.Errorf("Pending() = %d, want the comment on the folded file kept", got)
	}
	if got := d.Comments(); len(got) != 1 || got[0].Text != "this line" {
		t.Errorf("Comments() = %+v, want the comment on the folded file", got)
	}
	for _, row := range bodyRows(d) {
		if strings.Contains(row, commentMark) {
			t.Errorf("row %q marks a comment, want the folded file's gutter off screen", row)
		}
	}
}

// TestDiffViewedGoesWithAFreshRead covers what a fold is worth after the branch
// has been read again: nothing, since the file it was an opinion about may have
// changed underneath.
func TestDiffViewedGoesWithAFreshRead(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("enter"))
	if !d.viewedFile(0) {
		t.Fatal("the first file should be folded")
	}

	d.SetFiles("origin/main", git.ParseFiles(sampleDiff), nil)
	if d.viewedFile(0) {
		t.Error("a fresh read should leave nothing folded")
	}

	d.Update(keyPress("enter"))
	d.Start("slice-1", "Diff viewer", "slice/diff-viewer", "/repos/nat")
	if d.viewedFile(0) {
		t.Error("a read starting should leave nothing folded")
	}
}

// TestDiffViewedWithNothingToFold covers the key on a screen with no diff on
// it, which is a read that has not landed.
func TestDiffViewedWithNothingToFold(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	if d.ToggleViewed() || d.ToggleViewedAt(0) {
		t.Error("there is nothing to fold on a screen with no diff on it")
	}
	d.Update(keyPress("enter"))
	if d.line != 0 {
		t.Errorf("line = %d, want the cursor left alone", d.line)
	}
}

// TestDiffViewedHintSaysWhichHalf covers the hints row: the one key is both
// halves of a fold, and the row says which half the file under the cursor is
// about to get.
func TestDiffViewedHintSaysWhichHalf(t *testing.T) {
	d := newTestDiff()
	if got := d.viewedBinding().Help().Desc; got != "collapse file" {
		t.Errorf("hint = %q, want the fold named on an open file", got)
	}
	d.Update(keyPress("enter"))
	if got := d.viewedBinding().Help().Desc; got != "expand file" {
		t.Errorf("hint = %q, want the unfold named on a collapsed file", got)
	}
}

// TestDiffToggleViewedAtTakesTheBoxRows covers what a click may land on: either
// of a box's own two rows folds the file it belongs to, and a line of the diff
// itself is not a fold.
func TestDiffToggleViewedAtTakesTheBoxRows(t *testing.T) {
	d := newTestDiff()
	footer := footerRow(d, 0)
	if got := d.lines[footer]; got.line != boxFooterRow {
		t.Fatalf("body line %d is %+v, want the first box's footer row", footer, got)
	}

	if !d.ToggleViewedAt(footer) || !d.viewedFile(0) {
		t.Error("a click on the footer row should fold its file")
	}
	if !d.ToggleViewedAt(d.tops[0]) || d.viewedFile(0) {
		t.Error("a click on the header row should unfold it again")
	}
	if d.ToggleViewedAt(d.offsets[0]) || d.viewedFile(0) {
		t.Error("a click on a line of the diff should fold nothing")
	}
	if d.ToggleViewedAt(len(d.lines)) {
		t.Error("a click past the end of the body should fold nothing")
	}
}

// TestDiffLineAtFindsTheBodyLine covers the cell arithmetic a click goes
// through: the columns of the file list are not the diff, nor is a row past the
// band or past the body.
func TestDiffLineAtFindsTheBodyLine(t *testing.T) {
	d := newTestDiff()
	const inDiff = diffListWidth + diffRuleWidth
	for _, tt := range []struct {
		what     string
		col, row int
		want     int
		ok       bool
	}{
		{"the first row of the diff", inDiff, 0, 0, true},
		{"a row further down", inDiff + 3, 4, 4, true},
		{"the file list", 2, 0, 0, false},
		{"the rule between them", diffListWidth, 0, 0, false},
		{"past the right of the diff", inDiff + d.diffWidth(), 0, 0, false},
		{"below the band", inDiff, d.vp.Height(), 0, false},
		{"above the band", inDiff, -1, 0, false},
	} {
		got, ok := d.LineAt(tt.col, tt.row)
		if got != tt.want || ok != tt.ok {
			t.Errorf("LineAt on %s = %d, %v; want %d, %v", tt.what, got, ok, tt.want, tt.ok)
		}
	}

	// Scrolled, the same row of the band is a later line of the body.
	d.vp.SetYOffset(3)
	if got, ok := d.LineAt(inDiff, 1); got != 4 || !ok {
		t.Errorf("LineAt on a scrolled body = %d, %v; want line 4", got, ok)
	}
	// With no room for the list, the diff has the columns it was taking.
	d.vp.SetYOffset(0)
	d.SetSize(diffSplitMin-1, diffTestHeight)
	if got, ok := d.LineAt(0, 0); got != 0 || !ok {
		t.Errorf("LineAt on a narrow screen = %d, %v; want the first line", got, ok)
	}

	// A band taller than the body has rows on no line at all.
	short := NewDiff(DefaultStyles())
	short.SetSize(diffTestWidth, diffTestHeight)
	short.SetFiles("origin/main", manyFiles(1), nil)
	if got, ok := short.LineAt(inDiff, len(short.lines)); ok {
		t.Errorf("LineAt past the end of the body = %d, %v; want nothing", got, ok)
	}
}

// TestDiffFoldedLinesAreOffTheBody covers the one thing the body must not do:
// hold a line of a file it has folded away, since the line cursor and the
// comments are line numbers into it.
func TestDiffFoldedLinesAreOffTheBody(t *testing.T) {
	d := newTestDiff()
	for range len(d.files) {
		d.Update(keyPress("enter"))
		d.Update(keyPress("n"))
	}
	rows := bodyRows(d)
	if len(rows) != len(d.files) || len(d.lines) != len(d.files) {
		t.Fatalf("body is %d rows over %d body lines, want one of each per file (%d)",
			len(rows), len(d.lines), len(d.files))
	}
	for i, row := range rows {
		if !strings.HasPrefix(row, boxTopLeft+viewedMark) {
			t.Errorf("row %d = %q, want every file folded to its header", i, row)
		}
	}
	// The cursor still has somewhere to be, and the keys still somewhere to go.
	d.Update(keyPress("j"))
	d.Update(keyPress("k"))
	if got := xansi.Strip(d.View("")); !strings.Contains(got, "docs/shot.png") {
		t.Errorf("view = %q, want the folded files still named", got)
	}
}
