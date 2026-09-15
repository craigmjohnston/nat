package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// PlanMarkdown renders a project's whole plan as markdown: the conventions as
// written, then the milestones in plan order, then the slices under the
// milestone each belongs to — the same document `nat info` prints. It lives
// here rather than in internal/cli, which is where it was first written, so
// a planning launch's prompt can render it too: internal/cli imports
// internal/agent (for its prompt templates), so internal/agent can never
// import internal/cli back without a cycle, and this is the lowest package
// both already depend on.
func PlanMarkdown(p Project, conventions string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", p.Name)
	if conventions != "" {
		fmt.Fprintf(&b, "\n%s\n", conventions)
	}

	b.WriteString("\n## Milestones\n\n")
	if len(p.Milestones) == 0 {
		b.WriteString("_none_\n")
	}
	for _, g := range p.Groups() {
		if g.Milestone == nil {
			continue
		}
		fmt.Fprintf(&b, "- %s. %s — %s\n", planOrder(g.Milestone.Order), g.Milestone.Name, planStatus(string(g.Milestone.Status)))
	}

	b.WriteString("\n## Slices\n\n")
	if len(p.Slices) == 0 {
		b.WriteString("_none_\n")
	}
	first := true
	for _, g := range p.Groups() {
		if len(g.Slices) == 0 {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		first = false
		fmt.Fprintf(&b, "### %s\n\n", g.Name())
		for _, s := range g.Slices {
			fmt.Fprintf(&b, "- %s — %s\n", s.Name, strings.Join(planSliceFacts(s), " · "))
		}
	}
	return b.String()
}

// planSliceFacts is what is worth saying about a slice beside its name: its
// status, whoever holds it, and whatever pull request came of it, each left
// out when there is none rather than printed as an empty field.
func planSliceFacts(s Slice) []string {
	facts := []string{planStatus(s.StatusName)}
	if s.AssigneeName != "" {
		facts = append(facts, s.AssigneeName)
	}
	if s.PRURL != "" {
		facts = append(facts, "PR "+s.PRURL)
	}
	return facts
}

// planOrder prints a milestone's order without a trailing ".0": the orders
// are whole numbers in practice, and fractions only appear when something was
// slotted between two of them.
func planOrder(order float64) string {
	return strconv.FormatFloat(order, 'f', -1, 64)
}

// planStatus names an empty status, which is what a page missing the
// property or carrying an unset select reads as. Printing nothing there would
// leave a line ending in a dash.
func planStatus(status string) string {
	if status == "" {
		return "(no status)"
	}
	return status
}
