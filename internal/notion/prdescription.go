package notion

import "strings"

// PRDescriptionHeading is the heading a slice page's pull request description
// is filed under, written by `complete-slice --pr-description` when an agent
// hands its branch back. It is matched case-insensitively, at any heading
// level.
const PRDescriptionHeading = "PR description"

// FollowUpsHeading is the heading `slice-followups` files an agent's proposed
// follow-ups under, one numbered item per proposal; FollowUpsTriagedHeading is
// the one `slice-triage` records the user's decision on them under. Both are
// matched as PRDescriptionHeading is, and both are read by store.PendingFollowUps.
const (
	FollowUpsHeading        = "Follow-ups"
	FollowUpsTriagedHeading = "Follow-ups triaged"
)

// VisualChangesHeading is the heading `slice-visuals` files the images an agent
// rendered of its change under, one numbered item per image: what it shows,
// with where it is nested under that. Matched as PRDescriptionHeading is, and
// read by store.VisualChanges.
const VisualChangesHeading = "Visual changes"

// SentBackHeading is the heading `slice-rework` files its review comments
// under, when it sends a handed-back slice back for another pass.
// RelaunchedHeading is the heading a relaunch (actions.Launch, picking a slice
// back up that was not Todo, or whose brief already carries a task event)
// files its one fixed line under. Both are matched as PRDescriptionHeading is,
// and both are events store.TaskEvents reads back off a slice's body.
const (
	SentBackHeading   = "Sent back"
	RelaunchedHeading = "Relaunched"
)

// NoteHeading is the heading `slice-note` files a note under: a paragraph of
// provenance nat composed, then the note as it was given. Matched as
// PRDescriptionHeading is, and read back by store.TaskEvents.
const NoteHeading = "Note"

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
