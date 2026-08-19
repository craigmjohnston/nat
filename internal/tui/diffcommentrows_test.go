package tui

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/git"
)

// commentRowsUnder is the rows the body draws a comment on under a file's line:
// the rows after the last the line itself is wrapped onto, up to the next row
// that is a line of the diff again.
func commentRowsUnder(t *testing.T, d *Diff, file, line int) []string {
	t.Helper()
	at := rowAt(d, file, line)
	if at < 0 {
		t.Fatalf("file %d line %d is on no row of the body", file, line)
	}
	rows := bodyRows(d)
	var out []string
	for i := d.lineEnd(at) + 1; i < len(d.lines) && d.lines[i].line == boxCommentRow; i++ {
		out = append(out, rows[i])
	}
	return out
}

// TestDiffDrawsACommentUnderItsLine covers the whole point of the rows: what
// was typed about a line is readable under it, inside the file's own box and
// carrying the same mark as the line it is about.
func TestDiffDrawsACommentUnderItsLine(t *testing.T) {
	d := commented(t, "needs a nil check")
	rows := commentRowsUnder(t, d, 0, addedLine)
	if len(rows) != 1 || !strings.Contains(rows[0], "needs a nil check") {
		t.Fatalf("the rows under the line are %q, want the comment's text", rows)
	}
	if !strings.HasPrefix(rows[0], boxSide+commentMark) || !strings.HasSuffix(rows[0], boxSide) {
		t.Errorf("comment row = %q, want the mark inside the box's own sides", rows[0])
	}
	if width := len([]rune(rows[0])); width != d.diffWidth() {
		t.Errorf("comment row is %d columns wide, want the diff's own %d", width, d.diffWidth())
	}
}

// TestDiffDrawsARangeCommentUnderItsLastLine covers a comment on a run of
// lines: it hangs off the end of the run rather than the start, the way a
// remark about a passage of code is read after it.
func TestDiffDrawsARangeCommentUnderItsLastLine(t *testing.T) {
	d := newTestDiff()
	d.SetComment(firstFile, removedLine, 2, "the replacement is the bug")
	if rows := commentRowsUnder(t, d, 0, removedLine); len(rows) != 0 {
		t.Errorf("rows under the first line of the range = %q, want none", rows)
	}
	rows := commentRowsUnder(t, d, 0, addedLine)
	if len(rows) != 1 || !strings.Contains(rows[0], "the replacement is the bug") {
		t.Errorf("rows under the last line of the range = %q, want the comment", rows)
	}
}

// TestDiffWrapsACommentToTheBox covers a comment too wide for the box and one
// typed over several lines: the first is wrapped the way a line of the diff is,
// and the second keeps the breaks the user made.
func TestDiffWrapsACommentToTheBox(t *testing.T) {
	long := strings.Repeat("wordy ", 20)
	d := commented(t, long)
	rows := commentRowsUnder(t, d, 0, addedLine)
	if len(rows) < 2 {
		t.Fatalf("a comment of %d columns took %d rows, want it wrapped",
			len(long), len(rows))
	}
	for _, row := range rows {
		if width := len([]rune(row)); width != d.diffWidth() {
			t.Errorf("comment row %q is %d columns wide, want the diff's own %d",
				row, width, d.diffWidth())
		}
	}

	d = commented(t, "first thought\n\nsecond thought")
	rows = commentRowsUnder(t, d, 0, addedLine)
	if len(rows) != 3 || !strings.Contains(rows[0], "first thought") ||
		!strings.Contains(rows[2], "second thought") {
		t.Errorf("rows under the line = %q, want the paragraphs as they were typed", rows)
	}
}

// TestDiffDrawsTwoCommentsEndingOnOneLine covers the order they are drawn in
// when a range and a line end together: by where each starts, so the rows read
// down the file the way the prompt does.
func TestDiffDrawsTwoCommentsEndingOnOneLine(t *testing.T) {
	d := newTestDiff()
	d.SetComment(firstFile, addedLine, 1, "about the line")
	d.SetComment(firstFile, removedLine, 2, "about the pair")
	rows := commentRowsUnder(t, d, 0, addedLine)
	if len(rows) != 2 || !strings.Contains(rows[0], "about the pair") ||
		!strings.Contains(rows[1], "about the line") {
		t.Errorf("the rows under the line are %q, want the wider comment first", rows)
	}
}

