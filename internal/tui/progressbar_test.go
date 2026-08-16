package tui

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/domain"
)

// sgr matches the colour escapes lipgloss wraps rendered text in; the plain
// text under them is what most of these assertions are about.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain is a rendered string with its styling taken off.
func plain(s string) string { return sgr.ReplaceAllString(s, "") }

// barOf is a rendered bar — one line, and no label under it — with its styling
// taken off.
func barOf(rendered string) string { return plain(rendered) }

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
	// Three segments in three cells: one cell each, no room for gaps.
	styles := DefaultStyles()
	rendered := RenderProgressBar(styles, 3, segs([2]int{1, 1}, [2]int{0, 1}, [2]int{0, 1}))

	bar := barOf(rendered)
	if strings.Contains(bar, barBoundary) {
		t.Errorf("bar %q still draws gaps at width 3", bar)
	}
	want := styles.BarFillDone.Render(barCell) +
		styles.BarEmpty.Render(barCell) + styles.BarEmpty.Render(barCell)
	if got := strings.SplitN(rendered, "\n", 2)[0]; got != want {
		t.Errorf("bar = %q, want %q", got, want)
	}
}

func TestProgressBarCollapsesWhenThereAreMoreMilestonesThanColumns(t *testing.T) {
	// Four milestones, two of them done, in three cells: one bar over the lot,
	// half of it filled — in the bright fill, since the project as a whole is
	// not finished history.
	styles := DefaultStyles()
	rendered := RenderProgressBar(styles, 3,
		segs([2]int{1, 1}, [2]int{1, 1}, [2]int{0, 1}, [2]int{0, 1}))

	want := styles.BarFill.Render(barCell) + styles.BarEmpty.Render(strings.Repeat(barCell, 2))
	if got := strings.SplitN(rendered, "\n", 2)[0]; got != want {
		t.Errorf("bar = %q, want %q", got, want)
	}
}

func TestProgressBarWithNoSlicesAtAllIsEmptyRatherThanBlank(t *testing.T) {
	for _, segments := range [][]ProgressSegment{nil, segs([2]int{0, 0}, [2]int{0, 0})} {
		rendered := RenderProgressBar(DefaultStyles(), 5, segments)
		if got, want := barOf(rendered), strings.Repeat(barCell, 5); got != want {
			t.Errorf("bar = %q, want %q", got, want)
		}
	}
}

func TestProgressBarDrawsASingleMilestoneWithoutBoundaries(t *testing.T) {
	styles := DefaultStyles()
	rendered := RenderProgressBar(styles, 8, segs([2]int{1, 4}))

	want := styles.BarFill.Render(strings.Repeat(barCell, 2)) +
		styles.BarEmpty.Render(strings.Repeat(barCell, 6))
	if got := strings.SplitN(rendered, "\n", 2)[0]; got != want {
		t.Errorf("bar = %q, want %q", got, want)
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

// The milestone the heading names is the earliest one that has been started
// and not finished — the work the plan is in, past the milestones behind it.
func TestCurrentSegmentNameIsTheEarliestStartedAndUnfinishedMilestone(t *testing.T) {
	segments := segs([2]int{2, 2}, [2]int{0, 0}, [2]int{4, 10}, [2]int{0, 3})
	segments[2].Name = "M3: Notion client"

	if got, want := CurrentSegmentName(segments), "M3: Notion client"; got != want {
		t.Errorf("CurrentSegmentName() = %q, want %q", got, want)
	}
}

// A milestone with a slice claimed but none done yet has been started too: it
// is where the work is, whatever the tally says.
func TestCurrentSegmentNameCountsAClaimedSliceAsStarted(t *testing.T) {
	segments := segs([2]int{0, 4}, [2]int{0, 4})
	segments[1].Progress = domain.Progress{Claimed: 1, Todo: 3, Total: 4}

	if got, want := CurrentSegmentName(segments), "M2"; got != want {
		t.Errorf("CurrentSegmentName() = %q, want %q", got, want)
	}
}

// Nothing started at all: the name falls back to the earliest milestone with
// work left in it, which is the one the next slice comes out of. Milestones
// with no slices are passed over — there is nothing in them to begin.
func TestCurrentSegmentNameFallsBackToTheEarliestMilestoneWithWorkLeft(t *testing.T) {
	if got, want := CurrentSegmentName(segs([2]int{0, 0}, [2]int{0, 3}, [2]int{0, 2})), "M2"; got != want {
		t.Errorf("CurrentSegmentName() = %q, want %q", got, want)
	}
}

func TestCurrentSegmentNameIsEmptyWithNoMilestoneToPointAt(t *testing.T) {
	for _, segments := range [][]ProgressSegment{
		nil,
		segs([2]int{0, 0}, [2]int{0, 0}),
		segs([2]int{2, 2}, [2]int{3, 3}),
	} {
		if got := CurrentSegmentName(segments); got != "" {
			t.Errorf("CurrentSegmentName(%+v) = %q, want no milestone named", segments, got)
		}
	}
}

func TestProgressBarMergesFinishedMilestonesIntoOneQuietRun(t *testing.T) {
	styles := DefaultStyles()
	rendered := RenderProgressBar(styles, 8, segs([2]int{2, 2}, [2]int{4, 4}, [2]int{1, 2}))

	// The one gap — before the unfinished milestone — leaves 7 cells, shared
	// 2/3/2. The finished segments draw in the dimmed fill with nothing
	// between them; the unfinished one is half bright fill, half empty.
	want := styles.BarFillDone.Render(strings.Repeat(barCell, 2)) +
		styles.BarFillDone.Render(strings.Repeat(barCell, 3)) +
		barBoundary +
		styles.BarFill.Render(barCell) + styles.BarEmpty.Render(barCell)
	if got := strings.SplitN(rendered, "\n", 2)[0]; got != want {
		t.Errorf("bar = %q, want %q", got, want)
	}
}

func TestProgressBarPartsUnfinishedMilestonesWithAGap(t *testing.T) {
	bar := barOf(RenderProgressBar(DefaultStyles(), 5, segs([2]int{0, 2}, [2]int{0, 2})))

	if want := strings.Repeat(barCell, 2) + barBoundary + strings.Repeat(barCell, 2); bar != want {
		t.Errorf("bar = %q, want %q", bar, want)
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
