package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/craigmjohnston/nat/internal/git"
)

// tokenKind is what one stretch of a line of the diff is, as far as the colour
// it is drawn in goes: the shape of the line it belongs to — which is the whole
// of a line no language was found for, and the +/- that opens one that was —
// or one of the few things a lexer tells apart that are worth a colour of their
// own.
//
// They are kinds rather than styles because a file is lexed once, when the
// branch is read, and the palette can be swapped under the screen afterwards:
// a run holds what it is, and the render asks the styles what that looks like.
type tokenKind int

const (
	kindShape tokenKind = iota
	kindPlain
	kindComment
	kindKeyword
	kindString
	kindNumber
	kindName
)

// tokenRun is a stretch of one line of the diff drawn in a single style. A line
// is several of them once its language is known, and exactly one when it is not.
type tokenRun struct {
	text string
	kind tokenKind
}

// wrapped is one line of the diff broken into the rows it takes at the width it
// is drawn at, each row the runs it is drawn as.
type wrapped [][]tokenRun

// fileSyntax is one file of the diff as the boxes draw its content: whether a
// language was found for it at all, and each of its lines broken into runs.
//
// lexed is what the wash under an added or removed line hangs off: a file with
// no language keeps the green and red it has always been drawn in, and one with
// a language gives that foreground to the code and says added and removed with
// a background instead.
type fileSyntax struct {
	lexed bool
	// lex is the lexer the lines were run through, kept so the lines an expand
	// zone reveals — which are the file's own and not the diff's, and so are not
	// lexed with it — can be run through the same one at the render that draws
	// them.
	lex   chroma.Lexer
	lines [][]tokenRun
}

// highlightFiles lexes a whole diff, in step with its files. It is run once,
// when the branch is read, rather than at every render: the render happens on
// every cursor move and every resize, and lexing a branch's worth of code that
// often would be a great deal of work for a screen that has not changed.
func highlightFiles(files []git.File) []fileSyntax {
	out := make([]fileSyntax, len(files))
	for i, f := range files {
		out[i] = highlightFile(f)
	}
	return out
}

// highlightFile is one file's lines as runs, lexed by the language its path
// names.
func highlightFile(f git.File) fileSyntax {
	lex := lexerFor(f)
	out := fileSyntax{lexed: lex != nil, lex: lex, lines: make([][]tokenRun, len(f.Lines))}
	for i, line := range f.Lines {
		out.lines[i] = lineRuns(lex, line)
	}
	return out
}

// lexerFor is the lexer a file's content is highlighted with, or nil where
// there is none — which is what falls the file back on the shape colouring the
// viewer has always drawn. A file git described rather than diffed has no
// content to lex at all, and a path chroma knows no language for is the other
// half of that: a diff is whatever the branch touched, and plenty of it is
// nothing chroma has ever heard of.
//
// Coalesced, since the runs are drawn one styled string at a time and a lexer
// that emits a token per character would be a line of escape sequences with
// code somewhere inside it.
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

// lineRuns breaks one line of a file's section into the runs it is drawn as.
//
// Only the lines that carry the file's own content are lexed — an added, a
// removed or a context line — and the +/- or space each of them opens with is
// kept out of the lexer and drawn in the line's own shape: it is git's
// character rather than the language's, and a "-" handed to a lexer is an
// operator that was never in the file. Everything else in a section is about
// the change rather than in it, and stays exactly what it was.
func lineRuns(lex chroma.Lexer, line string) []tokenRun {
	if lex == nil {
		return []tokenRun{{text: line}}
	}
	var marker, code string
	switch lineShapeOf(line) {
	case shapeAdd, shapeDel, shapeContext:
		if line == "" {
			return []tokenRun{{text: line}}
		}
		marker, code = line[:1], line[1:]
	default:
		return []tokenRun{{text: line}}
	}
	iter, err := lex.Tokenise(nil, code)
	if err != nil {
		// A lexer that will not lex is one more language the viewer does not
		// know: the line falls back rather than going missing.
		return []tokenRun{{text: line}}
	}
	out := []tokenRun{{text: marker}}
	for _, tok := range iter.Tokens() {
		// chroma ends what it is given with a newline of its own where the text
		// did not have one, and a newline inside a box's row would break it.
		// A token that was nothing else goes altogether: an empty run holds no
		// column of the row, and [wrapRuns] drops it on its way to the render.
		text := strings.Trim(tok.Value, "\r\n")
		// Neighbours that came out the same kind are one run: chroma coalesces
		// by its own token types, which are far finer than the handful of
		// colours here, and every run left standing is an escape sequence in
		// the middle of a line of code.
		kind := kindOf(tok.Type)
		if last := len(out) - 1; out[last].kind == kind {
			out[last].text += text
			continue
		}
		out = append(out, tokenRun{text: text, kind: kind})
	}
	return out
}