// TestDiffCursorStepsOverCommentRows covers what the rows are not: a place the
// cursor rests, a line to comment on, or an end of a range.
func TestDiffCursorStepsOverCommentRows(t *testing.T) {
	d := commented(t, "needs a nil check")
	at := rowAt(d, 0, addedLine)
	d.setLine(at)
	d.moveCursor(1)
	if want := rowAt(d, 0, addedLine+1); d.line != want {
		t.Errorf("j from the commented line landed on row %d, want the next line at %d", d.line, want)
	}
	d.moveCursor(-1)
	if d.line != at {
		t.Errorf("k back landed on row %d, want the commented line at %d", d.line, at)
	}
	for i, l := range d.lines {
		if l.line == boxCommentRow && d.stop(i) {
			t.Errorf("the cursor stops on comment row %d, want it stepped over", i)
		}
	}
}

// TestDiffCommentRowsAreNoFold covers the click that lands on one: the rows are
// inside the box rather than part of it, so nothing folds.
func TestDiffCommentRowsAreNoFold(t *testing.T) {
	d := commented(t, "needs a nil check")
	row := d.lineEnd(rowAt(d, 0, addedLine)) + 1
	if d.lines[row].line != boxCommentRow {
		t.Fatalf("row %d is %+v, want a comment row", row, d.lines[row])
	}
	if d.ToggleViewedAt(row) || d.viewedFile(0) {
		t.Error("a click on a comment row should fold nothing")
	}
}

// TestDiffFoldHidesTheCommentRows covers the collapse: a file put away takes
// the comments left on it off the screen with the rest of its body, and gives
// them back when it is opened again.
func TestDiffFoldHidesTheCommentRows(t *testing.T) {
	d := commented(t, "needs a nil check")
	d.cursor = 0
	if !d.ToggleViewed() {
		t.Fatal("enter should fold the file")
	}
	if strings.Contains(d.vp.GetContent(), "needs a nil check") {
		t.Error("a collapsed file should draw none of its comment rows")
	}
	if d.Pending() != 1 {
		t.Errorf("Pending() = %d, want the comment kept behind the fold", d.Pending())
	}
	d.ToggleViewed()
	if rows := commentRowsUnder(t, d, 0, addedLine); len(rows) != 1 {
		t.Errorf("rows under the line once opened again = %q, want the comment back", rows)
	}
}

// TestDiffCommentRowsFollowAReadOfTheBranch covers what a re-read does to them:
// a comment carried onto the lines it was left on is drawn under wherever they
// have got to, and one the read dropped goes with them.
func TestDiffCommentRowsFollowAReadOfTheBranch(t *testing.T) {
	d := commented(t, "needs a nil check")
	// The same branch read again with a line added above the comment's own: the
	// rows move down with it.
	moved := strings.Replace(sampleDiff, "@@ -12,7 +12,7 @@ func (b Board) View() string {\n",
		"@@ -12,7 +12,8 @@ func (b Board) View() string {\n+\t// a note\n", 1)
	d.SetFiles("origin/main", git.ParseFiles(moved))
	if d.Pending() != 1 {
		t.Fatalf("Pending() = %d after a re-read that moved the line, want it carried over", d.Pending())
	}
	if rows := commentRowsUnder(t, d, 0, addedLine+1); len(rows) != 1 {
		t.Errorf("rows under the moved line = %q, want the comment drawn there", rows)
	}

	// A read the comment's lines are not in at all drops it, and the rows go.
	d.SetFiles("origin/main", git.ParseFiles(strings.Replace(sampleDiff,
		`+	return strings.Join(fitRow(lines), "\n")`, "+\tpanic(1)", 1)))
	if d.Pending() != 0 {
		t.Fatalf("Pending() = %d after a read that changed the line, want the comment dropped", d.Pending())
	}
	if strings.Contains(d.vp.GetContent(), "needs a nil check") {
		t.Error("a dropped comment should take its rows with it")
	}
}

// TestDiffCommentRowsRebuiltOnResize covers the re-render every width change
// is: the rows are wrapped again to the box they are drawn in, and stay under
// the line they belong to wherever the wrapping has moved it.
func TestDiffCommentRowsRebuiltOnResize(t *testing.T) {
	d := commented(t, strings.Repeat("wordy ", 20))
	wide := len(commentRowsUnder(t, d, 0, addedLine))
	d.SetSize(diffTestWidth/2, diffTestHeight)
	narrow := len(commentRowsUnder(t, d, 0, addedLine))
	if narrow <= wide {
		t.Errorf("the comment took %d rows narrow and %d wide, want more of them narrow",
			narrow, wide)
	}
}
