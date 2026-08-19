package tui

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/git"
)

// TestLineRolesStripTheGitNoise pins what the render drops: git's own header
// for a file, the blob hashes and the two paths under it, and the hunk headers
// — the first of them outright and every later one as the break that says lines
// were skipped. Everything else is a line of the change and is drawn.
func TestLineRolesStripTheGitNoise(t *testing.T) {
	lines := []string{
		"diff --git a/x.go b/x.go",
		"index 1111111..2222222 100644",
		"--- a/x.go",
		"+++ b/x.go",
		"@@ -1,2 +1,2 @@ func x() {",
		" context",
		"-was",
		"+++ now",
		"@@ -9,2 +9,2 @@",
		"@@ this is no header git wrote",
		" more",
	}
	want := []lineRole{
		roleDrop, roleDrop, roleDrop, roleDrop, roleDrop,
		roleDraw, roleDraw, roleDraw,
		roleBreak,
		roleDraw, roleDraw,
	}
	got := lineRoles(lines)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q is %d, want %d", lines[i], got[i], want[i])
		}
	}
}

// TestLineRolesKeepADescribedFile covers a file git described rather than
// diffed: it has no hunk at all, and the one line git wrote for it is said
// nowhere else on the screen, so it stays.
func TestLineRolesKeepADescribedFile(t *testing.T) {
	lines := git.ParseFiles("diff --git a/p.png b/p.png\nindex 111..222 100644\n" +
		"Binary files a/p.png and b/p.png differ\n")[0].Lines
	if got := lineRoles(lines); got[2] != roleDraw {
		t.Errorf("the binary line is %d, want it drawn", got[2])
	}
}

// TestDiffBodyHoldsNoGitHeaders covers the acceptance the strip is for: no raw
// git header line is anywhere in a box, since the box's own header row names
// the path and its tally and the gutter carries the hunks' numbers.
func TestDiffBodyHoldsNoGitHeaders(t *testing.T) {
	d := newTestDiff()
	for _, row := range bodyRows(d) {
		for _, noise := range []string{"diff --git", "index 1111111", "--- a/", "+++ b/", "@@ -"} {
			if strings.Contains(row, noise) {
				t.Errorf("row %q still holds %q", row, noise)
			}
		}
	}
}

// TestDiffDrawsABreakBetweenHunks covers what stands where a hunk header was:
// a row of the box's own width between the two hunks of the sample's first
// file, and none above its first hunk, where nothing was skipped.
func TestDiffDrawsABreakBetweenHunks(t *testing.T) {
	d := newTestDiff()
	rows := bodyRows(d)
	breaks := 0
	for i, row := range rows {
		if !strings.Contains(row, boxBreakRule) {
			continue
		}
		breaks++
		if width := len([]rune(row)); width != d.diffWidth() {
			t.Errorf("the break is %d columns wide, want the diff's own %d", width, d.diffWidth())
		}
		if !strings.Contains(rows[i-1], "}") || !strings.Contains(rows[i+1], "total := 0") {
			t.Errorf("the break at row %d sits between %q and %q, want it between the two hunks",
				i, rows[i-1], rows[i+1])
		}
	}
	if breaks != 1 {
		t.Errorf("%d breaks in the body, want the one between the sample's two hunks", breaks)
	}
}

// TestDiffBreakIsNoLineToActOn covers the row itself: it stands for the lines
// that were skipped rather than being one of them, so the cursor steps over it,
// a click on it folds nothing, and it belongs to no file's section.
func TestDiffBreakIsNoLineToActOn(t *testing.T) {
	d := newTestDiff()
	at := -1
	for i, row := range d.lines {
		if row.line == boxBreakRow {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("the sample's two hunks should leave a break row in the body")
	}
	if isBoxRow(boxBreakRow) {
		t.Error("a break is inside the box rather than one of its own rows")
	}
	if d.stop(at) {
		t.Error("the cursor should step over the break between two hunks")
	}
	if d.ToggleViewedAt(at) || d.viewedFile(0) {
		t.Error("a click on the break should fold nothing")
	}

	// Stepping down from the line above it lands on the line below.
	d.setLine(at - 1)
	d.Update(keyPress("j"))
	if d.line != at+1 {
		t.Errorf("j from the row above the break reached %d, want %d", d.line, at+1)
	}
}

// TestDiffSectionWithNothingToDraw covers a file whose whole section is git's
// own headers: there is no line for the cursor to rest on, so the row that
// names the file is where a jump into it leaves the cursor.
func TestDiffSectionWithNothingToDraw(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.SetFiles("origin/main", git.ParseFiles(sampleDiff+
		"diff --git a/empty.go b/empty.go\nindex 8888888..9999999 100644\n"))
	last := len(d.files) - 1
	if got := d.offsets[last]; got != d.tops[last] {
		t.Errorf("the section opens the cursor at row %d, want its header row %d", got, d.tops[last])
	}
	d.jump(last)
	if got := d.lines[d.line]; got.file != last {
		t.Errorf("a jump into it left the cursor at %+v, want it inside the file", got)
	}
	if _, _, _, _, ok := d.Selection(); ok {
		t.Error("Selection() should find nothing in a section with no lines drawn")
	}
}
