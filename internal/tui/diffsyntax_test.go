package tui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/craigmjohnston/nat/internal/git"
)

// fallbackDiff is a change to two files the viewer can find no language for: a
// suffix chroma has never heard of, and a file git described rather than
// diffed. Both are drawn exactly as the whole viewer was before there was any
// highlighting at all.
const fallbackDiff = `diff --git a/notes.xyzzy b/notes.xyzzy
index 1111111..2222222 100644
--- a/notes.xyzzy
+++ b/notes.xyzzy
@@ -1,2 +1,2 @@
 keep this line
-return old
+return new
diff --git a/docs/shot.png b/docs/shot.png
index 3333333..4444444 100644
Binary files a/docs/shot.png and b/docs/shot.png differ
`

// opener is the escape sequence a style opens with, which is what a rendered
// row is searched for: the colours themselves are the palette's and the test
// has no business repeating them.
func opener(s lipgloss.Style) string { return strings.Split(s.Render("x"), "x")[0] }

// bodyRow is one row of the rendered body as it was drawn, escapes and all.
func bodyRow(d *Diff, row int) string { return strings.Split(d.vp.GetContent(), "\n")[row] }

// TestDiffHighlightsAFileByItsLanguage covers the whole of what highlighting
// does to a line of Go: the keywords, the names, the strings and the numbers
// each take the palette's colour for what they are, the +/- keeps the line's
// own, and the wash under the row is what still says it was added.
func TestDiffHighlightsAFileByItsLanguage(t *testing.T) {
	d := newTestDiff()
	// Wide enough that the line is not wrapped: every one of its runs is on the
	// one row, which is what the assertions below read.
	d.SetSize(160, diffTestHeight)
	s := DefaultStyles()
	// +	return strings.Join(fitRow(lines), "\n")
	row := bodyRow(d, rowAt(d, 0, 7))
	for what, style := range map[string]lipgloss.Style{
		"the added line's own colour on its +": s.DiffAdd,
		"a keyword":                            s.SyntaxKeyword,
		"a name":                               s.SyntaxName,
		"a string":                             s.SyntaxString,
	} {
		if want := opener(style.Background(s.DiffAddFill)); !strings.Contains(row, want) {
			t.Errorf("the added line is drawn without %s:\n%q", what, row)
		}
	}
	// 	total := 0 — a context line, which takes no wash and so is the one place
	// a number's own colour is drawn on the terminal's background.
	if want := opener(s.SyntaxNumber); !strings.Contains(bodyRow(d, rowAt(d, 0, 10)), want) {
		t.Errorf("a context line's number is drawn without its own colour:\n%q",
			bodyRow(d, rowAt(d, 0, 10)))
	}
	// The tab expanded, which is the render's own doing and not the lexer's.
	if got := xansi.Strip(row); !strings.Contains(got, `+    return strings.Join(fitRow(lines), "\n")`) {
		t.Errorf("the line reads %q once its colours are stripped, want git's own text", got)
	}
}

// TestDiffFallsBackWithoutALanguage covers the other half: a file chroma knows
// no language for, and one git described rather than diffed, are drawn in the
// shape colouring the viewer has always used — the whole line in the diff's own
// green or red, and no wash under it, since there is no syntax to have taken
// the foreground.
func TestDiffFallsBackWithoutALanguage(t *testing.T) {
	d := NewDiff(DefaultStyles())
	d.SetSize(diffTestWidth, diffTestHeight)
	d.Start("slice-1", "Notes", "slice/notes", "/repos/nat")
	d.SetFiles("origin/main", git.ParseFiles(fallbackDiff), nil)
	golden(t, "diff-fallback", d.View(""))

	s := DefaultStyles()
	row := bodyRow(&d, rowAt(&d, 0, 7)) // +return new
	if want := opener(s.DiffAdd); !strings.Contains(row, want) {
		t.Errorf("the added line is not drawn in the diff's own green:\n%q", row)
	}
	if wash := opener(lipgloss.NewStyle().Background(s.DiffAddFill)); strings.Contains(row, wash) {
		t.Errorf("an unhighlighted line should take no wash:\n%q", row)
	}
	if want := opener(s.SyntaxKeyword); strings.Contains(row, want) {
		t.Errorf("an unhighlighted line should hold no syntax colours:\n%q", row)
	}
}

// TestLexerForFindsWhatItCan covers which files are highlighted at all: one
// whose path names a language chroma knows, one whose does not, and a file git
// described rather than diffed, which has no content to lex in the first place.
func TestLexerForFindsWhatItCan(t *testing.T) {
	for _, tt := range []struct {
		file git.File
		want bool
	}{
		{git.File{Path: "internal/tui/diff.go"}, true},
		{git.File{Path: "notes.xyzzy"}, false},
		{git.File{Path: "docs/shot.png", Binary: true}, false},
		{git.File{Path: "main.go", Binary: true}, false},
	} {
		if got := lexerFor(tt.file) != nil; got != tt.want {
			t.Errorf("lexerFor(%q, binary=%v) found a lexer = %v, want %v",
				tt.file.Path, tt.file.Binary, got, tt.want)
		}
	}
}

