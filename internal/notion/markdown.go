package notion

import (
	"strconv"
	"strings"
)

// MaxMarkdownDepth caps how far Markdown descends into nested blocks. It
// matches MaxBlockDepth, so a tree from GetBlockChildren always renders in
// full; blocks below the cap are dropped.
const MaxMarkdownDepth = MaxBlockDepth

// listTypes are the block types that render as list items. Consecutive
// siblings of the same one form a single tight list — no blank line between
// them.
var listTypes = map[string]bool{
	"bulleted_list_item": true,
	"numbered_list_item": true,
	"to_do":              true,
}

// blockText is the payload shape shared by every block whose content is rich
// text. checked belongs to to_do and language to code; both read as their zero
// value elsewhere.
type blockText struct {
	RichText []RichText `json:"rich_text"`
	Checked  bool       `json:"checked"`
	Language string     `json:"language"`
}

// blockRef is the payload shape of the blocks that point at something else: a
// URL for bookmark/embed/link_preview, a title for child_page/child_database.
type blockRef struct {
	URL     string     `json:"url"`
	Title   string     `json:"title"`
	Caption []RichText `json:"caption"`
}

// Markdown renders a block tree as markdown, for glamour to display in the
// Info view. An unknown block type becomes an italic placeholder rather than
// vanishing, so a page never silently loses content. The result ends with a
// newline unless it is empty.
func Markdown(blocks []Block) string {
	out := renderBlocks(blocks, "", MaxMarkdownDepth)
	if out == "" {
		return ""
	}
	return out + "\n"
}

// renderBlocks renders sibling blocks at one indent level and joins them,
// separated by a blank line except within a run of same-type list items.
// Blocks that render to nothing (an empty paragraph, say) are dropped along
// with their separator. The returned string has no trailing newline.
func renderBlocks(blocks []Block, indent string, depth int) string {
	var out strings.Builder
	var prev string
	num := 0
	for _, b := range blocks {
		if b.Type == "numbered_list_item" {
			num++
		} else {
			num = 0
		}
		chunk := renderBlock(b, indent, num, depth)
		if chunk == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n")
			if prev != b.Type || !listTypes[b.Type] {
				out.WriteString("\n")
			}
		}
		out.WriteString(chunk)
		prev = b.Type
	}
	return out.String()
}

// renderBlock renders one block and its children. num is the block's position
// within the current run of numbered list items, and is ignored by every other
// type.
func renderBlock(b Block, indent string, num, depth int) string {
	var t blockText
	b.decodePayload(&t)
	text := inline(t.RichText)

	switch b.Type {
	case "paragraph":
		return withChildren(marked(indent, "", text), b, indent, indent+"  ", depth)

	case "heading_1", "heading_2", "heading_3":
		level := strings.Repeat("#", int(b.Type[len("heading_")]-'0'))
		// A toggle heading's children hang below it, not indented under it.
		return withChildren(marked(indent, level+" ", text), b, indent, indent, depth)

	case "bulleted_list_item":
		return withChildren(marked(indent, "- ", text), b, indent, indent+"  ", depth)

	case "numbered_list_item":
		marker := strconv.Itoa(num) + ". "
		child := indent + strings.Repeat(" ", len(marker))
		return withChildren(marked(indent, marker, text), b, indent, child, depth)

	case "to_do":
		marker := "- [ ] "
		if t.Checked {
			marker = "- [x] "
		}
		return withChildren(marked(indent, marker, text), b, indent, indent+"  ", depth)

	case "quote":
		// Children of a quote are rendered after it, unquoted — nothing this
		// app writes nests under a quote, and re-quoting arbitrary blocks
		// costs more than it is worth.
		return withChildren(quoted(indent, text), b, indent, indent, depth)

	case "code":
		return fenced(indent, t.Language, PlainText(t.RichText))

	case "divider":
		return indent + "---"

	case "toggle":
		// Flattened: the toggle's own text, then its children in line with it.
		return withChildren(marked(indent, "", text), b, indent, indent, depth)

	case "bookmark", "embed", "link_preview":
		var r blockRef
		b.decodePayload(&r)
		label := inline(r.Caption)
		if label == "" {
			label = r.URL
		}
		return marked(indent, "", "["+label+"]("+r.URL+")")

	case "child_page":
		var r blockRef
		b.decodePayload(&r)
		return marked(indent, "", "**Page:** "+r.Title)

	case "child_database":
		var r blockRef
		b.decodePayload(&r)
		return marked(indent, "", "**Database:** "+r.Title)

	default:
		return marked(indent, "", "*[unsupported: "+b.Type+"]*")
	}
}

// withChildren appends a block's rendered children to head. Children indented
// past their parent continue it, so they follow on the next line; children in
// line with it are separate blocks, so they get a blank line.
func withChildren(head string, b Block, indent, childIndent string, depth int) string {
	if depth <= 1 || len(b.Children) == 0 {
		return head
	}
	kids := renderBlocks(b.Children, childIndent, depth-1)
	switch {
	case kids == "":
		return head
	case head == "":
		return kids
	case childIndent == indent:
		return head + "\n\n" + kids
	default:
		return head + "\n" + kids
	}
}

// marked prefixes text with a list marker or heading marker, indenting any
// continuation lines to line up under the first. Empty text with no marker
// renders as nothing, which is how blank spacer paragraphs disappear.
func marked(indent, marker, text string) string {
	cont := indent + strings.Repeat(" ", len(marker))
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if i == 0 {
			lines[i] = strings.TrimRight(indent+marker+ln, " ")
			continue
		}
		lines[i] = strings.TrimRight(cont+ln, " ")
	}
	return strings.Join(lines, "\n")
}

// quoted prefixes every line of text with a blockquote marker.
func quoted(indent, text string) string {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(indent+"> "+ln, " ")
	}
	return strings.Join(lines, "\n")
}

// fenced wraps code in a fence tagged with its language.
func fenced(indent, language, code string) string {
	var out strings.Builder
	out.WriteString(indent + "```" + language)
	for _, ln := range strings.Split(code, "\n") {
		out.WriteString("\n" + strings.TrimRight(indent+ln, " "))
	}
	out.WriteString("\n" + indent + "```")
	return out.String()
}

// inline renders rich text spans as markdown.
func inline(spans []RichText) string {
	var out strings.Builder
	for _, s := range spans {
		out.WriteString(span(s))
	}
	return out.String()
}

// span renders one rich text span, applying its annotations innermost-first so
// the markers nest. Annotations are skipped on blank text, where `**  **`
// would render as literal asterisks instead of styling anything.
func span(s RichText) string {
	text := s.PlainText
	if a := s.Annotations; a != nil && strings.TrimSpace(text) != "" {
		if a.Code {
			text = "`" + text + "`"
		}
		if a.Bold {
			text = "**" + text + "**"
		}
		if a.Italic {
			text = "*" + text + "*"
		}
		if a.Strikethrough {
			text = "~~" + text + "~~"
		}
	}
	if s.Href != "" {
		text = "[" + text + "](" + s.Href + ")"
	}
	return text
}
