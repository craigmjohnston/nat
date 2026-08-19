package tui

import (
	"errors"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/git"
)

// diffWidth and diffHeight are the size the diff tests render at: wide enough
// for the file list beside the diff, short enough that the sample overflows and
// has to be scrolled and jumped through.
const (
	diffTestWidth  = 72
	diffTestHeight = 10
)

// sampleDiff is a diff with one of every line shape the screen colours: a file
// header, a hunk, context, additions, removals, a created file and a binary one
// — a file of two hunks, so the break between them has somewhere to be drawn,
// and enough files that the list scrolls and the jumps have somewhere to go.
const sampleDiff = `diff --git a/internal/tui/board.go b/internal/tui/board.go
index 1111111..2222222 100644
--- a/internal/tui/board.go
+++ b/internal/tui/board.go
@@ -12,7 +12,7 @@ func (b Board) View() string {
 	lines := b.rows
-	return strings.Join(lines, "\n")
+	return strings.Join(fitRow(lines), "\n")
 }
@@ -30,5 +30,5 @@ func (b Board) rowLines() int {
 	total := 0
 	for _, r := range b.rows {
 		total += len(r.lines)
-	}
-	return total
+	}
+	return total + 1
diff --git a/internal/tui/diff.go b/internal/tui/diff.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/internal/tui/diff.go
@@ -0,0 +1,3 @@
+package tui
+
+// Diff is the review screen.
diff --git a/a/very/deeply/nested/directory/somewhere/settings.go b/a/very/deeply/nested/directory/somewhere/settings.go
index 4444444..5555555 100644
--- a/a/very/deeply/nested/directory/somewhere/settings.go
+++ b/a/very/deeply/nested/directory/somewhere/settings.go
@@ -1 +1 @@
-package old
+package new
diff --git a/docs/shot.png b/docs/shot.png
index 6666666..7777777 100644
Binary files a/docs/shot.png and b/docs/shot.png differ
`

// newTestDiff returns a diff screen showing the sample at a fixed size, as the
// review of a branch handed back on.
func newTestDiff() *Diff {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.Start("slice-1", "Diff viewer", "slice/diff-viewer", "/repos/nat")
	d.SetFiles("origin/main", git.ParseFiles(sampleDiff))
	return &d
}

// rowAt is the body row a file's line starts on. The cursor, the range mark and
// every click are numbers into the body, and a line too wide for the box takes
// more than one row of it, so a test cannot count them off an offset.
func rowAt(d *Diff, file, line int) int {
	return d.rowOf(bodyLine{file: file, line: line}, -1)
}

// shownLines is the lines of a file's section the body draws, by their index in
// it: the render strips git's own headers and its hunk markers, so the first
// line on screen is no longer the section's line 0.
func shownLines(f git.File) []int {
	var out []int
	for i, role := range lineRoles(f.Lines) {
		if role == roleDraw {
			out = append(out, i)
		}
	}
	return out
}

// firstShown is the first line of a file's section the body draws, which is
// where a jump into that file leaves the line cursor.
func firstShown(f git.File) int { return shownLines(f)[0] }

// footerRow is the row that closes a file's box, which is the row before the
// next box opens — or the last row of the body, for the last file.
func footerRow(d *Diff, file int) int {
	if file+1 < len(d.tops) {
		return d.tops[file+1] - 1
	}
	return len(d.lines) - 1
}

// TestDiffRendersTheBranch pins the screen at a width the sample's longest
// lines do not fit: they are wrapped onto the rows below them, in the colour of
// the line they continue and with the numbers only on the row it starts on.
func TestDiffRendersTheBranch(t *testing.T) {
	golden(t, "diff-files", newTestDiff().View(""))
}

