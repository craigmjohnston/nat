package tui

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/git"
)

// bodyRows is the rendered body of the diff, line by line and stripped of its
// colours: the boxes as they are drawn, whether or not the viewport is scrolled
// to them.
func bodyRows(d *Diff) []string {
	return strings.Split(xansi.Strip(d.vp.GetContent()), "\n")
}

// TestDiffBoxesEachFileSeparately covers the shape of the body: a header row
// naming the path with its tally, the file's diff between the box's sides, and a
// footer row closing it — one box per file, and every row of every box the same
// width.
func TestDiffBoxesEachFileSeparately(t *testing.T) {
	d := newTestDiff()
	rows := bodyRows(d)
	tops, bottoms := 0, 0
	for i, row := range rows {
		if width := len([]rune(row)); width != d.diffWidth() {
			t.Fatalf("row %d is %d columns wide, want the diff's own %d: %q",
				i, width, d.diffWidth(), row)
		}
		switch {
		case strings.HasPrefix(row, boxTopLeft):
			tops++
			if !strings.HasSuffix(row, boxTopRight) {
				t.Errorf("row %d opens a box but does not close it: %q", i, row)
			}
		case strings.HasPrefix(row, boxBottomLeft):
			bottoms++
		case !strings.HasPrefix(row, boxSide) || !strings.HasSuffix(row, boxSide):
			t.Errorf("row %d is inside no box: %q", i, row)
		}
	}
	if tops != len(d.files) || bottoms != len(d.files) {
		t.Errorf("%d header and %d footer rows, want one of each per file (%d)",
			tops, bottoms, len(d.files))
	}
	if want := "internal/tui/board.go ─"; !strings.Contains(rows[0], want) ||
		!strings.Contains(rows[0], "+3 -3") {
		t.Errorf("first header row = %q, want the path and its tally", rows[0])
	}
}

// rowHolding is the one row of the body carrying text, which is how a test
// names the line of the diff it is about without counting rows.
func rowHolding(t *testing.T, rows []string, text string) string {
	t.Helper()
	for _, row := range rows {
		if strings.Contains(row, text) {
			return row
		}
	}
	t.Fatalf("no row of the body holds %q", text)
	return ""
}

// TestDiffBoxNumbersTheLines covers the gutter down the left of a box: the old
// and new numbers of every line the hunk headers cover, one side alone for a
// line only one side has, and neither for a line no hunk covers.
func TestDiffBoxNumbersTheLines(t *testing.T) {
	d := newTestDiff()
	rows := bodyRows(d)
	numWidth := numberWidth(d.lineNumbers())
	for _, tt := range []struct {
		holds string
		want  string
	}{
		{"new file mode", ""},                // a line above the first hunk is at no number at all
		{"lines := b.rows", "12 12"},         // context: the same line on both sides
		{"return strings.Join(lines", "13"},  // removed: the number it had in the base
		{"return strings.Join(fitRow", "13"}, // added: the one it has on the branch
		{"}", "14 14"},
	} {
		row := rowHolding(t, rows, tt.holds)
		// The two number columns and the space between them, after the box's own
		// side and the column a comment is marked in.
		from := len(boxSide) + diffGutterWidth
		gutter := strings.TrimSpace(row[from : from+2*numWidth+1])
		if gutter != tt.want {
			t.Errorf("the line holding %q is numbered %q, want %q", tt.holds, gutter, tt.want)
		}
	}
}

// TestDiffBoxNumbersLineUpAcrossFiles covers the width of the number columns:
// they are measured across the whole diff, so a file's code starts at the same
// column in every box rather than shifting from one to the next.
func TestDiffBoxNumbersLineUpAcrossFiles(t *testing.T) {
	// A change whose second file runs past line 999, so the columns are wider
	// than the first file alone would make them.
	deep := sampleDiff + `diff --git a/deep.go b/deep.go
index 8888888..9999999 100644
--- a/deep.go
+++ b/deep.go
@@ -1000,2 +1000,2 @@
-was
+now
`
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.SetFiles("origin/main", git.ParseFiles(deep), nil)
	if got := numberWidth(d.lineNumbers()); got != 4 {
		t.Fatalf("numberWidth = %d, want the four digits of the widest number", got)
	}
	rows := bodyRows(&d)
	// Where a diff line's own +/-/space starts, given the gutter those four
	// digits make: the same column in the first file's box as in the last's.
	start := len(boxSide) + diffGutterWidth + 2*4 + 2
	for _, tt := range []struct{ holds, want string }{
		{"lines := b.rows", " "}, // a context line of the first file
		{"was", "-"},             // a removed line of the last, numbered past 999
	} {
		row := rowHolding(t, rows, tt.holds)
		if got := row[start : start+1]; got != tt.want {
			t.Errorf("the line holding %q starts at column %d with %q, want %q",
				tt.holds, start, got, tt.want)
		}
	}
}

