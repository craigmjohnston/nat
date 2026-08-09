package tui

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
)

// sgr matches the colour escapes lipgloss wraps rendered text in; the plain
// text under them is what most of these assertions are about.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain is a rendered string with its styling taken off.
func plain(s string) string { return sgr.ReplaceAllString(s, "") }

// barOf and labelOf split a rendered bar into its two lines.
func barOf(rendered string) string   { return plain(strings.SplitN(rendered, "\n", 2)[0]) }
func labelOf(rendered string) string { return plain(strings.SplitN(rendered, "\n", 2)[1]) }

// segs builds segments from done/total pairs, named M1, M2, … so that the
// widths under test are readable at the call site.
func segs(pairs ...[2]int) []ProgressSegment {
	out := make([]ProgressSegment, len(pairs))
	for i, p := range pairs {
		out[i] = ProgressSegment{
			Name:     "M" + string(rune('1'+i)),
			Progress: domain.Progress{Done: p[0], Todo: p[1] - p[0], Total: p[1]},
		}
	}
	return out
}

// segWidths is the width of each drawn segment, read back off the bar line by
// splitting on the boundary rune.
func segWidths(bar string) []int {
	parts := strings.Split(bar, barBoundary)
	widths := make([]int, len(parts))
	for i, p := range parts {
		widths[i] = lipgloss.Width(p)
	}
	return widths
}

