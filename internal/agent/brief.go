package agent

import "strings"

// BriefSections renders a slice's own body and the project's conventions as
// the two sections every brief shows them in — the CLI's `start-slice` and
// `next-slice` output, and a board launch's opening prompt — so an agent
// reads the same document however it was told about the slice.
func BriefSections(body, conventions string) string {
	var b strings.Builder
	b.WriteString("## Brief\n\n")
	b.WriteString(Section(body))
	b.WriteString("\n## Project conventions\n\n")
	b.WriteString(Section(conventions))
	return b.String()
}

// Section prints a page body, or says it is empty — an empty heading reads as
// output that got cut off.
func Section(text string) string {
	if text == "" {
		return "_none_\n"
	}
	return text + "\n"
}