// TestDiffBoxKeepsADescribedFileInside covers a binary file, which git described
// rather than diffed: the one line it wrote goes in the box like any other, with
// no numbers beside it, and the header row says "bin" where a tally would be.
func TestDiffBoxKeepsADescribedFileInside(t *testing.T) {
	d := newTestDiff()
	rows := bodyRows(d)
	head, body := "", ""
	for i, row := range rows {
		if strings.Contains(row, "Binary files") {
			body = row
			// Its box's header row is the last one above it that opens a box:
			// the lines between the two are as many rows as they wrapped onto.
			for j := i - 1; j >= 0; j-- {
				if strings.HasPrefix(rows[j], boxTopLeft) {
					head = rows[j]
					break
				}
			}
			break
		}
	}
	if body == "" {
		t.Fatal("the binary file's own line is nowhere in the body")
	}
	if !strings.HasPrefix(body, boxSide) || !strings.HasSuffix(body, boxSide) {
		t.Errorf("the binary file's line = %q, want it inside its box", body)
	}
	if !strings.Contains(head, "docs/shot.png") || !strings.Contains(head, "bin") {
		t.Errorf("its header row = %q, want the path and bin", head)
	}
}

// TestDiffBoxWithoutRoomForTheTally covers a box too narrow for both: the path
// keeps the header row, since the list beside the diff carries the tally too.
func TestDiffBoxWithoutRoomForTheTally(t *testing.T) {
	d := newTestDiff()
	d.SetSize(14, diffTestHeight)
	head := bodyRows(d)[0]
	if strings.Contains(head, "+1 -1") {
		t.Errorf("header row = %q, want the tally dropped for want of room", head)
	}
	if !strings.Contains(head, "rd.go") {
		t.Errorf("header row = %q, want the tail of the path kept", head)
	}
	if width := len([]rune(head)); width != 14 {
		t.Errorf("header row is %d columns wide, want 14", width)
	}
}

// TestDiffBoxMarksACommentedLine covers the gutter a comment is marked in,
// which sits inside the box beside the numbers.
func TestDiffBoxMarksACommentedLine(t *testing.T) {
	d := newTestDiff()
	d.SetComment(firstFile, 5, 1, "this line")
	// The first marked row is the line itself: the comment's own rows are drawn
	// under it and carry the mark too.
	var marked string
	for _, row := range bodyRows(d) {
		if strings.Contains(row, commentMark) {
			marked = row
			break
		}
	}
	if !strings.Contains(marked, "lines := b.rows") {
		t.Errorf("the marked row is %q, want the line the comment was left on", marked)
	}
	if !strings.HasPrefix(marked, boxSide+commentMark) {
		t.Errorf("the marked row is %q, want the mark inside the box's own side", marked)
	}
}

// TestDiffContentLineFindsTheNearestLineOfTheDiff covers the snap onto a line
// there is something to comment on: forward off a box's header row, back off the
// footer row that ends the body, and 0 on a body with nothing in it at all.
func TestDiffContentLineFindsTheNearestLineOfTheDiff(t *testing.T) {
	d := newTestDiff()
	if got, want := d.contentLine(0), d.offsets[0]; got != want {
		t.Errorf("contentLine(0) = %d, want the first line of the first box, %d", got, want)
	}
	last := len(d.lines) - 1
	if got := d.lines[last]; got.line != boxFooterRow {
		t.Fatalf("body line %d is %+v, want the last box's footer row", last, got)
	}
	if got, want := d.contentLine(last), rowAt(d, len(d.files)-1, len(d.files[len(d.files)-1].Lines)-1); got != want {
		t.Errorf("contentLine(%d) = %d, want the line above the footer row, %d", last, got, want)
	}

	empty := NewDiff(DefaultStyles())
	if got := empty.contentLine(3); got != 0 {
		t.Errorf("contentLine on an empty body = %d, want 0", got)
	}
}

// TestDiffCellCutsAndPads covers the row-width helper every box row goes
// through: a short interior is padded out so the right border lands in the same
// column, and a long one is cut rather than wrapped onto a second line.
func TestDiffCellCutsAndPads(t *testing.T) {
	if got := cell("ab", 5); got != "ab   " {
		t.Errorf("cell(\"ab\", 5) = %q, want it padded to five columns", got)
	}
	if got := cell("abcdefg", 3); got != "abc" {
		t.Errorf("cell(\"abcdefg\", 3) = %q, want it cut to three columns", got)
	}
}
