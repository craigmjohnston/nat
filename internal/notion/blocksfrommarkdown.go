package notion

import "strings"

// maxRichTextLength is Notion's limit on one rich_text span's content. It is
// enforced per span rather than per block, so text past it is split across
// more spans of the same block rather than sent as one span Notion refuses.
const maxRichTextLength = 2000

// BlocksFromMarkdown converts a brief written in this app's own markdown
// dialect — paragraphs, bulleted and numbered lists, and headings, and
// nothing else — into the blocks a page body is written as. It is the reverse
// of [Markdown], covering exactly what a brief is drafted in and nothing
// more: no nesting, no inline emphasis, no quotes or code.
//
// A blank line ends whatever block is open, so consecutive non-blank lines
// that are not a list item or a heading join into one paragraph, newlines and
// all — the same shape [Markdown] reads a multi-line paragraph back into. Long
// text is split under Notion's per-span character limit rather than refused.
func BlocksFromMarkdown(text string) []map[string]any {
	var blocks []map[string]any
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		blocks = append(blocks, textBlock("paragraph", strings.Join(para, "\n")))
		para = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush()
		case mdHeadingLevel(trimmed) > 0:
			flush()
			level := mdHeadingLevel(trimmed)
			blocks = append(blocks, textBlock(headingType(level), strings.TrimSpace(trimmed[level+1:])))
		case isBullet(trimmed):
			flush()
			blocks = append(blocks, textBlock("bulleted_list_item", strings.TrimSpace(trimmed[2:])))
		case isNumbered(trimmed):
			flush()
			blocks = append(blocks, textBlock("numbered_list_item", numberedText(trimmed)))
		default:
			para = append(para, trimmed)
		}
	}
	flush()
	return blocks
}

// mdHeadingLevel reads the number of leading '#' markers a heading line opens
// with, so long as one to six are followed by a space; anything else is not a
// heading and reads as 0.
func mdHeadingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

// headingType maps a markdown heading level onto the three Notion has, the
// way [Markdown] reads them back: a level past heading_3 flattens to it
// rather than being refused.
func headingType(level int) string {
	if level > 3 {
		level = 3
	}
	return "heading_" + string(rune('0'+level))
}

// isBullet reports whether a line opens a bulleted list item.
func isBullet(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

// isNumbered reports whether a line opens a numbered list item: one or more
// digits, a dot, then a space. The number itself is not read — a run of
// numbered_list_item blocks renumbers on its own.
func isNumbered(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
}

// numberedText is the text of a numbered list item, past its marker.
func numberedText(line string) string {
	i := strings.Index(line, ". ")
	return strings.TrimSpace(line[i+2:])
}

// textBlock builds a block of the given type holding text as one or more
// rich_text spans, chunked under Notion's per-span character limit.
func textBlock(blockType, text string) map[string]any {
	return map[string]any{
		"object":  "block",
		"type":    blockType,
		blockType: map[string]any{"rich_text": richTextSpans(text)},
	}
}

// richTextSpans splits text into spans of at most maxRichTextLength runes
// each, so text longer than Notion's own limit still applies rather than
// being refused. Empty text is one empty span, which is what an empty block
// needs to be valid.
func richTextSpans(text string) []map[string]any {
	runes := []rune(text)
	if len(runes) == 0 {
		return []map[string]any{textSpan("")}
	}
	var spans []map[string]any
	for len(runes) > 0 {
		n := min(len(runes), maxRichTextLength)
		spans = append(spans, textSpan(string(runes[:n])))
		runes = runes[n:]
	}
	return spans
}

// textSpan is one rich_text span of plain text.
func textSpan(content string) map[string]any {
	return map[string]any{
		"type": "text",
		"text": map[string]any{"content": content},
	}
}
