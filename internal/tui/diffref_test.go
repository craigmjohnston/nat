package tui

import (
	"strings"
	"testing"
)

// refSection is a file's section as git writes one, with two hunks so the
// numbering has to restart part way down.
var refSection = strings.Split(`diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -10,4 +10,5 @@ func main() {
 	first
-	removed
+	added
+	also added
 	last
@@ -40,2 +41,2 @@ func other() {
 	context
-	gone`, "\n")

// TestLineRef covers how a comment names the lines it was left on: the numbers
// the branch leaves them at, the base's numbers for a run that is nothing but
// deletions, and nothing at all where no hunk covers them.
func TestLineRef(t *testing.T) {
	tests := []struct {
		name        string
		start, span int
		want        string
	}{
		{"a context line", 5, 1, "line 10"},
		{"a removed line alone", 6, 1, "line 11 of the base"},
		{"an added line", 7, 1, "line 11"},
		{"a run across a removal", 6, 3, "lines 11-12"},
		{"the whole first hunk", 4, 6, "lines 10-13"},
		{"a header line", 1, 1, ""},
		{"the second hunk's context", 11, 1, "line 41"},
		{"the second hunk's removal", 12, 1, "line 41 of the base"},
		{"a run past the end", 12, 9, "line 41 of the base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineRef(refSection, tt.start, tt.span); got != tt.want {
				t.Errorf("lineRef(%d, %d) = %q, want %q", tt.start, tt.span, got, tt.want)
			}
		})
	}
}

// TestLineRefOutsideTheSection covers a run that starts before the lines there
// are, which is what a re-anchored comment would ask for if it could.
func TestLineRefOutsideTheSection(t *testing.T) {
	if got := lineRef(refSection, -2, 2); got != "" {
		t.Errorf("lineRef before the section = %q, want nothing", got)
	}
}

// TestLineNumbersSkipsWhatIsAboutTheChange covers the lines a hunk does not
// number: the headers above the first one, git's "no newline" note, and a hunk
// header this build does not recognise, which leaves the lines under it
// unnumbered rather than numbered from a guess.
func TestLineNumbersSkipsWhatIsAboutTheChange(t *testing.T) {
	lines := []string{
		"diff --git a/f b/f",
		`@@ what @@`,
		"+unnumbered",
		"@@ -1 +1 @@",
		"+first",
		`\ No newline at end of file`,
		" second",
	}
	added, removed := lineNumbers(lines)
	want := []int{0, 0, 0, 0, 1, 0, 2}
	for i, n := range want {
		if added[i] != n {
			t.Errorf("added[%d] = %d, want %d", i, added[i], n)
		}
	}
	if removed[6] != 1 {
		t.Errorf("removed[6] = %d, want the context line's own number on the base", removed[6])
	}
}

// TestLineNumbersOfAFileRemovedWhole covers the hunk header of a deletion,
// whose new side covers no lines at all: the base's numbers are the only ones
// there are, and they are what the comment names.
func TestLineNumbersOfAFileRemovedWhole(t *testing.T) {
	lines := []string{"@@ -1,2 +0,0 @@", "-gone", "-also gone"}
	if got := lineRef(lines, 1, 2); got != "lines 1-2 of the base" {
		t.Errorf("lineRef = %q, want the base's numbers", got)
	}
}

// TestHunkStartsRejectsWhatItCannotRead covers the headers that are not one:
// too few fields, and a side that does not start with the sign it should.
func TestHunkStartsRejectsWhatItCannotRead(t *testing.T) {
	for _, line := range []string{"@@ -1,2", "@@ -1,2 @@", "@@ 1,2 +1,2 @@", "@@ -1,2 1,2 @@", "@@ -x,2 +1,2 @@"} {
		if _, _, ok := hunkStarts(line); ok {
			t.Errorf("hunkStarts(%q) = ok, want it refused", line)
		}
	}
}