// TestDiffRendersABranchThatFits pins the same diff on a window wide enough to
// hold every one of its lines outright, where nothing is wrapped at all.
func TestDiffRendersABranchThatFits(t *testing.T) {
	d := newTestDiff()
	d.SetSize(160, diffTestHeight)
	for _, f := range d.files {
		for _, line := range f.Lines {
			if rows := wrapLine(line, d.textWidth(numberWidth(d.lineNumbers()))); len(rows) != 1 {
				t.Fatalf("%q takes %d rows at this width, want the whole diff to fit", line, len(rows))
			}
		}
	}
	golden(t, "diff-wide", d.View(""))
}

// TestDiffWithoutRoomForTheFileList covers a narrow window: the columns go to
// the diff, which is where the content is.
func TestDiffWithoutRoomForTheFileList(t *testing.T) {
	d := newTestDiff()
	d.SetSize(diffSplitMin-1, diffTestHeight)
	golden(t, "diff-narrow", d.View(""))
	if strings.Contains(xansi.Strip(d.View("")), d.listHeading()) {
		t.Error("a narrow window should draw no file list")
	}
}

// TestDiffStatesEachSayWhatIsGoingOn covers the four things the screen can be
// showing besides a diff.
func TestDiffStatesEachSayWhatIsGoingOn(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	if got := xansi.Strip(d.View("")); !strings.Contains(got, "press v") {
		t.Errorf("idle view = %q, want it to say where a diff comes from", got)
	}

	d.Start("slice-1", "Diff viewer", "slice/diff-viewer", "/repos/nat")
	if !d.Busy() {
		t.Error("Busy() = false while a read is in flight")
	}
	if got := xansi.Strip(d.View("~")); !strings.Contains(got, "~ Reading the diff of slice/diff-viewer") {
		t.Errorf("loading view = %q, want the spinner and the branch", got)
	}

	d.SetFiles("origin/main", nil)
	if got := xansi.Strip(d.View("")); !strings.Contains(got, "slice/diff-viewer has no changes against origin/main") {
		t.Errorf("empty view = %q, want it to name the branch and the base", got)
	}

	d.Fail(errors.New("fatal: bad revision\nand a second line"))
	if got := xansi.Strip(d.View("")); !strings.Contains(got, "fatal: bad revision") ||
		strings.Contains(got, "second line") {
		t.Errorf("failed view = %q, want git's first line alone", got)
	}
}

// TestDiffFailureDropsWhatWasOnScreen covers a reread that failed over a diff
// already up: a diff is of one branch at one moment, and the old one is not it.
func TestDiffFailureDropsWhatWasOnScreen(t *testing.T) {
	d := newTestDiff()
	d.Fail(errors.New("boom"))
	if len(d.files) != 0 || d.vp.GetContent() != "" {
		t.Error("a failed read should take the diff it replaced with it")
	}
}

// TestDiffJumpsBetweenFiles covers the per-file navigation: each jump scrolls
// the diff to the top of the section the list's cursor lands on, and the ends
// hold rather than wrap.
func TestDiffJumpsBetweenFiles(t *testing.T) {
	d := newTestDiff()
	if d.cursor != 0 || d.vp.YOffset() != 0 {
		t.Fatalf("a fresh diff starts at file %d, offset %d, want the top of the first",
			d.cursor, d.vp.YOffset())
	}
	// The last file's section starts nearer the end than a screenful, so the
	// viewport holds at the bottom rather than scrolling past it.
	bottom := strings.Count(d.vp.GetContent(), "\n") + 1 - diffTestHeight
	for want := 1; want < len(d.files); want++ {
		d.Update(keyPress("n"))
		if d.cursor != want {
			t.Fatalf("after %d jumps the cursor is on file %d, want %d", want, d.cursor, want)
		}
		// The box's header row rather than its first diff line: a jump lands on
		// the row that names the file.
		wantLine := min(d.offsets[want]-1, bottom)
		if got := d.vp.YOffset(); got != wantLine {
			t.Errorf("file %d is scrolled to line %d, want %d", want, got, wantLine)
		}
	}
	last := d.cursor
	d.Update(keyPress("n"))
	if d.cursor != last {
		t.Errorf("a jump past the end moved to %d, want it to hold at %d", d.cursor, last)
	}
	for range len(d.files) + 2 {
		d.Update(keyPress("p"))
	}
	if d.cursor != 0 || d.vp.YOffset() != 0 {
		t.Errorf("jumping back reached file %d at line %d, want the first file at the top",
			d.cursor, d.vp.YOffset())
	}
}

