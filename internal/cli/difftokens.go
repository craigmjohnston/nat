package cli

// The lexing here is a port of internal/tui/diffsyntax.go's rules — the lexer
// match on a file's path, its token-kind mapping and its line-into-runs split
// — into internal/cli, which cannot import internal/tui (that package is the
// TUI's own). See diffsyntax.go for the full rationale; this file keeps only
// what slice-diff --json needs to hand the same runs out on the wire, as a
// kind and a length rather than a kind and the text, since the text is
// already on the wire as the line itself.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/craigmjohnston/nat/internal/git"
)

// tokenKind is one of the diff viewer's few colours, named as the lowercase
// string the wire carries. Unlike diffsyntax.go's tokenKind this has no
// "shape" member: the marker a line opens with is never part of a wire run
// (tokens cover only the content after it), and a line with no language, or
// one lexing does not reach, is simply an empty run list.
type tokenKind string

const (
	kindText    tokenKind = "text"
	kindComment tokenKind = "comment"
	kindKeyword tokenKind = "keyword"
	kindString  tokenKind = "string"
	kindNumber  tokenKind = "number"
	kindName    tokenKind = "name"
)

// tokenRun is one stretch of a line's content drawn in a single kind, wire
// form: a [kind, length] pair rather than a [kind, text] one, so a run says
// nothing the line above it does not already say.
type tokenRun struct {
	Kind   tokenKind
	Length int
}

// MarshalJSON writes a run as the two-element array the wire shape asks for.
func (r tokenRun) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]any{r.Kind, r.Length})
}

// UnmarshalJSON reads a run back from the [kind, length] pair MarshalJSON
// wrote, which is what lets a test — or anything else in this process reading
// its own JSON back — assert on the typed value rather than the raw array. A
// slice rather than a fixed-size array, so a shape with the wrong element
// count is refused rather than silently truncated the way unmarshalling into
// a Go array would.
func (r *tokenRun) UnmarshalJSON(data []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if len(pair) != 2 {
		return fmt.Errorf("token run: want a [kind, length] pair, got %d elements", len(pair))
	}
	if err := json.Unmarshal(pair[0], &r.Kind); err != nil {
		return err
	}
	return json.Unmarshal(pair[1], &r.Length)
}

// lexerFor is the lexer a file's content is highlighted with, or nil where
// there is none: a described file has no content to lex, and a path chroma
// knows no language for falls back the same way. Ported from
// internal/tui/diffsyntax.go's lexerFor.
func lexerFor(f git.File) chroma.Lexer {
	if f.Binary {
		return nil
	}
	lex := lexers.Match(f.Path)
	if lex == nil {
		return nil
	}
	return chroma.Coalesce(lex)
}

// languageOf is the lexer name a file was matched to, empty where none was.
func languageOf(lex chroma.Lexer) string {
	if lex == nil {
		return ""
	}
	return lex.Config().Name
}

// lineTokens is one line of a file's section broken into wire runs over its
// content after the diff's own +/-/space prefix. Only an added, removed or
// context line is lexed — the same three [lineShapeOf] treats as the file's
// own content in diffsyntax.go — and a line lexing does not reach (no lexer,
// an empty line, a lexer that refuses to tokenise) comes back an empty array
// rather than none at all: this only ever runs against a file with a matched
// language, and the wire says a header, a hunk or an empty line got no colour
// the same way whether or not the file around it did.
func lineTokens(lex chroma.Lexer, line string) []tokenRun {
	out := []tokenRun{}
	if lex == nil {
		return out
	}
	var code string
	switch lineShapeOf(line) {
	case shapeAdd, shapeDel, shapeContext:
		if line == "" {
			return out
		}
		code = line[1:]
	default:
		return out
	}
	if code == "" {
		return out
	}
	iter, err := lex.Tokenise(nil, code)
	if err != nil {
		// A lexer that will not lex is one more language the wire does not
		// colour: the line is left with no runs, exactly as an unmatched one.
		return out
	}
	for _, tok := range iter.Tokens() {
		// chroma ends what it is given with a newline of its own where the
		// text did not have one; a run's length is measured in bytes of the
		// content, so it slices the same UTF-8 string the line already is,
		// and that trailing newline is not part of it.
		text := strings.Trim(tok.Value, "\r\n")
		kind := kindOf(tok.Type)
		n := len(text)
		if last := len(out) - 1; last >= 0 && out[last].Kind == kind {
			out[last].Length += n
			continue
		}
		out = append(out, tokenRun{Kind: kind, Length: n})
	}
	return out
}

// kindOf is which of the diff's few colours a lexer's token takes. Ported
// verbatim from internal/tui/diffsyntax.go's kindOf, with kindPlain renamed
// kindText for the wire's own word.
func kindOf(t chroma.TokenType) tokenKind {
	switch t {
	case chroma.NameFunction, chroma.NameClass, chroma.NameBuiltin,
		chroma.NameNamespace, chroma.NameAttribute, chroma.NameTag, chroma.NameDecorator:
		return kindName
	}
	switch {
	case t.InCategory(chroma.Comment):
		return kindComment
	case t.InCategory(chroma.Keyword):
		return kindKeyword
	case t.InSubCategory(chroma.LiteralString):
		return kindString
	case t.InSubCategory(chroma.LiteralNumber):
		return kindNumber
	}
	return kindText
}

// lineShape and lineShapeOf are ported from internal/tui/diff.go, which
// classifies a diff line by its prefix — the header lines tested before the
// +/- ones they look like, since "+++ b/main.go" is a header and not three
// added characters — so this file lexes exactly the lines diffsyntax.go does
// and none of the ones it leaves alone.
type lineShape int

const (
	shapeContext lineShape = iota
	shapeAdd
	shapeDel
	shapeFile
	shapeMeta
	shapeHunk
)

func lineShapeOf(line string) lineShape {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		return shapeFile
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return shapeMeta
	case strings.HasPrefix(line, "@@"):
		return shapeHunk
	case strings.HasPrefix(line, "+"):
		return shapeAdd
	case strings.HasPrefix(line, "-"):
		return shapeDel
	case strings.HasPrefix(line, " "), line == "":
		return shapeContext
	default:
		return shapeMeta
	}
}
