package notion

import "strings"

// headingLevel is the level of a heading block — 1 for heading_1, and so on —
// or 0 for anything that is not a heading.
func headingLevel(b Block) int {
	rest, ok := strings.CutPrefix(b.Type, "heading_")
	if !ok || len(rest) != 1 || rest[0] < '1' || rest[0] > '9' {
		return 0
	}
	return int(rest[0] - '0')
}

// blockPlainText is a block's own rich text, unstyled and without its children.
func blockPlainText(b Block) string {
	var t blockText
	b.decodePayload(&t)
	return PlainText(t.RichText)
}