// TestDiffJumpsWithNothingToJumpThrough covers the keys on an empty diff, which
// have nowhere to go and must not run off the end of the list.
func TestDiffJumpsWithNothingToJumpThrough(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.SetFiles("origin/main", nil)
	d.Update(keyPress("n"))
	d.Update(keyPress("p"))
	if d.cursor != 0 {
		t.Errorf("cursor = %d, want it left alone", d.cursor)
	}
}

// TestDiffScrollsTheFileListWithTheCursor covers a change of more files than
// the list has room for: the cursor stays on the list rather than running off
// the bottom of it.
func TestDiffScrollsTheFileListWithTheCursor(t *testing.T) {
	d := NewDiff(DefaultStyles())
	// Three rows of list, and five files to move through them.
	d.SetSize(diffTestWidth, 4)
	d.SetFiles("origin/main", manyFiles(5))
	for range 4 {
		d.Update(keyPress("n"))
	}
	if d.cursor != 4 {
		t.Fatalf("cursor = %d, want the last file", d.cursor)
	}
	if d.listTop != 2 {
		t.Errorf("listTop = %d, want the list scrolled to keep the cursor on it", d.listTop)
	}
	if got := xansi.Strip(d.listView()); strings.Contains(got, "file0.go") {
		t.Errorf("list = %q, want the files scrolled past to be off it", got)
	}
	for range 4 {
		d.Update(keyPress("p"))
	}
	if d.listTop != 0 {
		t.Errorf("listTop = %d, want the list back at the top with the cursor", d.listTop)
	}
}

// TestDiffListWithNoRoomAtAll covers a band with no lines to draw rows on,
// which is the state a window shorter than its own frame leaves.
func TestDiffListWithNoRoomAtAll(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, 0)
	d.SetFiles("origin/main", manyFiles(3))
	d.Update(keyPress("n"))
	if d.listTop != 0 {
		t.Errorf("listTop = %d, want it left at the top when there are no rows", d.listTop)
	}
}

// manyFiles is n one-line changes, for the tests about the list rather than
// about the diff.
func manyFiles(n int) []git.File {
	var diff strings.Builder
	for i := range n {
		diff.WriteString("diff --git a/file" + string(rune('0'+i)) + ".go b/file" + string(rune('0'+i)) + ".go\n")
		diff.WriteString("@@ -1 +1 @@\n-old\n+new\n")
	}
	return git.ParseFiles(diff.String())
}

// TestDiffScrollsWithTheViewportKeys covers the keys the screen does not claim
// for itself, which belong to the viewport under it — and the cursor being
// brought back onto the body after one of them has scrolled out from under it.
func TestDiffScrollsWithTheViewportKeys(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("f"))
	if d.vp.YOffset() == 0 {
		t.Error("a page key should scroll the diff")
	}
	if d.line < d.vp.YOffset() {
		t.Errorf("cursor left at line %d above the view at %d", d.line, d.vp.YOffset())
	}
}

// TestDiffRerendersOnResize covers the body being cut to the width it is drawn
// at: a resize renders again rather than leaving lines cut to the old one.
func TestDiffRerendersOnResize(t *testing.T) {
	d := newTestDiff()
	wide := d.vp.GetContent()
	d.SetSize(40, diffTestHeight)
	if d.vp.GetContent() == wide {
		t.Error("a resize should render the diff again at the new width")
	}
}