func TestSegmentsOfCarriesEachGroupsNameAndTally(t *testing.T) {
	p := testProject()
	got := SegmentsOf(p.Groups())

	want := []ProgressSegment{
		{Name: "M1: Config", Progress: domain.Progress{Done: 2, Total: 2}},
		{Name: "M2: Board", Progress: domain.Progress{Done: 1, Claimed: 1, Todo: 1, Total: 3}},
		{Name: "M3: Mutations"},
		{Name: domain.UnassignedName, Progress: domain.Progress{Total: 1}},
	}
	if len(got) != len(want) {
		t.Fatalf("SegmentsOf() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestProgressBarIsExactlyAsWideAsAsked(t *testing.T) {
	all := segs([2]int{2, 2}, [2]int{1, 3}, [2]int{0, 0}, [2]int{0, 6}, [2]int{3, 4})
	for _, width := range []int{1, 2, 3, 4, 7, 12, 40, 81} {
		bar := barOf(RenderProgressBar(DefaultStyles(), width, all))
		if got := lipgloss.Width(bar); got != width {
			t.Errorf("bar at width %d is %d wide: %q", width, got, bar)
		}
	}
}

func TestProgressBarSizesSegmentsByTheirSliceCount(t *testing.T) {
	// 3 boundaries leave 40 cells for 10+20+5+5 slices: 10, 20, 5, 5.
	bar := barOf(RenderProgressBar(DefaultStyles(), 43,
		segs([2]int{0, 10}, [2]int{0, 20}, [2]int{0, 5}, [2]int{0, 5})))

	want := []int{10, 20, 5, 5}
	if got := segWidths(bar); !equalInts(got, want) {
		t.Errorf("segment widths = %v, want %v", got, want)
	}
}

func TestProgressBarGivesEveryMilestoneWithSlicesACell(t *testing.T) {
	// A milestone of one slice beside one of a hundred still gets a cell, paid
	// for out of the big one.
	bar := barOf(RenderProgressBar(DefaultStyles(), 6, segs([2]int{0, 100}, [2]int{0, 1})))

	if got := segWidths(bar); !equalInts(got, []int{4, 1}) {
		t.Errorf("segment widths = %v, want %v", got, []int{4, 1})
	}
}

func TestProgressBarShrinksTheWidestSegmentToPayForTheFloor(t *testing.T) {
	// 3 cells between two boundaries, and 100+1+1 slices: the big milestone
	// wants all three, but the other two need one each and are given it —
	// wherever in the plan the big one sits.
	for _, segments := range [][]ProgressSegment{
		segs([2]int{0, 100}, [2]int{0, 1}, [2]int{0, 1}),
		segs([2]int{0, 1}, [2]int{0, 100}, [2]int{0, 1}),
	} {
		bar := barOf(RenderProgressBar(DefaultStyles(), 5, segments))
		if got := segWidths(bar); !equalInts(got, []int{1, 1, 1}) {
			t.Errorf("segment widths = %v, want %v", got, []int{1, 1, 1})
		}
	}
}

func TestProgressBarSkipsMilestonesWithNoSlices(t *testing.T) {
	bar := barOf(RenderProgressBar(DefaultStyles(), 21,
		segs([2]int{0, 5}, [2]int{0, 0}, [2]int{0, 5})))

	if got := segWidths(bar); !equalInts(got, []int{10, 10}) {
		t.Errorf("segment widths = %v, want %v", got, []int{10, 10})
	}
}

func TestProgressBarDropsBoundariesBeforeItDropsCells(t *testing.T) {
	// Three segments in three cells: one cell each, no room for boundaries.
	bar := barOf(RenderProgressBar(DefaultStyles(), 3,
		segs([2]int{1, 1}, [2]int{0, 1}, [2]int{0, 1})))

	if strings.Contains(bar, barBoundary) {
		t.Errorf("bar %q still draws boundaries at width 3", bar)
	}
	if want := barFilled + barEmpty + barEmpty; bar != want {
		t.Errorf("bar = %q, want %q", bar, want)
	}
}

func TestProgressBarCollapsesWhenThereAreMoreMilestonesThanColumns(t *testing.T) {
	// Four milestones, two of them done, in three cells: one bar over the lot,
	// half of it filled.
	bar := barOf(RenderProgressBar(DefaultStyles(), 4,
		segs([2]int{1, 1}, [2]int{1, 1}, [2]int{0, 1}, [2]int{0, 1})))

	if want := strings.Repeat(barFilled, 2) + strings.Repeat(barEmpty, 2); bar != want {
		t.Errorf("bar = %q, want %q", bar, want)
	}
}

func TestProgressBarWithNoSlicesAtAllIsEmptyRatherThanBlank(t *testing.T) {
	for _, segments := range [][]ProgressSegment{nil, segs([2]int{0, 0}, [2]int{0, 0})} {
		rendered := RenderProgressBar(DefaultStyles(), 5, segments)
		if got, want := barOf(rendered), strings.Repeat(barEmpty, 5); got != want {
			t.Errorf("bar = %q, want %q", got, want)
		}
		if got, want := labelOf(rendered), "0/0"; got != want {
			t.Errorf("label = %q, want %q", got, want)
		}
	}
}

func TestProgressBarDrawsASingleMilestoneWithoutBoundaries(t *testing.T) {
	bar := barOf(RenderProgressBar(DefaultStyles(), 8, segs([2]int{1, 4})))

	if want := strings.Repeat(barFilled, 2) + strings.Repeat(barEmpty, 6); bar != want {
		t.Errorf("bar = %q, want %q", bar, want)
	}
}

func TestProgressBarAtZeroWidthDrawsNothing(t *testing.T) {
	for _, width := range []int{0, -1} {
		if got := RenderProgressBar(DefaultStyles(), width, segs([2]int{1, 2})); got != "" {
			t.Errorf("bar at width %d = %q, want empty", width, got)
		}
	}
}

func TestProgressBarFillsOnlyAFinishedSegmentCompletely(t *testing.T) {
	tests := []struct {
		name     string
		fraction float64
		width    int
		want     int
	}{
		{"nothing done", 0, 4, 0},
		{"a quarter", 0.25, 4, 1},
		{"rounds down", 0.7, 4, 2},
		{"all but one slice keeps a cell empty", 0.99, 4, 3},
		{"done fills it", 1, 4, 4},
		{"nearly done in one cell shows nothing", 0.9, 1, 0},
		{"done in one cell fills it", 1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filledCells(tt.fraction, tt.width); got != tt.want {
				t.Errorf("filledCells(%v, %d) = %d, want %d", tt.fraction, tt.width, got, tt.want)
			}
		})
	}
}

func TestProgressBarLabelPointsAtTheFirstUnfinishedMilestone(t *testing.T) {
	segments := segs([2]int{2, 2}, [2]int{0, 0}, [2]int{4, 10}, [2]int{0, 3})
	segments[2].Name = "M3: Notion client"

	if got, want := labelOf(RenderProgressBar(DefaultStyles(), 40, segments)), "6/15 · M3: Notion client"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}

func TestProgressBarLabelNamesNoMilestoneWhenEverythingIsDone(t *testing.T) {
	rendered := RenderProgressBar(DefaultStyles(), 20, segs([2]int{2, 2}, [2]int{3, 3}))

	if got, want := labelOf(rendered), "5/5"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}

func TestProgressBarLabelIsTruncatedToTheBarWidth(t *testing.T) {
	segments := segs([2]int{0, 4})
	segments[0].Name = "M1: a very long milestone name indeed"

	label := labelOf(RenderProgressBar(DefaultStyles(), 10, segments))
	if got := lipgloss.Width(label); got > 10 {
		t.Errorf("label %q is %d wide, want at most 10", label, got)
	}
	if !strings.HasPrefix(label, "0/4 · M1:") {
		t.Errorf("label = %q, want it to start with the tally", label)
	}
}

func TestProgressBarAlternatesTheHueOfAdjacentSegments(t *testing.T) {
	styles := DefaultStyles()
	rendered := RenderProgressBar(styles, 9, segs([2]int{2, 2}, [2]int{4, 4}, [2]int{1, 1}))

	// Widths 2, 4 and 1, all filled: the middle one in the alternate hue.
	want := styles.BarFill.Render(strings.Repeat(barFilled, 2)) +
		styles.BarBoundary.Render(barBoundary) +
		styles.BarFillAlt.Render(strings.Repeat(barFilled, 4)) +
		styles.BarBoundary.Render(barBoundary) +
		styles.BarFill.Render(barFilled)
	if got := strings.SplitN(rendered, "\n", 2)[0]; got != want {
		t.Errorf("bar = %q, want %q", got, want)
	}
}

func TestProgressBarRendersTheProjectAtSeveralWidths(t *testing.T) {
	p := testProject()
	segments := SegmentsOf(p.Groups())

	for _, tt := range []struct {
		name  string
		width int
	}{
		{"progressbar-wide", 60},
		{"progressbar-medium", 24},
		{"progressbar-narrow", 8},
		{"progressbar-tiny", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			golden(t, tt.name, RenderProgressBar(DefaultStyles(), tt.width, segments))
		})
	}
}

// equalInts reports whether two width slices match.
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
