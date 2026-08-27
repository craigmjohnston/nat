package notion

import "strings"

// PRDescriptionHeading is the heading a slice page's pull request description
// is filed under, written by `complete-slice --pr-description` when an agent
// hands its branch back. It is matched case-insensitively, at any heading
// level, exactly as WishlistHeading is.
const PRDescriptionHeading = "PR description"

// PRDescriptionOf is the pull request description an agent left on a slice
// page: the blocks between its PR description heading and the next heading of
// the same or higher level, rendered as markdown. A page with no such heading —
// every hand-back written before there was a flag for one — comes back empty,
// which is the caller's cue to let gh fill the pull request from the commits
// instead.
//
// The last such section wins rather than the first: a slice handed back twice —
// reviewed, commented on, pushed again — carries one section per hand-back, and
// the description of the work as it now stands is the one written last.
func PRDescriptionOf(blocks []Block) string {
	var section []Block
	level := 0
	for _, b := range blocks {
		h := headingLevel(b)
		if h > 0 && h <= level {
			level = 0
		}
		if h > 0 && strings.EqualFold(strings.TrimSpace(blockPlainText(b)), PRDescriptionHeading) {
			level, section = h, nil
			continue
		}
		if level > 0 {
			section = append(section, b)
		}
	}
	return strings.TrimSpace(Markdown(section))
}