// TestDiffBodyIsTheWrappedLines covers what the file jumps and the line cursor
// are numbers into: a body row per row a line is wrapped onto, plus the two
// rows of every file's box.
func TestDiffBodyIsTheWrappedLines(t *testing.T) {
	d := newTestDiff()
	want, wrapped := 2*len(d.files), 0
	for _, f := range d.files {
		for j, role := range lineRoles(f.Lines) {
			if role != roleDraw {
				// A dropped line takes no row at all, and a hunk break takes the
				// one row the header it stands in for would have.
				if role == roleBreak {
					want++
				}
				continue
			}
			rows := len(wrapLine(f.Lines[j], d.textWidth(numberWidth(d.lineNumbers()))))
			want += rows
			if rows > 1 {
				wrapped++
			}
		}
	}
	if wrapped == 0 {
		t.Fatal("the sample should have lines too wide for the box at the test width")
	}
	if got := strings.Count(d.vp.GetContent(), "\n") + 1; got != want {
		t.Errorf("body is %d rows, want %d — one per wrapped row of the diff", got, want)
	}
	if got := len(d.lines); got != want {
		t.Errorf("%d rows are accounted for, want %d — one per row of the body", got, want)
	}
}

// TestDiffShowsTheTailOfALongLine covers the whole point of the wrap: the end
// of a line too wide for the box is on the screen rather than cut off it.
func TestDiffShowsTheTailOfALongLine(t *testing.T) {
	d := newTestDiff()
	body := strings.Join(bodyRows(d), "")
	width := d.textWidth(numberWidth(d.lineNumbers()))
	long := 0
	for _, f := range d.files {
		for _, j := range shownLines(f) {
			line := f.Lines[j]
			rows := wrapLine(line, width)
			if len(rows) == 1 {
				continue
			}
			long++
			if tail := rows[len(rows)-1]; !strings.Contains(body, tail) {
				t.Errorf("the body does not hold %q, the end of %q", tail, line)
			}
		}
	}
	if long == 0 {
		t.Fatal("the sample should have lines too wide for the box at the test width")
	}
}

// TestDiffWrapKeepsTheLineWhole covers what a continuation row is drawn as: the
// line's own colour carries onto it, so a wrapped removal does not read as an
// added or removed line of its own, and only the row the line starts on carries
// its numbers.
func TestDiffWrapKeepsTheLineWhole(t *testing.T) {
	d := newTestDiff()
	head := rowAt(d, 0, 6) // -	return strings.Join(lines, "\n")
	if d.lines[head+1] != (bodyLine{file: 0, line: 6, seg: 1}) {
		t.Fatalf("row %d is %+v, want the second row of the wrapped removal", head+1, d.lines[head+1])
	}
	cont := strings.Split(d.vp.GetContent(), "\n")[head+1]
	if strings.Contains(xansi.Strip(cont), "13") {
		t.Errorf("continuation row = %q, want the numbers only on the row the line starts on",
			xansi.Strip(cont))
	}
	// The escape the removal's own style opens with, which is what says the
	// continuation is still part of that line rather than a plain one.
	red := strings.Split(DefaultStyles().DiffDel.Render("x"), "x")[0]
	if !strings.Contains(cont, red) {
		t.Errorf("continuation row = %q, want the removed line's own colour carried onto it", cont)
	}
}

// TestWrapLineBreaksOnTheColumn covers the wrap itself: a line that fits comes
// back whole, a wider one is cut on the column, a tab is expanded to what the
// renderer draws it as, and a rune too wide for the width still gets a row.
func TestWrapLineBreaksOnTheColumn(t *testing.T) {
	for _, tt := range []struct {
		line  string
		width int
		want  []string
	}{
		{"short", 10, []string{"short"}},
		{"abcdef", 3, []string{"abc", "def"}},
		{"abcdefg", 3, []string{"abc", "def", "g"}},
		{"", 3, []string{""}},
		{"\tx", 4, []string{"    ", "x"}},
		{"日本", 1, []string{"日", "本"}},
		{"anything at all", 0, []string{"anything at all"}},
	} {
		got := wrapLine(tt.line, tt.width)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("wrapLine(%q, %d) = %q, want %q", tt.line, tt.width, got, tt.want)
		}
	}
}

