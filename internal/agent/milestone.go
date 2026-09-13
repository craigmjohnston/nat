package agent

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
)

// MaxHandbackSummary caps how much of a Done sibling's hand-back summary the
// milestone digest carries, collapsed to one line first, so one meandering
// old summary cannot flood the prompt on its own.
const MaxHandbackSummary = 400

// MilestoneDigest renders the settled state of a slice's own milestone: every
// sibling slice as one line, its title and status, and — for each Done one —
// the hand-back summary a session already left on it. It is what a launch
// hands over in place of the "read the other slices in this milestone" step a
// session used to be told to run itself: the plan is already in the
// launcher's hand by the time a slice is claimed, so a design decision a Done
// sibling already settled comes with the brief rather than being left for the
// agent to go and dig up with `nat info`.
//
// siblings is every other slice under the milestone, in plan order —
// deliberately not the slice being launched itself, which the rest of the
// brief already names; summaries is the hand-back summary already read for
// each Done one, keyed by slice ID (see [store.HandbackSummaryOf]) — a page
// fetch per Done sibling, so it is done by the caller and handed in rather
// than by the digest itself. A slice filed under no milestone, or a milestone
// with no other slices under it yet, renders nothing.
func MilestoneDigest(milestone domain.Milestone, siblings []domain.Slice, summaries map[string]string) string {
	if milestone.Name == "" || len(siblings) == 0 {
		return ""
	}
	lines := []string{milestone.Name, ""}
	for _, s := range siblings {
		status := s.StatusName
		if status == "" {
			status = string(s.Status)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", status, s.Name))
		if s.Status != domain.SliceDone {
			continue
		}
		if summary := capHandbackSummary(summaries[s.ID]); summary != "" {
			lines = append(lines, "  "+summary)
		}
	}
	return strings.Join(lines, "\n")
}

// capHandbackSummary collapses a hand-back summary to one line and truncates
// it to MaxHandbackSummary runes, so a summary that ran long — or was never
// meant to be read back verbatim — cannot flood the prompt on its own.
func capHandbackSummary(summary string) string {
	summary = strings.Join(strings.Fields(summary), " ")
	if summary == "" {
		return ""
	}
	r := []rune(summary)
	if len(r) <= MaxHandbackSummary {
		return summary
	}
	return string(r[:MaxHandbackSummary]) + "…"
}
