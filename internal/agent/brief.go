package agent

import "strings"

// BriefSections renders a slice's own body, its milestone's digest and the
// project's conventions as the three sections every brief shows them in — the
// CLI's `start-slice` and `next-slice` output, and a board launch's opening
// prompt — so an agent reads the same document however it was told about the
// slice.
//
// milestoneDigest is [MilestoneDigest] already rendered by the caller: the
// settled state of the slice's own milestone, in place of the step a session
// used to be told to go and read with `nat info`.
func BriefSections(body, milestoneDigest, conventions string) string {
	var b strings.Builder
	b.WriteString("## Brief\n\n")
	b.WriteString(Section(body))
	b.WriteString("\n## This slice's milestone\n\n")
	b.WriteString(Section(milestoneDigest))
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