// TestDiffJumpsSurviveAResize covers the numbers a jump is made of being
// rebuilt with the body: a narrower window wraps more lines, so every row moves,
// and n still lands on the row that names the file it moved to.
func TestDiffJumpsSurviveAResize(t *testing.T) {
	d := newTestDiff()
	for _, width := range []int{160, diffTestWidth, 44, 160} {
		d.SetSize(width, diffTestHeight)
		d.jump(-d.cursor) // back to the first file
		for want := 1; want < len(d.files); want++ {
			d.Update(keyPress("n"))
			if d.cursor != want {
				t.Fatalf("at %d columns, %d jumps reach file %d, want %d", width, want, d.cursor, want)
			}
			if got := d.lines[d.line]; got != (bodyLine{file: want, line: firstShown(d.files[want])}) {
				t.Errorf("at %d columns, the cursor is at %+v, want the first line of file %d",
					width, got, want)
			}
			if got := d.lines[d.tops[want]]; got.line != boxHeaderRow {
				t.Errorf("at %d columns, file %d opens at row %+v, want its header row",
					width, want, got)
			}
		}
	}
}

// TestDiffResizeKeepsTheCursorOnItsLine covers where the cursor is put back
// after a re-render: on the line it was on rather than the row that line used
// to be at, since a resize wraps differently and moves every row under it.
func TestDiffResizeKeepsTheCursorOnItsLine(t *testing.T) {
	d := newTestDiff()
	for range 7 {
		d.Update(keyPress("j"))
	}
	was := d.lines[d.line]
	if was.line == 0 {
		t.Fatalf("cursor at %+v, want it well into the first file", was)
	}
	d.SetSize(160, diffTestHeight)
	if got := d.lines[d.line]; got != was {
		t.Errorf("cursor at %+v after a resize, want the line it was on, %+v", got, was)
	}
	d.SetSize(44, diffTestHeight)
	if got := d.lines[d.line]; got != was {
		t.Errorf("cursor at %+v after a second resize, want %+v", got, was)
	}
}

// TestDiffCursorOnALineTallerThanTheBand covers a line wrapped onto more rows
// than the band has: there is no offset that holds all of it, so the body is
// scrolled to where the line starts rather than to where it ends.
func TestDiffCursorOnALineTallerThanTheBand(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(40, 3)
	d.SetFiles("origin/main", git.ParseFiles("diff --git a/x.go b/x.go\n@@ -1 +1 @@\n-old\n+"+
		strings.Repeat("wide ", 40)+"\n"))
	long := len(d.files[0].Lines) - 1
	for range long {
		d.Update(keyPress("j"))
	}
	start := rowAt(&d, 0, long)
	if d.line != start {
		t.Fatalf("cursor at row %d, want the long line's own row %d", d.line, start)
	}
	if d.lineEnd(start) < start+d.vp.Height() {
		t.Fatalf("the long line takes rows %d..%d, want more than the band's %d",
			start, d.lineEnd(start), d.vp.Height())
	}
	if got := d.vp.YOffset(); got != start {
		t.Errorf("body scrolled to %d, want the row the long line starts on, %d", got, start)
	}
}

// TestDiffTargetAndReset cover what the refresh key reads and what a project
// switch clears.
func TestDiffTargetAndReset(t *testing.T) {
	d := newTestDiff()
	if !d.Loadable() {
		t.Error("Loadable() = false on a screen pointed at a branch")
	}
	slice, branch, dir := d.Target()
	if slice != "Diff viewer" || branch != "slice/diff-viewer" || dir != "/repos/nat" {
		t.Errorf("Target() = %q/%q/%q, want what the screen was opened on", slice, branch, dir)
	}

	d.Reset()
	if d.Loadable() || len(d.files) != 0 || d.state != diffIdle {
		t.Error("Reset() should leave nothing of the branch it was showing")
	}
	if d.width != diffTestWidth || d.height != diffTestHeight {
		t.Error("Reset() should keep the room the window gave the screen")
	}
}