// TestLineRunsKeepsWhatIsNotCode covers the lines of a section that are not the
// file's own content: they go to the lexer not at all, and come back as the one
// run the shape colouring draws.
func TestLineRunsKeepsWhatIsNotCode(t *testing.T) {
	lex := lexerFor(git.File{Path: "main.go"})
	for _, line := range []string{
		"diff --git a/main.go b/main.go",
		"index 1111111..2222222 100644",
		"--- a/main.go",
		"+++ b/main.go",
		"@@ -1,2 +1,2 @@",
		"",
	} {
		runs := lineRuns(lex, line)
		if len(runs) != 1 || runs[0].text != line || runs[0].kind != kindShape {
			t.Errorf("lineRuns(%q) = %+v, want the whole line in its own shape", line, runs)
		}
	}
}

// refusingLexer is a lexer that will not lex, which is the one failure the
// highlighter can meet that a path cannot rule out beforehand.
type refusingLexer struct{ chroma.Lexer }

func (refusingLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	return nil, errors.New("no")
}

// TestLineRunsFallsBackOnALexerThatRefuses covers that failure: the line is
// drawn the way an unknown language's is rather than going missing.
func TestLineRunsFallsBackOnALexerThatRefuses(t *testing.T) {
	runs := lineRuns(refusingLexer{}, "+	return 1")
	if len(runs) != 1 || runs[0].text != "+	return 1" || runs[0].kind != kindShape {
		t.Errorf("lineRuns with a refusing lexer = %+v, want the line whole", runs)
	}
}

// TestKindOfReadsChromasTypes covers the mapping from chroma's own token types
// onto the handful of colours the diff draws, category by category.
func TestKindOfReadsChromasTypes(t *testing.T) {
	for _, tt := range []struct {
		token chroma.TokenType
		want  tokenKind
	}{
		{chroma.CommentSingle, kindComment},
		{chroma.CommentPreproc, kindComment},
		{chroma.Keyword, kindKeyword},
		{chroma.KeywordType, kindKeyword},
		{chroma.LiteralStringDouble, kindString},
		{chroma.LiteralNumberInteger, kindNumber},
		{chroma.NameFunction, kindName},
		{chroma.NameTag, kindName},
		{chroma.NameVariable, kindPlain},
		{chroma.Operator, kindPlain},
		{chroma.Punctuation, kindPlain},
		{chroma.Text, kindPlain},
	} {
		if got := kindOf(tt.token); got != tt.want {
			t.Errorf("kindOf(%s) = %d, want %d", tt.token, got, tt.want)
		}
	}
}

// TestRunStyleAnswersEveryKind covers the other end of that mapping: every kind
// draws in a style of the palette's, and a run that is not the language's takes
// the shape style it was handed.
func TestRunStyleAnswersEveryKind(t *testing.T) {
	d := NewDiff(DefaultStyles())
	shape := d.styles.DiffAdd
	for kind, want := range map[tokenKind]lipgloss.Style{
		kindShape:   shape,
		kindPlain:   d.styles.SyntaxText,
		kindComment: d.styles.SyntaxComment,
		kindKeyword: d.styles.SyntaxKeyword,
		kindString:  d.styles.SyntaxString,
		kindNumber:  d.styles.SyntaxNumber,
		kindName:    d.styles.SyntaxName,
	} {
		if got := d.runStyle(kind, shape); got.Render("x") != want.Render("x") {
			t.Errorf("runStyle(%d) draws %q, want %q", kind, got.Render("x"), want.Render("x"))
		}
	}
}

// TestWrapRunsCutsARunInTwo covers a line wrapped in the middle of one of its
// runs: both halves keep what the run was, so the tail of a string is still
// drawn as a string.
func TestWrapRunsCutsARunInTwo(t *testing.T) {
	rows := wrapRuns([]tokenRun{
		{text: "+", kind: kindShape},
		{text: `"abcdef"`, kind: kindString},
	}, 4)
	want := wrapped{
		{{text: "+", kind: kindShape}, {text: `"ab`, kind: kindString}},
		{{text: "cdef", kind: kindString}},
		{{text: `"`, kind: kindString}},
	}
	if len(rows) != len(want) {
		t.Fatalf("wrapRuns gave %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rowText(rows[i]) != rowText(want[i]) || len(rows[i]) != len(want[i]) {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
		for j := range want[i] {
			if rows[i][j] != want[i][j] {
				t.Errorf("row %d run %d = %+v, want %+v", i, j, rows[i][j], want[i][j])
			}
		}
	}
}

// TestLineFillOnlyWashesAHighlightedChange covers what takes a wash: the added
// and removed lines of a file with a language, and nothing else — not its
// context lines, and not one line of a file without one.
func TestLineFillOnlyWashesAHighlightedChange(t *testing.T) {
	d := NewDiff(DefaultStyles())
	for _, tt := range []struct {
		line  string
		lexed bool
		want  bool
	}{
		{"+	return 1", true, true},
		{"-	return 1", true, true},
		{" 	return 1", true, false},
		{"@@ -1 +1 @@", true, false},
		{"+	return 1", false, false},
		{"-	return 1", false, false},
	} {
		if got := d.lineFill(tt.line, tt.lexed) != nil; got != tt.want {
			t.Errorf("lineFill(%q, lexed=%v) washed = %v, want %v",
				tt.line, tt.lexed, got, tt.want)
		}
	}
}