// kindOf is which of the diff's few colours a lexer's token takes. It is a
// coarse reading of chroma's own types on purpose: a screen for reading a
// change wants keywords, strings, numbers, names and comments told apart, and
// a colour per token type would be a diff that is harder to read rather than
// easier.
//
// The names are the declared ones alone — a function, a type, a package — since
// colouring every identifier would leave the line with no ordinary text in it
// at all.
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
	return kindPlain
}

// runStyle is the style a run is drawn in: the palette's own colour for what
// the lexer made of it, and the line's shape style for the runs that are not
// the language's — an unlexed line, and the +/- a lexed one opens with.
func (d Diff) runStyle(kind tokenKind, shape lipgloss.Style) lipgloss.Style {
	switch kind {
	case kindPlain:
		return d.styles.SyntaxText
	case kindComment:
		return d.styles.SyntaxComment
	case kindKeyword:
		return d.styles.SyntaxKeyword
	case kindString:
		return d.styles.SyntaxString
	case kindNumber:
		return d.styles.SyntaxNumber
	case kindName:
		return d.styles.SyntaxName
	}
	return shape
}

// wash is a style with the row's background under it, for the pieces of a line
// drawn on the wash an added or removed line carries. It is merged into every
// piece's own style rather than applied to the finished row, because a row is
// several rendered runs and each of them ends with a reset the background would
// not survive.
//
// A nil fill is how a line says it is drawn on no wash at all: the context
// lines of a highlighted file, and every line of one whose language was not
// found.
func wash(s lipgloss.Style, fill color.Color) lipgloss.Style {
	if fill == nil {
		return s
	}
	return s.Background(fill)
}

// wrapRuns breaks a line's runs into the rows they take at width columns, cut
// on the column rather than on a word for the reason [wrapLine] is: a diff is
// code, where the run of spaces at the front of a line is what says where it
// sits.
//
// A run that straddles the cut is split, and both halves keep what it was, so
// the tail of a wrapped string is still drawn as a string. Tabs are expanded
// first, to the spaces the renderer draws them as, so what is measured is what
// is drawn.
func wrapRuns(runs []tokenRun, width int) wrapped {
	var rows wrapped
	var row []tokenRun
	var b strings.Builder
	w := 0
	for _, r := range runs {
		for _, ch := range strings.ReplaceAll(r.text, "\t", tabSpaces) {
			cw := lipgloss.Width(string(ch))
			if width > 0 && w > 0 && w+cw > width {
				rows = append(rows, append(row, tokenRun{text: b.String(), kind: r.kind}))
				row, w = nil, 0
				b.Reset()
			}
			b.WriteRune(ch)
			w += cw
		}
		if b.Len() > 0 {
			row = append(row, tokenRun{text: b.String(), kind: r.kind})
			b.Reset()
		}
	}
	return append(rows, row)
}

// rowText is one wrapped row as the plain text it holds, for the places a row
// is measured or drawn without its colours: its width, and the fill of the row
// the cursor is on.
func rowText(row []tokenRun) string {
	var b strings.Builder
	for _, r := range row {
		b.WriteString(r.text)
	}
	return b.String()
}

// wrapLine breaks one unlexed line into the rows it takes at width columns.
// It is [wrapRuns] over a line that is all one run, so the comment rows drawn
// inside a box wrap exactly as the code above them does.
func wrapLine(line string, width int) []string {
	rows := wrapRuns([]tokenRun{{text: line}}, width)
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = rowText(row)
	}
	return out
}
