package cli

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/alecthomas/chroma/v2"

	"github.com/craigmjohnston/nat/internal/git"
)

// TestLexerForFindsWhatItCan mirrors internal/tui's own TestLexerForFindsWhatItCan:
// a path chroma knows a language for, one it does not, and a file git described
// rather than diffed, which has no content to lex at all whatever its path.
func TestLexerForFindsWhatItCan(t *testing.T) {
	for _, tt := range []struct {
		file git.File
		want bool
	}{
		{git.File{Path: "main.go"}, true},
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

// TestLanguageOf covers the name a matched file reports on the wire, and the
// empty string a file with none does.
func TestLanguageOf(t *testing.T) {
	if got := languageOf(lexerFor(git.File{Path: "main.go"})); got != "Go" {
		t.Errorf("languageOf(main.go) = %q, want Go", got)
	}
	if got := languageOf(lexerFor(git.File{Path: "notes.xyzzy"})); got != "" {
		t.Errorf("languageOf(notes.xyzzy) = %q, want empty", got)
	}
	if got := languageOf(nil); got != "" {
		t.Errorf("languageOf(nil) = %q, want empty", got)
	}
}

// TestLineTokensGo covers a Go file's content lines run by run, exactly: a
// keyword and a declared name, a string and a comment, a number, and a
// plain name run — the same handful of colours diffsyntax.go draws with, read
// off the same lexer.
func TestLineTokensGo(t *testing.T) {
	lex := lexerFor(git.File{Path: "main.go"})
	for _, tt := range []struct {
		name string
		line string
		want []tokenRun
	}{
		{
			name: "keyword and a declared name",
			line: "+func main() {",
			want: []tokenRun{
				{Kind: kindKeyword, Length: len("func")},
				{Kind: kindText, Length: len(" ")},
				{Kind: kindName, Length: len("main")},
				{Kind: kindText, Length: len("() {")},
			},
		},
		{
			name: "string and a comment",
			line: `+	x := "hello" // a comment`,
			want: []tokenRun{
				{Kind: kindText, Length: len("\tx := ")},
				{Kind: kindString, Length: len(`"hello"`)},
				{Kind: kindText, Length: len(" ")},
				{Kind: kindComment, Length: len("// a comment")},
			},
		},
		{
			name: "number",
			line: "+\tn := 42",
			want: []tokenRun{
				{Kind: kindText, Length: len("\tn := ")},
				{Kind: kindNumber, Length: len("42")},
			},
		},
		{
			name: "a plain (undeclared) name beside a declared one",
			line: "+\tfmt.Println(x, n)",
			want: []tokenRun{
				{Kind: kindText, Length: len("\tfmt.")},
				{Kind: kindName, Length: len("Println")},
				{Kind: kindText, Length: len("(x, n)")},
			},
		},
		{
			name: "a line with no run to colour beyond text",
			line: "+}",
			want: []tokenRun{{Kind: kindText, Length: len("}")}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := lineTokens(lex, tt.line)
			assertRuns(t, got, tt.want)

			sum := 0
			for _, r := range got {
				sum += r.Length
			}
			if want := len(tt.line) - 1; sum != want {
				t.Errorf("run lengths sum to %d, want the content's own length %d", sum, want)
			}
		})
	}
}

// assertRuns compares two run slices exactly, kind and length both.
func assertRuns(t *testing.T, got, want []tokenRun) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("runs = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestLineTokensSkipsWhatIsNotCode covers the lines of a section that carry no
// content of the file's own: a diff's header, its index/---/+++ meta, its hunk
// header, and an empty context line all come back with no runs at all, exactly
// the lines diffsyntax.go's own lineRuns leaves unlexed.
func TestLineTokensSkipsWhatIsNotCode(t *testing.T) {
	lex := lexerFor(git.File{Path: "main.go"})
	for _, line := range []string{
		"diff --git a/main.go b/main.go",
		"index 1111111..2222222 100644",
		"--- a/main.go",
		"+++ b/main.go",
		"@@ -1,2 +1,2 @@",
		"",
		"+",
		"-",
		" ",
	} {
		if got := lineTokens(lex, line); len(got) != 0 {
			t.Errorf("lineTokens(%q) = %+v, want no runs", line, got)
		}
	}
}

// TestLineTokensWithoutALexer covers the file-level fallback: a nil lexer,
// which is what a file with no matched language is never even asked to lex
// (slicediff.go only calls lineTokens when a language was found), comes back
// with no runs either, so the function is total rather than assuming its
// caller's own precondition.
func TestLineTokensWithoutALexer(t *testing.T) {
	if got := lineTokens(nil, "+func main() {"); len(got) != 0 {
		t.Errorf("lineTokens(nil, ...) = %+v, want no runs", got)
	}
}

// refusingLexer is a lexer that will not lex, the one failure a matched path
// cannot rule out ahead of time. Mirrors internal/tui/diffsyntax_test.go's own
// fake of the same name.
type refusingLexer struct{ chroma.Lexer }

func (refusingLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	return nil, errors.New("no")
}

// TestLineTokensFallsBackOnALexerThatRefuses covers that failure: the line
// comes back with no runs rather than the wire read going missing.
func TestLineTokensFallsBackOnALexerThatRefuses(t *testing.T) {
	if got := lineTokens(refusingLexer{}, "+\treturn 1"); len(got) != 0 {
		t.Errorf("lineTokens with a refusing lexer = %+v, want no runs", got)
	}
}

// TestKindOfReadsChromasTypes covers the mapping from chroma's own token types
// onto the wire's kinds, category by category, the same cases
// internal/tui/diffsyntax_test.go asserts against the TUI's tokenKind.
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
		{chroma.NameClass, kindName},
		{chroma.NameBuiltin, kindName},
		{chroma.NameNamespace, kindName},
		{chroma.NameAttribute, kindName},
		{chroma.NameTag, kindName},
		{chroma.NameDecorator, kindName},
		{chroma.NameVariable, kindText},
		{chroma.Operator, kindText},
		{chroma.Punctuation, kindText},
		{chroma.Text, kindText},
	} {
		if got := kindOf(tt.token); got != tt.want {
			t.Errorf("kindOf(%s) = %q, want %q", tt.token, got, tt.want)
		}
	}
}

// TestTokenRunJSONRoundTrips covers the wire shape itself: a run is a
// [kind, length] pair, not an object, and reads back to the same value.
func TestTokenRunJSONRoundTrips(t *testing.T) {
	run := tokenRun{Kind: kindKeyword, Length: 4}
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(data); got != `["keyword",4]` {
		t.Errorf("Marshal(%+v) = %s, want [\"keyword\",4]", run, got)
	}

	var back tokenRun
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back != run {
		t.Errorf("round trip = %+v, want %+v", back, run)
	}
}

// TestTokenRunUnmarshalRefusesBadShapes covers the read side refusing what it
// did not write: anything that is not a two-element array.
func TestTokenRunUnmarshalRefusesBadShapes(t *testing.T) {
	var r tokenRun
	for _, data := range []string{
		`{"kind":"text","length":1}`,
		`["text"]`,
		`["text",1,2]`,
		`[1,"text"]`,
		`not json`,
	} {
		if err := json.Unmarshal([]byte(data), &r); err == nil {
			t.Errorf("Unmarshal(%s) = nil error, want a refusal", data)
		}
	}
}