// TestDiffLineStyles pins which style each shape of line is drawn in, since the
// colours are the whole of how a unified diff is read.
func TestDiffLineStyles(t *testing.T) {
	s := DefaultStyles()
	d := NewDiff(s)
	for _, tt := range []struct {
		line string
		want string
	}{
		{"diff --git a/x b/x", "DiffFile"},
		{"--- a/x", "DiffMeta"},
		{"+++ b/x", "DiffMeta"},
		{"index 111..222 100644", "DiffMeta"},
		{"Binary files a/x and b/x differ", "DiffMeta"},
		{"@@ -1 +1 @@", "DiffHunk"},
		{"+added", "DiffAdd"},
		{"-removed", "DiffDel"},
		{" context", "plain"},
		{"", "plain"},
	} {
		styles := map[string]string{
			"DiffFile": s.DiffFile.Render("x"),
			"DiffMeta": s.DiffMeta.Render("x"),
			"DiffHunk": s.DiffHunk.Render("x"),
			"DiffAdd":  s.DiffAdd.Render("x"),
			"DiffDel":  s.DiffDel.Render("x"),
			"plain":    "x",
		}
		if got := d.lineStyle(tt.line).Render("x"); got != styles[tt.want] {
			t.Errorf("%q is drawn as %q, want %s", tt.line, got, tt.want)
		}
	}
}

// TestElideLeftKeepsTheTail covers the file list's paths: what names a file is
// the end of its path, so that is what survives a narrow column.
func TestElideLeftKeepsTheTail(t *testing.T) {
	for _, tt := range []struct {
		path  string
		width int
		want  string
	}{
		{"internal/tui/diff.go", 30, "internal/tui/diff.go"},
		{"internal/tui/diff.go", 20, "internal/tui/diff.go"},
		{"internal/tui/diff.go", 12, "…tui/diff.go"},
		{"internal/tui/diff.go", 1, "…"},
		{"internal/tui/diff.go", 0, ""},
		{"日本語", 2, "…"},
	} {
		got := elideLeft(tt.path, tt.width)
		if got != tt.want {
			t.Errorf("elideLeft(%q, %d) = %q, want %q", tt.path, tt.width, got, tt.want)
		}
	}
}

// TestDiffPluralNamesOneFile covers the file list's heading on a change of a
// single file.
func TestDiffPluralNamesOneFile(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.SetFiles("origin/main", manyFiles(1))
	if want := "1 file vs origin/main"; d.listHeading() != want {
		t.Errorf("heading = %q, want %q", d.listHeading(), want)
	}
	d.SetFiles("origin/main", manyFiles(2))
	if want := "2 files vs origin/main"; d.listHeading() != want {
		t.Errorf("heading = %q, want %q", d.listHeading(), want)
	}
}

// TestDiffListMarksABinaryFile covers the tally of a file git described rather
// than diffed, which has no ± to show — both where the row is drawn plain and
// where the cursor fills it and its colours come off.
func TestDiffListMarksABinaryFile(t *testing.T) {
	d := newTestDiff()
	if got := xansi.Strip(d.listView()); !strings.Contains(got, "bin") {
		t.Errorf("list = %q, want the binary file marked", got)
	}
	d.cursor = len(d.files) - 1
	row := xansi.Strip(d.fileRow(d.cursor))
	if !strings.Contains(row, "shot.png") || !strings.Contains(row, "bin") {
		t.Errorf("selected row = %q, want the binary file marked under the cursor", row)
	}
}

