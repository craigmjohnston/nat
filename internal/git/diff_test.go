package git

import (
	"reflect"
	"strings"
	"testing"
)

// sampleDiff is a unified diff with one of every shape the parser has to tell
// apart: an edited file, a created one, a deleted one, a rename, and a binary
// file git described rather than diffed.
const sampleDiff = `diff --git a/internal/tui/board.go b/internal/tui/board.go
index 1111111..2222222 100644
--- a/internal/tui/board.go
+++ b/internal/tui/board.go
@@ -1,4 +1,4 @@
 package tui
-old line
+new line
+another new line
 trailing context
diff --git a/internal/tui/diff.go b/internal/tui/diff.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/internal/tui/diff.go
@@ -0,0 +1,2 @@
+package tui
+
diff --git a/internal/tui/gone.go b/internal/tui/gone.go
deleted file mode 100644
index 4444444..0000000
--- a/internal/tui/gone.go
+++ /dev/null
@@ -1,1 +0,0 @@
-package tui
diff --git a/old/name.go b/new/name.go
similarity index 98%
rename from old/name.go
rename to new/name.go
--- a/old/name.go
+++ b/new/name.go
@@ -1,1 +1,1 @@
-package old
+package new
diff --git a/docs/shot.png b/docs/shot.png
index 5555555..6666666 100644
Binary files a/docs/shot.png and b/docs/shot.png differ
`

// TestParseFilesSplitsTheDiff covers the whole sample at once: every file, in
// the order git wrote them, with the paths and tallies the file list draws
// from.
func TestParseFilesSplitsTheDiff(t *testing.T) {
	files := ParseFiles(sampleDiff)
	type want struct {
		path, old       string
		added, removed  int
		binary, renamed bool
	}
	wants := []want{
		{path: "internal/tui/board.go", old: "internal/tui/board.go", added: 2, removed: 1},
		// A created file is named on both sides of its own header, so it has an
		// old path although one side of the diff is /dev/null — and is not a
		// rename, because the two are the same.
		{path: "internal/tui/diff.go", old: "internal/tui/diff.go", added: 2},
		{path: "internal/tui/gone.go", old: "internal/tui/gone.go", removed: 1},
		{path: "new/name.go", old: "old/name.go", added: 1, removed: 1, renamed: true},
		{path: "docs/shot.png", old: "docs/shot.png", binary: true},
	}
	if len(files) != len(wants) {
		t.Fatalf("ParseFiles() found %d files, want %d", len(files), len(wants))
	}
	for i, w := range wants {
		f := files[i]
		got := want{f.Path, f.OldPath, f.Added, f.Removed, f.Binary, f.Renamed()}
		if got != w {
			t.Errorf("file %d = %+v, want %+v", i, got, w)
		}
	}
}

// TestParseFilesKeepsTheLinesVerbatim covers what the viewer actually draws:
// each file's section exactly as git wrote it, from its own header to the line
// before the next file's.
func TestParseFilesKeepsTheLinesVerbatim(t *testing.T) {
	files := ParseFiles(sampleDiff)
	want := []string{
		"diff --git a/internal/tui/gone.go b/internal/tui/gone.go",
		"deleted file mode 100644",
		"index 4444444..0000000",
		"--- a/internal/tui/gone.go",
		"+++ /dev/null",
		"@@ -1,1 +0,0 @@",
		"-package tui",
	}
	if !reflect.DeepEqual(files[2].Lines, want) {
		t.Errorf("Lines = %q, want %q", files[2].Lines, want)
	}
	// The whole diff is accounted for: every line of it belongs to some file.
	var n int
	for _, f := range files {
		n += len(f.Lines)
	}
	if lines := strings.Count(strings.TrimSuffix(sampleDiff, "\n"), "\n") + 1; n != lines {
		t.Errorf("files hold %d lines, want all %d of the diff", n, lines)
	}
}

// TestParseFilesReadsABinaryPatch covers the block a repository configured to
// write binary patches produces, which has no "Binary files ... differ" line to
// recognise it by.
func TestParseFilesReadsABinaryPatch(t *testing.T) {
	files := ParseFiles("diff --git a/x.png b/x.png\nGIT binary patch\ndelta 12\n")
	if len(files) != 1 || !files[0].Binary {
		t.Errorf("ParseFiles() = %+v, want one binary file", files)
	}
}

// TestParseFilesDropsAPreamble covers a diff that arrived with something in
// front of the first file, which belongs to no file and is not drawn.
func TestParseFilesDropsAPreamble(t *testing.T) {
	files := ParseFiles("commit deadbeef\nAuthor: someone\n\ndiff --git a/x b/x\n+one\n")
	if len(files) != 1 {
		t.Fatalf("ParseFiles() found %d files, want 1", len(files))
	}
	if got := files[0].Lines[0]; got != "diff --git a/x b/x" {
		t.Errorf("first line = %q, want the file's own header", got)
	}
}

// TestParseFilesOfNoChanges covers a branch whose work is already in the base:
// no files, and nothing to say about them.
func TestParseFilesOfNoChanges(t *testing.T) {
	for _, diff := range []string{"", "\n"} {
		if files := ParseFiles(diff); len(files) != 0 {
			t.Errorf("ParseFiles(%q) = %+v, want no files", diff, files)
		}
	}
}

// TestParseFilesReadsAPathWithSpaces covers the one place the "diff --git"
// header is ambiguous: the +++/--- lines under it each carry exactly one path,
// and they are what settles it.
func TestParseFilesReadsAPathWithSpaces(t *testing.T) {
	files := ParseFiles("diff --git a/a b c.txt b/a b c.txt\n" +
		"--- a/a b c.txt\n+++ b/a b c.txt\n@@ -1 +1 @@\n-x\n+y\n")
	if len(files) != 1 {
		t.Fatalf("ParseFiles() found %d files, want 1", len(files))
	}
	if files[0].Path != "a b c.txt" || files[0].OldPath != "a b c.txt" {
		t.Errorf("paths = %q/%q, want the file with spaces in its name",
			files[0].OldPath, files[0].Path)
	}
	if files[0].Renamed() {
		t.Error("Renamed() = true, want a file that only looked renamed")
	}
}

// TestHeaderPathsWithoutADestination covers a header the split cannot make
// sense of, which is not something git writes but is what an unprefixed diff
// from elsewhere would look like: the whole rest is taken as the path rather
// than the file being dropped.
func TestHeaderPathsWithoutADestination(t *testing.T) {
	old, path := headerPaths("diff --git a/only")
	if old != "" || path != "only" {
		t.Errorf("headerPaths() = %q/%q, want the one path it could find", old, path)
	}
}
