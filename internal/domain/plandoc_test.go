package domain

import "testing"

// TestPlanMarkdownRendersTheWholePlan covers the ordinary path: conventions,
// two milestones in plan order, their slices (one with an assignee and a
// pull request, one bare), and a trailing Unassigned group.
func TestPlanMarkdownRendersTheWholePlan(t *testing.T) {
	p := NewProject("proj-1", "nat", []Milestone{
		{ID: "m1", Name: "M1: Client", Order: 0},
		{ID: "m2", Name: "M2: Board", Order: 1},
		{ID: "m3", Name: "M3: Empty", Order: 2},
	}, []Slice{
		{ID: "s1", Name: "Notion client", MilestoneID: "m1", Status: SliceDone, StatusName: "Done",
			AssigneeName: "Craig Johnston", PRURL: "https://github.com/nat/pull/1"},
		{ID: "s2", Name: "Render the board", MilestoneID: "m2", Status: SliceTodo, StatusName: "Todo"},
		{ID: "s3", Name: "Stray idea", Status: SliceTodo},
	})

	want := `# nat

Branch per slice.

## Milestones

- 0. M1: Client — Done
- 1. M2: Board — Queued
- 2. M3: Empty — Queued

## Slices

### M1: Client

- Notion client — Done · Craig Johnston · PR https://github.com/nat/pull/1

### M2: Board

- Render the board — Todo

### Unassigned

- Stray idea — (no status)
`
	if got := PlanMarkdown(p, "Branch per slice."); got != want {
		t.Errorf("output =\n%s\nwant:\n%s", got, want)
	}
}

// A project with no milestones and no slices at all still renders both
// headings, saying so plainly rather than leaving them blank.
func TestPlanMarkdownWithNoMilestonesOrSlices(t *testing.T) {
	p := NewProject("proj-1", "nat", nil, nil)
	want := `# nat

## Milestones

_none_

## Slices

_none_
`
	if got := PlanMarkdown(p, ""); got != want {
		t.Errorf("output =\n%s\nwant:\n%s", got, want)
	}
}

// Empty conventions print no section at all, rather than a blank line where
// they would have gone.
func TestPlanMarkdownWithNoConventions(t *testing.T) {
	p := NewProject("proj-1", "nat", nil, nil)
	got := PlanMarkdown(p, "")
	if got[:len("# nat\n\n## Milestones")] != "# nat\n\n## Milestones" {
		t.Errorf("output = %q, want no blank conventions section before the milestones", got)
	}
}