// TestDiffCursorMovesByLine covers the line cursor: j and k step it, the ends
// hold it, and the blank line between two sections is stepped over — there is
// nothing there to comment on.
func TestDiffCursorMovesByLine(t *testing.T) {
	d := newTestDiff()
	shown := shownLines(d.files[0])
	if got := d.lines[d.line]; got != (bodyLine{file: 0, line: shown[0]}) {
		t.Fatalf("cursor at %+v, want the first line of the first file", got)
	}
	first := d.line
	d.Update(keyPress("j"))
	if want := rowAt(d, 0, shown[1]); d.line != want {
		t.Errorf("line = %d after j, want the row the second line starts on, %d", d.line, want)
	}
	d.Update(keyPress("k"))
	d.Update(keyPress("k"))
	if d.line != first {
		t.Errorf("line = %d at the top, want the cursor to hold at %d", d.line, first)
	}

	// The last line of the first file, then one more: the border rows between
	// the two boxes are stepped over onto the first line of the next.
	last := shown[len(shown)-1]
	for range len(shown) - 1 {
		d.Update(keyPress("j"))
	}
	d.Update(keyPress("j"))
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: firstShown(d.files[1])}) {
		t.Errorf("cursor at %+v, want the first line of the second file", got)
	}
	d.Update(keyPress("k"))
	if got := d.lines[d.line]; got != (bodyLine{file: 0, line: last}) {
		t.Errorf("cursor at %+v, want the last line of the first file", got)
	}
}

// TestDiffCursorHoldsAtTheEnd covers the bottom of the body, where a step down
// has nowhere to go: the last box's footer row is not a line to stop on, so the
// cursor holds on the last line of the diff itself.
func TestDiffCursorHoldsAtTheEnd(t *testing.T) {
	d := newTestDiff()
	for range len(d.lines) + 2 {
		d.Update(keyPress("j"))
	}
	last := len(d.files) - 1
	if got := d.lines[d.line]; got != (bodyLine{file: last, line: len(d.files[last].Lines) - 1}) {
		t.Errorf("cursor at %+v, want the last line of the last file", got)
	}
}

// TestDiffCursorMovesOnAnEmptyDiff covers the keys on a screen with nothing on
// it, which is a read that has not landed.
func TestDiffCursorMovesOnAnEmptyDiff(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.Update(keyPress("j"))
	d.Update(keyPress("f"))
	if d.line != 0 {
		t.Errorf("line = %d on an empty screen, want 0", d.line)
	}
}

// TestDiffCursorScrolls covers the body following the cursor down and back up:
// the line a comment goes on is always one the user can see.
func TestDiffCursorScrolls(t *testing.T) {
	d := newTestDiff()
	for range d.vp.Height() + 2 {
		d.Update(keyPress("j"))
	}
	if d.vp.YOffset() == 0 {
		t.Error("the body should scroll to keep the cursor on screen")
	}
	if d.line < d.vp.YOffset() || d.line >= d.vp.YOffset()+d.vp.Height() {
		t.Errorf("cursor at %d is outside the view at %d", d.line, d.vp.YOffset())
	}
	for range len(d.lines) {
		d.Update(keyPress("k"))
	}
	if d.vp.YOffset() != 0 {
		t.Errorf("YOffset = %d back at the top, want 0", d.vp.YOffset())
	}
}

// TestDiffJumpTakesTheCursorWithIt covers n and p moving the line cursor as
// well as the view: it is what a comment is left on, and leaving it in the file
// the jump was away from would comment on a section that is no longer up.
func TestDiffJumpTakesTheCursorWithIt(t *testing.T) {
	d := newTestDiff()
	d.Update(keyPress("n"))
	if got := d.lines[d.line]; got != (bodyLine{file: 1, line: firstShown(d.files[1])}) {
		t.Errorf("cursor at %+v after n, want the top of the second file", got)
	}
	d.Update(keyPress("p"))
	if got := d.lines[d.line]; got != (bodyLine{file: 0, line: firstShown(d.files[0])}) {
		t.Errorf("cursor at %+v after p, want the top of the first file", got)
	}
}
